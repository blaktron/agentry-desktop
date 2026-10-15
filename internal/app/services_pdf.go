package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PDF reports in the run result. A run's export_pdf records each PDF it made
// in the envelope the engine writes after the run, result.json's `pdfs`
// block: path, SHA-256, pages, and the theme. A PDF that block lists with
// the SHA-256 of its bytes opens in the app's viewer (a bundled pdf.js,
// scripting off) and in the system's PDF viewer; any other PDF in the folder
// is Save only. The check is Go's, made on the bytes it then hands on, never
// the page's: RunPDF gives the viewer checked bytes, OpenRunPDF opens a
// fresh copy of checked bytes (never the file in the run folder, which could
// be swapped between the check and the open), and SaveRunPDF writes where a
// native dialog in this process says. Like the rest of the result view's
// bindings, they take a run id this app produced, and a path inside that
// run's outputs/ or reports/ that is read with no link at any part of it
// (readRunFile), nothing else.

// OutputPDFView is what the result view may do with a PDF: Exported is true
// when the run's envelope lists it with these bytes, and then Pages and
// Theme are the record's.
type OutputPDFView struct {
	Exported bool   `json:"exported"`
	Pages    int    `json:"pages,omitempty"`
	Theme    string `json:"theme,omitempty"`
}

// maxPDFBytes is the largest PDF the bindings read: export_pdf's own cap on
// a render. maxEnvelopeBytes bounds the read of result.json.
const (
	maxPDFBytes      = 64 << 20
	maxEnvelopeBytes = 8 << 20
)

type pdfRecord struct {
	sha256 string
	pages  int
}

// pdfRecords is the envelope's pdfs block: the theme's name, and each
// listed PDF by its path in the run folder.
type pdfRecords struct {
	theme string
	files map[string]pdfRecord
}

func isPDFPath(rel string) bool { return strings.EqualFold(path.Ext(rel), ".pdf") }

// readPDFRecords reads the pdfs block of the run folder's result.json. A
// missing, linked, oversized or unreadable envelope, or an entry that is
// not well formed, lists nothing, so nothing opens.
func readPDFRecords(dir string) pdfRecords {
	recs := pdfRecords{files: map[string]pdfRecord{}}
	b, ok := readRunFile(dir, "result.json", maxEnvelopeBytes)
	if !ok {
		return recs
	}
	var env struct {
		PDFs *struct {
			Theme struct {
				Name string `json:"name"`
			} `json:"theme"`
			Files []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
				Pages  int    `json:"pages"`
			} `json:"files"`
		} `json:"pdfs"`
	}
	if json.Unmarshal(b, &env) != nil || env.PDFs == nil {
		return recs
	}
	recs.theme = env.PDFs.Theme.Name
	for _, f := range env.PDFs.Files {
		sum := strings.ToLower(f.SHA256)
		if d, err := hex.DecodeString(sum); err != nil || len(d) != sha256.Size || !validRunPath(f.Path) || !strings.HasPrefix(f.Path, "reports/") {
			continue
		}
		recs.files[f.Path] = pdfRecord{sha256: sum, pages: max(f.Pages, 0)}
	}
	return recs
}

