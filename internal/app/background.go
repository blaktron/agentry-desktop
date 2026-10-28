package app

// The background app. While the Assistant or the Coding agent is enabled,
// or Meeting notes is on, closing the window hides it instead of quitting:
// the agents go on and calls are still noticed, Main (which lives in the
// window) stops, and a tray icon (the menu bar's on macOS) opens the window
// again, shows and acts on a meeting being recorded, opens a running
// agent's page, or quits.
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
	"errors"
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
	"github.com/blaktron/agentry-desktop/internal/meeting"
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
	// trayMu orders the tray's updates: each reads the state and draws it
	// before the next reads, so an older state is never drawn last.
	trayMu sync.Mutex
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
	// trayStart, traySet and trayMeeting are the tray's, a test's seams.
	trayStart   = tray.Start
	traySet     = tray.SetRunning
	trayMeeting = tray.SetMeeting
)

// keepsRunning says whether the app runs on in the background with its
// window closed: an agent that runs there is enabled, or Meeting notes is
// on.
func (s *Services) keepsRunning() bool {
	return s.persistent.keepsRunning() || s.meeting.keepsRunning()
}

// meetingTray is a recording as the tray shows it; canRoom says a meeting
// in the room can be recorded while none is.
func meetingTray(st *meeting.RecordingStatus, canRoom bool) tray.Meeting {
	if st == nil {
		return tray.Meeting{CanRecordRoom: canRoom}
	}
	what := st.App
	switch {
	case st.How == meeting.InRoom:
		what = "a meeting in the room"
	case st.FarSide == meeting.FarSideWholeDevice || what == "":
		what = "everything this computer plays"
	}
	return tray.Meeting{Recording: true, Paused: st.Paused, What: what}
}

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
	s.meeting.setHooks(func() { s.followAgents(ctx) }, func(*meeting.RecordingStatus) { s.followAgents(ctx) })
	// A press on one of Meeting notes' buttons (Record, Not now, Pause,
	// Resume, Stop) is the person's own, from the notification server.
	notify.SetOnAction(func(key, action string) {
		if key == meetingNoticeKey {
			s.meeting.handleAction(action)
		}
	})
	// A click on a task's notification opens its agent's page, and one on
	// Meeting notes' opens that page.
	notify.SetOnClick(func(name string) {
		if name == meetingNoticeKey {
			windowShow(ctx)
			emitEvent(s.app, eventOpenMeetings)
			return
		}
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
	if s.bg.hidden && !s.keepsRunning() {
		windowShow(ctx)
	}
	s.bg.launching.Add(1)
	go func() {
		defer s.bg.launching.Done()
		s.persistent.startOnLaunch()
	}()
}

// followAgents starts the tray once an agent that runs in the background is
// enabled or Meeting notes is on, and keeps its menu to the agents running
// and the meeting being recorded.
func (s *Services) followAgents(ctx context.Context) {
	s.bg.trayMu.Lock()
	defer s.bg.trayMu.Unlock()
	if s.keepsRunning() {
		trayStart(tray.Menu{
			Tooltip: "Agentry",
			OnOpen:  func() { windowShow(ctx) },
			OnAgent: func(name string) {
				windowShow(ctx)
				emitEvent(s.app, eventTrayOpenAgent, name)
			},
			OnQuit:          func() { s.quit(ctx) },
			Agents:          backgroundAgents,
			OnMeetingPause:  func() { s.meeting.handleAction(meeting.ActionPause) },
			OnMeetingResume: func() { s.meeting.handleAction(meeting.ActionResume) },
			OnMeetingStop:   func() { s.meeting.handleAction(meeting.ActionStop) },
			// The window comes up first, so the native dialog that asks
			// before a recording starts is seen with it.
			OnMeetingRecordRoom: func() {
				windowShow(ctx)
				// The tray has no form: the count is left to be estimated.
				if _, err := s.meeting.RecordRoom(0); err != nil && !errors.Is(err, errNotConfirmed) {
					log.Printf("meeting notes: recording a meeting in the room from the tray: %v", err)
				}
			},
		})
	}
	traySet(s.persistent.runningLabels())
	trayMeeting(meetingTray(s.meeting.recording(), s.meeting.recordsRoomsNow()))
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
	if s.bg.quitting.Load() || !s.keepsRunning() {
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
	what := "Your persistent agents keep running with the window closed."
	switch agents, meetings := s.persistent.keepsRunning(), s.meeting.keepsRunning(); {
	case agents && meetings:
		what = "Your persistent agents keep running with the window closed, and Meeting notes still notices your calls."
	case meetings:
		what = "Meeting notes still notices your calls with the window closed."
	}
	quits := "quits Agentry and stops them."
	if !s.persistent.keepsRunning() {
		quits = "quits Agentry."
	}
	go notice(ctx, "Agentry is still running",
		what+" The Agentry icon in "+where+" opens the window again, or "+quits+extra)
}

// notice is the OS information dialog, a test's seam.
var notice = func(ctx context.Context, title, message string) {
	_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.InfoDialog, Title: title, Message: message})
}

// StartsAtSignIn reports whether Agentry starts, hidden, when the person
// signs in: the login item is there and launches this copy of the app.
func (p *PersistentAPI) StartsAtSignIn() (bool, error) { return startsAtSignIn() }

// SetStartsAtSignIn writes or removes the login item that starts this copy
// of the app hidden at sign-in, and answers how it now stands.
func (p *PersistentAPI) SetStartsAtSignIn(on bool) (bool, error) { return setStartsAtSignIn(on) }

func startsAtSignIn() (bool, error) {
	exe, err := executable()
	if err != nil {
		return false, err
	}
	return loginitem.Enabled(exe)
}

func setStartsAtSignIn(on bool) (bool, error) {
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
