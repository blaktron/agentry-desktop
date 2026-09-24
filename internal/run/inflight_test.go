package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/owner"
)

// deadPID starts a process that exits at once, waits for it, and returns
// its pid (with the start time it had, where the platform has one): an
// owner that is provably gone.
func deadPID(t *testing.T) (int, uint64) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // this test binary
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	start, _ := owner.StartTime(pid)
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	return pid, start
}

func writeRecord(t *testing.T, dir string, rec inFlight) {
	t.Helper()
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rec.RunID+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// A run in flight is recorded with its id and this process, and the record
// goes when the run ends.
func TestRecordInFlightThenForget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), InFlightDirName)
	if err := RecordInFlight(dir, "ab12cd34"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ab12cd34.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec inFlight
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.RunID != "ab12cd34" || rec.PID != os.Getpid() {
		t.Fatalf("record = %+v", rec)
	}
	if start, ok := owner.StartTime(os.Getpid()); ok && rec.Start != start {
		t.Fatalf("start = %d, want %d", rec.Start, start)
	}
	if err := ForgetInFlight(dir, "ab12cd34"); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "ab12cd34.json")) {
		t.Fatal("record outlived the run")
	}
	if err := ForgetInFlight(dir, "ab12cd34"); err != nil {
		t.Fatalf("forgetting twice: %v", err)
	}
	// A run id that is not the engine's never becomes a file name.
	if err := RecordInFlight(dir, "../x"); err == nil {
		t.Fatal("recorded a bad run id")
	}
}

// At startup a record whose owner is gone is swept by its run id and
// removed; a live owner's record and this process's own are left alone,
// and nothing but the dead run's label is ever asked about — so a CLI
// run's containers, under their own run id, are never touched.
func TestSweepOrphanedSweepsOnlyADeadOwnersRun(t *testing.T) {
	fake := &dockerx.Fake{
		ByLabel: map[string][]string{
			"agentry.run=dead0001": {"agentry-dead0001-agent", "agentry-dead0001-gateway"},
			"agentry.run=a11e0002": {"agentry-a11e0002-agent"},
			"agentry.run=c11c0003": {"agentry-c11c0003-agent"}, // a CLI run: no record
		},
	}
	prev := newDockerClient
	newDockerClient = func() dockerx.Client { return fake }
	t.Cleanup(func() { newDockerClient = prev })

	dir := filepath.Join(t.TempDir(), InFlightDirName)
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: "dead0001", PID: pid, Start: start})
	parent := os.Getppid()
	pstart, _ := owner.StartTime(parent)
	writeRecord(t, dir, inFlight{RunID: "a11e0002", PID: parent, Start: pstart})
	if err := RecordInFlight(dir, "5e1f0004"); err != nil {
		t.Fatal(err)
	}
	// Malformed records are never swept: a name that disagrees with its
	// id, and an id that is not the engine's.
	writeRecord(t, dir, inFlight{RunID: "bad", PID: pid})
	if err := os.WriteFile(filepath.Join(dir, "abcdef01.json"), []byte(`{"run_id":"abcdef02","pid":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	swept, err := SweepOrphaned(context.Background(), dir, Sweep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(swept, ",") != "dead0001" {
		t.Fatalf("swept %v, want [dead0001]", swept)
	}
	for _, c := range fake.Calls {
		if strings.Contains(c.Arg, "a11e0002") || strings.Contains(c.Arg, "c11c0003") || strings.Contains(c.Arg, "5e1f0004") {
			t.Fatalf("touched a run it does not own: %s %s", c.Op, c.Arg)
		}
	}
	var removed []string
	for _, c := range fake.Calls {
		if c.Op == "RemoveForce" {
			removed = append(removed, c.Arg)
		}
	}
	if strings.Join(removed, ",") != "agentry-dead0001-agent,agentry-dead0001-gateway" {
		t.Fatalf("removed %v", removed)
	}
	if exists(filepath.Join(dir, "dead0001.json")) {
		t.Fatal("the swept run's record was kept")
	}
	for _, keep := range []string{"a11e0002.json", "5e1f0004.json", "bad.json", "abcdef01.json"} {
		if !exists(filepath.Join(dir, keep)) {
			t.Fatalf("%s was removed", keep)
		}
	}
}

// A sweep that fails (Docker not running yet at startup) keeps the record,
// so the next start tries again.
func TestSweepOrphanedKeepsTheRecordWhenTheSweepFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), InFlightDirName)
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: "dead0001", PID: pid, Start: start})
	swept, err := SweepOrphaned(context.Background(), dir, func(context.Context, string) error {
		return errors.New("docker is not running")
	})
	if err == nil || len(swept) != 0 {
		t.Fatalf("swept %v, err %v", swept, err)
	}
	if !exists(filepath.Join(dir, "dead0001.json")) {
		t.Fatal("the record went with the sweep failed")
	}
}

// No directory yet (no run ever started) is nothing to do.
func TestSweepOrphanedWithNoRecords(t *testing.T) {
	swept, err := SweepOrphaned(context.Background(), filepath.Join(t.TempDir(), "absent"), func(context.Context, string) error {
		t.Fatal("swept with no records")
		return nil
	})
	if err != nil || len(swept) != 0 {
		t.Fatalf("swept %v, err %v", swept, err)
	}
}
