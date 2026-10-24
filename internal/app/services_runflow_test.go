package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// events captures what the app emits, in order, for one test.
type events struct {
	mu  sync.Mutex
	got []struct {
		name string
		data any
	}
}

func captureEvents(t *testing.T) *events {
	t.Helper()
	e := &events{}
	prev := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) != 1 {
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.got = append(e.got, struct {
			name string
			data any
		}{name, data[0]})
	}
	t.Cleanup(func() { emitEvent = prev })
	return e
}

func (e *events) progress() []RunProgress {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []RunProgress
	for _, g := range e.got {
		if p, ok := g.data.(RunProgress); ok && g.name == eventRunProgress {
			out = append(out, p)
		}
	}
	return out
}

func (e *events) lines() []RunLine {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []RunLine
	for _, g := range e.got {
		if l, ok := g.data.(RunLine); ok && g.name == eventRunLine {
			out = append(out, l)
		}
	}
	return out
}

// flowAPI is a RunAPI whose engine is run, with its own runner home.
func flowAPI(t *testing.T, engine func(context.Context, run.Request, io.Writer) (*run.Result, error)) *RunAPI {
	t.Helper()
	return &RunAPI{
		runnerHomeDir: t.TempDir(),
		app:           &App{ctx: context.Background()},
		newRunID:      func() (string, error) { return "ab12cd34", nil },
		engineRun:     engine,
		sweep:         func(context.Context, string) error { return nil },
	}
}

