package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The in-app PDF viewer's bindings (plan scratch-and-pdf-reports.md D15,
// D16; agentry-desktop#730). A PDF opens in the app or the system viewer
// only when the run's envelope (result.json) lists it with the SHA-256 of
// its bytes; any other PDF is Save only. Go holds the bytes to the record,
// never the page, and hands on only bytes it has just checked.

const examplePDF = "%PDF-1.7\nexported\n%%EOF\n"

// headlessBytes is listed with its own SHA-256 but is not a PDF: no
// "%PDF-" header, so it is neither marked exported nor handed on.
const headlessBytes = "<html>not a pdf</html>"

func sumOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// pdfRun is a run folder with result.json listing reports/exported.pdf.
func pdfRun(t *testing.T) (*RunAPI, string) {
	t.Helper()
	r, dir := outputsAPI(t)
	env := map[string]any{
		"agentry": "run/v1", "result": "done",
		"pdfs": map[string]any{
			"theme": map[string]any{"name": "letter", "version": "1.0.0", "sha256": strings.Repeat("a", 64)},
			"files": []map[string]any{
				{"path": "reports/exported.pdf", "sha256": sumOf(examplePDF), "pages": 2, "bytes": len(examplePDF)},
				{"path": "reports/changed.pdf", "sha256": sumOf(examplePDF), "pages": 1, "bytes": len(examplePDF)},
				{"path": "reports/headless.pdf", "sha256": sumOf(headlessBytes), "pages": 1, "bytes": len(headlessBytes)},
			},
		},
	}
	b, _ := json.Marshal(env)
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	writeRunFile(t, filepath.Join(dir, "reports", "exported.pdf"), examplePDF)
	writeRunFile(t, filepath.Join(dir, "reports", "changed.pdf"), "%PDF-1.7\nother bytes\n%%EOF\n")
	writeRunFile(t, filepath.Join(dir, "reports", "unrecorded.pdf"), examplePDF)
	writeRunFile(t, filepath.Join(dir, "reports", "headless.pdf"), headlessBytes)
	writeRunFile(t, filepath.Join(dir, "outputs", "elsewhere.pdf"), examplePDF)
	writeRunFile(t, filepath.Join(dir, "reports", "exported.md"), "# Exported\n")
	return r, dir
}

func TestRunOutputsMarksOnlyTheExportedPDF(t *testing.T) {
	r, _ := pdfRun(t)
	v, err := r.RunOutputs("run-1")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*OutputPDFView{}
	for _, f := range v.Files {
		got[f.Path] = f.PDF
	}
	if p := got["reports/exported.pdf"]; p == nil || !p.Exported || p.Pages != 2 || p.Theme != "letter" {
		t.Fatalf("exported: %+v", p)
	}
	for _, path := range []string{"reports/changed.pdf", "reports/unrecorded.pdf", "outputs/elsewhere.pdf", "reports/headless.pdf"} {
		if p := got[path]; p == nil || p.Exported {
			t.Errorf("%s: %+v, want a PDF that is not exported", path, p)
		}
	}
	if got["reports/exported.md"] != nil {
		t.Error("a Markdown file is not a PDF")
	}
}

func TestRunPDFHandsOverOnlyTheExportedBytes(t *testing.T) {
	r, _ := pdfRun(t)
	b64, err := r.RunPDF("run-1", "reports/exported.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := base64.StdEncoding.DecodeString(b64); string(b) != examplePDF {
		t.Fatalf("bytes = %q", b)
	}
	for _, path := range []string{"reports/changed.pdf", "reports/unrecorded.pdf", "outputs/elsewhere.pdf", "reports/headless.pdf", "reports/exported.md", "../result.json", "/etc/passwd", "reports/../reports/exported.pdf"} {
		if _, err := r.RunPDF("run-1", path); err == nil {
			t.Errorf("RunPDF(%q) handed bytes over", path)
		}
	}
	if _, err := r.RunPDF("run-2", "reports/exported.pdf"); err == nil {
		t.Error("RunPDF of a run this app did not produce")
	}
}

// A PDF swapped for a link after the run is refused, even under its name.
func TestRunPDFRefusesALink(t *testing.T) {
	r, dir := pdfRun(t)
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	writeRunFile(t, outside, examplePDF)
	p := filepath.Join(dir, "reports", "exported.pdf")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, p); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if _, err := r.RunPDF("run-1", "reports/exported.pdf"); err == nil {
		t.Fatal("a link was followed")
	}
}

