// The run slots and the runs waiting for one. Runs on this computer (and the
// runs this app sends to a runhost) share a number of slots, runsAtOnce: up
// to that many run at the same time. Every other local job (a chat, a draft,
// a build, a review, a setup step) runs alone: it is refused while any run is
// running or waiting, and while it holds the lock no run starts.
//
// A run started while every slot is taken, or while another job holds the
// lock, is queued instead of refused, and starts once a slot is free, in the
// order the runs were started: a free slot always goes to the head of the
// queue, so no run is overtaken. Its gates (signature, scan, consents,
// inputs, output folder) are passed before it waits, and nothing of it is
// created until it starts. The page hears the queue as runner:run-queue, and
// Cancel on a queued run's card is CancelQueuedRun. Quitting cancels every
// queued run.
package app

import (
	"errors"
	"fmt"
	"slices"
)

// eventRunQueue carries the runs waiting for a slot ([]QueuedRunView), all
// of them, each time the queue changes.
const eventRunQueue = "runner:run-queue"

// errQueueCancelled is a queued run's end when it is cancelled before it
// started.
var errQueueCancelled = errors.New("the run was cancelled before it started")

// errQueueClosed refuses a run as the app quits.
var errQueueClosed = errors.New("the app is quitting, so the run was not started")

// queuedRun is a run waiting for a slot.
type queuedRun struct {
	id, ref string
	// what names the run as a slot's holder will.
	what string
	// granted is closed when the run is given a slot.
	granted chan struct{}
	// cancel ends the wait; dropped says the app's quitting took the run
	// out of the queue. Under queueMu.
	cancel  chan struct{}
	dropped bool
}

// runSlot is a run holding a slot.
type runSlot struct {
	id   string
	what string
}

// QueuedRunView is one run waiting for a slot, for its card.
type QueuedRunView struct {
	RunID string `json:"runId"`
	Ref   string `json:"ref"`
	// Position is its place in the queue, 1 for the next to start.
	Position int `json:"position"`
	// WaitingFor names what the queue waits for now, in a sentence's
	// subject position: the job holding the lock, the one run under way, or
	// "One of the N runs under way".
	WaitingFor string `json:"waitingFor"`
}

// slotLimitLocked is how many runs may run at once: runsAtOnce, at least
// one, so an unset or nonsensical number runs one. The caller holds queueMu.
func (r *RunAPI) slotLimitLocked() int {
	if r.runsAtOnce < 1 {
		return 1
	}
	return r.runsAtOnce
}

// slotFreeLocked says whether a run could start now: no other job holds the
// lock and a slot is free. The caller holds queueMu.
func (r *RunAPI) slotFreeLocked() bool {
	return r.exclusive == nil && len(r.running) < r.slotLimitLocked()
}

// acquireRun takes a slot for run id of ref, waiting behind the runs and
// the job that hold the slots and the runs queued before this one. queued,
// when given, is told once the run waits. It returns errQueueCancelled when
// the run is cancelled while it waits, and errQueueClosed as the app quits;
// on nil the caller holds a slot and gives it back with releaseRun(id).
func (r *RunAPI) acquireRun(id, ref string, queued func()) error {
	what := "a run of " + ref
	r.queueMu.Lock()
	if r.queueClosed {
		r.queueMu.Unlock()
		return errQueueClosed
	}
	// A slot is taken here only when no run waits: one that waits holds its
	// place, and a slot passes to it straight from the run before.
	if len(r.queue) == 0 && r.slotFreeLocked() {
		r.running = append(r.running, runSlot{id: id, what: what})
		r.queueMu.Unlock()
		return nil
	}
	q := &queuedRun{id: id, ref: ref, what: what, granted: make(chan struct{}), cancel: make(chan struct{})}
	r.queue = append(r.queue, q)
	r.queueMu.Unlock()
	r.emitQueue()
	if queued != nil {
		queued()
	}
	select {
	case <-q.granted:
		return nil
	case <-q.cancel:
	}
	r.queueMu.Lock()
	if q.dropped {
		r.queueMu.Unlock()
		return errQueueClosed
	}
	if i := slices.Index(r.queue, q); i >= 0 {
		r.queue = slices.Delete(r.queue, i, i+1)
		r.queueMu.Unlock()
		r.emitQueue()
		return errQueueCancelled
	}
	r.queueMu.Unlock()
	select {
	case <-q.granted:
		// The slot was handed over just before the cancel came: the run has
		// started, and its card has Stop.
		return nil
	default:
		// grantLocked took the cancelled run out of the queue without a slot.
		return errQueueCancelled
	}
}

