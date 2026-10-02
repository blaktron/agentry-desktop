package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

// twoModeYAML is a schema v2 release declaring both run modes and an intake
// question, for the interactive lifecycle.
const twoModeYAML = `schemaVersion: "2"
name: chatty
namespace: acme
modes: [autonomous, interactive]
version: 1.0.0
summary: "desktop session fixture"
intake:
  - id: region
    prompt: "Which region?"
    type: choice
    choices: [emea, apac]
interactive:
  opening: "What should I look at?"
publish:
  include: [agent.yaml, AGENT.md]
`

// fakeACPHarness plays docker-agent's ACP server on the agent container's
// attached streams: one tool call and an echo per prompt, exit on EOF.
func fakeACPHarness(_ context.Context, r dockerx.RunSpec, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	if !strings.Contains(strings.Join(r.Argv, " "), "serve acp") {
		return 2, nil
	}
	write := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = stdout.Write(append(b, '\n'))
	}
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Prompt []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			} `json:"params"`
		}
		_ = json.Unmarshal(sc.Bytes(), &m)
		switch m.Method {
		case "initialize":
			write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"protocolVersion": 1}})
		case "session/new":
			write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"sessionId": "s1"}})
		case "session/prompt":
			text := ""
			if len(m.Params.Prompt) > 0 {
				text = m.Params.Prompt[0].Text
			}
			write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": map[string]any{"sessionUpdate": "tool_call", "toolCallId": "c1", "title": "fs_list", "status": "pending"}}})
			write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "echo: " + text}}}})
			write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": 5, "outputTokens": 2, "totalTokens": 7}}})
		default:
			write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "unknown"}})
		}
	}
	return 0, nil
}

func sessionFixture(t *testing.T) (string, *dockerx.Fake) {
	t.Helper()
	return sessionFixtureFor(t, twoModeYAML)
}

// sessionFixtureFor is sessionFixture for a release of the caller's.
func sessionFixtureFor(t *testing.T, agentYAML string) (string, *dockerx.Fake) {
	t.Helper()
	isolateRunEnv(t)
	fr := &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, agentYAML),
		archive:     buildRunArchive(t, map[string]string{"agent.yaml": agentYAML, "AGENT.md": "Chat helpfully.\n"}),
		namespace:   "acme", name: "chatty", version: "1.0.0",
	}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	fake.Foreground = map[string]*dockerx.FakeForeground{
		"-probe": fakeProbe(),
		"-agent": {Handler: fakeACPHarness},
	}
	scriptDecode(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	return regURL, fake
}

// TestStartSessionLifecycle drives StartSession → Send → AddInput → End
// through the same install/plan/profile path as Run and checks the
// flattened envelope, the transcript receipt, and the intake staging.
func TestStartSessionLifecycle(t *testing.T) {
	regURL, fake := sessionFixture(t)
	outDir := filepath.Join(t.TempDir(), "out")
	var log strings.Builder
	s, err := StartSession(context.Background(), Request{
		AcceptUnsigned: true, Registry: regURL, Ref: "acme/chatty@1.0.0", OutDir: outDir, MaxTurns: -1,
		Intake: map[string]string{"region": "apac"},
	}, &log)
	if err != nil {
		t.Fatalf("StartSession: %v\nlog:\n%s", err, log.String())
	}
	if s.Ref() != "acme/chatty@1.0.0" {
		t.Errorf("ref = %q", s.Ref())
	}
	var events []session.Event
	done := make(chan struct{})
	go func() {
		for ev := range s.Events() {
			events = append(events, ev)
		}
		close(done)
	}()
	if err := s.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	inputFile := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(inputFile, []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.AddInput(context.Background(), inputFile); err != nil {
		t.Fatalf("AddInput: %v", err)
	}
	res, err := s.End(context.Background(), true)
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	<-done
	if res.Status != "ok" || res.ExitCode != 0 || !strings.Contains(res.ResultJSON, "echo:") {
		t.Fatalf("result = %+v", res)
	}
	assertPolicyBlock(t, res)
	for _, name := range []string{"transcript.md", "events.ndjson", "intake.json"} {
		if _, err := os.Stat(filepath.Join(res.ReceiptsDir, name)); err != nil {
			t.Errorf("receipt %s: %v", name, err)
		}
	}
	tr, _ := os.ReadFile(filepath.Join(res.ReceiptsDir, "transcript.md"))
	if !strings.Contains(string(tr), "## User\n\nhello") || !strings.Contains(string(tr), "input added: /inputs/notes.txt") {
		t.Errorf("transcript:\n%s", tr)
	}
	kinds := map[session.Kind]int{}
	for _, ev := range events {
		kinds[ev.Kind]++
	}
	if kinds[session.KindIntake] != 1 || kinds[session.KindUserTurn] != 2 || kinds[session.KindSessionEnd] != 1 {
		t.Errorf("event kinds = %v", kinds)
	}
	// End is idempotent, and the session's sandbox is gone.
	if res2, err2 := s.End(context.Background(), true); err2 != nil || res2 != res {
		t.Errorf("second End = %v %v", res2, err2)
	}
	if len(fake.Leftover) != 0 {
		t.Errorf("leftovers: %v", fake.Leftover)
	}
}

// TestStartSessionRefusesAutonomousOnlyRelease: a release without the
// interactive mode cannot start a session.
func TestStartSessionRefusesAutonomousOnlyRelease(t *testing.T) {
	_, regURL, _, _ := runFixture(t) // the v1 skill fixture: autonomous only
	_, err := StartSession(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{})
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("err = %v, want ErrNotInteractive", err)
	}
}

