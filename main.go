// Agentry Desktop: a client for agentry registries — browse scanned
// and signed agents, install them with digest + DSSE verification on-device,
// track what this machine accepted, and validate local agent folders with
// the same scanners the registry runs.
package main

import (
	"context"
	"embed"
	"log/slog"
	"os"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/blaktron/agentry-desktop/internal/app"
	"github.com/blaktron/agentry-desktop/internal/applog"
	"github.com/blaktron/agentry-desktop/internal/baked"
	"github.com/blaktron/agentry-desktop/internal/clipath"
	"github.com/blaktron/agentry-desktop/internal/dockerimages"
	"github.com/blaktron/agentry-desktop/internal/hostpath"
	"github.com/blaktron/agentry-desktop/internal/loginitem"
	"github.com/blaktron/agentry-desktop/internal/meeting"
	"github.com/blaktron/agentry-desktop/internal/uninstall"
	"github.com/blaktron/agentry-desktop/internal/webview"
)

//go:embed all:frontend/dist
var assets embed.FS

// notices is the third-party notices file the Licences page in Settings
// shows; the packages ship the same file beside the app.
//
//go:embed THIRD_PARTY_NOTICES
var notices string

func main() {
	// Meeting notes records through Windows' audio stack in a child process
	// of this same binary, so a fault there ends that process and not the
	// app (internal/meeting). No window, no other argument is read.
	if len(os.Args) > 1 && os.Args[1] == meeting.CaptureFlag {
		os.Exit(meeting.CaptureMain(os.Stdin, os.Stdout))
	}
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
	// Launched by Start Agentry when you sign in: the window starts hidden,
	// and the agents set to run when Agentry starts run in the background
	// (internal/loginitem).
	hidden := slices.Contains(os.Args[1:], loginitem.HiddenFlag)
	services.SetHidden(hidden)

	run(services, hidden)
}

// windowName is the main window's name: the one the services show, hide and
// close.
const windowName = "main"

// run builds the application, its one window and runs the event loop until
// the app quits.
func run(services *app.Services, hidden bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wails := application.New(application.Options{
		Name: "Agentry Desktop",
		// The About box on macOS.
		Description: "A client for agentry registries.\nEvery install is digest-checked and signature-verified on this machine.",
		// The runtime's logger, stated rather than defaulted. At debug level
		// the framework logs every bound call with its arguments and its
		// result, and SignIn (the access token), SaveProfile (MCP secret
		// values) and StoreModelKey (the model key) carry secrets as
		// arguments. applog withholds the arguments and the result of every
		// call and keeps the method and the error, so a stale binding is
		// still diagnosable from the log. The level is info, and errors only
		// in a production build (loglevel.go); the framework's own default
		// logger, which a production build turns off, is not used.
		Logger:   slog.New(applog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))),
		LogLevel: logLevel,
		// The bound services: every exported method on these is callable
		// from the frontend, and nothing else is.
		Services: services.Bind(),
		Assets: application.AssetOptions{
			// Every response carries the Content-Security-Policy: the one
			// HTML sink renders registry-supplied README HTML, and the
			// bindings are the local machine (internal/webview).
			Middleware: webview.Middleware,
			// The bundle, and what it does not hold: /theme.js, the stored
			// theme before the first paint.
			Handler: app.Assets(assets),
		},
		// One app per user: each persistent agent has one owner, and a
		// second launch (the app opened again while it runs in the
		// background) shows the window that is already there.
		SingleInstance: singleInstance(services),
		// A quit asks the services first (a run in flight, the agents that
		// go on in the background); the signals are theirs too.
		ShouldQuit:                  services.ShouldQuit,
		DisableDefaultSignalHandler: true,
		OnShutdown: func() {
			services.Shutdown(ctx)
			cancel()
		},
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Linux: application.LinuxOptions{
			ProgramName:   "agentry-desktop",
			ApplicationID: "net.secureagentry.agentry-desktop",
		},
	})

	window := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      windowName,
		Title:     "Agentry Desktop",
		Width:     1200,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		Hidden:    hidden,
		URL:       "/",
		// No browser context menu over the page.
		DefaultContextMenuDisabled: true,
	})
	// The window's close, before it happens: the services may keep the
	// window and hide it, or ask first.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if services.BeforeClose(ctx) {
			e.Cancel()
			return
		}
		// The shutdown runs here, off the thread that draws the window.
		services.Shutdown(ctx)
	})
	wails.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		services.Startup(ctx)
	})

	if err := wails.Run(); err != nil {
		println("Error:", err.Error())
	}
}

// singleInstance is the one-app-per-user lock: nil, which is no lock, where
// the session has no message bus to hold it on (main_linux.go).
func singleInstance(services *app.Services) *application.SingleInstanceOptions {
	if !sessionBusUsable() {
		return nil
	}
	return &application.SingleInstanceOptions{
		UniqueID:               "net.secureagentry.agentry-desktop",
		OnSecondInstanceLaunch: func(application.SecondInstanceData) { services.ShowWindow() },
	}
}