// releaseRun gives run id's slot back, and hands every free slot to the
// runs at the head of the queue.
func (r *RunAPI) releaseRun(id string) {
	r.queueMu.Lock()
	if i := slices.IndexFunc(r.running, func(s runSlot) bool { return s.id == id }); i >= 0 {
		r.running = slices.Delete(r.running, i, i+1)
	}
	granted := r.grantLocked()
	r.queueMu.Unlock()
	if granted {
		r.emitQueue()
	}
}

// grantLocked hands free slots to the runs at the head of the queue, in
// order, and says whether the queue changed. The caller holds queueMu.
// A run cancelled while it waited is taken out without a slot, so Cancel
// never lets a run start.
func (r *RunAPI) grantLocked() bool {
	changed := false
	for len(r.queue) > 0 && r.slotFreeLocked() {
		q := r.queue[0]
		r.queue = r.queue[1:]
		changed = true
		select {
		case <-q.cancel:
			continue
		default:
		}
		r.running = append(r.running, runSlot{id: q.id, what: q.what})
		close(q.granted)
	}
	return changed
}

// setRunsAtOnce sets how many runs may run at once, at least one. Raising
// it starts the runs waiting at the head of the queue at once; lowering it
// stops nothing, and the runs under way finish.
func (r *RunAPI) setRunsAtOnce(n int) {
	r.queueMu.Lock()
	r.runsAtOnce = n
	granted := r.grantLocked()
	r.queueMu.Unlock()
	if granted {
		r.emitQueue()
	}
}

// CancelQueuedRun is Cancel on a queued run's card: the run leaves the
// queue without starting, and the runs under way and the runs after it are
// left as they were. A run that has already started is refused: Stop ends
// that.
func (r *RunAPI) CancelQueuedRun(runID string) error {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	for _, q := range r.queue {
		if q.id == runID {
			select {
			case <-q.cancel:
			default:
				close(q.cancel)
			}
			return nil
		}
	}
	return errors.New("that run is not waiting to start: it has started or ended")
}

// QueuedRuns lists the runs waiting for a slot, the next to start first.
func (r *RunAPI) QueuedRuns() []QueuedRunView {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	return r.queueViewsLocked()
}

// queueViewsLocked is QueuedRuns for a caller that holds queueMu.
func (r *RunAPI) queueViewsLocked() []QueuedRunView {
	waitingFor := capitalize(r.holderWhatLocked())
	out := make([]QueuedRunView, 0, len(r.queue))
	for i, q := range r.queue {
		out = append(out, QueuedRunView{RunID: q.id, Ref: q.ref, Position: i + 1, WaitingFor: waitingFor})
	}
	return out
}

// holderWhatLocked names what holds the lock or the slots, in a sentence's
// subject position, lower case: the job holding the lock, the one run under
// way, or one of the runs under way. The caller holds queueMu.
func (r *RunAPI) holderWhatLocked() string {
	switch {
	case r.exclusive != nil:
		return r.exclusive.what
	case len(r.running) == 1:
		return r.running[0].what
	case len(r.running) > 1:
		return fmt.Sprintf("one of the %d runs under way", len(r.running))
	}
	return "another job"
}

// emitQueue tells the page the queue as it is now.
func (r *RunAPI) emitQueue() {
	r.queueEmitMu.Lock()
	defer r.queueEmitMu.Unlock()
	emitEvent(r.app, eventRunQueue, r.QueuedRuns())
}

// closeQueue cancels every queued run as the app quits, and refuses any
// run started after: none is handed a slot as the runs in flight end. It
// returns how many runs it cancelled.
func (r *RunAPI) closeQueue() int {
	r.queueMu.Lock()
	r.queueClosed = true
	dropped := r.queue
	r.queue = nil
	for _, q := range dropped {
		q.dropped = true
		select {
		case <-q.cancel:
		default:
			close(q.cancel)
		}
	}
	r.queueMu.Unlock()
	if len(dropped) > 0 {
		r.emitQueue()
	}
	return len(dropped)
}

// queuedCount is how many runs wait for a slot.
func (r *RunAPI) queuedCount() int {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	return len(r.queue)
}
