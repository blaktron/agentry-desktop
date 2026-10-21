package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// stateEvents counts the runner:agent-state events each agent was named in,
// beside the rig's own record of its agent events.
func stateEvents(t *testing.T, rig *persistentRig) func(name string) int {
	t.Helper()
	counts := map[string]int{}
	inner := emitEvent
	emitEvent = func(a *App, name string, data ...any) {
		if name == eventAgentState && len(data) == 1 {
			if agent, ok := data[0].(string); ok {
				rig.mu.Lock()
				counts[agent]++
				rig.mu.Unlock()
			}
		}
		inner(a, name, data...)
	}
	t.Cleanup(func() { emitEvent = inner })
	return func(name string) int {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		return counts[name]
	}
}

func readLog(t *testing.T, home, name string) string {
	t.Helper()
	b, err := os.ReadFile(persistent.LogPath(home, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// While an agent starts, its view carries the step the start is on, in the
// CLI's words, and each step is pushed with a state event. The start's
// progress and its steps go to the agent's supervisor.log, as the CLI's
// supervisor writes them; once the agent runs, the view carries no step.
func TestAStartsStagesShowUnderTheAgentAndInItsLog(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(persistent.LogPath(rig.home, "assistant"), []byte("a line of the start before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	states := stateEvents(t, rig)
	reached, release := make(chan struct{}), make(chan struct{})
	var opts persistent.Options
	rig.p.start = func(_ context.Context, o persistent.Options) (agentSupervisor, error) {
		opts = o
		o.OnStage(persistent.Stage{Name: receipt.StageCompile})
		_, _ = fmt.Fprintln(o.Stderr, "building the runner image")
		o.OnStage(persistent.Stage{Name: receipt.StageImages})
		close(reached)
		<-release
		o.OnStage(persistent.Stage{Name: receipt.StageAgent})
		return rig.agent, nil
	}
	errc := make(chan error, 1)
	go func() {
		_, err := rig.p.Start("assistant")
		errc <- err
	}()
	<-reached
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if v.State != agentStarting || v.Stage != "Getting images ready" {
		t.Fatalf("while starting: state %q, stage %q", v.State, v.Stage)
	}
	if n := states("assistant"); n < 3 {
		t.Fatalf("%d state events by the images step: the start and each step push one", n)
	}
	close(release)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	v, _ = rig.p.Get("assistant")
	if v.State != persistent.StateRunning || v.Stage != "" {
		t.Fatalf("running: state %q, stage %q", v.State, v.Stage)
	}
	// A restart for a change reports its steps too: logged, not shown.
	opts.OnStage(persistent.Stage{Name: receipt.StageInfra})
	if v, _ = rig.p.Get("assistant"); v.Stage != "" {
		t.Fatalf("a running agent's restart shows %q", v.Stage)
	}
	got := readLog(t, rig.home, "assistant")
	want := "stage: Preparing\nbuilding the runner image\nstage: Getting images ready\nstage: Starting the agent\nstage: Starting the gateway\n"
	if got != want {
		t.Fatalf("supervisor.log:\n%s\nwant:\n%s", got, want)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(persistent.LogPath(rig.home, "assistant")); fi.Mode().Perm() != 0o600 {
			t.Fatalf("supervisor.log's mode is %v", fi.Mode().Perm())
		}
	}
}

// Stop while an agent starts cancels the start, which tears down what it
// made, and waits for it: the agent is stopped, and neither the Start nor
// the Stop is an error, nor is a failure recorded.
func TestStopDuringAStartCancelsIt(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	entered, tornDown := make(chan struct{}), make(chan struct{})
	rig.p.start = func(ctx context.Context, o persistent.Options) (agentSupervisor, error) {
		o.OnStage(persistent.Stage{Name: receipt.StageMCPServers, Subject: "office-word-mcp-server"})
		close(entered)
		<-ctx.Done()
		close(tornDown)
		return nil, fmt.Errorf("install office-word-mcp-server: %w", ctx.Err())
	}
	errc := make(chan error, 1)
	go func() {
		_, err := rig.p.Start("assistant")
		errc <- err
	}()
	<-entered
	v, err := rig.p.Stop("assistant")
	if err != nil {
		t.Fatalf("Stop during a start: %v", err)
	}
	select {
	case <-tornDown:
	default:
		t.Fatal("Stop returned before the start was torn down")
	}
	if v.State != agentStopped || v.Failure != nil {
		t.Fatalf("after Stop: state %q, failure %+v", v.State, v.Failure)
	}
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("the cancelled start answered %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the start did not end")
	}
	if log := readLog(t, rig.home, "assistant"); !strings.Contains(log, "stage: Installing office-word-mcp-server\n") || !strings.Contains(log, "The start was cancelled.") {
		t.Fatalf("supervisor.log:\n%s", log)
	}
}

// A failed start keeps its error, the step it stopped in and its reason on
// the agent's view, and in its log, until the next start.
func TestAFailedStartShowsItsErrorAndReason(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	rig.p.start = func(_ context.Context, o persistent.Options) (agentSupervisor, error) {
		o.OnStage(persistent.Stage{Name: receipt.StageMCPServers, Subject: "office-word-mcp-server"})
		return nil, receipt.WithReason(errors.New("pip install exited 1"), receipt.ReasonMCPInstallFailed, "", "pypi:office-word-mcp-server")
	}
	if _, err := rig.p.Start("assistant"); err == nil || !strings.Contains(err.Error(), "pip install exited 1") {
		t.Fatalf("the failed start answered %v", err)
	}
	v, _ := rig.p.Get("assistant")
	f := v.Failure
	if v.State != agentStopped || f == nil {
		t.Fatalf("after a failed start: state %q, failure %+v", v.State, f)
	}
	if f.Error != "pip install exited 1" || f.Stage != "Installing office-word-mcp-server" || f.Log != persistent.LogPath(rig.home, "assistant") {
		t.Fatalf("the failure: %+v", f)
	}
	if f.Reason == nil || f.Reason.Code != string(receipt.ReasonMCPInstallFailed) || strings.Join(f.Reason.Names, ",") != "pypi:office-word-mcp-server" {
		t.Fatalf("the reason: %+v", f.Reason)
	}
	if log := readLog(t, rig.home, "assistant"); !strings.Contains(log, "The start failed: pip install exited 1") {
		t.Fatalf("supervisor.log:\n%s", log)
	}
	rig.p.start = func(context.Context, persistent.Options) (agentSupervisor, error) { return rig.agent, nil }
	if v, err := rig.p.Start("assistant"); err != nil || v.Failure != nil {
		t.Fatalf("the next start: %v, failure %+v", err, v.Failure)
	}
}

// A segment that failed in the engine wrote its envelope: the failure is
// the envelope's error and reason.
func TestAFailedStartReadsItsSegmentsEnvelope(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	rig.p.start = func(_ context.Context, o persistent.Options) (agentSupervisor, error) {
		o.OnStage(persistent.Stage{Name: receipt.StageInfra})
		dir := filepath.Join(persistent.Dir(rig.home, "assistant"), "segments", "run-1")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		env := receipt.Envelope{Status: "error", Error: "the gateway could not start office-word-mcp-server", Stage: receipt.StageInfra,
			Reason: &receipt.Reason{Code: receipt.ReasonMCPStartFailed, Names: []string{"pypi:office-word-mcp-server"}}}
		b, _ := json.Marshal(env)
		if err := os.WriteFile(filepath.Join(dir, "result.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		return nil, errors.New("gateway: exit 1")
	}
	if _, err := rig.p.Start("assistant"); err == nil {
		t.Fatal("the failed start answered nil")
	}
	v, _ := rig.p.Get("assistant")
	f := v.Failure
	if f == nil || f.Error != "the gateway could not start office-word-mcp-server" || f.Stage != "Starting the gateway" ||
		f.Reason == nil || f.Reason.Code != string(receipt.ReasonMCPStartFailed) {
		t.Fatalf("the failure: %+v", f)
	}
}

// A start refused because another client holds the agent leaves that
// client's supervisor.log as it is, and records no failure: the agent is
// running elsewhere.
func TestAStartRefusedByAnotherOwnerLeavesItsLog(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(persistent.LogPath(rig.home, "assistant"), []byte("the CLI's start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rig.p.start = func(context.Context, persistent.Options) (agentSupervisor, error) {
		return nil, &persistent.HeldError{Agent: "assistant"}
	}
	if _, err := rig.p.Start("assistant"); err == nil {
		t.Fatal("a held agent started")
	}
	if log := readLog(t, rig.home, "assistant"); log != "the CLI's start\n" {
		t.Fatalf("supervisor.log:\n%s", log)
	}
	if v, _ := rig.p.Get("assistant"); v.Failure != nil {
		t.Fatalf("a held agent recorded %+v", v.Failure)
	}
}

// A Stop that comes as the start finishes stops the agent that started.
func TestStopAsAStartFinishesStopsTheAgent(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	rig.p.start = func(ctx context.Context, _ persistent.Options) (agentSupervisor, error) {
		close(entered)
		<-ctx.Done()
		return rig.agent, nil
	}
	errc := make(chan error, 1)
	go func() {
		_, err := rig.p.Start("assistant")
		errc <- err
	}()
	<-entered
	v, err := rig.p.Stop("assistant")
	if err != nil || v.State != agentStopped {
		t.Fatalf("Stop: %v, state %+v", err, v)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	rig.agent.mu.Lock()
	stopped := rig.agent.stopped
	rig.agent.mu.Unlock()
	if stopped != 1 {
		t.Fatalf("the agent that started was stopped %d times", stopped)
	}
}

// A Stop while a start is refused because another client runs the agent
// leaves that client's log as it is.
func TestStopDuringARefusedStartLeavesTheOtherClientsLog(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(persistent.LogPath(rig.home, "assistant"), []byte("the CLI's start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	rig.p.start = func(ctx context.Context, _ persistent.Options) (agentSupervisor, error) {
		close(entered)
		<-ctx.Done()
		return nil, &persistent.HeldError{Agent: "assistant"}
	}
	errc := make(chan error, 1)
	go func() {
		_, err := rig.p.Start("assistant")
		errc <- err
	}()
	<-entered
	if _, err := rig.p.Stop("assistant"); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("the cancelled start answered %v", err)
	}
	if log := readLog(t, rig.home, "assistant"); log != "the CLI's start\n" {
		t.Fatalf("supervisor.log:\n%s", log)
	}
}

// A Stop while the start's dialog is up cancels the start, and a Yes given
// after it agrees to nothing: the next start asks again.
func TestStopDuringTheStartsDialogAgreesToNothing(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	asking, answer := make(chan struct{}), make(chan struct{})
	rig.p.app.confirm = func(context.Context, string, string) (bool, error) {
		close(asking)
		<-answer
		return true, nil
	}
	errc := make(chan error, 1)
	go func() {
		_, err := rig.p.Start("assistant")
		errc <- err
	}()
	<-asking
	stopped := make(chan error, 1)
	go func() {
		_, err := rig.p.Stop("assistant")
		stopped <- err
	}()
	// The Stop has cancelled the start once the agent's view says so.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rig.p.mu.Lock()
		cancelled := rig.p.starting["assistant"] != nil && rig.p.starting["assistant"].stopped
		rig.p.mu.Unlock()
		if cancelled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the Stop did not reach the start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(answer)
	if err := <-errc; err != nil {
		t.Fatalf("the cancelled start answered %v", err)
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if rig.started != 0 {
		t.Fatal("a cancelled start started the agent")
	}
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := persistent.Lookup("assistant")
	if len(persistent.Unconsented(persistent.BuildRoster(a, cfg, nil), cfg)) == 0 {
		t.Fatal("a Yes after the Stop was recorded as an agreement")
	}
}
