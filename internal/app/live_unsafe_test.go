package app

// Unsafe Mode live, as an end-to-end check: the bound methods driven as the
// page drives them, against a real registry, this machine's docker and a
// real model, with the person's part (the native dialog, the file picker)
// answered by the test. Skipped unless AGENTRY_LIVE_UNSAFE=1, because it
// pulls the Unsafe Mode image, builds an exec image from Kali's repository
// and PyPI, and spends model tokens:
//
//	AGENTRY_LIVE_UNSAFE=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home with a policy.yaml naming a model> \
//	AGENTRY_LIVE_UNSAFE_INPUT=<a binary for the agent to take apart> \
//	AGENTRY_LIVE_UNSAFE_OUT=<a folder to keep the runs' output in, optional> \
//	AGENTRY_LIVE_UNSAFE_EXPECT_FAILED=<a package or extra expected NOT to install, optional> \
//	go test -run TestLiveUnsafeMode -v -timeout 90m ./internal/app
//
// It refuses to run on the person's own settings: both directories must be
// named, so the switch it turns on is never the real desktop.yaml's. The
// model key is the one the policy's apiKeyEnv names, in the environment. The
// registry and release default to production's
// blaktron/binary-reverse-engineer@0.1.0, the QuickAgent the check was
// written for (AGENTRY_LIVE_UNSAFE_REGISTRY, AGENTRY_LIVE_UNSAFE_REF).
//
// What it holds: with the switch off the run is refused by the exec
// catalogue; the switch goes on only through the dialog and the image's
// pull; with it on the same run completes, its receipt's exec line names
// Unsafe Mode and the image, and the run's transcript shows the agent
// calling a declared command the catalogue never carried; turned off
// again, the run is refused again though an Unsafe Mode plan is installed.
// The forced install failure is the caller's to arrange, with a policy whose
// images.unsafe names a base whose index maps a command to a package Kali
// does not carry. A base must be pinned by digest, so it goes through a
// registry on this machine:
//
//	docker run -d --name unsafe-registry -p 127.0.0.1:5418:5000 registry:2
//	printf 'FROM <the pinned agentry-unsafe-kali ref>\nRUN sed -i "s/^binwalk\\t.*/binwalk\\tagentry-no-such-package/" /etc/agentry/unsafe/commands.tsv\n' > Dockerfile
//	docker build -t localhost:5418/agentry-unsafe-kali-forced:1 . && docker push localhost:5418/agentry-unsafe-kali-forced:1
//	# policy.yaml: images.unsafe: localhost:5418/agentry-unsafe-kali-forced:1@sha256:<the pushed digest>
//
// EXPECT_FAILED=agentry-no-such-package then holds the receipt to naming it
// and the command it leaves absent, while the run still completes.

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func TestLiveUnsafeMode(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_UNSAFE") != "1" {
		t.Skip("runs a real Unsafe Mode run: set AGENTRY_LIVE_UNSAFE=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s and %s to scratch directories: this test turns Unsafe Mode on and must not do it in your own settings", configdir.EnvVar, policy.HomeEnv)
	}
	input := os.Getenv("AGENTRY_LIVE_UNSAFE_INPUT")
	if input == "" {
		t.Fatal("set AGENTRY_LIVE_UNSAFE_INPUT to a binary for the agent to analyse")
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_UNSAFE_REGISTRY"), "https://secureagentry.net")
	ref := cmp.Or(os.Getenv("AGENTRY_LIVE_UNSAFE_REF"), "blaktron/binary-reverse-engineer@0.1.0")
	outRoot := cmp.Or(os.Getenv("AGENTRY_LIVE_UNSAFE_OUT"), t.TempDir())
	// A bounded task: a model left to itself spends the whole turn budget on
	// a small binary, and the check is that the run completes with Ghidra's
	// output, not how far an agent will dig.
	const task = "Identify what this binary is. Run Ghidra's analyzeHeadless once to decompile it, quote the decompiled main function in your report, say which of your tools were not available, and finish. Use at most 20 tool calls."

	// No webview: what the panel would be shown goes to the test's log.
	var mu sync.Mutex
	var lines []string
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) == 1 {
			if s, ok := data[0].(string); ok {
				mu.Lock()
				lines = append(lines, s)
				mu.Unlock()
				t.Logf("[%s] %s", name, s)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })
	logged := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, l := range lines {
			if strings.Contains(l, want) {
				return true
			}
		}
		return false
	}

	var dialogs []string
	answer := false
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		dialogs = append(dialogs, title+"\n"+message)
		return answer, nil
	}}
	app.markPicked(input, outRoot) // what the native pickers record
	r := &RunAPI{app: app}
	runIt := func(name string) (*RunResultView, error) {
		out := filepath.Join(outRoot, name)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		return r.RunAgent(reg, ref, task, []string{input}, out, nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil)
	}

	// 1. Off: refused, as today.
	if unsafeModeOn() {
		t.Fatal("the scratch settings already have Unsafe Mode on")
	}
	if _, err := runIt("off"); err == nil || !strings.Contains(err.Error(), "does not catalogue") {
		t.Fatalf("with Unsafe Mode off: err = %v, want the exec catalogue refusal", err)
	} else {
		t.Logf("off: refused: %v", err)
	}
	if n, err := r.UnsafeNotice(reg, ref); err != nil || n.On {
		t.Fatalf("the notice with the switch off: %+v, %v", n, err)
	}

	// 2. On: a No leaves it off; a Yes pulls the image and only then is
	// the switch on.
	if _, err := r.SetUnsafeMode(true); err == nil || unsafeModeOn() {
		t.Fatalf("the dialog answered No turned Unsafe Mode on (err %v)", err)
	}
	answer = true
	v, err := r.SetUnsafeMode(true)
	if err != nil {
		t.Fatalf("turning Unsafe Mode on: %v", err)
	}
	if !v.UnsafeMode || !unsafeModeOn() || len(dialogs) != 2 || !logged("the Unsafe Mode image is ready") {
		t.Fatalf("after Yes: view %+v, dialogs %d", v, len(dialogs))
	}
	t.Logf("on: %s", v.UnsafeImage)
	n, err := r.UnsafeNotice(reg, ref)
	if err != nil || !n.On || !strings.Contains(n.Line, "Unsafe Mode is on") {
		t.Fatalf("the notice with the switch on: %+v, %v", n, err)
	}
	t.Logf("the run's line: %s", n.Line)

	// 3. The same run completes. The release is verified and scanned, so
	// no consent is due and no dialog is raised.
	res, err := runIt("on")
	if err != nil {
		t.Fatalf("with Unsafe Mode on: %v", err)
	}
	t.Logf("on: status %s, exit %d, %d turns, run %s, folder %s\nresult: %s\nexec: %s", res.Status, res.ExitCode, res.Turns, res.RunID, res.OutDir, res.ResultJSON, res.UnsafeExec)
	if res.Status != "ok" {
		t.Fatalf("the Unsafe Mode run ended %q: %s", res.Status, res.Error)
	}
	if len(dialogs) != 2 {
		t.Fatalf("a run with nothing to accept raised a dialog: %q", dialogs[2:])
	}
	for _, want := range []string{"UNSAFE MODE:", v.UnsafeImage, "this machine's exec catalogue was not consulted", "analyzeHeadless ← link ghidra:"} {
		if !strings.Contains(res.UnsafeExec, want) {
			t.Errorf("the receipt's exec line lacks %q", want)
		}
	}
	if want := os.Getenv("AGENTRY_LIVE_UNSAFE_EXPECT_FAILED"); want != "" {
		if !strings.Contains(res.UnsafeExec, "NOT installed at image build") || !strings.Contains(res.UnsafeExec, want+" (") {
			t.Errorf("the receipt does not name %s as not installed:\n%s", want, res.UnsafeExec)
		}
	} else if strings.Contains(res.UnsafeExec, "NOT installed at image build") {
		t.Errorf("an install failed that was not forced:\n%s", res.UnsafeExec)
	}
	// Ghidra ran: the run's event receipt carries a run_command call of
	// analyzeHeadless whose answer is Ghidra's own import report.
	if !ghidraRan(t, res.ReceiptsDir) {
		t.Errorf("no run_command of analyzeHeadless in %s/events.ndjson answered with Ghidra's import report: the agent did not get Ghidra's output", res.ReceiptsDir)
	}

	// 4. Off again: immediate, and refused again although an Unsafe Mode
	// plan is installed for this release.
	if v, err := r.SetUnsafeMode(false); err != nil || v.UnsafeMode || len(dialogs) != 2 {
		t.Fatalf("turning it off: %+v, %v, dialogs %d", v, err, len(dialogs))
	}
	if _, err := runIt("off-again"); err == nil || !strings.Contains(err.Error(), "does not catalogue") {
		t.Fatalf("with Unsafe Mode off again: err = %v, want the exec catalogue refusal", err)
	} else {
		t.Logf("off again: refused: %v", err)
	}
	if !logged("compiled in Unsafe Mode, which is off") {
		t.Error("the run log does not say the Unsafe Mode plan was recompiled")
	}
}

