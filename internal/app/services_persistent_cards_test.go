package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// memSecrets is a keychain in memory.
type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSecrets) Get(account string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[account]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (s *memSecrets) Set(account, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[account] = value
	return nil
}

func (s *memSecrets) Delete(account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, account)
	return nil
}

func rosterEntry(v *PersistentAgentView, name string) (PersistentRosterEntry, bool) {
	for _, e := range v.Roster {
		if e.Name == name {
			return e, true
		}
	}
	return PersistentRosterEntry{}, false
}

// The Coding agent's cards carry Agentry's own glyphs for its offered and
// suggested servers, never a brand's mark, and what an offered server's Set
// up asks for; Main takes no skills and keeps no secrets.
func TestTheCodingAgentsCardsCarryGlyphsAndSetUps(t *testing.T) {
	rig := newPersistentRig(t)
	v, err := rig.p.Get(coderAgent)
	if err != nil {
		t.Fatal(err)
	}
	for name, glyph := range map[string]string{"GitHub": "git", "GitLab": "git", "Library docs (Context7)": "docs"} {
		e, ok := rosterEntry(v, name)
		if !ok || e.IconKey != glyph {
			t.Errorf("%s: %+v (found %v), want glyph %q", name, e, ok, glyph)
		}
	}
	if e, _ := rosterEntry(v, "GitHub"); e.SetUpAccount != "github.com" || e.SetUpSecret != "" {
		t.Errorf("GitHub's set-up: account %q secret %q", e.SetUpAccount, e.SetUpSecret)
	}
	if len(v.Suggested) != 1 || v.Suggested[0].Key != "semgrep" || v.Suggested[0].IconKey != "scan" {
		t.Errorf("suggested: %+v", v.Suggested)
	}
	if !v.KeepsSecrets || !v.TakesSkills {
		t.Errorf("the Coding agent keeps secrets %v, takes skills %v", v.KeepsSecrets, v.TakesSkills)
	}
	m, err := rig.p.Get(mainAgent)
	if err != nil {
		t.Fatal(err)
	}
	if m.KeepsSecrets || m.TakesSkills {
		t.Errorf("Main keeps secrets %v, takes skills %v", m.KeepsSecrets, m.TakesSkills)
	}
}

// A suggested server is added as the person's own in one call, through the
// consent dialog: a No adds nothing; once added it keeps its glyph and is
// no longer suggested.
func TestASuggestedServerIsAddedThroughTheDialog(t *testing.T) {
	rig := newPersistentRig(t)
	rig.answer = false
	if _, err := rig.p.AddSuggested(coderAgent, "semgrep"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if v, _ := rig.p.Get(coderAgent); len(v.Suggested) != 1 {
		t.Fatalf("a refused add changed the suggestions: %+v", v.Suggested)
	}
	rig.answer = true
	v, err := rig.p.AddSuggested(coderAgent, "semgrep")
	if err != nil {
		t.Fatal(err)
	}
	if len(rig.dialogs()) != 2 {
		t.Errorf("dialogs: %q", rig.dialogs())
	}
	var added *PersistentRosterEntry
	for i, e := range v.Roster {
		if e.Kind == "server" && e.GrantedBy == persistent.GrantedByYou {
			added = &v.Roster[i]
		}
	}
	if added == nil || added.IconKey != "scan" || !added.On {
		t.Fatalf("the added server: %+v", added)
	}
	if len(v.Suggested) != 0 {
		t.Errorf("still suggested: %+v", v.Suggested)
	}
	if _, err := rig.p.AddSuggested(coderAgent, "nothing-of-that-key"); err == nil {
		t.Error("an unknown suggestion was added")
	}
}

// writeSkill makes a skill folder with a SKILL.md naming it.
func writeSkill(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: Says hello when asked to.\n---\n\nSay hello.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A skill is added from the folder the native picker returns, checked and
// through the consent dialog; a cancel changes nothing, a No leaves no files,
// and Remove takes the entry and its files away.
func TestASkillIsAddedFromThePickerAndRemoved(t *testing.T) {
	rig := newPersistentRig(t)
	src := writeSkill(t, "say-hello")
	picked := ""
	checked := 0
	rig.p.pickSkillSource = func(from string) (string, error) {
		if from != SkillFromFolder {
			t.Errorf("picker for %q", from)
		}
		return picked, nil
	}
	rig.p.checkSkill = func(context.Context, string) (bool, error) { checked++; return false, nil }
	if _, err := rig.p.AddSkill("assistant", SkillFromFolder); err != nil || checked != 0 {
		t.Fatalf("a cancel: %v, checked %d", err, checked)
	}
	picked = src
	kept := filepath.Join(persistent.AddedSkillsPath(rig.home, "assistant"), "say-hello")
	rig.answer = false
	if _, err := rig.p.AddSkill("assistant", SkillFromFolder); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Fatalf("a refused skill left its files: %v", err)
	}
	rig.answer = true
	v, err := rig.p.AddSkill("assistant", SkillFromFolder)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := rosterEntry(v, "say-hello")
	if !ok || e.Kind != "skill" || e.GrantedBy != persistent.GrantedByYou || !e.On || checked != 2 {
		t.Fatalf("the added skill: %+v (found %v), checked %d", e, ok, checked)
	}
	if _, err := rig.p.AddSkill("assistant", "url"); err == nil {
		t.Error("a skill from neither a folder nor a .zip")
	}
	v, err = rig.p.RemoveSkill("assistant", "say-hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rosterEntry(v, "say-hello"); ok {
		t.Error("the skill is still on the roster")
	}
	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Errorf("the removed skill's files: %v", err)
	}
}

