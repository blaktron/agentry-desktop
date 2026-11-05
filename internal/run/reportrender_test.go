package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/sockfilter"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
)

// fakePDF is a PDF's bytes as far as the render's checks read them.
var fakePDF = []byte("%PDF-1.7\n% a stand-in for the render\n%%EOF\n")

func fakeSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// outSource is the host folder the container sees at /out.
func outSource(t *testing.T, r dockerx.RunSpec) string {
	t.Helper()
	for _, m := range r.Mounts {
		if m.Target == spec.OutMount {
			return m.Source
		}
	}
	t.Fatal("no output folder mounted")
	return ""
}

// exportPlayed is what a tools server that laid the sample out leaves: the
// PDF, the ledger's line for it, and export_pdf's answer.
func exportPlayed(t *testing.T, pdf []byte, entry toolsrv.PDFEntry) *dockerx.FakeForeground {
	return &dockerx.FakeForeground{
		Before: func(r dockerx.RunSpec) {
			out := outSource(t, r)
			if err := os.WriteFile(filepath.Join(out, "reports", "sample.pdf"), pdf, 0o644); err != nil {
				t.Error(err)
			}
			line, _ := json.Marshal(entry)
			if err := os.WriteFile(filepath.Join(out, "receipts", "pdfs.ndjson"), append(line, '\n'), 0o644); err != nil {
				t.Error(err)
			}
		},
		Stdout: `{"jsonrpc":"2.0","id":1,"result":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"report written: reports/sample.pdf"}],"isError":false}}` + "\n",
	}
}

func goodEntry(pdf []byte) toolsrv.PDFEntry {
	return toolsrv.PDFEntry{Path: "reports/sample.pdf", SHA256: fakeSum(pdf), Pages: 5, Bytes: int64(len(pdf)), Form: "report", Cover: "split"}
}

func renderSetup(t *testing.T) reportRenderSetup {
	return reportRenderSetup{
		image: "agentry-runner:test", root: t.TempDir(),
		engine: dockerx.Engine{Kind: dockerx.KindDocker},
		limits: policy.Limits{Memory: "3g", CPUs: 2, Pids: 300},
	}
}

