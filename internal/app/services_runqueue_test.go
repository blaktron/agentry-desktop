package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// queueRig is a RunAPI whose engine records each run it is given and holds
// it until the test lets it end.
type queueRig struct {
	r       *RunAPI
	mu      sync.Mutex
	ran     []string
	next    int
	release map[string]chan struct{}
}

func newQueueRig(t *testing.T) *queueRig {
	t.Helper()
	q := &queueRig{release: map[string]chan struct{}{}}
	q.r = flowAPI(t, func(ctx context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		q.mu.Lock()
		q.ran = append(q.ran, req.RunID)
		ch := q.gate(req.RunID)
		q.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return &run.Result{Status: "ok", RunID: req.RunID}, nil
	})
	q.r.newRunID = func() (string, error) {
		q.mu.Lock()
		defer q.mu.Unlock()
		q.next++
		return fmt.Sprintf("ab12cd%02d", q.next), nil
	}
	return q
}

// gate is run id's release channel. The caller holds mu.
func (q *queueRig) gate(id string) chan struct{} {
	ch, ok := q.release[id]
	if !ok {
		ch = make(chan struct{})
		q.release[id] = ch
	}
	return ch
}

// finish lets run id's engine return.
func (q *queueRig) finish(id string) {
	q.mu.Lock()
	ch := q.gate(id)
	q.mu.Unlock()
	close(ch)
}

func (q *queueRig) runs() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.ran...)
}

// start runs a run in the background, its outcome on the channel returned.
func (q *queueRig) start() <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := runAgent(q.r)
		done <- err
	}()
	return done
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func queuedIDs(r *RunAPI) []string {
	var ids []string
	for _, v := range r.QueuedRuns() {
		ids = append(ids, v.RunID)
	}
	return ids
}

func outcome(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not return")
		return nil
	}
}

// A run started while another job holds the run lock waits its turn: runs
// start in the order they were started, each as the job before it ends, and
// a job other than a run asked for meanwhile is refused, naming the job and
// the runs waiting, so no queued run is overtaken.
func TestARunStartedWhileAnotherJobRunsWaitsItsTurn(t *testing.T) {
	ev := captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	if err := r.acquire(jobSetup, "the container engine install"); err != nil {
		t.Fatal(err)
	}
	first := q.start()
	waitUntil(t, "the first run queued", func() bool { return len(r.QueuedRuns()) == 1 })
	second := q.start()
	waitUntil(t, "the second run queued", func() bool { return len(r.QueuedRuns()) == 2 })

	views := r.QueuedRuns()
	for i, v := range views {
		if v.RunID != fmt.Sprintf("ab12cd%02d", i+1) || v.Position != i+1 || v.Ref != "acme/agent@1.0.0" || v.WaitingFor != "The container engine install" {
			t.Errorf("queued run %d: %+v", i, v)
		}
	}
	if len(q.runs()) != 0 {
		t.Fatalf("a queued run reached the engine: %v", q.runs())
	}
	_, err := r.RemoveAgentryImages()
	if want := "The container engine install is under way on this computer, and one job runs here at a time: wait for it and the 2 runs waiting after it to finish."; err == nil || err.Error() != want {
		t.Fatalf("a setup while runs wait: %v, want %q", err, want)
	}

	r.release()
	waitUntil(t, "the first run started", func() bool { return len(q.runs()) == 1 })
	if got := q.runs()[0]; got != "ab12cd01" {
		t.Fatalf("the first to start was %s", got)
	}
	if ids := queuedIDs(r); len(ids) != 1 || ids[0] != "ab12cd02" {
		t.Fatalf("queued after the first started: %v", ids)
	}
	if v := r.QueuedRuns()[0]; v.Position != 1 || v.WaitingFor != "A run of acme/agent@1.0.0" {
		t.Errorf("the second run's place: %+v", v)
	}
	if v := r.InFlight(); v == nil || v.Kind != string(jobRun) {
		t.Errorf("InFlight while the first run runs: %+v", v)
	}

	q.finish("ab12cd01")
	if err := outcome(t, first); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the second run started", func() bool { return len(q.runs()) == 2 })
	if len(r.QueuedRuns()) != 0 {
		t.Errorf("queued after the second started: %+v", r.QueuedRuns())
	}
	q.finish("ab12cd02")
	if err := outcome(t, second); err != nil {
		t.Fatal(err)
	}
	if v := r.InFlight(); v != nil {
		t.Errorf("InFlight after both runs: %+v", v)
	}
	// The lock is free again.
	if err := r.acquire(jobSetup, "x"); err != nil {
		t.Fatalf("the lock after the queue emptied: %v", err)
	}
	r.release()

	// The page heard the queue each time it changed, the last time empty.
	ev.mu.Lock()
	var reports [][]QueuedRunView
	for _, g := range ev.got {
		if g.name == eventRunQueue {
			reports = append(reports, g.data.([]QueuedRunView))
		}
	}
	ev.mu.Unlock()
	if len(reports) < 3 || len(reports[0]) != 1 || len(reports[1]) != 2 || len(reports[len(reports)-1]) != 0 {
		t.Errorf("the queue's reports: %+v", reports)
	}
}

