package app

// The background app. While the Assistant or the Coding agent is enabled,
// closing the window hides it instead of quitting: the agents go on, Main
// (which lives in the window) stops, and a tray icon (the menu bar's on
// macOS) opens the window again, opens a running agent's page, or quits.
// Quit means stop: every agent's segment ends with its receipts and its
// volumes kept, and the next launch starts each agent set to run when
// Agentry starts and continues its conversation.
//
// The window's close, a quit from the tray, Cmd-Q, the Dock and a signal all
// reach OnBeforeClose alike, so a quit carries an intent of the app's own:
// the tray's Quit, macOS's application Quit and quit Apple event
// (internal/macquit), and SIGTERM, SIGINT and SIGHUP (a session ending on
// Linux) set it, and the window's close hides only without it.

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-desktop/internal/loginitem"
	"github.com/blaktron/agentry-desktop/internal/macquit"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/tray"
)

// eventTrayOpenAgent asks the page to open a persistent agent's page, from
// the tray's menu or a click on one of its task's notifications.
const eventTrayOpenAgent = "tray:open-agent"

// background is the app's background state.
type background struct {
	// quitting is the person's quit, set before the window's close runs.
	quitting atomic.Bool
	// hidden: the app was launched with loginitem.HiddenFlag.
	hidden bool
	// launching is the work the app starts on goroutines of its own: the
	// start of the agents set to run when Agentry starts, and the stop of
	// Main as the window closes. A caller that needs it over waits for it.
	launching sync.WaitGroup
}

// windowShow and windowHide are the runtime's, a test's seams.
var (
	windowShow = func(ctx context.Context) {
		runtime.WindowShow(ctx)
		runtime.WindowUnminimise(ctx)
		tray.Activate()
	}
	windowHide = runtime.WindowHide
	appQuit    = runtime.Quit
	// trayStart and traySet are the tray's, a test's seams.
	trayStart = tray.Start
	traySet   = tray.SetRunning
)

// SetHidden says the app was launched to start hidden, at sign-in.
func (s *Services) SetHidden(hidden bool) { s.bg.hidden = hidden }

// ShowWindow shows the window, as a second launch of the app does. Not a
// binding: main hands it to the window's single-instance lock.
func (s *Services) ShowWindow() {
	if s.app.ctx != nil {
		go windowShow(s.app.ctx)
	}
}

// startBackground is the background app's part of OnStartup.
func (s *Services) startBackground(ctx context.Context) {
	macquit.Install(func() { s.bg.quitting.Store(true) })
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		<-sigs
		s.quit(ctx)
	}()
	s.persistent.onState = func() { s.followAgents(ctx) }
	// A click on a task's notification opens its agent's page.
	notify.SetOnClick(func(name string) {
		if !slices.Contains(shownAgents, name) {
			return
		}
		windowShow(ctx)
		emitEvent(s.app, eventTrayOpenAgent, name)
	})
	if err := notify.Init(); err != nil {
		log.Printf("desktop notifications: %v", err)
	}
	s.followAgents(ctx)
	// Launched hidden at sign-in with nothing to run in the background, the
	// app would be invisible with no icon: it shows its window.
	if s.bg.hidden && !s.persistent.keepsRunning() {
		windowShow(ctx)
	}
	s.bg.launching.Add(1)
	go func() {
		defer s.bg.launching.Done()
		s.persistent.startOnLaunch()
	}()
}

// followAgents starts the tray once an agent that runs in the background is
// enabled, and keeps its menu to the agents running.
func (s *Services) followAgents(ctx context.Context) {
	if s.persistent.keepsRunning() {
		trayStart(tray.Menu{
			Tooltip: "Agentry",
			OnOpen:  func() { windowShow(ctx) },
			OnAgent: func(name string) {
				windowShow(ctx)
				emitEvent(s.app, eventTrayOpenAgent, name)
			},
			OnQuit: func() { s.quit(ctx) },
			Agents: backgroundAgents,
		})
	}
	traySet(s.persistent.runningLabels())
}

// quit is the person's quit: the intent, then the window's quit, which runs
// OnBeforeClose (a run in flight still asks) and OnShutdown.
func (s *Services) quit(ctx context.Context) {
	s.bg.quitting.Store(true)
	appQuit(ctx)
}

// closeToBackground hides the window instead of closing it when an agent
// that runs in the background is enabled and the person did not ask to quit,
// and stops Main, which lives in the window. It reports whether it did.
func (s *Services) closeToBackground(ctx context.Context) bool {
	if s.bg.quitting.Load() || !s.persistent.keepsRunning() {
		return false
	}
	// Hidden, the window shows no agent, whatever the page last said: a
	// hide does not always reach the page as a blur.
	s.persistent.SetViewing("")
	windowHide(ctx)
	s.followAgents(ctx)
	s.bg.launching.Add(1)
	go func() {
		defer s.bg.launching.Done()
		if _, err := s.persistent.Stop(mainAgent); err != nil {
			log.Printf("agentry-desktop: stopping Agentry Helper as the window closed: %v", err)
		}
	}()
	s.noteBackgroundOnce(ctx)
	return true
}

// noteBackgroundOnce tells the person, the first time the window hides, that
// the app is still running and where its icon is.
func (s *Services) noteBackgroundOnce(ctx context.Context) {
	st, err := settings.Load()
	if err != nil || st.BackgroundNoted {
		return
	}
	st.BackgroundNoted = true
	if err := settings.Save(st); err != nil {
		return
	}
	where, extra := "the system tray", ""
	switch goruntime.GOOS {
	case "darwin":
		where = "the menu bar"
	case "windows":
		where, extra = "the notification area", "\n\nWindows puts a new icon among the hidden icons (the ^ beside the clock). Drag it onto the taskbar to keep it in view."
	}
	go notice(ctx, "Agentry is still running",
		"Your persistent agents keep running with the window closed. The Agentry icon in "+where+" opens the window again, or quits Agentry and stops them."+extra)
}

// notice is the OS information dialog, a test's seam.
var notice = func(ctx context.Context, title, message string) {
	_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.InfoDialog, Title: title, Message: message})
}

// StartsAtSignIn reports whether Agentry starts, hidden, when the person
// signs in: the login item is there and launches this copy of the app.
func (p *PersistentAPI) StartsAtSignIn() (bool, error) {
	exe, err := executable()
	if err != nil {
		return false, err
	}
	return loginitem.Enabled(exe)
}

// SetStartsAtSignIn writes or removes the login item that starts this copy
// of the app hidden at sign-in, and answers how it now stands.
func (p *PersistentAPI) SetStartsAtSignIn(on bool) (bool, error) {
	exe, err := executable()
	if err != nil {
		return false, err
	}
	if on {
		err = loginitem.Enable(exe)
	} else {
		err = loginitem.Disable()
	}
	if err != nil {
		return false, err
	}
	return loginitem.Enabled(exe)
}

// executable is this app's binary, a test's seam.
var executable = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		return real, nil
	}
	return exe, nil
}
