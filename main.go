// Agentry Desktop: a client for agentry registries — browse scanned
// and signed agents, install them with digest + DSSE verification on-device,
// track what this machine accepted, and validate local agent folders with
// the same scanners the registry runs.
package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/blaktron/agentry-desktop/internal/webview"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/blaktron/agentry-desktop/internal/applog"
	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/uninstall"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// The Windows uninstaller's "Also remove my Agentry settings and
	// sign-in" runs this binary once more, headless, before deleting it
	// (build/windows/installer/project.nsi; internal/uninstall says what
	// goes). No window, no other argument is read.
	if len(os.Args) > 1 && os.Args[1] == uninstall.Flag {
		os.Exit(uninstall.Main(os.Stderr))
	}
	app := NewApp()
	settingsAPI := &SettingsAPI{app: app}
	browseAPI := &BrowseAPI{app: app}
	installAPI := &InstallAPI{app: app, engine: install.NewEngine()}
	validateAPI := &ValidateAPI{app: app}
	runAPI := &RunAPI{app: app}
	buildAPI := &BuildAPI{app: app}
	selfUpdateAPI := newSelfUpdateAPI(app)

	err := wails.Run(&options.App{
		Title:     "Agentry Desktop",
		Width:     1200,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		// The runtime's logger, stated rather than defaulted
		// (agentry-desktop#102). On a dispatch error Wails logs the raw
		// IPC message, arguments included, and SignIn (the access token),
		// SaveProfile (MCP secret values) and StoreModelKey (the model key)
		// carry secrets as arguments. applog withholds every call's
		// arguments and result and keeps the method and the error, so a
		// stale binding is still diagnosable from the log. The levels are
		// Wails' own defaults, written out: INFO under `wails dev`, ERROR
		// in a production build.
		Logger:             applog.New(logger.NewDefaultLogger()),
		LogLevel:           logger.INFO,
		LogLevelProduction: logger.ERROR,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// Every response carries the Content-Security-Policy: the one
			// HTML sink renders registry-supplied README HTML, and the
			// bindings are the local machine (internal/webview).
			Middleware: webview.Middleware,
		},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			// A run an earlier session was killed in the middle of is
			// swept by its record (agentry-desktop#198); in its own
			// goroutine, since Docker may be slow or not yet running.
			go runAPI.sweepOrphans(ctx)
			// Last, and in its own goroutine: the check for a newer Agentry
			// Desktop must not delay the first paint or the first-run wizard
			// (agentry-desktop#87).
			selfUpdateAPI.start(ctx)
			// And the installed BriefAgents' check, once (agentry-desktop#147).
			installAPI.start(ctx)
		},
		// "Install automatically when I quit" (agentry-desktop#88, D6). Off
		// by default; does nothing unless a verified release is waiting and
		// the person turned it on. OnShutdown rather than OnBeforeClose: by
		// here the window is going anyway, so replacing the application
		// cannot interrupt anything the person is still doing.
		//
		// A run in flight is ended first (agentry-desktop#125): closing the
		// window with one live asks in a native dialog, and shutdown cancels
		// the run, waits (bounded) for the engine's teardown and sweeps the
		// run's label, so no agent or gateway container outlives the app.
		OnBeforeClose: runAPI.beforeClose,
		OnShutdown: func(context.Context) {
			runAPI.shutdown()
			selfUpdateAPI.applyOnQuit()
		},
		Bind: []interface{}{
			app,
			settingsAPI,
			browseAPI,
			installAPI,
			validateAPI,
			runAPI,
			buildAPI,
			selfUpdateAPI,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   "Agentry Desktop",
				Message: "A client for agentry registries.\nEvery install is digest-checked and signature-verified on this machine.",
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