// Cancel on a queued run takes it out of the queue without starting it, and
// leaves the job running and the runs after it as they were. A run that has
// started is not cancelled this way.
func TestACancelledQueuedRunLeavesTheRestAsTheyWere(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	running := q.start()
	waitUntil(t, "the running run", func() bool { return len(q.runs()) == 1 })
	a := q.start()
	waitUntil(t, "a queued", func() bool { return len(r.QueuedRuns()) == 1 })
	b := q.start()
	waitUntil(t, "b queued", func() bool { return len(r.QueuedRuns()) == 2 })

	if err := r.CancelQueuedRun("ab12cd02"); err != nil {
		t.Fatal(err)
	}
	if err := outcome(t, a); !errors.Is(err, errQueueCancelled) {
		t.Fatalf("the cancelled run: %v", err)
	}
	if ids := queuedIDs(r); len(ids) != 1 || ids[0] != "ab12cd03" || r.QueuedRuns()[0].Position != 1 {
		t.Fatalf("queued after the cancel: %+v", r.QueuedRuns())
	}
	if len(q.runs()) != 1 {
		t.Fatalf("the cancel touched the running run: %v", q.runs())
	}
	if err := r.CancelQueuedRun("ab12cd01"); err == nil {
		t.Error("a running run was cancelled as if queued")
	}

	q.finish("ab12cd01")
	if err := outcome(t, running); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "b started", func() bool { return len(q.runs()) == 2 })
	if got := q.runs()[1]; got != "ab12cd03" {
		t.Fatalf("started after the cancel: %s", got)
	}
	q.finish("ab12cd03")
	if err := outcome(t, b); err != nil {
		t.Fatal(err)
	}
}

// Quitting cancels every queued run before the run in flight ends, so none
// is handed the lock; a run started after is refused.
func TestQuittingCancelsTheQueuedRuns(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	if err := r.acquire(jobChat, "a chat with acme/agent@1.0.0"); err != nil {
		t.Fatal(err)
	}
	a := q.start()
	waitUntil(t, "a queued", func() bool { return len(r.QueuedRuns()) == 1 })
	if n := r.closeQueue(); n != 1 {
		t.Fatalf("closeQueue cancelled %d", n)
	}
	if err := outcome(t, a); !errors.Is(err, errQueueClosed) {
		t.Fatalf("the queued run at quit: %v", err)
	}
	r.release()
	if len(q.runs()) != 0 {
		t.Fatalf("a run started as the app quit: %v", q.runs())
	}
	if _, err := runAgent(r); !errors.Is(err, errQueueClosed) || !strings.Contains(err.Error(), "quitting") {
		t.Fatalf("a run after the queue closed: %v", err)
	}
}
