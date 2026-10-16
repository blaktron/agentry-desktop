package run

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-desktop/internal/managedengine"
)

// installedManagedEngine is a managed engine under a temporary directory
// with its three binaries and record in place, as an install leaves them.
func installedManagedEngine(t *testing.T) *managedengine.Engine {
	t.Helper()
	e := &managedengine.Engine{Dir: filepath.Join(t.TempDir(), "engine")}
	for _, b := range []string{"colima", "limactl", "docker"} {
		p := filepath.Join(e.Dir, "bin", b)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { //nolint:gosec // a test stand-in
			t.Fatal(err)
		}
	}
	rec := `{"colima":"` + managedengine.ColimaVersion + `","lima":"` + managedengine.LimaVersion + `","docker":"` + managedengine.DockerVersion + `","arch":"` + runtime.GOARCH + `"}`
	if err := os.WriteFile(filepath.Join(e.Dir, "installed.json"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	return e
}

// The card's state off Linux: an engine answering wins; the app's own
// engine installed and silent is stopped; where the build can install one
// it offers; otherwise the hand-off.
func TestTheManagedEngineStates(t *testing.T) {
	for _, tc := range []struct {
		ok   bool
		m    Managed
		want string
	}{
		{true, Managed{Supported: true, Installed: true}, StateReady},
		{true, Managed{}, StateReady},
		{false, Managed{Supported: true, Installed: true}, StateManagedStopped},
		{false, Managed{Supported: true}, StateManagedOffer},
		{false, Managed{}, StateManual},
	} {
		if got := managedState(tc.ok, tc.m); got != tc.want {
			t.Errorf("managedState(%v, %+v) = %s, want %s", tc.ok, tc.m, got, tc.want)
		}
	}
}

// Detection asks the app's own engine first: installed and answering, it
// is adopted by its socket and its own docker command and named as the
// app's; installed and silent, it is said and the order goes on; not
// installed, nothing is said of it.
func TestDetectionAsksTheManagedEngineFirst(t *testing.T) {
	e := installedManagedEngine(t)
	// A docker CLI that answers only at the managed engine's socket, as
	// the engine's own docker command would.
	f := engineFixture{docker: &contextDocker{answersAt: e.Endpoint()}, answering: map[string]bool{e.Socket(): true}}
	withEngines(t, f)
	t.Setenv(dockerx.EnvDocker, "")
	t.Setenv("DOCKER_CONFIG", "")
	managed := e
	saved := managedEngine
	managedEngine = func() *managedengine.Engine { return managed }
	t.Cleanup(func() { managedEngine = saved })

	got := DetectEngine(context.Background(), true)
	if !got.Found || got.How != managedHow {
		t.Fatalf("found=%v how=%q err=%q tried=%q", got.Found, got.How, got.Err, got.Tried)
	}
	if os.Getenv("DOCKER_HOST") != e.Endpoint() || os.Getenv(dockerx.EnvDocker) != filepath.Join(e.Dir, "bin", "docker") || os.Getenv("DOCKER_CONFIG") != e.DockerConfigDir() {
		t.Errorf("adopted DOCKER_HOST=%q AGENTRY_DOCKER=%q DOCKER_CONFIG=%q", os.Getenv("DOCKER_HOST"), os.Getenv(dockerx.EnvDocker), os.Getenv("DOCKER_CONFIG"))
	}
	if !strings.Contains(got.Summary(), "found as Agentry's engine (Colima)") {
		t.Errorf("summary = %q", got.Summary())
	}
	// A page's read keeps it.
	if again := DetectEngine(context.Background(), false); !again.Found || again.How != managedHow {
		t.Errorf("a later read: found=%v how=%q", again.Found, again.How)
	}
	// Silent: said first, and the order goes on to the docker CLI (which
	// answers nowhere else here) and podman, finding nothing.
	f.answering = map[string]bool{}
	withEngines(t, f)
	managedEngine = func() *managedengine.Engine { return managed }
	got = DetectEngine(context.Background(), true)
	if got.Found {
		t.Fatalf("found %q with nothing answering", got.How)
	}
	if len(got.Tried) == 0 || !strings.HasPrefix(got.Tried[0], managedHow+": installed under "+e.Dir+" and not running") {
		t.Errorf("tried = %q", got.Tried)
	}
	if os.Getenv("DOCKER_HOST") != "" || os.Getenv(dockerx.EnvDocker) != "" {
		t.Errorf("an adoption was left: DOCKER_HOST=%q AGENTRY_DOCKER=%q", os.Getenv("DOCKER_HOST"), os.Getenv(dockerx.EnvDocker))
	}
	// Not installed: not a word of it.
	managed = &managedengine.Engine{Dir: t.TempDir()}
	got = DetectEngine(context.Background(), true)
	for _, tr := range got.Tried {
		if strings.Contains(tr, managedHow) {
			t.Errorf("the app's engine was named although not installed: %q", tr)
		}
	}
	// No managed engine at all (another platform): the same.
	managed = nil
	if got = DetectEngine(context.Background(), true); got.Found {
		t.Errorf("found %q", got.How)
	}
}

// The managed story for the card: supported with the facts, installed and
// running by the socket's answer, current by the record.
func TestTheManagedStory(t *testing.T) {
	e := installedManagedEngine(t)
	withEngines(t, engineFixture{answering: map[string]bool{e.Socket(): true}})
	saved := managedEngine
	managedEngine = func() *managedengine.Engine { return e }
	t.Cleanup(func() { managedEngine = saved })
	m := managedStory(context.Background())
	if !m.Supported || !m.Installed || !m.Current || !m.Running || m.Dir != e.Dir || m.Socket != e.Socket() {
		t.Errorf("story = %+v", m)
	}
	if !strings.Contains(m.Versions, managedengine.ColimaVersion) || !strings.Contains(m.Licences, "MIT") {
		t.Errorf("versions %q licences %q", m.Versions, m.Licences)
	}
	managedEngine = func() *managedengine.Engine { return nil }
	if m := managedStory(context.Background()); m.Supported {
		t.Errorf("unsupported build has a story: %+v", m)
	}
}
