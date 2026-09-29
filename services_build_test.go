package main

// Describe it, on this computer (#282, plan builder-rework.md §7.1, D22,
// D25) through the bound surface: no key, no draft and no dialog; a native
// dialog before anything leaves this computer; the draft a run under the
// run lock, decoded into the form's Model, with no key in anything the
// page is given.

import (
	"cmp"
	"github.com/blaktron/agentry-cli/pkg/drafter"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/toolsrv"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"
	"github.com/blaktron/agentry/pkg/draftcheck"

	"github.com/blaktron/agentry-desktop/internal/draft"
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
	// checks are the web configurations AI Review's check was built for,
	// and checksAsked how many times it was asked; it passes, and never
	// reaches TypeSafe.
	checks      []*engine.WebRun
	checksAsked int
	// result is the Drafter's answer; empty is describeAnswer.
	result string
	// repairRecorded says the repair turn's crash-path record was on disk
	// while it ran.
	repairRecorded bool
}

// passCheck is AI Review's check of a finished draft, both halves passed.
type passCheck struct{ f *describeFixture }

func (c passCheck) Check(context.Context, draftcheck.Package) (*drafter.Check, error) {
	c.f.checksAsked++
	return &drafter.Check{Model: "jev-1.13.0", Requests: 5,
		Rubric: &drafter.CheckRubric{Verdict: drafter.CheckPass, Findings: []drafter.CheckFinding{}, RubricHash: "sha256:r"},
		Draft:  &drafter.CheckDraft{Verdict: drafter.CheckPass, Flagged: []string{}, QuestionSet: "sha256:d"}}, nil
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
		draftChecker: func(w *engine.WebRun) (drafter.Checker, error) {
			f.checks = append(f.checks, w)
			return passCheck{f}, nil
		},
		draftRunner: func(_ context.Context, r localrun.Run) (*receipt.Envelope, error) {
			f.runs = append(f.runs, r)
			// The first turn is the run in flight; the repair turn runs
			// inside it, under its own run id, in its folder.
			cur := runAPI.current()
			switch {
			case cur == nil:
				t.Fatal("the Drafter ran outside the run in flight")
			case len(f.runs) == 1 && cur.id != r.RunID:
				t.Fatalf("the Drafter's first turn is not the run in flight: %+v", cur)
			case len(f.runs) > 1 && (r.Web != nil || filepath.Dir(r.OutDir) != f.runs[0].OutDir):
				t.Fatalf("the repair turn has a web configuration %+v or runs outside the draft's folder: %s", r.Web, r.OutDir)
			case len(f.runs) > 1:
				_, err := os.Stat(filepath.Join(runAPI.inFlightDir, r.RunID+".json"))
				f.repairRecorded = err == nil
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
				Result: json.RawMessage(cmp.Or(f.result, describeAnswer)),
				Web:    &receipt.Web{Scan: toolsrv.WebScanJev, Read: 3, Passed: 2, Withheld: 1}}, nil
		},
	}
	return f
}

// describeAnswer is the Drafter's answer in the describe fixture: a manifest
// with a field the form does not read (x-extra), which the decode names and
// the core's parser refuses.
const describeAnswer = `{"manifest": {"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
	"summary": "Turns meeting notes into actions.", "license": "Apache-2.0", "modes": ["autonomous"], "x-extra": 1},
	"matches": [{"ref": "acme/minutes", "why": "writes minutes"}], "sources": [{"url": "https://bad.example/", "used": "the format", "withheld": false}], "openQuestions": ["Dates?"]}`

