package main

import (
	"context"
	"fmt"
	"log"
	"trade_activity_gui/exchange"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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

	// окно всегда поверх всех
	runtime.WindowSetAlwaysOnTop(a.ctx, true)

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

func (a *App) UpdateWindowTitle(pnl float64) error {
	var status string
	if pnl > 0 {
		status = "📈"
	} else if pnl < 0 {
		status = "📉"
	} else {
		status = "📈" // Точка
	}

	title := fmt.Sprintf("%s$%.2f", status, pnl)
	runtime.WindowSetTitle(a.ctx, title)
	return nil
}
