package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/owner"
)

// The crash path. A killed or crashed app runs no hook, so a run it had in
// flight keeps its agent, gateway and proxy containers up. The app
// therefore records each run it starts — its run id and the process that
// owns it — in a small file under the runner home, removes the file when
// the run ends, and at the next start, and when it quits, sweeps the run of
// any record whose owner is provably gone.
//
// A record names one run id, minted by the process that wrote it and used
// by nobody else, so sweeping by that id can never reach the CLI's runs or
// another desktop's under a different id. Whether the run is over is judged
// by its owner: liveness errs toward "alive" — an unreadable process
// record, a pid the kernel reused — which leaves litter for a later start,
// never a run swept mid-turn that is live where this process can see it.
//
// That judgement is this machine's and this pid namespace's, and the runner
// home is taken to be one machine's. A home shared with another machine, or
// with a container of its own pid namespace, has desktops that cannot see
// each other's pids: the owner of a run live over there reads as gone here,
// and its record, and its scratch directory, would be swept from under it.
// A record does not name its machine because no identity tried for it was
// stable enough: a host name follows the network on macOS, and a record
// that stopped matching would leave a killed run's secrets file for good,
// which is the worse fault.
//
// Where the CLI's owner package can identify a process across pid reuse
// (Linux: pid plus start time), the record carries the start time and is
// judged by it; elsewhere by the pid alone, which is enough here because a
// record's run id is exclusive to its writer. The CLI's own sweep
// (engine.SweepAbandoned) labels containers instead and removes nothing off
// Linux, which is where most desktops run.

// InFlightDirName is the runner-home subdirectory holding the records.
const InFlightDirName = "desktop-runs"

// inFlight is one record.
type inFlight struct {
	RunID string `json:"run_id"`
	PID   int    `json:"pid"`
	// Start is the owner's start time where owner.StartTime has one
	// (Linux); zero elsewhere.
	Start uint64 `json:"start,omitempty"`
	// Stage is where a review's pre-step staged its copy of the QuickAgent
	// (RecordStage): outside the runner home, in the system's temp directory, so
	// the run's own sweep does not reach it, and the record that is forgotten
	// removes it.
	Stage string `json:"stage,omitempty"`
}

// runIDPattern is the engine's run id (engine.NewRunID: 8 hex digits),
// with room to grow. A record whose id does not match is never swept: its
// id becomes a docker label and object names.
var runIDPattern = regexp.MustCompile(`^[0-9a-f]{8,64}$`)

// RecordInFlight writes the record of a run this process is about to start.
func RecordInFlight(dir, runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("record run: bad run id %q", runID)
	}
	rec := inFlight{RunID: runID, PID: os.Getpid()}
	if start, ok := owner.StartTime(rec.PID); ok {
		rec.Start = start
	}
	return saveRecord(dir, rec)
}

// saveRecord writes rec as its run's record.
func saveRecord(dir string, rec inFlight) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	// Written whole then renamed, so a crash mid-write leaves no half
	// record for the next start to misread.
	tmp, err := os.CreateTemp(dir, ".run-*")
	if err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("record run: %w", err)
	}
	if err := os.Rename(tmp.Name(), recordPath(dir, rec.RunID)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("record run: %w", err)
	}
	return nil
}

// ForgetInFlight removes a run's record; a record already gone is fine. A
// review's staged copy the record names goes first, and a copy that cannot
// be removed keeps the record, so the next start or quit tries again.
func ForgetInFlight(dir, runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("forget run: bad run id %q", runID)
	}
	if rec, err := readRecord(dir, runID); err == nil && rec.Stage != "" {
		if err := removeStage(rec.Stage); err != nil {
			return fmt.Errorf("forget run: %w", err)
		}
	}
	if err := os.Remove(recordPath(dir, runID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("forget run: %w", err)
	}
	return nil
}

func recordPath(dir, runID string) string { return filepath.Join(dir, runID+".json") }

// readRecord reads runID's record, held to its own name.
func readRecord(dir, runID string) (inFlight, error) {
	var rec inFlight
	b, err := os.ReadFile(recordPath(dir, runID))
	if err != nil {
		return rec, err
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return rec, err
	}
	if rec.RunID != runID {
		return rec, fmt.Errorf("the record of run %s names run %s", runID, rec.RunID)
	}
	return rec, nil
}

// SweepOrphaned sweeps, by run id, every recorded run whose owner is gone,
// and removes its record once the sweep succeeded (a failed sweep — Docker
// not running yet — keeps the record for the next start). A live owner's
// record, this process's own, and anything unreadable or malformed are left
// alone. It returns the run ids it swept, sorted. The app runs it at start.
func SweepOrphaned(ctx context.Context, dir string, sweep func(context.Context, string) error) ([]string, error) {
	return sweepRecorded(ctx, dir, false, sweep)
}

// SweepAtQuit is SweepOrphaned for an app that is closing, after it has torn
// down what it had live: every record this process still holds is swept as
// well, since nothing of this process's is running any more. Those are the
// run the quit's bounded wait gave up on, and a run that ended leaving files
// its engine could not delete, whose record the app kept for this. Another
// live process's record is still left alone.
func SweepAtQuit(ctx context.Context, dir string, sweep func(context.Context, string) error) ([]string, error) {
	return sweepRecorded(ctx, dir, true, sweep)
}

func sweepRecorded(ctx context.Context, dir string, own bool, sweep func(context.Context, string) error) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var swept []string
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // a file this directory listed
		if err != nil {
			continue
		}
		var rec inFlight
		if json.Unmarshal(b, &rec) != nil || !runIDPattern.MatchString(rec.RunID) || name != rec.RunID+".json" {
			continue
		}
		if !ownerGone(rec.PID, rec.Start) && (!own || rec.PID != os.Getpid()) {
			continue
		}
		if err := sweep(ctx, rec.RunID); err != nil {
			errs = append(errs, fmt.Errorf("sweep run %s: %w", rec.RunID, err))
			continue
		}
		if err := ForgetInFlight(dir, rec.RunID); err != nil {
			errs = append(errs, err)
		}
		swept = append(swept, rec.RunID)
	}
	sort.Strings(swept)
	return swept, errors.Join(errs...)
}

// ownerGone reports whether the process a record names is provably gone.
func ownerGone(pid int, start uint64) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		// Our own pid: the writer is this process unless a start time
		// proves it an earlier one that the kernel's pid reuse succeeded.
		self, ok := owner.StartTime(pid)
		return start != 0 && ok && self != start
	}
	if start != 0 {
		if now, ok := owner.StartTime(pid); ok {
			return now != start
		}
	}
	return !processExists(pid)
}
