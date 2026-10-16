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
	standIn := func() *exec.Cmd {
		cmd := exec.Command("sleep", "300")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
		return cmd
	}
	dir := t.TempDir()
	write := func(rec podmanServiceRecord) {
		b, _ := json.Marshal(rec)
		if err := os.WriteFile(filepath.Join(dir, podmanServiceRecordFile), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sock := filepath.Join(t.TempDir(), "podman.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// A record whose start time is another process's: the process is left
	// alone, the dead run's socket and record go.
	reused := standIn()
	start, ok := owner.StartTime(reused.Process.Pid)
	if !ok {
		t.Skip("this process cannot read start times")
	}
	write(podmanServiceRecord{PID: reused.Process.Pid, Start: start + 7, Socket: sock})
	stopLeftPodmanService(dir)
	if !processExists(reused.Process.Pid) {
		t.Fatal("a reused pid was ended")
	}
	for _, p := range []string{sock, filepath.Join(dir, podmanServiceRecordFile)} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was kept: %v", p, err)
		}
	}
	// The recorded process: ended, with its socket and record.
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	svc := standIn()
	start, _ = owner.StartTime(svc.Process.Pid)
	write(podmanServiceRecord{PID: svc.Process.Pid, Start: start, Socket: sock})
	done := make(chan struct{})
	go func() { _, _ = svc.Process.Wait(); close(done) }()
	stopLeftPodmanService(dir)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the recorded service was not ended")
	}
	for _, p := range []string{sock, filepath.Join(dir, podmanServiceRecordFile)} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was kept: %v", p, err)
		}
	}
	// A directory with no record, and a live record of this test's own
	// process family, are left as they are.
	stopLeftPodmanService(t.TempDir())
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
	cmd := exec.Command("sleep", "300")
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
