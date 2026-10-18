package deliverables

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
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

	if p, ok := FindCopy(root, rec, f); !ok || p != copyPath {
		t.Fatalf("FindCopy = %q, %v; want %q", p, ok, copyPath)
	}
	// The person edited their copy: it is theirs now, and not the run's bytes.
	if err := os.WriteFile(copyPath, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindCopy(root, rec, f); ok {
		t.Fatal("an edited copy was taken for the run's file")
	}
	// A path the envelope names outside the folder, whatever it holds.
	outside := filepath.Join(t.TempDir(), "elsewhere.md")
	_ = os.WriteFile(outside, []byte("# Summary\n"), 0o644)
	forged := Record{Files: []File{f}, Deliveries: []Delivery{{Object: outside, SHA256: f.SHA256, Status: "delivered"}}}
	if _, ok := FindCopy(root, forged, f); ok {
		t.Fatal("a copy outside the Deliverables folder was found")
	}
	forged.Deliveries[0].Object = filepath.Join(root, "..", filepath.Base(filepath.Dir(outside)), "elsewhere.md")
	if _, ok := FindCopy(root, forged, f); ok {
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
	if _, ok := FindCopy(root, rec, f); ok {
		t.Fatal("a copy reached through a link was found")
	}
}

func TestOpenCopyMakesAFreshCopyUnderAPlainName(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/Q3 report: final?.pdf", pdfBytes())
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
