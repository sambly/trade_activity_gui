package exchange

import (
	"context"
	"log/slog"
	"trade_activity_gui/hub"
)

type Exchange interface {
	GetPositionInfo() ([]Position, error)

	SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error, critical bool)) (func() error, error)
	SubscribeTickerStart(ctx context.Context, symbol string, onData func(price float64), onError func(err error, critical bool)) (func() error, error)
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
