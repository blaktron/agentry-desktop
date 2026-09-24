package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// The result view's two bindings (plan guided-runs.md §5, D8;
// agentry-desktop#227). They are handed a run id and nothing else, so a
// page that has been made to call them reaches a folder this app produced
// a result for and no other; and they never open what a run wrote.

func outputsAPI(t *testing.T) (*RunAPI, string) {
	t.Helper()
	dir := t.TempDir()
	r := &RunAPI{app: &App{ctx: context.Background()}}
	r.resultView(&run.Result{RunID: "run-1", OutDir: dir, Status: "ok"})
	return r, dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunOutputsNamesARunNeverAPath(t *testing.T) {
	r, _ := outputsAPI(t)
	for _, id := range []string{"", "run-2", "../run-1", "/etc", os.TempDir()} {
		if _, err := r.RunOutputs(id); err == nil {
			t.Errorf("RunOutputs(%q) listed a folder this app produced no result for", id)
		}
		if err := r.ShowRunFolder(id); err == nil {
			t.Errorf("ShowRunFolder(%q) opened a folder this app produced no result for", id)
		}
	}
}

func TestRunOutputsListsOutputsThenReportsWithPreviews(t *testing.T) {
	r, dir := outputsAPI(t)
	write(t, filepath.Join(dir, "outputs", "b.md"), "# B\n")
	write(t, filepath.Join(dir, "outputs", "deep", "a.csv"), "x,y\n1,2\n")
	write(t, filepath.Join(dir, "outputs", "image.png"), "\x89PNG\r\n")
	write(t, filepath.Join(dir, "outputs", "notes"), "plain, no extension")
	write(t, filepath.Join(dir, "outputs", "big.txt"), strings.Repeat("a", maxPreviewBytes+1))
	write(t, filepath.Join(dir, "outputs", "bad.txt"), "a\x00b")
	write(t, filepath.Join(dir, "reports", "report.md"), "report")
	// Not listed at all: the envelope and the receipts are not the run's
	// output, and the receipts are shown from what the registry or the
	// runner kept, never from a file the agent could have written.
	write(t, filepath.Join(dir, "result.json"), "{}")
	write(t, filepath.Join(dir, "receipts", "run-plan.json"), "{}")

	v, err := r.RunOutputs("run-1")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	kinds := map[string]string{}
	for _, f := range v.Files {
		got = append(got, f.Path)
		kinds[f.Path] = f.PreviewKind + "/" + f.NoPreview
	}
	want := []string{"outputs/b.md", "outputs/bad.txt", "outputs/big.txt", "outputs/deep/a.csv", "outputs/image.png", "outputs/notes", "reports/report.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("listed %v, want %v", got, want)
	}
	for path, kind := range map[string]string{
		"outputs/b.md":       "markdown/",
		"outputs/deep/a.csv": "text/",
		"outputs/notes":      "text/",
		"outputs/image.png":  "none/binary",
		"outputs/bad.txt":    "none/binary",
		"outputs/big.txt":    "none/large",
	} {
		if kinds[path] != kind {
			t.Errorf("%s: preview %q, want %q", path, kinds[path], kind)
		}
	}
	for _, f := range v.Files {
		if f.PreviewKind == "none" && f.Preview != "" {
			t.Errorf("%s carries a preview it said it has none of", f.Path)
		}
	}
}

func TestRunOutputsFollowsNoLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	r, dir := outputsAPI(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	write(t, secret, "not the run's")
	write(t, filepath.Join(dir, "outputs", "ok.txt"), "fine")
	if err := os.Symlink(secret, filepath.Join(dir, "outputs", "link.txt")); err != nil {
		t.Fatal(err)
	}
	// reports/ itself a link to somewhere else: not walked.
	other := t.TempDir()
	write(t, filepath.Join(other, "elsewhere.md"), "outside")
	if err := os.Symlink(other, filepath.Join(dir, "reports")); err != nil {
		t.Fatal(err)
	}

	v, err := r.RunOutputs("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Files) != 1 || v.Files[0].Path != "outputs/ok.txt" {
		t.Fatalf("listed %+v, want outputs/ok.txt alone", v.Files)
	}
	if v.Skipped != 2 {
		t.Errorf("skipped %d, want 2 (the file link and the linked reports/)", v.Skipped)
	}
	for _, f := range v.Files {
		if strings.Contains(f.Preview, "not the run's") || strings.Contains(f.Preview, "outside") {
			t.Fatalf("a link was followed: %+v", f)
		}
	}
}

func TestShowRunFolderOpensTheFolderAndNothingElse(t *testing.T) {
	r, dir := outputsAPI(t)
	var opened []string
	r.openFolder = func(d string) error {
		opened = append(opened, d)
		return nil
	}
	if err := r.ShowRunFolder("run-1"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != filepath.Clean(dir) {
		t.Fatalf("opened %v, want the run folder %s", opened, dir)
	}

	// A folder replaced by a file is not opened.
	gone, _ := outputsAPI(t)
	gone.openFolder = r.openFolder
	d2, _ := gone.folderOf("run-1")
	if err := os.RemoveAll(d2); err != nil {
		t.Fatal(err)
	}
	write(t, d2, "a file now")
	if err := gone.ShowRunFolder("run-1"); err == nil {
		t.Fatal("ShowRunFolder opened a file in place of the run folder")
	}
	if len(opened) != 1 {
		t.Fatalf("the opener was called again: %v", opened)
	}
}

func TestAHostedRunsFolderIsRememberedByItsID(t *testing.T) {
	r := &RunAPI{app: &App{ctx: context.Background()}}
	dir := t.TempDir()
	r.rememberFolder("hosted-1", dir)
	if got, err := r.folderOf("hosted-1"); err != nil || got != filepath.Clean(dir) {
		t.Fatalf("folderOf = %q, %v", got, err)
	}
	r.rememberFolder("", dir)
	r.rememberFolder("x", "")
	if _, err := r.folderOf(""); err == nil {
		t.Fatal("an empty id was remembered")
	}
	if _, err := r.folderOf("x"); err == nil {
		t.Fatal("an empty folder was remembered")
	}
}
