package main

import (
	"context"
	"log"
	"trade_activity_gui/exchange"
)

type App struct {
	ctx      context.Context
	dataFeed *exchange.DataFeed
}

func NewApp(dataFeed *exchange.DataFeed) *App {
	return &App{
		dataFeed: dataFeed,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go func() {
		if err := a.dataFeed.Start(ctx); err != nil {
			log.Printf("Data feed unavailable: %v", err)
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.dataFeed.Stop()
}

func (a *App) GetPositions() []exchange.Position {
	return a.dataFeed.PosSrv.GetAllPosition()
}
