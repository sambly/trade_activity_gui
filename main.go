package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"trade_activity_gui/exchange"
	"trade_activity_gui/hub"
	"trade_activity_gui/logger"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	loggerWails "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// В зависимости от wails dev или wails build формируются разные -tags
func getBuildMode() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "-tags" {
				tags := strings.Split(setting.Value, ",")
				for _, tag := range tags {
					if tag == "production" {
						return "production"
					}
				}
			}
		}
	}
	return "dev"
}

func main() {

	debug := false
	build := getBuildMode()
	if build == "dev" {
		debug = true
	}

	logFile, err := logger.SetupLogger(build)
	if err != nil {
		log.Fatal("Failed to setup logger: ", err)
	}
	defer func() {
		slog.Info("Application stop")
		logFile.Close()
	}()

	slog.Info("Application starting")

	if err := godotenv.Load(); err != nil {
		slog.Warn("Failed to load .env file", "error", err)
	}

	APIKey := os.Getenv("EXCHANGE_BYBIT_API_KEY")
	APISecret := os.Getenv("EXCHANGE_BYBIT_SECRET_KEY")

	if APIKey == "" || APISecret == "" {
		slog.Error("Missing API credentials")
		panic("EXCHANGE_BYBIT_API_KEY and EXCHANGE_BYBIT_SECRET_KEY must be set")
	}

	ex := exchange.NewBybit(
		APIKey,
		APISecret,
		slog.Default(),
		debug,
	)

	hub := hub.NewHub()

	dataFeed := exchange.NewDataFeed(ex, slog.Default(), hub)

	app := NewApp(dataFeed, slog.Default())

	err = wails.Run(&options.App{
		Title:  "",
		Width:  160,
		Height: 52,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Frameless:          true,     // Убирает рамку окна
		CSSDragProperty:    "widows", // Для перемещения без рамки
		CSSDragValue:       "1",      // Для перемещения без рамки
		AlwaysOnTop:        true,     // Окно поверх всех окон,
		DisableResize:      true,     // Запрет изменения размера окна
		Logger:             logger.NewWailsLoggerAdapter(slog.Default()),
		LogLevel:           loggerWails.DEBUG,
		LogLevelProduction: loggerWails.WARNING,
	})
	if err != nil {
		slog.Error("Wails application failed", "error", err)
		panic(fmt.Sprintf("Wails application failed %v", err))

	}
}
