package build

import (
	"slices"
	"strings"
	"testing"
)

// A release as the registry serves its manifest: every field the site's
// Builder cannot hold set, and one the schema does not know.
const releaseManifest = `{"schemaVersion": "2", "name": "meeting-actions", "namespace": "someone-else", "version": "1.4.2",
  "summary": "Turns meeting notes into decisions and action items.", "license": "Apache-2.0",
  "homepage": "https://example.test", "authors": [{"name": "Ada", "email": "ada@example.test"}],
  "modes": ["autonomous", "interactive"], "tags": ["meetings"],
  "entrypoints": {"instruction": "PROMPT.md"},
  "interactive": {"opening": "Paste the notes.", "idleTimeoutSeconds": 900},
  "intake": [{"id": "team", "prompt": "Which team?", "type": "choice", "required": true, "choices": ["Sales", "Support"]}],
  "runtime": {"models": ["claude-opus"], "interfaces": ["agents-md"]},
  "capabilities": {"network": {"egress": ["slack.com"]}, "filesystem": {"read": "workspace", "write": "none"}, "exec": "none",
    "secrets": ["SLACK_TOKEN"]},
  "knowledge": {"accepts": {"prompt": "Your team's glossary.", "required": false}, "ships": [{"name": "glossary"}]},
  "dependencies": {"agents": [{"ref": "acme/helper", "version": "^1.0.0"}]},
  "publish": {"include": ["agent.yaml", "PROMPT.md", "README.md", "LICENSE", "extra/**"], "channel": "beta"},
  "x-unknown": {"anything": true}}`

func TestFromReleaseFixesTheNameSetsTheVersionAndDropsWhatTheBuilderCannotHold(t *testing.T) {
	m, err := FromRelease([]byte(releaseManifest), "# Meeting actions\n\nRead the notes.\n", "chris", "meeting-actions", "1.5.0")
	if err != nil {
		t.Fatal(err)
	}
	// The QuickAgent's namespace and name, never the manifest's; the
	// version given.
	if m.Namespace != "chris" || m.Name != "meeting-actions" || m.Version != "1.5.0" {
		t.Fatalf("identity %s/%s@%s", m.Namespace, m.Name, m.Version)
	}
	// What the form holds is the release's.
	if m.Summary != "Turns meeting notes into decisions and action items." || m.License != "Apache-2.0" ||
		!slices.Equal(m.Modes, []string{"autonomous", "interactive"}) || !slices.Equal(m.Tags, []string{"meetings"}) ||
		m.Opening != "Paste the notes." || len(m.Intake) != 1 || m.Intake[0].ID != "team" ||
		m.EgressMode != "hosts" || !slices.Equal(m.EgressHosts, []string{"slack.com"}) || !slices.Equal(m.Secrets, []string{"SLACK_TOKEN"}) ||
		m.KnowledgePrompt != "Your team's glossary." || m.Instructions != "# Meeting actions\n\nRead the notes.\n" {
		t.Fatalf("held fields: %+v", m)
	}
	// What the site's Builder cannot hold is dropped, and the runtime and
	// the instruction file are the form's own.
	fresh := Default("chris")
	if m.Homepage != "" || len(m.Authors) != 0 || m.IdleTimeoutSeconds != 0 ||
		len(m.Dependencies) != 0 || m.PublishChannel != "" || len(m.PublishInclude) != 0 ||
		!slices.Equal(m.RuntimeModels, fresh.RuntimeModels) || !slices.Equal(m.RuntimeInterfaces, fresh.RuntimeInterfaces) ||
		m.InstructionFile != "AGENT.md" || m.HadComments {
		t.Fatalf("unheld fields kept: %+v", m)
	}
	// The packs it shipped stay ticked by name, for the caller to tie to the
	// person's packs; nothing is laid by the read.
	if !slices.Equal(m.KnowledgeShips, []string{"glossary"}) || len(m.Ships) != 0 {
		t.Fatalf("ships %v from %v", m.KnowledgeShips, m.Ships)
	}
	// The lists the page maps over and that FromRelease resets are never
	// nil.
	if m.Authors == nil || m.Dependencies == nil || m.PublishInclude == nil {
		t.Fatal("a nil list reaches the page")
	}
	// And the form writes it: the manifest it renders parses, with none of
	// the dropped fields in it.
	yaml := m.manifestYAML()
	for _, gone := range []string{"homepage", "authors", "idleTimeoutSeconds", "dependencies", "channel", "x-unknown", "PROMPT.md", "claude-opus"} {
		if strings.Contains(yaml, gone) {
			t.Errorf("the rendered manifest still names %s:\n%s", gone, yaml)
		}
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("the edit does not validate: %v", err)
	}
}

func TestFromReleaseKeepsAnEmptyLicenceAsTheFormsAndRefusesWhatIsNotText(t *testing.T) {
	m, err := FromRelease([]byte(`{"schemaVersion":"2","name":"a","namespace":"b","version":"0.1.0","summary":"s","modes":["autonomous"]}`), "x", "chris", "a", "0.2.0")
	if err != nil || m.License != Default("").License {
		t.Fatalf("licence %q, %v", m.License, err)
	}
	for what, c := range map[string]struct{ manifest, instructions string }{
		"a manifest that is not JSON":    {"name: a", "x"},
		"a manifest that is not a map":   {`["a"]`, "x"},
		"instructions that are not text": {`{"name":"a"}`, "\xff\xfe"},
		"instructions over the limit":    {`{"name":"a"}`, strings.Repeat("x", maxInstructionBytes+1)},
	} {
		if _, err := FromRelease([]byte(c.manifest), c.instructions, "chris", "a", "0.2.0"); err == nil {
			t.Errorf("%s: read", what)
		}
	}
}

func TestReleaseInstructionPath(t *testing.T) {
	for manifest, want := range map[string]string{
		`{"entrypoints":{"instruction":"PROMPT.md"}}`:         "PROMPT.md",
		`{"kind":"skill","entrypoints":{"skill":"SKILL.md"}}`: "SKILL.md",
		`{"entrypoints":{}}`:                                  "AGENT.md",
		`not json`:                                            "AGENT.md",
	} {
		if got := ReleaseInstructionPath([]byte(manifest)); got != want {
			t.Errorf("%s: %q, want %q", manifest, got, want)
		}
	}
}

func TestNextMinorIsAboveEveryVersionInAnyState(t *testing.T) {
	for want, versions := range map[string][]string{
		"1.3.0":  {"1.2.3"},
		"2.1.0":  {"1.9.9", "2.0.4", "0.3.0"},
		"0.6.0":  {"0.5.0-rc.1", "0.4.9"},
		"10.1.0": {"9.9.0", "10.0.0"},
		"0.1.0":  {},
		"1.1.0":  {"not-a-version", "1.0.7"},
	} {
		if got := NextMinor(versions); got != want {
			t.Errorf("NextMinor(%v) = %q, want %q", versions, got, want)
		}
	}
}
