package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/removal"
	"github.com/blaktron/agentry-cli/pkg/runner/owner"
)

// podmanServiceRecordFile is the record a run under a policy runtime on
// podman writes in its scratch directory for the podman API service of its
// own: the service's pid, the start time of that process, and its socket.
// The CLI writes it (pkg/runner/engine) and reads it back the same way in
// its own sweep; the shape is shared through the file, not a type.
const podmanServiceRecordFile = "podman-service.pid"

type podmanServiceRecord struct {
	PID    int    `json:"pid"`
	Start  uint64 `json:"start"`
	Socket string `json:"socket"`
}

// stopLeftPodmanService ends the podman API service the scratch directory
// dir of run runID names, before the directory and the record go: the
// process, only when it is still the one the record describes (the same
// start time, so a reused pid is never signalled) and is a podman service
// by its own command line, then the socket, only at the one path the run
// gives its service under the runner home (or the short path it falls
// back to under the system's temp directory), and the record, always. The
// record is a file in a directory on this computer, so what it names is
// held to what a run's service can be, not believed. Off Linux a start
// time cannot be read, so no process is touched there and the service,
// which exists only on Linux, is not left in question. Best effort:
// nothing here fails the sweep.
func stopLeftPodmanService(dir, home, runID string) {
	path := filepath.Join(dir, podmanServiceRecordFile)
	b, err := os.ReadFile(path) //nolint:gosec // a fixed name in a run directory
	if err != nil {
		return
	}
	var rec podmanServiceRecord
	if json.Unmarshal(b, &rec) != nil || rec.PID <= 0 {
		return
	}
	if start, ok := owner.StartTime(rec.PID); ok && start == rec.Start && isPodmanService(rec.PID) {
		endProcess(rec.PID)
	}
	if serviceSocketOfRun(rec.Socket, home, runID) {
		_ = removal.Remove(rec.Socket, false)
	}
	_ = removal.Remove(path, false)
}

// serviceSocketOfRun says whether path is where the run's podman service
// puts its socket: <runner home>/s/<run id>-podman.sock, or, when that is
// too long to bind, podman.sock in a directory named agentry-pm-* under
// the system's temp directory. Anything else is not removed.
func serviceSocketOfRun(path, home, runID string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	if home != "" && path == filepath.Join(home, "s", runID+"-podman.sock") {
		return true
	}
	dir, base := filepath.Split(path)
	dir = filepath.Clean(dir)
	return base == "podman.sock" && filepath.Dir(dir) == filepath.Clean(os.TempDir()) && strings.HasPrefix(filepath.Base(dir), "agentry-pm-")
}

// isPodmanService reads the process's own command line: a podman binary
// running `system service`. Where the command line cannot be read, no.
func isPodmanService(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline")) //nolint:gosec // the pid's own entry
	if err != nil {
		return false
	}
	argv := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	podman, system, service := false, false, false
	for _, a := range argv {
		switch {
		case filepath.Base(a) == "podman":
			podman = true
		case a == "system":
			system = true
		case a == "service":
			service = true
		}
	}
	return podman && system && service
}

// processGone bounds how long endProcess waits for the gentle signal.
const processGone = 5 * time.Second

// endProcess asks a process to end and kills it when it has not within
// processGone.
func endProcess(pid int) {
	if !terminateProcess(pid) {
		return
	}
	deadline := time.Now().Add(processGone)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
