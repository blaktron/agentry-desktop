package build

// An agent that stays running, in the form's Model: its mode, the keys of
// capabilities.persistent, the persona and its text, and the subagents it
// declares are read from a draft, written back by a save, listed in the
// summary, and kept from one load to the next.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

const persistentManifest = `{"schemaVersion": "2", "name": "notes-keeper", "namespace": "draft", "version": "0.1.0",
  "summary": "Keeps the notes you tell it and reminds you of tasks.", "license": "MIT",
  "modes": ["persistent"], "entrypoints": {"instruction": "AGENT.md"},
  "persona": {"name": "Penny"},
  "capabilities": {"network": {"egress": "none"}, "filesystem": {"read": "none", "write": "none"}, "exec": "none",
    "persistent": {"clock": true, "memory": true, "mail": ["google"], "imageServers": ["google-workspace"], "secrets": ["PENNY_NOTE_KEY"]}},
  "dependencies": {"agents": [{"ref": "acme/pdf-summary", "version": "^1.2.0", "why": "Summarises a PDF the person drops in its share."}]},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE", "PERSONA.md"]}}`

func persistentModel(t *testing.T) *Model {
	t.Helper()
	m, notes, err := FromDraft([]byte(persistentManifest), "# Notes keeper\n\nKeep the notes.\n", "draft")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Fatalf("a persistent draft's fields are all held by the form, notes: %v", notes)
	}
	more, err := m.SetPersonaText("You are Penny, a calm notes keeper.\n")
	if err != nil || len(more) != 0 {
		t.Fatalf("persona text: %v %v", more, err)
	}
	return m
}

func TestFromDraftHoldsAPersistentAgent(t *testing.T) {
	m := persistentModel(t)
	if !m.IsPersistent() || !m.PersistentOnly() {
		t.Fatalf("modes %v", m.Modes)
	}
	if m.Persona == nil || m.Persona.Name != "Penny" || m.PersonaText == "" {
		t.Fatalf("persona %+v %q", m.Persona, m.PersonaText)
	}
	if p := m.Persistent; p == nil || !p.Clock || !p.Memory || !slices.Equal(p.Mail, []string{"google"}) || !slices.Equal(p.Secrets, []string{"PENNY_NOTE_KEY"}) {
		t.Fatalf("persistent %+v", m.Persistent)
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0].Why != "Summarises a PDF the person drops in its share." {
		t.Fatalf("subagents %+v", m.Dependencies)
	}
}

func TestAPersistentDraftWritesAManifestThatPassesTheRules(t *testing.T) {
	m := persistentModel(t)
	res := m.Check()
	if res.First != nil {
		t.Fatalf("first problem %+v\n%s", res.First, res.YAML)
	}
	parsed, err := manifest.Parse([]byte(res.YAML))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.IsPersistent() || parsed.Persona == nil || parsed.Persona.Name != "Penny" || len(parsed.Subagents()) != 1 {
		t.Fatalf("parsed back %+v", parsed)
	}
	if got := parsed.Capabilities.Persistent.Keys(); !slices.Equal(got, []string{"clock", "memory", "secrets", "mail", "imageServers"}) {
		t.Fatalf("keys %v", got)
	}
	if !strings.Contains(res.YAML, "PERSONA.md") {
		t.Fatalf("the include list leaves the persona out:\n%s", res.YAML)
	}
	if res.Fragments[string(SectionInstructions)] == "" || !strings.Contains(res.Fragments[string(SectionInstructions)], "persona:") {
		t.Fatalf("the instructions step does not show the persona: %v", res.Fragments)
	}
}

func TestTheBuildersIncludeListCarriesThePersonaFile(t *testing.T) {
	m := persistentModel(t)
	m.PublishInclude = []string{"agent.yaml"}
	SetByBuilder(m, "", nil)
	if !slices.Contains(m.defaultInclude(), "PERSONA.md") || len(m.PublishInclude) != 0 {
		t.Fatalf("include %v", m.defaultInclude())
	}
	plain := Default("alice")
	if slices.Contains(plain.defaultInclude(), "PERSONA.md") {
		t.Fatal("an ordinary QuickAgent has no persona file")
	}
}

func TestSaveWritesThePersonaAndLoadReadsItBack(t *testing.T) {
	m := persistentModel(t)
	dir := filepath.Join(t.TempDir(), "keeper")
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "PERSONA.md"))
	if err != nil || string(got) != "You are Penny, a calm notes keeper.\n" {
		t.Fatalf("PERSONA.md %q %v", got, err)
	}
	back, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.PersonaText != m.PersonaText || back.Persona.Name != "Penny" || !back.PersistentOnly() ||
		!slices.Equal(back.Persistent.Keys(), m.Persistent.Keys()) || back.Dependencies[0] != m.Dependencies[0] {
		t.Fatalf("loaded %+v", back)
	}
	if again := back.manifestYAML(); again != m.manifestYAML() {
		t.Fatalf("a load then a save changes the file:\n%s\n---\n%s", m.manifestYAML(), again)
	}
}

