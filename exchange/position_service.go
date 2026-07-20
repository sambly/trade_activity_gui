package exchange

import (
	"context"
	"fmt"
	"log/slog"
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

	return nil
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

		// определяем сторону по positionIdx (buy, sell )
		var target map[string]*Position

		switch pos.PositionIdx {
		case 1:
			target = ps.positionLong
		case 2:
			target = ps.positionShort
		default:
			return
		}

		existing, exists := target[pos.Symbol]

		// Игнор старых апдейтов
		if exists && pos.UpdatedTime < existing.UpdatedTime {
			return
		}

		// Закрытие позиции
		if pos.Size == 0 {
			if exists {
				delete(target, pos.Symbol)
				ps.log.Info("position closed", "symbol", pos.Symbol, "side", pos.Side)
				// если обе стороны пусты → можно отписаться от тикера
				if ps.positionLong[pos.Symbol] == nil && ps.positionShort[pos.Symbol] == nil {
					ps.DeleteSubscribeTicker(pos.Symbol)
					delete(ps.priceHistory, pos.Symbol)
				}
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
		existing.UpdatedTime = pos.UpdatedTime
		existing.Side = pos.Side
		existing.Size = pos.Size
		existing.EntryPrice = pos.EntryPrice

		ps.log.Info("position updated", "symbol", pos.Symbol, "side", pos.Side, "size", pos.Size)

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

	dataHandler := func(price float64) {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		if connectionStatus == hub.Disconnected {
			ps.hub.UpdateConnection(serviceName, hub.Connected, nil)
			connectionStatus = hub.Connected
		}

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

func (ps *PositionService) StopAllSubscriptions() error {

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
