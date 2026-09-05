package install

// Target-export and engine-flow tests: the agents-md §5.6 row end
// to end against the fake registry, the first-install capability gate, the
// Prepare refusals that never touch the network, and the list surface.

import (
	"bytes"
	"context"
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

// TestDefaultTarget: the default install target follows runtime.interfaces
// (first emittable entry), never the run modes; claude-skill otherwise.
func TestDefaultTarget(t *testing.T) {
	cases := []struct {
		interfaces []string
		want       string
	}{
		{nil, manifest.InterfaceClaudeSkill},
		{[]string{manifest.InterfaceClaudeSkill, manifest.InterfaceAgentsMD}, manifest.InterfaceClaudeSkill},
		{[]string{manifest.InterfaceAgentsMD, manifest.InterfaceClaudeSkill}, manifest.InterfaceAgentsMD},
		{[]string{"mcp", manifest.InterfaceAgentsMD}, manifest.InterfaceAgentsMD}, // a v1 archive: the withdrawn target is skipped
		{[]string{"mcp"}, manifest.InterfaceClaudeSkill},
	}
	for _, tc := range cases {
		m := &manifest.Manifest{}
		m.Runtime.Interfaces = tc.interfaces
		if got := DefaultTarget(m); got != tc.want {
			t.Errorf("DefaultTarget(%v) = %q, want %q", tc.interfaces, got, tc.want)
		}
	}
}

// TestInstallAgentsMDTargetEndToEnd pins the §5.6 agents-md row plus the
// reinstall path: the section is upserted (never duplicated) and an
// unchanged contract re-installs without any consent gate.
func TestInstallAgentsMDTargetEndToEnd(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "2", "name": "demo", "namespace": "acme",
			"modes": []string{"autonomous"}, "version": "1.0.0", "summary": "Prompt agent",
			"runtime": map[string]any{"interfaces": []string{"agents-md", "claude-skill"}},
		},
		archive: buildArchiveFiles(t, map[string]string{"AGENT.md": "Do the agents-md thing.\n"}),
	}
	reg := startRegistry(t, f)
	project := t.TempDir()

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "", project)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != manifest.InterfaceAgentsMD {
		t.Fatalf("default target from interfaces = %q", plan.Target)
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

// TestPrepareRefusesTargetNeedingDir: the mcp target's missing-project
// refusal surfaces from Prepare, before anything is parked.
func TestPrepareRefusesTargetNeedingDir(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{})
	e := NewEngine()
	if _, err := e.Prepare(context.Background(), reg, "acme/demo", "agents-md", ""); err == nil || !strings.Contains(err.Error(), "project directory") {
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
	m := &manifest.Manifest{Runtime: manifest.Runtime{Interfaces: []string{"agents-md"}}}
	if w := undeclaredTargetWarning(m, "agents-md"); w != "" {
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
		{Registry: "r", Namespace: "acme", Name: "b", Target: "agents-md", Dir: "/d2"},
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
