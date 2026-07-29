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
	"trade_activity_gui/tray"

	"fyne.io/systray"
	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	loggerWails "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	optionsWindows "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

// windowClassName — имя класса нативного окна (Windows). Используется в tray.HideFromTaskbar
// для поиска HWND через FindWindow; задаётся отдельно от дефолтного "wailsWindow", чтобы не
// зацепить окно другого wails-приложения, если оно тоже запущено в системе.
const windowClassName = "TradeActivityGuiWindow"

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

	ex, err := exchange.NewBybit(
		APIKey,
		APISecret,
		slog.Default(),
		debug,
	)

	if err != nil {
		slog.Error("Failed to create Bybit exchange", "error", err)
		panic(fmt.Sprintf("Failed to create Bybit exchange: %v", err))
	}

	hub := hub.NewHub()

	dataFeed := exchange.NewDataFeed(ex, slog.Default(), hub)

	app := NewApp(dataFeed, slog.Default(), hub)

	// wails.Run владеет нативным окном и его циклом сообщений (WebView2 требует, чтобы это
	// происходило на стабильном, не мигрирующем между OS-потоками, goroutine) — поэтому именно
	// он остаётся блокирующим вызовом в main(), как и раньше. Трей запускаем в отдельной
	// горутине; tray.Run сам вызывает runtime.LockOSThread(), чтобы его собственный цикл
	// сообщений тоже был закреплён за одним потоком.
	go tray.Run(trayIcon, tray.Callbacks{
		OnShow: app.ShowWindow,
		OnHide: app.HideWindow,
		OnQuit: app.QuitApp,
	})

	err = wails.Run(&options.App{
		Title:  "",
		Width:  180,
		Height: 52,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:  &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		StartHidden:       true, // Запускаемся сразу свёрнутыми в трей
		HideWindowOnClose: true, // Alt+F4/закрытие прячет окно вместо выхода из приложения
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
		Windows: &optionsWindows.Options{
			WindowClassName: windowClassName,
		},
	})
	if err != nil {
		slog.Error("Wails application failed", "error", err)
		panic(fmt.Sprintf("Wails application failed %v", err))
	}
	// Окно закрылось (обычно через пункт "Выход" в трее) — гасим иконку трея,
	// чтобы разблокировать tray.Run() и дать процессу завершиться.
	systray.Quit()
}
