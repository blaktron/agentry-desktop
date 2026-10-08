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
)

// The in-app PDF viewer's bindings (plan scratch-and-pdf-reports.md D15,
// D16; agentry-desktop#730). A PDF opens in the app or the system viewer
// only when the run's envelope (result.json) lists it with the SHA-256 of
// its bytes; any other PDF is Save only. Go holds the bytes to the record,
// never the page, and hands on only bytes it has just checked.

const examplePDF = "%PDF-1.7\nexported\n%%EOF\n"

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
			},
		},
	}
	b, _ := json.Marshal(env)
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	writeRunFile(t, filepath.Join(dir, "reports", "exported.pdf"), examplePDF)
	writeRunFile(t, filepath.Join(dir, "reports", "changed.pdf"), "%PDF-1.7\nother bytes\n%%EOF\n")
	writeRunFile(t, filepath.Join(dir, "reports", "unrecorded.pdf"), examplePDF)
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
	for _, path := range []string{"reports/changed.pdf", "reports/unrecorded.pdf", "outputs/elsewhere.pdf"} {
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
	for _, path := range []string{"reports/changed.pdf", "reports/unrecorded.pdf", "outputs/elsewhere.pdf", "reports/exported.md", "../result.json", "/etc/passwd", "reports/../reports/exported.pdf"} {
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