// idleHintYAML is twoModeYAML with the release's own idle hint, which the
// CLI's compile applies when it is shorter than the policy's.
var idleHintYAML = strings.Replace(twoModeYAML, `  opening: "What should I look at?"`,
	"  opening: \"What should I look at?\"\n  idleTimeoutSeconds: 60", 1)

// runPlanOf reads the plan the engine was handed, as the receipt keeps it
// (receipts/run-plan.json), and its session caps.
func runPlanOf(t *testing.T, res *Result) (spec.Plan, spec.SessionBudgets) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(res.ReceiptsDir, "run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan spec.Plan
	if err := json.Unmarshal(b, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Harness.Session == nil {
		t.Fatalf("the run plan has no session block:\n%s", b)
	}
	return plan, plan.Harness.Session.Budgets
}

// A chat the desktop starts has no idle timeout, no wall clock and no
// count of the person's turns (agentry-desktop#468; plan
// desktop-review-key-and-run-cards.md D21, D22): not the runner policy's
// 900 s, 7,200 s and 200, and not the release's own 60 s idle hint; and
// its receipt names no run timeout, which a session never applies. The
// policy file, which the CLI's chats read, is not written; the tool-call
// budget is still max turns, the one the setup names when it names one.
func TestAChatOnThisComputerHasNoLimitsByDefault(t *testing.T) {
	regURL, _ := sessionFixtureFor(t, idleHintYAML)
	polPath := filepath.Join(os.Getenv(policy.HomeEnv), "policy.yaml")
	if _, _, err := policy.EnsureDefault(polPath); err != nil {
		t.Fatal(err)
	}
	polBefore, err := os.ReadFile(polPath)
	if err != nil {
		t.Fatal(err)
	}
	statBefore, err := os.Stat(polPath)
	if err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	s, err := StartSession(context.Background(), Request{
		AcceptUnsigned: true, Registry: regURL, Ref: "acme/chatty@1.0.0", OutDir: filepath.Join(t.TempDir(), "out"), MaxTurns: 7,
		Intake: map[string]string{"region": "apac"},
	}, &log)
	if err != nil {
		t.Fatalf("StartSession: %v\nlog:\n%s", err, log.String())
	}
	go func() {
		for range s.Events() {
		}
	}()
	res, err := s.End(context.Background(), false)
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	plan, got := runPlanOf(t, res)
	if got != (spec.SessionBudgets{}) {
		t.Errorf("the chat ran under %+v, want no limits", got)
	}
	if plan.Budgets.TimeoutSeconds != 0 {
		t.Errorf("the run plan names a run timeout of %d s for a chat", plan.Budgets.TimeoutSeconds)
	}
	var meta struct {
		Budgets struct {
			TimeoutSeconds int `json:"timeoutSeconds"`
			MaxTurns       int `json:"maxTurns"`
		} `json:"budgets"`
	}
	mb, err := os.ReadFile(filepath.Join(res.ReceiptsDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Budgets.MaxTurns != 7 || meta.Budgets.TimeoutSeconds != 0 {
		t.Errorf("meta.json budgets = %+v, want the setup's 7 tool calls and no run timeout", meta.Budgets)
	}
	polAfter, err := os.ReadFile(polPath)
	if err != nil {
		t.Fatal(err)
	}
	statAfter, err := os.Stat(polPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(polAfter) != string(polBefore) || !statAfter.ModTime().Equal(statBefore.ModTime()) {
		t.Errorf("the runner policy was written by a chat")
	}
	pol, _, err := policy.Load(polPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := (policy.Sessions{IdleTimeoutSeconds: 900, MaxWallSeconds: 7200, MaxUserTurns: 200}); pol.Sessions != want {
		t.Errorf("the policy's sessions = %+v, want the CLI's %+v", pol.Sessions, want)
	}
}

// Run defaults' Chat idle timeout is the one limit a chat here runs under
// when it is set, in place of the release's own hint even when that is
// shorter; an idle chat then ends with the engine's idle_timeout and its
// receipt (agentry-desktop#468, D32). One second keeps the test short:
// internal/run takes any seconds from 0, and only the setting holds the
// person to whole minutes.
func TestAChatEndsAfterTheChosenIdleTimeout(t *testing.T) {
	regURL, _ := sessionFixtureFor(t, idleHintYAML)
	var log strings.Builder
	if _, err := StartSession(context.Background(), Request{
		AcceptUnsigned: true, Registry: regURL, Ref: "acme/chatty@1.0.0", OutDir: filepath.Join(t.TempDir(), "out"), MaxTurns: -1,
		Intake: map[string]string{"region": "apac"}, ChatIdleTimeoutSeconds: -1,
	}, &log); err == nil || !strings.Contains(err.Error(), "idle timeout is 0 (none) or more seconds") {
		t.Fatalf("a negative idle timeout: err = %v, want the refusal", err)
	}
	s, err := StartSession(context.Background(), Request{
		AcceptUnsigned: true, Registry: regURL, Ref: "acme/chatty@1.0.0", OutDir: filepath.Join(t.TempDir(), "out"), MaxTurns: -1,
		Intake: map[string]string{"region": "apac"}, ChatIdleTimeoutSeconds: 1,
	}, &log)
	if err != nil {
		t.Fatalf("StartSession: %v\nlog:\n%s", err, log.String())
	}
	var events []session.Event
	done := make(chan struct{})
	go func() {
		for ev := range s.Events() {
			events = append(events, ev)
		}
		close(done)
	}()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if ended, why := s.Ended(); ended {
			if why != session.EndIdle {
				t.Fatalf("the chat ended for %q, want %q", why, session.EndIdle)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("an idle chat with a 1 s idle timeout had not ended after 15 s")
		}
		time.Sleep(100 * time.Millisecond)
	}
	res, err := s.End(context.Background(), false)
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	<-done
	if _, got := runPlanOf(t, res); got != (spec.SessionBudgets{IdleTimeoutSeconds: 1}) {
		t.Errorf("the chat ran under %+v, want an idle timeout of 1 s alone", got)
	}
	if res.ExitCode != engine.ExitTimeout || !strings.Contains(res.Error, "idle") {
		t.Errorf("result = %+v, want the idle timeout's exit", res)
	}
	var end *session.Event
	for i := range events {
		if events[i].Kind == session.KindSessionEnd {
			end = &events[i]
		}
	}
	if end == nil || end.Reason != session.EndIdle {
		t.Errorf("session_end = %+v, want reason %q", end, session.EndIdle)
	}
	if _, err := os.Stat(filepath.Join(res.ReceiptsDir, "events.ndjson")); err != nil {
		t.Errorf("receipt: %v", err)
	}
}
