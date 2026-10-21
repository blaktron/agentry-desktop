package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-cli/pkg/runner/session"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// What a run was for, listed first and opened with one click. The run's
// envelope names the files; the desktop lists a file only when its bytes are
// the envelope's, and opens only a short list of types, each checked by its
// first bytes, never the run folder's own file.

// isolateDeliverables gives the test a Deliverables folder of its own, which
// no other test copies into.
func isolateDeliverables(t *testing.T) {
	t.Helper()
	old := defaultDeliverablesRoot
	root := filepath.Join(t.TempDir(), "Documents", "Agentry")
	defaultDeliverablesRoot = func() (string, error) { return root, nil }
	t.Cleanup(func() { defaultDeliverablesRoot = old })
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func officeZip(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte("<x/>"))
	}
	_ = zw.Close()
	return buf.Bytes()
}

// deliverableRun is a run folder (run id run-1) holding the files given and
// an envelope that lists each of them as a deliverable.
func deliverableRun(t *testing.T, files map[string][]byte) (*RunAPI, string) {
	t.Helper()
	freshConfig(t)
	r, dir := outputsAPI(t)
	var list []map[string]any
	for rel, body := range files {
		writeRunFile(t, filepath.Join(dir, filepath.FromSlash(rel)), string(body))
		list = append(list, map[string]any{"path": rel, "title": "T " + filepath.Base(rel), "sha256": sum(body), "bytes": len(body), "type": filepath.Ext(rel), "marked": true})
	}
	b, _ := json.Marshal(map[string]any{
		"agentry": "run/v1", "status": "ok", "runId": "run-1", "ref": "acme/weekly@1.0.0",
		"deliverables": map[string]any{"files": list},
	})
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	return r, dir
}

func opened(r *RunAPI) *[]string {
	var got []string
	r.openFile = func(p string) error { got = append(got, p); return nil }
	return &got
}

func TestRunDeliverablesListsOnlyWhatMatchesTheEnvelope(t *testing.T) {
	good := []byte("# Report\n")
	r, dir := deliverableRun(t, map[string][]byte{"reports/report.md": good, "reports/changed.md": []byte("original")})
	// The file changes after the runner took its digest.
	writeRunFile(t, filepath.Join(dir, "reports", "changed.md"), "something else")
	v, err := r.RunDeliverables("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Files) != 1 || v.Files[0].Path != "reports/report.md" || v.Withheld != 1 {
		t.Fatalf("listed %+v, withheld %d; want only report.md and one withheld", v.Files, v.Withheld)
	}
	if f := v.Files[0]; !f.CanOpen || f.ViewKind != "markdown" || f.Name != "report.md" || f.Type != "md" {
		t.Fatalf("file view %+v", f)
	}
	if _, err := r.RunDeliverables("run-2"); err == nil {
		t.Fatal("a run this app produced no result for was listed")
	}
}

func TestARunWithNoDeliverablesListsAnEmptyListNotNull(t *testing.T) {
	freshConfig(t)
	r, dir := outputsAPI(t)
	writeRunFile(t, filepath.Join(dir, "result.json"), `{"agentry":"run/v1","status":"ok"}`)
	v, err := r.RunDeliverables("run-1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if !strings.Contains(string(b), `"files":[]`) {
		t.Fatalf("a run with none marshals to %s", b)
	}
	h, err := r.RecentDeliverables(5)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(h)
	if !strings.Contains(string(b), `"files":[]`) {
		t.Fatalf("Home's card with none marshals to %s", b)
	}
}

