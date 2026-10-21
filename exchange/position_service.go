package exchange

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"
	"trade_activity_gui/hub"
)

// priceChangeWindow — окно, за которое считается скорость изменения цены.
// minPriceSampleAge — минимальный возраст самой старой точки в окне, при котором
// расчёт производится (иначе результат слишком шумный на коротком интервале).
const (
	priceChangeWindow = 60 * time.Second
	minPriceSampleAge = 1 * time.Second
)

type pricePoint struct {
	ts    time.Time
	price float64
}

type Position struct {
	Symbol             string
	CreatedTime        string
	UpdatedTime        string
	PositionIdx        int
	Side               string
	Size               float64
	EntryPrice         float64
	UnrealisedPnl      float64
	CurrentPrice       float64
	CurrentValue       float64
	PriceChangePercent float64 // скорость изменения цены, % в минуту, за последние priceChangeWindow
	LimitCloseActive   bool    // true, пока идёт автоматическое закрытие позиции лимитками
	LimitCloseProgress float64 // 0-100, % от стартового объёма, уже закрытого лимитками
	LimitCloseError    string  // причина, по которой автозакрытие остановилось само (не по кнопке "Стоп")
}

// fillRecord — одно уменьшение размера позиции, зафиксированное во время работы джоба,
// с ценой ордера, который в этот момент был выставлен на бирже. Своей WS-подписки на
// экзекуции у нас нет, поэтому это приближение: цена берётся с текущего резидентного
// ордера джоба на момент, когда WS-стрим позиции сообщил об уменьшении Size.
type fillRecord struct {
	price float64
	qty   float64
}

// LimitCloseSummary — итог законченного джоба лимитного закрытия. Хранится отдельно от
// Position: сама позиция при полном закрытии удаляется из positionLong/positionShort
// (см. SubscribePositionStart), а сводку по уже завершённой продаже нужно показывать и
// после этого.
type LimitCloseSummary struct {
	Symbol          string
	DurationSeconds float64
	StartSize       float64
	FilledQty       float64
	Progress        float64 // 0-100
	MinPrice        float64
	MaxPrice        float64
	AvgPrice        float64
	FillCount       int
}

// limitCloseJob — состояние одного запущенного "закрытия позиции лимитками" (см.
// StartLimitClose). Поля, кроме reprice/ctx/cancel/done, защищены ps.mu — так же,
// как и поля *Position, по тем же соглашениям, отдельный мьютекс не заводим.
type limitCloseJob struct {
	symbol      string
	orderSide   string // сторона closing-ордера (противоположна стороне позиции)
	positionIdx int    // 0 в One-Way Mode, 1/2 (Buy/Sell) в Hedge Mode — см. Position.PositionIdx
	startSize   float64
	tickSize    float64
	qtyStep     float64
	startTime   time.Time

	orderID    string
	orderPrice float64
	fills      []fillRecord

	// Троттлинг обращений к бирже и счётчик подряд идущих неудач — см.
	// processLimitCloseTick/registerLimitCloseFailure. Ликвидные пары шлют тики по
	// несколько раз в секунду, без этого джоб мог бы стучаться в API на каждый тик.
	lastAttempt   time.Time
	amendFailures int

	// finished — true после первого вызова finishLimitCloseJob. Джоб может завершиться
	// сам (позиция закрылась/повторные неудачи) ровно в момент, когда пользователь жмёт
	// "Стоп" — без этого флага finishLimitCloseJob выполнился бы второй раз (лишний
	// CancelOrder на уже неактуальный ордер, задвоенный лог).
	finished bool

	// Небуферизованный (буфер 1) сигнал воркеру перепроверить состояние — шлётся из
	// тикер- и позиционного дата-хендлеров без блокировки на отправке.
	reprice chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

type PositionService struct {
	wg       *sync.WaitGroup
	log      *slog.Logger
	exchange Exchange
	hub      *hub.Hub

	mu            sync.RWMutex
	positionLong  map[string]*Position
	positionShort map[string]*Position

	// Подписки на тикеры с функцией отписки
	subscribedTickers  map[string]func() error
	subscribedPosition func() error

	// История цен по символу для расчёта скорости изменения (PriceChangePercent)
	priceHistory map[string][]pricePoint

	// Лучшая цена bid/ask по символу — из тикера, нужна для репрайсинга ордеров
	// в limitCloseJobs.
	bestBid map[string]float64
	bestAsk map[string]float64

	// Активные джобы автоматического закрытия позиции лимитками, ключ — символ.
	limitCloseJobs map[string]*limitCloseJob
	// Сводки по джобам, завершившимся полным закрытием позиции — переживают удаление
	// самой позиции из positionLong/positionShort, см. LimitCloseSummary.
	limitCloseSummaries map[string]*LimitCloseSummary

	positionStreamErrorCritical chan error
	tickerStreamErrorCritical   chan error
}

func NewPositionService(exchange Exchange, logger *slog.Logger, hub *hub.Hub) *PositionService {
	return &PositionService{
		wg:                          &sync.WaitGroup{},
		log:                         logger.With("component", "PositionService"),
		exchange:                    exchange,
		hub:                         hub,
		positionLong:                make(map[string]*Position),
		positionShort:               make(map[string]*Position),
		subscribedTickers:           make(map[string]func() error),
		priceHistory:                make(map[string][]pricePoint),
		bestBid:                     make(map[string]float64),
		bestAsk:                     make(map[string]float64),
		limitCloseJobs:              make(map[string]*limitCloseJob),
		limitCloseSummaries:         make(map[string]*LimitCloseSummary),
		positionStreamErrorCritical: make(chan error, 1),
		tickerStreamErrorCritical:   make(chan error, 1),
	}
}

func (ps *PositionService) GetAllPosition() []Position {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	positions := make([]Position, 0, len(ps.positionLong)+len(ps.positionShort))
	for _, pos := range ps.positionLong {
		positions = append(positions, *pos)
	}
	for _, pos := range ps.positionShort {
		positions = append(positions, *pos)
	}

	return positions
}

// GetLimitCloseSummary возвращает сводку по последнему завершённому полным закрытием
// позиции джобу для символа, если такая есть. nil, если джоб для символа никогда не
// завершался полным закрытием (позиция ещё открыта, джоб не запускали, или его
// остановили вручную/из-за ошибок, не закрыв позицию целиком).
func (ps *PositionService) GetLimitCloseSummary(symbol string) *LimitCloseSummary {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	s, ok := ps.limitCloseSummaries[symbol]
	if !ok {
		return nil
	}
	cp := *s
	return &cp
}

func (ps *PositionService) AddPosition(pos *Position) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if pos.Side == "Buy" {
		ps.positionLong[pos.Symbol] = pos
	} else if pos.Side == "Sell" {
		ps.positionShort[pos.Symbol] = pos
	} else {
		ps.log.Warn("unknown position side, skipping add", "symbol", pos.Symbol, "side", pos.Side)
		return
	}
	ps.log.Info("position added", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)
}

