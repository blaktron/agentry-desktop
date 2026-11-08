package designer

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/reportdesign"

	"github.com/blaktron/agentry-cli/pkg/reportdesigner/build"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/progress"
)

// The inputs are held to the run's own rules before anything is read: a
// brief within 4 KiB, a Design's references and addresses within their
// counts and each address as the CLI parses it, and a Cover written for a
// design, with no logo or addresses of its own.
func TestTheInputsAreCheckedBeforeTheStart(t *testing.T) {
	refs := []string{"a.png", "b.png", "c.png", "d.png"}
	cases := []struct {
		name   string
		job    string
		brief  string
		from   bool
		logo   string
		images []string
		urls   []string
		want   string
		// document is the case's document, "" for none.
		document string
	}{
		{"a brief alone", JobDesign, "Quarterly reports for a harbour trust.", false, "", nil, nil, "", ""},
		{"everything a design takes", JobDesign, "b", true, "logo.png", refs, []string{"https://example.com/", " ", "https://example.org/about"}, "", ""},
		{"no brief", JobDesign, "  \n", false, "", nil, nil, "works from a brief", ""},
		{"a brief over 4 KiB", JobDesign, strings.Repeat("x", MaxBriefBytes+1), false, "", nil, nil, "at most 4 KiB", ""},
		{"not UTF-8", JobDesign, "caf\xe9", false, "", nil, nil, "UTF-8", ""},
		{"five references", JobDesign, "b", false, "", append(refs, "e.png"), nil, "at most 4 reference images", ""},
		{"three addresses", JobDesign, "b", false, "", nil, []string{"https://a.example/", "https://b.example/", "https://c.example/"}, "at most 2 web addresses", ""},
		{"a plain-http address", JobDesign, "b", false, "", nil, []string{"http://example.com/"}, "the web addresses: ", ""},
		{"a cover for no design", JobCover, "b", false, "", nil, nil, "choose it first", ""},
		{"a cover with a logo", JobCover, "b", true, "logo.png", nil, nil, "design's own logo", ""},
		{"a cover with an address", JobCover, "b", true, "", nil, []string{"https://example.com/"}, "no web addresses", ""},
		{"a cover drawing too many", JobCover, "b", true, "", make([]string, MaxCoverImages+1), nil, "at most", ""},
		{"a cover", JobCover, "A dawn harbour band.", true, "", refs, nil, "", ""},
		{"a design from a document", JobDesign, "Like our annual report.", false, "", nil, nil, "", "annual-report.pdf"},
		{"a cover from a document", JobCover, "b", true, "", nil, nil, "no document", "annual-report.pdf"},
		{"another job", "edit", "b", true, "", nil, nil, "not design or cover", ""},
	}
	for _, c := range cases {
		err := Check(c.job, c.brief, c.from, c.logo, c.images, c.urls, c.document)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

// An image is read within the CLI's cap on the file, and one past it is
// refused by name before the run.
func TestAnImageIsReadWithinItsCap(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "logo.png")
	big := filepath.Join(dir, "poster.png")
	if err := os.WriteFile(ok, []byte("\x89PNG small"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(big, make([]byte, MaxImageBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadImage("the logo", ok); err != nil || string(b) != "\x89PNG small" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := ReadImage("the image", big); err == nil || !strings.Contains(err.Error(), "poster.png is over 4 MB") {
		t.Fatalf("over the cap: %v", err)
	}
	if _, err := ReadImage("the image", filepath.Join(dir, "missing.png")); err == nil {
		t.Fatal("a missing file")
	}
}

// The run hands the CLI's Report Designer exactly what was picked: the job,
// the brief, the design to start from with its images, the logo's and the
// images' bytes, the runner home and the output folder, with the built-ins'
// budgets; and answers the design it made with what the run said of it.
func TestARunHandsTheDesignerWhatWasPicked(t *testing.T) {
	home := t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	t.Setenv("AGENTRY_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	logo, img := filepath.Join(dir, "logo.png"), filepath.Join(dir, "ref.jpg")
	_ = os.WriteFile(logo, []byte("logo-bytes"), 0o600)
	_ = os.WriteFile(img, []byte("ref-bytes"), 0o600)
	from, ok := reportdesign.Builtin(reportdesign.BuiltinIDs()[0])
	if !ok {
		t.Fatal("no built-in theme")
	}
	made := from
	made.Name = "Northwind Harbour Trust"
	out := t.TempDir()
	var seen build.Options
	fake := func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		seen = o
		return &build.Result{Design: made, Looked: true, Previews: 3, Dir: filepath.Join(o.OutDir, "design"), DocumentPages: 4, LogoFromDocument: true}, nil
	}
	docPath := filepath.Join(dir, "Annual Report.pdf")
	_ = os.WriteFile(docPath, []byte("%PDF-1.7 brand"), 0o600)
	res, err := Run(context.Background(), Request{
		Job: JobDesign, Brief: "  Reports for a harbour trust.  ", From: &from, Logo: logo, Images: []string{img}, Document: docPath,
		Home: home, OutDir: out, RunID: "run-1", Build: fake,
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Job != build.Design || seen.Brief != "  Reports for a harbour trust.  " || seen.From != &from || string(seen.Logo) != "logo-bytes" ||
		len(seen.Images) != 1 || string(seen.Images[0]) != "ref-bytes" || seen.Home != home || seen.OutDir != out || seen.RunID != "run-1" ||
		seen.Budgets == nil || seen.Policy == nil || len(seen.URLs) != 0 ||
		seen.Document == nil || seen.Document.Name != "Annual Report.pdf" || string(seen.Document.Data) != "%PDF-1.7 brand" {
		t.Fatalf("handed %+v", seen)
	}
	if res.Design.Name != "Northwind Harbour Trust" || !res.Looked || res.Previews != 3 || res.Dir != filepath.Join(out, "design") ||
		res.DocumentPages != 4 || !res.LogoFromDocument {
		t.Fatalf("result %+v", res)
	}

	// A refused input never reaches the run.
	seen = build.Options{}
	if _, err := Run(context.Background(), Request{Job: JobCover, Brief: "b", Home: home, OutDir: out, Build: fake}); err == nil || seen.Job != "" {
		t.Fatalf("a cover without its design ran: %v %+v", err, seen)
	}
}

// A document is held to the CLI's own rule before a run reads it: one
// regular file, not empty, at most 20 MB, a PDF or a Word document as its
// bytes and its name say.
func TestADocumentIsCheckedAsTheCLIChecksIt(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var docx bytes.Buffer
	z := zip.NewWriter(&docx)
	w, _ := z.Create("word/document.xml")
	_, _ = w.Write([]byte("<w:document/>"))
	_ = z.Close()
	big, err := os.Create(filepath.Join(dir, "big.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = big.Write([]byte("%PDF-1.7"))
	_ = big.Truncate(MaxDocumentBytes + 1)
	_ = big.Close()
	cases := []struct {
		path, kind, want string
	}{
		{write("brand.pdf", []byte("%PDF-1.7\n%…")), DocumentPDF, ""},
		{write("letterhead.docx", docx.Bytes()), DocumentDOCX, ""},
		{write("pdf-named.docx", []byte("%PDF-1.4 x")), "", "a PDF or a Word document"},
		{write("archive.zip", docx.Bytes()), "", "a PDF or a Word document"},
		{write("notes.pdf", []byte("plain text")), "", "a PDF or a Word document"},
		{write("empty.pdf", nil), "", "is empty"},
		{filepath.Join(dir, "big.pdf"), "", "over 20 MB"},
		{dir, "", "not a file"},
		{filepath.Join(dir, "missing.pdf"), "", "the document"},
	}
	for _, c := range cases {
		kind, err := CheckDocument(c.path)
		switch {
		case c.want == "" && (err != nil || kind != c.kind):
			t.Errorf("%s: %q %v", filepath.Base(c.path), kind, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: %v, want %q", filepath.Base(c.path), err, c.want)
		}
	}
	if !DocumentHere {
		t.Fatal("the Report Designer this app carries reads a document")
	}
}

// A Cover counts the images the design's own custom cover already draws:
// with four there, three more are refused before the run; a Design's
// references are not cover images.
func TestACoverCountsTheDesignsOwnCoverImages(t *testing.T) {
	d, _ := reportdesign.Builtin(reportdesign.BuiltinIDs()[0])
	d.CustomCover.Images = make([]reportdesign.Image, 4)
	if err := CheckCoverImages(JobCover, d, 2); err != nil {
		t.Fatal(err)
	}
	if err := CheckCoverImages(JobCover, d, 3); err == nil || !strings.Contains(err.Error(), "already has 4") {
		t.Fatalf("seven: %v", err)
	}
	if err := CheckCoverImages(JobDesign, d, 4); err != nil {
		t.Fatalf("a design's references: %v", err)
	}
	ran := false
	fake := func(context.Context, dockerx.Client, build.Options) (*build.Result, error) {
		ran = true
		return &build.Result{}, nil
	}
	if _, err := Run(context.Background(), Request{Job: JobCover, Brief: "b", From: &d, Images: []string{"a", "b", "c"}, Build: fake}); err == nil || ran {
		t.Fatalf("ran past the count: %v", err)
	}
}

// A Report Designer run reports its progress as the other built-ins do: once
// at its start, then at every usage report.
func TestARunReportsItsProgress(t *testing.T) {
	home := t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	t.Setenv("AGENTRY_CONFIG_DIR", t.TempDir())
	made, ok := reportdesign.Builtin(reportdesign.BuiltinIDs()[0])
	if !ok {
		t.Fatal("no built-in theme")
	}
	fake := func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		o.OnUsage(receipt.Usage{Turns: 3, InputTokens: 900, OutputTokens: 120})
		return &build.Result{Design: made, Dir: filepath.Join(o.OutDir, "design")}, nil
	}
	var reports []progress.Report
	if _, err := Run(context.Background(), Request{
		Job: JobDesign, Brief: "Reports for a harbour trust.", Home: home, OutDir: t.TempDir(), RunID: "run-1", Build: fake,
		Progress: func(p progress.Report) { reports = append(reports, p) },
	}); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 || reports[0].StartedAt == 0 || reports[0].Turns != 0 || reports[1].Turns != 3 || reports[1].InputTokens != 900 || reports[1].OutputTokens != 120 {
		t.Fatalf("reports %+v", reports)
	}
}
