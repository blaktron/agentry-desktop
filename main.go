// Agentry Desktop: a client for agentry registries — browse scanned
// and signed agents, install them with digest + DSSE verification on-device,
// track what this machine accepted, and validate local agent folders with
// the same scanners the registry runs.
package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/blaktron/agentry-desktop/internal/webview"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/blaktron/agentry-desktop/internal/app"
	"github.com/blaktron/agentry-desktop/internal/applog"
	"github.com/blaktron/agentry-desktop/internal/baked"
	"github.com/blaktron/agentry-desktop/internal/clipath"
	"github.com/blaktron/agentry-desktop/internal/dockerimages"
	"github.com/blaktron/agentry-desktop/internal/hostpath"
	"github.com/blaktron/agentry-desktop/internal/uninstall"
)

//go:embed all:frontend/dist
var assets embed.FS

// notices is the third-party notices file the Licences page in Settings
// shows; the packages ship the same file beside the app.
//
//go:embed THIRD_PARTY_NOTICES
var notices string

func main() {
	// Opened from Finder, the app has launchd's PATH, which holds no docker;
	// relaunched by a self-update on Windows, it has the first launch's:
	// give it the PATH a terminal opened now would have before anything
	// looks a program up (internal/hostpath says why).
	hostpath.ForGUI()
	// A release reads back what the build was made to talk to: the
	// registry and the update channel its linker flags set
	// (internal/baked). No window, no other argument is read.
	if len(os.Args) > 1 && os.Args[1] == baked.Flag {
		os.Exit(baked.Main(os.Stdout))
	}
	// The Windows uninstaller's "Also remove my Agentry settings and
	// sign-in" runs this binary once more, headless, before deleting it
	// (build/windows/installer/project.nsi; internal/uninstall says what
	// goes). No window, no other argument is read.
	if len(os.Args) > 1 && os.Args[1] == uninstall.Flag {
		os.Exit(uninstall.Main(os.Stderr))
	}
	// Its images step, unless "Keep my Docker images" is ticked: Agentry's
	// Docker images, chosen by name (internal/dockerimages), before the
	// settings step deletes the policy that may name them.
	if len(os.Args) > 1 && os.Args[1] == uninstall.ImagesFlag {
		os.Exit(uninstall.ImagesMain(os.Stderr, dockerimages.NewCLI()))
	}
	// The installer puts the agentry CLI it carries on the user Path, and
	// the uninstaller takes it off, through the app rather than NSIS, which
	// would cut a long Path short (internal/clipath).
	if len(os.Args) > 1 && (os.Args[1] == clipath.AddFlag || os.Args[1] == clipath.RemoveFlag) {
		os.Exit(clipath.Main(os.Stderr, os.Args[1]))
	}
	services := app.New(notices)

	err := wails.Run(&options.App{
		Title:     "Agentry Desktop",
		Width:     1200,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		// The runtime's logger, stated rather than defaulted. On a dispatch
		// error Wails logs the raw IPC message, arguments included, and
		// SignIn (the access token), SaveProfile (MCP secret values) and
		// StoreModelKey (the model key) carry secrets as arguments. applog
		// withholds every call's arguments and result and keeps the method
		// and the error, so a stale binding is still diagnosable from the
		// log. The levels are Wails' own defaults, written out: INFO under
		// `wails dev`, ERROR in a production build.
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
		OnStartup:     services.Startup,
		OnBeforeClose: services.BeforeClose,
		OnShutdown:    services.Shutdown,
		Bind:          services.Bind(),
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