func (ps *PositionService) AddSubscribeTicker(ctx context.Context, symbol string) error {
	if _, ok := ps.subscribedTickers[symbol]; ok {
		return nil
	}
	unsubscribe, err := ps.SubscribeTickerStart(ctx, symbol)
	if err != nil {
		return err
	}
	ps.subscribedTickers[symbol] = unsubscribe
	return nil
}

func (ps *PositionService) DeleteSubscribeTicker(symbol string) {

	if _, ok := ps.subscribedTickers[symbol]; !ok {
		ps.log.Debug("ticker not subscribed, skipping unsubscribe", "symbol", symbol)
		return
	}
	ps.subscribedTickers[symbol]()
	delete(ps.subscribedTickers, symbol)
}

func (ps *PositionService) Init(ctx context.Context) error {

	position, err := ps.exchange.GetPositionInfo()
	if err != nil {
		return err
	}
	for _, pos := range position {
		newPos := &Position{
			Symbol:        pos.Symbol,
			CreatedTime:   pos.CreatedTime,
			PositionIdx:   pos.PositionIdx,
			Side:          pos.Side,
			Size:          pos.Size,
			EntryPrice:    pos.EntryPrice,
			UnrealisedPnl: pos.UnrealisedPnl,
		}
		ps.AddPosition(newPos)
	}

	for _, pos := range ps.positionLong {
		if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
			return fmt.Errorf("failed subscribe to ticker %s: %W", pos.Symbol, err)
		}
	}
	for _, pos := range ps.positionShort {
		if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
			return fmt.Errorf("failed subscribe to ticker %s: %W", pos.Symbol, err)
		}
	}

	ps.adoptOrphanedLimitCloseOrders(ctx)

	return nil
}

// adoptOrphanedLimitCloseOrders подхватывает резидентные reduce-only ордера,
// оставшиеся на бирже от джоба лимитного закрытия, прерванного предыдущим
// выключением приложения (StopAllSubscriptions останавливает репрайсинг, но не
// отменяет сам ордер). Без этого такой ордер был бы "невидимым" для UI — свежее
// in-memory состояние показало бы LimitCloseActive=false, хотя ордер на бирже всё ещё
// жив, не репрайсится и может исполниться в любой момент по устаревшей цене.
func (ps *PositionService) adoptOrphanedLimitCloseOrders(ctx context.Context) {
	ps.mu.RLock()
	symbols := make(map[string]struct{}, len(ps.positionLong)+len(ps.positionShort))
	for symbol := range ps.positionLong {
		symbols[symbol] = struct{}{}
	}
	for symbol := range ps.positionShort {
		symbols[symbol] = struct{}{}
	}
	ps.mu.RUnlock()

	for symbol := range symbols {
		orders, err := ps.exchange.GetOpenOrders(symbol)
		if err != nil {
			ps.log.Error("limit close: failed to check for orphaned orders", "symbol", symbol, "error", err)
			continue
		}

		for _, order := range orders {
			if !order.ReduceOnly {
				continue
			}
			ps.adoptLimitCloseOrder(ctx, symbol, order)
			break // джоб на символ поддерживается только один
		}
	}
}

