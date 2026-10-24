// Quitting mid-run. A run's containers are torn down by the engine when its
// Run or its session's End returns, deferred on the binding's goroutine; if
// the app simply exits while that goroutine is still inside the engine, the
// teardown never runs and the agent, gateway (holding the Docker socket) and
// proxy containers outlive the app. So the app keeps a record of the one run
// in flight, asks before a close would cut it short, and on shutdown ends it
// and waits — bounded — for the engine's teardown, then sweeps the run's
// label itself in case the wait ran out.
//
// The same sweep runs for what is not live: at start for every recorded run
// whose owner is gone, and at quit for those and for the records this app
// still holds, which are the run the quit's wait gave up on and a run that
// ended with files its engine could not delete (settle). The rule it serves
// is that nothing about a run stays on this computer but its output folder;
// run.Sweep says what the sweep reaches and what it does not.
package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"

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
	id string
	// kind is the job's (jobRun, jobChat, jobDraft, jobBuild, jobReview):
	// Stop stops only its own kind.
	kind jobKind
	ref  string
	// started is when the run began, for its history entry; folder is the
	// folder a built-in's flow kept for it (keepFolder).
	started time.Time
	folder  string
	// registry is a run's or a chat's, for its history entry.
	registry string
	// turns are the run ids of the run's later turns: a draft's one repair
	// turn runs under its own id (drafter.PostStep), so the crash path
	// records it and quitting sweeps it with the run's own.
	turns []string
	// cancel ends the run's engine context: an autonomous run returns and
	// tears down; a session still starting stops starting. A session that
	// has started is ended through its End instead, which lets the harness
	// quit and writes the receipts.
	cancel context.CancelFunc
	// done is closed when the binding that owns the run has returned from
	// the engine (RunAgent), or the session has ended (EndSession).
	done chan struct{}
	// ending is set by the endRun call that ends the run, under liveMu: a
	// second call returns at once.
	ending bool
	// unsafe says the run is live in Unsafe Mode, and unsafeDone that it has
	// ended, both under RunAPI.unsafeMu (setUnsafeLive).
	unsafe, unsafeDone bool
	// release removes the opened copies of the knowledge packs attached to
	// the run (nil when none was). endRun calls it before closing done,
	// which quitting waits for, up to quitWait: a run that ends in that time
	// leaves no opened copy in the store; one that does not leaves it for
	// the store's own sweep, an hour on, as a killed app does.
	release func()
}

// beginRun records a run about to enter the engine and returns its id and
// the record; endRun clears it.
func (r *RunAPI) beginRun(kind jobKind, ref string, cancel context.CancelFunc) (*liveRun, error) {
	id, err := r.mintRunID()
	if err != nil {
		return nil, err
	}
	return r.beginRunAs(kind, ref, id, cancel)
}

// beginRunAs is beginRun for a run whose id was minted before it waited
// for the run lock.
func (r *RunAPI) beginRunAs(kind jobKind, ref, id string, cancel context.CancelFunc) (*liveRun, error) {
	l := &liveRun{id: id, kind: kind, ref: ref, started: time.Now().UTC(), cancel: cancel, done: make(chan struct{})}
	r.liveMu.Lock()
	r.live = l
	r.liveMu.Unlock()
	// The crash path's record: written before the engine creates anything,
	// so a kill from here on leaves the next start a run id to sweep. Best
	// effort: a run is not refused because the runner home is unwritable;
	// the record is litter insurance.
	if dir, err := r.inFlight(); err == nil {
		if err := run.RecordInFlight(dir, id); err != nil {
			log.Printf("agentry-desktop: %v", err)
		}
	}
	return l, nil
}

// addTurn records a later turn of the run in flight, under its own run id,
// before the engine creates anything for it: its crash-path record, and
// the id quitting sweeps with the run's.
func (r *RunAPI) addTurn(id string) {
	r.liveMu.Lock()
	if r.live != nil {
		r.live.turns = append(r.live.turns, id)
	}
	r.liveMu.Unlock()
	if dir, err := r.inFlight(); err == nil {
		if err := run.RecordInFlight(dir, id); err != nil {
			log.Printf("agentry-desktop: %v", err)
		}
	}
}

