package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Anything",
		Width:  700,
		Height: 60, // Start small (Input Bar only)

		StartHidden: true,

		// --- APPEARANCE & BEHAVIOR ---
		Frameless:     true,
		DisableResize: true,
		AlwaysOnTop:   true,

		// Transparent background required to control rounded corners in CSS
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},

		// --- WINDOWS SPECIFIC POLISH ---
		Windows: &windows.Options{
			WebviewIsTransparent: true,         // Required for rounded corners
			WindowIsTranslucent:  true,         // Required for blur effects
			BackdropType:         windows.Mica, // Modern Windows 11 texture
			Theme:                windows.Dark,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