// adoptLimitCloseOrder регистрирует найденный в adoptOrphanedLimitCloseOrders ордер
// как джоб лимитного закрытия и запускает для него репрайсинг — так, будто
// StartLimitClose был вызван прямо сейчас, только без повторной постановки ордера.
// Прогресс (LimitCloseProgress) считается от размера позиции на момент адопции, а не
// от размера до перезапуска — историю заполнения через предыдущий запуск мы не храним.
func (ps *PositionService) adoptLimitCloseOrder(ctx context.Context, symbol string, order OpenOrderInfo) {
	ps.mu.RLock()
	var pos *Position
	switch order.PositionIdx {
	case 1:
		pos = ps.positionLong[symbol]
	case 2:
		pos = ps.positionShort[symbol]
	default:
		pos = ps.positionLong[symbol]
		if pos == nil {
			pos = ps.positionShort[symbol]
		}
	}
	var startSize float64
	if pos != nil {
		startSize = pos.Size
	}
	ps.mu.RUnlock()

	if pos == nil || startSize <= 0 {
		return
	}

	info, err := ps.exchange.GetInstrumentInfo(symbol)
	if err != nil {
		ps.log.Error("limit close: failed to adopt orphaned order, instrument info", "symbol", symbol, "orderId", order.OrderID, "error", err)
		return
	}
	if info.TickSize <= 0 {
		ps.log.Error("limit close: failed to adopt orphaned order, invalid tick size", "symbol", symbol, "orderId", order.OrderID)
		return
	}

	jobCtx, cancel := context.WithCancel(ctx)
	job := &limitCloseJob{
		symbol:      symbol,
		orderSide:   order.Side,
		positionIdx: order.PositionIdx,
		startSize:   startSize,
		tickSize:    info.TickSize,
		qtyStep:     info.QtyStep,
		startTime:   time.Now(),
		orderID:     order.OrderID,
		orderPrice:  order.Price,
		reprice:     make(chan struct{}, 1),
		ctx:         jobCtx,
		cancel:      cancel,
		done:        make(chan struct{}),
	}

	if !ps.registerLimitCloseJob(job) {
		cancel()
		return
	}

	ps.log.Info("limit close: resumed orphaned order from previous run", "symbol", symbol, "orderSide", order.Side, "orderId", order.OrderID, "price", order.Price)

	ps.wg.Add(1)
	go ps.runLimitCloseJob(job)
}

func (ps *PositionService) Start(ctx context.Context) error {
	if ps.exchange == nil {
		return fmt.Errorf("exchange client is not initialized")
	}

	if err := ps.Init(ctx); err != nil {
		return fmt.Errorf("init positions: %w", err)
	}

	unsubscribe, err := ps.SubscribePositionStart(ctx)
	if err != nil {
		return err
	}
	ps.subscribedPosition = unsubscribe

	select {
	case <-ctx.Done():
		return nil
	case err := <-ps.positionStreamErrorCritical:
		return err
	case err := <-ps.tickerStreamErrorCritical:
		return err
	}
}

func (ps *PositionService) SubscribePositionStart(ctx context.Context) (func() error, error) {

	serviceName := "SubscribePosition"
	ps.hub.AddConnection(serviceName, hub.Connected, nil)
	var connectionStatus hub.ConnectionStatus = hub.Connected

	dataHandler := func(pos Position) {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		if connectionStatus == hub.Disconnected {
			ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
			connectionStatus = hub.Connected
		}

		// Определяем сторону позиции. В Hedge Mode её однозначно задаёт PositionIdx
		// (1 = long, 2 = short). В One-Way Mode (дефолт для большинства аккаунтов)
		// PositionIdx всегда 0 — тогда берём сторону из Side. А при полном закрытии
		// позиции в One-Way Mode Bybit иногда отдаёт пустой Side, поэтому в этом
		// случае определяем сторону по тому, где уже есть запись с этим символом
		// (иначе апдейт о закрытии молча терялся бы и позиция зависала в списке).
		var target map[string]*Position

		switch pos.PositionIdx {
		case 1:
			target = ps.positionLong
		case 2:
			target = ps.positionShort
		default:
			switch pos.Side {
			case "Buy":
				target = ps.positionLong
			case "Sell":
				target = ps.positionShort
			default:
				if _, ok := ps.positionLong[pos.Symbol]; ok {
					target = ps.positionLong
				} else if _, ok := ps.positionShort[pos.Symbol]; ok {
					target = ps.positionShort
				} else {
					return
				}
			}
		}

		existing, exists := target[pos.Symbol]

		// Игнор старых апдейтов
		if exists && pos.UpdatedTime < existing.UpdatedTime {
			return
		}

		// Закрытие позиции
		if pos.Size == 0 {
			if exists {
				// Последний филл, закрывший позицию целиком — existing.Size тут ещё старый
				// (домашний) остаток, target ещё не тронут.
				if job, ok := ps.limitCloseJobs[pos.Symbol]; ok && existing.Size > 0 {
					job.fills = append(job.fills, fillRecord{price: job.orderPrice, qty: existing.Size})
				}
				delete(target, pos.Symbol)
				ps.log.Info("position closed", "symbol", pos.Symbol, "side", pos.Side)
				// если обе стороны пусты → можно отписаться от тикера
				if ps.positionLong[pos.Symbol] == nil && ps.positionShort[pos.Symbol] == nil {
					ps.DeleteSubscribeTicker(pos.Symbol)
					delete(ps.priceHistory, pos.Symbol)
					delete(ps.bestBid, pos.Symbol)
					delete(ps.bestAsk, pos.Symbol)
				}
				// Если позицию закрыл наш же ордер лимитного закрытия — джобу нужно
				// об этом узнать и подчистить за собой состояние.
				ps.signalLimitCloseJob(pos.Symbol)
			}
			return
		}

		// Добавление новой позиции
		if !exists {

			if err := ps.AddSubscribeTicker(ctx, pos.Symbol); err != nil {
				ps.tickerStreamErrorCritical <- err
			}

			target[pos.Symbol] = &Position{
				Symbol:        pos.Symbol,
				CreatedTime:   pos.CreatedTime,
				UpdatedTime:   pos.UpdatedTime,
				PositionIdx:   pos.PositionIdx,
				Side:          pos.Side,
				Size:          pos.Size,
				EntryPrice:    pos.EntryPrice,
				UnrealisedPnl: pos.UnrealisedPnl,
			}

			ps.log.Info("new position detected", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)
			return
		}

		// Обновление позиции
		prevSize := existing.Size
		existing.UpdatedTime = pos.UpdatedTime
		existing.Side = pos.Side
		existing.Size = pos.Size
		existing.EntryPrice = pos.EntryPrice

		// Если по символу идёт лимитное закрытие — пересчитываем прогресс от стартового
		// объёма джоба (а не текущего Size, он и есть источник изменений) и, если Size
		// уменьшился, записываем частичный филл по текущей цене резидентного ордера.
		if job, ok := ps.limitCloseJobs[pos.Symbol]; ok && job.startSize > 0 {
			if filled := prevSize - existing.Size; filled > 0 {
				job.fills = append(job.fills, fillRecord{price: job.orderPrice, qty: filled})
			}

			progress := (job.startSize - existing.Size) / job.startSize * 100
			if progress < 0 {
				progress = 0
			} else if progress > 100 {
				progress = 100
			}
			existing.LimitCloseProgress = progress
		}

		ps.log.Info("position updated", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)

		ps.signalLimitCloseJob(pos.Symbol)
	}

	errHandler := func(err error, critical bool) {
		if !critical {
			ps.log.Error("position stream error", "error", err)
		} else {
			ps.positionStreamErrorCritical <- err
		}
		ps.hub.UpdateConnection(serviceName, hub.Disconnected, err)
		connectionStatus = hub.Disconnected
	}

	unsubscribe, err := ps.exchange.SubscribePositionStart(ctx, dataHandler, errHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe position stream: %w", err)
	}

	ps.wg.Add(1)
	wrappedUnsubscribe := func() error {
		defer ps.wg.Done()

		ps.hub.RemoveConnection(serviceName)
		if err := unsubscribe(); err != nil {
			ps.log.Error("failed to unsubscribe position")
			return err
		}

		ps.log.Info("unsubscribed position successfully")
		return nil
	}

	return wrappedUnsubscribe, nil

}

