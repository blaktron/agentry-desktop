package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// Each engine is named by its virtual machine, and only the ones the app can
// size say so.
func TestTheEnginesVirtualMachineIsNamed(t *testing.T) {
	for _, c := range []struct {
		goos      string
		e         Engine
		kind      string
		resizable bool
	}{
		{"darwin", Engine{Found: true, How: managedHow, Endpoint: "unix:///Users/me/.agentry-vm/colima/default/docker.sock"}, VMManaged, true},
		{"darwin", Engine{Found: true, How: howSocketOf + "Rancher Desktop", Endpoint: "unix:///Users/me/.rd/docker.sock"}, VMRancher, true},
		{"linux", Engine{Found: true, How: howCurrentContext, Endpoint: "unix:///home/me/.rd/docker.sock"}, VMRancher, true},
		{"windows", Engine{Found: true, How: howCurrentContext, Endpoint: "npipe:////./pipe/docker_engine", Kind: dockerx.KindDocker}, VMOther, false},
		{"darwin", Engine{Found: true, How: howSocketOf + "the podman machine", Kind: dockerx.KindPodman, Endpoint: "unix:///var/folders/x/podman.sock"}, VMPodmanMachine, true},
		{"darwin", Engine{Found: true, How: howCurrentContext, Endpoint: "unix:///Users/me/.docker/run/docker.sock"}, VMDockerDesktop, false},
		{"darwin", Engine{Found: true, How: howSocketOf + "OrbStack", Endpoint: "unix:///Users/me/.orbstack/run/docker.sock"}, VMOther, false},
		{"linux", Engine{Found: true, How: howCurrentContext, Endpoint: "unix:///var/run/docker.sock"}, VMNone, false},
		{"linux", Engine{Found: true, How: howPodman, Kind: dockerx.KindPodman, Endpoint: "unix:///run/user/1000/podman/podman.sock"}, VMNone, false},
	} {
		v := vmOf(c.goos, c.e)
		if v.Kind != c.kind || v.Resizable != c.resizable {
			t.Errorf("%s %s: %+v, want %q resizable=%v", c.goos, c.e.Endpoint, v, c.kind, c.resizable)
		}
	}
}

// fakeCommand puts a script named name on PATH that records its arguments,
// one call a line, in the file it returns.
func fakeCommand(t *testing.T, name string) (path, calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	path = filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho \"$*\" >> "+calls+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path, calls
}

func answering(t *testing.T, script ...bool) {
	t.Helper()
	old, oldPoll, oldGo := engineAnswers, enginePoll, rancherGoWait
	i := 0
	engineAnswers = func(context.Context) (bool, string) {
		ok := script[min(i, len(script)-1)]
		i++
		return ok, "docker 29.5.3"
	}
	enginePoll, rancherGoWait = time.Millisecond, time.Second
	t.Cleanup(func() { engineAnswers, enginePoll, rancherGoWait = old, oldPoll, oldGo })
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// Rancher Desktop is given its memory by rdctl, and the resize waits for
// its engine to go and to answer again.
func TestRancherDesktopIsResizedByRdctl(t *testing.T) {
	bin, calls := fakeCommand(t, "rdctl")
	old := lookPath
	lookPath = func(file string) (string, error) {
		if file == "rdctl" {
			return bin, nil
		}
		return old(file)
	}
	t.Cleanup(func() { lookPath = old })
	answering(t, true, false, false, true)
	var log strings.Builder
	if err := ResizeVM(context.Background(), VM{Kind: VMRancher, Name: "Rancher Desktop", Resizable: true}, 8, 3, &log); err != nil {
		t.Fatal(err)
	}
	if got := lines(t, calls); len(got) != 1 || got[0] != "set --virtual-machine.memory-in-gb=8" {
		t.Fatalf("rdctl was run with %q", got)
	}
}

// A podman machine is stopped, given its memory in MiB and started again.
func TestAPodmanMachineIsStoppedSetAndStarted(t *testing.T) {
	bin, calls := fakeCommand(t, "podman")
	t.Setenv(dockerx.EnvPodman, bin)
	answering(t, true)
	if err := ResizeVM(context.Background(), VM{Kind: VMPodmanMachine, Name: "the podman machine", Resizable: true}, 6, 3, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"machine stop", "machine set --memory 6144", "machine start"}
	if got := lines(t, calls); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("podman was run with %q, want %q", got, want)
	}
}

// A machine the app does not size is refused, and so is no size.
func TestOnlyTheEnginesTheAppSizesAreResized(t *testing.T) {
	if err := ResizeVM(context.Background(), VM{Kind: VMDockerDesktop, Name: "Docker Desktop"}, 8, 3, &strings.Builder{}); err == nil {
		t.Error("Docker Desktop was resized")
	}
	if err := ResizeVM(context.Background(), VM{Kind: VMManaged, Name: "Agentry's engine", Resizable: true}, 0, 3, &strings.Builder{}); err == nil {
		t.Error("a size of 0 was accepted")
	}
}

// A resized engine that never answers again is said so.
func TestAResizedEngineThatNeverAnswersIsSaid(t *testing.T) {
	answering(t, false)
	old := resizeWait
	resizeWait = 10 * time.Millisecond
	t.Cleanup(func() { resizeWait = old })
	if err := waitForEngine(context.Background(), &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "did not answer again") {
		t.Fatalf("waitForEngine: %v", err)
	}
}
