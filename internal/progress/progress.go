// Package progress is how far a long job of the app's own has got, for the
// progress dialog and the corner card it shrinks to (agentry-desktop#352,
// #410; plan desktop-review-key-and-run-cards.md §5.1): the report every job
// carries, and the meter that fills it from the engine's usage reports and
// from what the job itself counts. A draft and a knowledge build use it
// (agentry-desktop#464); a run and a chat will.
package progress

import (
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// Report is what every job's progress carries. A job embeds it beside its
// own facts (a draft its stage, a build its pages), so the JSON the page
// reads is one flat object.
type Report struct {
	// StartedAt is when the job's clock started, in Unix milliseconds: the
	// page counts the elapsed time itself. For most jobs that is when they
	// began; a run on this computer starts it again once it is prepared.
	StartedAt int64 `json:"startedAt"`
	// TimeoutSeconds and MaxTurns are the job's budgets. Zero is no limit.
	TimeoutSeconds int `json:"timeoutSeconds"`
	MaxTurns       int `json:"maxTurns"`
	// Turns, InputTokens and OutputTokens are the harness's usage as it
	// last reported it.
	Turns        int   `json:"turns"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
}

// shared is the report inside a job's progress: embedding Report promotes
// it, which is how the meter reaches the report in whatever embeds it.
func (r *Report) shared() *Report { return r }

// Start is a job's report as it begins: now, under its budgets.
func Start(timeoutSeconds, maxTurns int) Report {
	return Report{StartedAt: time.Now().UnixMilli(), TimeoutSeconds: timeoutSeconds, MaxTurns: maxTurns}
}

// Meter reports a job's progress, T, under one lock, so that a report made
// from the engine's usage and one made by the job's own watcher are each
// whole and arrive in the order they were made. With no report function
// every method does nothing. The report function is called under the lock
// and must not block: the engine's usage hook waits on it.
//
// T embeds Report: that is how the meter reaches the shared fields, through
// Report's unexported shared method, and a T that does not embed it does not
// satisfy P.
type Meter[T any, P interface {
	*T
	shared() *Report
}] struct {
	report func(T)
	mu     sync.Mutex
	p      T
}

// NewMeter starts a meter at p, reporting to report.
func NewMeter[T any, P interface {
	*T
	shared() *Report
}](report func(T), p T) *Meter[T, P] {
	return &Meter[T, P]{report: report, p: p}
}

// Reporting says whether anyone is told the job's progress, for a job
// whose own watcher need not run when no one is.
func (m *Meter[T, P]) Reporting() bool { return m.report != nil }

// Emit reports the job's progress as it stands.
func (m *Meter[T, P]) Emit() {
	if m.report == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report(m.p)
}

// Usage is the engine's OnUsage: the harness's usage replaces the last.
func (m *Meter[T, P]) Usage(u receipt.Usage) {
	if m.report == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := P(&m.p).shared()
	r.Turns, r.InputTokens, r.OutputTokens = u.Turns, u.InputTokens, u.OutputTokens
	m.report(m.p)
}

// Set changes the job's own facts under the lock, and reports them when set
// says it changed something.
func (m *Meter[T, P]) Set(set func(*T) bool) {
	if m.report == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if set(&m.p) {
		m.report(m.p)
	}
}