// recordPriceChange добавляет новую точку цены в скользящее окно символа,
// вычищает устаревшие точки и возвращает скорость изменения цены в % за минуту
// относительно самой старой точки, ещё оставшейся в окне.
// Вызывающий код должен уже удерживать ps.mu.
func (ps *PositionService) recordPriceChange(symbol string, price float64) float64 {
	now := time.Now()

	history := append(ps.priceHistory[symbol], pricePoint{ts: now, price: price})

	cutoff := now.Add(-priceChangeWindow)
	i := 0
	for i < len(history) && history[i].ts.Before(cutoff) {
		i++
	}
	history = history[i:]
	ps.priceHistory[symbol] = history

	if len(history) < 2 {
		return 0
	}

	oldest := history[0]
	elapsed := now.Sub(oldest.ts)
	if elapsed < minPriceSampleAge || oldest.price == 0 {
		return 0
	}

	return (price - oldest.price) / oldest.price * 100 * (60 / elapsed.Seconds())
}

func (ps *PositionService) SubscribeTickerStart(ctx context.Context, symbol string) (func() error, error) {

	serviceName := fmt.Sprintf("SubscribeTicker_%s", symbol)
	ps.hub.AddConnection(serviceName, hub.Connected, nil)
	var connectionStatus hub.ConnectionStatus = hub.Connected

	dataHandler := func(update TickerUpdate) {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		if connectionStatus == hub.Disconnected {
			ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
			connectionStatus = hub.Connected
		}

		if update.Bid1 != nil {
			ps.bestBid[symbol] = *update.Bid1
		}
		if update.Ask1 != nil {
			ps.bestAsk[symbol] = *update.Ask1
		}

		if update.Price != nil {
			price := *update.Price
			changePercent := ps.recordPriceChange(symbol, price)

			// =========================
			// LONG POSITION
			// =========================
			if longPos, exists := ps.positionLong[symbol]; exists && longPos != nil {

				longPos.CurrentPrice = price
				longPos.CurrentValue = price * longPos.Size
				longPos.PriceChangePercent = changePercent

				longPos.UnrealisedPnl = (price - longPos.EntryPrice) * longPos.Size
			}

			// =========================
			// SHORT POSITION
			// =========================
			if shortPos, exists := ps.positionShort[symbol]; exists && shortPos != nil {

				shortPos.CurrentPrice = price
				shortPos.CurrentValue = price * shortPos.Size
				shortPos.PriceChangePercent = changePercent

				shortPos.UnrealisedPnl = (shortPos.EntryPrice - price) * shortPos.Size
			}
		}

		// bid1/ask1 — то, на что реагирует репрайсинг лимитного закрытия позиции.
		if update.Bid1 != nil || update.Ask1 != nil {
			ps.signalLimitCloseJob(symbol)
		}
	}

	errHandler := func(err error, critical bool) {
		if !critical {
			ps.log.Error("ticker stream error", "symbol", symbol, "error", err)
		} else {
			ps.tickerStreamErrorCritical <- err
		}
		ps.hub.UpdateConnection(serviceName, hub.Disconnected, err)
		connectionStatus = hub.Disconnected
	}

	unsubscribe, err := ps.exchange.SubscribeTickerStart(ctx, symbol, dataHandler, errHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe ticker %s: %w", symbol, err)
	}

	ps.wg.Add(1)
	wrappedUnsubscribe := func() error {
		defer ps.wg.Done()

		ps.hub.RemoveConnection(serviceName)
		if err := unsubscribe(); err != nil {
			ps.log.Error("failed to unsubscribe from ticker", "symbol", symbol, "error", err)
			return err
		}

		ps.log.Info("unsubscribed ticker successfully", "symbol", symbol)
		return nil
	}

	return wrappedUnsubscribe, nil
}