func stamped(t *testing.T, id string) *toolsrv.ThemeStamp {
	t.Helper()
	st, err := toolsrv.StampTheme(mustBuiltin(t, id).Canonical())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestARenderIsHeldAsARunsToolsServer: the render's one container runs the
// runner image's tools server with no network, every capability dropped but
// the tools server's own, no new privileges, a read-only root, the run's two
// bounded tmpfs mounts and the policy's limits; its tools spec grants
// export_pdf with the design stamped as a run's is and nothing beyond the
// scratch folder; it is sent the handshake and one export of the sample; and
// what it laid out comes back, its folder removed after.
func TestARenderIsHeldAsARunsToolsServer(t *testing.T) {
	rs := renderSetup(t)
	st := stamped(t, "boardroom")
	var seen dockerx.RunSpec
	var specDoc []byte
	var sample []byte
	play := exportPlayed(t, fakePDF, goodEntry(fakePDF))
	before := play.Before
	play.Before = func(r dockerx.RunSpec) {
		seen = r
		for _, m := range r.Mounts {
			if m.Target == spec.ToolSpecMount {
				specDoc, _ = os.ReadFile(m.Source)
			}
		}
		sample, _ = os.ReadFile(filepath.Join(outSource(t, r), "reports", "sample.md"))
		before(r)
	}
	fake := &dockerx.Fake{Foreground: map[string]*dockerx.FakeForeground{"report-render": play}}
	r, err := renderReportSample(context.Background(), fake, rs, st)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(r.PDF, fakePDF) || r.Pages != 5 || r.Cover != "split" || r.SHA256 != fakeSum(fakePDF) {
		t.Fatalf("render %+v", r)
	}

	if seen.Image != rs.image || !slices.Equal(seen.Argv, spec.ToolsCmd()) || seen.Network != "none" || len(seen.ExtraNetworks) != 0 || len(seen.Publish) != 0 {
		t.Errorf("image, command or network: %q %q %q %v %v", seen.Image, seen.Argv, seen.Network, seen.ExtraNetworks, seen.Publish)
	}
	if !slices.Equal(seen.CapDrop, []string{"ALL"}) || !slices.Equal(seen.CapAdd, sockfilter.ToolsCapabilities()) || !seen.NoNewPrivileges || !seen.ReadOnly {
		t.Errorf("posture: drop %v add %v nnp %v ro %v", seen.CapDrop, seen.CapAdd, seen.NoNewPrivileges, seen.ReadOnly)
	}
	if seen.Memory != "3g" || seen.CPUs != 2 || seen.PidsLimit != 300 || seen.Runtime != "" || seen.DisableSELinuxLabel {
		t.Errorf("limits: %q %d %d %q %v", seen.Memory, seen.CPUs, seen.PidsLimit, seen.Runtime, seen.DisableSELinuxLabel)
	}
	tmpfs := fmt.Sprintf("size=%d,mode=1777", spec.ToolsTmpfsBytes)
	if want := []string{"/tmp:" + tmpfs, "/scratch:" + tmpfs}; !slices.Equal(seen.Tmpfs, want) {
		t.Errorf("tmpfs %v, want %v", seen.Tmpfs, want)
	}
	if len(seen.Env) != 0 || len(seen.Volumes) != 0 || seen.User != "" || seen.Entrypoint != "" || !seen.AutoRemove {
		t.Errorf("env %v, volumes %v, user %q, entrypoint %q, rm %v", seen.Env, seen.Volumes, seen.User, seen.Entrypoint, seen.AutoRemove)
	}
	mounts := map[string]bool{}
	for _, m := range seen.Mounts {
		mounts[m.Target] = m.ReadOnly
		if !strings.HasPrefix(m.Source, rs.root) {
			t.Errorf("a mount from outside the render's folder: %s", m.Source)
		}
	}
	if len(mounts) != 3 || !mounts[spec.ToolSpecMount] || !mounts["/workspace"] || mounts[spec.OutMount] {
		t.Errorf("mounts %v: the spec and the workspace read-only, the output folder writable, nothing else", mounts)
	}

	var ts toolsrv.Spec
	dec := json.NewDecoder(bytes.NewReader(specDoc))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ts); err != nil {
		t.Fatalf("the tools spec as the tools server reads it: %v", err)
	}
	// write_report is every tools server's; the scratch folder's tools come
	// with the run's scratch folder.
	want := []string{toolsrv.ToolScratchList, toolsrv.ToolScratchRead, toolsrv.ToolScratchWrite, toolsrv.ToolScratchSearch, toolsrv.ToolWriteReport, toolsrv.ToolExportPDF}
	if got := ts.GrantedTools(); !sameSet(got, want) {
		t.Errorf("granted %v, want %v", got, want)
	}
	if ts.SpecVersion != toolsrv.SpecVersion || ts.TmpfsBytes != spec.ToolsTmpfsBytes || ts.OutDir != "/out" || ts.ScratchDir != "/scratch" {
		t.Errorf("spec %+v", ts)
	}
	if ts.PDF.Theme == nil || *ts.PDF.Theme != *st || ts.PDF.Author != ReportSampleAuthor || ts.PDF.Ledger != "/out/receipts/pdfs.ndjson" {
		t.Errorf("pdf grant %+v", ts.PDF)
	}
	if ts.Owner == nil || ts.Owner.UID != os.Getuid() {
		t.Errorf("owner %+v", ts.Owner)
	}
	if string(sample) != reportSample {
		t.Error("the sample written is not the app's sample")
	}

	lines := strings.Split(strings.TrimSpace(play.GotStdin), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdin %q", play.GotStdin)
	}
	var call struct {
		Method string `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &call); err != nil || call.Method != "tools/call" || call.Params.Name != "export_pdf" ||
		call.Params.Arguments["source"] != "reports/sample.md" || call.Params.Arguments["form"] != "report" || len(call.Params.Arguments) != 3 {
		t.Errorf("the call %s (%v)", lines[2], err)
	}
	if left, _ := os.ReadDir(rs.root); len(left) != 0 {
		t.Errorf("the render's folder was left: %v", left)
	}
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// TestARenderShowsOnlyTheLedgersCheckedPDF: the PDF comes back only when the
// ledger names the sample's PDF alone, under reports/, read with no link,
// starting %PDF- and hashing to the ledger's SHA-256.
func TestARenderShowsOnlyTheLedgersCheckedPDF(t *testing.T) {
	other := []byte("%PDF-1.7\n% another PDF\n%%EOF\n")
	cases := map[string]func(*dockerx.FakeForeground){
		"another SHA-256": func(fg *dockerx.FakeForeground) {
			e := goodEntry(fakePDF)
			e.SHA256 = fakeSum(other)
			*fg = *exportPlayed(t, fakePDF, e)
		},
		"not a PDF": func(fg *dockerx.FakeForeground) {
			b := []byte("<html>not a pdf</html>")
			e := goodEntry(b)
			*fg = *exportPlayed(t, b, e)
		},
		"another path": func(fg *dockerx.FakeForeground) {
			e := goodEntry(fakePDF)
			e.Path = "reports/other.pdf"
			*fg = *exportPlayed(t, fakePDF, e)
		},
		"outside reports": func(fg *dockerx.FakeForeground) {
			e := goodEntry(fakePDF)
			e.Path = "outputs/sample.pdf"
			*fg = *exportPlayed(t, fakePDF, e)
		},
		"two entries": func(fg *dockerx.FakeForeground) {
			*fg = *exportPlayed(t, fakePDF, goodEntry(fakePDF))
			before := fg.Before
			fg.Before = func(r dockerx.RunSpec) {
				before(r)
				line, _ := json.Marshal(goodEntry(fakePDF))
				f, _ := os.OpenFile(filepath.Join(outSource(t, r), "receipts", "pdfs.ndjson"), os.O_APPEND|os.O_WRONLY, 0)
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		},
		"no ledger": func(fg *dockerx.FakeForeground) {
			*fg = *exportPlayed(t, fakePDF, goodEntry(fakePDF))
			before := fg.Before
			fg.Before = func(r dockerx.RunSpec) {
				before(r)
				_ = os.Remove(filepath.Join(outSource(t, r), "receipts", "pdfs.ndjson"))
			}
		},
		"a link as the PDF": func(fg *dockerx.FakeForeground) {
			*fg = *exportPlayed(t, fakePDF, goodEntry(fakePDF))
			before := fg.Before
			fg.Before = func(r dockerx.RunSpec) {
				before(r)
				out := outSource(t, r)
				elsewhere := filepath.Join(filepath.Dir(out), "elsewhere.pdf")
				_ = os.WriteFile(elsewhere, fakePDF, 0o644)
				p := filepath.Join(out, "reports", "sample.pdf")
				_ = os.Remove(p)
				if err := os.Symlink(elsewhere, p); err != nil {
					t.Error(err)
				}
			}
		},
		"a link as reports/": func(fg *dockerx.FakeForeground) {
			*fg = *exportPlayed(t, fakePDF, goodEntry(fakePDF))
			before := fg.Before
			fg.Before = func(r dockerx.RunSpec) {
				before(r)
				out := outSource(t, r)
				moved := filepath.Join(filepath.Dir(out), "moved")
				if err := os.Rename(filepath.Join(out, "reports"), moved); err != nil {
					t.Error(err)
				}
				if err := os.Symlink(moved, filepath.Join(out, "reports")); err != nil {
					t.Error(err)
				}
			}
		},
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			fg := &dockerx.FakeForeground{}
			script(fg)
			fake := &dockerx.Fake{Foreground: map[string]*dockerx.FakeForeground{"report-render": fg}}
			r, err := renderReportSample(context.Background(), fake, renderSetup(t), stamped(t, "plain"))
			if err == nil || r != nil {
				t.Fatalf("shown: %+v", r)
			}
		})
	}
}

// TestARenderSaysWhyItDidNotLayTheSampleOut: export_pdf's refusal in its
// own words, a container that ended with nothing in its last line and exit
// code, a stop when the caller cancels, and an engine that drops the
// policy's runtime refused before anything runs.
func TestARenderSaysWhyItDidNotLayTheSampleOut(t *testing.T) {
	run := func(t *testing.T, ctx context.Context, rs reportRenderSetup, fg *dockerx.FakeForeground) error {
		fake := &dockerx.Fake{Foreground: map[string]*dockerx.FakeForeground{"report-render": fg}}
		r, err := renderReportSample(ctx, fake, rs, stamped(t, "plain"))
		if err == nil || r != nil {
			t.Fatalf("shown: %+v", r)
		}
		return err
	}
	t.Run("refused", func(t *testing.T) {
		err := run(t, context.Background(), renderSetup(t), &dockerx.FakeForeground{
			Stdout: `{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"export_pdf: Typst failed: the cover's code"}],"isError":true}}` + "\n",
		})
		if !strings.Contains(err.Error(), "could not be laid out: export_pdf: Typst failed") {
			t.Fatal(err)
		}
	})
	t.Run("ended", func(t *testing.T) {
		err := run(t, context.Background(), renderSetup(t), &dockerx.FakeForeground{Stderr: "noise\nagentry-runner tools: parse tool spec: bad\n", ExitCode: 1})
		if !strings.Contains(err.Error(), "exit 1") || !strings.Contains(err.Error(), "parse tool spec: bad") {
			t.Fatal(err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fg := &dockerx.FakeForeground{Block: true, Before: func(dockerx.RunSpec) { cancel() }}
		if err := run(t, ctx, renderSetup(t), fg); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
	t.Run("runtime dropped", func(t *testing.T) {
		rs := renderSetup(t)
		rs.runtime = "runsc"
		played := false
		fg := exportPlayed(t, fakePDF, goodEntry(fakePDF))
		fg.Before = func(dockerx.RunSpec) { played = true }
		fake := &dockerx.Fake{DropRuntime: true, Foreground: map[string]*dockerx.FakeForeground{"report-render": fg}}
		r, err := renderReportSample(context.Background(), fake, rs, stamped(t, "plain"))
		if played || r != nil || err == nil || !strings.Contains(err.Error(), "runsc") {
			t.Fatalf("played %v, %+v: %v", played, r, err)
		}
	})
}

// TestARenderRunsUnderThePolicysRuntime: a policy's runtime is the
// container's, read back from the engine before it starts.
func TestARenderRunsUnderThePolicysRuntime(t *testing.T) {
	rs := renderSetup(t)
	rs.runtime = "runsc"
	var seen dockerx.RunSpec
	fg := exportPlayed(t, fakePDF, goodEntry(fakePDF))
	fake := &dockerx.Fake{OnForeground: func(r dockerx.RunSpec) { seen = r }, Foreground: map[string]*dockerx.FakeForeground{"report-render": fg}}
	if _, err := renderReportSample(context.Background(), fake, rs, stamped(t, "plain")); err != nil {
		t.Fatal(err)
	}
	if seen.Runtime != "runsc" {
		t.Fatalf("runtime %q", seen.Runtime)
	}
}

// TestTheSampleGetsACover: the sample is long enough for a design whose
// cover is drawn for a long report, and starts with the title the slots'
// facts name.
func TestTheSampleGetsACover(t *testing.T) {
	title, body, _ := strings.Cut(reportSample, "\n")
	if title != "# "+ReportSampleTitle {
		t.Fatalf("title %q", title)
	}
	if n := len(strings.Fields(body)); n < 1600 {
		t.Fatalf("%d words: a long report's cover needs 1,500 after the title and the subtitle", n)
	}
}
