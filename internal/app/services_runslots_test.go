package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func runID(n int) string { return fmt.Sprintf("ab12cd%02d", n) }

// startN starts n runs, one after the other, each once the one before it has
// begun or queued, so their ids are minted in order.
func (q *queueRig) startN(t *testing.T, n int) []<-chan error {
	t.Helper()
	var out []<-chan error
	for i := 0; i < n; i++ {
		before := len(q.runs()) + len(q.r.QueuedRuns())
		out = append(out, q.start())
		waitUntil(t, "a run begun or queued", func() bool { return len(q.runs())+len(q.r.QueuedRuns()) == before+1 })
	}
	return out
}

func (q *queueRig) running() []string {
	q.r.queueMu.Lock()
	defer q.r.queueMu.Unlock()
	var ids []string
	for _, s := range q.r.running {
		ids = append(ids, s.id)
	}
	return ids
}

// Runs share the slots: up to the number at once run, the rest wait in the
// order they were started, and a slot that comes free goes to the head of
// the queue. Stop stops the run it names and nothing else, and a job other
// than a run is refused while runs are under way.
func TestRunsShareTheSlotsInTheOrderTheyWereStarted(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	r.setRunsAtOnce(3)
	done := q.startN(t, 5)

	if got := q.runs(); !slices.Equal(got, []string{runID(1), runID(2), runID(3)}) {
		t.Fatalf("running: %v", got)
	}
	views := r.QueuedRuns()
	if ids := queuedIDs(r); !slices.Equal(ids, []string{runID(4), runID(5)}) {
		t.Fatalf("queued: %v", ids)
	}
	if views[0].WaitingFor != "One of the 3 runs under way" || views[1].Position != 2 {
		t.Errorf("the queue's views: %+v", views)
	}
	if v := r.InFlight(); v == nil || v.Kind != string(jobRun) || v.Line != "3 runs" {
		t.Errorf("InFlight with three runs: %+v", v)
	}
	_, err := r.RemoveAgentryImages()
	if want := "3 runs are under way on this computer, and this job runs only when none is: wait for them and the 2 runs waiting after them to finish, or stop them."; err == nil || !errors.Is(err, errBusy) || err.Error() != want {
		t.Fatalf("a setup step while runs are under way: %v\nwant %q", err, want)
	}

	// The second run ends: the head of the queue takes its slot.
	q.finish(runID(2))
	if err := outcome(t, done[1]); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "run 4 started", func() bool { return len(q.runs()) == 4 })
	if got := q.runs()[3]; got != runID(4) {
		t.Fatalf("the slot went to %s", got)
	}

	// Stop names run 3 alone; run 5 takes its slot.
	if err := r.StopRun(runID(3)); err != nil {
		t.Fatal(err)
	}
	if err := outcome(t, done[2]); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "run 5 started", func() bool { return len(q.runs()) == 5 })
	if got := q.running(); !slices.Equal(got, []string{runID(1), runID(4), runID(5)}) {
		t.Fatalf("running after the stop: %v", got)
	}
	if err := r.StopRun(runID(3)); !errors.Is(err, errNoRun) {
		t.Errorf("Stop of a run that has ended: %v", err)
	}

	for _, i := range []int{1, 4, 5} {
		q.finish(runID(i))
		if err := outcome(t, done[i-1]); err != nil {
			t.Fatal(err)
		}
	}
	if v := r.InFlight(); v != nil {
		t.Errorf("InFlight after every run: %+v", v)
	}
}

// Raising the number starts the runs waiting at its head at once; lowering
// it stops nothing, and a run started then waits until fewer than the new
// number are under way.
func TestChangingRunsAtOnceStopsNothing(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	done := q.startN(t, 3)
	if len(q.runs()) != 1 || len(r.QueuedRuns()) != 2 {
		t.Fatalf("at one: running %v, queued %v", q.runs(), queuedIDs(r))
	}
	r.setRunsAtOnce(3)
	waitUntil(t, "the waiting runs started", func() bool { return len(q.runs()) == 3 })
	if len(r.QueuedRuns()) != 0 {
		t.Fatalf("queued after raising: %v", queuedIDs(r))
	}

	r.setRunsAtOnce(1)
	if got := q.running(); len(got) != 3 {
		t.Fatalf("lowering stopped a run: %v", got)
	}
	fourth := q.start()
	waitUntil(t, "run 4 queued", func() bool { return len(r.QueuedRuns()) == 1 })
	for i := 1; i <= 2; i++ {
		q.finish(runID(i))
		if err := outcome(t, done[i-1]); err != nil {
			t.Fatal(err)
		}
	}
	if len(q.runs()) != 3 {
		t.Fatalf("run 4 started with %d runs under way at a limit of one", len(q.running()))
	}
	q.finish(runID(3))
	if err := outcome(t, done[2]); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "run 4 started", func() bool { return len(q.runs()) == 4 })
	q.finish(runID(4))
	if err := outcome(t, fourth); err != nil {
		t.Fatal(err)
	}
	r.setRunsAtOnce(0)
	r.queueMu.Lock()
	n := r.slotLimitLocked()
	r.queueMu.Unlock()
	if n != 1 {
		t.Errorf("a limit of zero reads as %d", n)
	}
}