// roundToTick округляет цену до ближайшего шага цены инструмента (tickSize) — иначе
// биржа отклонит ордер как невалидный.
func roundToTick(price, tickSize float64) float64 {
	if tickSize <= 0 {
		return price
	}
	return math.Round(price/tickSize) * tickSize
}

// roundToStep округляет объём до ближайшего шага лота инструмента (qtyStep).
func roundToStep(qty, step float64) float64 {
	if step <= 0 {
		return qty
	}
	return math.Round(qty/step) * step
}

// pegPrice — цена лимитника на один тик агрессивнее собственной лучшей цены (становимся
// новым лучшим предложением на своей стороне стакана), но только если для этого есть
// место: при спреде в 1 тик (частый случай у ликвидных пар вроде BTCUSDT) улучшение на
// тик легло бы ровно на противоположную лучшую цену — это уже не "лучшее предложение",
// а пересечение спреда (сделка тейкера), которое PostOnly обязан отклонить. В этом
// случае просто остаёмся на своей текущей лучшей цене (bestAsk/bestBid) — это всегда
// валидный, непересекающий спред мейкер-ордер.
// Даже с этой поправкой репрайсинг всё ещё может изредка ловить пересечение спреда из-за
// задержки сети (рынок успел уйти, пока запрос летел до биржи) — PostOnly и это отклонит,
// а мы просто попробуем ещё раз на следующем обновлении цены.
func pegPrice(orderSide string, bestBid, bestAsk, tickSize float64) float64 {
	if orderSide == "Sell" {
		improved := roundToTick(bestAsk-tickSize, tickSize)
		if improved > bestBid {
			return improved
		}
		return roundToTick(bestAsk, tickSize)
	}

	improved := roundToTick(bestBid+tickSize, tickSize)
	if improved < bestAsk {
		return improved
	}
	return roundToTick(bestBid, tickSize)
}

// StartLimitClose запускает автоматическое закрытие позиции по символу серией
// reduce-only PostOnly лимитных ордеров, которые репрайсятся вслед за лучшей ценой
// (см. pegPrice), пока позиция не закроется целиком или пока не вызовут StopLimitClose.
//
// Известное упрощение v1: если по символу одновременно открыты и long, и short
// (Hedge Mode), закрывается только первая найденная сторона (long в приоритете) —
// закрытие обеих сторон одним вызовом не поддержано.
func (ps *PositionService) StartLimitClose(ctx context.Context, symbol string) error {
	err := ps.startLimitClose(ctx, symbol)
	if err != nil {
		// Раньше ранние отказы (позиция не найдена, нет данных стакана, ошибка биржи на
		// создании ордера) уходили только во фронтенд и не попадали в лог — из-за этого
		// нечем было диагностировать "Qty invalid" на XRPUSDT.
		ps.log.Error("limit close: start failed", "symbol", symbol, "error", err)
	}
	return err
}