func runAgent(r *RunAPI) (*RunResultView, error) {
	return r.RunAgent("https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil)
}

// A run on this computer reports its progress as it goes: a first report the
// moment it has begun, with its id and release and no budgets yet; its
// effective budgets once it is prepared, when its clock starts again so that
// it measures the run against its own timeout; the harness's usage as it
// arrives; and each line of its log with its id, so the page follows this
// run and no other.
func TestARunReportsItsProgressAndItsLinesByItsID(t *testing.T) {
	ev := captureEvents(t)
	r := flowAPI(t, func(_ context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
		_, _ = io.WriteString(logw, "pulling images\n")
		req.OnBudgets(1800, 40)
		req.OnUsage(receipt.Usage{Turns: 3, InputTokens: 3000, OutputTokens: 300})
		_, _ = io.WriteString(logw, "the agent answered\n")
		return &run.Result{Status: "ok", RunID: req.RunID}, nil
	})
	if _, err := runAgent(r); err != nil {
		t.Fatal(err)
	}
	got := ev.progress()
	if len(got) != 3 {
		t.Fatalf("progress reports %+v, want three", got)
	}
	first, second, last := got[0], got[1], got[2]
	if first.RunID != "ab12cd34" || first.Ref != "acme/agent@1.0.0" || first.StartedAt == 0 || first.TimeoutSeconds != 0 || first.Turns != 0 || first.Prepared {
		t.Fatalf("first report %+v", first)
	}
	if second.TimeoutSeconds != 1800 || second.MaxTurns != 40 || second.Turns != 0 || !second.Prepared || second.StartedAt < first.StartedAt {
		t.Fatalf("second report %+v, want the budgets and the run prepared", second)
	}
	if last.Turns != 3 || last.InputTokens != 3000 || last.OutputTokens != 300 || last.TimeoutSeconds != 1800 || last.StartedAt != second.StartedAt || last.RunID != "ab12cd34" {
		t.Fatalf("last report %+v", last)
	}
	lines := ev.lines()
	if len(lines) != 2 || lines[0] != (RunLine{RunID: "ab12cd34", Line: "pulling images"}) || lines[1].Line != "the agent answered" {
		t.Fatalf("lines %+v", lines)
	}
	ev.mu.Lock()
	defer ev.mu.Unlock()
	for _, g := range ev.got {
		if g.name == eventRunLog {
			t.Fatalf("a run on this computer wrote the unkeyed log event: %v", g.data)
		}
	}
}

// StopRun stops the run on this computer and nothing else: with nothing
// running, or a draft, a knowledge build or a chat holding the lock, it
// refuses and cancels nothing; with a run, the run's context is cancelled
// and RunAgent returns what the engine made of it. StopDraft and StopBuild
// still refuse a run.
func TestStopRunStopsOnlyARun(t *testing.T) {
	started := make(chan struct{})
	r := flowAPI(t, func(ctx context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		close(started)
		<-ctx.Done()
		// The engine writes the envelope as far as the run got.
		return &run.Result{Status: "error", ExitCode: 130, Error: "run interrupted", RunID: req.RunID}, nil
	})
	if err := r.StopRun(); !errors.Is(err, errNoRun) {
		t.Fatalf("StopRun with nothing running: %v", err)
	}
	for _, j := range []struct {
		kind jobKind
		ref  string
	}{{jobDraft, draftRunRef}, {jobBuild, knowledgeRunRef}, {jobChat, "acme/agent@1.0.0"}} {
		cancelled := false
		l, err := r.beginRun(j.kind, j.ref, func() { cancelled = true })
		if err != nil {
			t.Fatal(err)
		}
		if err := r.StopRun(); err == nil || cancelled {
			t.Fatalf("StopRun while a %s runs: %v, cancelled %v", j.kind, err, cancelled)
		}
		r.endRun(l)
	}

	done := make(chan *RunResultView, 1)
	go func() {
		v, err := runAgent(r)
		if err != nil {
			t.Error(err)
		}
		done <- v
	}()
	<-started
	b := &BuildAPI{app: r.app, run: r}
	if err := b.StopDraft(); err == nil {
		t.Fatal("StopDraft stopped a run")
	}
	k := &KnowledgeAPI{app: r.app, run: r}
	if err := k.StopBuild(); err == nil {
		t.Fatal("StopBuild stopped a run")
	}
	if err := r.StopRun(); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-done:
		if v == nil || v.Status != "error" || v.ExitCode != 130 {
			t.Fatalf("the stopped run's result %+v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not return after Stop")
	}
}

// The busy refusal names the job holding the run lock, and InFlight does
// too, for every kind of job; with nothing running InFlight is nil. (A run
// started meanwhile waits its turn instead: services_runqueue_test.go.)
func TestTheBusyRefusalNamesWhatIsRunning(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	r := flowAPI(t, func(ctx context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		close(started)
		<-release
		return &run.Result{Status: "ok", RunID: req.RunID}, nil
	})
	if v := r.InFlight(); v != nil {
		t.Fatalf("InFlight with nothing running: %+v", v)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = runAgent(r)
	}()
	<-started
	if v := r.InFlight(); v == nil || v.Kind != string(jobRun) || v.Line != "A run of acme/agent@1.0.0" {
		t.Fatalf("InFlight during a run: %+v", v)
	}
	for name, call := range map[string]func() error{
		"removing images": func() error { _, err := r.RemoveAgentryImages(); return err },
		"saving a key":    func() error { _, err := r.StoreJevKey("x"); return err },
	} {
		err := call()
		if !errors.Is(err, errBusy) {
			t.Fatalf("%s during a run: %v, want the busy refusal", name, err)
		}
		if want := "A run of acme/agent@1.0.0 is under way on this computer, and one job runs here at a time: wait for it to finish, or stop it."; err.Error() != want {
			t.Fatalf("%s during a run: %q, want %q", name, err.Error(), want)
		}
	}
	close(release)
	<-done
	if v := r.InFlight(); v != nil {
		t.Fatalf("InFlight after the run: %+v", v)
	}

	for _, j := range []struct {
		kind       jobKind
		what, want string
	}{
		{jobDraft, "a draft by the Drafter", "A draft by the Drafter is under way on this computer, and one job runs here at a time: wait for it to finish, or stop it."},
		{jobBuild, "a knowledge build by the Knowledge Creator", "A knowledge build by the Knowledge Creator is under way on this computer, and one job runs here at a time: wait for it to finish, or stop it."},
		{jobChat, "a chat with acme/agent@1.0.0", "A chat with acme/agent@1.0.0 is under way on this computer, and one job runs here at a time: wait for it to finish, or end it."},
		{jobSetup, "the container engine install", "The container engine install is under way on this computer, and one job runs here at a time: wait for it to finish."},
	} {
		if err := r.acquire(j.kind, j.what); err != nil {
			t.Fatal(err)
		}
		_, err := r.RemoveAgentryImages()
		if err == nil || err.Error() != j.want {
			t.Fatalf("removing images while %s holds the lock: %v, want %q", j.what, err, j.want)
		}
		if v := r.InFlight(); v == nil || v.Kind != string(j.kind) {
			t.Fatalf("InFlight while %s: %+v", j.what, v)
		}
		r.release()
	}
}

// With Remember recent runs off, Dismiss makes the app forget a run:
// ForgetRun drops its folder, so RunOutputs and ShowRunFolder refuse its id
// afterwards, and the folder, its receipts included, is left as it was.
// (With it on, the history keeps the run: services_history_test.go.)
func TestAForgottenRunsFolderIsRefusedAndLeftAlone(t *testing.T) {
	freshConfig(t)
	setRemember(t, false)
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(out, "receipts"), 0o700); err != nil {
		t.Fatal(err)
	}
	receiptFile := filepath.Join(out, "receipts", "run-plan.json")
	if err := os.WriteFile(receiptFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := flowAPI(t, func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		return &run.Result{Status: "ok", RunID: req.RunID, OutDir: out, ReceiptsDir: filepath.Join(out, "receipts")}, nil
	})
	opened := ""
	r.openFolder = func(dir string) error { opened = dir; return nil }
	v, err := runAgent(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunOutputs(v.RunID); err != nil {
		t.Fatalf("RunOutputs before Dismiss: %v", err)
	}
	r.ForgetRun(v.RunID)
	if _, err := r.RunOutputs(v.RunID); !errors.Is(err, errUnknownRun) {
		t.Fatalf("RunOutputs after Dismiss: %v", err)
	}
	if err := r.ShowRunFolder(v.RunID); !errors.Is(err, errUnknownRun) || opened != "" {
		t.Fatalf("ShowRunFolder after Dismiss: %v, opened %q", err, opened)
	}
	if b, err := os.ReadFile(receiptFile); err != nil || strings.TrimSpace(string(b)) != "{}" {
		t.Fatalf("the run's receipt after Dismiss: %q, %v", b, err)
	}
}

// The engine's usage hook never waits for the page: with the page taking no
// usage report at all, the engine's calls return at once, and the run goes
// on.
func TestTheEnginesUsageHookNeverWaitsForThePage(t *testing.T) {
	release := make(chan struct{})
	prev := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if p, ok := data[0].(RunProgress); ok && name == eventRunProgress && p.Turns > 0 {
			<-release
		}
	}
	t.Cleanup(func() { emitEvent = prev })
	var took time.Duration
	r := flowAPI(t, func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		start := time.Now()
		for i := 1; i <= 50; i++ {
			req.OnUsage(receipt.Usage{Turns: i})
		}
		took = time.Since(start)
		return &run.Result{Status: "ok", RunID: req.RunID}, nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = runAgent(r)
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not return once the page took its reports")
	}
	if took > 100*time.Millisecond {
		t.Fatalf("50 usage reports took the engine %v with the page taking none", took)
	}
}
