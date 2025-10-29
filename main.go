package main

import (
	"embed"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"trade_activity_gui/exchange"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func setupLogger() (*os.File, error) {

	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)

	logsDir := filepath.Join(exeDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logsDir, "app.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	var multiWriter io.Writer
	build := getBuildMode()

	switch build {
	case "production":
		multiWriter = io.MultiWriter(logFile)
	default:
		multiWriter = io.MultiWriter(os.Stdout, logFile)
	}

	handler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.SourceKey:
				if source, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(filepath.Base(source.File) + ":" + strconv.Itoa(source.Line))
				}
			case slog.TimeKey:
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05"))
				}
			}
			return a
		},
	})

	slog.SetDefault(slog.New(handler))

	// Стандартный log для совместимости
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return logFile, nil
}

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

	logFile, err := setupLogger()
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

	ex := exchange.NewBybit(APIKey, APISecret)
	dataFeed := exchange.NewDataFeed(ex)

	app := NewApp(dataFeed)

	err = wails.Run(&options.App{
		Title:  "",
		Width:  135,
		Height: 55,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Frameless:       true,     // Убирает рамку окна
		CSSDragProperty: "widows", // Для перемещения без рамки
		CSSDragValue:    "1",      // Для перемещения без рамки
		AlwaysOnTop:     true,     // Окно поверх всех окон
	})
	if err != nil {
		slog.Error("Wails application failed", "error", err)
		panic(fmt.Sprintf("Wails application failed %v", err))

	}
}
