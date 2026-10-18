package deliverables

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// runFolder is a run's output folder holding the files named, and an envelope
// that lists each of them as a deliverable.
type runFolder struct {
	t     *testing.T
	dir   string
	files []File
}

func newRun(t *testing.T) *runFolder {
	t.Helper()
	return &runFolder{t: t, dir: t.TempDir()}
}

func (r *runFolder) put(rel string, b []byte) File {
	r.t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		r.t.Fatal(err)
	}
	f := File{Path: rel, Title: "Title of " + filepath.Base(rel), SHA256: digest(b), Bytes: int64(len(b)), Marked: true}
	r.files = append(r.files, f)
	return f
}

// envelope writes result.json listing the files put so far, with extra
// top-level fields merged in.
func (r *runFolder) envelope(extra map[string]any) {
	r.t.Helper()
	env := map[string]any{
		"agentry": "run/v1", "status": "ok", "runId": "0a1b2c3d4e5f", "ref": "acme/weekly@1.0.0",
		"deliverables": map[string]any{"files": r.files},
	}
	for k, v := range extra {
		env[k] = v
	}
	b, err := json.Marshal(env)
	if err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, "result.json"), b, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func pdfBytes() []byte { return []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n") }
func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("<x/>"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadRecordListsWhatTheEnvelopeNamesAndWithholdsWhatItShouldNot(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	good := sha256.Sum256([]byte("x"))
	r.files = append(r.files,
		File{Path: "outputs/loose.pdf", SHA256: hex.EncodeToString(good[:])},        // not under reports/
		File{Path: "reports/../result.json", SHA256: hex.EncodeToString(good[:])},   // climbs
		File{Path: `reports\win.pdf`, SHA256: hex.EncodeToString(good[:])},          // a separator the runner never writes
		File{Path: "reports/browser/shot.png", SHA256: hex.EncodeToString(good[:])}, // a browsing run's capture
		File{Path: "reports/nodigest.md", SHA256: "abc"},                            // not a SHA-256
		File{Path: "reports/summary.md", SHA256: hex.EncodeToString(good[:])},       // the same path twice
		File{Path: "/etc/passwd", SHA256: hex.EncodeToString(good[:])},
	)
	r.envelope(nil)
	rec := ReadRecord(r.dir)
	if len(rec.Files) != 1 || rec.Files[0].Path != "reports/summary.md" {
		t.Fatalf("listed %+v, want only reports/summary.md", rec.Files)
	}
	if rec.Withheld != 7 {
		t.Fatalf("withheld %d, want 7", rec.Withheld)
	}
}

func TestReadRecordOfAFolderWithNoEnvelopeListsNothingAndNeverNil(t *testing.T) {
	for _, setup := range []func(dir string){
		func(string) {},
		func(dir string) { _ = os.WriteFile(filepath.Join(dir, "result.json"), []byte("not json"), 0o644) },
		func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"deliverables":null}`), 0o644)
		},
		func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"deliverables":{"files":null}}`), 0o644)
		},
	} {
		dir := t.TempDir()
		setup(dir)
		rec := ReadRecord(dir)
		if rec.Files == nil || rec.Deliveries == nil || len(rec.Files) != 0 {
			t.Fatalf("record = %+v, want empty lists that are not nil", rec)
		}
		b, _ := json.Marshal(rec.Files)
		if string(b) != "[]" {
			t.Fatalf("an empty list marshals to %s", b)
		}
	}
}

func TestReadRecordIgnoresAnEnvelopeThatIsALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links need privileges on Windows")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.json")
	_ = os.WriteFile(outside, []byte(`{"deliverables":{"files":[{"path":"reports/a.md","sha256":"`+digest(nil)+`"}]}}`), 0o644)
	if err := os.Symlink(outside, filepath.Join(dir, "result.json")); err != nil {
		t.Fatal(err)
	}
	if rec := ReadRecord(dir); len(rec.Files) != 0 {
		t.Fatalf("an envelope reached through a link listed %+v", rec.Files)
	}
}

