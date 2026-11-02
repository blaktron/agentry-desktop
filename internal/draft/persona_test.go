package draft

// A persistent agent's draft carries its persona text beside its manifest:
// a hosted draft.json as the field personaText, a draft made on this
// computer as the report PERSONA.md, which draft.json then records too.

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const persistentHosted = `{"schemaVersion": "2", "name": "notes-keeper", "namespace": "draft", "version": "0.1.0",
  "summary": "Keeps the notes you tell it.", "license": "MIT", "modes": ["persistent"], "entrypoints": {"instruction": "AGENT.md"},
  "persona": {"name": "Penny"},
  "capabilities": {"network": {"egress": "none"}, "filesystem": {"read": "none", "write": "none"}, "exec": "none", "persistent": {"memory": true}},
  "dependencies": {"agents": [{"ref": "acme/pdf-summary", "version": "^1.2.0", "why": "Summarises a PDF."}]},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE", "PERSONA.md"]}}`

func persistentDoc(t *testing.T, persona any) []byte {
	return draftDoc(t, func(d map[string]any) {
		d["manifest"] = json.RawMessage(persistentHosted)
		if persona != nil {
			d["personaText"] = persona
		}
	})
}

func TestDecodeReadsAPersistentDraftWithItsPersona(t *testing.T) {
	_, m, notes, err := Decode(persistentDoc(t, "You are Penny.\n"), "draft")
	if err != nil {
		t.Fatal(err)
	}
	if !m.PersistentOnly() || m.Persona == nil || m.Persona.Name != "Penny" || m.PersonaText != "You are Penny.\n" {
		t.Fatalf("model %+v", m)
	}
	if m.Persistent == nil || !m.Persistent.Memory || len(m.Dependencies) != 1 || m.Dependencies[0].Why != "Summarises a PDF." {
		t.Fatalf("persistent %+v subagents %+v", m.Persistent, m.Dependencies)
	}
	if len(notes) != 0 {
		t.Fatalf("notes %q: the persona text and the persistent fields are all read", notes)
	}
}

func TestDecodeRefusesAPersonaTextThatIsNotBoundedText(t *testing.T) {
	for name, persona := range map[string]any{
		"not a string": []any{"x"},
		"over bound":   strings.Repeat("a", 33<<10),
	} {
		if _, _, _, err := Decode(persistentDoc(t, persona), "draft"); err == nil {
			t.Errorf("%s: the draft was opened", name)
		}
	}
}

func TestDecodeLeavesPersonaTextWithNoPersonaOut(t *testing.T) {
	raw := draftDoc(t, func(d map[string]any) { d["personaText"] = "You are someone." })
	_, m, notes, err := Decode(raw, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.PersonaText != "" || !slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "persona") }) {
		t.Fatalf("persona %q notes %q", m.PersonaText, notes)
	}
}
