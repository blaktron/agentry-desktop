package deliverables

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// snapshot is every file under dir by relative path and contents.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func names(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestCollectCopiesAfterTheDigestAndLeavesTheRunFolderAsItWas(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	r.put("reports/report.pdf", pdfBytes())
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	before := snapshot(t, r.dir)

	got, err := Collect(r.dir, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d records, want 2: %+v", len(got), got)
	}
	copies := snapshot(t, root)
	if len(copies) != 2 {
		t.Fatalf("copies = %v", names(copies))
	}
	for name := range copies {
		if !strings.HasPrefix(name, "weekly/") {
			t.Errorf("%s is not under the QuickAgent's name", name)
		}
	}
	// The run folder is the proof: nothing in it changes but the record of
	// what was copied.
	after := snapshot(t, r.dir)
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed in the run folder", k)
		}
	}
	for k := range after {
		if _, was := before[k]; !was && k != "receipts/deliveries.ndjson" {
			t.Errorf("%s was added to the run folder", k)
		}
	}
	if !strings.Contains(after["receipts/deliveries.ndjson"], `"delivered"`) {
		t.Errorf("the ledger does not say what was copied: %q", after["receipts/deliveries.ndjson"])
	}
}

func TestCollectingAgainCopiesNothingMore(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	if _, err := Collect(r.dir, root); err != nil {
		t.Fatal(err)
	}
	again, err := Collect(r.dir, root)
	if err != nil || len(again) != 0 {
		t.Fatalf("a second collect = %+v, %v; want nothing", again, err)
	}
	if n := len(snapshot(t, root)); n != 1 {
		t.Fatalf("%d copies after two collects, want 1", n)
	}
}

func TestCollectRefusesAChangedFileAndAPathOutsideReports(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/summary.md", []byte("# Summary\n"))
	r.put("outputs/loose.md", []byte("not a report"))
	r.envelope(nil)
	if err := os.WriteFile(filepath.Join(r.dir, f.Path), []byte("# Something else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Agentry")
	got, err := Collect(r.dir, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if d.Status == "delivered" {
			t.Errorf("delivered %+v", d)
		}
	}
	if n := len(snapshot(t, root)); n != 0 {
		t.Fatalf("a refused file left %d copies", n)
	}
}

func TestCollectOfARunThatFailedOrListsNothingDoesNothing(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("x"))
	r.envelope(map[string]any{"status": "error"})
	root := filepath.Join(t.TempDir(), "Agentry")
	if got, err := Collect(r.dir, root); err != nil || len(got) != 0 {
		t.Fatalf("a failed run: %+v, %v", got, err)
	}
	empty := t.TempDir()
	if got, err := Collect(empty, root); err != nil || len(got) != 0 {
		t.Fatalf("a folder with no envelope: %+v, %v", got, err)
	}
	if got, err := Collect(r.dir, ""); err != nil || len(got) != 0 {
		t.Fatalf("no folder to collect to: %+v, %v", got, err)
	}
	if _, err := os.Stat(root); err == nil {
		t.Fatal("the Deliverables folder was made with nothing to put in it")
	}
}

func TestFindCopyHoldsACopyToTheRunsBytes(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/summary.md", []byte("# Summary\n"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	if _, err := Collect(r.dir, root); err != nil {
		t.Fatal(err)
	}
	// What the envelope would say after the desktop collected it.
	var copyPath string
	for name := range snapshot(t, root) {
		copyPath = filepath.Join(root, filepath.FromSlash(name))
	}
	rec := Record{Files: []File{f}, Deliveries: []Delivery{{Object: copyPath, SHA256: f.SHA256, Status: "delivered"}}}

	if p, ok := FindCopy(r.dir, root, rec, f); !ok || p != copyPath {
		t.Fatalf("FindCopy = %q, %v; want %q", p, ok, copyPath)
	}
	// The person edited their copy: it is theirs now, and not the run's bytes.
	if err := os.WriteFile(copyPath, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindCopy(r.dir, root, rec, f); ok {
		t.Fatal("an edited copy was taken for the run's file")
	}
	// A path the envelope names outside the folder, whatever it holds.
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	_ = os.WriteFile(outside, []byte("# Summary\n"), 0o644)
	forged := Record{Files: []File{f}, Deliveries: []Delivery{{Object: outside, SHA256: f.SHA256, Status: "delivered"}}}
	if _, ok := FindCopy(r.dir, root, forged, f); ok {
		t.Fatal("a copy outside the Deliverables folder was found")
	}
	forged.Deliveries[0].Object = filepath.Join(root, "..", filepath.Base(filepath.Dir(outside)), "elsewhere.md")
	if _, ok := FindCopy(r.dir, root, forged, f); ok {
		t.Fatal("a path that climbs out of the folder was found")
	}
}