func TestCheckHoldsAFileToTheEnvelopesDigest(t *testing.T) {
	r := newRun(t)
	f := r.put("reports/report.pdf", pdfBytes())
	c, err := Check(r.dir, f)
	if err != nil || c.TypeErr != nil {
		t.Fatalf("Check = %+v, %v; want a file one click opens", c, err)
	}
	// The agent's file changes after the runner took its digest.
	if err := os.WriteFile(filepath.Join(r.dir, "reports/report.pdf"), append(pdfBytes(), 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(r.dir, f); !errors.Is(err, ErrChanged) {
		t.Fatalf("a changed file: err = %v, want ErrChanged", err)
	}
	if _, err := ReadChecked(r.dir, f, 1<<20); !errors.Is(err, ErrChanged) {
		t.Fatalf("ReadChecked of a changed file: err = %v, want ErrChanged", err)
	}
	if _, err := Check(r.dir, File{Path: "reports/gone.pdf", SHA256: f.SHA256}); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a missing file: err = %v, want ErrUnreadable", err)
	}
}

func TestCheckRefusesALinkAnywhereInThePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links need privileges on Windows")
	}
	r := newRun(t)
	secret := filepath.Join(t.TempDir(), "secret.md")
	_ = os.WriteFile(secret, []byte("private notes"), 0o644)
	if err := os.MkdirAll(filepath.Join(r.dir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The file itself is a link to a file elsewhere, with that file's digest.
	if err := os.Symlink(secret, filepath.Join(r.dir, "reports/linked.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(r.dir, File{Path: "reports/linked.md", SHA256: digest([]byte("private notes"))}); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a linked file: err = %v, want ErrUnreadable", err)
	}
	// A folder inside reports/ that is a link.
	elsewhere := t.TempDir()
	_ = os.WriteFile(filepath.Join(elsewhere, "inside.md"), []byte("x"), 0o644)
	if err := os.Symlink(elsewhere, filepath.Join(r.dir, "reports/dir")); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(r.dir, File{Path: "reports/dir/inside.md", SHA256: digest([]byte("x"))}); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("a file through a linked folder: err = %v, want ErrUnreadable", err)
	}
}

