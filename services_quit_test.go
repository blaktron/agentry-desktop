package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/session"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// fakeEngine stands in for internal/run: Run blocks like a live run until
// its context is cancelled, then "tears down" (the engine's deferred
// teardown) before returning. Every step lands in one ordered log.
type fakeEngine struct {
	mu      sync.Mutex
	log     []string
	started chan run.Request
	// hang makes Run ignore cancellation until released.
	hang    bool
	release chan struct{}
}

func (f *fakeEngine) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, s)
}

func (f *fakeEngine) steps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}

func (f *fakeEngine) run(ctx context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
	f.started <- req
	if f.hang {
		<-f.release
	} else {
		<-ctx.Done()
	}
	time.Sleep(20 * time.Millisecond) // the teardown takes a moment
	f.record("teardown " + req.RunID)
	return nil, ctx.Err()
}

func newQuitAPI(t *testing.T, f *fakeEngine, confirm func(context.Context, string, string) (bool, error)) *RunAPI {
	t.Helper()
	return &RunAPI{
		runnerHomeDir: t.TempDir(),
		app:           &App{ctx: context.Background(), confirm: confirm},
		newRunID:      func() (string, error) { return "ab12cd34", nil },
		engineRun:     f.run,
		sweep: func(_ context.Context, id string) error {
			f.record("sweep " + id)
			return nil
		},
	}
}

func startRun(t *testing.T, r *RunAPI, f *fakeEngine) (run.Request, chan error) {
	t.Helper()
	errc := make(chan error, 1)
	go func() {
		_, err := r.RunAgent("https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false, nil, false)
		errc <- err
	}()
	select {
	case req := <-f.started:
		return req, errc
	case err := <-errc:
		t.Fatalf("the run never reached the engine: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the run never reached the engine")
	}
	return run.Request{}, nil
}

// §9 "teardown on every path": quitting mid-run cancels the run, and the
// engine's teardown, then the label sweep, have both happened before
// shutdown returns — the process does not exit under a live run.
func TestQuittingMidRunTearsDownBeforeShutdownReturns(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	req, errc := startRun(t, r, f)
	if req.RunID != "ab12cd34" {
		t.Fatalf("the engine was not given the app's run id: %q", req.RunID)
	}

	r.shutdown()

	got := f.steps()
	want := []string{"teardown ab12cd34", "sweep ab12cd34"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("shutdown returned with steps %q, want %q", got, want)
	}
	if err := <-errc; err == nil {
		t.Fatal("a cancelled run reported success")
	}
	if r.current() != nil {
		t.Fatal("the run is still recorded as live")
	}
	if recorded(r, "ab12cd34") {
		t.Fatal("the crash-path record outlived a run that was torn down")
	}
}

// A draft's repair turn runs under its own run id (#395): it is recorded for
// the crash path while it runs, and quitting sweeps it with the run's own.
func TestARepairTurnIsSweptWithTheRun(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	_, errc := startRun(t, r, f)
	r.addTurn("ef56ab78")
	if !recorded(r, "ef56ab78") {
		t.Fatal("the repair turn has no crash-path record")
	}
	r.shutdown()
	<-errc
	got := strings.Join(f.steps(), ",")
	if !strings.Contains(got, "sweep ab12cd34") || !strings.Contains(got, "sweep ef56ab78") {
		t.Fatalf("shutdown steps %q", got)
	}
	if recorded(r, "ab12cd34") || recorded(r, "ef56ab78") {
		t.Fatal("a crash-path record outlived the run")
	}
}

// An engine that does not return in time does not hold the quit hostage:
// the wait is bounded, and the label sweep runs regardless.
func TestAHungEngineIsSweptWhenTheWaitRunsOut(t *testing.T) {
	prev := quitWait
	quitWait = 100 * time.Millisecond
	t.Cleanup(func() { quitWait = prev })
	f := &fakeEngine{started: make(chan run.Request, 1), hang: true, release: make(chan struct{})}
	r := newQuitAPI(t, f, nil)
	_, errc := startRun(t, r, f)

	start := time.Now()
	r.shutdown()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("shutdown waited %v on a hung engine", d)
	}
	if got := f.steps(); len(got) != 1 || got[0] != "sweep ab12cd34" {
		t.Fatalf("steps %q, want only the sweep", got)
	}
	if recorded(r, "ab12cd34") {
		t.Fatal("a swept run's crash-path record was kept")
	}
	close(f.release)
	<-errc
}

