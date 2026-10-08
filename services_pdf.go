package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PDF reports in the run result (plan scratch-and-pdf-reports.md D10, D15,
// D16; agentry-desktop#730, D-072). A run's export_pdf records each PDF it
// made in the envelope the engine writes after the run, result.json's
// `pdfs` block: path, SHA-256, pages, and the theme. A PDF that block lists
// with the SHA-256 of its bytes opens in the app's viewer (a bundled
// pdf.js, scripting off) and in the system's PDF viewer; any other PDF in
// the folder is Save only. The check is Go's, made on the bytes it then
// hands on, never the page's: RunPDF gives the viewer checked bytes,
// OpenRunPDF opens a fresh copy of checked bytes (never the file in the
// run folder, which could be swapped between the check and the open), and
// SaveRunPDF writes where a native dialog in this process says. Like the
// rest of the result view's bindings, they take a run id this app produced
// and a path inside that run's outputs/ or reports/, nothing else.

// OutputPDFView is what the result view may do with a PDF: Exported is true
// when the run's envelope lists it with these bytes, and then Pages and
// Theme are the record's.
type OutputPDFView struct {
	Exported bool   `json:"exported"`
	Pages    int    `json:"pages,omitempty"`
	Theme    string `json:"theme,omitempty"`
}

// maxPDFBytes is the largest PDF the bindings read: export_pdf's own cap on
// a render (DR-81). maxEnvelopeBytes bounds the read of result.json.
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
	b, ok := readRegular(filepath.Join(dir, "result.json"), maxEnvelopeBytes)
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
		if d, err := hex.DecodeString(sum); err != nil || len(d) != sha256.Size || !validRunPath(f.Path) {
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

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
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
		b, ok := readRegular(filepath.Join(dir, filepath.FromSlash(f.Path)), maxPDFBytes)
		if !ok || sha256Hex(b) != rec.sha256 {
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
	var rec pdfRecord
	if exported {
		var listed bool
		if rec, listed = readPDFRecords(dir).files[rel]; !listed {
			return nil, errNotExported
		}
	}
	b, ok := readRegular(filepath.Join(dir, filepath.FromSlash(rel)), maxPDFBytes)
	if !ok {
		return nil, fmt.Errorf("%s could not be read: it is gone, not a regular file, or larger than %d MiB", path.Base(rel), maxPDFBytes>>20)
	}
	if exported && (sha256Hex(b) != rec.sha256 || !bytes.HasPrefix(b, []byte("%PDF-"))) {
		return nil, errNotExported
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
		d, err := os.MkdirTemp("", "agentry-pdf-")
		if err != nil {
			r.pdfMu.Unlock()
			return err
		}
		r.pdfDir = d
	}
	dir := r.pdfDir
	r.pdfMu.Unlock()
	sub, err := os.MkdirTemp(dir, "open-")
	if err != nil {
		return err
	}
	p := filepath.Join(sub, path.Base(rel))
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return err
	}
	open := r.openFile
	if open == nil {
		open = openWithSystemViewer
	}
	return open(p)
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
	if err != nil || target == "" {
		return "", err
	}
	if err := os.WriteFile(target, b, 0o600); err != nil {
		return "", err
	}
	return target, nil
}

// removePDFCopies removes the copies OpenRunPDF made; the app's shutdown
// calls it.
func (r *RunAPI) removePDFCopies() {
	r.pdfMu.Lock()
	defer r.pdfMu.Unlock()
	if r.pdfDir != "" {
		_ = os.RemoveAll(r.pdfDir)
		r.pdfDir = ""
	}
}

// openWithSystemViewer asks the OS to open a PDF copy OpenRunPDF wrote,
// with the program the person has for PDFs.
func openWithSystemViewer(p string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", p)
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	return cmd.Start()
}
