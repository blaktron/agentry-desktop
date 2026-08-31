package install

// Target-export and engine-flow tests: the mcp and agents-md §5.6 rows end
// to end against the fake registry, the first-install capability gate, the
// Prepare refusals that never touch the network, and the list surface.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
)

// buildArchiveFiles produces a real release archive holding the given
// files (path → content), via the same builder every publish uses.
func buildArchiveFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	ws := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(ws, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list, err := pkgarchive.WalkWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := pkgarchive.BuildArchive(ws, list, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDefaultTarget(t *testing.T) {
	cases := map[manifest.Kind]string{
		manifest.KindSkill:       manifest.InterfaceClaudeSkill,
		manifest.KindMCPServer:   manifest.InterfaceMCP,
		manifest.KindPromptAgent: manifest.InterfaceAgentsMD,
		manifest.Kind("future"):  manifest.InterfaceClaudeSkill,
	}
	for kind, want := range cases {
		if got := DefaultTarget(kind); got != want {
			t.Errorf("DefaultTarget(%s) = %q, want %q", kind, got, want)
		}
	}
}

// TestInstallMCPTargetEndToEnd pins the §5.6 mcp row: default target from
// the kind, extraction under <project>/agents/<name>, a server entry
// merged into .mcp.json with args re-rooted at the extraction path, and
// empty env placeholders for the declared secret names.
func TestInstallMCPTargetEndToEnd(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "mcp-server", "version": "1.0.0", "summary": "Test MCP server",
			"entrypoints": map[string]any{"mcp": map[string]any{
				"command": "node", "args": []string{"server.js"}, "transport": "stdio",
			}},
			"capabilities": map[string]any{"secrets": []string{"DEMO_TOKEN"}},
			"runtime":      map[string]any{"interfaces": []string{"mcp"}},
		},
		archive: buildArchiveFiles(t, map[string]string{"server.js": "// server\n"}),
	}
	reg := startRegistry(t, f)
	project := t.TempDir()
	// Pre-existing config: the merge must preserve unrelated servers.
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"),
		[]byte(`{"mcpServers":{"other":{"command":"python"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine()
	// Empty target: the manifest kind picks mcp (§5.6 defaults).
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "", project)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != manifest.InterfaceMCP || !plan.Paths.Managed {
		t.Fatalf("plan target/paths = %q %+v", plan.Target, plan.Paths)
	}
	if plan.TargetWarning != "" {
		t.Fatalf("declared interface must not warn: %q", plan.TargetWarning)
	}
	if len(plan.Secrets) != 1 || plan.Secrets[0] != "DEMO_TOKEN" {
		t.Fatalf("plan secrets = %v", plan.Secrets)
	}

	res, err := e.Confirm(plan.ID, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project, "agents", "demo", "server.js")); err != nil {
		t.Errorf("extracted server missing: %v", err)
	}
	if !strings.Contains(res.SecretsNote, "DEMO_TOKEN") {
		t.Errorf("secrets note = %q", res.SecretsNote)
	}
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	entry, ok := doc.Servers["demo"]
	if !ok {
		t.Fatalf("no demo entry merged:\n%s", raw)
	}
	if entry.Command != "node" || len(entry.Args) != 1 || entry.Args[0] != "agents/demo/server.js" {
		t.Errorf("entry args not re-rooted: %+v", entry)
	}
	if v, ok := entry.Env["DEMO_TOKEN"]; !ok || v != "" {
		t.Errorf("secret env placeholder wrong: %+v", entry.Env)
	}
	if _, ok := doc.Servers["other"]; !ok {
		t.Errorf("unrelated server lost:\n%s", raw)
	}

	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Records) != 1 || !st.Records[0].Managed || st.Records[0].Target != manifest.InterfaceMCP {
		t.Fatalf("record = %+v", st.Records)
	}
}

// TestInstallAgentsMDTargetEndToEnd pins the §5.6 agents-md row plus the
// reinstall path: the section is upserted (never duplicated) and an
// unchanged contract re-installs without any consent gate.
func TestInstallAgentsMDTargetEndToEnd(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "prompt-agent", "version": "1.0.0", "summary": "Prompt agent",
		},
		archive: buildArchiveFiles(t, map[string]string{"AGENT.md": "Do the prompt-agent thing.\n"}),
	}
	reg := startRegistry(t, f)
	project := t.TempDir()

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "", project)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != manifest.InterfaceAgentsMD {
		t.Fatalf("default target for prompt-agent = %q", plan.Target)
	}
	if _, err := e.Confirm(plan.ID, false, false); err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "## acme/demo (") || !strings.Contains(string(doc), "Prompt agent") {
		t.Fatalf("AGENTS.md section missing:\n%s", doc)
	}
	if _, err := os.Stat(filepath.Join(project, "agents", "demo", "AGENT.md")); err != nil {
		t.Errorf("extracted files missing: %v", err)
	}

	// Reinstall the same version: prev recorded, contract unchanged, no
	// gates, and the section is upserted rather than appended twice.
	plan2, err := e.Prepare(context.Background(), reg, "acme/demo", "", project)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.PrevVersion != "1.0.0" || plan2.Diff == nil || plan2.RequiresWidenedConfirm {
		t.Fatalf("reinstall plan = %+v", plan2)
	}
	if _, err := e.Confirm(plan2.ID, false, false); err != nil {
		t.Fatal(err)
	}
	doc, err = os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(doc), "## acme/demo ("); got != 1 {
		t.Fatalf("section count after reinstall = %d:\n%s", got, doc)
	}
}

// TestInstallMCPRefusesBrokenConfig: an existing .mcp.json that is not
// valid JSON refuses the merge instead of overwriting the user's file.
func TestInstallMCPRefusesBrokenConfig(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "mcp-server", "version": "1.0.0", "summary": "Test MCP server",
			"entrypoints": map[string]any{"mcp": map[string]any{
				"command": "node", "args": []string{"server.js"}, "transport": "stdio",
			}},
		},
		archive: buildArchiveFiles(t, map[string]string{"server.js": "// server\n"}),
	}
	reg := startRegistry(t, f)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "mcp", project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan.ID, false, false); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("err = %v, want the merge refusal", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(project, ".mcp.json")); string(raw) != "{broken" {
		t.Errorf("the user's config was rewritten: %s", raw)
	}
}

// TestInstallMCPNeedsEntrypoint: exporting to mcp without a declared mcp
// entrypoint is a refusal, not an empty server entry.
func TestInstallMCPNeedsEntrypoint(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "mcp-server", "version": "1.0.0", "summary": "No entrypoint",
		},
		archive: buildArchiveFiles(t, map[string]string{"server.js": "// server\n"}),
	}
	reg := startRegistry(t, f)
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "mcp", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan.ID, false, false); err == nil || !strings.Contains(err.Error(), "no mcp entrypoint") {
		t.Fatalf("err = %v, want the entrypoint refusal", err)
	}
}

// TestPrepareRefusesTargetNeedingDir: the mcp target's missing-project
// refusal surfaces from Prepare, before anything is parked.
func TestPrepareRefusesTargetNeedingDir(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{})
	e := NewEngine()
	if _, err := e.Prepare(context.Background(), reg, "acme/demo", "mcp", ""); err == nil || !strings.Contains(err.Error(), "project directory") {
		t.Fatalf("err = %v, want the pick-a-project refusal", err)
	}
}

// TestFirstInstallCapabilityGate pins §11: the FIRST install of a contract
// declaring exec (or egress) requires explicit capability acceptance.
func TestFirstInstallCapabilityGate(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "skill", "version": "1.0.0", "summary": "Test agent",
			"capabilities": map[string]any{"exec": map[string]any{"commands": []string{"jq"}}},
		},
	}
	reg := startRegistry(t, f)

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsCapabilityConfirm {
		t.Fatalf("first install with exec must gate: %+v", plan)
	}
	if _, err := e.Confirm(plan.ID, false, false); err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("err = %v, want the capability gate", err)
	}
	if _, err := e.Confirm(plan.ID, false, true); err != nil {
		t.Fatalf("acceptance must unlock the install: %v", err)
	}
}

// TestAttestationURLFallback: a resolve result with no attestationUrl
// falls back to the canonical path (here 404 ⇒ unsigned, gated as usual).
func TestAttestationURLFallback(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{omitAttURL: true})
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unsigned {
		t.Fatalf("canonical-path 404 must mean unsigned: %+v", plan.Attestation)
	}
}

// TestPrepareRefusalsBeforeNetwork: a bad ref or unknown target refuses
// before any request is made (the registry here is unroutable).
func TestPrepareRefusalsBeforeNetwork(t *testing.T) {
	isolate(t)
	e := NewEngine()
	if _, err := e.Prepare(context.Background(), "http://127.0.0.1:1", "not a ref", "claude-skill", ""); err == nil {
		t.Fatal("invalid ref must refuse")
	}
	_, err := e.Prepare(context.Background(), "http://127.0.0.1:1", "acme/demo", "bogus", "")
	if err == nil || !strings.Contains(err.Error(), `unknown target "bogus"`) {
		t.Fatalf("err = %v, want the unknown-target refusal", err)
	}
}

// TestConfirmUnknownAndCancel: Confirm on an unknown or cancelled plan id
// refuses, and a confirmed plan cannot be confirmed twice.
func TestConfirmUnknownAndCancel(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{})
	e := NewEngine()

	if _, err := e.Confirm("nope", true, true); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not-found", err)
	}

	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.Cancel(plan.ID)
	if _, err := e.Confirm(plan.ID, true, true); err == nil {
		t.Fatal("a cancelled plan must not confirm")
	}

	plan2, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan2.ID, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan2.ID, false, false); err == nil {
		t.Fatal("a plan must be single-use")
	}
}

// TestConfirmMissingSkillEntrypoint: an archive without its declared skill
// file fails the export, and the failure is not recorded as an install.
func TestConfirmMissingSkillEntrypoint(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{archive: buildArchiveFiles(t, map[string]string{"README.md": "no skill here\n"})}
	reg := startRegistry(t, f)
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan.ID, false, false); err == nil || !strings.Contains(err.Error(), "skill entrypoint") {
		t.Fatalf("err = %v, want the entrypoint refusal", err)
	}
	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Records) != 0 {
		t.Fatalf("failed install must not be recorded: %+v", st.Records)
	}
}

// TestUndeclaredTargetWarning pins §5.2: interfaces, when declared, list
// what §5.6 may emit — an off-list target warns, an empty list never does.
func TestUndeclaredTargetWarning(t *testing.T) {
	none := &manifest.Manifest{}
	if w := undeclaredTargetWarning(none, "claude-skill"); w != "" {
		t.Errorf("no interfaces declared must not warn: %q", w)
	}
	m := &manifest.Manifest{Runtime: manifest.Runtime{Interfaces: []string{"mcp"}}}
	if w := undeclaredTargetWarning(m, "mcp"); w != "" {
		t.Errorf("declared target must not warn: %q", w)
	}
	if w := undeclaredTargetWarning(m, "claude-skill"); !strings.Contains(w, "claude-skill") {
		t.Errorf("off-list target must warn naming it: %q", w)
	}
}

// TestListSorted pins the display order: namespace, then name, then dir.
func TestListSorted(t *testing.T) {
	isolate(t)
	var st installed.State
	for _, k := range []installed.Key{
		{Registry: "r", Namespace: "zeta", Name: "a", Target: "claude-skill", Dir: "/d1"},
		{Registry: "r", Namespace: "acme", Name: "b", Target: "claude-skill", Dir: "/d3"},
		{Registry: "r", Namespace: "acme", Name: "b", Target: "mcp", Dir: "/d2"},
		{Registry: "r", Namespace: "acme", Name: "a", Target: "claude-skill", Dir: "/d4"},
	} {
		st.Upsert(k, "1.0.0", installed.CapSnapshot{}, false)
	}
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	recs, err := List()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range recs {
		got = append(got, r.Namespace+"/"+r.Name+":"+r.Dir)
	}
	want := []string{"acme/a:/d4", "acme/b:/d2", "acme/b:/d3", "zeta/a:/d1"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("List order = %v, want %v", got, want)
		}
	}
}

// TestListSurfacesCorruptState: unreadable install state is an error the
// page must show, not an empty list pretending nothing is installed.
func TestListSurfacesCorruptState(t *testing.T) {
	isolate(t)
	path, err := installed.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(":\tnot yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := List(); err == nil {
		t.Fatal("corrupt state must surface as an error")
	}
}
