package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

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
// clear, and the token is keyed by host: http://secureagentry.net resolves the
// same entry as https://secureagentry.net. Loopback keeps http for a dev stack.
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

// TestCodingAgentExportIsOffByDefault: a fresh settings file has "also
// install into <coding agent>" off and no project directory for
// AGENTS.md; both round-trip through Save.
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

// Run defaults' no-limit switch for the built-in agents (on by default): every
// built-in runs with no timeout and to the hidden failsafe of 200 turns, and is
// shown no limit at all; off, every one starts from the CLI's LocalBudgets, its
// timeout floor over the policy's, and the policy's turns. The cost cap is the
// policy's either way, a policy that allows more turns than the failsafe keeps
// them, and a settings file that cannot be read keeps the floor.
func TestTheBuiltinAgentsSwitchIsOnByDefaultAndLiftsTheTimeAndTurnsOfEveryBuiltin(t *testing.T) {
	isolate(t)
	pol := policy.Budgets{TimeoutSeconds: 900, MaxTurns: 60, MaxCostUSD: 2}
	if len(localrun.Builtins()) != 4 {
		t.Fatalf("%d built-ins: the switch's sentence in Run defaults names four", len(localrun.Builtins()))
	}
	noLimit := BuiltinLimits{
		Run:   spec.Budgets{TimeoutSeconds: 0, MaxTurns: BuiltinFailsafeTurns, MaxCostUSD: 2},
		Shown: spec.Budgets{TimeoutSeconds: 0, MaxTurns: 0, MaxCostUSD: 2},
	}
	floor := func(b localrun.Builtin, p policy.Budgets) BuiltinLimits {
		return BuiltinLimits{Run: b.LocalBudgets(p), Shown: b.LocalBudgets(p)}
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != noLimit {
			t.Errorf("%s by default (no file): %+v, want %+v", b.Ref(), got, noLimit)
		}
		if got := BuiltinBudgets(b, policy.Budgets{TimeoutSeconds: 900, MaxTurns: 500}); got.Run.MaxTurns != 500 || got.Shown.MaxTurns != 0 {
			t.Errorf("%s under a policy of 500 turns: %+v, want its 500 run and none shown", b.Ref(), got)
		}
		// A policy of 0 turns, unlimited, still gets the failsafe: no
		// built-in runs here with no stop at all by default.
		if got := BuiltinBudgets(b, policy.Budgets{TimeoutSeconds: 900, MaxTurns: 0}); got.Run.MaxTurns != BuiltinFailsafeTurns || got.Shown.MaxTurns != 0 {
			t.Errorf("%s under a policy of unlimited turns: %+v, want the failsafe run and none shown", b.Ref(), got)
		}
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	off := false
	s.BuiltinNoLimit = &off
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != floor(b, pol) || got.Run.TimeoutSeconds < 1800 || got.Run.MaxTurns != 60 {
			t.Errorf("%s with the switch off: %+v, want the floor %+v", b.Ref(), got, floor(b, pol))
		}
	}
	// Another setting's save keeps it.
	if s, err = Load(); err != nil {
		t.Fatal(err)
	}
	s.RunWhere = RunWhereAgentry
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	if s, err = Load(); err != nil || s.BuiltinNoLimitOn() {
		t.Fatal("saving another setting turned the switch back on")
	}

	// A file that cannot be parsed keeps the floor.
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("builtinNoLimit: true\n: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != floor(b, pol) {
			t.Errorf("%s with an unreadable desktop.yaml: %+v, want the floor", b.Ref(), got)
		}
	}

	// The switch's first key, which defaulted off, is not read: a file
	// that says only it is the default, on.
	if err := os.WriteFile(path, []byte("builtinNoTimeout: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != noLimit {
			t.Errorf("%s with only the old key: %+v, want %+v", b.Ref(), got, noLimit)
		}
	}

	on := true
	if err := write(Settings{BuiltinNoLimit: &on}); err != nil {
		t.Fatal(err)
	}
	for _, b := range localrun.Builtins() {
		if got := BuiltinBudgets(b, pol); got != noLimit {
			t.Errorf("%s with the switch turned on again: %+v", b.Ref(), got)
		}
	}
}

// A token goes to no URL ValidateURL refuses: the keychain keys it by host,
// so an http:// spelling of a signed-in https registry gets nothing, from the
// keychain or the settings file, while http on this machine still does.
func TestTokenForRefusesAURLSettingsWouldRefuse(t *testing.T) {
	for _, c := range []struct {
		self string
		want map[string]string
	}{
		{"https://registry.example", map[string]string{
			"https://registry.example": "agy_keychaintoken0000",
			"http://registry.example":  "",
			"ftp://registry.example":   "",
		}},
		{"http://old.example", map[string]string{"http://old.example": ""}},
		{"http://127.0.0.1:8080", map[string]string{"http://127.0.0.1:8080": "agy_localtoken0000"}},
	} {
		isolate(t)
		keyring.MockInit()
		if err := keyring.Set(KeyringService, "registry.example", "agy_keychaintoken0000"); err != nil {
			t.Fatal(err)
		}
		s, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		tok := map[string]string{"http://old.example": "agy_filetoken0000", "http://127.0.0.1:8080": "agy_localtoken0000"}[c.self]
		s.Upsert(Registry{URL: c.self, Token: tok})
		if err := Save(s); err != nil {
			t.Fatal(err)
		}
		for url, want := range c.want {
			if got := TokenFor(url); got != want {
				t.Errorf("TokenFor(%q) = %q, want %q", url, got, want)
			}
		}
	}
}
