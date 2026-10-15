package run

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
)

// A run whose profile is not to be kept removes the profile when it
// completes (finishProfile): the secret values the guided form saved with
// Keep off live only while the run does. A run that never completes (the
// app killed, or quitting while the engine outlasts the quit's wait) would
// leave them, so the run's record also carries its profile once the run is
// prepared (RecordProfile), and forgetting the record settles the profile
// as the completing run would have (settleProfile).

// recordedProfile is the profile a recorded run holds.
type recordedProfile struct {
	// Dir is the release's profile directory, profile.Dir under the runner
	// home; removing it removes the release's MCP server store too, as a
	// completing run does.
	Dir string `json:"dir"`
	// RunRoot is the release's directory the engine runs in, whose run
	// directories a live run of the release holds a lock in.
	RunRoot string `json:"run_root"`
	// Keep is the run's own decision: the stored flag, or the run setup's
	// tick over it.
	Keep bool `json:"keep"`
	// SHA256 is the profile.json the run loaded.
	SHA256 string `json:"sha256"`
}

// runsInUse is engine.RunsInUse, a variable so a test can name a run as
// live without holding a lock.
var runsInUse = engine.RunsInUse

// heldProfile is the profile in profDir as a run about to use it holds it,
// with the run's keep decision; nil when the release has no profile.
func heldProfile(profDir, runRoot string, keep bool) (*recordedProfile, error) {
	sum, err := profileSum(profDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the profile: %w", err)
	}
	return &recordedProfile{Dir: profDir, RunRoot: runRoot, Keep: keep, SHA256: sum}, nil
}

// profileSum is the SHA-256 of the profile.json in dir.
func profileSum(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "profile.json")) //nolint:gosec // a release's profile directory
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// RecordProfile adds a prepared run's profile to the run's record, which
// RecordInFlight wrote.
func RecordProfile(dir, runID string, p recordedProfile) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("bad run id %q", runID)
	}
	rec, err := readRecord(dir, runID)
	if err != nil {
		return err
	}
	rec.Profile = &p
	return saveRecord(dir, rec)
}

// settleProfile removes the profile a forgotten record names when its run
// was not to keep it, whole, as the completing run would have. What the
// record cannot know is decided now. The profile is removed only when it is
// the one the run loaded: the same profile.json, so a profile saved again
// since (the guided form, a run that completed and kept it) is the newer
// writer's. It is left while a live run of the release may be using it
// (profileInUse), which settles it when it completes. And only a release's
// profile directory under the runner home is ever removed. An error keeps
// the record, so the next start or quit tries again.
func settleProfile(dir string, rec inFlight) error {
	p := rec.Profile
	if p == nil || p.Keep {
		return nil
	}
	home := filepath.Dir(dir)
	if !isReleaseDir(filepath.Join(home, "profiles"), p.Dir) {
		return nil
	}
	sum, err := profileSum(p.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the run's profile: %w", err)
	}
	if sum != p.SHA256 || profileInUse(dir, home, rec) {
		return nil
	}
	if err := profile.Remove(p.Dir); err != nil {
		return fmt.Errorf("remove the profile the run was not to keep: %w", err)
	}
	return nil
}

// profileInUse says whether a live run other than rec's may be using the
// profile rec names: one holding a run lock in the release's run
// directories (the CLI's or a desktop's), or another live app's record that
// names the profile. A run root that is not the release's directory under
// the runner home cannot be asked, and counts as in use.
func profileInUse(dir, home string, rec inFlight) bool {
	if !isReleaseDir(installdir.AgentsRoot(home), rec.Profile.RunRoot) {
		return true
	}
	if slices.ContainsFunc(runsInUse(rec.Profile.RunRoot), func(id string) bool { return id != rec.RunID }) {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !e.Type().IsRegular() || !ok || id == rec.RunID || !runIDPattern.MatchString(id) {
			continue
		}
		other, err := readRecord(dir, id)
		if err != nil || other.Profile == nil || other.Profile.Dir != rec.Profile.Dir {
			continue
		}
		// This process's other records are its own runs, which are ending
		// with it (a quit) or hold their own run lock (a live run).
		if other.PID != os.Getpid() && !ownerGone(other.PID, other.Start) {
			return true
		}
	}
	return false
}

// isReleaseDir says whether path is a release's directory under root,
// root/<namespace>/<name>/<release>, with root and every level a real
// directory at the time of asking, so that a record naming any other path,
// or a link standing for a level, removes nothing.
func isReleaseDir(root, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 3 || slices.ContainsFunc(parts, func(s string) bool { return s == "" || s == "." || s == ".." }) {
		return false
	}
	at := root
	for _, part := range append([]string{""}, parts...) {
		at = filepath.Join(at, part)
		if fi, err := os.Lstat(at); err != nil || !fi.IsDir() {
			return false
		}
	}
	return true
}