// One click opens a document type on the list whose bytes are what the type
// says; anything the system could act on, and anything that is not what it
// says, is refused and nothing is opened.
func TestOpenRunDeliverableOpensOnlyTheListedTypes(t *testing.T) {
	files := map[string][]byte{
		"reports/ok.pdf":      []byte("%PDF-1.7\nx\n%%EOF\n"),
		"reports/ok.docx":     officeZip(t, "[Content_Types].xml", "word/document.xml"),
		"reports/page.html":   []byte("<html><script>alert(1)</script></html>"),
		"reports/prog.html":   []byte("MZ\x90\x00 this is a program"),
		"reports/prog.exe":    []byte("MZ\x90\x00 this is a program"),
		"reports/Run.desktop": []byte("[Desktop Entry]\nExec=sh"),
		"reports/x.lnk":       []byte("L\x00\x00\x00"),
		"reports/x.command":   []byte("#!/bin/sh\necho hi\n"),
		"reports/macro.docm":  officeZip(t, "[Content_Types].xml", "word/vbaProject.bin"),
		"reports/fake.pdf":    []byte("MZ this is a program"),
		"reports/macro.docx":  officeZip(t, "[Content_Types].xml", "word/vbaProject.bin"),
	}
	r, _ := deliverableRun(t, files)
	got := opened(r)
	for rel, want := range map[string]bool{
		"reports/ok.pdf": true, "reports/ok.docx": true, "reports/page.html": true,
		"reports/prog.html": false, "reports/prog.exe": false, "reports/Run.desktop": false, "reports/x.lnk": false, "reports/x.command": false,
		"reports/macro.docm": false, "reports/fake.pdf": false, "reports/macro.docx": false,
	} {
		before := len(*got)
		err := r.OpenRunDeliverable("run-1", rel)
		if (err == nil) != want {
			t.Errorf("OpenRunDeliverable(%s) = %v, want opened=%v", rel, err, want)
		}
		if opens := len(*got) > before; opens != want {
			t.Errorf("%s: the system was asked to open it = %v, want %v", rel, opens, want)
		}
	}
	// What was opened is never the run folder's own file.
	for _, p := range *got {
		if strings.Contains(p, "run-1") || strings.Contains(p, string(filepath.Separator)+"reports"+string(filepath.Separator)) {
			t.Errorf("opened %s, a file in the run folder", p)
		}
		if b, err := os.ReadFile(p); err != nil || len(b) == 0 {
			t.Errorf("the copy %s does not exist: %v", p, err)
		}
	}
	// The icons of a listing say so too.
	v, _ := r.RunDeliverables("run-1")
	notes := map[string]string{}
	for _, f := range v.Files {
		notes[f.Path] = f.NotOpened
	}
	if notes["reports/page.html"] != "" || notes["reports/prog.html"] != "contents" || notes["reports/prog.exe"] != "type" || notes["reports/fake.pdf"] != "contents" || notes["reports/ok.pdf"] != "" {
		t.Fatalf("NotOpened: %v", notes)
	}
}

func TestAPathTheRecordDoesNotNameIsNeverOpened(t *testing.T) {
	r, dir := deliverableRun(t, map[string][]byte{"reports/a.md": []byte("a")})
	writeRunFile(t, filepath.Join(dir, "reports", "unlisted.pdf"), "%PDF-1.7\n")
	writeRunFile(t, filepath.Join(dir, "outputs", "loose.pdf"), "%PDF-1.7\n")
	got := opened(r)
	for _, rel := range []string{"reports/unlisted.pdf", "outputs/loose.pdf", "../x.pdf", "/etc/passwd", "reports/../result.json", ""} {
		if err := r.OpenRunDeliverable("run-1", rel); err == nil {
			t.Errorf("OpenRunDeliverable(%q) opened a file the record does not name", rel)
		}
	}
	if len(*got) != 0 {
		t.Fatalf("the system was asked to open %v", *got)
	}
}

