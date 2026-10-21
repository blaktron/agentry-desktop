// Package app is the desktop's bound service layer: the App and the APIs the
// window binds, each turning the internal packages' work into the view models
// and events the frontend reads. The entry point builds them once with New and
// hands Wails the lifecycle hooks and the list to bind.
package app

import (
	"context"
	"sync"

	"github.com/blaktron/agentry-desktop/internal/install"
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
	schedule     *ScheduleAPI
	destinations *DestinationsAPI
	notices      *NoticesAPI
	persistent   *PersistentAPI
}

// New builds the App and its APIs. notices is the third-party notices text
// the Licences page shows.
func New(notices string) *Services {
	app := NewApp()
	run := &RunAPI{app: app}
	browse := &BrowseAPI{app: app}
	installs := &InstallAPI{app: app, engine: install.NewEngine()}
	schedule := &ScheduleAPI{app: app, run: run}
	link := newMainLink(app, appMainOps{browse: browse, installer: installs, runner: run, scheduler: schedule}, run.runnerHome)
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
		selfUpdate:   newSelfUpdateAPI(app),
		schedule:     schedule,
		destinations: &DestinationsAPI{app: app},
		notices:      &NoticesAPI{text: notices},
		persistent:   &PersistentAPI{app: app, run: run, link: link},
	}
}

// Startup is the window's OnStartup.
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
}

// BeforeClose is the window's OnBeforeClose. A run in flight is ended first:
// closing the window with one live asks in a native dialog, and Shutdown
// cancels the run, waits (bounded) for the engine's teardown and sweeps the
// run's label, so no agent or gateway container outlives the app.
func (s *Services) BeforeClose(ctx context.Context) (prevent bool) {
	return s.run.beforeClose(ctx)
}

// Shutdown is the window's OnShutdown. It also applies "Install automatically
// when I quit", which is off by default and does nothing unless a verified
// release is waiting and the person turned it on. OnShutdown rather than
// OnBeforeClose: by here the window is going anyway, so replacing the
// application cannot interrupt anything the person is still doing.
//
// The persistent agents running here stop beside the run's teardown, each
// segment ending without a final result and its volumes kept, so the next
// start continues each conversation; both before the managed engine stops.
func (s *Services) Shutdown(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.persistent.stopAll()
	}()
	s.run.shutdown()
	wg.Wait()
	s.run.stopManagedEngine(ctx)
	s.selfUpdate.applyOnQuit()
}

// Bind is what the window binds: every exported method on these is callable
// from the frontend, and nothing else is.
func (s *Services) Bind() []interface{} {
	return []interface{}{
		s.app,
		s.settings,
		s.browse,
		s.install,
		s.validate,
		s.run,
		s.build,
		s.knowledge,
		s.voice,
		s.home,
		s.publisher,
		s.selfUpdate,
		s.schedule,
		s.destinations,
		s.notices,
		s.persistent,
	}
}
