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
// as the completing run would have.
//
// What the record cannot know is decided when it is forgotten. The profile
// is removed only when it is the one the run held: the same profile.json,
// byte for byte, so a profile saved again since (the guided form, a run
// that completed and kept it) is the newer writer's. It is left while a
// live run of the release can be using it: another app's recorded run that
// names it, or any run, the CLI's or a desktop's, that holds a run lock in
// the release's run directories (engine.RunsInUse). That run settles the
// profile when it completes.

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
	// SHA256 is the profile.json the run read.
	SHA256 string `json:"sha256"`
}

// runsInUse is engine.RunsInUse, a variable so a test can name a run as
// live without holding a lock.
var runsInUse = engine.RunsInUse

// RecordProfile adds a prepared run's profile, and the run's keep decision
// for it, to the run's record, which RecordInFlight wrote. A run with no
// profile records none.
func RecordProfile(dir, runID, profDir, runRoot string, keep bool) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("record run: bad run id %q", runID)
	}
	b, err := os.ReadFile(filepath.Join(profDir, "profile.json")) //nolint:gosec // the release's profile
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	rec, err := readRecord(dir, runID)
	if err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	sum := sha256.Sum256(b)
	rec.Profile = &recordedProfile{Dir: profDir, RunRoot: runRoot, Keep: keep, SHA256: hex.EncodeToString(sum[:])}
	return saveRecord(dir, rec)
}

// settleProfile removes the profile a forgotten record names when its run
// was not to keep it, as the head of this file says. An error keeps the
// record, so the next start or quit tries again.
func settleProfile(dir string, rec inFlight) error {
	p := rec.Profile
	if p == nil || p.Keep {
		return nil
	}
	home := filepath.Dir(dir)
	if !underRunnerHome(filepath.Join(home, "profiles"), p.Dir) {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(p.Dir, "profile.json")) //nolint:gosec // a profile directory under the runner home
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the run's profile: %w", err)
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != p.SHA256 {
		return nil
	}
	if profileInUse(dir, home, rec) {
		return nil
	}
	return profile.Remove(p.Dir)
}

// profileInUse says whether a live run other than rec's may be using the
// profile rec names.
func profileInUse(dir, home string, rec inFlight) bool {
	if underRunnerHome(installdir.AgentsRoot(home), rec.Profile.RunRoot) {
		for _, id := range runsInUse(rec.Profile.RunRoot) {
			if id != rec.RunID {
				return true
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".json") || name == rec.RunID+".json" {
			continue
		}
		other, err := readRecord(dir, strings.TrimSuffix(name, ".json"))
		if err != nil || other.Profile == nil || other.Profile.Dir != rec.Profile.Dir {
			continue
		}
		// This process's other records are its own runs, which are ending
		// with it (a quit) or have their own lock (a live run).
		if other.PID != os.Getpid() && !ownerGone(other.PID, other.Start) {
			return true
		}
	}
	return false
}

// underRunnerHome says whether path is a release's directory under root,
// root/<namespace>/<name>/<release>, every level a real directory: a
// record is never a way to delete or open something else.
func underRunnerHome(root, path string) bool {
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
	for _, part := range parts {
		at = filepath.Join(at, part)
		if fi, err := os.Lstat(at); err != nil || !fi.IsDir() {
			return false
		}
	}
	return true
}
