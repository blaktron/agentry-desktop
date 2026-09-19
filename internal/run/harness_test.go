package run

// The Coding agent step's backend (agentry-desktop#89): the probe reads and
// never creates the policy; choosing writes exactly harness.<name>.path and
// leaves every other block alone; a harness that is not here is refused
// with the vendor's page named; skipping (not calling) writes nothing.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// fakeHarnessOnPath puts an executable named name on a PATH of its own
// that prints version, and returns its path. isolateRunEnv has already
// moved HOME, so the policy the tests write is the isolated one.
func fakeHarnessOnPath(t *testing.T, name, version string) string {
	t.Helper()
	bin := t.TempDir()
	exe := filepath.Join(bin, name)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho '"+version+"'\n"), 0o755); err != nil { //nolint:gosec // a test fixture
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return exe
}

func TestHarnessStatusReadsAndCreatesNothing(t *testing.T) {
	isolateRunEnv(t)
	exe := fakeHarnessOnPath(t, "claude", "2.1.263 (Claude Code)")
	hs, err := HarnessStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Harness{}
	for _, h := range hs {
		byName[h.Name] = h
	}
	c, ok := byName["claude"]
	if !ok || !c.Found || c.Path != exe || c.Version != "2.1.263 (Claude Code)" || !c.VersionMatches || c.Chosen || len(c.Warnings) != 0 {
		t.Fatalf("claude = %+v (present %v)", c, ok)
	}
	x, ok := byName["codex"]
	if !ok || x.Found || x.Path != "" || x.InstallURL != "https://github.com/openai/codex" || !strings.Contains(x.Reason, "not installed") {
		t.Fatalf("codex = %+v (present %v)", x, ok)
	}
	if _, ok := byName["docker-agent"]; ok {
		t.Fatal("the containerised harness is not a coding agent to choose")
	}
	if !strings.HasPrefix(c.MCPCommand, "claude mcp add agentry -- node ") || !strings.HasPrefix(x.MCPCommand, "codex mcp add agentry -- node ") {
		t.Fatalf("mcp commands: %q %q", c.MCPCommand, x.MCPCommand)
	}
	path, _ := policy.DefaultPath()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("HarnessStatus created %s", path)
	}
}

func TestChooseHarnessWritesThePathAndPreservesTheRest(t *testing.T) {
	isolateRunEnv(t)
	exe := fakeHarnessOnPath(t, "claude", "2.1.263")
	path, _ := policy.DefaultPath()
	pol := policy.Default()
	pol.Model = policy.Model{Provider: "custom", Name: "qwen", APIKeyEnv: "OPENAI_API_KEY", BaseURL: "https://llm.example.test/v1", ProxyAllow: []string{"llm.example.test:443"}}
	pol.Exec.Packages = map[string]string{"jq": "jq"}
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	if err := ChooseHarness(context.Background(), "claude"); err != nil {
		t.Fatal(err)
	}
	got, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Harness["claude"].Path != exe {
		t.Fatalf("harness.claude = %+v, want path %s", got.Harness["claude"], exe)
	}
	if got.Model.Provider != "custom" || got.Model.Name != "qwen" || got.Model.BaseURL != pol.Model.BaseURL || got.Exec.Packages["jq"] != "jq" || got.Model.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("other blocks changed: %+v", got)
	}
	// Only the env var NAME is in the file, never a key (plan §3).
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sk-") || !strings.Contains(string(raw), "OPENAI_API_KEY") {
		t.Fatalf("policy file: %s", raw)
	}
	// The choice shows as chosen, and a policy path is honoured next time.
	hs, _ := HarnessStatus(context.Background())
	for _, h := range hs {
		if h.Name == "claude" && (!h.Chosen || !h.PathFromPolicy || h.Path != exe) {
			t.Fatalf("after choosing: %+v", h)
		}
	}
}

func TestChooseHarnessCreatesThePolicyFromDefaultsWhenNone(t *testing.T) {
	isolateRunEnv(t)
	exe := fakeHarnessOnPath(t, "codex", "codex-cli 0.153.4")
	if err := ChooseHarness(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	path, _ := policy.DefaultPath()
	got, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	def := policy.Default()
	if got.Harness["codex"].Path != exe || got.Model.Provider != def.Model.Provider || got.Images.Gateway != def.Images.Gateway {
		t.Fatalf("policy = %+v", got)
	}
}

func TestChooseHarnessRefusesWhatIsNotHere(t *testing.T) {
	isolateRunEnv(t)
	t.Setenv("PATH", t.TempDir())
	err := ChooseHarness(context.Background(), "claude")
	if err == nil || !strings.Contains(err.Error(), "not installed") || !strings.Contains(err.Error(), "https://github.com/anthropics/claude-code") {
		t.Fatalf("err = %v", err)
	}
	if err := ChooseHarness(context.Background(), "docker-agent"); err == nil || !strings.Contains(err.Error(), "containerised") {
		t.Fatalf("docker-agent: %v", err)
	}
	if err := ChooseHarness(context.Background(), "no-such"); err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("unknown: %v", err)
	}
	path, _ := policy.DefaultPath()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("a refused choice created %s", path)
	}
}

func TestChosenHarnessesReadsWithoutExecuting(t *testing.T) {
	isolateRunEnv(t)
	// A "claude" on PATH that would fail loudly if run: the recap's read
	// must never touch it.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\ntouch \"$0.ran\"\nexit 9\n"), 0o755); err != nil { //nolint:gosec // a test fixture
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	names, err := ChosenHarnesses()
	if err != nil || len(names) != 0 {
		t.Fatalf("no policy: names=%v err=%v", names, err)
	}
	path, _ := policy.DefaultPath()
	pol := policy.Default()
	pol.Harness = map[string]policy.HarnessPolicy{"codex": {Path: "/opt/codex"}, "claude": {}}
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	names, err = ChosenHarnesses()
	if err != nil || len(names) != 2 || names[0].Name != "claude" || names[0].DisplayName != "Claude Code" || names[1].Name != "codex" || names[1].DisplayName != "Codex" || !names[1].Chosen {
		t.Fatalf("names=%+v err=%v", names, err)
	}
	if _, statErr := os.Stat(filepath.Join(bin, "claude.ran")); !os.IsNotExist(statErr) {
		t.Fatal("ChosenHarnesses executed the harness")
	}
}

func TestHarnessStatusSurfacesABrokenPolicy(t *testing.T) {
	isolateRunEnv(t)
	path, _ := policy.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("policyVersion: \"1\"\nnotAField: true\n"), 0o644); err != nil { //nolint:gosec // a test fixture
		t.Fatal(err)
	}
	if _, err := HarnessStatus(context.Background()); err == nil || !strings.Contains(err.Error(), "operator policy") {
		t.Fatalf("err = %v", err)
	}
}
