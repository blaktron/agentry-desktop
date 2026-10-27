package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"

	"github.com/blaktron/agentry-desktop/internal/tray"
)

// bgRig is the background app over a persistent rig, its window and tray
// played by the test.
type bgRig struct {
	*persistentRig
	s *Services

	mu      sync.Mutex
	hidden  int
	shown   int
	quits   int
	trays   int
	notices []string
	running map[string]string
}

func newBGRig(t *testing.T) *bgRig {
	t.Helper()
	pr := newPersistentRig(t)
	r := &bgRig{persistentRig: pr, s: &Services{app: pr.p.app, run: pr.p.run, persistent: pr.p}}
	saved := []any{windowShow, windowHide, appQuit, trayStart, traySet, notice}
	windowShow = func(context.Context) { r.mu.Lock(); r.shown++; r.mu.Unlock() }
	windowHide = func(context.Context) { r.mu.Lock(); r.hidden++; r.mu.Unlock() }
	appQuit = func(context.Context) { r.mu.Lock(); r.quits++; r.mu.Unlock() }
	trayStart = func(tray.Menu) { r.mu.Lock(); r.trays++; r.mu.Unlock() }
	traySet = func(m map[string]string) { r.mu.Lock(); r.running = m; r.mu.Unlock() }
	notice = func(_ context.Context, title, _ string) {
		r.mu.Lock()
		r.notices = append(r.notices, title)
		r.mu.Unlock()
	}
	t.Cleanup(func() {
		// Work the app started on goroutines reads these hooks, and an agent
		// that is stopped tells the tray: all of it ends before they are put
		// back, or the tray this machine really has is started.
		r.s.bg.launching.Wait()
		r.p.stopAll()
		r.p.followers.Wait()
		windowShow = saved[0].(func(context.Context))
		windowHide = saved[1].(func(context.Context))
		appQuit = saved[2].(func(context.Context))
		trayStart = saved[3].(func(tray.Menu))
		traySet = saved[4].(func(map[string]string))
		notice = saved[5].(func(context.Context, string, string))
	})
	r.s.persistent.onState = func() { r.s.followAgents(context.Background()) }
	return r
}

// With no agent that runs in the background enabled, the window's close is
// a close: nothing hides and no tray starts.
func TestWithNoBackgroundAgentTheCloseQuits(t *testing.T) {
	r := newBGRig(t)
	if prevent := r.s.BeforeClose(context.Background()); prevent || r.hidden != 0 || r.trays != 0 {
		t.Fatalf("prevent=%v hidden=%d trays=%d", prevent, r.hidden, r.trays)
	}
}

// With the Assistant enabled, the window's close hides the window, starts
// the tray, stops Main, and says so once; the person's quit is a quit. Main
// runs beside the Assistant only when the Assistant is enabled while Main
// runs, as the CLI's agentry agent enable does.
func TestTheCloseHidesWhileABackgroundAgentIsEnabledAndQuitQuits(t *testing.T) {
	r := newBGRig(t)
	if _, err := r.p.SetEnabled(mainAgent, true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.p.Start(mainAgent); err != nil {
		t.Fatal(err)
	}
	if err := persistent.SaveConfig(r.home, assistantAgent, persistent.Config{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	r.p.stateChanged(assistantAgent)
	if r.trays == 0 {
		t.Fatal("enabling the Assistant started no tray")
	}
	for range 2 {
		if prevent := r.s.BeforeClose(context.Background()); !prevent {
			t.Fatal("the close went through with the Assistant enabled")
		}
	}
	r.p.followers.Wait()
	// Main is stopped on a goroutine of its own, as the note is shown.
	r.s.bg.launching.Wait()
	var hidden int
	var notices []string
	for range 100 {
		r.mu.Lock()
		hidden, notices = r.hidden, append([]string{}, r.notices...)
		r.mu.Unlock()
		if len(notices) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hidden != 2 || len(notices) != 1 {
		t.Fatalf("hidden %d times, %d notices", hidden, len(notices))
	}
	if _, ours := r.p.running(mainAgent); ours {
		t.Fatal("Main still runs with its window closed")
	}

	r.s.quit(context.Background())
	if r.quits != 1 {
		t.Fatalf("quit asked the window to quit %d times", r.quits)
	}
	if prevent := r.s.BeforeClose(context.Background()); prevent {
		t.Fatal("the person's quit was hidden")
	}
}

// The tray names the agents running here, by the name the person gave them.
func TestTheTrayFollowsTheRunningAgents(t *testing.T) {
	r := newBGRig(t)
	if _, err := r.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	name := "Sandra"
	if _, err := r.p.Configure("assistant", PersistentChange{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	got := r.running["assistant"]
	r.mu.Unlock()
	if got != "Sandra" {
		t.Fatalf("the tray shows %q", got)
	}
}

// At launch, an enabled agent set to run when Agentry starts is started; one
// that is not set, and one whose roster needs the person's agreement, are
// not, and no dialog is raised.
func TestAgentsSetToRunWhenAgentryStartsStartAtLaunch(t *testing.T) {
	r := newBGRig(t)
	on := true
	if _, err := r.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.p.Configure("assistant", PersistentChange{RunOnStart: &on}); err != nil {
		t.Fatal(err)
	}
	// Unagreed: the launch leaves it.
	r.p.startOnLaunch()
	if r.started != 0 || len(r.dialogs()) != 0 {
		t.Fatalf("started %d, dialogs %q", r.started, r.dialogs())
	}
	a, _ := persistent.Lookup("assistant")
	if err := persistent.AgreeToRoster(r.home, a, persistent.ClientDesktop); err != nil {
		t.Fatal(err)
	}
	r.p.startOnLaunch()
	if r.started != 1 || len(r.dialogs()) != 0 {
		t.Fatalf("started %d, dialogs %q", r.started, r.dialogs())
	}
}

// Launched hidden with nothing to run in the background, the app shows its
// window rather than run invisible.
func TestAHiddenLaunchWithNothingToRunShowsTheWindow(t *testing.T) {
	r := newBGRig(t)
	r.s.SetHidden(true)
	r.s.startBackground(context.Background())
	r.s.bg.launching.Wait()
	if r.shown != 1 {
		t.Fatalf("shown %d times", r.shown)
	}
}
