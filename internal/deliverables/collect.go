package deliverables

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/deliver"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/runfile"
)

// Collect copies the deliverables a downloaded run folder's envelope lists
// into root, as `agentry deliver` does: each after its digest is shown to
// be the envelope's, under a name made plain for any system, never over a
// file that is there, with the system's flag for a download on the copy.
// The copy is recorded in the folder's receipts/deliveries.ndjson, and the
// folder is otherwise left as it was.
//
// A folder already collected (its record holds a delivered copy) is left
// alone, so opening a run again never copies it a second time. A run that
// did not succeed, or lists nothing, has nothing to collect: no error.
func Collect(runDir, root string) ([]receipt.Delivery, error) {
	if root == "" {
		return nil, nil
	}
	if collected(runDir) {
		return nil, nil
	}
	env, err := deliver.ReadEnvelope(runDir)
	if err != nil || env.Status != "ok" || env.Deliverables == nil || len(env.Deliverables.Files) == 0 {
		return nil, nil //nolint:nilerr // a folder that lists nothing has nothing to collect
	}
	return deliver.Redeliver(runDir, deliver.FolderOptions{Root: root})
}

// collected says the run folder's own delivery record already holds a copy.
func collected(runDir string) bool {
	b, ok := runfile.Read(runDir, "receipts/"+deliver.LedgerFile, 8<<20)
	if !ok {
		return false
	}
	return bytes.Contains(b, []byte(`"status":"`+receipt.DeliveryDelivered+`"`))
}

// FindCopy is the copy of f this computer's Deliverables folder holds, when
// one does: a delivery the envelope records, inside root, that is a regular
// file reached through no link whose bytes are f's. The path the envelope
// names is only a hint; the file is hashed, so a path an envelope was made
// to name is never opened unless it holds the very bytes the run produced.
func FindCopy(root string, rec Record, f File) (string, bool) {
	if root == "" || !filepath.IsAbs(root) {
		return "", false
	}
	for _, d := range rec.Deliveries {
		if d.SHA256 != f.SHA256 || !filepath.IsAbs(d.Object) {
			continue
		}
		rel, err := filepath.Rel(root, d.Object)
		if err != nil || !filepath.IsLocal(rel) {
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
		return filepath.Join(root, rel), true
	}
	return "", false
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
	return ds[0].Object, nil
}
