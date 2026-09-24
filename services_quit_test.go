package main

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

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

func newQuitAPI(f *fakeEngine, confirm func(context.Context, string, string) (bool, error)) *RunAPI {
	return &RunAPI{
		app:       &App{ctx: context.Background(), confirm: confirm},
		newRunID:  func() (string, error) { return "ab12cd34", nil },
		engineRun: f.run,
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
		_, err := r.RunAgent("https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false)
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
	r := newQuitAPI(f, nil)
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
}

// An engine that does not return in time does not hold the quit hostage:
// the wait is bounded, and the label sweep runs regardless.
func TestAHungEngineIsSweptWhenTheWaitRunsOut(t *testing.T) {
	prev := quitWait
	quitWait = 100 * time.Millisecond
	t.Cleanup(func() { quitWait = prev })
	f := &fakeEngine{started: make(chan run.Request, 1), hang: true, release: make(chan struct{})}
	r := newQuitAPI(f, nil)
	_, errc := startRun(t, r, f)

	start := time.Now()
	r.shutdown()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("shutdown waited %v on a hung engine", d)
	}
	if got := f.steps(); len(got) != 1 || got[0] != "sweep ab12cd34" {
		t.Fatalf("steps %q, want only the sweep", got)
	}
	close(f.release)
	<-errc
}

// With nothing running, quitting touches no docker at all.
func TestShutdownWithNoRunSweepsNothing(t *testing.T) {
	f := &fakeEngine{started: make(chan run.Request, 1)}
	newQuitAPI(f, nil).shutdown()
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
	r := newQuitAPI(f, confirm)

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
