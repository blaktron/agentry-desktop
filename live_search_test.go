package main

// The runner's grep and glob in a local run live (agentry-desktop#877;
// plan ripgrep-search.md M6): RunAgent driven as the run panel drives it,
// against a real registry, this machine's docker and a real model, so a
// desktop run carries the CLI's search tools (agentry-cli#912, first in
// v0.6.114). Skipped unless AGENTRY_LIVE_SEARCH=1, because it pulls the
// runner image and spends model tokens:
//
//	AGENTRY_LIVE_SEARCH=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home with a policy.yaml naming a model> \
//	AGENTRY_LIVE_SEARCH_OUT=<a folder to keep the run's output in, optional> \
//	go test -run TestLiveSearchTools -v -timeout 30m .
//
// Both directories must be named, so nothing is written to the person's
// own settings or runner home. The model key is the one the policy's
// apiKeyEnv names, in the environment. The registry and release default to
// production's blaktron/resume-ats-review, a public autonomous release with
// no exec and no egress (AGENTRY_LIVE_SEARCH_REGISTRY, _REF).
//
// What it holds: the run completes; its calls ledger records a grep over
// the input; and the report quotes the two lines that match, so the tool
// answered with matches, not only that it was offered.

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func TestLiveSearchTools(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_SEARCH") != "1" {
		t.Skip("runs a real local run: set AGENTRY_LIVE_SEARCH=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s and %s to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_SEARCH_REGISTRY"), "https://secureagentry.net")
	ref := cmp.Or(os.Getenv("AGENTRY_LIVE_SEARCH_REF"), "blaktron/resume-ats-review")
	outRoot := cmp.Or(os.Getenv("AGENTRY_LIVE_SEARCH_OUT"), t.TempDir())

	dir := t.TempDir()
	input := filepath.Join(dir, "ledger.txt")
	if err := os.WriteFile(input, []byte("INV-1001 alpha\nplain line\nINV-1002 beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const task = `This run tests the runner's search tools, not this QuickAgent's usual task. ` +
		`Call grep with path "/inputs", pattern "INV-[0-9]+" and output "content", once. ` +
		`Then write a short report with write_report that quotes grep's answer exactly, and finish.`

	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) == 1 {
			if s, ok := data[0].(string); ok {
				t.Logf("[%s] %s", name, s)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		t.Logf("dialog: %s\n%s", title, message)
		return true, nil
	}}
	app.markPicked(input, outRoot) // what the native pickers record (D-030)
	r := &RunAPI{app: app}

	out := filepath.Join(outRoot, "search")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := r.RunAgent(reg, ref, task, []string{input}, out, nil, 0, -1, false, nil, false, false, nil, false, nil, nil)
	if err != nil {
		t.Fatalf("the run: %v", err)
	}
	t.Logf("status %s, exit %d, %d turns, run %s, folder %s", res.Status, res.ExitCode, res.Turns, res.RunID, res.OutDir)
	if res.Status != "ok" {
		t.Fatalf("the run ended %q: %s", res.Status, res.Error)
	}

	calls, err := os.ReadFile(filepath.Join(res.OutDir, "receipts", "calls.ndjson"))
	if err != nil {
		t.Fatalf("the calls ledger: %v", err)
	}
	if !strings.Contains(string(calls), `"tool":"grep"`) {
		t.Fatalf("the calls ledger records no grep:\n%s", calls)
	}
	var report string
	reports, _ := filepath.Glob(filepath.Join(res.OutDir, "reports", "*"))
	for _, p := range reports {
		b, err := os.ReadFile(p)
		if err == nil {
			report += string(b)
		}
	}
	t.Logf("report:\n%s", report)
	for _, want := range []string{"INV-1001 alpha", "INV-1002 beta"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report does not quote grep's match %q", want)
		}
	}
}