func TestFindCopyDoesNotFollowALinkInsideTheFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links need privileges on Windows")
	}
	r := newRun(t)
	f := r.put("reports/summary.md", []byte("# Summary\n"))
	root := t.TempDir()
	elsewhere := t.TempDir()
	_ = os.WriteFile(filepath.Join(elsewhere, "summary.md"), []byte("# Summary\n"), 0o644)
	if err := os.Symlink(elsewhere, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	rec := Record{Deliveries: []Delivery{{Object: filepath.Join(root, "linked", "summary.md"), SHA256: f.SHA256, Status: "delivered"}}}
	if _, ok := FindCopy(r.dir, root, rec, f); ok {
		t.Fatal("a copy reached through a link was found")
	}
}

func TestOpenCopyMakesAFreshCopyUnderAPlainName(t *testing.T) {
	r := newRun(t)
	name := "reports/Q3 report: final?.pdf"
	if runtime.GOOS == "windows" {
		name = "reports/Q3 report - final.pdf" // Windows cannot make the other name
	}
	f := r.put(name, pdfBytes())
	r.envelope(nil)
	temp := t.TempDir()
	p, err := OpenCopy(r.dir, temp, "weekly", f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, temp) || strings.ContainsAny(filepath.Base(p), `:?*"<>|`) || !strings.HasSuffix(p, ".pdf") {
		t.Fatalf("the copy is %q", p)
	}
	if got, _ := os.ReadFile(p); string(got) != string(pdfBytes()) {
		t.Fatal("the copy holds other bytes")
	}
	// A file whose contents changed after the digest was taken is not copied.
	if err := os.WriteFile(filepath.Join(r.dir, f.Path), []byte("%PDF-changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenCopy(r.dir, t.TempDir(), "weekly", f); err == nil {
		t.Fatal("a changed file was copied to open")
	}
}

// Two files with the same bytes and different types: a click on one is never
// given the other's copy.
func TestFindCopyNeverHandsOverACopyOfAnotherType(t *testing.T) {
	r := newRun(t)
	body := []byte("echo hi\n")
	bat := r.put("reports/a.bat", body)
	txt := r.put("reports/a.txt", body)
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	got, err := Collect(r.dir, root)
	if err != nil || len(got) != 2 {
		t.Fatalf("Collect = %+v, %v", got, err)
	}
	var ds []Delivery
	for _, d := range got { // the .bat first, as an envelope may list it
		ds = append(ds, Delivery{Object: d.Object, SHA256: d.SHA256, Status: d.Status})
	}
	sort.Slice(ds, func(i, j int) bool {
		return strings.HasSuffix(ds[i].Object, ".bat") && !strings.HasSuffix(ds[j].Object, ".bat")
	})
	rec := Record{Files: []File{bat, txt}, Deliveries: ds}
	p, ok := FindCopy(r.dir, root, rec, txt)
	if !ok || !strings.HasSuffix(p, ".txt") {
		t.Fatalf("the copy for a.txt is %q, %v; want a .txt", p, ok)
	}
}

// A Deliverables folder that holds the run folders does not make a run's own
// file its copy.
func TestFindCopyNeverReturnsTheRunFoldersOwnFile(t *testing.T) {
	parent := t.TempDir()
	runDir := filepath.Join(parent, "run-1")
	r := &runFolder{t: t, dir: runDir}
	f := r.put("reports/a.md", []byte("# A\n"))
	rec := Record{Files: []File{f}, Deliveries: []Delivery{{Object: filepath.Join(runDir, "reports", "a.md"), SHA256: f.SHA256, Status: "delivered"}}}
	if p, ok := FindCopy(runDir, parent, rec, f); ok {
		t.Fatalf("the run folder's own file %s was taken for a copy", p)
	}
}

func TestTwoCollectionsOfOneRunCopyItOnce(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Collect(r.dir, root); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := len(snapshot(t, root)); n != 1 {
		t.Fatalf("%d copies after six collections at once, want 1", n)
	}
	if lines := strings.Count(snapshot(t, r.dir)["receipts/deliveries.ndjson"], "\n"); lines != 1 {
		t.Fatalf("the ledger has %d lines, want 1", lines)
	}
}

func TestWhatIsPendingIsReadPerDestination(t *testing.T) {
	a := File{Path: "reports/a.md", SHA256: strings.Repeat("a", 64)}
	b := File{Path: "reports/b.md", SHA256: strings.Repeat("b", 64)}
	led := []receipt.Delivery{
		{Destination: "/other", SHA256: a.SHA256, Status: "delivered"},
		{Destination: "/mine", SHA256: b.SHA256, Status: "delivered"},
	}
	got := pendingFiles([]File{a, b}, led, "/mine")
	if len(got) != 1 || got[0].Path != a.Path {
		t.Fatalf("pending = %+v, want only a.md: a copy for another folder does not count, and b.md is done", got)
	}
	led = append(led, receipt.Delivery{Destination: "/mine", SHA256: a.SHA256, Status: "refused"}, receipt.Delivery{Destination: "/mine", SHA256: a.SHA256, Status: "failed"})
	if got := pendingFiles([]File{a, b}, led, "/mine"); len(got) != 0 {
		t.Fatalf("pending = %+v after two failed tries, want none", got)
	}
}

func TestACollectionOfAChangedFileIsTriedTwiceAndThenLeftAlone(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/summary.md", []byte("# Summary\n"))
	r.envelope(nil)
	if err := os.WriteFile(filepath.Join(r.dir, f.Path), []byte("# Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "Agentry")
	for range 5 {
		if _, err := Collect(r.dir, root); err != nil {
			t.Fatal(err)
		}
	}
	if lines := strings.Count(snapshot(t, r.dir)["receipts/deliveries.ndjson"], "\n"); lines != 2 {
		t.Fatalf("the ledger has %d lines after five collections of a changed file, want 2", lines)
	}
}

func TestCollectCopiesOnlyWhatTheRecordOffers(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	r.put("reports/browser/shot.png", []byte("\x89PNG\r\n\x1a\n"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	if _, err := Collect(r.dir, root); err != nil {
		t.Fatal(err)
	}
	for name := range snapshot(t, root) {
		if strings.HasSuffix(name, "shot.png") {
			t.Fatalf("a browsing run's capture was copied: %s", name)
		}
	}
	if n := len(snapshot(t, root)); n != 1 {
		t.Fatalf("%d copies, want 1", n)
	}
}

func TestACheckHoldsTheFileToTheSizeTheEnvelopeNames(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/a.md", []byte("# A\n"))
	f.Bytes += 7
	if _, err := Check(r.dir, f); !errors.Is(err, ErrChanged) {
		t.Fatalf("a wrong size: err = %v, want ErrChanged", err)
	}
}

// A copy that could not be flagged as a download is not opened when the program
// that opens it treats a flagged file with more care: Office, a table, a web page.
func TestATypeThatNeedsTheDownloadFlagIsNamed(t *testing.T) {
	for _, ext := range []string{"docx", "xlsx", "pptx", "csv", "html", "htm"} {
		if !needsDownloadFlag(ext) {
			t.Errorf("%s is opened without the download flag", ext)
		}
	}
	for _, ext := range []string{"pdf", "png", "md", "txt", "json"} {
		if needsDownloadFlag(ext) {
			t.Errorf("%s needs the download flag", ext)
		}
	}
}

// A copy the system could not flag as a download is never opened, whichever
// other line of the record names the same file as delivered.
func TestFindCopySkipsAPathAnyLineSaysWasNotFlagged(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/page.html", []byte("<p>x</p>"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	if _, err := Collect(r.dir, root); err != nil {
		t.Fatal(err)
	}
	var copyPath string
	for name := range snapshot(t, root) {
		copyPath = filepath.Join(root, filepath.FromSlash(name))
	}
	unflagged := Delivery{Object: copyPath, SHA256: f.SHA256, Status: "delivered", Error: receipt.DeliveryErrNotMarked}
	plain := Delivery{Object: copyPath, SHA256: f.SHA256, Status: "delivered"}
	for _, ds := range [][]Delivery{{unflagged, plain}, {plain, unflagged}} {
		rec := Record{Files: []File{f}, Deliveries: ds}
		if p, ok := FindCopy(r.dir, root, rec, f); ok {
			t.Fatalf("a copy that was not flagged was taken: %s", p)
		}
	}
}

// A file named only by its extension is copied under a name with none, which
// the system would open as a program of its own choosing: it is not opened.
func TestOpenCopyRefusesACopyWhoseTypeIsNotTheFiles(t *testing.T) {
	r := newRun(t)
	for _, name := range []string{"reports/.html", "reports/.pdf"} {
		f := r.put(name, []byte("<p>x</p>"))
		temp := t.TempDir()
		if p, err := OpenCopy(r.dir, temp, "agent", f); err == nil {
			t.Errorf("%s: a copy named %s was offered to open", name, p)
		}
	}
	f := r.put("reports/ok.html", []byte("<p>x</p>"))
	if p, err := OpenCopy(r.dir, t.TempDir(), "agent", f); err != nil || !strings.HasSuffix(p, ".html") {
		t.Fatalf("a plain page: %q, %v", p, err)
	}
}
