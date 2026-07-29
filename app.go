package main

import (
	"context"
	"log/slog"
	"trade_activity_gui/exchange"
	"trade_activity_gui/hub"
	"trade_activity_gui/tray"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	log *slog.Logger

	hub *hub.Hub

	dataFeed *exchange.DataFeed
}

func NewApp(dataFeed *exchange.DataFeed, logger *slog.Logger, hub *hub.Hub) *App {
	return &App{
		log:      logger.With("component", "app"),
		hub:      hub,
		dataFeed: dataFeed,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	tray.HideFromTaskbar(windowClassName)
	go func() {
		if err := a.dataFeed.Start(ctx); err != nil {
			a.log.Error("data feed unavailable", "error", err)
			a.hub.UpdateStatus(hub.Error)
			a.dataFeed.Stop()
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.dataFeed.Stop()
}

func (a *App) GetPositions() []exchange.Position {
	return a.dataFeed.PosService.GetAllPosition()
}

func (a *App) GetConnectionStatus() string {
	if a.hub == nil {
		return "Error"
	}
	return string(a.hub.GetOverallStatus())
}

// ShowWindow, HideWindow и QuitApp вызываются из меню в системном трее (tray/tray.go).
// a.ctx устанавливается в startup и может быть ещё nil, если по иконке трея кликнули
// в первые миллисекунды после запуска, пока Wails не успел вызвать OnStartup.
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsRuntime.WindowShow(a.ctx)
	}
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		wailsRuntime.WindowHide(a.ctx)
	}
}

func (a *App) QuitApp() {
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
	}
}