// A secret's value goes into the keychain and never comes back to the page;
// a refused change puts the keychain back as it was, the first value deleted
// and a value set again restored; Remove takes it out of the keychain too.
func TestASecretIsKeptInTheKeychainAndNeverReadBack(t *testing.T) {
	rig := newPersistentRig(t)
	keys := &memSecrets{}
	rig.p.secrets = keys
	account := persistent.SecretAccount("assistant", "WEATHER_KEY")
	if _, err := rig.p.SetSecret(mainAgent, "WEATHER_KEY", "planted-value-1"); err == nil {
		t.Fatal("Main kept a secret")
	}
	rig.answer = false
	if _, err := rig.p.SetSecret("assistant", "WEATHER_KEY", "planted-value-1"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if _, err := keys.Get(account); err == nil {
		t.Fatal("a refused first set left its value in the keychain")
	}
	rig.answer = true
	v, err := rig.p.SetSecret("assistant", "WEATHER_KEY", "planted-value-1")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := keys.Get(account); got != "planted-value-1" {
		t.Fatalf("the keychain holds %q", got)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "planted-value") {
		t.Fatal("the view carries the secret's value")
	}
	if e, ok := rosterEntry(v, "Your secret WEATHER_KEY"); !ok || e.Kind != "secret" || !e.On {
		t.Fatalf("the secret's entry: %+v (found %v)", e, ok)
	}
	// Set again: the native dialog asks, since the change widens nothing,
	// and a No leaves the keychain as it was.
	asked := len(rig.dialogs())
	rig.answer = false
	if _, err := rig.p.SetSecret("assistant", "WEATHER_KEY", "planted-value-2"); !errors.Is(err, errNotConfirmed) || len(rig.dialogs()) != asked+1 {
		t.Fatalf("a value set again: %v, dialogs %d then %d", err, asked, len(rig.dialogs()))
	}
	if got, _ := keys.Get(account); got != "planted-value-1" {
		t.Fatalf("a refused value set again changed the keychain: %q", got)
	}
	rig.answer = true
	if _, err := rig.p.SetSecret("assistant", "WEATHER_KEY", "planted-value-2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := keys.Get(account); got != "planted-value-2" {
		t.Fatalf("the value set again: %q", got)
	}
	if _, err := rig.p.SetSecret("assistant", "WEATHER_KEY", "short"); err == nil {
		t.Fatal("a value under the bound was stored")
	}
	if got, _ := keys.Get(account); got != "planted-value-2" {
		t.Fatalf("a refused value changed the keychain: %q", got)
	}
	if _, err := rig.p.RemoveSecret("assistant", "WEATHER_KEY"); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Get(account); err == nil {
		t.Fatal("the removed secret is still in the keychain")
	}
}

// A server the person added gets the icon its registry entry names, looked
// up once in the background and kept; the next view draws it as a data:
// URL. A built-in's server is never looked up.
func TestAnAddedServersIconIsLookedUpOnceAndKept(t *testing.T) {
	rig := newPersistentRig(t)
	var mu sync.Mutex
	var looked []string
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	old := findIcon
	findIcon = func(_ context.Context, srv manifest.MCPServer) ([]byte, string, error) {
		mu.Lock()
		defer mu.Unlock()
		looked = append(looked, srv.SourceID())
		return png, "image/png", nil
	}
	t.Cleanup(func() { findIcon = old })
	if _, err := rig.p.AddSuggested(coderAgent, "semgrep"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		v, err := rig.p.Get(coderAgent)
		if err != nil {
			t.Fatal(err)
		}
		var icon string
		for _, e := range v.Roster {
			if e.Kind == "server" && e.GrantedBy == persistent.GrantedByYou {
				icon = e.Icon
			}
		}
		if strings.HasPrefix(icon, "data:image/png;base64,") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no icon kept; looked up %v", looked)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, _ = rig.p.Get(coderAgent)
	mu.Lock()
	defer mu.Unlock()
	if len(looked) != 1 || looked[0] != "pypi:semgrep" {
		t.Errorf("looked up %v, want pypi:semgrep once", looked)
	}
}
