package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func newQuitAPI(t *testing.T, f *fakeEngine, confirm func(context.Context, string, string) (bool, error)) *RunAPI {
	t.Helper()
	return &RunAPI{
		inFlightDir: filepath.Join(t.TempDir(), run.InFlightDirName),
		app:         &App{ctx: context.Background(), confirm: confirm},
		newRunID:    func() (string, error) { return "ab12cd34", nil },
		engineRun:   f.run,
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
		_, err := r.RunAgent("https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false, nil)
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
	_, err := os.Stat(filepath.Join(r.inFlightDir, id+".json"))
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
	if err := os.MkdirAll(r.inFlightDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // this test binary
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	write := func(id string, pid int) {
		b, _ := json.Marshal(map[string]any{"run_id": id, "pid": pid})
		if err := os.WriteFile(filepath.Join(r.inFlightDir, id+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("dead0001", cmd.Process.Pid)
	write("a11e0002", os.Getppid())

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
