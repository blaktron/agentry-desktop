package run

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The picker against this computer's engines, on a Linux machine where
// Docker Engine and rootless podman both answer: both are listed, podman is
// chosen and remembered in a desktop.yaml of the test's own, a later launch
// adopts it although the docker CLI's context answers first in the order,
// and the agentry command run with the app's environment, as a run on this
// computer is, drives podman. Automatic goes back to the order.
func TestLivePickerOnThisComputer(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_ENGINE") != "1" {
		t.Skip("set AGENTRY_LIVE_ENGINE=1 to run the picker against this computer's engines")
	}
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	t.Setenv(configdir.EnvVar, t.TempDir())
	for _, v := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", dockerx.EnvEngine} {
		t.Setenv(v, "")
		_ = os.Unsetenv(v)
	}
	t.Cleanup(func() {
		adoptMu.Lock()
		clearAdoption()
		adoptMu.Unlock()
	})

	first := DetectEngine(t.Context(), true)
	t.Logf("automatic: %s", first.Summary())
	list := ListEngines(t.Context())
	for _, c := range list.Candidates {
		t.Logf("listed %s: %s (current %v)", c.Choice, c.Engine.Summary(), c.Current)
	}
	t.Logf("tried: %q", list.Tried)
	var podman bool
	for _, c := range list.Candidates {
		podman = podman || c.Choice == ChoicePodman
	}
	if len(list.Candidates) < 2 || !podman {
		t.Fatalf("want Docker Engine and podman listed, got %+v", list.Candidates)
	}
	if a := AdoptedEngine(); a == nil || a.Endpoint != first.Endpoint {
		t.Fatalf("the listing changed the adoption: %+v", a)
	}

	chosen, err := SetEngineChoice(t.Context(), ChoicePodman)
	if err != nil {
		t.Fatal(err)
	}
	st, err := settings.Load()
	if err != nil || st.EngineChoice != ChoicePodman {
		t.Fatalf("desktop.yaml holds %q (%v)", st.EngineChoice, err)
	}
	t.Logf("chosen: %s", chosen.Summary())

	// A later launch: nothing adopted, and the choice read back from the file.
	adoptMu.Lock()
	clearAdoption()
	adoptMu.Unlock()
	launch := DetectEngine(t.Context(), true)
	if !launch.Found || launch.Kind != dockerx.KindPodman || !launch.Chosen {
		t.Fatalf("at launch: %+v", launch)
	}
	// AGENTRY_LIVE_CLI names the agentry binary when the one on PATH
	// predates the engine line.
	bin := os.Getenv("AGENTRY_LIVE_CLI")
	if bin == "" {
		bin, _ = exec.LookPath("agentry")
	}
	if bin != "" {
		out, _ := exec.CommandContext(t.Context(), bin, "doctor").CombinedOutput() //nolint:gosec // the CLI on PATH, fixed args
		var line string
		for _, l := range strings.Split(string(out), "\n") {
			if f := strings.Fields(l); len(f) > 2 && f[1] == "docker" {
				line = strings.TrimSpace(l)
				break
			}
		}
		t.Logf("agentry doctor with the app's environment: %s", line)
		if !strings.Contains(line, "podman") {
			t.Errorf("the CLI does not drive podman with the app's environment: %q", line)
		}
	}

	back, err := SetEngineChoice(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if back.Chosen || back.Endpoint != first.Endpoint {
		t.Fatalf("automatic: %+v, want %s", back, first.Summary())
	}
	if st, _ := settings.Load(); st.EngineChoice != "" {
		t.Fatalf("desktop.yaml still holds %q", st.EngineChoice)
	}
}