// endRun may be called from both ends of a session (EndSession and
// shutdown): only the first call releases the attached knowledge, settles
// the run's crash-path records and closes done.
func (r *RunAPI) endRun(l *liveRun) {
	r.liveMu.Lock()
	if r.live != l || l.ending {
		r.liveMu.Unlock()
		return
	}
	l.ending = true
	if l.release != nil {
		l.release()
	}
	ids := append([]string{l.id}, l.turns...)
	r.liveMu.Unlock()
	// Deferred, so that a run whose settling panics is not left in flight
	// with nobody ever told it is done.
	defer func() {
		r.liveMu.Lock()
		if r.live == l {
			r.live = nil
		}
		close(l.done)
		r.liveMu.Unlock()
		r.unsafeMu.Lock()
		defer r.unsafeMu.Unlock()
		l.unsafeDone = true
		if l.unsafe {
			l.unsafe = false
			emitEvent(r.app, eventUnsafeLive, false)
		}
	}()
	// Settled outside the lock, which the close dialog and the Stop buttons
	// read: deleting a file Docker Desktop still holds can take seconds on
	// Windows. The run stays the one in flight until its records are
	// settled, so a quit in that time waits for it as for any run.
	for _, id := range ids {
		r.settle(id)
	}
}

// settle is what becomes of a run's crash-path record once its engine has
// returned, which it does only after its own teardown. What that teardown
// could not delete under the runner home (a file Docker Desktop still held;
// a directory the run's tools left unwritable) is tried once more here, and
// the record goes only when none of the run's files are left; otherwise it
// stays, and quitting and the next start sweep the run by it. Docker is not
// asked: a container the engine's teardown could not remove is not seen
// here, as it was not before.
func (r *RunAPI) settle(id string) {
	home, err := r.runnerHome()
	if err != nil {
		log.Printf("agentry-desktop: run %s: %v", id, err)
		return
	}
	if err := run.SweepFiles(home, id); err != nil {
		log.Printf("agentry-desktop: run %s left files behind, to be removed when the app quits or next starts: %v", id, err)
		return
	}
	r.forgetInFlight(id)
}

// attach opens the named knowledge packs for the run l, whose endRun
// removes the opened copies.
func (r *RunAPI) attach(ctx context.Context, l *liveRun, reg string, names []string) ([]engine.AttachedPack, error) {
	packs, release, err := r.knowledge().Attach(ctx, reg, names)
	if err != nil {
		return nil, err
	}
	r.liveMu.Lock()
	l.release = release
	r.liveMu.Unlock()
	return packs, nil
}

// forgetInFlight removes a run's crash-path record once nothing of it can
// be left: the engine returned and its files are gone (settle), or the run
// was swept.
func (r *RunAPI) forgetInFlight(id string) {
	if dir, err := r.inFlight(); err == nil {
		if err := run.ForgetInFlight(dir, id); err != nil {
			log.Printf("agentry-desktop: %v", err)
		}
	}
}

// sweepOrphans is the start half of the crash path: a run recorded by an app
// that died without tearing it down — killed, crashed, the machine lost
// power — is swept by its run id, and its record removed: its containers,
// volumes and networks, and its scratch directory with the secrets file.
// Only records whose owning process is gone from this machine are touched
// (run.SweepOrphaned; the head of internal/run/inflight.go says what that
// judgement cannot see); a second desktop's live run and every CLI run carry
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
	sweepStaleStages()
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
//
// A session In Agentry is not stopped by quitting: with no local run in
// flight, the dialog says it goes on until its idle close, and that
// reopening its run picks the conversation up.
func (r *RunAPI) beforeClose(ctx context.Context) (prevent bool) {
	l := r.current()
	if r.app == nil || r.app.confirm == nil {
		return false
	}
	if l == nil {
		cur := r.hostedCurrent()
		if cur == nil || !cur.session {
			return false
		}
		yes, err := r.app.confirm(ctx, "Quit Agentry Desktop?", hostedSessionQuitNote(cur.ref, cur.idle))
		if err != nil {
			return false
		}
		return !yes
	}
	queued := ""
	switch n := r.queuedCount(); {
	case n == 1:
		queued = " The run waiting to start after it is cancelled."
	case n > 1:
		queued = fmt.Sprintf(" The %d runs waiting to start after it are cancelled.", n)
	}
	yes, err := r.app.confirm(ctx, "Quit Agentry Desktop?",
		"A run of "+l.ref+" is in progress. Quitting stops it and removes its containers; its receipts are written as far as it got."+queued+" Quit anyway?")
	if err != nil {
		return false
	}
	return !yes
}

