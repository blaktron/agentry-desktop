package app

// A run's deliverables, the files it was for. The run's envelope names them
// (result.json's deliverables block: path, title, SHA-256); the desktop lists
// them first on a run's result and on Home, and a click opens, shows or
// reveals one. What the agent wrote stays hostile, so:
//
//   - the page names a run and a path this app listed, never a folder; Go
//     reads the run's folder by its run id (folderOf) and the envelope's
//     paths through no link (internal/runfile);
//   - a file is listed only after its bytes hash to the envelope's SHA-256,
//     and every binding that hands bytes on or opens a file checks again,
//     on the bytes it uses;
//   - one click opens only the document types on internal/deliverables'
//     list, each also checked by its first bytes, and never the run
//     folder's own file: it opens the copy in the Deliverables folder when
//     that holds the very same bytes, else a fresh copy made for it in a
//     folder of this app's, with the system's flag for a download.
//
// The copy into the Deliverables folder is the engine's own (a local run
// given Request.DeliverTo) or internal/deliverables.Collect (a run In
// Agentry brought down).

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/blaktron/agentry-cli/pkg/runner/deliver"

	"github.com/blaktron/agentry-desktop/internal/deliverables"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// defaultDeliverablesRoot names the Deliverables folder when none was
// chosen; a field so a test never meets the person's own Documents folder.
var defaultDeliverablesRoot = deliverables.DefaultRoot

// maxViewBytes is the largest text file the viewer reads.
const maxViewBytes = 2 << 20

// deliverablesFolder is the Deliverables folder as Settings has it: the
// folder in effect (the chosen one, else the default), whether the person
// kept the default, and whether deliveries are on. The folder is "" when the
// default cannot be named (no Documents folder); err says why.
func deliverablesFolder() (dir string, isDefault, on bool, err error) {
	st, err := settings.Load()
	if err != nil {
		return "", true, false, err
	}
	on = st.DeliverablesEnabled()
	if d := st.DeliverablesDir; d != "" && filepath.IsAbs(d) {
		return filepath.Clean(d), false, on, nil
	}
	dir, err = defaultDeliverablesRoot()
	return dir, true, on, err
}

// deliverFolder is the folder a run on this computer is given to copy its
// deliverables into, "" when the switch is off or no folder can be named. A
// run is never refused for want of one.
func deliverFolder() string {
	dir, _, on, err := deliverablesFolder()
	if err != nil || !on {
		return ""
	}
	return dir
}

// DeliverableView is one deliverable as the page draws it.
type DeliverableView struct {
	Path string `json:"path"`
	// Name is the file's own name, Title the one the agent or the runner gave
	// it ("" when it gave none).
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	// Type is the file's extension, lower case.
	Type   string `json:"type"`
	Bytes  int64  `json:"bytes"`
	Marked bool   `json:"marked"`
	// CanOpen says one click opens it. ViewKind is how the app can show it
	// itself: pdf, markdown or text, "" for none.
	CanOpen  bool   `json:"canOpen"`
	ViewKind string `json:"viewKind"`
	// NotOpened says why a click does not open it: "type" (its type is on no
	// list) or "contents" (its bytes are not what its type says).
	NotOpened string `json:"notOpened,omitempty"`
	// CopiedTo is the folder in the Deliverables folder that holds a copy of
	// these very bytes, "" when none does.
	CopiedTo string `json:"copiedTo,omitempty"`
}

// RunDeliverablesView is a run's deliverables, listed first on its result.
type RunDeliverablesView struct {
	Files []DeliverableView `json:"files"`
	// Withheld counts the files the run's record names that are not offered:
	// not well formed, gone, or not the bytes the record names.
	Withheld  int  `json:"withheld"`
	Truncated bool `json:"truncated"`
	// On is the Deliverables switch; Folder the folder in effect, "" when
	// none can be named.
	On     bool   `json:"on"`
	Folder string `json:"folder,omitempty"`
}

