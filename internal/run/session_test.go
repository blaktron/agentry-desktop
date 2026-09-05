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

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
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
	isolateRunEnv(t)
	fr := &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, twoModeYAML),
		archive:     buildRunArchive(t, map[string]string{"agent.yaml": twoModeYAML, "AGENT.md": "Chat helpfully.\n"}),
		namespace:   "acme", name: "chatty", version: "1.0.0",
	}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	fake.Foreground = map[string]*dockerx.FakeForeground{
		"-probe": {ExitCode: 0},
		"-agent": {Handler: fakeACPHarness},
	}
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
