package main

import (
	"embed"
	"log"
	"os"
	"trade_activity_gui/exchange"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {

	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file: ", err)
	}

	APIKey := os.Getenv("EXCHANGE_BYBIT_API_KEY")
	APISecret := os.Getenv("EXCHANGE_BYBIT_SECRET_KEY")
	ex := exchange.NewBybit(APIKey, APISecret)
	dataFeed := exchange.NewDataFeed(ex)

	// Create an instance of the app structure
	app := NewApp(dataFeed)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "trade_activity_gui",
		Width:  300,
		Height: 200,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