// A job other than a run holds the lock alone: runs started meanwhile wait,
// and as it ends they take every free slot at once, in order.
func TestRunsWaitingForAnotherJobTakeEverySlotAsItEnds(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	r.setRunsAtOnce(2)
	r.holdLock()
	done := q.startN(t, 3)
	if len(q.runs()) != 0 {
		t.Fatalf("a run started beside another job: %v", q.runs())
	}
	if v := r.QueuedRuns()[0]; v.WaitingFor != "Another job" {
		t.Errorf("the head waits for %q", v.WaitingFor)
	}
	r.release()
	waitUntil(t, "two runs started", func() bool { return len(q.runs()) == 2 })
	// Granted in order; the two then reach the engine in either order.
	if got := q.running(); !slices.Equal(got, []string{runID(1), runID(2)}) {
		t.Fatalf("given slots: %v", got)
	}
	if ids := queuedIDs(r); !slices.Equal(ids, []string{runID(3)}) {
		t.Fatalf("still queued: %v", ids)
	}
	for i := 1; i <= 3; i++ {
		waitUntil(t, "run started", func() bool { return len(q.runs()) >= i })
		q.finish(runID(i))
		if err := outcome(t, done[i-1]); err != nil {
			t.Fatal(err)
		}
	}
}

// Twenty runs at once, and five waiting: quitting cancels the five first,
// then ends the twenty and sweeps each by its own id.
func TestQuittingEndsAndSweepsEveryRun(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	r := q.r
	var mu sync.Mutex
	var swept []string
	r.sweep = func(_ context.Context, id string) error {
		mu.Lock()
		defer mu.Unlock()
		swept = append(swept, id)
		return nil
	}
	r.setRunsAtOnce(20)
	done := q.startN(t, 25)
	if len(q.runs()) != 20 || len(r.QueuedRuns()) != 5 {
		t.Fatalf("running %d, queued %d", len(q.runs()), len(r.QueuedRuns()))
	}
	if note := quitNote(r.liveNow(), r.queuedCount()); !strings.HasPrefix(note, "20 runs are in progress.") || !strings.Contains(note, "The 5 runs waiting to start after them are cancelled.") {
		t.Errorf("the close dialog: %q", note)
	}

	r.shutdown()

	for i, d := range done {
		err := outcome(t, d)
		if i >= 20 && !errors.Is(err, errQueueClosed) {
			t.Errorf("queued run %d at quit: %v", i+1, err)
		}
	}
	if len(q.runs()) != 20 {
		t.Fatalf("a queued run started as the app quit: %d ran", len(q.runs()))
	}
	for i := 1; i <= 20; i++ {
		if !slices.Contains(swept, runID(i)) {
			t.Errorf("run %d was not swept: %v", i, swept)
		}
	}
	if l := r.liveNow(); len(l) != 0 {
		t.Errorf("%d runs still recorded as live", len(l))
	}
}

// The close dialog names one run, or counts several, and the runs waiting.
func TestTheCloseDialogCountsTheRuns(t *testing.T) {
	one := []*liveRun{{ref: "acme/agent@1.0.0"}}
	two := []*liveRun{{ref: "a"}, {ref: "b"}}
	for _, c := range []struct {
		lives  []*liveRun
		queued int
		want   string
	}{
		{one, 0, "A run of acme/agent@1.0.0 is in progress. Quitting stops it and removes its containers; its receipts are written as far as it got. Quit anyway?"},
		{one, 1, "A run of acme/agent@1.0.0 is in progress. Quitting stops it and removes its containers; its receipts are written as far as it got. The run waiting to start after it is cancelled. Quit anyway?"},
		{two, 0, "2 runs are in progress. Quitting stops them and removes their containers; their receipts are written as far as each got. Quit anyway?"},
		{two, 3, "2 runs are in progress. Quitting stops them and removes their containers; their receipts are written as far as each got. The 3 runs waiting to start after them are cancelled. Quit anyway?"},
	} {
		if got := quitNote(c.lives, c.queued); got != c.want {
			t.Errorf("quitNote(%d, %d):\n got %q\nwant %q", len(c.lives), c.queued, got, c.want)
		}
	}
}

