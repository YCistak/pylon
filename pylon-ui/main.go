package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set at build time via -ldflags "-X main.version=...", the same way
// the daemon's is. The Makefile and the release workflow were already passing
// that flag; with no variable to receive it the linker discarded it in silence,
// so the GUI has never known what it was. It matters now that Hakkında shows
// both versions — a GUI left behind by an interrupted update is exactly what
// that screen exists to make visible.
var version = "dev"

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Pylon",
		Width:     1100,
		Height:    720,
		MinWidth:  460,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
