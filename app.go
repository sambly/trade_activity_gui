package main

import (
	"context"
	"log"
	"trade_activity_gui/exchange"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx      context.Context
	dataFeed *exchange.DataFeed
}

// NewApp creates a new App application struct
func NewApp(dataFeed *exchange.DataFeed) *App {
	return &App{
		dataFeed: dataFeed,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Делаем окно всегда поверх всех
	runtime.WindowSetAlwaysOnTop(a.ctx, true)

	if err := a.dataFeed.Start(ctx); err != nil {
		log.Printf("Data feed unavailable: %v", err)
	}
}

func (a *App) GetPositions() []exchange.Position {

	return a.dataFeed.PosSrv.GetAllPosition()
}