// shutdown is the run half of OnShutdown: end the run in flight, wait for
// the engine's teardown (bounded by quitWait), then sweep what it left; and,
// with or without a run in flight, sweep what any earlier run of this app
// and any run of an app that died left (run.SweepAtQuit). Another app's
// run that is live on this machine is never touched.
func (r *RunAPI) shutdown() {
	defer r.removePDFCopies()
	// The queued runs go first, so none is handed the lock as the run in
	// flight ends.
	r.closeQueue()
	var ids []string
	if l := r.current(); l != nil {
		if s := r.session; s != nil {
			// The session's End asks the harness to quit, waits for it,
			// writes the receipts as far as the chat got and runs the
			// engine's teardown; bounded like the autonomous wait.
			endCtx, cancel := context.WithTimeout(context.Background(), quitWait)
			go func() {
				defer cancel()
				_, _ = s.End(endCtx, false)
				l.cancel()
				r.endRun(l)
			}()
		} else {
			l.cancel()
		}
		select {
		case <-l.done:
		case <-time.After(quitWait):
		}
		r.liveMu.Lock()
		ids = append([]string{l.id}, l.turns...)
		r.liveMu.Unlock()
	}
	sweepCtx, cancel := context.WithTimeout(context.Background(), sweepWait)
	defer cancel()
	// Each run id is swept once: the live run's by its id, whether or not
	// its record could be written, and then not again by that record.
	tried := map[string]error{}
	sweep := func(ctx context.Context, id string) error {
		if err, ok := tried[id]; ok {
			return err
		}
		err := r.sweepRun(ctx, id)
		tried[id] = err
		return err
	}
	for _, id := range ids {
		if sweep(sweepCtx, id) == nil {
			r.forgetInFlight(id)
		}
	}
	dir, err := r.inFlight()
	if err != nil {
		return
	}
	swept, err := run.SweepAtQuit(sweepCtx, dir, sweep)
	if len(swept) > 0 {
		log.Printf("agentry-desktop: removed what %d run(s) left behind: %s", len(swept), strings.Join(swept, ", "))
	}
	if err != nil {
		log.Printf("agentry-desktop: %v", err)
	}
	sweepStaleStages()
}

// sweepStaleStages removes the review copies no record names once they are
// a day old (run.SweepStaleStages): a review's pre-step the app died inside.
func sweepStaleStages() {
	removed, err := run.SweepStaleStages(run.StaleStageAge)
	if len(removed) > 0 {
		log.Printf("agentry-desktop: removed %d stale review copies: %s", len(removed), strings.Join(removed, ", "))
	}
	if err != nil {
		log.Printf("agentry-desktop: %v", err)
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

func (r *RunAPI) startEngineSession(ctx context.Context, req run.Request, logw io.Writer) (localSession, error) {
	if r.engineSession != nil {
		return r.engineSession(ctx, req, logw)
	}
	s, err := run.StartSession(ctx, req, logw)
	if err != nil {
		// Not the nil *run.Session in the interface: that would read as a
		// session.
		return nil, err
	}
	return s, nil
}

// runnerHome is the runner home: a test's, or this machine's.
func (r *RunAPI) runnerHome() (string, error) {
	if r.runnerHomeDir != "" {
		return r.runnerHomeDir, nil
	}
	return run.Home()
}

// inFlight is the crash-path records' directory, the runner home's
// desktop-runs.
func (r *RunAPI) inFlight() (string, error) {
	home, err := r.runnerHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, run.InFlightDirName), nil
}

// inFlightDir is inFlight for a run request, "" when the runner home
// cannot be found (the run then records no profile, as it records no run).
func (r *RunAPI) inFlightDir() string {
	dir, err := r.inFlight()
	if err != nil {
		return ""
	}
	return dir
}

func (r *RunAPI) sweepRun(ctx context.Context, id string) error {
	if r.sweep != nil {
		return r.sweep(ctx, id)
	}
	return run.Sweep(ctx, id)
}
