package exchange

import (
	"context"
	"fmt"
	"log"
)

type Exchange interface {
	GetPositionInfo() ([]Position, error)

	SubscribePositionStart(ctx context.Context, onData func(pos Position), onError func(err error)) error
	SubscribeTickerStart(ctx context.Context, symbol string, onData func(price float64), onError func(err error)) (func() error, error)
}

type DataFeed struct {
	Exchange Exchange
	PosSrv   *PositionService
}

func NewDataFeed(exchange Exchange) *DataFeed {

	posSrv := NewPositionService(exchange)
	return &DataFeed{
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
				log.Printf("➕ ❌ Error adding position & subscribing to ticker %s: %v", p.Symbol, err)
			} else {
				log.Printf("➕ ✅ Successfully added position & subscribed to ticker: %s", p.Symbol)
			}
		}()
	}
	d.PosSrv.SubscribePositionStart(ctx)

	<-ctx.Done()
	log.Println("🛑 Context cancelled, stopping data feed...")

	d.PosSrv.StopAllSubscriptions()

	return nil
}