// A local draft the rules pass is checked by AI Review with the person's
// own key, and the page's view carries the check's one line for the
// summary, as a hosted draft's does (#395).
func TestALocalDraftsViewCarriesItsCheckLine(t *testing.T) {
	f := newDescribeFixture(t)
	f.result = `{"manifest": {"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
		"summary": "Turns meeting notes into actions.", "license": "Apache-2.0", "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
		"capabilities": {"network": {"egress": "none"}, "filesystem": {"read": "none", "write": "none"}, "exec": "none"},
		"publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}},
		"matches": [], "sources": [], "openQuestions": []}`
	saveDescribeKey(t)
	v, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runs) != 1 || f.checksAsked != 1 || v.Check == nil || v.Check.Checks != 1 || v.Check.Rubric != drafter.CheckPass ||
		v.Check.Line == nil || !strings.Contains(v.Check.Line.Text, "flagged nothing") || len(v.Problems) != 0 {
		t.Fatalf("%d run(s), %d check(s), view %+v problems %q", len(f.runs), f.checksAsked, v.Check, v.Problems)
	}
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
	if err == nil || !strings.Contains(err.Error(), "Settings → Provider settings → AI Review key") {
		t.Fatalf("err = %v", err)
	}
	if len(f.asked) != 0 || f.scan.read != 0 || len(f.runs) != 0 {
		t.Fatalf("asked %v, scanned %d, ran %d", f.asked, f.scan.read, len(f.runs))
	}
	st, err := f.b.DescribeStatus()
	if err != nil || st.Key.Ready || st.Search != "no-provider" || st.ModelName == "" {
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
	// This draft's manifest does not parse (its x-extra), which the rules
	// report, so the CLI's post-step gives it its one repair turn, without
	// the web tools, under the same run lock, and recorded for the crash
	// path while it runs (#395).
	if f.scan.read != 1 || len(f.runs) != 2 || f.b.run.current() != nil {
		t.Fatalf("scanned %d, ran %d, still in flight %v", f.scan.read, len(f.runs), f.b.run.current())
	}
	if !f.repairRecorded || f.runs[1].RunID == f.runs[0].RunID {
		t.Fatal("the repair turn ran with no crash-path record of its run id")
	}
	if entries, _ := os.ReadDir(f.b.run.inFlightDir); len(entries) != 0 {
		t.Fatalf("crash-path records outlived the draft: %v", entries)
	}
	if f.runs[1].Web != nil || f.runs[1].Builtin != localrun.Drafter().WithoutWebTools() {
		t.Fatal("the repair turn has the web tools or a web configuration")
	}
	// AI Review's check is built with the person's own key, and is not asked
	// of a draft whose manifest does not parse (its x-extra), since there is
	// no package to read: the page names the problem instead (internal/draft's
	// tests hold the check of a draft that renders).
	if len(f.checks) != 1 || f.checks[0].JevKey != describeJev || f.checksAsked != 0 || v.Check != nil ||
		!strings.Contains(v.ManifestError, "x-extra") || len(v.Problems) == 0 {
		t.Fatalf("the check built for %+v, asked %d, view %+v (manifestError %q, problems %q)", f.checks, f.checksAsked, v.Check, v.ManifestError, v.Problems)
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
	// The notes are the Drafter's text as written, then the post-step's own
	// sections (here the repair turn's), as a hosted draft's are, for the
	// page's reader to render; the withheld list the post-step appends is
	// shown from AI Review's record instead, as text, never as links.
	if !strings.HasPrefix(v.Notes, "Its notes: **bold** <script>alert(1)</script>\n\n## Repaired once\n") ||
		strings.Contains(v.Notes, "Withheld by AI Review") {
		t.Fatalf("notes %q", v.Notes)
	}
	// The Drafter says it used the page AI Review withheld; the record
	// wins, so the page never offers it as a link.
	if len(v.Sources) != 1 || v.Sources[0].URL != "https://bad.example/" || !v.Sources[0].Withheld {
		t.Fatalf("sources %+v", v.Sources)
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

// A check's verdicts reach the page as one of the check's three words: a
// draft.json that writes its own ("safe to publish") reads not_scanned, so
// no words of the draft's stand where the page says the verdict.
func TestTheChecksVerdictsAreTheChecksOwnWords(t *testing.T) {
	c := &drafter.Check{Rubric: &drafter.CheckRubric{Verdict: "safe to publish", Findings: []drafter.CheckFinding{}},
		Draft: &drafter.CheckDraft{Verdict: "flagged", Flagged: []string{"off_task"}}}
	v := checkView(c)
	if v.Rubric != "not_scanned" || v.Draft != "flagged" {
		t.Fatalf("verdicts %q %q", v.Rubric, v.Draft)
	}
	if u := checkView(&drafter.Check{Unanswered: "no answer"}); u.Rubric != "" || u.Draft != "" || u.Unanswered != "no answer" {
		t.Fatalf("unanswered %+v", u)
	}
}

// A draft on this computer reports its progress as build:draft-progress,
// as a knowledge build reports its own (agentry-desktop#410).
func TestADraftOnThisComputerReportsItsProgress(t *testing.T) {
	f := newDescribeFixture(t)
	saveDescribeKey(t)
	if _, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil); err != nil {
		t.Fatal(err)
	}
	var stages []string
	for _, e := range f.events {
		if rest, ok := strings.CutPrefix(e, eventDraftProgress+"["); ok {
			stages = append(stages, rest)
		}
	}
	if len(stages) == 0 || !strings.Contains(stages[0], string(draft.StageDescription)) || !strings.Contains(stages[len(stages)-1], string(draft.StageChecking)) {
		t.Fatalf("progress events %q", stages)
	}
}

// StopDraft stops the draft in flight and nothing else: the Drafter's
// context is cancelled and the draft returns; with no draft running, or a
// run or a knowledge build holding the lock instead, it refuses and
// cancels nothing (agentry-desktop#410).
func TestStopDraftStopsOnlyADraft(t *testing.T) {
	f := newDescribeFixture(t)
	saveDescribeKey(t)
	if err := f.b.StopDraft(); err == nil {
		t.Fatal("StopDraft with nothing running passed")
	}
	// A run the page started under the Drafter's own ref is a run, not a
	// draft.
	for _, ref := range []string{"acme/triage@1.0.0", drafter.Namespace + "/" + drafter.Name, knowledgeRunRef} {
		cancelled := false
		l, err := f.b.run.beginRun(ref, func() { cancelled = true })
		if err != nil {
			t.Fatal(err)
		}
		if err := f.b.StopDraft(); err == nil || cancelled {
			t.Fatalf("StopDraft while %s runs: %v, cancelled %v", ref, err, cancelled)
		}
		f.b.run.endRun(l)
	}

	started := make(chan struct{})
	f.b.draftRunner = func(ctx context.Context, _ localrun.Run) (*receipt.Envelope, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil)
		done <- err
	}()
	<-started
	if err := f.b.StopDraft(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the stopped draft returned a draft")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the draft did not stop")
	}
	if f.b.run.current() != nil {
		t.Fatal("the stopped draft is still the run in flight")
	}
}
