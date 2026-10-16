package run

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// stopLeftPodmanService ends the podman API service a run's scratch
// directory names, before the directory and the record go: the process,
// only when it is still the one the record describes (the same start
// time, so a reused pid is never signalled), then its socket and the
// record, always. Off Linux a start time cannot be read, so no process is
// touched there and the service, which exists only on Linux, is not left
// in question. Best effort: nothing here fails the sweep.
func stopLeftPodmanService(dir string) {
	path := filepath.Join(dir, podmanServiceRecordFile)
	b, err := os.ReadFile(path) //nolint:gosec // a fixed name in a run directory
	if err != nil {
		return
	}
	var rec podmanServiceRecord
	if json.Unmarshal(b, &rec) != nil || rec.PID <= 0 {
		return
	}
	if start, ok := owner.StartTime(rec.PID); ok && start == rec.Start {
		endProcess(rec.PID)
	}
	if rec.Socket != "" && filepath.IsAbs(rec.Socket) {
		_ = removal.Remove(rec.Socket, false)
	}
	_ = removal.Remove(path, false)
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
