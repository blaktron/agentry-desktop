package main

// What a finished run wrote, for the result view (plan guided-runs.md §5,
// M4; agentry-desktop#227): the files under its folder's outputs/ and
// reports/, each with a text preview where it is text, and a way to show
// the folder in the file manager.
//
// Two rules hold these bindings to the run and to nothing else:
//
//   - The page names a run, never a path. A run's folder is known here only
//     because Go produced the run's result itself (RunAgent, EndSession, a
//     hosted run's download) and remembered its folder under its run id; an
//     id this process did not produce is refused. So a page that has been
//     made to call these bindings can list or reveal a run's folder, and
//     nothing else on the machine.
//   - Nothing an agent wrote is opened. A file is read for its preview,
//     through no link, and only when it is a regular file inside the folder;
//     ShowRunFolder opens the run's folder itself in the file manager, never
//     a file in it, because a file a run wrote can be one the OS would
//     execute (plan D8).

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// Caps on what a listing reads: a preview is never larger than this, and a
// folder with more entries than this is listed up to the cap.
const (
	maxPreviewBytes = 256 << 10
	maxOutputFiles  = 500
	// maxPreviewTotal bounds what one listing hands the page: past it, a
	// file is listed with no preview.
	maxPreviewTotal = 4 << 20
)