func TestCheckSaysWhatOneClickMayOpen(t *testing.T) {
	r := newRun(t)
	nul := []byte("text\x00more")
	cases := []struct {
		name string
		body []byte
		open bool
	}{
		{"reports/a.pdf", pdfBytes(), true},
		{"reports/a.PDF", pdfBytes(), true},
		{"reports/a.md", []byte("# hi\n"), true},
		{"reports/a.txt", []byte("héllo"), true},
		{"reports/a.csv", []byte("a,b\n1,2\n"), true},
		{"reports/a.json", []byte(`{"a":1}`), true},
		{"reports/a.png", []byte("\x89PNG\r\n\x1a\nrest"), true},
		{"reports/a.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0}, true},
		{"reports/a.jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0}, true},
		{"reports/a.docx", zipOf(t, "[Content_Types].xml", "word/document.xml"), true},
		{"reports/a.xlsx", zipOf(t, "[Content_Types].xml", "xl/workbook.xml"), true},
		{"reports/a.pptx", zipOf(t, "[Content_Types].xml", "ppt/presentation.xml"), true},
		// A file named for a type that is not what its bytes are.
		{"reports/fake.pdf", []byte("MZ\x90\x00 an executable"), false},
		{"reports/fake.png", pdfBytes(), false},
		{"reports/fake.docx", []byte("just text"), false},
		{"reports/fake.txt", nul, false},
		{"reports/fake.json", []byte("<html></html>"), false},
		// An Office file that carries code or a control.
		{"reports/macro.docx", zipOf(t, "[Content_Types].xml", "word/vbaProject.bin"), false},
		{"reports/embed.xlsx", zipOf(t, "[Content_Types].xml", "xl/embeddings/oleObject1.bin"), false},
		{"reports/nocontent.pptx", zipOf(t, "ppt/presentation.xml"), false},
		// Types on no list.
		{"reports/page.html", []byte("<html></html>"), false},
		{"reports/Run.desktop", []byte("[Desktop Entry]\nExec=sh"), false},
		{"reports/x.lnk", []byte("L\x00\x00\x00"), false},
		{"reports/x.command", []byte("#!/bin/sh\n"), false},
		{"reports/macro.docm", zipOf(t, "[Content_Types].xml"), false},
		{"reports/noext", []byte("text"), false},
		{"reports/x.pdf.exe", []byte("MZ"), false},
	}
	for _, c := range cases {
		f := r.put(c.name, c.body)
		got, err := Check(r.dir, f)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if opens := got.TypeErr == nil; opens != c.open {
			t.Errorf("%s: opens = %v (%v), want %v", c.name, opens, got.TypeErr, c.open)
		}
	}
}

func TestTextThatEndsInHalfACharacterStillCounts(t *testing.T) {
	b := []byte(strings.Repeat("a", HeadBytes-1) + "é") // cut inside the two-byte character
	if err := CheckHead("txt", b[:HeadBytes]); err != nil {
		t.Fatalf("a head cut mid-character: %v", err)
	}
	if err := CheckHead("txt", []byte("ok\xff\xfeok")); err == nil {
		t.Fatal("invalid UTF-8 in the middle passed as text")
	}
}

func TestEveryTypeOnTheListHasACheck(t *testing.T) {
	for _, ext := range Allowed() {
		if !CanOpen(ext) {
			t.Errorf("%s is listed but not opened", ext)
		}
		if err := CheckHead(ext, []byte("\x00\x01\x02 nothing a type starts with")); err == nil {
			t.Errorf("%s: arbitrary bytes passed its check", ext)
		}
	}
	if CanOpen("") || CanOpen("html") || CanOpen("exe") {
		t.Fatal("a type that is on no list is opened")
	}
}

func TestViewKinds(t *testing.T) {
	want := map[string]string{"pdf": "pdf", "md": "markdown", "txt": "text", "csv": "text", "json": "text", "png": "", "docx": "", "html": ""}
	for ext, k := range want {
		if got := ViewKind(ext); got != k {
			t.Errorf("ViewKind(%s) = %q, want %q", ext, got, k)
		}
	}
}

func TestUserDirsDocuments(t *testing.T) {
	home := "/home/ada"
	dir := t.TempDir()
	cases := []struct {
		name, env, file, want string
	}{
		{"the environment wins", "/data/Docs", `XDG_DOCUMENTS_DIR="$HOME/Papers"`, "/data/Docs"},
		{"the user-dirs file", "", "# comment\nXDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_DOCUMENTS_DIR=\"$HOME/Papers\"\n", "/home/ada/Papers"},
		{"an absolute value", "", `XDG_DOCUMENTS_DIR="/mnt/docs"`, "/mnt/docs"},
		{"a relative environment value is not used", "Docs", "", "/home/ada/Documents"},
		{"nothing set", "", "", "/home/ada/Documents"},
		{"a command in the value is not run or used", "", `XDG_DOCUMENTS_DIR="$(touch /tmp/x)"`, "/home/ada/Documents"},
	}
	for _, c := range cases {
		cfg := filepath.Join(dir, strings.ReplaceAll(c.name, " ", "_"))
		_ = os.MkdirAll(cfg, 0o755)
		if c.file != "" {
			_ = os.WriteFile(filepath.Join(cfg, "user-dirs.dirs"), []byte(c.file), 0o644)
		}
		getenv := func(k string) string {
			switch k {
			case "XDG_DOCUMENTS_DIR":
				return c.env
			case "XDG_CONFIG_HOME":
				return cfg
			}
			return ""
		}
		got, err := unixDocuments("linux", getenv, func() (string, error) { return home, nil })
		if err != nil || filepath.ToSlash(got) != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	if got, _ := unixDocuments("darwin", func(string) string { return "/ignored" }, func() (string, error) { return "/Users/ada", nil }); filepath.ToSlash(got) != "/Users/ada/Documents" {
		t.Errorf("macOS: %q, want ~/Documents", got)
	}
	if _, err := unixDocuments("linux", func(string) string { return "" }, func() (string, error) { return "", errors.New("no home") }); err == nil {
		t.Error("no home folder answered a Documents folder")
	}
}

func TestDefaultRootIsAnAgentryFolderInDocuments(t *testing.T) {
	root, err := DefaultRoot()
	if err != nil {
		t.Skipf("this machine names no Documents folder: %v", err)
	}
	if filepath.Base(root) != FolderName || !filepath.IsAbs(root) {
		t.Fatalf("DefaultRoot = %q", root)
	}
}
