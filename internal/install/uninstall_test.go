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
	// mcp and agents-md refuse to guess a project directory.
	if _, err := ResolvePaths("mcp", "", "demo"); err == nil {
		t.Error("mcp without a directory must refuse")
	}
	if _, err := ResolvePaths("agents-md", "", "demo"); err == nil {
		t.Error("agents-md without a directory must refuse")
	}
	m, err := ResolvePaths("mcp", "/proj", "demo")
	if err != nil || m.Dest != filepath.Join("/proj", "agents", "demo") || m.MCPConfig != filepath.Join("/proj", ".mcp.json") || !m.Managed {
		t.Errorf("mcp paths wrong: %+v err=%v", m, err)
	}
	a, err := ResolvePaths("agents-md", "/proj", "demo")
	if err != nil || a.AgentsMD != filepath.Join("/proj", "AGENTS.md") || !a.Managed {
		t.Errorf("agents-md paths wrong: %+v err=%v", a, err)
	}
	if _, err := ResolvePaths("nope", "", "demo"); err == nil {
		t.Error("unknown target resolved")
	}
}