// Open in system viewer opens a fresh copy of the checked bytes, never the
// file in the run folder, and nothing for a PDF that is not exported.
func TestOpenRunPDFOpensACheckedCopy(t *testing.T) {
	r, dir := pdfRun(t)
	var opened []string
	r.openFile = func(p string) error { opened = append(opened, p); return nil }
	if err := r.OpenRunPDF("run-1", "reports/exported.pdf"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || strings.HasPrefix(opened[0], dir) || filepath.Ext(opened[0]) != ".pdf" {
		t.Fatalf("opened %v, want one copy outside the run folder", opened)
	}
	if b, err := os.ReadFile(opened[0]); err != nil || string(b) != examplePDF {
		t.Fatalf("the copy holds %q, %v", b, err)
	}
	if err := r.OpenRunPDF("run-1", "reports/unrecorded.pdf"); err == nil || len(opened) != 1 {
		t.Fatalf("a PDF that is not exported was opened: %v %v", err, opened)
	}
	r.removePDFCopies()
	if _, err := os.Stat(opened[0]); !os.IsNotExist(err) {
		t.Fatalf("the copy outlived the cleanup: %v", err)
	}
}

// Save writes any PDF of the run where the person chose: an exported one
// as checked, any other as it is.
func TestSaveRunPDFWritesWhereThePersonChose(t *testing.T) {
	r, _ := pdfRun(t)
	target := filepath.Join(t.TempDir(), "saved.pdf")
	var offered string
	r.saveFile = func(name string) (string, error) { offered = name; return target, nil }
	got, err := r.SaveRunPDF("run-1", "reports/unrecorded.pdf")
	if err != nil || got != target || offered != "unrecorded.pdf" {
		t.Fatalf("save: %q, %v (offered %q)", got, err, offered)
	}
	if b, _ := os.ReadFile(target); string(b) != examplePDF {
		t.Fatalf("saved %q", b)
	}
	r.saveFile = func(string) (string, error) { return "", nil }
	if got, err := r.SaveRunPDF("run-1", "reports/exported.pdf"); err != nil || got != "" {
		t.Fatalf("a cancelled dialog: %q, %v", got, err)
	}
	if _, err := r.SaveRunPDF("run-1", "reports/exported.md"); err == nil {
		t.Fatal("Save is for PDFs")
	}
}

// A page-supplied path that passes through a link the run left (a folder
// linked to ~/Documents, or outputs/ itself linked to /) reads nothing:
// listOutputs never walks a linked folder, and neither does a binding
// (found in review; ARCHITECTURE §9, D-035).
func TestAPathThroughALinkedFolderReadsNothing(t *testing.T) {
	r, dir := pdfRun(t)
	outside := t.TempDir()
	writeRunFile(t, filepath.Join(outside, "secret.pdf"), examplePDF)
	if err := os.Symlink(outside, filepath.Join(dir, "outputs", "docs")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	target := filepath.Join(t.TempDir(), "got.pdf")
	r.saveFile = func(string) (string, error) { return target, nil }
	if _, err := r.SaveRunPDF("run-1", "outputs/docs/secret.pdf"); err == nil {
		t.Fatal("Save read a file through a linked folder")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("something was written")
	}

	// outputs/ itself a link out of the run folder.
	if err := os.RemoveAll(filepath.Join(dir, "outputs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "outputs")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveRunPDF("run-1", "outputs/secret.pdf"); err == nil {
		t.Fatal("Save read a file through a linked outputs/")
	}

	// reports/ a link to a folder holding the exported bytes under the
	// recorded name: the record matches, the path does not lead there.
	other := t.TempDir()
	writeRunFile(t, filepath.Join(other, "exported.pdf"), examplePDF)
	if err := os.RemoveAll(filepath.Join(dir, "reports")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(dir, "reports")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunPDF("run-1", "reports/exported.pdf"); err == nil {
		t.Fatal("RunPDF read through a linked reports/")
	}
}

// The record is held to reports/, the one folder only write_report and
// export_pdf write: an entry naming outputs/ opens nothing.
func TestARecordOutsideReportsOpensNothing(t *testing.T) {
	r, dir := pdfRun(t)
	b, _ := json.Marshal(map[string]any{"pdfs": map[string]any{"files": []map[string]any{
		{"path": "outputs/elsewhere.pdf", "sha256": sumOf(examplePDF), "pages": 1},
	}}})
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	if _, err := r.RunPDF("run-1", "outputs/elsewhere.pdf"); err == nil {
		t.Fatal("a record outside reports/ opened a PDF")
	}
}

func TestCopyNameIsPlain(t *testing.T) {
	for in, want := range map[string]string{
		"reports/letter.pdf":            "letter.pdf",
		"reports/Q3 report.PDF":         "Q3 report.pdf",
		"reports/a\u202egpj.exe.pdf":    "a_gpj.exe.pdf",
		"reports/..pdf":                 "report.pdf",
		"reports/x;calc&.pdf":           "x_calc_.pdf",
		"reports/naïve-résumé_v1.0.pdf": "na_ve-r_sum__v1.0.pdf",
	} {
		if got := copyName(in); got != want {
			t.Errorf("copyName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Copies an earlier app left a day ago are swept; today's are kept, and
// nothing that is not one of them is touched.
func TestSweepStalePDFCopies(t *testing.T) {
	tmp := t.TempDir()
	old := filepath.Join(tmp, "agentry-pdf-old")
	fresh := filepath.Join(tmp, "agentry-pdf-new")
	other := filepath.Join(tmp, "something-else")
	for _, d := range []string{old, fresh, other} {
		writeRunFile(t, filepath.Join(d, "x.pdf"), examplePDF)
	}
	past := time.Now().Add(-48 * time.Hour)
	for _, d := range []string{old, other} {
		if err := os.Chtimes(d, past, past); err != nil {
			t.Fatal(err)
		}
	}
	sweepStalePDFCopies(tmp, time.Now().Add(-24*time.Hour))
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("a day-old copy folder was kept")
	}
	for _, d := range []string{fresh, other} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s was removed", filepath.Base(d))
		}
	}
}
