// A run on this computer followed above the pages (agentry-desktop#465;
// plan desktop-review-key-and-run-cards.md §5.2, §5.4; D5, D7, D9, D10,
// D16): the page's store follows the run by its id through
// runner:run-progress and runner:run-line, Stop ends it (StopRun, which
// stops a run and nothing else), the busy refusal and InFlight say which
// job holds the run lock, and Dismiss makes the app forget the run
// (ForgetRun).
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/progress"
)

const (
	// eventRunProgress carries a run's RunProgress; eventRunLine a line of
	// its log with its id. A run on this computer is followed by its id,
	// so a setup open on another QuickAgent never shows its lines.
	eventRunProgress = "runner:run-progress"
	eventRunLine     = "runner:run-line"
)

// The kinds of local job the run lock is held for.
const (
	jobRun   = "run"
	jobChat  = "chat"
	jobDraft = "draft"
	jobBuild = "build"
	jobSetup = "setup"
)

// jobHolder is what holds the run lock: its kind and a phrase naming it.
type jobHolder struct {
	kind, what string
}

// busyError refuses a second local job, naming the one that holds the run
// lock (D16). It is errBusy for errors.Is.
type busyError struct{ msg string }

func (e busyError) Error() string        { return e.msg }
func (e busyError) Is(target error) bool { return target == errBusy }

// acquire takes the run lock for a job of kind, named by what, or refuses
// with the job that holds it.
func (r *RunAPI) acquire(kind, what string) error {
	if !r.mu.TryLock() {
		return r.busy()
	}
	r.holderMu.Lock()
	r.holder = &jobHolder{kind: kind, what: what}
	r.holderMu.Unlock()
	return nil
}

// release gives the run lock back.
func (r *RunAPI) release() {
	r.holderMu.Lock()
	r.holder = nil
	r.holderMu.Unlock()
	r.mu.Unlock()
}

// busy is the refusal while another job holds the run lock.
func (r *RunAPI) busy() error {
	r.holderMu.Lock()
	h := r.holder
	r.holderMu.Unlock()
	what, how := "another job", ""
	if h != nil {
		what = h.what
		switch h.kind {
		case jobRun, jobDraft, jobBuild:
			how = ", or stop it"
		case jobChat:
			how = ", or end it"
		}
	}
	return busyError{msg: fmt.Sprintf("%s is under way on this computer, and one job runs here at a time: wait for it to finish%s.", capitalize(what), how)}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// InFlightView names the local job holding the run lock.
type InFlightView struct {
	// Kind is run, chat, draft, build or setup.
	Kind string `json:"kind"`
	// Line names the job in a sentence's subject position.
	Line string `json:"line"`
}

// InFlight names the local job holding the run lock, or nil when there is
// none (D16).
func (r *RunAPI) InFlight() *InFlightView {
	r.holderMu.Lock()
	defer r.holderMu.Unlock()
	if r.holder == nil {
		return nil
	}
	return &InFlightView{Kind: r.holder.kind, Line: capitalize(r.holder.what)}
}

// RunProgress is how far a run on this computer has got, for the progress
// dialog and its card: the report every job carries, the run's id and the
// release. Its budgets are the run's effective ones, known once the run is
// prepared; until then they are zero.
type RunProgress struct {
	progress.Report
	RunID string `json:"runId"`
	Ref   string `json:"ref"`
}

// RunLine is one line of a run's log, with the run's id.
type RunLine struct {
	RunID string `json:"runId"`
	Line  string `json:"line"`
}

// runMeter is a run's progress meter: its first report says the run has
// begun, before anything is prepared; then its budgets, then the harness's
// usage. It only emits events, so the engine's usage hook never waits on
// the page.
func (r *RunAPI) runMeter(runID, ref string) *progress.Meter[RunProgress, *RunProgress] {
	emit := func(p RunProgress) { emitEvent(r.app, eventRunProgress, p) }
	return progress.NewMeter(emit, RunProgress{Report: progress.Start(0, 0), RunID: runID, Ref: ref})
}

func budgetsOf(m *progress.Meter[RunProgress, *RunProgress]) func(int, int) {
	return func(timeoutSeconds, maxTurns int) {
		m.Set(func(p *RunProgress) bool {
			p.TimeoutSeconds, p.MaxTurns = timeoutSeconds, maxTurns
			return true
		})
	}
}

// usageOf is the engine's usage hook for a run's meter.
func usageOf(m *progress.Meter[RunProgress, *RunProgress]) func(receipt.Usage) { return m.Usage }

// runLineWriter emits each line of a run's log with its id.
func (r *RunAPI) runLineWriter(runID string) *lineEventWriter {
	return &lineEventWriter{ctx: r.app, event: eventRunLine, wrap: func(line string) any { return RunLine{RunID: runID, Line: line} }}
}

var errNoRun = errors.New("no run is under way on this computer")

// StopRun ends the run on this computer (D5): its context is cancelled, the
// engine tears the run down and writes its receipts as far as it got, and
// RunAgent returns its result. It stops a run and nothing else: a draft, a
// knowledge build and a chat each have their own way to end.
func (r *RunAPI) StopRun() error {
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.live == nil || r.live.kind != jobRun {
		return errNoRun
	}
	r.live.cancel()
	return nil
}

// ForgetRun makes the app forget a run on this computer once its card is
// dismissed (D7): its folder is dropped, so RunOutputs and ShowRunFolder
// refuse its id afterwards. The folder itself, its receipts included, is
// not touched. Quitting forgets every run, since nothing of one is kept.
func (r *RunAPI) ForgetRun(runID string) {
	r.foldersMu.Lock()
	defer r.foldersMu.Unlock()
	delete(r.folders, runID)
}

// runCtx is the run's context, the parent of everything the engine starts
// for it.
func (r *RunAPI) runCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(r.app.ctx)
}