// ghidraRan reads the run's event receipt for a run_command call of
// analyzeHeadless that Ghidra answered with an import report. The receipt's
// other files name analyzeHeadless whatever the agent did (the allowlist,
// the task), so only a call and its answer count.
func ghidraRan(t *testing.T, receiptsDir string) bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(receiptsDir, "events.ndjson")) //nolint:gosec // the run's own folder
	if err != nil {
		t.Logf("read the event receipt: %v", err)
		return false
	}
	calls := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		var ev struct {
			Type     string `json:"type"`
			ToolCall struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_call"`
			ToolCallID string `json:"tool_call_id"`
			Response   string `json:"response"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "tool_call":
			var args struct {
				Command string `json:"command"`
			}
			if ev.ToolCall.Function.Name == "run_command" && json.Unmarshal([]byte(ev.ToolCall.Function.Arguments), &args) == nil && args.Command == "analyzeHeadless" {
				calls[ev.ToolCall.ID] = true
			}
		case "tool_call_response":
			if calls[ev.ToolCallID] && strings.Contains(ev.Response, "REPORT: Import succeeded") {
				t.Logf("Ghidra's answer to analyzeHeadless (%d bytes) reports the import succeeded", len(ev.Response))
				return true
			}
		}
	}
	return false
}