// With nothing running, quitting touches no docker at all.
func TestShutdownWithNoRunSweepsNothing(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	newQuitAPI(t, f, nil).shutdown()
	if got := f.steps(); len(got) != 0 {
		t.Fatalf("steps %q with no run", got)
	}
}

// Closing the window asks only when a run is live; No keeps it open, Yes
// (or a dialog that cannot be shown) lets it close — never a trapped app.
func TestBeforeCloseAsksOnlyWithARunLive(t *testing.T) {
	asked := 0
	answer, answerErr := false, error(nil)
	confirm := func(context.Context, string, string) (bool, error) {
		asked++
		return answer, answerErr
	}
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, confirm)

	if r.beforeClose(context.Background()) || asked != 0 {
		t.Fatalf("idle app: prevented or asked (%d)", asked)
	}
	_, errc := startRun(t, r, f)
	if !r.beforeClose(context.Background()) || asked != 1 {
		t.Fatal("No did not keep the window open")
	}
	answer = true
	if r.beforeClose(context.Background()) {
		t.Fatal("Yes did not let the window close")
	}
	answer, answerErr = false, errors.New("no dialog")
	if r.beforeClose(context.Background()) {
		t.Fatal("a failed dialog trapped the app")
	}
	r.shutdown()
	<-errc
}

func recorded(r *RunAPI, id string) bool {
	_, err := os.Stat(filepath.Join(r.runnerHomeDir, run.InFlightDirName, id+".json"))
	return err == nil
}

// A run in flight is recorded under its id for the crash path
// (agentry-desktop#198) from before the engine starts, and the record goes
// when the engine has returned.
func TestARunIsRecordedWhileInFlight(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	_, errc := startRun(t, r, f)
	if !recorded(r, "ab12cd34") {
		t.Fatal("a run in flight has no crash-path record")
	}
	r.current().cancel()
	<-errc
	if recorded(r, "ab12cd34") {
		t.Fatal("the record outlived the run")
	}
}

// At the next start, the run an app was killed in the middle of is swept by
// its record, and the record removed; a live app's run (here the test's
// parent process owns it) is left alone. The sweep reaches the engine only
// by run id, so a CLI run — no record, another id — cannot be touched.
func TestStartupSweepsTheRunOfAnAppThatDied(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	writeOwner(t, r, "dead0001", goneOwner(t))
	writeOwner(t, r, "a11e0002", os.Getppid())

	swept := r.sweepOrphans(context.Background())
	if strings.Join(swept, ",") != "dead0001" {
		t.Fatalf("swept %v", swept)
	}
	if got := f.steps(); len(got) != 1 || got[0] != "sweep dead0001" {
		t.Fatalf("steps %q, want only the dead app's run swept", got)
	}
	if recorded(r, "dead0001") || !recorded(r, "a11e0002") {
		t.Fatal("wrong records removed")
	}
}

// writeOwner writes the crash-path record of a run owned by pid.
func writeOwner(t *testing.T, r *RunAPI, id string, pid int) {
	t.Helper()
	dir := filepath.Join(r.runnerHomeDir, run.InFlightDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"run_id": id, "pid": pid})
	if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// goneOwner is the pid of a process that has exited.
