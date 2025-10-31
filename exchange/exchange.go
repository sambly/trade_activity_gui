package exchange

import (
	"context"
	"fmt"
	"log/slog"
)

type Exchange interface {
	GetPositionInfo() ([]Position, error)

	SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error)) error
	SubscribeTickerStart(ctx context.Context, symbol string, onData func(price float64), onError func(err error)) (func() error, error)
}

type DataFeed struct {
	log      *slog.Logger
	Exchange Exchange
	PosSrv   *PositionService
}

func NewDataFeed(exchange Exchange, logger *slog.Logger) *DataFeed {
	posSrv := NewPositionService(exchange, logger)
	return &DataFeed{
		log:      logger.With("component", "exchange"),
		Exchange: exchange,
		PosSrv:   posSrv,
	}
}

func (d *DataFeed) Start(ctx context.Context) error {

	if err := d.PosSrv.Init(); err != nil {
		return fmt.Errorf("init positions: %w", err)
	}

	for _, pos := range d.PosSrv.position {
		p := pos
		go func() {
			if err := d.PosSrv.AddSubscribeTicker(ctx, p.Symbol); err != nil {
				d.log.Error("failed to add position and subscribe to ticker",
					"symbol", p.Symbol,
					"error", err,
					"operation", "subscribe_ticker")
			} else {
				d.log.Info("successfully added position and subscribed to ticker",
					"symbol", p.Symbol,
					"operation", "subscribe_ticker")
			}
		}()
	}
	d.PosSrv.SubscribePositionStart(ctx)

	return nil
}

func (d *DataFeed) Stop() {

	d.PosSrv.StopAllSubscriptions()
	d.log.Info("Context cancelled, stopping data feed...")
}
