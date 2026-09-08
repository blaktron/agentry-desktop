package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
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