func (ps *PositionService) startLimitClose(ctx context.Context, symbol string) error {
	ps.mu.Lock()
	if _, exists := ps.limitCloseJobs[symbol]; exists {
		ps.mu.Unlock()
		return fmt.Errorf("limit close already running for %s", symbol)
	}

	pos := ps.positionLong[symbol]
	if pos == nil {
		pos = ps.positionShort[symbol]
	}
	if pos == nil || pos.Size <= 0 {
		ps.mu.Unlock()
		return fmt.Errorf("position not found: %s", symbol)
	}

	orderSide := "Sell"
	if pos.Side == "Sell" {
		orderSide = "Buy"
	}

	startSize := pos.Size
	positionIdx := pos.PositionIdx
	bestBid, hasBid := ps.bestBid[symbol]
	bestAsk, hasAsk := ps.bestAsk[symbol]
	ps.mu.Unlock()

	if !hasBid || !hasAsk {
		return fmt.Errorf("нет данных стакана по %s, попробуйте ещё раз через момент", symbol)
	}
	if bestBid >= bestAsk {
		return fmt.Errorf("некорректные данные стакана по %s (bid %.8f >= ask %.8f), попробуйте ещё раз через момент", symbol, bestBid, bestAsk)
	}

	info, err := ps.exchange.GetInstrumentInfo(symbol)
	if err != nil {
		return fmt.Errorf("get instrument info: %w", err)
	}
	if info.TickSize <= 0 {
		return fmt.Errorf("invalid tick size for %s", symbol)
	}

	price := pegPrice(orderSide, bestBid, bestAsk, info.TickSize)
	qty := roundToStep(startSize, info.QtyStep)

	orderID, err := ps.exchange.PlaceReduceOnlyLimitOrder(symbol, orderSide, qty, price, positionIdx)
	if err != nil {
		return fmt.Errorf("place order: %w", err)
	}

	jobCtx, cancel := context.WithCancel(ctx)
	job := &limitCloseJob{
		symbol:      symbol,
		orderSide:   orderSide,
		positionIdx: positionIdx,
		startSize:   startSize,
		tickSize:    info.TickSize,
		qtyStep:     info.QtyStep,
		startTime:   time.Now(),
		orderID:     orderID,
		orderPrice:  price,
		reprice:     make(chan struct{}, 1),
		ctx:         jobCtx,
		cancel:      cancel,
		done:        make(chan struct{}),
	}

	// Позицию могли закрыть, пока мы ждали ответ биржи на GetInstrumentInfo/CreateOrder —
	// registerLimitCloseJob перепроверяет это перед тем, как зарегистрировать джоб.
	if !ps.registerLimitCloseJob(job) {
		cancel()
		_ = ps.exchange.CancelOrder(symbol, orderID)
		return fmt.Errorf("position was closed while starting limit close")
	}

	ps.log.Info("limit close started", "symbol", symbol, "orderSide", orderSide, "qty", qty, "price", price, "orderId", orderID)

	ps.wg.Add(1)
	go ps.runLimitCloseJob(job)

	return nil
}

// registerLimitCloseJob регистрирует новый джоб лимитного закрытия (из StartLimitClose
// или adoptLimitCloseOrder) в ps.limitCloseJobs и выставляет LimitCloseActive/Progress/
// Error на соответствующей Position. Возвращает false, если регистрация не удалась
// (джоб для символа уже есть, либо позиция успела закрыться, пока вызывающий код ждал
// ответ биржи) — в этом случае он не трогает состояние, и вызывающий код должен сам
// освободить ресурсы джоба (как минимум job.cancel()).
func (ps *PositionService) registerLimitCloseJob(job *limitCloseJob) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, exists := ps.limitCloseJobs[job.symbol]; exists {
		return false
	}
	if ps.positionLong[job.symbol] == nil && ps.positionShort[job.symbol] == nil {
		return false
	}

	ps.limitCloseJobs[job.symbol] = job
	// Новый запуск — прошлая сводка (если была) больше не актуальна.
	delete(ps.limitCloseSummaries, job.symbol)
	if p, ok := ps.positionLong[job.symbol]; ok {
		p.LimitCloseActive = true
		p.LimitCloseProgress = 0
		p.LimitCloseError = ""
	}
	if p, ok := ps.positionShort[job.symbol]; ok {
		p.LimitCloseActive = true
		p.LimitCloseProgress = 0
		p.LimitCloseError = ""
	}
	return true
}

// StopLimitClose останавливает джоб и отменяет ещё не исполненный ордер.
func (ps *PositionService) StopLimitClose(symbol string) error {
	ps.mu.RLock()
	job, exists := ps.limitCloseJobs[symbol]
	ps.mu.RUnlock()

	if !exists {
		return fmt.Errorf("limit close is not running for %s", symbol)
	}

	job.cancel()
	<-job.done
	ps.finishLimitCloseJob(job, "stopped by user")
	return nil
}

// signalLimitCloseJob — небуферизованный (не более одного отложенного) сигнал джобу
// перепроверить состояние (новую цену или то, что позиция уже закрылась). Вызывающий
// код должен уже удерживать ps.mu.
func (ps *PositionService) signalLimitCloseJob(symbol string) {
	job, ok := ps.limitCloseJobs[symbol]
	if !ok {
		return
	}
	select {
	case job.reprice <- struct{}{}:
	default:
	}
}

// runLimitCloseJob — воркер джоба: ждёт сигналов о новой цене/закрытии позиции и
// реагирует на них. Останавливается, когда job.ctx отменяют (StopLimitClose,
// самозавершение при закрытии позиции, либо мягкая остановка при выключении
// приложения — см. StopAllSubscriptions).
func (ps *PositionService) runLimitCloseJob(job *limitCloseJob) {
	defer ps.wg.Done()
	defer close(job.done)

	for {
		select {
		case <-job.ctx.Done():
			return
		case <-job.reprice:
			ps.processLimitCloseTick(job)
		}
	}
}

// minRepriceInterval — не чаще какого интервала пытаемся обратиться к бирже за один
// джоб. Ликвидные пары (BTCUSDT и т.п.) шлют тики по несколько раз в секунду — без
// этого лимита джоб стучался бы в API на каждый тик и упирался в рейт-лимит биржи.
const minRepriceInterval = 500 * time.Millisecond

// maxConsecutiveFailures — после скольких подряд неудачных попыток (amend или
// replace) джоб сдаётся и останавливается сам, вместо того чтобы долбить биржу
// бесконечно.
const maxConsecutiveFailures = 5

// isOrderMissingError — true, если Bybit ответил кодом 110001 ("order not exists or
// too late to replace"): ордера с таким ID на бирже больше нет — исполнился, отменён
// или так и не был подтверждён. Это единственный случай, когда безопасно выставить
// новый ордер взамен — раз старого точно нет, дублирования живых ордеров не будет.
func isOrderMissingError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "110001")
}

