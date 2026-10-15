package run

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A review's staged copy. The Examiner's pre-step (the CLI's
// examiner.Prepare) copies the QuickAgent under review into a directory of
// its own in the system's temp directory, named agentry-examiner-*, which
// its Cleanup removes when the review returns. An app killed mid-review
// runs no Cleanup, and the copy is outside the runner home, where Sweep
// looks. So the review records the copy's path in its run's record as soon
// as the pre-step returns (RecordStage), and forgetting the record removes
// it (ForgetInFlight): at the next start for a review whose app died, at
// quit for one the quit's wait gave up on.
//
// What the record cannot name is a copy made by a pre-step the app died
// inside, in the seconds before it returned; SweepStaleStages removes this
// user's agentry-examiner-* directories once they are a day old, the CLI's
// `agentry review` included. Removing a live review's copy that old harms
// nothing: a run reads its own copy of its inputs, made when it starts.

// StagePrefix is the name the Examiner's pre-step gives its staged copy
// (the CLI's examiner.Prepare); TestAReviewsStagedCopyIsOnItsRecordAndGoesWithIt
// holds it to the copy the CLI makes.
const StagePrefix = "agentry-examiner-"

// StaleStageAge is how old a staged copy no record names must be before
// SweepStaleStages removes it.
const StaleStageAge = 24 * time.Hour

// RecordStage adds a review's staged copy to its run's record, which
// RecordInFlight wrote.
func RecordStage(dir, runID, stage string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("record run: bad run id %q", runID)
	}
	if !isStage(stage) {
		return fmt.Errorf("record run: %q is not a review's staged copy", stage)
	}
	rec, err := readRecord(dir, runID)
	if err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	rec.Stage = stage
	return saveRecord(dir, rec)
}

// isStage says whether path names a staged copy where the pre-step makes
// one: directly in the system's temp directory, under its name.
func isStage(path string) bool {
	return filepath.IsAbs(path) && filepath.Dir(path) == filepath.Clean(os.TempDir()) &&
		strings.HasPrefix(filepath.Base(path), StagePrefix)
}

// removeStage removes a staged copy a record names. A path that is not one
// is left alone, as is anything standing in its place that is not a real
// directory this user owns: a record is never a way to delete something
// else. A copy whose temp directory is not this process's (TMPDIR changed
// between launches) is not reached; SweepStaleStages does not reach it
// either.
func removeStage(path string) error {
	if !isStage(path) {
		return nil
	}
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove the review's staged copy: %w", err)
	}
	if !fi.IsDir() || !ownedByMe(fi) {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove the review's staged copy: %w", err)
	}
	return nil
}

// SweepStaleStages removes this user's staged copies in the system's temp
// directory that are older than age, by their own time, and returns the
// names it removed. Only real directories under the pre-step's name are
// touched.
func SweepStaleStages(age time.Duration) ([]string, error) {
	tmp := filepath.Clean(os.TempDir())
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-age)
	var removed []string
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, StagePrefix) {
			continue
		}
		path := filepath.Join(tmp, name)
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() || !fi.ModTime().Before(cutoff) || !ownedByMe(fi) {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, fmt.Errorf("remove a stale review copy: %w", err))
			continue
		}
		removed = append(removed, name)
	}
	return removed, errors.Join(errs...)
}
