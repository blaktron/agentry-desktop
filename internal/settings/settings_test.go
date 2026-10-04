package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

// isolate points the config dir at a fresh temp dir so a test never reads or
// writes the developer's real ~/.config/agentry/desktop.yaml.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
}

// A missing settings file yields the shippable default: the production
// registry, nothing signed in, and — the wizard gate — not yet onboarded.
func TestLoadDefaultIsNotOnboarded(t *testing.T) {
	isolate(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Onboarded {
		t.Fatal("a fresh install must not report Onboarded")
	}
	if s.DefaultRegistry != DefaultRegistryURL {
		t.Fatalf("default registry = %q, want %q", s.DefaultRegistry, DefaultRegistryURL)
	}
}

// SetOnboarded persists the flag across a reload, and does so even when no
// settings file existed yet (a fresh machine that skips straight through).
func TestSetOnboardedPersists(t *testing.T) {
	isolate(t)
	if err := SetOnboarded(); err != nil {
		t.Fatalf("SetOnboarded: %v", err)
	}
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file not written: %v", err)
	}
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s.Onboarded {
		t.Fatal("Onboarded did not persist across a reload")
	}
}

// SetOnboarded must not disturb the registries or the chosen default: a
// person who signs in first and skips the runtime step keeps their sign-in.
func TestSetOnboardedPreservesRegistries(t *testing.T) {
	isolate(t)
	want := Settings{
		DefaultRegistry: "https://example.test",
		Registries: []Registry{
			{URL: "https://example.test"},
			{URL: "http://agentry:8080"},
		},
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := SetOnboarded(); err != nil {
		t.Fatalf("SetOnboarded: %v", err)
	}
	// Idempotent: a second call is a harmless no-op.
	if err := SetOnboarded(); err != nil {
		t.Fatalf("SetOnboarded (2nd): %v", err)
	}
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s.Onboarded {
		t.Fatal("Onboarded not set")
	}
	if s.DefaultRegistry != want.DefaultRegistry {
		t.Fatalf("default registry changed to %q", s.DefaultRegistry)
	}
	if len(s.Registries) != len(want.Registries) {
		t.Fatalf("registries changed: got %d, want %d", len(s.Registries), len(want.Registries))
	}
}

// A registry over plain http off loopback would carry the keychain PAT in
// clear, and the token is keyed by host: http://secureagentry.net resolves
// the same entry as https://secureagentry.net (security review 2026-09-20
// §8.6; D-030). Loopback keeps http for a dev stack.
func TestValidateURLRefusesPlainHTTPOffLoopback(t *testing.T) {
	for _, u := range []string{"http://secureagentry.net", "http://10.0.0.5:8080", "http://agentry:8080", "ftp://registry.example", "https://"} {
		if err := ValidateURL(u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
	for _, u := range []string{"https://dev.secureagentry.net", "http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		if err := ValidateURL(u); err != nil {
			t.Errorf("%q refused: %v", u, err)
		}
	}
}

// TestCodingAgentExportIsOffByDefault (agentry-desktop#148, plan D6): a
// fresh settings file has "also install into <coding agent>" off and no
// project directory for AGENTS.md; both round-trip through Save.
func TestCodingAgentExportIsOffByDefault(t *testing.T) {
	isolate(t)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.InstallIntoCodingAgent || s.CodingAgentProjectDir != "" {
		t.Fatalf("defaults: %+v", s)
	}
	s.InstallIntoCodingAgent = true
	s.CodingAgentProjectDir = "/home/me/work/oncall"
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	again, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !again.InstallIntoCodingAgent || again.CodingAgentProjectDir != "/home/me/work/oncall" {
		t.Fatalf("round trip: %+v", again)
	}
}

// Run defaults' Built-in agents switch (agentry-desktop#606, D-062): off,
// every built-in starts from the CLI's LocalBudgets, its timeout floor over
// the policy's; on, every one of them has no timeout, and the turn and cost
// caps are the policy's either way. A settings file that cannot be read
// keeps the floor.
func TestTheBuiltinAgentsSwitchTakesAwayTheTimeoutOfEveryBuiltinAndNothingElse(t *testing.T) {
	isolate(t)
	pol := policy.Budgets{TimeoutSeconds: 900, MaxTurns: 60, MaxCostUSD: 2}
	if len(localrun.Builtins()) != 4 {
		t.Fatalf("%d built-ins: the switch's sentence in Run defaults names four", len(localrun.Builtins()))
	}
	for _, b := range localrun.Builtins() {
		if got, want := BuiltinBudgets(b, pol), b.LocalBudgets(pol); got != want || got.TimeoutSeconds < 1800 {
			t.Errorf("%s with the switch off (no file): %+v, want the floor %+v", b.Ref(), got, want)
		}
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s.BuiltinNoTimeout = true
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got, want := BuiltinBudgets(b, pol), (spec.Budgets{TimeoutSeconds: 0, MaxTurns: 60, MaxCostUSD: 2}); got != want {
			t.Errorf("%s with the switch on: %+v, want %+v", b.Ref(), got, want)
		}
	}
	// Another setting's save keeps it.
	s, _ = Load()
	s.RunWhere = RunWhereAgentry
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	if s, _ = Load(); !s.BuiltinNoTimeout {
		t.Fatal("saving another setting turned the switch off")
	}

	// A file that cannot be parsed keeps the floor.
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("builtinNoTimeout: true\n: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != b.LocalBudgets(pol) {
			t.Errorf("%s with an unreadable desktop.yaml: %+v, want the floor", b.Ref(), got)
		}
	}

	s = Settings{BuiltinNoTimeout: false}
	if err := write(s); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != b.LocalBudgets(pol) {
			t.Errorf("%s with the switch turned off again: %+v", b.Ref(), got)
		}
	}
}
