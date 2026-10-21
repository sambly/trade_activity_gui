package exchange

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"trade_activity_gui/logger"

	"github.com/hirokisan/bybit/v2"
	"github.com/jpillora/backoff"
)

const maxReconnectAttempts = 10

// Demo Trading — режим Bybit с реальными рыночными данными, но виртуальным балансом.
// У него отдельные REST/WS-хосты и отдельные API-ключи (создаются в разделе "Demo Trading"
// личного кабинета Bybit, обычные ключи для боевой торговли тут не подойдут).
// Публичный стрим (тикеры) у demo-домена не работает — он всегда идёт через основной wss://stream.bybit.com,
// см. https://bybit-exchange.github.io/docs/v5/demo
const (
	demoRestBaseURL = "https://api-demo.bybit.com"
	demoWSBaseURL   = "wss://stream-demo.bybit.com"
)

func createLogAdapter(slogLogger *slog.Logger) *log.Logger {
	slogLogger = slogLogger.With("component", "bybit_lib")

	reader, writer := io.Pipe()

	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			line = logger.MaskSensitive(line)
			slogLogger.Info(line)
		}
	}()

	return log.New(writer, "", 0)
}

type Bybit struct {
	log *slog.Logger

	client *bybit.Client

	// Публичный стрим (тикеры) у Demo Trading не работает и всегда идёт через основной хост,
	// поэтому публичный и приватный WS-клиенты держим раздельно — в demo-режиме на разные хосты.
	wsPublicClient  *bybit.WebSocketClient
	wsPrivateClient *bybit.WebSocketClient
}

func NewBybit(key, secret string, logger *slog.Logger, debug bool, demo bool) (*Bybit, error) {

	client := bybit.NewClient().WithAuth(key, secret).WithLogger(createLogAdapter(logger)).WithDebug(debug)
	wsPublicClient := bybit.NewWebsocketClient().WithAuth(key, secret).WithLogger(createLogAdapter(logger)).WithDebug(debug)
	wsPrivateClient := bybit.NewWebsocketClient().WithAuth(key, secret).WithLogger(createLogAdapter(logger)).WithDebug(debug)

	// /v3/public/time не отдаёт данные на api-demo.bybit.com (пустое тело при статусе 200),
	// поэтому синхронизируем время по основному хосту — смещение системных часов от того,
	// на какой хост потом переключимся для торговли, не зависит. WithBaseURL меняет baseURL
	// у того же клиента, не затрагивая уже посчитанное смещение времени.
	timeOffset, err := client.SyncServerTime()
	if err != nil {
		return nil, fmt.Errorf("sync server time: %w", err)
	}
	wsPublicClient.UpdateTimeDelta(timeOffset)
	wsPrivateClient.UpdateTimeDelta(timeOffset)

	if demo {
		client = client.WithBaseURL(demoRestBaseURL)
		wsPrivateClient = wsPrivateClient.WithBaseURL(demoWSBaseURL)
		logger.Info("bybit demo trading mode enabled", "restBaseURL", demoRestBaseURL, "wsPrivateBaseURL", demoWSBaseURL)
	}

	return &Bybit{
		log:             logger.With("component", "bybit"),
		client:          client,
		wsPublicClient:  wsPublicClient,
		wsPrivateClient: wsPrivateClient,
	}, nil
}

func (b *Bybit) GetInstrumentInfo(symbol string) (InstrumentInfo, error) {
	sym := bybit.SymbolV5(symbol)

	resp, err := b.client.V5().Market().GetInstrumentsInfo(bybit.V5GetInstrumentsInfoParam{
		Category: bybit.CategoryV5Linear,
		Symbol:   &sym,
	})
	if err != nil {
		return InstrumentInfo{}, fmt.Errorf("get instruments info: %w", err)
	}
	if resp.Result.LinearInverse == nil || len(resp.Result.LinearInverse.List) == 0 {
		return InstrumentInfo{}, fmt.Errorf("instrument info not found: %s", symbol)
	}

	item := resp.Result.LinearInverse.List[0]

	tickSize, err := strconv.ParseFloat(item.PriceFilter.TickSize, 64)
	if err != nil {
		return InstrumentInfo{}, fmt.Errorf("parse tick size: %w", err)
	}
	qtyStep, err := strconv.ParseFloat(item.LotSizeFilter.QtyStep, 64)
	if err != nil {
		return InstrumentInfo{}, fmt.Errorf("parse qty step: %w", err)
	}

	return InstrumentInfo{TickSize: tickSize, QtyStep: qtyStep}, nil
}

