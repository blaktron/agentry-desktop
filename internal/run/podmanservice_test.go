package run

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/owner"
)

// standInEnv, when set, makes this test binary stand in for a long-running
// process: it says it is ready on standard output, waits for SIGTERM and
// exits. A test reaches it through a link named for the program it plays,
// since the sweep reads a process's own command line.
const standInEnv = "AGENTRY_DESKTOP_TEST_STAND_IN"

func TestMain(m *testing.M) {
	if os.Getenv(standInEnv) != "" {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		_, _ = os.Stdout.WriteString("ready\n")
		<-stop
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// The sweep of a run's files ends the podman API service the run's record
// names before the scratch directory and the record go, through the CLI's
// own sweep, which holds the record to the service's shape: a binary named
// podman running `system service` on the run's own socket, with the
// recorded start time. The stand-in is this test binary behind a link
// named podman, given that command line.
func TestSweepFilesEndsTheServiceBeforeTheDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the service exists only on Linux")
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
	podman := filepath.Join(t.TempDir(), "podman")
	if err := os.Symlink(os.Args[0], podman); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(podman, "--runtime", "runsc", "system", "service", "--time=0", "unix://"+sock) //nolint:gosec // this test binary
	cmd.Env = append(os.Environ(), standInEnv+"=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	// Start returns once the exec has begun, which can be before the kernel
	// has given the process its new command line; the sweep would then read
	// an empty one and rightly leave the process alone. The stand-in's own
	// line says the exec is complete.
	if _, err := bufio.NewReader(out).ReadString('\n'); err != nil {
		t.Fatalf("the stand-in did not start: %v", err)
	}
	start, ok := owner.StartTime(cmd.Process.Pid)
	if !ok {
		t.Skip("this process cannot read start times")
	}
	// The record as the CLI writes it: pid, start time, socket.
	b, _ := json.Marshal(map[string]any{"pid": cmd.Process.Pid, "start": start, "socket": sock})
	if err := os.WriteFile(filepath.Join(scratch, "podman-service.pid"), b, 0o600); err != nil {
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
