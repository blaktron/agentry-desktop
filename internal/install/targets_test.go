package install

// Target-export and engine-flow tests: the agents-md export end to end
// against the fake registry, the first-install capability gate, the
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
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
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

// TestDefaultTarget: the default install target is the runner home
// whatever runtime.interfaces lists.
func TestDefaultTarget(t *testing.T) {
	for _, interfaces := range [][]string{nil, {manifest.InterfaceClaudeSkill}, {manifest.InterfaceAgentsMD, manifest.InterfaceClaudeSkill}} {
		m := &manifest.Manifest{}
		m.Runtime.Interfaces = interfaces
		if got := DefaultTarget(m); got != installdir.Target {
			t.Errorf("DefaultTarget(%v) = %q, want %q", interfaces, got, installdir.Target)
		}
	}
}

// TestInstallAgentsMDTargetEndToEnd pins the agents-md export plus the
// reinstall path: the section is upserted (never duplicated) and an
// unchanged contract re-installs without any consent gate.
func TestInstallAgentsMDTargetEndToEnd(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "2", "name": "demo", "namespace": "acme",
			"modes": []string{"autonomous"}, "version": "1.0.0", "summary": "Prompt agent",
			"runtime":     map[string]any{"interfaces": []string{"agents-md", "claude-skill"}},
			"entrypoints": map[string]any{"instruction": "AGENT.md"}, "publish": map[string]any{"include": []string{"AGENT.md"}},
		},
		archive: buildArchiveFiles(t, map[string]string{"AGENT.md": "Do the agents-md thing.\n"}),
	}
	reg := startRegistry(t, f)
	project := t.TempDir()

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "agents-md", project)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != manifest.InterfaceAgentsMD {
		t.Fatalf("default target from interfaces = %q", plan.Target)
	}
	if _, err := e.Confirm(plan.ID, false, false, false); err != nil {
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
	plan2, err := e.Prepare(context.Background(), reg, "acme/demo", "agents-md", project)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.PrevVersion != "1.0.0" || plan2.Diff == nil || plan2.RequiresWidenedConfirm {
		t.Fatalf("reinstall plan = %+v", plan2)
	}
	if _, err := e.Confirm(plan2.ID, false, false, false); err != nil {
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

// TestFirstInstallCapabilityGate pins that the FIRST install of a contract
// declaring exec (or egress) requires explicit capability acceptance.
func TestFirstInstallCapabilityGate(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "skill", "version": "1.0.0", "summary": "Test agent",
			"entrypoints": map[string]any{"skill": "SKILL.md"}, "publish": map[string]any{"include": []string{"SKILL.md"}},
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
	if _, err := e.Confirm(plan.ID, false, false, false); err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("err = %v, want the capability gate", err)
	}
	if _, err := e.Confirm(plan.ID, false, true, false); err != nil {
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

	if _, err := e.Confirm("nope", true, true, true); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not-found", err)
	}

	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e.Cancel(plan.ID)
	if _, err := e.Confirm(plan.ID, true, true, false); err == nil {
		t.Fatal("a cancelled plan must not confirm")
	}

	plan2, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan2.ID, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan2.ID, false, false, false); err == nil {
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
	if _, err := e.Confirm(plan.ID, false, false, false); err == nil || !strings.Contains(err.Error(), "skill entrypoint") {
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

// TestUndeclaredTargetWarning pins that interfaces, when declared, list what an
// export may emit — an off-list target warns, an empty list never does.
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

// TestListPresentAndManageable pins the two facts the Installed page sorts
// on: a manageable install whose directory exists, a runner consent the CLI
// wrote (present, not manageable), and a record whose directory is gone.
func TestListPresentAndManageable(t *testing.T) {
	isolate(t)
	here := t.TempDir()
	gone := filepath.Join(t.TempDir(), "deleted-project")
	var st installed.State
	st.Upsert(installed.Key{Registry: "r", Namespace: "acme", Name: "skill", Target: "claude-skill", Dir: here}, "1.0.0", installed.CapSnapshot{}, true)
	// A runner record is an install this app manages (a consent the CLI
	// wrote is one too).
	st.Upsert(installed.Key{Registry: "r", Namespace: "acme", Name: "consent", Target: "runner", Dir: here}, "1.0.0", installed.CapSnapshot{}, false)
	st.Upsert(installed.Key{Registry: "r", Namespace: "acme", Name: "vanished", Target: "agents-md", Dir: gone}, "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	items, err := List()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]bool{}
	for _, it := range items {
		got[it.Name] = [2]bool{it.Present, it.Manageable}
	}
	want := map[string][2]bool{"skill": {true, true}, "consent": {true, true}, "vanished": {false, true}}
	for name, w := range want {
		if got[name] != w {
			t.Fatalf("%s: present/manageable = %v, want %v", name, got[name], w)
		}
	}

	// Forget removes only the vanished record; a present one is refused.
	if err := Forget(installed.Key{Registry: "r", Namespace: "acme", Name: "skill", Target: "claude-skill", Dir: here}); err == nil {
		t.Fatal("forgetting a record whose directory exists must be refused")
	}
	if err := Forget(installed.Key{Registry: "r", Namespace: "acme", Name: "vanished", Target: "agents-md", Dir: gone}); err != nil {
		t.Fatal(err)
	}
	back, _ := installed.Load()
	if len(back.Records) != 2 {
		t.Fatalf("forget left %d records, want 2", len(back.Records))
	}
	if _, err := os.Stat(here); err != nil {
		t.Fatalf("forget touched the disk: %v", err)
	}
	if err := Forget(installed.Key{Registry: "r", Namespace: "acme", Name: "vanished", Target: "agents-md", Dir: gone}); err == nil {
		t.Fatal("forgetting a missing record must be an error")
	}
}
