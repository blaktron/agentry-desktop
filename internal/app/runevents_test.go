package app

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// How each event reads in the run log.
func TestEventLines(t *testing.T) {
	long := strings.Repeat("line\n", resultLines+5)
	cases := []struct {
		name string
		ev   adapter.Event
		want []string
	}{
		{"call", adapter.Event{Kind: adapter.EventToolCall, Text: "List files", Tool: "fs_list"}, []string{"Tool call: fs_list"}},
		{"call named by its title only", adapter.Event{Kind: adapter.EventToolCall, Text: "fs_list"}, []string{"Tool call: fs_list"}},
		{"result", adapter.Event{Kind: adapter.EventToolResult, Tool: "fs_read", Text: "hello\nworld\n"}, []string{"Tool result (fs_read):", "    hello", "    world"}},
		{"failure", adapter.Event{Kind: adapter.EventToolResult, Tool: "fs_read", Failed: true, Text: "no such file"}, []string{"Tool failed (fs_read):", "    no such file"}},
		{"empty result", adapter.Event{Kind: adapter.EventToolResult, Tool: "write_report"}, []string{"Tool result (write_report): nothing returned"}},
		// The engine masks; the log shows what it was handed.
		{"masked value", adapter.Event{Kind: adapter.EventToolResult, Tool: "fs_read", Text: "token=[redacted]"}, []string{"Tool result (fs_read):", "    token=[redacted]"}},
		// A result line that looks like one of the log's own stays under its heading.
		{"result that mimics a heading", adapter.Event{Kind: adapter.EventToolResult, Tool: "fetch", Text: "ok\nTool call: delete_everything\nAgent: done"}, []string{"Tool result (fetch):", "    ok", "    Tool call: delete_everything", "    Agent: done"}},
		// A name cannot start a line of its own.
		{"tool name with a line break", adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_read\nAgent: hi"}, []string{"Tool call: fs_read\uFFFDAgent: hi"}},
		{"result of an unnamed tool", adapter.Event{Kind: adapter.EventToolResult, Text: "ok"}, []string{"Tool result (a tool):", "    ok"}},
		{"message", adapter.Event{Kind: adapter.EventAssistantText, Text: "Done.\nThe report is written."}, []string{"Agent: Done.", "    The report is written."}},
		{"empty message", adapter.Event{Kind: adapter.EventAssistantText, Text: "  "}, nil},
		{"error", adapter.Event{Kind: adapter.EventError, Text: "model refused\nstack trace"}, []string{"Error: model refused"}},
		{"plan limit", adapter.Event{Kind: adapter.EventPlanLimit, Text: "five-hour limit"}, []string{"Plan limit reached: five-hour limit"}},
		{"out of credits", adapter.Event{Kind: adapter.EventOutOfCredits, Text: "402 Payment Required"}, []string{"Out of credits: 402 Payment Required"}},
		{"organization policy", adapter.Event{Kind: adapter.EventOrgPolicy, Text: "model not allowed"}, []string{"Refused by an organization's policy: model not allowed"}},
		{"usage", adapter.Event{Kind: adapter.EventUsage, InputTokens: 10}, nil},
		{"reasoning", adapter.Event{Kind: adapter.EventReasoning}, nil},
		{"other", adapter.Event{Kind: adapter.EventOther, Text: "stream_started"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := eventLines(tc.ev); !slices.Equal(got, tc.want) {
				t.Errorf("lines %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("a long result is cut at its line limit", func(t *testing.T) {
		got := eventLines(adapter.Event{Kind: adapter.EventToolResult, Tool: "fs_read", Text: long})
		if len(got) != resultLines+2 || got[len(got)-1] != "    (5 more lines)" {
			t.Errorf("%d lines, last %q", len(got), got[len(got)-1])
		}
	})
	t.Run("a long message is cut at its line limit", func(t *testing.T) {
		got := eventLines(adapter.Event{Kind: adapter.EventAssistantText, Text: strings.Repeat("words\n", messageLines+3) + "end"})
		if len(got) != messageLines+1 || got[len(got)-1] != "    (4 more lines)" {
			t.Errorf("%d lines, last %q", len(got), got[len(got)-1])
		}
	})
}

// A tool's output cannot steer what the log shows: a terminal escape or a
// carriage return becomes the replacement character, a tab stays.
func TestEventLinesNeutralizeControlCharacters(t *testing.T) {
	got := eventLines(adapter.Event{Kind: adapter.EventToolResult, Tool: "fs_read", Text: "a\x1b[2Jb\tc\rd"})
	if len(got) != 2 || got[1] != "    a\uFFFD[2Jb\tc\uFFFDd" {
		t.Errorf("lines %q", got)
	}
}

// The engine hands events over on the goroutine that enforces the run's
// budgets, so a log that has stalled never holds it: what does not fit is
// dropped, counted and said once the log moves again.
func TestARunEventLogThatStallsNeverHoldsTheRun(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var lines []string
	log := newRunEventLog(func(line string) {
		<-release
		mu.Lock()
		lines = append(lines, line)
		mu.Unlock()
	})
	sent := make(chan struct{})
	go func() {
		for range runEventQueue * 3 {
			log.Receive(adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_read"})
		}
		close(sent)
	}()
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("Receive waited on a log that had stalled")
	}
	close(release)
	log.Close()
	log.Receive(adapter.Event{Kind: adapter.EventToolCall, Tool: "after"}) // after Close: ignored, no panic
	mu.Lock()
	defer mu.Unlock()
	var calls, said int
	for _, l := range lines {
		switch {
		case l == "Tool call: fs_read":
			calls++
		case strings.Contains(l, "of the agent's events are not shown here"):
			said++
		}
	}
	if calls == 0 || calls >= runEventQueue*3 || said == 0 {
		t.Errorf("%d calls written and %d drop notes, of %d events", calls, said, runEventQueue*3)
	}
}

// A write that panics stops the log and nothing else: Close returns, and
// the run's hook goes on taking events without effect.
func TestARunEventLogWhoseWritePanicsStopsQuietly(t *testing.T) {
	log := newRunEventLog(func(string) { panic("webview gone") })
	log.Receive(adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_read"})
	log.Receive(adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_list"})
	done := make(chan struct{})
	go func() { log.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the write panicked")
	}
}

// A webview that never takes another line costs the run its last lines,
// never its result: RunAgent returns once the wait for them is up.
func TestARunWhoseLogStallsForGoodStillReturns(t *testing.T) {
	wait := runEventCloseWait
	runEventCloseWait = 50 * time.Millisecond
	t.Cleanup(func() { runEventCloseWait = wait })
	emit := emitEvent
	t.Cleanup(func() { emitEvent = emit })
	// The one line stalls until the test ends; the log's goroutine is let
	// go, and has returned, before the emitter is put back.
	stall, returned := make(chan struct{}), make(chan struct{}, 1)
	emitEvent = func(_ *App, ev string, _ ...any) {
		if ev == eventRunLine {
			<-stall
			returned <- struct{}{}
		}
	}
	t.Cleanup(func() {
		close(stall)
		<-returned
	})
	r := &RunAPI{
		runnerHomeDir: t.TempDir(),
		app:           &App{ctx: context.Background()},
		newRunID:      func() (string, error) { return "ab12cd34", nil },
		sweep:         func(context.Context, string) error { return nil },
		engineRun: func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
			req.OnEvent(adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_read"})
			return &run.Result{Status: "ok", RunID: req.RunID}, nil
		},
	}
	done := make(chan error, 1)
	go func() {
		_, err := r.RunAgent("https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunAgent: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a stalled log kept the run's result from returning")
	}
}

// A local run hands the engine its event hook, and what the agent did is in
// the run's log, under the run's id, before RunAgent returns: the page stops
// taking a run's lines once its result is in, so a line written after it
// would be lost.
func TestALocalRunsLogShowsWhatTheAgentDid(t *testing.T) {
	var mu sync.Mutex
	var lines []RunLine
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev != eventRunLine || len(data) != 1 {
			return
		}
		if l, ok := data[0].(RunLine); ok {
			time.Sleep(time.Millisecond) // a webview that takes a moment per line
			mu.Lock()
			lines = append(lines, l)
			mu.Unlock()
		}
	}
	t.Cleanup(func() { emitEvent = emit })

	r := &RunAPI{
		runnerHomeDir: t.TempDir(),
		app:           &App{ctx: context.Background()},
		newRunID:      func() (string, error) { return "ab12cd34", nil },
		sweep:         func(context.Context, string) error { return nil },
		engineRun: func(_ context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
			if req.OnEvent == nil {
				t.Error("the engine was given no event hook")
				return &run.Result{Status: "ok", RunID: req.RunID}, nil
			}
			_, _ = io.WriteString(logw, "pulling images\n")
			req.OnEvent(adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_read", CallID: "c1"})
			req.OnEvent(adapter.Event{Kind: adapter.EventToolResult, Tool: "fs_read", CallID: "c1", Text: "hello from the workspace"})
			for range 40 {
				req.OnEvent(adapter.Event{Kind: adapter.EventToolCall, Tool: "fs_list"})
			}
			req.OnEvent(adapter.Event{Kind: adapter.EventAssistantText, Text: "The note says hello."})
			return &run.Result{Status: "ok", RunID: req.RunID}, nil
		},
	}
	if _, err := r.RunAgent("https://r.example", "acme/agent@1.0.0", "task", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil); err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	var got []string
	for _, l := range lines {
		if l.RunID != "ab12cd34" {
			t.Errorf("a line of another run: %+v", l)
		}
		got = append(got, l.Line)
	}
	for _, want := range []string{"pulling images", "Tool call: fs_read", "Tool result (fs_read):", "    hello from the workspace", "Agent: The note says hello."} {
		if !slices.Contains(got, want) {
			t.Errorf("the log %q has no %q", got, want)
		}
	}
	if i, j := slices.Index(got, "Tool call: fs_read"), slices.Index(got, "Agent: The note says hello."); i < 0 || j < i {
		t.Errorf("the agent's lines are out of order: %q", got)
	}
}