// formatOrderNumber форматирует цену/объём ордера для отправки на биржу. roundToTick/
// roundToStep в position_service.go считают через math.Round(x/step)*step, что из-за
// погрешности float64 может дать, например, 48.400000000000006 вместо 48.4 —
// strconv.FormatFloat(..., -1, ...) добросовестно печатает это как есть, и биржа такую
// строку отклоняет ("Qty invalid"). Округляем до 8 знаков (с запасом для любого
// крипто-инструмента на Bybit) и обрезаем незначащие нули — округление до фиксированной
// точности убирает шум, обрезка оставляет ровно то число знаков, что нёс сам шаг.
func formatOrderNumber(value float64) string {
	s := strconv.FormatFloat(value, 'f', 8, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" || s == "-0" {
		return "0"
	}
	return s
}

// PlaceReduceOnlyLimitOrder ставит reduce-only PostOnly лимитный ордер — reduceOnly не
// даёт ему открыть позицию в противоположную сторону, если размер разойдётся с реальным
// (например, позицию уже успели частично закрыть руками), PostOnly не даёт ему
// исполниться тейкером, если репрайсинг вдруг пересечёт спред.
func (b *Bybit) PlaceReduceOnlyLimitOrder(symbol, side string, qty, price float64, positionIdx int) (string, error) {
	qtyStr := formatOrderNumber(qty)
	priceStr := formatOrderNumber(price)
	reduceOnly := true
	tif := bybit.TimeInForcePostOnly
	pIdx := bybit.PositionIdx(positionIdx)

	resp, err := b.client.V5().Order().CreateOrder(bybit.V5CreateOrderParam{
		Category:    bybit.CategoryV5Linear,
		Symbol:      bybit.SymbolV5(symbol),
		Side:        bybit.Side(side),
		OrderType:   bybit.OrderTypeLimit,
		Qty:         qtyStr,
		Price:       &priceStr,
		TimeInForce: &tif,
		ReduceOnly:  &reduceOnly,
		PositionIdx: &pIdx,
	})
	if err != nil {
		return "", fmt.Errorf("create order: %w", err)
	}

	return resp.Result.OrderID, nil
}

// AmendOrderPrice переставляет цену уже стоящего ордера, не отменяя его — дешевле и без
// гонки (окно между cancel и create, в которое можно пропустить исполнение), чем
// cancel + create.
func (b *Bybit) AmendOrderPrice(symbol, orderID string, price float64) error {
	priceStr := formatOrderNumber(price)

	_, err := b.client.V5().Order().AmendOrder(bybit.V5AmendOrderParam{
		Category: bybit.CategoryV5Linear,
		Symbol:   bybit.SymbolV5(symbol),
		OrderID:  &orderID,
		Price:    &priceStr,
	})
	if err != nil {
		return fmt.Errorf("amend order: %w", err)
	}

	return nil
}

func (b *Bybit) CancelOrder(symbol, orderID string) error {
	_, err := b.client.V5().Order().CancelOrder(bybit.V5CancelOrderParam{
		Category: bybit.CategoryV5Linear,
		Symbol:   bybit.SymbolV5(symbol),
		OrderID:  &orderID,
	})
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}

	return nil
}

// GetOrderStatus — см. OrderStatusInfo. Открытые ордера (GetOpenOrders) уже не найдут
// ордер, который биржа сочла завершённым, поэтому смотрим в историю (GetHistoryOrders).
func (b *Bybit) GetOrderStatus(symbol, orderID string) (OrderStatusInfo, error) {
	resp, err := b.client.V5().Order().GetHistoryOrders(bybit.V5GetHistoryOrdersParam{
		Category: bybit.CategoryV5Linear,
		Symbol:   (*bybit.SymbolV5)(&symbol),
		OrderID:  &orderID,
	})
	if err != nil {
		return OrderStatusInfo{}, fmt.Errorf("get history orders: %w", err)
	}
	if len(resp.Result.List) == 0 {
		return OrderStatusInfo{}, fmt.Errorf("order not found in history: %s", orderID)
	}

	o := resp.Result.List[0]
	return OrderStatusInfo{
		Status:       string(o.OrderStatus),
		CancelType:   o.CancelType,
		RejectReason: o.RejectReason,
	}, nil
}

// GetOpenOrders возвращает активные ордера по символу — используется при старте
// приложения, чтобы найти резидентные reduce-only ордера, оставшиеся от джоба
// лимитного закрытия, прерванного предыдущим выключением (см.
// PositionService.adoptOrphanedLimitCloseOrders).
func (b *Bybit) GetOpenOrders(symbol string) ([]OpenOrderInfo, error) {
	sym := bybit.SymbolV5(symbol)

	resp, err := b.client.V5().Order().GetOpenOrders(bybit.V5GetOpenOrdersParam{
		Category: bybit.CategoryV5Linear,
		Symbol:   &sym,
	})
	if err != nil {
		return nil, fmt.Errorf("get open orders: %w", err)
	}

	orders := make([]OpenOrderInfo, 0, len(resp.Result.List))
	for _, o := range resp.Result.List {
		price, _ := strconv.ParseFloat(o.Price, 64)
		orders = append(orders, OpenOrderInfo{
			OrderID:     o.OrderID,
			Side:        string(o.Side),
			Price:       price,
			PositionIdx: o.PositionIdx,
			ReduceOnly:  o.ReduceOnly,
		})
	}

	return orders, nil
}

