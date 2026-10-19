package deliverables

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/runner/deliver"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/runfile"
)

// Collect copies the deliverables a downloaded run folder lists into root, as
// `agentry deliver` does: each after its digest is shown to be the
// envelope's, under a name made plain for any system, never over a file that
// is there, with the system's flag for a download on the copy. It copies
// only what ReadRecord offers (a path under reports/, at most MaxFiles, no
// capture of a browsing run), and records the copies in the folder's
// receipts/deliveries.ndjson; the folder is otherwise left as it was.
//
// A file already copied to root is not copied again, and one that was
// refused or failed twice is left alone, so opening a run again never makes
// a second copy and never retries for ever. A run that did not succeed, or
// lists nothing, has nothing to collect: no error. Two collections of one
// folder never overlap.
func Collect(runDir, root string) ([]receipt.Delivery, error) {
	if root == "" {
		return nil, nil
	}
	mu := folderLock(runDir)
	mu.Lock()
	defer mu.Unlock()

	env, err := deliver.ReadEnvelope(runDir)
	if err != nil || env.Status != "ok" {
		return nil, nil //nolint:nilerr // a folder that lists nothing has nothing to collect
	}
	rec := ReadRecord(runDir)
	if len(rec.Files) == 0 {
		return nil, nil
	}
	pending := pendingFiles(rec.Files, ledger(runDir), root)
	if len(pending) == 0 {
		return nil, nil
	}
	files := make([]receipt.Deliverable, 0, len(pending))
	for _, f := range pending {
		files = append(files, receipt.Deliverable{Path: f.Path, Title: f.Title, SHA256: f.SHA256, Bytes: f.Bytes, Type: f.Ext(), Marked: f.Marked})
	}
	all := make([]receipt.Deliverable, 0, len(rec.Files))
	for _, f := range rec.Files {
		all = append(all, receipt.Deliverable{Title: f.Title})
	}
	ds := deliver.ToFolder(runDir, files, deliver.FolderOptions{
		Root: root, Agent: deliver.AgentName(env.Ref), Label: deliver.Label(all, env.RunID),
	})
	if err := deliver.WriteLedger(runDir, ds, true); err != nil {
		return ds, fmt.Errorf("the files were copied, but %s could not be recorded in the run's receipts: %w", deliver.LedgerFile, err)
	}
	return ds, nil
}

// maxAttempts is how many times a file that was refused or could not be
// written is tried again before a run is left alone.
const maxAttempts = 2

var (
	locksMu sync.Mutex
	locks   = map[string]*sync.Mutex{}
)

// folderLock is the lock for one run folder.
func folderLock(dir string) *sync.Mutex {
	locksMu.Lock()
	defer locksMu.Unlock()
	if locks[dir] == nil {
		locks[dir] = &sync.Mutex{}
	}
	return locks[dir]
}

// ledger is the run folder's own delivery record, line by line.
func ledger(runDir string) []receipt.Delivery {
	b, ok := runfile.Read(runDir, "receipts/"+deliver.LedgerFile, maxEnvelopeBytes)
	if !ok {
		return nil
	}
	var out []receipt.Delivery
	for _, line := range bytes.Split(b, []byte("\n")) {
		var d receipt.Delivery
		if len(bytes.TrimSpace(line)) > 0 && json.Unmarshal(line, &d) == nil {
			out = append(out, d)
		}
	}
	return out
}

// pendingFiles are the files with no copy in root yet and fewer than
// maxAttempts failed tries there: the record is read per destination, so a
// copy made for another folder neither counts nor blocks.
func pendingFiles(files []File, led []receipt.Delivery, root string) []File {
	var out []File
	for _, f := range files {
		delivered, failed := false, 0
		for _, d := range led {
			if d.Destination != root || d.SHA256 != f.SHA256 {
				continue
			}
			if d.Status == receipt.DeliveryDelivered {
				delivered = true
			} else {
				failed++
			}
		}
		if !delivered && failed < maxAttempts {
			out = append(out, f)
		}
	}
	return out
}