// describe is f as the page draws it, after its bytes were hashed against the
// record; ok is false for a file that is not the record's.
func describe(dir, root string, rec deliverables.Record, f deliverables.File) (DeliverableView, bool) {
	c, err := deliverables.Check(dir, f)
	if err != nil {
		return DeliverableView{}, false
	}
	v := DeliverableView{
		Path: f.Path, Name: filepath.Base(filepath.FromSlash(f.Path)), Title: f.Title, Type: f.Ext(),
		Bytes: f.Bytes, Marked: f.Marked,
	}
	switch {
	case c.TypeErr == nil:
		v.CanOpen, v.ViewKind = true, deliverables.ViewKind(v.Type)
	case errors.Is(c.TypeErr, deliverables.ErrNotThatType):
		v.NotOpened = "contents"
	case !deliverables.CanOpen(v.Type):
		v.NotOpened = "type"
	default:
		v.NotOpened = "contents" // a macro project, a control or an embedded object
	}
	if p, ok := deliverables.FindCopy(dir, root, rec, f); ok {
		v.CopiedTo = filepath.Dir(p)
	}
	return v, true
}

// RunDeliverables lists what the run was for, each file after its bytes were
// shown to be the record's. A run that lists none answers an empty list.
func (r *RunAPI) RunDeliverables(runID string) (*RunDeliverablesView, error) {
	dir, err := r.folderOf(runID)
	if err != nil {
		return nil, err
	}
	root, _, on, _ := deliverablesFolder()
	rec := deliverables.ReadRecord(dir)
	v := &RunDeliverablesView{Files: []DeliverableView{}, Withheld: rec.Withheld, Truncated: rec.Truncated, On: on, Folder: root}
	for _, f := range rec.Files {
		d, ok := describe(dir, root, rec, f)
		if !ok {
			v.Withheld++
			continue
		}
		v.Files = append(v.Files, d)
	}
	return v, nil
}

// deliverableOf is the run's deliverable at path: one its record names, with
// its bytes shown to be the record's. A path the record does not name is
// refused, whatever else is in the folder.
func (r *RunAPI) deliverableOf(runID, rel string) (dir string, rec deliverables.Record, f deliverables.File, c deliverables.Checked, err error) {
	if dir, err = r.folderOf(runID); err != nil {
		return
	}
	rec = deliverables.ReadRecord(dir)
	found := false
	for _, x := range rec.Files {
		if x.Path == rel {
			f, found = x, true
			break
		}
	}
	if !found {
		err = fmt.Errorf("%q is not a file this run was for", rel)
		return
	}
	if c, err = deliverables.Check(dir, f); err != nil {
		err = fmt.Errorf("%s: %w", filepath.Base(rel), err)
	}
	return
}

// notOpened is the sentence for a file one click does not open.
func notOpened(f deliverables.File, c deliverables.Checked) error {
	name := filepath.Base(filepath.FromSlash(f.Path))
	if !deliverables.CanOpen(f.Ext()) {
		return fmt.Errorf("%s is not opened from here: a .%s file can be acted on by the system, so Show in folder finds it and you open it yourself", name, f.Ext())
	}
	return fmt.Errorf("%s is not opened: %w. Show in folder finds it", name, c.TypeErr)
}

// copyRoot is the folder of this app's own that holds the copies made for
// opening, made when first needed and removed at quit (removePDFCopies).
func (r *RunAPI) copyRoot() (string, error) {
	r.pdfMu.Lock()
	defer r.pdfMu.Unlock()
	if r.pdfDir == "" {
		sweepStalePDFCopies(os.TempDir(), time.Now().Add(-24*time.Hour))
		d, err := os.MkdirTemp("", "agentry-pdf-")
		if err != nil {
			return "", fmt.Errorf("make a folder for the copy: %w", err)
		}
		r.pdfDir = d
	}
	return r.pdfDir, nil
}

