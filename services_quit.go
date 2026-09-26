// Quitting mid-run (agentry-desktop#125). A run's containers are torn down
// by the engine when its Run or its session's End returns, deferred on the
// binding's goroutine; if the app simply exits while that goroutine is
// still inside the engine, the teardown never runs and the agent, gateway
// (holding the Docker socket) and proxy containers outlive the app. So the
// app keeps a record of the one run in flight, asks before a close would
// cut it short, and on shutdown ends it and waits — bounded — for the
// engine's teardown, then sweeps the run's label itself in case the wait
// ran out (§9 "teardown on every path").
package main

import (
	"context"
	"io"
	"log"
	"strings"
	"time"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// quitWait bounds how long quitting waits for the engine to tear a run
// down: the engine gives a harness 20s to exit before killing it and its
// own teardown 60s, so 90s covers a run that behaves; one that does not is
// swept by label when the wait runs out.
var quitWait = 90 * time.Second

// sweepWait bounds the label sweep that follows.
const sweepWait = 30 * time.Second

// liveRun is the run or interactive session in flight.
type liveRun struct {
	id  string
	ref string
	// cancel ends the run's engine context: an autonomous run returns and
	// tears down; a session still starting stops starting. A session that
	// has started is ended through its End instead, which lets the harness
	// quit and writes the receipts.
	cancel context.CancelFunc
	// done is closed when the binding that owns the run has returned from
	// the engine (RunAgent), or the session has ended (EndSession).
	done chan struct{}
}

// beginRun records a run about to enter the engine and returns its id and
// the record; endRun clears it.
func (r *RunAPI) beginRun(ref string, cancel context.CancelFunc) (*liveRun, error) {
	id, err := r.mintRunID()
	if err != nil {
		return nil, err
	}
	l := &liveRun{id: id, ref: ref, cancel: cancel, done: make(chan struct{})}
	r.liveMu.Lock()
	r.live = l
	r.liveMu.Unlock()
	// The crash path's record (agentry-desktop#198): written before the
	// engine creates anything, so a kill from here on leaves the next
	// start a run id to sweep. Best effort: a run is not refused because
	// the runner home is unwritable; the record is litter insurance.
	if dir, err := r.inFlight(); err == nil {
		if err := run.RecordInFlight(dir, id); err != nil {
			log.Printf("agentry-desktop: %v", err)
		}
	}
	return l, nil
}

// endRun may be called from both ends of a session (EndSession and
// shutdown): only the call that clears the record closes done.
func (r *RunAPI) endRun(l *liveRun) {
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.live == l {
		r.live = nil
		close(l.done)
		r.forgetInFlight(l.id)
	}
}

// forgetInFlight removes a run's crash-path record once nothing of it can
// be left: the engine returned (its teardown ran) or the run was swept.
func (r *RunAPI) forgetInFlight(id string) {
	if dir, err := r.inFlight(); err == nil {
		if err := run.ForgetInFlight(dir, id); err != nil {
			log.Printf("agentry-desktop: %v", err)
		}
	}
}

// sweepOrphans is the start half of the crash path (agentry-desktop#198):
// a run recorded by an app that died without tearing it down — killed,
// crashed, the machine lost power — is swept by its run id, and its record
// removed. Only records whose owning process is provably gone are touched
// (run.SweepOrphaned); a second desktop's live run and every CLI run carry
// other run ids. Called once, in its own goroutine, from OnStartup.
func (r *RunAPI) sweepOrphans(ctx context.Context) []string {
	dir, err := r.inFlight()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, sweepWait)
	defer cancel()
	swept, err := run.SweepOrphaned(ctx, dir, r.sweepRun)
	if len(swept) > 0 {
		log.Printf("agentry-desktop: removed what %d run(s) of an earlier session left behind: %s", len(swept), strings.Join(swept, ", "))
	}
	if err != nil {
		log.Printf("agentry-desktop: %v", err)
	}
	return swept
}

func (r *RunAPI) current() *liveRun {
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	return r.live
}

// beforeClose is OnBeforeClose: with a run in flight it asks, in a native
// dialog, whether to quit and stop it. No keeps the window open. Yes — or
// a dialog that cannot be shown — lets the close go ahead; the run is then
// ended in shutdown, never left running.
func (r *RunAPI) beforeClose(ctx context.Context) (prevent bool) {
	l := r.current()
	if l == nil || r.app == nil || r.app.confirm == nil {
		return false
	}
	yes, err := r.app.confirm(ctx, "Quit Agentry Desktop?",
		"A run of "+l.ref+" is in progress. Quitting stops it and removes its containers; its receipts are written as far as it got. Quit anyway?")
	if err != nil {
		return false
	}
	return !yes
}

// shutdown is the run half of OnShutdown: end the run in flight, wait for
// the engine's teardown (bounded by quitWait), then sweep its label.
func (r *RunAPI) shutdown() {
	l := r.current()
	if l == nil {
		return
	}
	if s := r.session; s != nil {
		// The session's End asks the harness to quit, waits for it, and
		// runs the engine's teardown; bounded like the autonomous wait.
		endCtx, cancel := context.WithTimeout(context.Background(), quitWait)
		go func() {
			defer cancel()
			_, _ = s.End(endCtx, false)
			l.cancel()
			r.endRun(l)
			r.releaseSessionKnowledge()
		}()
	} else {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-time.After(quitWait):
	}
	sweepCtx, cancel := context.WithTimeout(context.Background(), sweepWait)
	defer cancel()
	if r.sweepRun(sweepCtx, l.id) == nil {
		r.forgetInFlight(l.id)
	}
}

// The engine seams: production uses internal/run; a test substitutes a
// fake engine the way the hosted-run tests substitute a fake registry.
func (r *RunAPI) mintRunID() (string, error) {
	if r.newRunID != nil {
		return r.newRunID()
	}
	return run.NewRunID()
}

func (r *RunAPI) runEngine(ctx context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
	if r.engineRun != nil {
		return r.engineRun(ctx, req, logw)
	}
	return run.Run(ctx, req, logw)
}

// inFlight is the crash-path records' directory: the test's, or the runner
// home's desktop-runs.
func (r *RunAPI) inFlight() (string, error) {
	if r.inFlightDir != "" {
		return r.inFlightDir, nil
	}
	return run.InFlightDir()
}

func (r *RunAPI) sweepRun(ctx context.Context, id string) error {
	if r.sweep != nil {
		return r.sweep(ctx, id)
	}
	return run.Sweep(ctx, id)
}