// processLimitCloseTick проверяет, не закрылась ли позиция сама (скорее всего — наш же
// ордер исполнился целиком), и если нет — при необходимости репрайсит ордер на новую
// лучшую цену.
func (ps *PositionService) processLimitCloseTick(job *limitCloseJob) {
	ps.mu.RLock()
	bestBid, hasBid := ps.bestBid[job.symbol]
	bestAsk, hasAsk := ps.bestAsk[job.symbol]
	pos := ps.positionLong[job.symbol]
	if pos == nil {
		pos = ps.positionShort[job.symbol]
	}
	posExists := pos != nil
	var posSize float64
	if posExists {
		// Копируем под локом — дальше posSize используется уже без него (в т.ч. в
		// сетевом вызове на replace), а *Position конкурентно пишет позиционный
		// дата-хендлер.
		posSize = pos.Size
	}
	currentPrice := job.orderPrice
	orderID := job.orderID
	lastAttempt := job.lastAttempt
	ps.mu.RUnlock()

	if !posExists || posSize <= 0 {
		job.cancel()
		ps.finishLimitCloseJob(job, "position closed")
		return
	}

	if !hasBid || !hasAsk {
		return
	}
	if bestBid >= bestAsk {
		// Перевёрнутый/нулевой спред — обычно кратковременный глюк данных стакана.
		// pegPrice тут посчитал бы цену, пересекающую спред, PostOnly её бы отклонил,
		// и это ушло бы в registerLimitCloseFailure как обычная неудача, хотя причина
		// отличается от "рынок/биржа не приняли реприйс" — логируем явно и ждём
		// следующего тика с нормальными данными, не тратя попытку из maxConsecutiveFailures.
		ps.log.Debug("limit close: crossed or zero spread, skipping reprice this tick", "symbol", job.symbol, "bestBid", bestBid, "bestAsk", bestAsk)
		return
	}

	desired := pegPrice(job.orderSide, bestBid, bestAsk, job.tickSize)
	if desired == currentPrice {
		return
	}

	if time.Since(lastAttempt) < minRepriceInterval {
		return
	}

	ps.mu.Lock()
	job.lastAttempt = time.Now()
	ps.mu.Unlock()

	err := ps.exchange.AmendOrderPrice(job.symbol, orderID, desired)
	if err == nil {
		ps.mu.Lock()
		job.orderPrice = desired
		job.amendFailures = 0
		ps.mu.Unlock()
		return
	}

	if isOrderMissingError(err) {
		// Позиция всё ещё открыта (иначе вышли бы выше) — значит ордер пропал не из-за
		// того, что он полностью исполнился. Раз его точно нет на бирже, безопасно
		// выставить новый на оставшийся объём вместо него. Перед этим смотрим в историю
		// ордеров — чисто для диагностики: помогает понять, что реально произошло
		// (исполнился? отменён биржей и почему?), а не гадать по косвенным признакам.
		if statusInfo, statusErr := ps.exchange.GetOrderStatus(job.symbol, orderID); statusErr != nil {
			ps.log.Warn("limit close: order missing on exchange, status lookup also failed", "symbol", job.symbol, "oldOrderId", orderID, "statusErr", statusErr)
		} else {
			ps.log.Warn("limit close: order missing on exchange, placing a replacement",
				"symbol", job.symbol, "oldOrderId", orderID,
				"orderStatus", statusInfo.Status, "cancelType", statusInfo.CancelType, "rejectReason", statusInfo.RejectReason)
		}

		qty := roundToStep(posSize, job.qtyStep)
		newOrderID, placeErr := ps.exchange.PlaceReduceOnlyLimitOrder(job.symbol, job.orderSide, qty, desired, job.positionIdx)
		if placeErr != nil {
			ps.registerLimitCloseFailure(job, fmt.Errorf("replace lost order: %w", placeErr))
			return
		}

		ps.mu.Lock()
		job.orderID = newOrderID
		job.orderPrice = desired
		job.amendFailures = 0
		ps.mu.Unlock()

		ps.log.Info("limit close: replacement order placed", "symbol", job.symbol, "orderId", newOrderID, "qty", qty, "price", desired)
		return
	}

	// Частый ожидаемый случай — новая цена всё же пересекла бы спред и PostOnly её
	// отклонил. Пробуем ещё раз на следующем тике (не чаще minRepriceInterval); если
	// это не разовая история, а он не может закрепиться подряд maxConsecutiveFailures
	// раз — джоб сам остановится, см. registerLimitCloseFailure.
	ps.log.Debug("limit close: amend order failed, will retry on next tick", "symbol", job.symbol, "error", err)
	ps.registerLimitCloseFailure(job, err)
}

