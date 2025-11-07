package main

import (
	"context"
	"log/slog"
	"trade_activity_gui/exchange"
	"trade_activity_gui/hub"
)

type App struct {
	ctx context.Context
	log *slog.Logger

	hub *hub.Hub

	dataFeed *exchange.DataFeed
}

func NewApp(dataFeed *exchange.DataFeed, logger *slog.Logger) *App {
	return &App{
		log:      logger.With("component", "app"),
		hub:      hub.NewHub(),
		dataFeed: dataFeed,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
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
