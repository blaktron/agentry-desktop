package run

import (
	"os"
	"runtime"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// The detection order against this computer's own engines, run only when
// AGENTRY_LIVE_ENGINE=1 and reported with -v: what the docker CLI's context
// finds, then, with a docker CLI that cannot answer, what the known
// sockets and podman natively find, and Re-detect switching back. It
// adopts engines in this process's environment and clears them after.
func TestLiveDetectionOnThisComputer(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_ENGINE") != "1" {
		t.Skip("set AGENTRY_LIVE_ENGINE=1 to run the detection against this computer's engines")
	}
	if runtime.GOOS != "linux" {
		t.Skip("the podman step is Linux's")
	}
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	t.Setenv(dockerx.EnvEngine, "")
	for _, v := range []string{"DOCKER_HOST", "DOCKER_CONTEXT", dockerx.EnvEngine} {
		_ = os.Unsetenv(v)
	}
	t.Cleanup(func() {
		adoptMu.Lock()
		clearAdoption()
		adoptMu.Unlock()
	})

	first := DetectEngine(t.Context(), true)
	t.Logf("docker CLI's context: found=%v %s; tried=%q warnings=%q", first.Found, first.Summary(), first.Tried, first.Warnings)

	// A docker CLI that answers nothing: the known sockets are asked (Docker
	// Engine's answers, but the CLI cannot drive it), then podman natively.
	t.Setenv(dockerx.EnvDocker, "/bin/false")
	second := DetectEngine(t.Context(), true)
	t.Logf("docker CLI unusable: found=%v %s; tried=%q warnings=%q; AGENTRY_ENGINE=%q DOCKER_HOST=%q",
		second.Found, second.Summary(), second.Tried, second.Warnings, os.Getenv(dockerx.EnvEngine), os.Getenv("DOCKER_HOST"))
	if second.Found && second.Kind == dockerx.KindPodman {
		// The adopted engine is what every client built now drives.
		if e, err := dockerx.DescribeEngine(t.Context(), newDockerClient()); err != nil || e.Kind != dockerx.KindPodman {
			t.Fatalf("the adopted podman is not what a new client drives: %+v, %v", e, err)
		}
		// A page's read keeps podman while it answers.
		if kept := DetectEngine(t.Context(), false); kept.Kind != dockerx.KindPodman {
			t.Fatalf("a read switched away from podman: %+v", kept)
		}
	}

	// The docker CLI back: Re-detect switches to it.
	_ = os.Unsetenv(dockerx.EnvDocker)
	third := DetectEngine(t.Context(), true)
	t.Logf("re-detect with docker back: found=%v %s; AGENTRY_ENGINE=%q", third.Found, third.Summary(), os.Getenv(dockerx.EnvEngine))
	if first.Found && (!third.Found || third.Kind != first.Kind || third.How != first.How) {
		t.Fatalf("re-detect did not come back to %s: %+v", first.Summary(), third)
	}
}