// FindCopy is the copy of f this computer's Deliverables folder holds, when
// one does: a delivery the envelope records, inside root and outside the run
// folder, of the same file type as f, that is a regular file reached through
// no link whose bytes are f's. The path the envelope names is only a hint;
// the file is hashed, so a path an envelope was made to name is never opened
// unless it holds the very bytes the run produced, and never when it is the
// run folder's own file (a Deliverables folder that holds the run folders
// would otherwise contain it) or another type's (an agent can list two files
// with the same bytes).
func FindCopy(runDir, root string, rec Record, f File) (string, bool) {
	if root == "" || !filepath.IsAbs(root) {
		return "", false
	}
	for _, d := range rec.Deliveries {
		if d.SHA256 != f.SHA256 || !filepath.IsAbs(d.Object) || d.Error == receipt.DeliveryErrNotMarked || Ext(d.Object) != f.Ext() {
			continue
		}
		rel, err := filepath.Rel(root, d.Object)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		p := filepath.Join(root, rel)
		if within(runDir, p) {
			continue
		}
		file, size, err := runfile.Open(root, filepath.ToSlash(rel), MaxFileBytes)
		if err != nil {
			continue
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(file, size+1))
		_ = file.Close()
		if err != nil || n != size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			continue
		}
		return p, true
	}
	return "", false
}

// within says p is dir or below it, by name and with links resolved.
func within(dir, p string) bool {
	inside := func(a, b string) bool {
		rel, err := filepath.Rel(a, b)
		return err == nil && filepath.IsLocal(rel) || rel == "."
	}
	if inside(dir, p) {
		return true
	}
	rd, err1 := filepath.EvalSymlinks(dir)
	rp, err2 := filepath.EvalSymlinks(p)
	return err1 == nil && err2 == nil && inside(rd, rp)
}

// ErrCopy says a fresh copy for opening could not be made.
var ErrCopy = errors.New("a copy to open could not be made")

// OpenCopy makes a fresh copy of the deliverable f in tempRoot, a folder of
// the app's own, for one click to open: made by the same code that collects
// (the digest checked as it is copied, a plain name, the system's flag for a
// download so Office opens it in Protected View), so a file is never opened
// from the run folder itself. It answers the copy's path.
func OpenCopy(runDir, tempRoot, agent string, f File) (string, error) {
	ds := deliver.ToFolder(runDir, []receipt.Deliverable{{
		Path: f.Path, Title: f.Title, SHA256: f.SHA256, Bytes: f.Bytes, Type: f.Ext(), Marked: f.Marked,
	}}, deliver.FolderOptions{Root: tempRoot, Agent: agent, Label: "open"})
	if len(ds) != 1 || ds[0].Status != receipt.DeliveryDelivered || ds[0].Object == "" {
		reason := "unknown"
		if len(ds) == 1 {
			reason = ds[0].Status + ", " + ds[0].Error
		}
		return "", fmt.Errorf("%w (%s)", ErrCopy, reason)
	}
	if !strings.HasPrefix(ds[0].Object, tempRoot) {
		return "", ErrCopy
	}
	// A document the system would open without its care for a download
	// (Protected View, a browser's warning) is not opened when the flag could not be written.
	if ds[0].Error == receipt.DeliveryErrNotMarked && needsDownloadFlag(f.Ext()) {
		return "", fmt.Errorf("%w (this file system cannot flag it as a download, so it is not opened)", ErrCopy)
	}
	return ds[0].Object, nil
}

// needsDownloadFlag says a type is opened by a program that treats a file
// from outside the computer with more care: an Office document, a table
// whose formulas a spreadsheet would run, or a web page whose scripts a
// browser would run with the rights of a local file.
func needsDownloadFlag(ext string) bool {
	switch ext {
	case "docx", "xlsx", "pptx", "csv", "html", "htm":
		return true
	}
	return false
}
