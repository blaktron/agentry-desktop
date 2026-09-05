package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/installed"
)

func TestUninstallManagedRemovesDirAndDemergesMCP(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	dest := filepath.Join(project, "agents", "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "server.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mcp := `{
  "mcpServers": {
    "demo": {"command": "node", "args": ["agents/demo/server.js"]},
    "other": {"command": "python"}
  }
}
`
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte(mcp), 0o644); err != nil {
		t.Fatal(err)
	}

	key := installed.Key{Registry: "https://reg.example", Namespace: "acme", Name: "demo", Target: "mcp", Dir: dest}
	var st installed.State
	st.Upsert(key, "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}

	msg, err := Uninstall(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("managed dir survived uninstall: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	servers := doc["mcpServers"].(map[string]any)
	if _, ok := servers["demo"]; ok {
		t.Errorf("demo server entry not demerged: %s", raw)
	}
	if _, ok := servers["other"]; !ok {
		t.Errorf("unrelated server entry lost: %s", raw)
	}
	if !strings.Contains(msg, "removed") {
		t.Errorf("message = %q", msg)
	}
	back, _ := installed.Load()
	if len(back.Records) != 0 {
		t.Errorf("record survived uninstall: %+v", back.Records)
	}
}

func TestUninstallUnmanagedLeavesFiles(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	dest := filepath.Join(dir, "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	key := installed.Key{Registry: "https://reg.example", Namespace: "acme", Name: "demo", Target: "claude-skill", Dir: dest}
	var st installed.State
	st.Upsert(key, "1.0.0", installed.CapSnapshot{}, false)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}

	msg, err := Uninstall(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("user-chosen dir was touched: %v", err)
	}
	if !strings.Contains(msg, "left in place") {
		t.Errorf("message should say files were left: %q", msg)
	}
	back, _ := installed.Load()
	if len(back.Records) != 0 {
		t.Errorf("record survived uninstall: %+v", back.Records)
	}
}

func TestRemoveAgentsMDSection(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "AGENTS.md")
	content := `# Agents

Intro.

## acme/demo (v1.0.0)

Demo text.

## other/agent (v2.0.0)

Keep me.
`
	if err := os.WriteFile(doc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeAgentsMDSection(doc, "acme", "demo"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(doc)
	if strings.Contains(string(got), "acme/demo") {
		t.Errorf("section not removed:\n%s", got)
	}
	for _, keep := range []string{"# Agents", "Intro.", "## other/agent (v2.0.0)", "Keep me."} {
		if !strings.Contains(string(got), keep) {
			t.Errorf("lost %q:\n%s", keep, got)
		}
	}
}

// TestUninstallNoRecordRefuses: uninstalling something never recorded is
// an error naming the key, not a silent no-op.
func TestUninstallNoRecordRefuses(t *testing.T) {
	isolate(t)
	_, err := Uninstall(installed.Key{Registry: "https://reg.example", Namespace: "acme", Name: "ghost", Target: "claude-skill", Dir: "/nowhere"})
	if err == nil || !strings.Contains(err.Error(), "no install record") {
		t.Fatalf("err = %v, want the no-record refusal", err)
	}
}

// TestUninstallManagedAgentsMDDemerges: a managed agents-md uninstall
// removes the extraction dir AND drops the AGENTS.md section, leaving the
// rest of the document alone.
func TestUninstallManagedAgentsMDDemerges(t *testing.T) {
	isolate(t)
	project := t.TempDir()
	dest := filepath.Join(project, "agents", "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "# Agents\n\n## acme/demo (v1.0.0)\n\nDemo.\n\n## other/agent (v2.0.0)\n\nKeep.\n"
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	key := installed.Key{Registry: "https://reg.example", Namespace: "acme", Name: "demo", Target: "agents-md", Dir: dest}
	var st installed.State
	st.Upsert(key, "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}

	msg, err := Uninstall(key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "removed the acme/demo section") {
		t.Errorf("message = %q", msg)
	}
	got, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "acme/demo") || !strings.Contains(string(got), "## other/agent (v2.0.0)") {
		t.Errorf("demerge wrong:\n%s", got)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("managed dir survived")
	}
}

// TestDemergeReportsFailureAsNote: a .mcp.json that is not valid JSON is
// reported as a note (the user's project config is never an error to keep).
func TestDemergeReportsFailureAsNote(t *testing.T) {
	project := t.TempDir()
	dest := filepath.Join(project, "agents", "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := demerge(installed.Record{Namespace: "acme", Name: "demo", Target: "mcp", Dir: dest})
	if !strings.Contains(note, "could not remove") {
		t.Errorf("note = %q", note)
	}
	// A target with nothing to demerge reports nothing.
	if note := demerge(installed.Record{Namespace: "acme", Name: "demo", Target: "claude-skill", Dir: dest}); note != "" {
		t.Errorf("claude-skill demerge note = %q", note)
	}
}

// TestRemoveMCPEntryEdgeCases pins the tolerant edges: a missing file, a
// document without the section, and an absent name are all no-ops, and
// invalid JSON refuses without touching the file.
func TestRemoveMCPEntryEdgeCases(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.json")
	if err := removeMCPEntry(missing, "demo"); err != nil {
		t.Errorf("missing file must be a no-op: %v", err)
	}

	noServers := filepath.Join(dir, "noservers.json")
	if err := os.WriteFile(noServers, []byte(`{"other":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeMCPEntry(noServers, "demo"); err != nil {
		t.Errorf("no mcpServers key must be a no-op: %v", err)
	}
	if raw, _ := os.ReadFile(noServers); string(raw) != `{"other":1}` {
		t.Errorf("no-op rewrote the file: %s", raw)
	}

	absent := filepath.Join(dir, "absententry.json")
	if err := os.WriteFile(absent, []byte(`{"mcpServers":{"other":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeMCPEntry(absent, "demo"); err != nil {
		t.Errorf("absent entry must be a no-op: %v", err)
	}

	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeMCPEntry(invalid, "demo"); err == nil || !strings.Contains(err.Error(), "left untouched") {
		t.Errorf("err = %v, want the refuse-to-touch error", err)
	}
	if raw, _ := os.ReadFile(invalid); string(raw) != "{broken" {
		t.Errorf("invalid JSON was rewritten: %s", raw)
	}
}

func TestResolvePathsRules(t *testing.T) {
	// claude-skill: default is managed, picked dir is not.
	def, err := ResolvePaths("claude-skill", "", "demo")
	if err != nil || !def.Managed || !strings.Contains(def.Dest, filepath.Join(".claude", "skills", "demo")) {
		t.Errorf("default skill paths wrong: %+v err=%v", def, err)
	}
	picked, err := ResolvePaths("claude-skill", "/tmp/x", "demo")
	if err != nil || picked.Managed || picked.Dest != filepath.Join("/tmp/x", "demo") {
		t.Errorf("picked skill paths wrong: %+v err=%v", picked, err)
	}
	// agents-md refuses to guess a project directory.
	if _, err := ResolvePaths("agents-md", "", "demo"); err == nil {
		t.Error("agents-md without a directory must refuse")
	}
	a, err := ResolvePaths("agents-md", "/proj", "demo")
	if err != nil || a.AgentsMD != filepath.Join("/proj", "AGENTS.md") || !a.Managed {
		t.Errorf("agents-md paths wrong: %+v err=%v", a, err)
	}
	// The mcp target left with the withdrawn kind (DEV-088).
	for _, bad := range []string{"mcp", "nope"} {
		if _, err := ResolvePaths(bad, "/proj", "demo"); err == nil {
			t.Errorf("target %q resolved", bad)
		}
	}
}