// Unsafe Mode's edge stays while any run in Unsafe Mode is live: one ending
// does not take it from another.
func TestTheUnsafeEdgeStaysWhileAnyUnsafeRunLives(t *testing.T) {
	ev := captureEvents(t)
	r := flowAPI(t, nil)
	a, _ := r.beginRunAs(jobRun, "acme/a@1.0.0", "ab12cd01", func() {})
	b, _ := r.beginRunAs(jobRun, "acme/b@1.0.0", "ab12cd02", func() {})
	r.setUnsafeLive(a, true)
	r.setUnsafeLive(b, true)
	r.endRun(a)
	if !r.UnsafeLive() {
		t.Fatal("the edge went with one unsafe run still live")
	}
	r.setUnsafeLive(b, false)
	if r.UnsafeLive() {
		t.Fatal("the edge stayed after the last run's compile said safe")
	}
	r.endRun(b)
	ev.mu.Lock()
	var said []bool
	for _, g := range ev.got {
		if g.name == eventUnsafeLive {
			said = append(said, g.data.(bool))
		}
	}
	ev.mu.Unlock()
	if !slices.Equal(said, []bool{true, false}) {
		t.Errorf("the shell heard %v, want [true false]", said)
	}
}

// A run cancelled while it waits is never handed a slot, even when the slot
// comes free before its own wait sees the cancel: the next run takes it.
func TestACancelledRunIsNeverHandedASlot(t *testing.T) {
	captureEvents(t)
	r := flowAPI(t, nil)
	r.holdLock()
	cancelled := &queuedRun{id: runID(1), what: "a run of a", granted: make(chan struct{}), cancel: make(chan struct{})}
	next := &queuedRun{id: runID(2), what: "a run of b", granted: make(chan struct{}), cancel: make(chan struct{})}
	r.queue = []*queuedRun{cancelled, next}
	if err := r.CancelQueuedRun(runID(1)); err != nil {
		t.Fatal(err)
	}
	r.release()
	select {
	case <-cancelled.granted:
		t.Fatal("the cancelled run was handed a slot")
	default:
	}
	select {
	case <-next.granted:
	default:
		t.Fatal("the run after it was not handed the slot")
	}
	if len(r.running) != 1 || r.running[0].id != runID(2) || len(r.queue) != 0 {
		t.Fatalf("running %+v, queued %d", r.running, len(r.queue))
	}
}

// One run at a time uses the computer: a second waits until the first's
// record ends, a run stopped while it waits gives up, and a bind that is not
// one of this app's runs takes nothing.
func TestOneRunAtATimeUsesTheComputer(t *testing.T) {
	r := flowAPI(t, nil)
	a, _ := r.beginRunAs(jobRun, "acme/a@1.0.0", runID(1), func() {})
	b, _ := r.beginRunAs(jobRun, "acme/b@1.0.0", runID(2), func() {})
	if _, err := r.holdComputer(context.Background(), a.id); err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		_, err := r.holdComputer(context.Background(), b.id)
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("a second run took the computer beside the first: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := r.beginRunAs(jobRun, "acme/c@1.0.0", runID(3), func() {})
	if _, err := r.holdComputer(ctx, c.id); !errors.Is(err, errComputerWaitStopped) {
		t.Fatalf("a run stopped while it waited: %v", err)
	}
	if release, err := r.holdComputer(context.Background(), "not-a-run"); err != nil || release == nil {
		t.Fatalf("a bind that is no run of this app: %v", err)
	}
	r.endRun(a)
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second run never got the computer")
	}
	r.endRun(b)
	r.endRun(c)
	if _, err := r.holdComputer(context.Background(), runID(9)); err != nil {
		t.Fatal(err)
	}
	select {
	case computerSlot <- struct{}{}:
		<-computerSlot
	default:
		t.Fatal("the computer was not given back")
	}
}
