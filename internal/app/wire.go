// Package app is the desktop's bound service layer: the App and the APIs the
// window binds, each turning the internal packages' work into the view models
// and events the frontend reads. The entry point builds them once with New and
// hands the application the lifecycle hooks and the list to bind.
package app

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/tray"
)

// Services is the App and every API the window binds, wired to one another.
// It is not bound itself: Bind lists what the frontend may call.
type Services struct {
	app          *App
	settings     *SettingsAPI
	browse       *BrowseAPI
	install      *InstallAPI
	validate     *ValidateAPI
	run          *RunAPI
	build        *BuildAPI
	knowledge    *KnowledgeAPI
	voice        *VoiceAPI
	home         *HomeAPI
	publisher    *PublisherAPI
	selfUpdate   *SelfUpdateAPI
	whatsNew     *WhatsNewAPI
	schedule     *ScheduleAPI
	destinations *DestinationsAPI
	notices      *NoticesAPI
	persistent   *PersistentAPI
	meeting      *MeetingAPI
	bg           background
	stopOnce     sync.Once
}

// New builds the App and its APIs. notices is the third-party notices text
// the Licences page shows.
func New(notices string) *Services {
	app := NewApp()
	run := &RunAPI{app: app}
	browse := &BrowseAPI{app: app}
	installs := &InstallAPI{app: app, engine: install.NewEngine()}
	schedule := &ScheduleAPI{app: app, run: run}
	ops := appMainOps{browse: browse, installer: installs, runner: run, scheduler: schedule}
	links := map[string]*mainLink{
		mainAgent:      newAgentLink(app, ops, run.runnerHome, mainAgent),
		assistantAgent: newAgentLink(app, ops, run.runnerHome, assistantAgent),
	}
	persistentAPI := &PersistentAPI{app: app, run: run, links: links}
	// The Assistant's meeting tools are Meeting notes', and Meeting notes
	// hands its transcripts to the Assistant.
	meetings := &MeetingAPI{app: app, assistant: persistentAssistant{p: persistentAPI}}
	links[assistantAgent].meetings = meetings.answerTool
	return &Services{
		app:          app,
		settings:     &SettingsAPI{app: app},
		browse:       browse,
		install:      installs,
		validate:     &ValidateAPI{app: app},
		run:          run,
		build:        &BuildAPI{app: app, run: run},
		knowledge:    &KnowledgeAPI{app: app, run: run},
		voice:        &VoiceAPI{app: app, run: run},
		home:         &HomeAPI{},
		publisher:    &PublisherAPI{app: app},
		selfUpdate:   newSelfUpdateAPI(app, meetings.suspendForUpdate),
		whatsNew:     newWhatsNewAPI(),
		schedule:     schedule,
		destinations: &DestinationsAPI{app: app},
		notices:      &NoticesAPI{text: notices},
		persistent:   persistentAPI,
		meeting:      meetings,
	}
}

// Startup is the application's start: the window exists and the event loop
// runs.
func (s *Services) Startup(ctx context.Context) {
	s.app.startup(ctx)
	// The engine the app manages itself, where one is installed, is
	// started; in its own goroutine, since a VM boot takes a while and
	// the card says what detection finds meanwhile.
	go s.run.startManagedEngine(ctx)
	// A run an earlier session was killed in the middle of is swept
	// by its record; in its own goroutine, since Docker may be slow
	// or not yet running.
	go s.run.sweepOrphans(ctx)
	// Last, and in its own goroutine: the check for a newer Agentry
	// Desktop must not delay the first paint or the first-run
	// wizard.
	s.selfUpdate.start(ctx)
	// And the installed QuickAgents' check, once.
	s.install.start(ctx)
	// The schedules' alarms rewritten from the store.
	s.schedule.start(ctx)
	// An older agentry CLI from the site's install script, first on
	// the PATH, brought up to the one this app carries; in its own
	// goroutine, since it runs both CLIs' --version.
	go refreshScriptCLI()
	// The background app: the quit intent, the tray, and the agents set to
	// run when Agentry starts (background.go).
	s.startBackground(ctx)
	// Agents another client starts or stops are seen within seconds.
	go s.persistent.watchElsewhere(ctx)
	// Meeting notes' leftovers recovered and its old audio swept; no
	// audio device is opened until the person presses Record.
	s.meeting.start(ctx)
}

// BeforeClose runs as the window closes, and reports whether to keep the
// window. A run in flight is ended first: closing the window with one live
// asks in a native dialog, and Shutdown cancels the run, waits (bounded) for
// the engine's teardown and sweeps the run's label, so no agent or gateway
// container outlives the app.
//
// While the Assistant or the Coding agent is enabled, the window's close
// hides the window instead, unless the person asked to quit (background.go).
func (s *Services) BeforeClose(ctx context.Context) (prevent bool) {
	// Asked already, and shut down: the application closing its own window
	// as it ends is not a close to ask about again.
	if s.bg.approved.Load() {
		return false
	}
	if s.closeToBackground(ctx) {
		return true
	}
	prevent = s.run.beforeClose(ctx)
	if prevent {
		// The person kept the run going: the next close is a close again.
		s.bg.quitting.Store(false)
	}
	return prevent
}

// Shutdown is the app's shutdown, run once and before the event loop ends
// (the window's close, a quit): the loop must stay free while it works, since
// the work reports to the page. It also applies "Install automatically when I
// quit", which is off by default and does nothing unless a verified release is
// waiting and the person turned it on. It follows BeforeClose: by here the
// window is going anyway, so replacing the application cannot interrupt
// anything the person is still doing. Approving the quit is its last step.
//
// The persistent agents running here stop beside the run's teardown, each
// segment ending without a final result and its volumes kept, so the next
// start continues each conversation; both before the managed engine stops.
func (s *Services) Shutdown(ctx context.Context) {
	s.stopOnce.Do(func() {
		shutdownWork(s, ctx)
		s.bg.approved.Store(true)
	})
}

// shutdownWork is what Shutdown runs, a test's seam: a test of the quit's
// order has no meeting notes, update or run engines to stop.
var shutdownWork = (*Services).shutdown

func (s *Services) shutdown(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.persistent.stopAll()
	}()
	// A meeting being recorded is kept for the next launch to transcribe.
	s.meeting.shutdown()
	s.run.shutdown()
	wg.Wait()
	s.run.stopManagedEngine(ctx)
	tray.Stop()
	s.selfUpdate.applyOnQuit()
}

// Bind is what the window binds: every exported method on these is callable
// from the frontend, and nothing else is.
func (s *Services) Bind() []application.Service {
	return []application.Service{
		application.NewService(s.app),
		application.NewService(s.settings),
		application.NewService(s.browse),
		application.NewService(s.install),
		application.NewService(s.validate),
		application.NewService(s.run),
		application.NewService(s.build),
		application.NewService(s.knowledge),
		application.NewService(s.voice),
		application.NewService(s.home),
		application.NewService(s.publisher),
		application.NewService(s.selfUpdate),
		application.NewService(s.whatsNew),
		application.NewService(s.schedule),
		application.NewService(s.destinations),
		application.NewService(s.notices),
		application.NewService(s.persistent),
		application.NewService(s.meeting),
	}
}