func (b *Bybit) GetPositionInfo() ([]Position, error) {

	settleCoin := bybit.CoinUSDT
	param := bybit.V5GetPositionInfoParam{
		Category:   bybit.CategoryV5Linear,
		SettleCoin: &settleCoin,
	}

	resp, err := b.client.V5().Position().GetPositionInfo(param)
	if err != nil {
		return nil, fmt.Errorf("get position info: %w", err)
	}

	positions := make([]Position, 0, len(resp.Result.List))

	for _, posEx := range resp.Result.List {
		pos := Position{
			Symbol:      string(posEx.Symbol),
			CreatedTime: posEx.CreatedTime,
			PositionIdx: posEx.PositionIdx,
			Side:        string(posEx.Side),
		}

		pos.Size, _ = strconv.ParseFloat(posEx.Size, 64)
		pos.EntryPrice, _ = strconv.ParseFloat(posEx.AvgPrice, 64)
		pos.UnrealisedPnl, _ = strconv.ParseFloat(posEx.UnrealisedPnl, 64)

		positions = append(positions, pos)
	}

	return positions, nil
}

func (b *Bybit) SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error, critical bool)) (func() error, error) {

	ctx, cancel := context.WithCancel(ctx)

	b.log.Info("starting position subscription")

	reconnectAttempt := 0
	ba := &backoff.Backoff{
		Min:    30 * time.Second,
		Max:    10 * time.Minute,
		Factor: 2,
		Jitter: true,
	}

	var actualUnsub atomic.Value

	unsubscribe := func() error {
		f, _ := actualUnsub.Load().(func() error)
		if f == nil {
			return fmt.Errorf("unsubscribe not set or not connected yet")
		}
		// в библиотеке bybit unsubscribe не корректно отрабатывает, успевает прийти ошибка func not found, буду вызывать cancel
		cancel()
		return f()

	}

	dataHandler := func(msg bybit.V5WebsocketPrivatePositionResponse) error {

		for _, posEx := range msg.Data {

			pos := Position{
				Symbol:      string(posEx.Symbol),
				CreatedTime: posEx.CreatedTime,
				UpdatedTime: posEx.UpdatedTime,
				PositionIdx: int(posEx.PositionIdx),
				Side:        string(posEx.Side),
			}

			pos.Size, _ = strconv.ParseFloat(posEx.Size, 64)
			pos.EntryPrice, _ = strconv.ParseFloat(posEx.EntryPrice, 64)
			pos.UnrealisedPnl, _ = strconv.ParseFloat(posEx.UnrealisedPnl, 64)

			onData(pos)
		}
		return nil
	}

	errHandler := func(isWebsocketClosed bool, err error) {
		onError(fmt.Errorf("websocket position error (closed=%t): %w", isWebsocketClosed, err), false)
	}

	go func() {
		for {
			startDone := make(chan error, 1)

			go func() {
				timerCtx, cancelTimer := context.WithCancel(ctx)
				defer cancelTimer()

				// Таймер успешного подключения для обнуления счетчика попыток
				go func() {
					select {
					case <-time.After(60 * time.Second):
						reconnectAttempt = 0
						ba.Reset()
						b.log.Info("websocket position connection stable")
						// Позиция редко шлет данные для обновления, поэтому для обновления статуса hub.Connected
						onData(Position{})
					case <-timerCtx.Done():
						return
					}
				}()

				svc, err := b.wsPrivateClient.V5().Private()
				if err != nil {
					onError(fmt.Errorf("create private ws client: %w", err), true)
					startDone <- err
					return
				}

				if err := svc.Subscribe(); err != nil {
					onError(fmt.Errorf("subscribe ws: %w", err), true)
					startDone <- err
					return
				}

				unsub, err := svc.SubscribePosition(dataHandler)
				if err != nil {
					onError(fmt.Errorf("subscribe position: %w", err), true)
					startDone <- err
					return
				}

				actualUnsub.Store(unsub)

				startDone <- svc.Start(ctx, errHandler)
				cancelTimer()
			}()

			select {
			case <-ctx.Done():
				return
			case err := <-startDone:
				if err != nil {
					onError(fmt.Errorf("websocket position start error: %w", err), false)
				}
				select {
				case <-ctx.Done():
					return
				default:
					reconnectAttempt++
					if reconnectAttempt >= maxReconnectAttempts {
						onError(fmt.Errorf("websocket position max reconnection attempts reached (%d)", maxReconnectAttempts), true)
						return
					}

					delay := ba.Duration()
					b.log.Info("reconnecting position websocket", "attempt", reconnectAttempt, "delay", delay)
					time.Sleep(delay)
					continue
				}
			}
		}
	}()

	return unsubscribe, nil
}

