package app

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// endedEvents records the runner:run-ended events and whether the run's slot
// was already given back when each was sent.
type endedEvents struct {
	mu      sync.Mutex
	ended   []RunEnded
	slotted []bool
}

func captureEnded(t *testing.T, r *RunAPI) *endedEvents {
	t.Helper()
	e := &endedEvents{}
	prev := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name != eventRunEnded || len(data) != 1 {
			return
		}
		got, ok := data[0].(RunEnded)
		if !ok {
			t.Errorf("runner:run-ended carried %T", data[0])
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.ended = append(e.ended, got)
		e.slotted = append(e.slotted, r.InFlight() != nil)
	}
	t.Cleanup(func() { emitEvent = prev })
	return e
}

func (e *endedEvents) all() []RunEnded {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]RunEnded(nil), e.ended...)
}

// A run that began ends with one runner:run-ended event carrying its id and
// its result, sent after its slot is free, so a page that followed it
// without starting it can list it as over.
func TestARunThatBeganEndsWithItsResult(t *testing.T) {
	r := flowAPI(t, func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		return &run.Result{Status: "ok", RunID: req.RunID, Ref: req.Ref}, nil
	})
	ended := captureEnded(t, r)
	if _, err := runAgent(r); err != nil {
		t.Fatal(err)
	}
	got := ended.all()
	if len(got) != 1 || got[0].RunID != "ab12cd34" || got[0].Result == nil || got[0].Result.Status != "ok" || got[0].Error != "" {
		t.Fatalf("end events %+v", got)
	}
	if ended.slotted[0] {
		t.Error("the end event was sent while the run still held its slot")
	}
}

// A run that ends with no result says why in its end event.
func TestARunThatEndsWithNoResultSaysWhy(t *testing.T) {
	r := flowAPI(t, func(context.Context, run.Request, io.Writer) (*run.Result, error) {
		return nil, errors.New("the engine would not start")
	})
	ended := captureEnded(t, r)
	if _, err := runAgent(r); err == nil {
		t.Fatal("the run did not fail")
	}
	got := ended.all()
	if len(got) != 1 || got[0].RunID != "ab12cd34" || got[0].Result != nil || got[0].Error != "the engine would not start" {
		t.Fatalf("end events %+v", got)
	}
}

// A run Go starts through its hooks ends the same way, and its own began
// hook is still told the run's id first.
func TestAHookedRunEndsWithTheEventAndToldItsHook(t *testing.T) {
	r := flowAPI(t, func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		return &run.Result{Status: "ok", RunID: req.RunID}, nil
	})
	ended := captureEnded(t, r)
	var began []string
	h := runHooks{began: func(id string) {
		began = append(began, id)
		if n := len(ended.all()); n != 0 {
			t.Errorf("%d end events before the run ended", n)
		}
	}}
	if _, err := r.runAgent(h, "https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(began) != 1 || began[0] != "ab12cd34" {
		t.Fatalf("began hook told %v", began)
	}
	if got := ended.all(); len(got) != 1 || got[0].RunID != "ab12cd34" {
		t.Fatalf("end events %+v", got)
	}
}

// A run that never began has nothing to end: a queued run cancelled while it
// waits sends no end event, and the run it waited behind sends its own.
func TestACancelledQueuedRunSendsNoEndEvent(t *testing.T) {
	captureEvents(t)
	q := newQueueRig(t)
	ended := captureEnded(t, q.r)
	running := q.start()
	waitUntil(t, "the running run", func() bool { return len(q.runs()) == 1 })
	waiting := q.start()
	waitUntil(t, "the queued run", func() bool { return len(q.r.QueuedRuns()) == 1 })
	if err := q.r.CancelQueuedRun("ab12cd02"); err != nil {
		t.Fatal(err)
	}
	if err := outcome(t, waiting); !errors.Is(err, errQueueCancelled) {
		t.Fatalf("the cancelled run: %v", err)
	}
	if got := ended.all(); len(got) != 0 {
		t.Fatalf("end events before any run ended: %+v", got)
	}
	q.finish("ab12cd01")
	if err := outcome(t, running); err != nil {
		t.Fatal(err)
	}
	if got := ended.all(); len(got) != 1 || got[0].RunID != "ab12cd01" {
		t.Fatalf("end events %+v", got)
	}
}