// OpenRunDeliverable opens a deliverable of the run in the program the
// system has for its type. The type must be on the list and its bytes what
// the type says; the file opened is the copy in the Deliverables folder when
// that holds the same bytes, else a fresh copy made for it.
func (r *RunAPI) OpenRunDeliverable(runID, rel string) error {
	dir, rec, f, c, err := r.deliverableOf(runID, rel)
	if err != nil {
		return err
	}
	if c.TypeErr != nil {
		return notOpened(f, c)
	}
	root, _, _, _ := deliverablesFolder()
	target, ok := deliverables.FindCopy(dir, root, rec, f)
	if !ok {
		base, err := r.copyRoot()
		if err != nil {
			return err
		}
		// A folder of its own for each click, so a second click does not
		// find the first's file under the name it wants.
		temp, err := os.MkdirTemp(base, "deliverable-")
		if err != nil {
			return fmt.Errorf("make a folder for the copy: %w", err)
		}
		agent := deliver.AgentName(rec.Ref)
		if agent == "" {
			agent = "run"
		}
		if target, err = deliverables.OpenCopy(dir, temp, agent, f); err != nil {
			return fmt.Errorf("open %s: %w", filepath.Base(rel), err)
		}
	}
	open := r.openFile
	if open == nil {
		open = openWithSystemViewer
	}
	if err := open(target); err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(rel), err)
	}
	return nil
}

// ShowRunDeliverable shows a deliverable in the file manager: its copy in the
// Deliverables folder when that holds the same bytes, else the run folder.
// The file manager is only asked to show, never to open, a file.
func (r *RunAPI) ShowRunDeliverable(runID, rel string) error {
	dir, rec, f, _, err := r.deliverableOf(runID, rel)
	if err != nil {
		return err
	}
	root, _, _, _ := deliverablesFolder()
	if p, ok := deliverables.FindCopy(dir, root, rec, f); ok {
		reveal := r.revealFile
		if reveal == nil {
			reveal = revealFileInFileManager
		}
		return reveal(p)
	}
	return r.ShowRunFolder(runID)
}

// revealFileInFileManager asks the OS to show where a file is, selecting it
// where the file manager can. It is given a copy this app found and checked.
func revealFileInFileManager(p string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", p)
	case "windows":
		cmd = exec.Command("explorer", "/select,"+p)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(p))
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// DeliverableTextView is a text deliverable for the app's own viewer.
type DeliverableTextView struct {
	// Kind is "markdown" or "text".
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// RunDeliverableText reads a Markdown, text, CSV or JSON deliverable for the
// viewer, only when its bytes are the record's. The page renders Markdown
// with raw HTML skipped and everything else as plain text.
func (r *RunAPI) RunDeliverableText(runID, rel string) (*DeliverableTextView, error) {
	dir, _, f, c, err := r.deliverableOf(runID, rel)
	if err != nil {
		return nil, err
	}
	if c.TypeErr != nil {
		return nil, notOpened(f, c)
	}
	kind := deliverables.ViewKind(f.Ext())
	if kind != "markdown" && kind != "text" {
		return nil, fmt.Errorf("%s is not shown as text here", filepath.Base(rel))
	}
	if f.Bytes > maxViewBytes {
		return nil, fmt.Errorf("%s is %s, too large to show here: Open it instead", filepath.Base(rel), "over 2 MiB")
	}
	b, err := deliverables.ReadChecked(dir, f, maxViewBytes)
	if err != nil {
		return nil, fmt.Errorf("%s is not shown: %w", filepath.Base(rel), err)
	}
	text := string(b)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "�")
	}
	return &DeliverableTextView{Kind: kind, Text: text}, nil
}