func (b *Bybit) SubscribeTickerStart(ctx context.Context, symbol string, onData func(update TickerUpdate), onError func(err error, critical bool)) (func() error, error) {

	ctx, cancel := context.WithCancel(ctx)

	b.log.Info("starting ticker subscription", "symbol", symbol)

	reconnectAttempt := 0
	ba := &backoff.Backoff{
		Min:    30 * time.Second,
		Max:    10 * time.Minute,
		Factor: 2,
		Jitter: true,
	}

	var actualUnsub atomic.Value

	unsubscribe := func() error {
		f, _ := actualUnsub.Load().(func() error)
		if f == nil {
			return fmt.Errorf("unsubscribe not set or not connected yet")
		}
		// в библиотеке bybit unsubscribe не корректно отрабатывает, успевает прийти ошибка func not found, буду вызывать cancel
		cancel()
		return f()
	}

	dataHandler := func(msg bybit.V5WebsocketPublicTickerResponse) error {
		if msg.Data.LinearInverse == nil {
			return nil
		}

		// Снэпшот шлёт все поля сразу, дельты — только изменившиеся, остальные придут
		// пустой строкой. Заполняем только то, что реально пришло в этом сообщении.
		var update TickerUpdate
		if raw := msg.Data.LinearInverse.LastPrice; raw != "" {
			if v, err := strconv.ParseFloat(raw, 64); err == nil {
				update.Price = &v
			}
		}
		if raw := msg.Data.LinearInverse.Bid1Price; raw != "" {
			if v, err := strconv.ParseFloat(raw, 64); err == nil {
				update.Bid1 = &v
			}
		}
		if raw := msg.Data.LinearInverse.Ask1Price; raw != "" {
			if v, err := strconv.ParseFloat(raw, 64); err == nil {
				update.Ask1 = &v
			}
		}

		if update.Price == nil && update.Bid1 == nil && update.Ask1 == nil {
			return nil
		}

		onData(update)
		return nil
	}

	errHandler := func(isWebsocketClosed bool, err error) {
		onError(fmt.Errorf("websocket ticker error (closed=%t, symbol=%s): %w", isWebsocketClosed, symbol, err), false)
	}

	go func() {
		for {
			startDone := make(chan error, 1)

			go func() {
				timerCtx, cancelTimer := context.WithCancel(ctx)
				defer cancelTimer()

				// Таймер успешного подключения для обнуления счетчика попыток
				go func() {
					select {
					case <-time.After(60 * time.Second):
						reconnectAttempt = 0
						ba.Reset()
						b.log.Info("websocket ticker connection stable", "symbol", symbol)
					case <-timerCtx.Done():
						return
					}
				}()

				svc, err := b.wsPublicClient.V5().Public(bybit.CategoryV5Linear)
				if err != nil {
					onError(fmt.Errorf("create public ws client: %w", err), true)
					startDone <- err
					return
				}

				unsub, err := svc.SubscribeTicker(bybit.V5WebsocketPublicTickerParamKey{Symbol: bybit.SymbolV5(symbol)}, dataHandler)
				if err != nil {
					onError(fmt.Errorf("subscribe ticker %s: %w", symbol, err), true)
					startDone <- err
					return
				}

				actualUnsub.Store(unsub)

				startDone <- svc.Start(ctx, errHandler)
				cancelTimer()
			}()

			select {
			case <-ctx.Done():
				return
			case err := <-startDone:
				if err != nil {
					onError(fmt.Errorf("websocket ticker start error (symbol=%s): %w", symbol, err), false)
				}
				select {
				case <-ctx.Done():
					return
				default:
					reconnectAttempt++
					if reconnectAttempt >= maxReconnectAttempts {
						onError(fmt.Errorf("websocket ticker max reconnection attempts reached (%d, symbol=%s)", maxReconnectAttempts, symbol), true)
						return
					}

					delay := ba.Duration()
					b.log.Info("reconnecting ticker websocket", "symbol", symbol, "attempt", reconnectAttempt, "delay", delay)
					time.Sleep(delay)
					continue
				}
			}
		}
	}()

	return unsubscribe, nil
}
