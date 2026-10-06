// Package runhistory is the desktop's short history of runs on this
// computer (plan desktop-home-and-library.md §4.6, D1, D8; D-068;
// agentry-desktop#659): the last 50 runs, chats and built-in jobs, each
// with its id, its kind, the release it ran, its times, how it ended and
// the folder Go chose for it, kept in runs.yaml beside desktop.yaml (0600).
//
// Only Go writes it, and only from what Go itself produced: the engine's
// result as it was read at the end of the run, a built-in's own return, or
// the registry's record of a run In Agentry whose output this app
// downloaded. It never holds the task, the inputs, the result text, a
// secret or the log, and the page never names a folder to it: it is read
// by run id (D-035).
package runhistory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

// Max is how many entries the history keeps; the oldest goes first.
const Max = 50

// FileName is the history's file in the config directory.
const FileName = "runs.yaml"

// Kinds of entry: a QuickAgent's run or chat, or a built-in's job (D8).
const (
	KindRun       = "run"
	KindChat      = "chat"
	KindDraft     = "draft"
	KindReview    = "review"
	KindKnowledge = "knowledge"
	KindVoice     = "voice"
)

// Where a run ran.
const (
	WhereHere    = "here"
	WhereAgentry = "agentry"
)

// Outcomes. A run In Agentry carries the registry's own words in
// Outcome and ExitClass instead.
const (
	OutcomeOK      = "ok"
	OutcomeError   = "error"
	OutcomeStopped = "stopped"
	// OutcomeBlocked: the QuickAgent could not do its task with what the run
	// had (the CLI's exit 18, agentry-cli#629; #680), by its own word or
	// the runner's evidence.
	OutcomeBlocked = "blocked"
)

// Entry is one run. Folder is absolute, or "" where the job kept none (a
// knowledge or voice build keeps its work in its store, not a run folder).
type Entry struct {
	ID        string    `yaml:"id"`
	Kind      string    `yaml:"kind"`
	Where     string    `yaml:"where"`
	Registry  string    `yaml:"registry,omitempty"`
	Ref       string    `yaml:"ref,omitempty"`
	Digest    string    `yaml:"digest,omitempty"`
	StartedAt time.Time `yaml:"startedAt"`
	EndedAt   time.Time `yaml:"endedAt"`
	Outcome   string    `yaml:"outcome"`
	ExitCode  int       `yaml:"exitCode,omitempty"`
	ExitClass string    `yaml:"exitClass,omitempty"`
	Folder    string    `yaml:"folder,omitempty"`
}

// file is runs.yaml.
type file struct {
	Version int     `yaml:"version"`
	Runs    []Entry `yaml:"runs"`
}

// version is the file's shape. A file of another version is read as empty
// and replaced on the next write: it is a cache of recent runs, not a
// record anything depends on.
const version = 1

// idPattern is a run id, a local one (hex) or the registry's (a UUID), as
// the result-view bindings take it.
var idPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]{3,63}$`)

// ValidID reports whether id can be an entry's.
func ValidID(id string) bool { return idPattern.MatchString(id) }

var kinds = map[string]bool{KindRun: true, KindChat: true, KindDraft: true, KindReview: true, KindKnowledge: true, KindVoice: true}

// valid is what an entry must be to be kept or read: an id, a kind, a
// place, and a folder that is absolute and clean when there is one. An
// entry that is not (a hand-edited file) is dropped, never shown.
func (e Entry) valid() bool {
	if !ValidID(e.ID) || !kinds[e.Kind] || (e.Where != WhereHere && e.Where != WhereAgentry) {
		return false
	}
	return e.Folder == "" || (filepath.IsAbs(e.Folder) && filepath.Clean(e.Folder) == e.Folder)
}

// mu serializes this process's reads and writes of the file.
var mu sync.Mutex

// Path is the history's file.
func Path() (string, error) {
	dir, err := configdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Add records e as the newest entry, replacing an entry with its id, and
// drops the oldest past Max.
func Add(e Entry) error {
	if e.Folder != "" {
		e.Folder = filepath.Clean(e.Folder)
	}
	if !e.valid() {
		return fmt.Errorf("a run history entry needs an id, a kind and an absolute folder or none (%q, %q)", e.ID, e.Kind)
	}
	mu.Lock()
	defer mu.Unlock()
	runs, err := load()
	if err != nil {
		return err
	}
	kept := []Entry{e}
	for _, x := range runs {
		if x.ID != e.ID {
			kept = append(kept, x)
		}
	}
	return save(kept)
}

// List is the history, newest first by when each run ended.
func List() ([]Entry, error) {
	mu.Lock()
	defer mu.Unlock()
	return load()
}

// Find is the entry with id.
func Find(id string) (Entry, bool, error) {
	if !ValidID(id) {
		return Entry{}, false, nil
	}
	runs, err := List()
	if err != nil {
		return Entry{}, false, err
	}
	for _, e := range runs {
		if e.ID == id {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

// Remove drops the entry with id. The run's folder is not touched.
func Remove(id string) error {
	mu.Lock()
	defer mu.Unlock()
	runs, err := load()
	if err != nil {
		return err
	}
	kept := runs[:0]
	for _, e := range runs {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	return save(kept)
}

// Clear removes the file. No run folder is touched.
func Clear() error {
	mu.Lock()
	defer mu.Unlock()
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("clear the run history: %w", err)
	}
	return nil
}

// load reads the file: absent is empty, and so is a file of another
// version; a file that does not parse is an error, said rather than
// written over.
func load() ([]Entry, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the config directory's fixed file
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the run history: %w", err)
	}
	var f file
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("the run history (%s) cannot be read: %w; Clear run history in Settings → Run defaults starts it again", path, err)
	}
	if f.Version != version {
		return nil, nil
	}
	runs := make([]Entry, 0, len(f.Runs))
	seen := map[string]bool{}
	for _, e := range f.Runs {
		if e.valid() && !seen[e.ID] {
			seen[e.ID] = true
			runs = append(runs, e)
		}
	}
	return newest(runs), nil
}

// newest is runs newest first by end time, at most Max.
func newest(runs []Entry) []Entry {
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].EndedAt.After(runs[j].EndedAt) })
	if len(runs) > Max {
		runs = runs[:Max]
	}
	return runs
}

// save writes the newest Max entries, 0600, through a file beside it that
// is renamed into place, so a reader never sees half a file.
func save(runs []Entry) error {
	runs = newest(runs)
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("write the run history: %w", err)
	}
	raw, err := yaml.Marshal(file{Version: version, Runs: runs})
	if err != nil {
		return fmt.Errorf("write the run history: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runs-*.yaml")
	if err != nil {
		return fmt.Errorf("write the run history: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write the run history: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write the run history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write the run history: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write the run history: %w", err)
	}
	return nil
}
