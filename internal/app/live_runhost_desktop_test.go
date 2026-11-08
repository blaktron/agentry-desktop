package app

// A run sent to a runhost that uses Blender on this computer, live: the app
// binds the run here, sends it to a runhost already paired with this runner
// home, and answers its calls over the link session it keeps open. Once the
// first call has been answered the app is closed (its context ends), and the
// run's next call is refused desktop-unreachable there while the run goes
// on. Skipped unless AGENTRY_LIVE_RUNHOST_DESKTOP names the runhost:
//
//	AGENTRY_LIVE_RUNHOST_DESKTOP=<a runhost paired in this runner home> \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<the scratch runner home, its policy.yaml naming the model> \
//	AGENTRY_LIVE_DESKTOP_OUT=<a scratch folder> \
//	AGENTRY_LIVE_DESKTOP_REGISTRY=<the registry, default https://dev.secureagentry.net> \
//	AGENTRY_LIVE_DESKTOP_REF=<a release that declares blender, required> \
//	go test -run TestLiveDesktopLinkOnARunhost -v -timeout 60m ./internal/app
//
// The model must make a second desktop call well after the first, so the
// app's close lands between them. The job is left on the runhost: collect it
// with `agentry runhost attach <runhost> <job>` and read its receipts.

import (
	"cmp"
	"context"
	"os"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/install"
)

func TestLiveDesktopLinkOnARunhost(t *testing.T) {
	name := os.Getenv("AGENTRY_LIVE_RUNHOST_DESKTOP")
	if name == "" {
		t.Skip("runs on a runhost with Blender on this computer: set AGENTRY_LIVE_RUNHOST_DESKTOP (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" || os.Getenv("AGENTRY_LIVE_DESKTOP_OUT") == "" {
		t.Fatalf("set %s, %s and AGENTRY_LIVE_DESKTOP_OUT to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_REGISTRY"), "https://dev.secureagentry.net")
	ref := os.Getenv("AGENTRY_LIVE_DESKTOP_REF")
	if ref == "" {
		t.Fatal("set AGENTRY_LIVE_DESKTOP_REF to a release that declares blender")
	}
	out := os.Getenv("AGENTRY_LIVE_DESKTOP_OUT")

	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) == 1 {
			if s, ok := data[0].(string); ok {
				t.Logf("[%s] %s", name, s)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })

	ctx, closeApp := context.WithCancel(context.Background())
	defer closeApp()
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, message string) (bool, error) {
		t.Logf("dialog %q:\n%s", title, message)
		return true, nil
	}}
	r := &RunAPI{app: app}
	y := newYourComputerAPI(app, r)
	defer y.shutdown()
	installs := &InstallAPI{app: app, engine: install.NewEngine()}
	if _, err := installs.InstallRunner(reg, ref); err != nil {
		t.Fatalf("InstallRunner: %v", err)
	}

	// The app is closed once the run's first call has been answered here.
	start := time.Now()
	answered := make(chan struct{})
	go func() {
		for ctx.Err() == nil {
			for _, run := range y.State().Recent {
				if run.Started.Before(start) {
					continue
				}
				for _, s := range run.Servers {
					n := 0
					for _, c := range s.Calls {
						n += c
					}
					if n > 0 {
						t.Logf("the run's first call was answered here (%s: %v)", s.Application, s.Calls)
						close(answered)
						return
					}
				}
			}
			time.Sleep(time.Second)
		}
	}()
	go func() {
		select {
		case <-answered:
			time.Sleep(3 * time.Second)
			t.Log("closing the app")
			closeApp()
		case <-ctx.Done():
		}
	}()

	app.markPicked(out)
	res, err := r.RunOnRunhost(name, reg, ref, "Check the Blender scene as your instructions say.", nil, out, 0, -1, false, nil, false)
	select {
	case <-answered:
	default:
		t.Fatalf("no call reached this computer before the run ended: %+v %v", res, err)
	}
	if err == nil {
		t.Fatalf("the run finished with the app closed: %+v", res)
	}
	t.Logf("the app left the run, which goes on there: %v", err)
}
