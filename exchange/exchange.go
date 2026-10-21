package exchange

import (
	"context"
	"log/slog"
	"trade_activity_gui/hub"
)

// TickerUpdate — данные тикера, которые реально пришли в конкретном WS-сообщении.
// Bybit шлёт снэпшот целиком, а дальше дельты только с изменившимися полями, поэтому
// значения, отсутствовавшие в конкретном сообщении, — nil, а не 0.
type TickerUpdate struct {
	Price *float64
	Bid1  *float64
	Ask1  *float64
}

// InstrumentInfo — торговые ограничения инструмента, нужны для округления цены/объёма
// ордера до допустимого шага перед отправкой на биржу.
type InstrumentInfo struct {
	TickSize float64
	QtyStep  float64
}

// OrderStatusInfo — финальный статус ордера из истории, для диагностики, когда
// AmendOrderPrice/CancelOrder отвечают "order not exists": помогает понять, был ли
// ордер исполнен, отменён биржей (и почему) или отклонён при создании.
type OrderStatusInfo struct {
	Status       string
	CancelType   string
	RejectReason string
}

// OpenOrderInfo — активный ордер на бирже. Нужен, чтобы при старте приложения найти
// резидентные reduce-only ордера, оставшиеся от джоба лимитного закрытия, прерванного
// предыдущим выключением приложения (см. PositionService.adoptOrphanedLimitCloseOrders).
type OpenOrderInfo struct {
	OrderID     string
	Side        string
	Price       float64
	PositionIdx int
	ReduceOnly  bool
}

type Exchange interface {
	GetPositionInfo() ([]Position, error)
	GetInstrumentInfo(symbol string) (InstrumentInfo, error)

	SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error, critical bool)) (func() error, error)
	SubscribeTickerStart(ctx context.Context, symbol string, onData func(update TickerUpdate), onError func(err error, critical bool)) (func() error, error)

	// PlaceReduceOnlyLimitOrder ставит reduce-only PostOnly лимитный ордер — он может
	// только закрывать/уменьшать позицию, никогда не откроет противоположную. positionIdx
	// берётся из Position.PositionIdx: 0 в One-Way Mode, 1/2 (Buy/Sell) в Hedge Mode — в
	// Hedge Mode Bybit требует его явно и отклоняет запрос без него ("position idx not
	// match position mode").
	PlaceReduceOnlyLimitOrder(symbol, side string, qty, price float64, positionIdx int) (orderID string, err error)
	// AmendOrderPrice переставляет цену уже стоящего ордера, не отменяя его — дешевле
	// и без гонки, чем cancel + create.
	AmendOrderPrice(symbol, orderID string, price float64) error
	CancelOrder(symbol, orderID string) error
	// GetOrderStatus — см. OrderStatusInfo, только для диагностики/логов.
	GetOrderStatus(symbol, orderID string) (OrderStatusInfo, error)
	// GetOpenOrders возвращает активные ордера по символу — см. OpenOrderInfo.
	GetOpenOrders(symbol string) ([]OpenOrderInfo, error)
}

type DataFeed struct {
	log        *slog.Logger
	exchange   Exchange
	PosService *PositionService
}

func NewDataFeed(exchange Exchange, logger *slog.Logger, hub *hub.Hub) *DataFeed {
	posSrv := NewPositionService(exchange, logger, hub)
	return &DataFeed{
		log:        logger.With("component", "exchange"),
		exchange:   exchange,
		PosService: posSrv,
	}
}

func (d *DataFeed) Start(ctx context.Context) error {
	d.PosService.Start(ctx)
	return nil
}

func (d *DataFeed) Stop() {
	d.PosService.StopAllSubscriptions()
}
