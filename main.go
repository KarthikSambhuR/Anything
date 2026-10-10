package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

var assets embed.FS

func main() {
	isSettings := false
	for _, arg := range os.Args {
		if arg == "-settings" {
			isSettings = true
			break
		}
	}

	app := NewApp(isSettings)

	if isSettings {
		err := wails.Run(&options.App{
			Title:  "Anything Settings",
			Width:  900,
			Height: 650,
			StartHidden: false,
			Frameless:   true,
			DisableResize: false,
			BackgroundColour: &options.RGBA{R: 27, G: 27, B: 36, A: 255},
			AssetServer: &assetserver.Options{
				Assets: assets,
			},
			OnStartup:  app.startup,
			OnShutdown: app.shutdown,
			Bind: []interface{}{app},
			Windows: &windows.Options{
				Theme: windows.Dark,
			},
		})
		if err != nil {
			println("Error:", err.Error())
		}
		return
	}

	err := wails.Run(&options.App{
		Title:  "Anything",
		Width:  700,
		Height: 60,

		StartHidden: true,

		// --- APPEARANCE & BEHAVIOR ---
		Frameless:     true,
		DisableResize: false,
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
