package settings

import (
	"errors"
	"strings"
	"testing"
)

// pinTo makes the machine settings pin reg (or fail with err) for the test.
func pinTo(t *testing.T, reg string, err error) {
	t.Helper()
	was := machineRegistry
	machineRegistry = func() (string, error) { return reg, err }
	t.Cleanup(func() { machineRegistry = was })
}

// A registry the machine settings pin is the default whatever the file
// says, is listed, is the only one admitted, and the only one a sign-in is
// sent to; the others stay in the file.
func TestThePinnedRegistryIsTheOnlyOneUsed(t *testing.T) {
	isolate(t)
	if err := Save(Settings{DefaultRegistry: "https://registry.example.org", Registries: []Registry{{URL: "https://registry.example.org", Token: "agy_other"}}}); err != nil {
		t.Fatal(err)
	}
	pinTo(t, "https://pinned.example.com/", nil)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultRegistry != "https://pinned.example.com" || s.Find("https://pinned.example.com") == nil || s.Find("https://registry.example.org") == nil {
		t.Fatalf("loaded %+v", s)
	}
	if err := s.Admit("https://registry.example.org"); err == nil || !strings.Contains(err.Error(), "pins Agentry to the registry") {
		t.Fatalf("another registry admitted: %v", err)
	}
	if err := s.Admit("https://pinned.example.com"); err != nil {
		t.Fatalf("the pinned registry refused: %v", err)
	}
	if kind, reg := s.SignInFor("https://registry.example.org"); kind != "" || reg != "" {
		t.Fatalf("a sign-in goes to another registry: %q %q", kind, reg)
	}
	if TokenFor("https://registry.example.org") != "" {
		t.Fatal("another registry's token is still sent")
	}
}

// Machine settings that cannot be used leave no registry chosen and admit
// none.
func TestUnusableMachineSettingsChooseNoRegistry(t *testing.T) {
	isolate(t)
	pinTo(t, "", errors.New("the file is writable by its group"))
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultRegistry != "" {
		t.Fatalf("default %q under unusable machine settings", s.DefaultRegistry)
	}
	if err := s.Admit("https://registry.example.org"); err == nil || !strings.Contains(err.Error(), "machine settings cannot be used") {
		t.Fatalf("admitted: %v", err)
	}
}

// With no pin, nothing changes.
func TestNoPinLeavesTheRegistriesAlone(t *testing.T) {
	isolate(t)
	pinTo(t, "", nil)
	if err := Save(Settings{DefaultRegistry: "https://registry.example.org", Registries: []Registry{{URL: "https://registry.example.org"}}}); err != nil {
		t.Fatal(err)
	}
	s, _ := Load()
	if s.DefaultRegistry != "https://registry.example.org" || len(s.Registries) != 1 {
		t.Fatalf("loaded %+v", s)
	}
}