func goneOwner(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // this test binary
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// The leftovers sweep runs at quit as well as at start (agentry-desktop#463,
// plan §5.4): with nothing live, quitting sweeps what a dead app left and
// what an earlier run of this app left, and never another live app's run.
func TestQuittingSweepsLeftoversWithNothingLive(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	writeOwner(t, r, "dead0001", goneOwner(t))
	writeOwner(t, r, "5e1f0004", os.Getpid())
	writeOwner(t, r, "a11e0002", os.Getppid())

	r.shutdown()

	if got := strings.Join(f.steps(), ","); got != "sweep 5e1f0004,sweep dead0001" {
		t.Fatalf("steps %q, want this app's leftover and the dead app's swept", got)
	}
	if recorded(r, "dead0001") || recorded(r, "5e1f0004") || !recorded(r, "a11e0002") {
		t.Fatal("wrong records removed")
	}
}

// With a run live, quitting tears it down first and sweeps it, then what a
// dead app left: the live run's id is swept once, not again by its record.
func TestQuittingMidRunSweepsLeftoversAfterTheTeardown(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	writeOwner(t, r, "dead0001", goneOwner(t))
	_, errc := startRun(t, r, f)

	r.shutdown()
	<-errc

	if got := strings.Join(f.steps(), ","); got != "teardown ab12cd34,sweep ab12cd34,sweep dead0001" {
		t.Fatalf("steps %q", got)
	}
	if recorded(r, "ab12cd34") || recorded(r, "dead0001") {
		t.Fatal("a swept run's record was kept")
	}
}

// A sweep that fails at quit (Docker already stopped) is tried once, not
// again through the run's own record, and the record stays for the next
// start.
func TestAFailedQuitSweepIsTriedOnceAndKeepsTheRecord(t *testing.T) {
	prev := quitWait
	quitWait = 50 * time.Millisecond
	t.Cleanup(func() { quitWait = prev })
	// An engine that does not return: nothing but the sweep could remove
	// the run's record.
	f := &fakeEngine{started: make(chan run.Request, 1), hang: true, release: make(chan struct{})}
	r := newQuitAPI(t, f, nil)
	r.sweep = func(_ context.Context, id string) error {
		f.record("sweep " + id)
		return errors.New("docker is not running")
	}
	_, errc := startRun(t, r, f)

	r.shutdown()

	if got := strings.Join(f.steps(), ","); got != "sweep ab12cd34" {
		t.Fatalf("steps %q, want the live run swept once", got)
	}
	if !recorded(r, "ab12cd34") {
		t.Fatal("a run that could not be swept lost its record")
	}
	close(f.release)
	<-errc
}

// fakeChat stands in for the engine's interactive session: End records
// whether a final result was asked for, as the engine's writes the receipts.
type fakeChat struct {
	f      *fakeEngine
	id     string
	events chan session.Event
}

func (c *fakeChat) Ref() string                            { return "acme/agent@1.0.0" }
func (c *fakeChat) Events() <-chan session.Event           { return c.events }
func (c *fakeChat) Send(context.Context, string) error     { return nil }
func (c *fakeChat) AddInput(context.Context, string) error { return nil }
func (c *fakeChat) End(_ context.Context, finish bool) (*run.Result, error) {
	time.Sleep(20 * time.Millisecond) // the teardown takes a moment
	c.f.record(fmt.Sprintf("end %s finish=%v", c.id, finish))
	return &run.Result{RunID: c.id}, nil
}

// Quitting with a chat started (the case the quit path's tests lacked,
// agentry-desktop#463): the chat is ended, without asking the QuickAgent for
// a final result, which is where the engine writes its receipts as far as
// it got and tears its sandbox down; then its run id is swept, and nothing
// of it is left recorded. The engine's half, the receipts on disk, is
// internal/run's TestAChatEndedByQuittingHasItsReceiptsAndLeavesNothing.
func TestQuittingWithAChatStartedEndsItThenSweeps(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	r.engineSession = func(_ context.Context, req run.Request, _ io.Writer) (localSession, error) {
		events := make(chan session.Event)
		close(events)
		return &fakeChat{f: f, id: req.RunID, events: events}, nil
	}
	if _, err := r.StartSession("https://r.example", "acme/agent@1.0.0", "", nil, "", nil, -1, false, nil, false, false, nil, false); err != nil {
		t.Fatal(err)
	}
	if !recorded(r, "ab12cd34") || r.current() == nil {
		t.Fatal("a started chat is not recorded as live")
	}

	r.shutdown()

	if got := strings.Join(f.steps(), ","); got != "end ab12cd34 finish=false,sweep ab12cd34" {
		t.Fatalf("steps %q, want the chat ended and then swept", got)
	}
	if r.current() != nil || recorded(r, "ab12cd34") {
		t.Fatal("the chat is still recorded after the quit")
	}
}

// leaveScratch makes the scratch directory a run leaves under the runner
// home the API's records belong to, with a secrets file in it.
func leaveScratch(t *testing.T, r *RunAPI, id string) string {
	t.Helper()
	dir := filepath.Join(r.runnerHomeDir, "agents", "acme", "agent", "1.0.0-0123456789ab", "runs", id)
	if err := os.MkdirAll(filepath.Join(dir, "cfg", "gateway"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cfg", "gateway", "secrets.env"), []byte("K=v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A run whose engine returned without deleting its files (Docker Desktop
// still held one, agentry-cli#422) is not forgotten with them on disk: they
// are tried again when the run ends, and the record goes with them.
func TestARunsFilesAreRemovedWhenItEnds(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	_, errc := startRun(t, r, f)
	scratch := leaveScratch(t, r, "ab12cd34")

	r.current().cancel()
	<-errc

	if _, err := os.Stat(scratch); err == nil {
		t.Fatal("the ended run's scratch directory is still there")
	}
	if recorded(r, "ab12cd34") {
		t.Fatal("the record outlived a run that left nothing")
	}
}

// And while they cannot be deleted the record stays, so the quit sweep (and
// the next start) has the run's id to try again with: without it nothing
// would ever look for that directory.
func TestARunThatLeavesFilesKeepsItsRecordForTheQuitSweep(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	_, errc := startRun(t, r, f)
	scratch := leaveScratch(t, r, "ab12cd34")
	run.SetRemoveTreeForTest(t, func(string) error { return errors.New("the file is in use") })

	r.current().cancel()
	<-errc

	if _, err := os.Stat(scratch); err != nil {
		t.Fatal("the test's delete was not the one that ran")
	}
	if !recorded(r, "ab12cd34") || r.current() != nil {
		t.Fatal("a run that left its scratch directory behind was forgotten")
	}

	r.shutdown()

	if got := strings.Join(f.steps(), ","); got != "teardown ab12cd34,sweep ab12cd34" {
		t.Fatalf("steps %q, want the ended run swept at quit by its record", got)
	}
	if recorded(r, "ab12cd34") {
		t.Fatal("the swept run's record was kept")
	}
}

// Settling a run's records is done outside the lock the close dialog and
// the Stop buttons read (a delete can wait seconds on Docker Desktop), and
// the run is still the one in flight until it is done, so a quit in that
// time waits for it.
func TestARunBeingSettledIsStillInFlightAndHoldsNoLock(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	r := newQuitAPI(t, f, nil)
	_, errc := startRun(t, r, f)
	leaveScratch(t, r, "ab12cd34")
	deleting, release := make(chan struct{}), make(chan struct{})
	run.SetRemoveTreeForTest(t, func(dir string) error {
		close(deleting)
		<-release
		return os.RemoveAll(dir)
	})
	live := r.current()
	live.cancel()
	select {
	case <-deleting:
	case <-time.After(5 * time.Second):
		t.Fatal("the ended run's files were never looked for")
	}

	got := make(chan *liveRun, 1)
	go func() { got <- r.current() }()
	select {
	case l := <-got:
		if l != live {
			t.Fatal("a run whose files are still being removed is no longer the run in flight")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the lock is held while the run's files are removed")
	}
	select {
	case <-live.done:
		t.Fatal("the run was reported done before its records were settled")
	default:
	}

	// A second end of the same run while the first is settling it (a chat
	// ended by its window and by a quit at once) does nothing: done is
	// closed once, by the first.
	r.endRun(live)
	select {
	case <-live.done:
		t.Fatal("a second end closed the run before the first had settled it")
	default:
	}

	close(release)
	<-errc
	if r.current() != nil || recorded(r, "ab12cd34") {
		t.Fatal("the settled run is still recorded")
	}
}
