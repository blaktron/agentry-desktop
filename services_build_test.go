package main

// Describe it, on this computer (#282, plan builder-rework.md §7.1, D22,
// D25) through the bound surface: no key, no draft and no dialog; a native
// dialog before anything leaves this computer; the draft a run under the
// run lock, decoded into the form's Model, with no key in anything the
// page is given.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"

	"github.com/blaktron/agentry-desktop/internal/run"
)

const describeJev = "jev-describe-test-key-e41b"

type passScan struct{ read int }

func (s *passScan) Scan(context.Context, webscan.Source, string) webscan.Result {
	s.read++
	return webscan.Result{Verdict: webscan.Pass}
}

// describeFixture is a BuildAPI over a fresh home and an in-memory
// keychain, with the dialog's answer, the scan and the Drafter's run as
// stand-ins, and every progress line kept.
type describeFixture struct {
	b      *BuildAPI
	asked  []string
	answer bool
	scan   *passScan
	runs   []localrun.Run
	events []string
}

func newDescribeFixture(t *testing.T) *describeFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	t.Setenv(policy.DefaultJudgeKeyEnv, "")
	t.Setenv(policy.DefaultWebSearchKeyEnv, "")
	f := &describeFixture{answer: true, scan: &passScan{}}
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) { f.events = append(f.events, fmt.Sprint(name, data)) }
	t.Cleanup(func() { emitEvent = emit })
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, _ string) (bool, error) {
		f.asked = append(f.asked, title)
		return f.answer, nil
	}}
	runAPI := &RunAPI{app: app, inFlightDir: t.TempDir()}
	f.b = &BuildAPI{app: app, run: runAPI,
		draftScanner: func(key string) (webscan.Scanner, error) {
			if key != describeJev {
				t.Fatalf("scanned with another key")
			}
			return f.scan, nil
		},
		draftRunner: func(_ context.Context, r localrun.Run) (*receipt.Envelope, error) {
			f.runs = append(f.runs, r)
			if cur := runAPI.current(); cur == nil || cur.id != r.RunID {
				t.Fatalf("the Drafter ran outside the run in flight: %+v", cur)
			}
			for name, body := range map[string]string{
				filepath.Join("reports", "AGENT.md"):            "# Meeting actions\n\nRead the notes.\n",
				filepath.Join("reports", "notes.md"):            "Its notes: **bold** <script>alert(1)</script>\n",
				filepath.Join("receipts", engine.WebLedgerFile): `{"tool":"web_read","source":"page","url":"https://bad.example/","verdict":"withheld","score":3,"flagged":["injection"]}` + "\n",
			} {
				p := filepath.Join(r.OutDir, name)
				_ = os.MkdirAll(filepath.Dir(p), 0o755)
				_ = os.WriteFile(p, []byte(body), 0o644)
			}
			return &receipt.Envelope{Status: "ok", RunID: r.RunID, Harness: receipt.Harness{Model: "openai/qwen3.8-flash-next"},
				Result: json.RawMessage(`{"manifest": {"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
					"summary": "Turns meeting notes into actions.", "license": "Apache-2.0", "modes": ["autonomous"], "x-extra": 1},
					"matches": [{"ref": "acme/minutes", "why": "writes minutes"}], "sources": [], "openQuestions": ["Dates?"]}`),
				Web: &receipt.Web{Scan: toolsrv.WebScanJev, Read: 3, Passed: 2, Withheld: 1}}, nil
		},
	}
	return f
}

func saveDescribeKey(t *testing.T) {
	t.Helper()
	if err := run.StoreJevKey(describeJev, io.Discard); err != nil {
		t.Fatal(err)
	}
}

// No key: refused with the Settings pointer, before any dialog and before
// anything is scanned or run.
func TestDescribeWithoutAKeyIsRefusedBeforeAnyDialog(t *testing.T) {
	f := newDescribeFixture(t)
	_, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil)
	if err == nil || !strings.Contains(err.Error(), "Settings → AI Review key") {
		t.Fatalf("err = %v", err)
	}
	if len(f.asked) != 0 || f.scan.read != 0 || len(f.runs) != 0 {
		t.Fatalf("asked %v, scanned %d, ran %d", f.asked, f.scan.read, len(f.runs))
	}
	st, err := f.b.DescribeStatus()
	if err != nil || st.Key.Ready || st.Search != "no-provider" || st.Model == "" {
		t.Fatalf("status %+v %v", st, err)
	}
}

// The dialog comes before anything leaves this computer: answered No, the
// description is not even scanned.
func TestDescribeAsksInANativeDialogFirst(t *testing.T) {
	f := newDescribeFixture(t)
	saveDescribeKey(t)
	f.answer = false
	_, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil)
	if err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("err = %v", err)
	}
	if len(f.asked) != 1 || f.asked[0] != "Draft on this computer?" || f.scan.read != 0 || len(f.runs) != 0 {
		t.Fatalf("asked %v, scanned %d, ran %d", f.asked, f.scan.read, len(f.runs))
	}
}

func TestDescribeIsARunUnderTheRunLockAndDecodesTheDraft(t *testing.T) {
	f := newDescribeFixture(t)
	saveDescribeKey(t)

	f.b.run.mu.Lock()
	if _, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil); !errors.Is(err, errBusy) {
		t.Fatalf("describe while a run holds the lock: %v", err)
	}
	f.b.run.mu.Unlock()
	if f.scan.read != 0 {
		t.Fatal("scanned while another run held the lock")
	}

	v, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", []string{"Slack"})
	if err != nil {
		t.Fatal(err)
	}
	if f.scan.read != 1 || len(f.runs) != 1 || f.b.run.current() != nil {
		t.Fatalf("scanned %d, ran %d, still in flight %v", f.scan.read, len(f.runs), f.b.run.current())
	}
	if f.runs[0].Web == nil || f.runs[0].Web.JevKey != describeJev {
		t.Fatal("the run's web configuration does not carry the person's key")
	}
	if v.Model == nil || v.Model.Name != "meeting-actions" || v.Model.Namespace != "draft" || v.Model.Instructions != "# Meeting actions\n\nRead the notes.\n" {
		t.Fatalf("model %+v", v.Model)
	}
	if v.AIReview.Read != 3 || v.AIReview.Withheld != 1 || len(v.AIReview.WithheldSources) != 1 || v.AIReview.WithheldSources[0].URL != "https://bad.example/" {
		t.Fatalf("aiReview %+v", v.AIReview)
	}
	if !strings.Contains(v.Notes, "<script>") || !strings.Contains(v.Notes, "## Withheld by AI Review") {
		t.Fatalf("notes %q: the notes are the Drafter's text as written, for the page's reader to render", v.Notes)
	}
	if len(v.DecodeNotes) == 0 || !strings.Contains(strings.Join(v.DecodeNotes, " "), "x-extra") {
		t.Fatalf("decode notes %q", v.DecodeNotes)
	}
	if len(v.Matches) != 1 || v.Matches[0].Ref != "acme/minutes" || len(v.OpenQuestions) != 1 || v.OutDir == "" || v.DrafterModel == "" {
		t.Fatalf("view %+v", v)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), describeJev) {
		t.Fatal("the view carries the key")
	}
	for _, e := range f.events {
		if strings.Contains(e, describeJev) {
			t.Fatalf("a progress line carries the key: %s", e)
		}
	}
}
