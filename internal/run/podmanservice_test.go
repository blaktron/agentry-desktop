package run

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/owner"
)

// The sweep of a run's files ends the podman API service the run's record
// names, as the CLI's own sweep does: the process only when its start time
// is the recorded one (a reused pid is left alone), the socket and the
// record always.
func TestTheSweepStopsADeadRunsPodmanService(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a process's start time is read from /proc; the service exists only on Linux")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep to stand in for the service")
	}
	// The stand-in is a shell script named podman that runs `system
	// service`, as the sweep reads the process's own command line; a plain
	// sleep is not a podman service and is never signalled.
	script := filepath.Join(t.TempDir(), "podman")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 300\n"), 0o700); err != nil { //nolint:gosec // a test's own stand-in
		t.Fatal(err)
	}
	standIn := func(argv ...string) *exec.Cmd {
		cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // a test's own stand-in
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
		return cmd
	}
	home := t.TempDir()
	const runID = "0123456789abcdef"
	dir := t.TempDir()
	write := func(rec podmanServiceRecord) {
		b, _ := json.Marshal(rec)
		if err := os.WriteFile(filepath.Join(dir, podmanServiceRecordFile), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "s"), 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(home, "s", runID+"-podman.sock")
	touch := func(p string) {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gone := func(paths ...string) {
		t.Helper()
		for _, p := range paths {
			if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s was kept: %v", p, err)
			}
		}
	}
	kept := func(p string) {
		t.Helper()
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed: %v", p, err)
		}
	}
	record := filepath.Join(dir, podmanServiceRecordFile)
	// A record whose start time is another process's: the process is left
	// alone, the dead run's socket and record go.
	touch(sock)
	reused := standIn(script, "system", "service", "--time=0")
	start, ok := owner.StartTime(reused.Process.Pid)
	if !ok {
		t.Skip("this process cannot read start times")
	}
	write(podmanServiceRecord{PID: reused.Process.Pid, Start: start + 7, Socket: sock})
	stopLeftPodmanService(dir, home, runID)
	if !processExists(reused.Process.Pid) {
		t.Fatal("a reused pid was ended")
	}
	gone(sock, record)
	// A record naming a process of the right start time that is not a
	// podman service: left alone; the record still goes.
	other := standIn("sleep", "300")
	start, _ = owner.StartTime(other.Process.Pid)
	write(podmanServiceRecord{PID: other.Process.Pid, Start: start, Socket: sock})
	stopLeftPodmanService(dir, home, runID)
	if !processExists(other.Process.Pid) {
		t.Fatal("a process that is not a podman service was ended")
	}
	gone(record)
	// A record naming a socket anywhere but the run's own path: that file
	// stays.
	elsewhere := filepath.Join(t.TempDir(), "podman.sock")
	touch(elsewhere)
	write(podmanServiceRecord{PID: reused.Process.Pid, Start: start + 7, Socket: elsewhere})
	stopLeftPodmanService(dir, home, runID)
	kept(elsewhere)
	gone(record)
	// The recorded service: ended, with its socket and record.
	touch(sock)
	svc := standIn(script, "system", "service", "--time=0", "unix://"+sock)
	start, _ = owner.StartTime(svc.Process.Pid)
	write(podmanServiceRecord{PID: svc.Process.Pid, Start: start, Socket: sock})
	done := make(chan struct{})
	go func() { _, _ = svc.Process.Wait(); close(done) }()
	stopLeftPodmanService(dir, home, runID)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the recorded service was not ended")
	}
	gone(sock, record)
	// A directory with no record is left as it is.
	stopLeftPodmanService(t.TempDir(), home, runID)
}

// serviceSocketOfRun admits the two paths a run's service can bind and
// nothing else.
func TestOnlyTheRunsOwnServiceSocketIsRemoved(t *testing.T) {
	home := t.TempDir()
	const runID = "0123456789abcdef"
	tmp := os.TempDir()
	for path, want := range map[string]bool{
		filepath.Join(home, "s", runID+"-podman.sock"):              true,
		filepath.Join(tmp, "agentry-pm-123456", "podman.sock"):      true,
		filepath.Join(home, "s", "fedcba9876543210-podman.sock"):    false,
		filepath.Join(home, "s", runID+".sock"):                     false,
		filepath.Join(tmp, "agentry-pm-123456", "other.sock"):       false,
		filepath.Join(tmp, "podman.sock"):                           false,
		filepath.Join(tmp, "agentry-pm-1", "deeper", "podman.sock"): false,
		filepath.Join(home, "agentry-pm-123456", "podman.sock"):     false,
		"relative/podman.sock":                                      false,
		"":                                                          false,
	} {
		if got := serviceSocketOfRun(path, home, runID); got != want {
			t.Errorf("%q: %v, want %v", path, got, want)
		}
	}
}

// SweepFiles reaches the service through the scratch directory it finds by
// the run id, before that directory goes.
func TestSweepFilesEndsTheServiceBeforeTheDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the service exists only on Linux")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep to stand in for the service")
	}
	home := t.TempDir()
	const runID = "0123456789abcdef"
	scratch := filepath.Join(home, "agents", "acme", "agent", "1.0.0", "runs", runID)
	if err := os.MkdirAll(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "s"), 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(home, "s", runID+"-podman.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "podman")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 300\n"), 0o700); err != nil { //nolint:gosec // a test's own stand-in
		t.Fatal(err)
	}
	cmd := exec.Command(script, "system", "service", "--time=0", "unix://"+sock) //nolint:gosec // a test's own stand-in
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	start, ok := owner.StartTime(cmd.Process.Pid)
	if !ok {
		t.Skip("this process cannot read start times")
	}
	b, _ := json.Marshal(podmanServiceRecord{PID: cmd.Process.Pid, Start: start, Socket: sock})
	if err := os.WriteFile(filepath.Join(scratch, podmanServiceRecordFile), b, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	if err := SweepFiles(home, runID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the service was not ended")
	}
	for _, p := range []string{scratch, sock} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was kept: %v", p, err)
		}
	}
}