// registerLimitCloseFailure считает идущие подряд неудачи джоба. После
// maxConsecutiveFailures — вместо бесконечных попыток останавливает джоб и оставляет
// причину в Position.LimitCloseError, чтобы это было видно на фронте.
func (ps *PositionService) registerLimitCloseFailure(job *limitCloseJob, cause error) {
	ps.mu.Lock()
	job.amendFailures++
	failures := job.amendFailures
	ps.mu.Unlock()

	if failures < maxConsecutiveFailures {
		return
	}

	ps.log.Error("limit close: giving up after repeated failures", "symbol", job.symbol, "failures", failures, "error", cause)

	ps.mu.Lock()
	if p, ok := ps.positionLong[job.symbol]; ok {
		p.LimitCloseError = fmt.Sprintf("остановлено: %v", cause)
	}
	if p, ok := ps.positionShort[job.symbol]; ok {
		p.LimitCloseError = fmt.Sprintf("остановлено: %v", cause)
	}
	ps.mu.Unlock()

	job.cancel()
	ps.finishLimitCloseJob(job, "repeated failures")
}

// reasonPositionClosed — единственная причина остановки джоба, при которой строим
// LimitCloseSummary (см. finishLimitCloseJob): позиция реально закрылась целиком,
// поэтому detailPosition на фронте исчезнет и сводку нужно показывать взамен неё.
// При "stopped by user"/"repeated failures" позиция обычно ещё частично открыта —
// живая строка на фронте остаётся, сводка не нужна.
const reasonPositionClosed = "position closed"

// finishLimitCloseJob подчищает состояние джоба и снимает флаг LimitCloseActive с
// позиции. Пытается отменить ещё не исполненный ордер — если он уже исполнился
// (в т.ч. это исполнение и закрыло позицию), CancelOrder вернёт ошибку, которую мы
// просто логируем на Debug: это ожидаемый исход, а не сбой.
//
// Идемпотентна: джоб может завершиться сам (позиция закрылась/повторные неудачи) ровно
// в момент, когда пользователь жмёт "Стоп" — StopLimitClose дожидается завершения
// воркера и затем всё равно вызывает finishLimitCloseJob ещё раз. Без проверки
// job.finished это отправило бы лишний CancelOrder на уже неактуальный ордер и
// задвоило бы лог "limit close finished".
func (ps *PositionService) finishLimitCloseJob(job *limitCloseJob, reason string) {
	ps.mu.Lock()
	if job.finished {
		ps.mu.Unlock()
		return
	}
	job.finished = true
	orderID := job.orderID
	ps.mu.Unlock()

	if err := ps.exchange.CancelOrder(job.symbol, orderID); err != nil {
		ps.log.Debug("limit close: cancel on finish (order may already be filled)", "symbol", job.symbol, "reason", reason, "error", err)
	}

	ps.mu.Lock()
	delete(ps.limitCloseJobs, job.symbol)
	if reason == reasonPositionClosed {
		ps.limitCloseSummaries[job.symbol] = buildLimitCloseSummary(job)
	}
	if p, ok := ps.positionLong[job.symbol]; ok {
		p.LimitCloseActive = false
	}
	if p, ok := ps.positionShort[job.symbol]; ok {
		p.LimitCloseActive = false
	}
	ps.mu.Unlock()

	ps.log.Info("limit close finished", "symbol", job.symbol, "reason", reason, "fills", len(job.fills))
}

// buildLimitCloseSummary сворачивает накопленные за время джоба филлы в сводку для
// фронта. Вызывающий код должен уже удерживать ps.mu (fills пишутся из дата-хендлера
// позиции под тем же локом).
func buildLimitCloseSummary(job *limitCloseJob) *LimitCloseSummary {
	s := &LimitCloseSummary{
		Symbol:          job.symbol,
		DurationSeconds: time.Since(job.startTime).Seconds(),
		StartSize:       job.startSize,
	}

	var weightedSum float64
	for i, f := range job.fills {
		s.FilledQty += f.qty
		weightedSum += f.price * f.qty
		if i == 0 || f.price < s.MinPrice {
			s.MinPrice = f.price
		}
		if i == 0 || f.price > s.MaxPrice {
			s.MaxPrice = f.price
		}
	}
	s.FillCount = len(job.fills)
	if s.FilledQty > 0 {
		s.AvgPrice = weightedSum / s.FilledQty
	}
	if job.startSize > 0 {
		s.Progress = s.FilledQty / job.startSize * 100
		if s.Progress > 100 {
			s.Progress = 100
		}
	}

	return s
}

func (ps *PositionService) StopAllSubscriptions() error {

	// "Мягкая" остановка джобов лимитного закрытия при выключении приложения: сам
	// воркер (репрайсинг) останавливаем, но выставленный на бирже reduce-only ордер
	// НЕ отменяем — это реальный резидентный ордер биржи, и он продолжит действовать,
	// даже пока приложение не запущено. Автоматического возобновления репрайсинга при
	// следующем старте приложения пока нет — сознательное упрощение v1.
	ps.mu.RLock()
	jobs := make([]*limitCloseJob, 0, len(ps.limitCloseJobs))
	for _, job := range ps.limitCloseJobs {
		jobs = append(jobs, job)
	}
	ps.mu.RUnlock()

	for _, job := range jobs {
		job.cancel()
	}

	for symbol, unsub := range ps.subscribedTickers {
		if err := unsub(); err != nil {
			ps.log.Error("failed to unsubscribe ticker", "symbol", symbol, "error", err)
		}
	}

	if ps.subscribedPosition != nil {
		if err := ps.subscribedPosition(); err != nil {
			ps.log.Error("failed to unsubscribe position")
		}
	}

	ps.wg.Wait()
	ps.log.Info("all subscriptions stopped")
	return nil
}
