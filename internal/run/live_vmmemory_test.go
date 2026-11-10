package run

// A podman machine resized live, as Runs at once resizes it: stopped, given
// its memory and started again, then read back from podman. It stops and
// restarts the person's default podman machine, so it runs only when asked:
//
//	AGENTRY_LIVE_PODMAN_MACHINE=1 \
//	AGENTRY_LIVE_PODMAN_MACHINE_GIB=<the memory to give it, default 6> \
//	go test ./internal/run -run TestLivePodmanMachineResize -v -timeout 20m
//
// The machine is left with the memory given.

import (
	"cmp"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

func TestLivePodmanMachineResize(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_PODMAN_MACHINE") != "1" {
		t.Skip("stops and restarts the default podman machine: set AGENTRY_LIVE_PODMAN_MACHINE=1")
	}
	gib, err := strconv.Atoi(cmp.Or(os.Getenv("AGENTRY_LIVE_PODMAN_MACHINE_GIB"), "6"))
	if err != nil || gib < 2 {
		t.Fatalf("AGENTRY_LIVE_PODMAN_MACHINE_GIB: %q", os.Getenv("AGENTRY_LIVE_PODMAN_MACHINE_GIB"))
	}
	bin := engineBin(dockerx.KindPodman)
	podman := func(args ...string) string {
		out, err := exec.Command(bin, args...).CombinedOutput() //nolint:gosec // the engine's own binary
		if err != nil {
			t.Fatalf("podman %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Logf("before: %s MiB", podman("machine", "inspect", "--format", "{{.Resources.Memory}}"))
	old := engineAnswers
	engineAnswers = func(ctx context.Context) (bool, string) {
		out, err := exec.CommandContext(ctx, bin, "info", "--format", "{{.Host.MemTotal}}").Output() //nolint:gosec // the engine's own binary
		return err == nil, "podman answers: MemTotal " + strings.TrimSpace(string(out))
	}
	t.Cleanup(func() { engineAnswers = old })
	var log strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	start := time.Now()
	if err := ResizeVM(ctx, VM{Kind: VMPodmanMachine, Name: "the podman machine", Resizable: true}, gib, 3, &log); err != nil {
		t.Fatalf("resize: %v\n%s", err, log.String())
	}
	t.Logf("resized in %s:\n%s", time.Since(start).Round(time.Second), log.String())
	if got := podman("machine", "inspect", "--format", "{{.Resources.Memory}}"); got != strconv.Itoa(gib*1024) {
		t.Errorf("the machine has %s MiB, want %d", got, gib*1024)
	}
	mem, _ := strconv.ParseUint(podman("info", "--format", "{{.Host.MemTotal}}"), 10, 64)
	t.Logf("podman info: %d bytes", mem)
	if g := (mem + 1<<29) >> 30; int(g) != gib && int(g) != gib-1 {
		t.Errorf("podman reports %d GiB, want about %d", g, gib)
	}
}