// runIDPattern is what a run id looks like, a local one (hex) or the
// registry's (a UUID): an id the registry sends is used as a folder name
// and a map key, so anything else is not remembered.
var runIDPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]{3,63}$`)

func validRunID(id string) bool { return runIDPattern.MatchString(id) }

// OutputFileView is one file a run wrote. Path is relative to the run
// folder, "/"-separated; Preview is the file's text when PreviewKind is
// "markdown" or "text", and PreviewKind "none" says why there is none.
type OutputFileView struct {
	Path        string `json:"path"`
	Bytes       int64  `json:"bytes"`
	PreviewKind string `json:"previewKind"`
	Preview     string `json:"preview,omitempty"`
	// NoPreview is "large" or "binary" when PreviewKind is "none".
	NoPreview string `json:"noPreview,omitempty"`
}

// RunOutputsView is a run folder's outputs/ and reports/, outputs first.
// Skipped counts what was left out: links, anything that is not a regular
// file, and entries past the cap.
type RunOutputsView struct {
	Files   []OutputFileView `json:"files"`
	Skipped int              `json:"skipped"`
	// Truncated says the listing stopped at its cap.
	Truncated bool `json:"truncated"`
}

var errUnknownRun = errors.New("this app did not run that, so it has no folder for it")

// rememberFolder records the folder of a run whose result Go produced.
func (r *RunAPI) rememberFolder(runID, dir string) {
	if runID == "" || dir == "" {
		return
	}
	r.foldersMu.Lock()
	defer r.foldersMu.Unlock()
	if r.folders == nil {
		r.folders = map[string]string{}
	}
	r.folders[runID] = filepath.Clean(dir)
}

func (r *RunAPI) folderOf(runID string) (string, error) {
	r.foldersMu.Lock()
	defer r.foldersMu.Unlock()
	dir, ok := r.folders[runID]
	if !ok {
		return "", errUnknownRun
	}
	return dir, nil
}

// resultView flattens an engine result and remembers its folder.
func (r *RunAPI) resultView(res *run.Result) *RunResultView {
	v := runResultView(res)
	r.rememberFolder(v.RunID, v.OutDir)
	return v
}

// RunOutputs lists what the run wrote under outputs/ and reports/.
func (r *RunAPI) RunOutputs(runID string) (*RunOutputsView, error) {
	dir, err := r.folderOf(runID)
	if err != nil {
		return nil, err
	}
	return listOutputs(dir)
}

// ShowRunFolder shows the run's folder in the file manager.
func (r *RunAPI) ShowRunFolder(runID string) error {
	dir, err := r.folderOf(runID)
	if err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("the run folder is gone: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	open := r.openFolder
	if open == nil {
		open = openFolderInFileManager
	}
	return open(dir)
}

// openFolderInFileManager asks the OS to show a folder. It is given the
// run folder alone, which ShowRunFolder has checked is a directory.
func openFolderInFileManager(dir string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		// -R reveals the folder in Finder rather than opening it, so a
		// folder named like an app bundle is shown, not launched.
		cmd = exec.Command("open", "-R", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

// listOutputs walks outputs/ then reports/ under dir. A part that is itself
// a link is not followed, and inside them WalkDir never follows one.
func listOutputs(dir string) (*RunOutputsView, error) {
	view := &RunOutputsView{Files: []OutputFileView{}}
	budget := maxPreviewTotal
	for _, part := range []string{"outputs", "reports"} {
		root := filepath.Join(dir, part)
		info, err := os.Lstat(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			view.Skipped++
			continue
		}
		var found []OutputFileView
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				view.Skipped++
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				view.Skipped++ // a link, a device, a socket, a FIFO
				return nil
			}
			if len(view.Files)+len(found) >= maxOutputFiles {
				view.Skipped++
				view.Truncated = true
				return fs.SkipAll
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil || !filepath.IsLocal(rel) {
				view.Skipped++
				return nil
			}
			f, ok := readOutputFile(p, filepath.ToSlash(rel), budget)
			if !ok {
				view.Skipped++
				return nil
			}
			budget -= len(f.Preview)
			found = append(found, f)
			return nil
		})
		if err != nil && !errors.Is(err, fs.SkipAll) {
			return nil, err
		}
		sort.Slice(found, func(i, j int) bool { return found[i].Path < found[j].Path })
		view.Files = append(view.Files, found...)
	}
	return view, nil
}

// readOutputFile reads one file for its preview. The file is opened and
// then checked to be the same regular file the walk saw, so a file swapped
// for a link between the two is not read.
func readOutputFile(p, rel string, budget int) (OutputFileView, bool) {
	before, err := os.Lstat(p)
	if err != nil || !before.Mode().IsRegular() {
		return OutputFileView{}, false
	}
	v := OutputFileView{Path: rel, Bytes: before.Size(), PreviewKind: "none"}
	if before.Size() > maxPreviewBytes || before.Size() > int64(budget) {
		v.NoPreview = "large"
		return v, true
	}
	// Opened without following a link and without blocking on a FIFO where
	// the OS allows it (openNoFollow), then checked to be the same file.
	f, err := openNoFollow(p)
	if err != nil {
		return OutputFileView{}, false
	}
	defer func() { _ = f.Close() }()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return OutputFileView{}, false
	}
	b, err := io.ReadAll(io.LimitReader(f, maxPreviewBytes+1))
	if err != nil || len(b) > maxPreviewBytes {
		v.NoPreview = "large"
		return v, true
	}
	if !isText(rel, b) {
		v.NoPreview = "binary"
		return v, true
	}
	v.Preview = string(b)
	if strings.HasSuffix(strings.ToLower(rel), ".md") || strings.HasSuffix(strings.ToLower(rel), ".markdown") {
		v.PreviewKind = "markdown"
	} else {
		v.PreviewKind = "text"
	}
	return v, true
}

var textExts = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".text": true, ".log": true, ".csv": true, ".tsv": true,
	".json": true, ".jsonl": true, ".ndjson": true, ".yaml": true, ".yml": true, ".xml": true, ".html": true,
	".htm": true, ".diff": true, ".patch": true, ".ini": true, ".toml": true, ".sql": true, ".sh": true,
	".py": true, ".js": true, ".ts": true, ".go": true, ".rs": true, ".java": true, ".rb": true, ".css": true,
}

// isText is a known text extension (or none at all) whose bytes are valid
// UTF-8 with no NUL: anything else is offered as a file, not as garbled
// characters.
func isText(rel string, b []byte) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	if ext != "" && !textExts[ext] {
		return false
	}
	head := b
	if len(head) > 8192 {
		head = head[:8192]
	}
	for _, c := range head {
		if c == 0 {
			return false
		}
	}
	return utf8.Valid(b)
}