func TestAPersonaWithoutTextOrWithAnotherFileIsRefused(t *testing.T) {
	m := persistentModel(t)
	m.PersonaText = "  "
	if res := m.Check(); res.Problems[string(SectionInstructions)] == "" {
		t.Fatalf("a persona with no text passed: %+v", res.Problems)
	}
	if err := Save(t.TempDir(), m); err == nil {
		t.Fatal("saved a persona with no text")
	}
	for _, f := range []string{"AGENT.md", "README.md", "../PERSONA.md", "notes.txt"} {
		m := persistentModel(t)
		m.Persona.File = f
		if err := m.Validate(); err == nil {
			t.Errorf("persona file %q was taken", f)
		}
	}
}

func TestADraftsPersonaFileIsKeptToAFileOfItsOwn(t *testing.T) {
	raw := strings.Replace(persistentManifest, `"persona": {"name": "Penny"}`, `"persona": {"name": "Penny", "file": "../etc/passwd"}`, 1)
	m, notes, err := FromDraft([]byte(raw), "# Notes keeper\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.Persona.File != "" || m.personaFile() != manifest.DefaultPersonaFile {
		t.Fatalf("file %q", m.Persona.File)
	}
	if !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "persona file") }) {
		t.Fatalf("notes %v", notes)
	}
}

func TestPersonaTextIsHeldToItsBounds(t *testing.T) {
	m := persistentModel(t)
	if _, err := m.SetPersonaText(strings.Repeat("a", manifest.MaxPersonaFileBytes+1)); err == nil {
		t.Error("a persona over its bound was taken")
	}
	if _, err := m.SetPersonaText("bad \xff text"); err == nil {
		t.Error("a persona that is not text was taken")
	}
	plain, _, err := FromDraft([]byte(draftManifest), "# x\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	notes, err := plain.SetPersonaText("You are someone.")
	if err != nil || len(notes) != 1 || plain.PersonaText != "" {
		t.Fatalf("text without a persona: %v %v %q", notes, err, plain.PersonaText)
	}
}

func TestTheSummaryListsAPersistentAgentsCapabilitiesPersonaAndSubagents(t *testing.T) {
	s := persistentModel(t).Summarize(nil)
	p := s.Persistent
	if p == nil || !p.Only || p.PersonaName != "Penny" {
		t.Fatalf("persistent %+v", p)
	}
	var keys []string
	for _, c := range p.Capabilities {
		keys = append(keys, c.Key)
		if c.Words == "" {
			t.Errorf("%s has no words", c.Key)
		}
	}
	if !slices.Equal(keys, []string{"clock", "memory", "secrets", "mail", "imageServers"}) {
		t.Fatalf("keys %v", keys)
	}
	for _, c := range p.Capabilities {
		switch c.Key {
		case "secrets":
			if !slices.Equal(c.Names, []string{"PENNY_NOTE_KEY"}) {
				t.Errorf("secrets %v", c.Names)
			}
		case "clock":
			if len(c.Names) != 0 {
				t.Errorf("clock names %v", c.Names)
			}
		}
	}
	// The words are the core's, not a copy.
	for _, want := range manifest.PersistentCapabilities() {
		for _, c := range p.Capabilities {
			if c.Key == want.Key && c.Words != want.Words {
				t.Errorf("%s: %q, the core says %q", c.Key, c.Words, want.Words)
			}
		}
	}
	if len(p.Subagents) != 1 || p.Subagents[0] != (SummarySubagent{Ref: "acme/pdf-summary", Version: "^1.2.0", Why: "Summarises a PDF the person drops in its share."}) {
		t.Fatalf("subagents %+v", p.Subagents)
	}
}

func TestTheSummaryOfAnOrdinaryOrMixedAgentIsNotPersistentOnly(t *testing.T) {
	if s := bare().Summarize(nil); s.Persistent != nil {
		t.Fatalf("an ordinary QuickAgent has no persistent part: %+v", s.Persistent)
	}
	m := persistentModel(t)
	m.Modes = []string{"autonomous", "persistent"}
	s := m.Summarize(nil)
	if s.Persistent == nil || s.Persistent.Only {
		t.Fatalf("a QuickAgent that also runs once is not persistent-only: %+v", s.Persistent)
	}
}

func TestAnUndeclaredPersistentKeyIsNotListed(t *testing.T) {
	m := persistentModel(t)
	m.Persistent = nil
	m.Persona = nil
	m.Dependencies = nil
	p := m.Summarize(nil).Persistent
	if p == nil || len(p.Capabilities) != 0 || p.PersonaName != "" || len(p.Subagents) != 0 {
		t.Fatalf("persistent %+v", p)
	}
}

func TestEditRefusesAnAgentThatStaysRunning(t *testing.T) {
	if _, err := FromRelease([]byte(persistentManifest), "# x\n", "alice", "notes-keeper", "0.2.0"); err == nil {
		t.Fatal("the form took a persistent release, and would publish it without its persona")
	}
}
