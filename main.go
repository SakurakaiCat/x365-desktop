package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Open365VPN",
		Width:  900,
		Height: 640,
		MinWidth:  520,
		MinHeight: 420,
		AssetServer: &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 26, G: 26, B: 46, A: 1},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,
		OnDomReady:    app.OnDomReady,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}