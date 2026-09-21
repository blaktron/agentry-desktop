package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// The install takes its destination directory and its skill entrypoint
// from the resolve response's manifest — which the archive digest and the
// attestation do not cover — and then os.RemoveAll's the former and
// os.WriteFile's the latter. A registry (or an on-path attacker against an
// http:// one) naming "../../.." and "../../.bashrc" used to get $HOME
// deleted and a shell file overwritten (security review 2026-09-20 §8.1,
// §8.2). Three guards now: the manifest is validated before any path is
// derived from it, the managed destination passes the same refusal
// uninstall applies, and the skill file must resolve under the destination.

func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills", "other-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"canary", ".claude/skills/other-skill/SKILL.md", ".bashrc"} {
		if err := os.WriteFile(filepath.Join(home, f), []byte("keep me\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func assertHomeIntact(t *testing.T, home string) {
	t.Helper()
	for _, f := range []string{"canary", ".claude/skills/other-skill/SKILL.md", ".bashrc"} {
		b, err := os.ReadFile(filepath.Join(home, f))
		if err != nil || string(b) != "keep me\n" {
			t.Fatalf("%s was touched: %v %q", f, err, b)
		}
	}
}

func TestInstallRefusesATraversalNameFromTheResolveResponse(t *testing.T) {
	isolate(t)
	home := fakeHome(t)
	for _, name := range []string{"../../..", "..", "../other-skill", "a/b", ""} {
		reg := startRegistry(t, &fakeRegistry{manifestDoc: map[string]any{
			"schemaVersion": "1", "name": name, "namespace": "acme",
			"kind": "skill", "version": "1.0.0", "summary": "Test agent",
			"entrypoints": map[string]any{"skill": "SKILL.md"}, "publish": map[string]any{"include": []string{"SKILL.md"}},
		}})
		e := NewEngine()
		plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", "")
		if err == nil {
			t.Fatalf("name %q: Prepare accepted it (dest %s)", name, plan.Paths.Dest)
		}
		if !strings.Contains(err.Error(), "manifest") {
			t.Fatalf("name %q: refused for another reason: %v", name, err)
		}
	}
	assertHomeIntact(t, home)
}

func TestInstallRefusesAnEscapingSkillEntrypoint(t *testing.T) {
	isolate(t)
	home := fakeHome(t)
	reg := startRegistry(t, &fakeRegistry{manifestDoc: map[string]any{
		"schemaVersion": "1", "name": "demo", "namespace": "acme",
		"kind": "skill", "version": "1.0.0", "summary": "Test agent",
		"publish":     map[string]any{"include": []string{"SKILL.md"}},
		"entrypoints": map[string]any{"skill": "../../.bashrc"},
	}})
	e := NewEngine()
	if plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", ""); err == nil {
		t.Fatalf("Prepare accepted an escaping entrypoint (dest %s)", plan.Paths.Dest)
	}
	assertHomeIntact(t, home)

	// The export's own check, for a manifest that reached it by another
	// road: the skill file must resolve under the destination.
	dest := filepath.Join(home, ".claude", "skills", "demo")
	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "SKILL.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Name: "demo", Summary: "s"}
	m.Entrypoints.Skill = "../../.bashrc"
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := exportClaudeSkill(staging, Paths{Dest: dest, Managed: true, ManagedRoot: filepath.Dir(dest)}, m); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("exportClaudeSkill wrote outside the destination: %v", err)
	}
	assertHomeIntact(t, home)
}

func TestPrepareDestRefusesWhatUninstallRefuses(t *testing.T) {
	home := fakeHome(t)
	root := filepath.Join(home, ".claude", "skills")
	for _, dest := range []string{home, filepath.Dir(home), filepath.Join(home, ".claude"), root, string(filepath.Separator), "relative/dir", "",
		filepath.Join(root, "..", "..", "canary"), filepath.Join(root, "a", "b")} {
		if err := prepareDest(Paths{Dest: dest, Managed: true, ManagedRoot: root}); err == nil {
			t.Fatalf("prepareDest cleared %q", dest)
		}
	}
	// No managed root recorded: nothing is cleared.
	if err := prepareDest(Paths{Dest: filepath.Join(root, "other-skill"), Managed: true}); err == nil {
		t.Fatal("prepareDest cleared a directory with no managed root")
	}
	assertHomeIntact(t, home)
	// A skill directory under the managed root is cleared, as before.
	mine := filepath.Join(root, "mine")
	if err := os.MkdirAll(mine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareDest(Paths{Dest: mine, Managed: true, ManagedRoot: root}); err != nil {
		t.Fatalf("a managed skill directory must be clearable: %v", err)
	}
	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Fatal("the managed skill directory was not cleared")
	}
}