// validRunPath is a "/"-separated path inside the run folder's outputs/ or
// reports/, already clean: what listOutputs names a file, and nothing a
// page could use to climb out.
func validRunPath(rel string) bool {
	if rel == "" || strings.Contains(rel, `\`) || path.Clean(rel) != rel || !filepath.IsLocal(filepath.FromSlash(rel)) {
		return false
	}
	return strings.HasPrefix(rel, "outputs/") || strings.HasPrefix(rel, "reports/")
}

// readRunFile reads a file of the run folder by its "/"-separated path, at
// most limit bytes, the way listOutputs walks it: every part before the
// file a real directory and the file a regular one, a link anywhere
// refused. listOutputs never walks a linked folder, so a path a page names
// through one (outputs/docs, a link the run left to ~/Documents) must not
// reach a file the listing would never show. The open goes through an
// os.Root on the run folder as well, so a part swapped for a link after
// the checks still cannot lead out of it; the file is opened without
// following a link where the OS allows it and checked to be the one Lstat
// saw.
func readRunFile(dir, rel string, limit int64) ([]byte, bool) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, false
	}
	defer func() { _ = root.Close() }()
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		fi, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i], "/")))
		if err != nil || !fi.IsDir() {
			return nil, false // a link, a junction or a file where a folder was
		}
	}
	name := filepath.FromSlash(rel)
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, false
	}
	f, err := root.OpenFile(name, os.O_RDONLY|noFollowOpenFlags, 0)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	if after, err := f.Stat(); err != nil || !os.SameFile(before, after) {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, false
	}
	return b, true
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// exportedBytes reads a PDF the record lists and answers its bytes only
// when they are a PDF whose SHA-256 is the record's: the one check both the
// listing's mark and every binding that hands bytes on make.
func exportedBytes(dir, rel string, rec pdfRecord) ([]byte, bool) {
	b, ok := readRunFile(dir, rel, maxPDFBytes)
	if !ok || !bytes.HasPrefix(b, []byte("%PDF-")) || sha256Hex(b) != rec.sha256 {
		return nil, false
	}
	return b, true
}

// markPDFs sets each PDF's view: exported when the record lists its path
// with the SHA-256 of the bytes read now.
func markPDFs(dir string, view *RunOutputsView) {
	var recs *pdfRecords
	for i := range view.Files {
		f := &view.Files[i]
		if !isPDFPath(f.Path) {
			continue
		}
		if recs == nil {
			r := readPDFRecords(dir)
			recs = &r
		}
		f.PDF = &OutputPDFView{}
		rec, ok := recs.files[f.Path]
		if !ok {
			continue
		}
		if _, ok := exportedBytes(dir, f.Path, rec); !ok {
			continue
		}
		*f.PDF = OutputPDFView{Exported: true, Pages: rec.pages, Theme: recs.theme}
	}
}

var errNotExported = errors.New("this PDF is not one the run's PDF export made, so it is not opened here: Save it to open it")

// runPDFBytes reads a PDF of the run: any PDF when exported is false, and
// only one the record lists with these bytes when it is true.
func (r *RunAPI) runPDFBytes(runID, rel string, exported bool) ([]byte, error) {
	dir, err := r.folderOf(runID)
	if err != nil {
		return nil, err
	}
	if !validRunPath(rel) || !isPDFPath(rel) {
		return nil, fmt.Errorf("%q is not a PDF in the run's outputs/ or reports/", rel)
	}
	if exported {
		rec, listed := readPDFRecords(dir).files[rel]
		if !listed {
			return nil, errNotExported
		}
		b, ok := exportedBytes(dir, rel, rec)
		if !ok {
			return nil, errNotExported
		}
		return b, nil
	}
	b, ok := readRunFile(dir, rel, maxPDFBytes)
	if !ok {
		return nil, fmt.Errorf("%s could not be read: it is gone, not a regular file, or larger than %d MiB", path.Base(rel), maxPDFBytes>>20)
	}
	return b, nil
}

// RunPDF is an exported PDF's bytes, base64, for the app's viewer.
func (r *RunAPI) RunPDF(runID, rel string) (string, error) {
	b, err := r.runPDFBytes(runID, rel, true)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// OpenRunPDF opens an exported PDF in the system's PDF viewer: a copy of the
// checked bytes in a folder of this app's, removed when the app quits.
func (r *RunAPI) OpenRunPDF(runID, rel string) error {
	b, err := r.runPDFBytes(runID, rel, true)
	if err != nil {
		return err
	}
	r.pdfMu.Lock()
	if r.pdfDir == "" {
		sweepStalePDFCopies(os.TempDir(), time.Now().Add(-24*time.Hour))
		d, err := os.MkdirTemp("", "agentry-pdf-")
		if err != nil {
			r.pdfMu.Unlock()
			return fmt.Errorf("make a folder for the PDF's copy: %w", err)
		}
		r.pdfDir = d
	}
	dir := r.pdfDir
	r.pdfMu.Unlock()
	sub, err := os.MkdirTemp(dir, "open-")
	if err != nil {
		return fmt.Errorf("make a folder for the PDF's copy: %w", err)
	}
	p := filepath.Join(sub, copyName(rel))
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return fmt.Errorf("write the PDF's copy: %w", err)
	}
	open := r.openFile
	if open == nil {
		open = openWithSystemViewer // services_pdf_unix.go, services_pdf_windows.go
	}
	if err := open(p); err != nil {
		return fmt.Errorf("open the PDF in your PDF viewer: %w", err)
	}
	return nil
}

// SaveRunPDF writes a PDF of the run, exported or not, where the person
// chooses in the native save dialog, and answers the path ("" when the
// dialog was cancelled).
func (r *RunAPI) SaveRunPDF(runID, rel string) (string, error) {
	b, err := r.runPDFBytes(runID, rel, false)
	if err != nil {
		return "", err
	}
	save := r.saveFile
	if save == nil {
		save = func(name string) (string, error) {
			return runtime.SaveFileDialog(r.app.ctx, runtime.SaveDialogOptions{
				Title:           "Save the PDF",
				DefaultFilename: name,
				Filters:         []runtime.FileFilter{{DisplayName: "PDF (*.pdf)", Pattern: "*.pdf"}},
			})
		}
	}
	target, err := save(path.Base(rel))
	if err != nil {
		return "", fmt.Errorf("ask where to save the PDF: %w", err)
	}
	if target == "" {
		return "", nil
	}
	if err := os.WriteFile(target, b, 0o600); err != nil {
		return "", fmt.Errorf("save the PDF: %w", err)
	}
	return target, nil
}

// removePDFCopies removes the copies OpenRunPDF made; the app's shutdown
// calls it. A copy a viewer still holds open on Windows cannot be removed
// then, and an app that crashed removes nothing: the next OpenRunPDF sweeps
// folders of this kind a day old (sweepStalePDFCopies).
func (r *RunAPI) removePDFCopies() {
	r.pdfMu.Lock()
	defer r.pdfMu.Unlock()
	if r.pdfDir != "" {
		_ = os.RemoveAll(r.pdfDir)
		r.pdfDir = ""
	}
}

// sweepStalePDFCopies removes agentry-pdf-* folders in tmp last changed
// before cutoff: copies an earlier app left. A folder another app made
// today is left alone.
func sweepStalePDFCopies(tmp string, cutoff time.Time) {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "agentry-pdf-") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(tmp, e.Name()))
		}
	}
}

// copyName is the name OpenRunPDF gives its copy: the PDF's own name with
// anything but letters, digits, space, '.', '-' and '_' made '_', so the
// OS's open sees a plain name ending in .pdf.
func copyName(rel string) string {
	var b strings.Builder
	for _, c := range strings.TrimSuffix(path.Base(rel), path.Ext(rel)) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == ' ', c == '.', c == '-', c == '_':
			b.WriteRune(c)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), " .")
	if name == "" {
		name = "report"
	}
	return name + ".pdf"
}