func TestAChangedFileIsNeitherListedNorOpenedNorShown(t *testing.T) {
	r, dir := deliverableRun(t, map[string][]byte{"reports/a.pdf": []byte("%PDF-1.7\noriginal\n")})
	writeRunFile(t, filepath.Join(dir, "reports", "a.pdf"), "%PDF-1.7\nswapped\n")
	got := opened(r)
	if err := r.OpenRunDeliverable("run-1", "reports/a.pdf"); err == nil || !strings.Contains(err.Error(), "recorded") {
		t.Fatalf("a changed file: %v", err)
	}
	if _, err := r.RunDeliverablePDF("run-1", "reports/a.pdf"); err == nil {
		t.Fatal("a changed PDF was handed to the viewer")
	}
	if _, err := r.RunDeliverableText("run-1", "reports/a.pdf"); err == nil {
		t.Fatal("a changed file was read as text")
	}
	if len(*got) != 0 {
		t.Fatalf("opened %v", *got)
	}
	if v, _ := r.RunDeliverables("run-1"); len(v.Files) != 0 || v.Withheld != 1 {
		t.Fatalf("listing %+v", v)
	}
}

func TestOpenRunDeliverableTakesTheCopyInTheDeliverablesFolderWhenItHoldsTheSameBytes(t *testing.T) {
	body := []byte("# Report\n")
	r, dir := deliverableRun(t, map[string][]byte{"reports/report.md": body})
	root, _, _, _ := deliverablesFolder()
	copyDir := filepath.Join(root, "weekly", "2026-10-17 1200 Report")
	copyPath := filepath.Join(copyDir, "report.md")
	writeRunFile(t, copyPath, string(body))
	// The envelope as the engine wrote it after copying.
	env := map[string]any{
		"agentry": "run/v1", "status": "ok", "ref": "acme/weekly@1.0.0",
		"deliverables": map[string]any{"files": []map[string]any{{"path": "reports/report.md", "title": "Report", "sha256": sum(body), "bytes": len(body), "marked": true}}},
		"deliveries":   []map[string]any{{"destination": root, "kind": "folder", "object": copyPath, "sha256": sum(body), "status": "delivered"}},
	}
	b, _ := json.Marshal(env)
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	got := opened(r)

	v, _ := r.RunDeliverables("run-1")
	if len(v.Files) != 1 || v.Files[0].CopiedTo != copyDir {
		t.Fatalf("CopiedTo = %+v, want %s", v.Files, copyDir)
	}
	if err := r.OpenRunDeliverable("run-1", "reports/report.md"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0] != copyPath {
		t.Fatalf("opened %v, want the copy %s", *got, copyPath)
	}
	// The person edited their copy: a click opens a fresh copy of the run's
	// bytes, not the file the envelope's path names.
	if err := os.WriteFile(copyPath, []byte("# My edits\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	*got = nil
	if err := r.OpenRunDeliverable("run-1", "reports/report.md"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0] == copyPath {
		t.Fatalf("an edited copy was opened: %v", *got)
	}
	if b, _ := os.ReadFile((*got)[0]); string(b) != string(body) {
		t.Fatalf("the fresh copy holds %q", b)
	}
	v, _ = r.RunDeliverables("run-1")
	if v.Files[0].CopiedTo != "" {
		t.Fatalf("an edited copy is listed as the run's copy: %+v", v.Files[0])
	}
}

func TestAPathTheEnvelopeNamesOutsideTheFolderIsNotOpenedAsACopy(t *testing.T) {
	body := []byte("# Report\n")
	r, dir := deliverableRun(t, map[string][]byte{"reports/report.md": body})
	elsewhere := filepath.Join(t.TempDir(), "report.md")
	writeRunFile(t, elsewhere, string(body))
	env := map[string]any{
		"agentry": "run/v1", "status": "ok",
		"deliverables": map[string]any{"files": []map[string]any{{"path": "reports/report.md", "sha256": sum(body), "bytes": len(body)}}},
		"deliveries":   []map[string]any{{"object": elsewhere, "sha256": sum(body), "status": "delivered"}},
	}
	b, _ := json.Marshal(env)
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	got := opened(r)
	if err := r.OpenRunDeliverable("run-1", "reports/report.md"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0] == elsewhere {
		t.Fatalf("opened %v, a path the envelope named outside the Deliverables folder", *got)
	}
}

func TestShowRunDeliverableShowsTheCopyElseTheRunFolder(t *testing.T) {
	body := []byte("# Report\n")
	r, dir := deliverableRun(t, map[string][]byte{"reports/report.md": body})
	var revealed, folder []string
	r.revealFile = func(p string) error { revealed = append(revealed, p); return nil }
	r.openFolder = func(d string) error { folder = append(folder, d); return nil }
	if err := r.ShowRunDeliverable("run-1", "reports/report.md"); err != nil {
		t.Fatal(err)
	}
	if len(revealed) != 0 || len(folder) != 1 || folder[0] != dir {
		t.Fatalf("with no copy: revealed %v, folder %v; want the run folder", revealed, folder)
	}
	root, _, _, _ := deliverablesFolder()
	copyPath := filepath.Join(root, "weekly", "d", "report.md")
	writeRunFile(t, copyPath, string(body))
	env := map[string]any{"agentry": "run/v1", "status": "ok",
		"deliverables": map[string]any{"files": []map[string]any{{"path": "reports/report.md", "sha256": sum(body), "bytes": len(body)}}},
		"deliveries":   []map[string]any{{"object": copyPath, "sha256": sum(body), "status": "delivered"}}}
	b, _ := json.Marshal(env)
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	revealed, folder = nil, nil
	if err := r.ShowRunDeliverable("run-1", "reports/report.md"); err != nil {
		t.Fatal(err)
	}
	if len(revealed) != 1 || revealed[0] != copyPath || len(folder) != 0 {
		t.Fatalf("with a copy: revealed %v, folder %v", revealed, folder)
	}
	// A type no click opens can still be shown.
	r2, _ := deliverableRun(t, map[string][]byte{"reports/page.exe": []byte("MZ")})
	r2.openFolder = func(string) error { return nil }
	if err := r2.ShowRunDeliverable("run-1", "reports/page.exe"); err != nil {
		t.Fatalf("Show in folder for a type that is not opened: %v", err)
	}
}

func TestTheViewerGetsMarkdownTextAndPDFOnlyWhenTheyMatch(t *testing.T) {
	pdf := []byte("%PDF-1.7\nx\n%%EOF\n")
	r, _ := deliverableRun(t, map[string][]byte{
		"reports/a.md": []byte("# A <b>bold</b>\n"), "reports/b.csv": []byte("x,y\n1,2\n"), "reports/c.pdf": pdf,
		"reports/d.docx": officeZip(t, "[Content_Types].xml"), "reports/e.html": []byte("<p>x</p>"),
		"reports/big.txt": bytes.Repeat([]byte("a"), maxViewBytes+1),
	})
	if v, err := r.RunDeliverableText("run-1", "reports/a.md"); err != nil || v.Kind != "markdown" || !strings.Contains(v.Text, "# A") {
		t.Fatalf("markdown: %+v %v", v, err)
	}
	if v, err := r.RunDeliverableText("run-1", "reports/b.csv"); err != nil || v.Kind != "text" || v.Text != "x,y\n1,2\n" {
		t.Fatalf("csv: %+v %v", v, err)
	}
	for _, rel := range []string{"reports/c.pdf", "reports/d.docx", "reports/e.html", "reports/big.txt", "reports/missing.md"} {
		if _, err := r.RunDeliverableText("run-1", rel); err == nil {
			t.Errorf("RunDeliverableText(%s) read it", rel)
		}
	}
	b64, err := r.RunDeliverablePDF("run-1", "reports/c.pdf")
	if got, _ := base64.StdEncoding.DecodeString(b64); err != nil || !bytes.Equal(got, pdf) {
		t.Fatalf("pdf: %v", err)
	}
	if _, err := r.RunDeliverablePDF("run-1", "reports/a.md"); err == nil {
		t.Fatal("a Markdown file went to the PDF viewer")
	}
}

func TestOpenDeliverablesFolderMakesItAndRefusesWhileOff(t *testing.T) {
	freshConfig(t)
	r := &RunAPI{app: &App{ctx: context.Background()}}
	var shown []string
	r.openFolder = func(d string) error { shown = append(shown, d); return nil }
	if err := r.OpenDeliverablesFolder(); err != nil {
		t.Fatal(err)
	}
	root, _, _, _ := deliverablesFolder()
	if len(shown) != 1 || shown[0] != root {
		t.Fatalf("shown %v, want %s", shown, root)
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		t.Fatalf("the folder was not made: %v", err)
	}
	if _, err := (&SettingsAPI{}).SetDeliverables(false); err != nil {
		t.Fatal(err)
	}
	shown = nil
	if err := r.OpenDeliverablesFolder(); err == nil || len(shown) != 0 {
		t.Fatalf("shown %v with the switch off, err %v", shown, err)
	}
}

func TestTheHomeCardReadsTheHistoryAndEachEnvelopeNeverTheDeliverablesFolder(t *testing.T) {
	freshConfig(t)
	now := time.Now().UTC()
	mk := func(id string, ended time.Time, files map[string][]byte) string {
		dir := t.TempDir()
		var list []map[string]any
		for rel, body := range files {
			writeRunFile(t, filepath.Join(dir, filepath.FromSlash(rel)), string(body))
			list = append(list, map[string]any{"path": rel, "title": rel, "sha256": sum(body), "bytes": len(body), "marked": true})
		}
		b, _ := json.Marshal(map[string]any{"agentry": "run/v1", "status": "ok", "deliverables": map[string]any{"files": list}})
		writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
		if err := runhistory.Add(runhistory.Entry{ID: id, Kind: runhistory.KindRun, Where: runhistory.WhereHere, Ref: "acme/" + id + "@1.0.0", Outcome: "ok", EndedAt: ended, Folder: dir}); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	mk("aaaa0001", now.Add(-3*time.Hour), map[string][]byte{"reports/old.md": []byte("old")})
	newest := mk("aaaa0002", now.Add(-1*time.Hour), map[string][]byte{"reports/new.md": []byte("new")})
	mk("aaaa0003", now.Add(-2*time.Hour), map[string][]byte{"reports/mid.md": []byte("mid")})
	// A build's own job is not a QuickAgent's deliverable.
	dir := t.TempDir()
	writeRunFile(t, filepath.Join(dir, "reports", "draft.md"), "draft")
	b, _ := json.Marshal(map[string]any{"agentry": "run/v1", "status": "ok", "deliverables": map[string]any{"files": []map[string]any{{"path": "reports/draft.md", "sha256": sum([]byte("draft")), "bytes": 5}}}})
	writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
	_ = runhistory.Add(runhistory.Entry{ID: "dddd0001", Kind: runhistory.KindDraft, Where: runhistory.WhereHere, Outcome: "ok", EndedAt: now, Folder: dir})
	// A file in the Deliverables folder that no run made is not listed.
	root, _, _, _ := deliverablesFolder()
	writeRunFile(t, filepath.Join(root, "stray.md"), "stray")
	// A newest run whose file changed after its digest.
	changed := mk("aaaa0004", now, map[string][]byte{"reports/changed.md": []byte("before")})
	writeRunFile(t, filepath.Join(changed, "reports", "changed.md"), "after")

	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.RecentDeliverables(5)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range v.Files {
		got = append(got, f.RunID+"/"+f.Name)
	}
	if want := "aaaa0002/new.md aaaa0003/mid.md aaaa0001/old.md"; strings.Join(got, " ") != want {
		t.Fatalf("the card lists %v, want %s", got, want)
	}
	if f := v.Files[0]; f.Ref != "acme/aaaa0002@1.0.0" || f.Where != "here" || f.EndedAt == "" {
		t.Fatalf("a row %+v", f)
	}
	_ = newest
	if v, _ := r.RecentDeliverables(2); len(v.Files) != 2 {
		t.Fatalf("limit 2 listed %d", len(v.Files))
	}
	// Remember recent runs off: nothing is remembered, so nothing is read.
	setRemember(t, false)
	v, _ = r.RecentDeliverables(5)
	if v.Remember || len(v.Files) != 0 {
		t.Fatalf("with the history off: %+v", v)
	}
}

// The card draws at most twelve icons, the newest first, however many a
// caller asks for, and five runs of which three left files are three icons.
func TestTheHomeCardListsAtMostTwelveFilesNewestFirst(t *testing.T) {
	freshConfig(t)
	now := time.Now().UTC()
	mk := func(id string, ended time.Time, files map[string][]byte) {
		dir := t.TempDir()
		var list []map[string]any
		for rel, body := range files {
			writeRunFile(t, filepath.Join(dir, filepath.FromSlash(rel)), string(body))
			list = append(list, map[string]any{"path": rel, "title": rel, "sha256": sum(body), "bytes": len(body), "marked": true})
		}
		b, _ := json.Marshal(map[string]any{"agentry": "run/v1", "status": "ok", "deliverables": map[string]any{"files": list}})
		writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
		if err := runhistory.Add(runhistory.Entry{ID: id, Kind: runhistory.KindRun, Where: runhistory.WhereHere, Ref: "acme/" + id + "@1.0.0", Outcome: "ok", EndedAt: ended, Folder: dir}); err != nil {
			t.Fatal(err)
		}
	}
	// Five runs, three of which left a file: two PDFs and a page.
	mk("bbbb0001", now.Add(-5*time.Hour), map[string][]byte{"reports/a.pdf": []byte("%PDF-1.7\n")})
	mk("bbbb0002", now.Add(-4*time.Hour), nil)
	mk("bbbb0003", now.Add(-3*time.Hour), map[string][]byte{"reports/b.html": []byte("<p>b</p>")})
	mk("bbbb0004", now.Add(-2*time.Hour), nil)
	mk("bbbb0005", now.Add(-1*time.Hour), map[string][]byte{"reports/c.pdf": []byte("%PDF-1.7\n")})
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.RecentDeliverables(12)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range v.Files {
		got = append(got, f.Name)
	}
	if want := "c.pdf b.html a.pdf"; strings.Join(got, " ") != want {
		t.Fatalf("five runs, three with files: %v, want %s", got, want)
	}
	if !v.Files[1].CanOpen || v.Files[1].ViewKind != "" {
		t.Fatalf("the page is a click that opens it in the system's app: %+v", v.Files[1])
	}
	for i := 0; i < 14; i++ {
		mk(fmt.Sprintf("cccc%04d", i), now.Add(time.Duration(i)*time.Minute), map[string][]byte{"reports/n.md": []byte("n")})
	}
	if v, _ = r.RecentDeliverables(50); len(v.Files) != 12 {
		t.Fatalf("a card of %d files, want at most 12", len(v.Files))
	}
	if v.Files[0].RunID != "cccc0013" {
		t.Fatalf("newest first: %s", v.Files[0].RunID)
	}
}

// One read of the card examines a bounded number of bytes: files past the
// budget are neither hashed nor listed, so an envelope naming many large
// files cannot keep Home busy.
func TestTheHomeCardExaminesABoundedNumberOfBytes(t *testing.T) {
	freshConfig(t)
	old := deliverableHashBudget
	deliverableHashBudget = 100
	t.Cleanup(func() { deliverableHashBudget = old })
	now := time.Now().UTC()
	for i, size := range []int{60, 60, 60} {
		dir := t.TempDir()
		body := bytes.Repeat([]byte("a"), size)
		writeRunFile(t, filepath.Join(dir, "reports", "n.md"), string(body))
		b, _ := json.Marshal(map[string]any{"agentry": "run/v1", "status": "ok", "deliverables": map[string]any{"files": []map[string]any{{"path": "reports/n.md", "sha256": sum(body), "bytes": size, "marked": true}}}})
		writeRunFile(t, filepath.Join(dir, "result.json"), string(b))
		if err := runhistory.Add(runhistory.Entry{ID: fmt.Sprintf("dddd%04d", i), Kind: runhistory.KindRun, Where: runhistory.WhereHere, Ref: "acme/x@1.0.0", Outcome: "ok", EndedAt: now.Add(-time.Duration(i) * time.Minute), Folder: dir}); err != nil {
			t.Fatal(err)
		}
	}
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.RecentDeliverables(12)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Files) != 1 {
		t.Fatalf("a budget of 100 bytes listed %d files of 60 bytes, want 1", len(v.Files))
	}
}

// Every QuickAgent run and chat on this computer is given the folder while
// the switch is on, and none when it is off.
func TestARunAndAChatAreGivenTheDeliverablesFolderWhileTheSwitchIsOn(t *testing.T) {
	// The test saves the chosen folder and the switch: its own settings, so
	// a repeat of the test starts from a fresh install.
	freshConfig(t)
	f := newKnowledgeFixture(t)
	r := f.k.run
	var asked []string
	r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		asked = append(asked, req.DeliverTo)
		return &run.Result{Status: "ok", RunID: req.RunID}, nil
	}
	runIt := func() {
		t.Helper()
		if _, err := r.RunAgent(f.reg.URL, "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	runIt()
	root, _, _, _ := deliverablesFolder()
	if len(asked) != 1 || asked[0] != root || !filepath.IsAbs(root) {
		t.Fatalf("a run was given %v, want %q", asked, root)
	}
	picked := filepath.Join(t.TempDir(), "Mine")
	st, _ := settings.Load()
	st.DeliverablesDir = picked
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	runIt()
	if asked[1] != picked {
		t.Fatalf("a run was given %q, want the chosen folder %q", asked[1], picked)
	}
	if _, err := (&SettingsAPI{}).SetDeliverables(false); err != nil {
		t.Fatal(err)
	}
	runIt()
	if asked[2] != "" {
		t.Fatalf("a run with the switch off was given %q", asked[2])
	}
}

func TestDeliverablesSettingsViewAndSwitch(t *testing.T) {
	freshConfig(t)
	s := &SettingsAPI{app: &App{ctx: context.Background()}}
	v, err := s.RunDefaults()
	if err != nil {
		t.Fatal(err)
	}
	root, _, _, _ := deliverablesFolder()
	if !v.Deliverables || !v.DeliverablesDefault || v.DeliverablesDir != root {
		t.Fatalf("a fresh install: %+v", v)
	}
	picked := t.TempDir()
	s.app.pickDirectory = func(context.Context, string) (string, error) { return picked, nil }
	if v, err = s.ChooseDeliverablesDir(); err != nil || v.DeliverablesDir != picked || v.DeliverablesDefault {
		t.Fatalf("after choosing: %+v %v", v, err)
	}
	// A cancelled picker changes nothing.
	s.app.pickDirectory = func(context.Context, string) (string, error) { return "", nil }
	if v, err = s.ChooseDeliverablesDir(); err != nil || v.DeliverablesDir != picked {
		t.Fatalf("after cancelling: %+v %v", v, err)
	}
	if v, err = s.SetDeliverables(false); err != nil || v.Deliverables || v.DeliverablesDir != picked {
		t.Fatalf("switched off: %+v %v", v, err)
	}
	if v, err = s.UseDefaultDeliverablesDir(); err != nil || !v.DeliverablesDefault || v.DeliverablesDir != root || v.Deliverables {
		t.Fatalf("back to the default: %+v %v", v, err)
	}
}

// A run In Agentry whose output comes down is copied to the Deliverables
// folder, once, through the same code `agentry deliver` is; opening it again
// copies nothing more, and with the switch off nothing is copied.
func TestADownloadedRunInAgentryIsCollectedOnce(t *testing.T) {
	body := "# Report\n"
	env, _ := json.Marshal(map[string]any{
		"agentry": "run/v1", "status": "ok", "ref": "acme/reporter@1.0.0", "result": "the summary", "runId": "22222222-2222-2222-2222-222222222222",
		"deliverables": map[string]any{"files": []map[string]any{{"path": "reports/report.md", "title": "Report", "sha256": sum([]byte(body)), "bytes": len(body), "marked": true}}},
	})
	f := &hostedFake{enabled: true, outputs: map[string]string{"result.json": string(env), "reports/report.md": body}}
	f.run = &hostedrun.Run{ID: "22222222-2222-2222-2222-222222222222", Ref: "acme/reporter@1.0.0", Mode: "autonomous",
		Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeSucceeded, CreatedAt: time.Now().UTC().Add(-time.Hour)}
	reg := f.start(t)
	signIn(t, reg)
	t.Setenv(policy.HomeEnv, t.TempDir())
	isolateDeliverables(t)
	r, _ := newRunAPI(t)
	f.finish(hostedrun.OutcomeSucceeded, "ok", 2)

	v, err := r.DownloadHostedOutput(reg, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, _, _, _ := deliverablesFolder()
	countCopies := func() int {
		n := 0
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	if n := countCopies(); n != 1 {
		t.Fatalf("%d copies after the download, want 1 (folder %s)", n, root)
	}
	// Opening the run page again copies nothing more.
	if _, err := r.HostedRecord(reg, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if n := countCopies(); n != 1 {
		t.Fatalf("%d copies after opening it again, want 1", n)
	}
	// The result lists the file and says where its copy is.
	d, err := r.RunDeliverables(f.run.ID)
	if err != nil || len(d.Files) != 1 || d.Files[0].CopiedTo == "" {
		t.Fatalf("the downloaded run's deliverables: %+v %v", d, err)
	}
	_ = v

	// With the switch off, a download copies nothing.
	root2 := filepath.Join(t.TempDir(), "other")
	st, _ := settings.Load()
	off := false
	st.Deliverables, st.DeliverablesDir = &off, root2
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
	writeRunFile(t, filepath.Join(dir2, "reports", "report.md"), body)
	writeRunFile(t, filepath.Join(dir2, "result.json"), string(env))
	collectDownloaded(dir2)
	if _, err := os.Stat(root2); err == nil {
		t.Fatal("a download copied with the switch off")
	}
}

func TestAChatIsGivenTheDeliverablesFolderToo(t *testing.T) {
	freshConfig(t)
	app, _ := runDefaultsApp(t, "")
	var asked []string
	r := &RunAPI{app: app, runnerHomeDir: t.TempDir(), newRunID: func() (string, error) { return "ab12cd34", nil }}
	r.engineSession = func(_ context.Context, req run.Request, _ io.Writer) (localSession, error) {
		asked = append(asked, req.DeliverTo)
		events := make(chan session.Event)
		close(events)
		return &fakeChat{f: &fakeEngine{}, id: req.RunID, events: events}, nil
	}
	chat := func() {
		t.Helper()
		if _, err := r.StartSession("https://r.example", "acme/agent@1.0.0", "", nil, "", nil, -1, false, nil, false, false, nil, false, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := r.EndSession(false); err != nil {
			t.Fatal(err)
		}
	}
	chat()
	root, _, _, _ := deliverablesFolder()
	if len(asked) != 1 || asked[0] != root || root == "" {
		t.Fatalf("a chat was given %v, want %q", asked, root)
	}
	if _, err := (&SettingsAPI{}).SetDeliverables(false); err != nil {
		t.Fatal(err)
	}
	chat()
	if asked[1] != "" {
		t.Fatalf("a chat with the switch off was given %q", asked[1])
	}
}

func TestTheViewersSayWhyTheyRefuse(t *testing.T) {
	r, _ := deliverableRun(t, map[string][]byte{"reports/a.md": []byte("# A\n"), "reports/big.txt": bytes.Repeat([]byte("a"), maxViewBytes+1)})
	if _, err := r.RunDeliverablePDF("run-1", "reports/a.md"); err == nil || !strings.Contains(err.Error(), "is not a PDF") {
		t.Fatalf("a Markdown file to the PDF viewer: %v", err)
	}
	if _, err := r.RunDeliverableText("run-1", "reports/big.txt"); err == nil || !strings.Contains(err.Error(), "too large to show here") {
		t.Fatalf("a text file over the viewer's limit: %v", err)
	}
}
