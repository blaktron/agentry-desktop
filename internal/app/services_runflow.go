// A run on this computer followed above the pages: the page's store follows
// the run by its id through runner:run-progress and runner:run-line, Stop
// ends it (StopRun, which stops a run and nothing else), the busy refusal
// and InFlight say which job holds the run lock, and Dismiss removes the
// run's card (ForgetRun; the run history keeps the run).
package app

import (
	"errors"
	"fmt"
	"strings"
	"sync"

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

// jobKind is the kind of local job the run lock is held for.
type jobKind string

const (
	jobRun   jobKind = "run"
	jobChat  jobKind = "chat"
	jobDraft jobKind = "draft"
	jobBuild jobKind = "build"
	// jobReview is a review by the Examiner.
	jobReview jobKind = "review"
	jobSetup  jobKind = "setup"
)

// jobHolder is what holds the run lock: its kind and a noun phrase naming
// it, which the busy refusal makes the subject of its sentence.
type jobHolder struct {
	kind jobKind
	what string
}

// busyError refuses a second local job, naming the one that holds the run
// lock. It is errBusy for errors.Is.
type busyError struct{ msg string }

func (e busyError) Error() string        { return e.msg }
func (e busyError) Is(target error) bool { return target == errBusy }

// acquire takes the run lock for a job of kind, named by what, or refuses
// with the job that holds it. A run waits for it instead (acquireRun).
func (r *RunAPI) acquire(kind jobKind, what string) error {
	if !r.mu.TryLock() {
		return r.busy()
	}
	r.setHolder(&jobHolder{kind: kind, what: what})
	return nil
}

// setHolder records what holds the run lock; nil when nothing does.
func (r *RunAPI) setHolder(h *jobHolder) {
	r.holderMu.Lock()
	r.holder = h
	r.holderMu.Unlock()
}

// release gives the run lock back, or hands it to the first run waiting
// for it, which the lock is never free between: a job that asks meanwhile
// is refused, and no queued run is overtaken.
func (r *RunAPI) release() {
	r.queueMu.Lock()
	if len(r.queue) > 0 {
		q := r.queue[0]
		r.queue = r.queue[1:]
		r.setHolder(&jobHolder{kind: jobRun, what: q.what})
		close(q.granted)
		r.queueMu.Unlock()
		r.emitQueue()
		return
	}
	r.setHolder(nil)
	r.queueMu.Unlock()
	r.mu.Unlock()
}

// busy is the refusal while another job holds the run lock.
func (r *RunAPI) busy() error {
	r.queueMu.Lock()
	waiting := len(r.queue)
	r.queueMu.Unlock()
	r.holderMu.Lock()
	h := r.holder
	r.holderMu.Unlock()
	what, how := "another job", ""
	if h != nil {
		what = h.what
		switch h.kind {
		case jobRun, jobDraft, jobBuild, jobReview:
			how = ", or stop it"
		case jobChat:
			how = ", or end it"
		}
	}
	after := ""
	switch {
	case waiting == 1:
		after = " and the run waiting after it"
	case waiting > 1:
		after = fmt.Sprintf(" and the %d runs waiting after it", waiting)
	}
	return busyError{msg: fmt.Sprintf("%s is under way on this computer, and one job runs here at a time: wait for it%s to finish%s.", capitalize(what), after, how)}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// InFlightView names the local job holding the run lock.
type InFlightView struct {
	// Kind is run, chat, draft, build, review or setup.
	Kind string `json:"kind"`
	// Line names the job in a sentence's subject position.
	Line string `json:"line"`
}

// InFlight names the local job holding the run lock, or nil when there is
// none.
func (r *RunAPI) InFlight() *InFlightView {
	r.holderMu.Lock()
	defer r.holderMu.Unlock()
	if r.holder == nil {
		return nil
	}
	return &InFlightView{Kind: string(r.holder.kind), Line: capitalize(r.holder.what)}
}

// RunProgress is how far a run on this computer has got, for the progress
// dialog and its card: the report every job carries, the run's id and the
// release. Its budgets are the run's effective ones, known once the run is
// prepared; until then they are zero.
type RunProgress struct {
	progress.Report
	RunID string `json:"runId"`
	Ref   string `json:"ref"`
	// Prepared says the run's install, plan and images are ready and the
	// engine is starting: its budgets are known from here, and StartedAt is
	// this moment, so the clock measures the run against its own timeout.
	Prepared bool `json:"prepared"`
}

// RunLine is one line of a run's log, with the run's id.
type RunLine struct {
	RunID string `json:"runId"`
	Line  string `json:"line"`
}

// runMeter is a run's progress meter: its first report says the run has
// begun, before anything is prepared; then its budgets, when the engine
// starts; then the harness's usage.
func (r *RunAPI) runMeter(runID, ref string) *progress.Meter[RunProgress, *RunProgress] {
	emit := func(p RunProgress) { emitEvent(r.app, eventRunProgress, p) }
	return progress.NewMeter(emit, RunProgress{Report: progress.Start(0, 0), RunID: runID, Ref: ref})
}

// budgetsOf is internal/run's OnBudgets for a run's meter: the run is
// prepared, and its clock starts now.
func budgetsOf(m *progress.Meter[RunProgress, *RunProgress]) func(int, int) {
	return func(timeoutSeconds, maxTurns int) {
		m.Set(func(p *RunProgress) bool {
			p.Report = progress.Start(timeoutSeconds, maxTurns)
			p.Prepared = true
			return true
		})
	}
}

// usageOf is the engine's usage hook for a run's meter. The engine calls it
// on the run's own path, so it only hands the usage over: a goroutine
// reports it, and a report the page is slow to take is replaced by the next
// one rather than waited for. stop, when the run has returned, reports the
// last usage handed over and ends the goroutine.
func usageOf(m *progress.Meter[RunProgress, *RunProgress]) (hook func(receipt.Usage), stop func()) {
	next := make(chan receipt.Usage, 1)
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for u := range next {
			m.Usage(u)
		}
	}()
	hook = func(u receipt.Usage) {
		mu.Lock()
		defer mu.Unlock()
		select {
		case <-next: // the page has not taken the last one: this one replaces it
		default:
		}
		next <- u
	}
	stop = func() {
		mu.Lock()
		close(next)
		mu.Unlock()
		<-done
	}
	return hook, stop
}

// runLineWriter emits each line of a run's log with its id.
func (r *RunAPI) runLineWriter(runID string) *lineEventWriter {
	return &lineEventWriter{ctx: r.app, event: eventRunLine, wrap: func(line string) any { return RunLine{RunID: runID, Line: line} }}
}

var errNoRun = errors.New("no run is under way on this computer")

// StopRun ends the run on this computer: its context is cancelled, the
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

// ForgetRun is Dismiss on a run's card: this process's memory of the run's
// folder is dropped, and the folder itself, its receipts included, is not
// touched. The run history keeps a run that has ended, so while Remember
// recent runs is on RunOutputs and ShowRunFolder still read its id through
// the history, for the run page; with it off they refuse the id afterwards,
// and quitting forgets every run.
func (r *RunAPI) ForgetRun(runID string) {
	r.foldersMu.Lock()
	delete(r.folders, runID)
	r.foldersMu.Unlock()
	// A run In Agentry the app lost touch with is not followed again once
	// its card is dismissed.
	r.hostedMu.Lock()
	for k := range r.lostDirs {
		if k.id == runID {
			delete(r.lostDirs, k)
		}
	}
	r.hostedMu.Unlock()
}