// RunDeliverablePDF is a PDF deliverable's bytes, base64, for the app's
// viewer, only when they are the record's and start as a PDF does.
func (r *RunAPI) RunDeliverablePDF(runID, rel string) (string, error) {
	dir, _, f, c, err := r.deliverableOf(runID, rel)
	if err != nil {
		return "", err
	}
	if f.Ext() != "pdf" {
		return "", fmt.Errorf("%s is not a PDF", filepath.Base(rel))
	}
	if c.TypeErr != nil {
		return "", notOpened(f, c)
	}
	b, err := deliverables.ReadChecked(dir, f, maxPDFBytes)
	if err != nil {
		return "", fmt.Errorf("%s: %w", filepath.Base(rel), err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// RecentDeliverableView is a deliverable on Home, with the run it came from.
type RecentDeliverableView struct {
	DeliverableView
	RunID string `json:"runId"`
	Ref   string `json:"ref,omitempty"`
	// Where is "here" or "agentry"; EndedAt when the run ended.
	Where   string `json:"where"`
	EndedAt string `json:"endedAt,omitempty"`
}

// RecentDeliverablesView is Home's Deliverables card.
type RecentDeliverablesView struct {
	Files []RecentDeliverableView `json:"files"`
	// Remember is Run defaults' switch: with it off no run is remembered,
	// so there is nothing for the card to read.
	Remember bool `json:"remember"`
	// On is the Deliverables switch; Folder the folder in effect.
	On      bool   `json:"on"`
	Folder  string `json:"folder,omitempty"`
	Error   string `json:"error,omitempty"`
	Scanned int    `json:"scanned"`
}

// How far the Deliverables card reads: this many recent runs, for at most
// this many files.
const (
	deliverableRunsRead = 25
	deliverableFilesMax = 5
)

// RecentDeliverables is Home's Deliverables card: the most recent
// deliverables across the runs the history keeps (and the runs schedules on
// this computer fired), newest first, at most limit (1 to 5). It reads the
// history and each run's envelope, never the Deliverables folder, so a file
// someone put there is not listed; each file is listed only when its bytes
// are the envelope's. It starts nothing.
func (r *RunAPI) RecentDeliverables(limit int) (*RecentDeliverablesView, error) {
	limit = min(max(limit, 1), deliverableFilesMax)
	root, _, on, ferr := deliverablesFolder()
	v := &RecentDeliverablesView{Files: []RecentDeliverableView{}, Remember: historyOn(), On: on, Folder: root}
	if ferr != nil && root == "" {
		v.Error = ferr.Error()
	}
	var entries []runhistory.Entry
	if v.Remember {
		es, err := runhistory.List()
		if err != nil {
			v.Error = err.Error()
		}
		for _, e := range es {
			if (e.Kind == runhistory.KindRun || e.Kind == runhistory.KindChat) && e.Folder != "" {
				entries = append(entries, e)
			}
		}
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.ID] = true
	}
	for _, row := range scheduledRows(deliverableRunsRead) {
		if seen[row.ID] {
			continue
		}
		if e, _, ok := scheduledEntry(row.ID); ok && e.Folder != "" {
			entries = append(entries, e)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].EndedAt.After(entries[j].EndedAt) })
	for _, e := range entries {
		if v.Scanned >= deliverableRunsRead || len(v.Files) >= limit {
			break
		}
		v.Scanned++
		dir, err := recordedFolder(e)
		if err != nil {
			continue
		}
		rec := deliverables.ReadRecord(dir)
		for _, f := range rec.Files {
			if len(v.Files) >= limit {
				break
			}
			d, ok := describe(dir, root, rec, f)
			if !ok {
				continue
			}
			v.Files = append(v.Files, RecentDeliverableView{
				DeliverableView: d, RunID: e.ID, Ref: e.Ref, Where: e.Where, EndedAt: rfc3339(e.EndedAt),
			})
		}
	}
	return v, nil
}

// OpenDeliverablesFolder shows the Deliverables folder in the file manager,
// making it first when nothing has been copied yet. It refuses while the
// switch is off, so a button is never offered for a folder nothing fills.
func (r *RunAPI) OpenDeliverablesFolder() error {
	dir, _, on, err := deliverablesFolder()
	if err != nil {
		return err
	}
	if !on {
		return errors.New("the Deliverables switch is off, so there is no folder to show: turn it on in Settings → Run defaults")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not a folder this app can show", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("make the Deliverables folder: %w", err)
	}
	open := r.openFolder
	if open == nil {
		open = openFolderInFileManager
	}
	return open(dir)
}

// collectDownloaded copies the deliverables of a run In Agentry whose output
// was brought down into dir to the Deliverables folder, once: a folder
// already collected is left alone. It never fails the download.
func collectDownloaded(dir string) {
	root := deliverFolder()
	if root == "" || dir == "" {
		return
	}
	if _, err := deliverables.Collect(dir, root); err != nil {
		slog.Warn("deliverables: a run's files were not copied to the Deliverables folder", "err", err)
	}
}
