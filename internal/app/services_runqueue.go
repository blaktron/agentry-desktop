// The runs waiting for the run lock: a run started while another local job
// holds it is queued instead of refused, and starts once the jobs before it
// have ended, in the order the runs were started. Its gates (signature,
// scan, consents, inputs, output folder) are passed before it waits, and
// nothing of it is created until it starts. The page hears the queue as
// runner:run-queue, and Cancel on a queued run's card is CancelQueuedRun.
// Quitting cancels every queued run.
package app

import (
	"errors"
	"slices"
)

// eventRunQueue carries the runs waiting for the run lock ([]QueuedRunView),
// all of them, each time the queue changes.
const eventRunQueue = "runner:run-queue"

// errQueueCancelled is a queued run's end when it is cancelled before it
// started.
var errQueueCancelled = errors.New("the run was cancelled before it started")

// errQueueClosed refuses a run as the app quits.
var errQueueClosed = errors.New("the app is quitting, so the run was not started")

// queuedRun is a run waiting for the run lock.
type queuedRun struct {
	id, ref string
	// what names the run as the run lock's holder will.
	what string
	// granted is closed when release hands the run the lock.
	granted chan struct{}
	// cancel ends the wait; dropped says the app's quitting took the run
	// out of the queue. Under queueMu.
	cancel  chan struct{}
	dropped bool
}

// QueuedRunView is one run waiting for the run lock, for its card.
type QueuedRunView struct {
	RunID string `json:"runId"`
	Ref   string `json:"ref"`
	// Position is its place in the queue, 1 for the next to start.
	Position int `json:"position"`
	// WaitingFor names the job that holds the run lock now, in a
	// sentence's subject position.
	WaitingFor string `json:"waitingFor"`
}

// acquireRun takes the run lock for run id of ref, waiting behind the job
// that holds it and the runs queued before this one. queued, when given, is
// told once the run waits. It returns errQueueCancelled when the run is
// cancelled while it waits, and errQueueClosed as the app quits; on nil
// the caller holds the lock and gives it back with release.
func (r *RunAPI) acquireRun(id, ref string, queued func()) error {
	what := "a run of " + ref
	r.queueMu.Lock()
	if r.queueClosed {
		r.queueMu.Unlock()
		return errQueueClosed
	}
	// The lock is taken here only when no run waits: one that waits holds
	// its place, and the lock passes to it straight from the job before.
	if len(r.queue) == 0 && r.mu.TryLock() {
		r.setHolder(&jobHolder{kind: jobRun, what: what})
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
	// The lock was handed over as the cancel came: the run has started, and
	// its card has Stop.
	return nil
}

// CancelQueuedRun is Cancel on a queued run's card: the run leaves the
// queue without starting, and the job running and the runs after it are
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

// QueuedRuns lists the runs waiting for the run lock, the next to start
// first.
func (r *RunAPI) QueuedRuns() []QueuedRunView {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	return r.queueViewsLocked()
}

// queueViewsLocked is QueuedRuns for a caller that holds queueMu.
func (r *RunAPI) queueViewsLocked() []QueuedRunView {
	waitingFor := "another job"
	r.holderMu.Lock()
	if r.holder != nil {
		waitingFor = capitalize(r.holder.what)
	}
	r.holderMu.Unlock()
	out := make([]QueuedRunView, 0, len(r.queue))
	for i, q := range r.queue {
		out = append(out, QueuedRunView{RunID: q.id, Ref: q.ref, Position: i + 1, WaitingFor: waitingFor})
	}
	return out
}

// emitQueue tells the page the queue as it is now.
func (r *RunAPI) emitQueue() {
	r.queueEmitMu.Lock()
	defer r.queueEmitMu.Unlock()
	emitEvent(r.app, eventRunQueue, r.QueuedRuns())
}

// closeQueue cancels every queued run as the app quits, and refuses any
// run started after: none is handed the lock as the run in flight ends.
// It returns how many runs it cancelled.
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

// queuedCount is how many runs wait for the run lock.
func (r *RunAPI) queuedCount() int {
	r.queueMu.Lock()
	defer r.queueMu.Unlock()
	return len(r.queue)
}
