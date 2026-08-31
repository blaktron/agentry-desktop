// SecureAgentry Desktop: a client for agentry registries — browse scanned
// and signed agents, install them with digest + DSSE verification on-device,
// track what this machine accepted, and validate local agent folders with
// the same scanners the registry runs.
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/blaktron/agentry-desktop/internal/install"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	settingsAPI := &SettingsAPI{app: app}
	browseAPI := &BrowseAPI{app: app}
	installAPI := &InstallAPI{app: app, engine: install.NewEngine()}
	validateAPI := &ValidateAPI{app: app}
	runAPI := &RunAPI{app: app}

	err := wails.Run(&options.App{
		Title:     "SecureAgentry Desktop",
		Width:     1200,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
			settingsAPI,
			browseAPI,
			installAPI,
			validateAPI,
			runAPI,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   "SecureAgentry Desktop",
				Message: "A client for agentry registries.\nEvery install is digest-checked and signature-verified on this machine.",
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
