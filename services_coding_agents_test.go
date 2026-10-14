package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgarchive "github.com/blaktron/agentry/pkg/archive"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// isolateHomes points every file this surface touches at temp dirs: the
// config dir (settings, installed.yaml), the runner home (the policy, the
// installs) and HOME (~/.claude/skills). Returns HOME and the runner home.
func isolateHomes(t *testing.T) (userHome, runnerHome string) {
	t.Helper()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	userHome, runnerHome = t.TempDir(), t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("AGENTRY_RUNNER_HOME", runnerHome)
	return userHome, runnerHome
}

// installStub is a registry that serves one unsigned release of acme/demo
// — resolve, the archive, no attestation — enough for the engine to
// install from, with the one gate that raises a dialog.
func installStub(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "SKILL.md"), []byte("# Demo\n\nDo the demo thing.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := pkgarchive.WalkWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if _, err := pkgarchive.BuildArchive(ws, files, &archive); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	digest := "sha256:" + hex.EncodeToString(sum[:])
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resolve", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": "/att", "digest": digest,
			"manifest": map[string]any{
				"schemaVersion": "1", "name": "demo", "namespace": "acme", "kind": "skill", "version": "1.0.0", "summary": "Test agent",
				"entrypoints": map[string]any{"skill": "SKILL.md"}, "publish": map[string]any{"include": []string{"SKILL.md"}},
			},
			"namespace": "acme", "name": "demo", "version": "1.0.0", "yanked": false, "scanStatus": "pipeline",
		})
	})
	mux.HandleFunc("/archive.tar.zst", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive.Bytes()) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func chooseInPolicy(t *testing.T, runnerHome string, names ...string) {
	t.Helper()
	pol := policy.Default()
	pol.Harness = map[string]policy.HarnessPolicy{}
	for _, n := range names {
		pol.Harness[n] = policy.HarnessPolicy{}
	}
	if err := policy.Save(filepath.Join(runnerHome, "policy.yaml"), pol); err != nil {
		t.Fatal(err)
	}
}

// TestInstallRunnerAlsoInstallsIntoTheCodingAgent (agentry-desktop#148,
// plan D6): with Settings' "also install into <coding agent>" off, the
// one-click Install writes the runner home and nothing else; with it on,
// it also writes the export the policy implies — the same verified bytes,
// its own record — behind the same dialogs: the provenance consent is
// asked once for the one release, and the contract dialog says the
// coding agent enforces nothing. An export that cannot be written (no
// project directory for AGENTS.md) is said, not a failed install.
func TestInstallRunnerAlsoInstallsIntoTheCodingAgent(t *testing.T) {
	userHome, runnerHome := isolateHomes(t)
	reg := installStub(t)
	var asked []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		asked = append(asked, title+"|"+message)
		return true, nil
	}}
	i := &InstallAPI{app: app, engine: install.NewEngine()}
	chooseInPolicy(t, runnerHome, "claude")

	// Off: the runner home alone, one dialog (the unsigned release).
	if _, err := i.InstallRunner(reg, "acme/demo"); err != nil {
		t.Fatal(err)
	}
	if st, _ := installed.Load(); len(st.Records) != 1 || st.Records[0].Target != installdir.Target {
		t.Fatalf("with the switch off: %+v", st.Records)
	}
	if _, err := os.Stat(filepath.Join(userHome, ".claude", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatal("the switch is off and the Claude skill was written")
	}
	if len(asked) != 1 || !strings.HasPrefix(asked[0], "Install anyway?|") {
		t.Fatalf("dialogs with the switch off: %v", asked)
	}

	// On: both, and still one provenance dialog for the one release.
	s, _ := settings.Load()
	s.InstallIntoCodingAgent = true
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	asked = nil
	res, err := i.InstallRunner(reg, "acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(userHome, ".claude", "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("the Claude skill was not written: %v (%s)", err, res.Message)
	}
	st, _ := installed.Load()
	targets := map[string]string{}
	for _, r := range st.Records {
		targets[r.Target] = r.Dir
	}
	if len(st.Records) != 2 || targets[installdir.Target] != installdir.AgentDir(runnerHome, "acme", "demo") || targets["claude-skill"] != filepath.Join(userHome, ".claude", "skills", "demo") {
		t.Fatalf("with the switch on: %+v", st.Records)
	}
	if len(asked) != 1 || !strings.HasPrefix(asked[0], "Install anyway?|") {
		t.Fatalf("dialogs with the switch on: %v", asked)
	}
	if res.Warning != "" || !strings.Contains(res.Message, "Claude skill") {
		t.Fatalf("result: %+v", res)
	}

	// Codex chosen instead, no project directory: the runner home is
	// written, the export is not, and the result says so.
	chooseInPolicy(t, runnerHome, "codex")
	if _, err := install.Uninstall(installed.Key{Registry: reg, Namespace: "acme", Name: "demo", Target: "claude-skill", Dir: filepath.Join(userHome, ".claude", "skills", "demo")}); err != nil {
		t.Fatal(err)
	}
	res, err = i.InstallRunner(reg, "acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if res.Warning == "" || !strings.Contains(res.Warning, "project directory") {
		t.Fatalf("no project directory: %+v", res)
	}
	if st, _ := installed.Load(); len(st.Records) != 1 {
		t.Fatalf("records after the missing directory: %+v", st.Records)
	}

	// With one, AGENTS.md lands there.
	project := t.TempDir()
	s, _ = settings.Load()
	s.CodingAgentProjectDir = project
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, err := i.InstallRunner(reg, "acme/demo"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AGENTS.md", filepath.Join("agents", "demo", "SKILL.md")} {
		if _, err := os.Stat(filepath.Join(project, want)); err != nil {
			t.Fatalf("%s: %v", want, err)
		}
	}
}

// TestContractDialogNamesTheCodingAgent: when the export owes the
// capability consent — a first export of a release that declares egress
// or exec — the one contract dialog says where else the release goes and
// that the coding agent does not enforce the contract; a runner plan that
// owes nothing still raises it for the export's sake.
func TestContractDialogNamesTheCodingAgent(t *testing.T) {
	var asked []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		asked = append(asked, title+"|"+message)
		return true, nil
	}}
	i := &InstallAPI{app: app, engine: install.NewEngine()}
	caps := installed.CapSnapshot{EgressHosts: []string{"api.example.com"}}
	runner := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.1.0", PrevVersion: "1.0.0", Diff: &installed.Diff{}, Capabilities: caps}
	export := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.1.0", Target: "claude-skill", NeedsCapabilityConfirm: true, Capabilities: caps,
		Paths: install.Paths{Dest: "/home/me/.claude/skills/demo"}}
	if err := i.consentForInstall(runner, export); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "api.example.com") || !strings.Contains(asked[0], "Claude skill") || !strings.Contains(asked[0], "/home/me/.claude/skills/demo") || !strings.Contains(asked[0], "does not enforce") {
		t.Fatalf("the contract dialog with an export: %v", asked)
	}
	asked = nil
	if err := i.consentForInstall(runner, nil); err != nil || len(asked) != 0 {
		t.Fatalf("an unwidened update with no export raised %v (%v)", asked, err)
	}

	// A widened runner update beside a first export: the dialog carries the
	// diff and the whole declaration, since the export's record will.
	asked = nil
	widened := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.1.0", PrevVersion: "1.0.0", RequiresWidenedConfirm: true,
		Diff: &installed.Diff{Widened: []string{"network egress: added api.example.com"}}, Capabilities: caps}
	if err := i.consentForInstall(widened, export); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "widens what it may do") || !strings.Contains(asked[0], "Version 1.1.0 declares:") || !strings.Contains(asked[0], "network egress to api.example.com") {
		t.Fatalf("the widened-plus-first-export dialog: %v", asked)
	}
}

// TestCodingAgentsSettingsSurface (agentry-desktop#148): Settings →
// Coding agents shows the chosen coding agents, the export they imply and
// its folder, and the switch, off by default; the switch will not go on
// while AGENTS.md has no project directory; the directory comes from the
// native picker in this process and from nowhere the page could name
// (D-030); un-choosing a coding agent changes the implied export.
func TestCodingAgentsSettingsSurface(t *testing.T) {
	userHome, runnerHome := isolateHomes(t)
	picked := ""
	app := &App{ctx: context.Background(), pickDirectory: func(_ context.Context, _ string) (string, error) { return picked, nil }}
	sAPI := &SettingsAPI{app: app}
	rAPI := &RunAPI{app: app}

	v, err := sAPI.CodingAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Chosen) != 0 || v.Export.Target != "agents-md" || !v.Export.NeedsDir || v.Export.Enabled {
		t.Fatalf("fresh: %+v", v)
	}
	if _, err := sAPI.SetInstallIntoCodingAgent(true); err == nil || !strings.Contains(err.Error(), "project directory") {
		t.Fatalf("the switch went on with no project directory: %v", err)
	}

	// A cancelled picker changes nothing; a picked directory is stored.
	v, err = sAPI.ChooseCodingAgentProjectDir()
	if err != nil || v.Export.Dir != "" {
		t.Fatalf("a cancelled picker: %+v %v", v, err)
	}
	picked = t.TempDir()
	v, err = sAPI.ChooseCodingAgentProjectDir()
	if err != nil || v.Export.Dir != picked || v.Export.NeedsDir {
		t.Fatalf("a picked directory: %+v %v", v, err)
	}
	v, err = sAPI.SetInstallIntoCodingAgent(true)
	if err != nil || !v.Export.Enabled {
		t.Fatalf("the switch: %+v %v", v, err)
	}
	if s, _ := settings.Load(); !s.InstallIntoCodingAgent || s.CodingAgentProjectDir != picked {
		t.Fatalf("settings: %+v", s)
	}

	// Claude Code chosen: the Claude skill in ~/.claude/skills, the project
	// directory kept for when Codex is the one; forgetting Claude Code
	// brings AGENTS.md back.
	chooseInPolicy(t, runnerHome, "claude", "codex")
	v, _ = sAPI.CodingAgents()
	if len(v.Chosen) != 2 || v.Chosen[0].DisplayName != "Claude Code" || v.Export.Target != "claude-skill" || v.Export.Dir != filepath.Join(userHome, ".claude", "skills") {
		t.Fatalf("claude and codex chosen: %+v", v)
	}
	if _, err := rAPI.ForgetHarness("claude"); err != nil {
		t.Fatal(err)
	}
	v, _ = sAPI.CodingAgents()
	if len(v.Chosen) != 1 || v.Chosen[0].Name != "codex" || v.Export.Target != "agents-md" || v.Export.Dir != picked {
		t.Fatalf("after forgetting claude: %+v", v)
	}
}

// TestContractDialogNamesShippedKnowledge (agentry-desktop#277): the
// consent names each knowledge pack the release ships, with its size, and
// an update that ships one more is a widening the dialog carries.
func TestContractDialogNamesShippedKnowledge(t *testing.T) {
	size := int64(216)
	first := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0", NeedsCapabilityConfirm: true,
		Capabilities:     installed.CapSnapshot{EgressHosts: []string{"api.example.com"}, Knowledge: []string{"aws-docs", "notes"}},
		ShippedKnowledge: []install.ShippedPack{{Name: "aws-docs", Bytes: &size}, {Name: "notes"}}}
	if got := contractConsentText(first, nil); !strings.Contains(got, "• knowledge: ships aws-docs (216 bytes), notes") {
		t.Fatalf("first install dialog:\n%s", got)
	}
	prev := installed.Snapshot(manifest.Capabilities{})
	next := installed.Snapshot(manifest.Capabilities{Knowledge: []string{"aws-docs"}})
	d := installed.DiffCapabilities(prev, next)
	update := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.1.0", PrevVersion: "1.0.0", RequiresWidenedConfirm: d.HasWidened(), Diff: &d, Capabilities: next}
	if got := contractConsentText(update, nil); !strings.Contains(got, "widens what it may do") || !strings.Contains(got, "• ships knowledge: aws-docs") {
		t.Fatalf("update dialog:\n%s", got)
	}
}

// The consent dialog names a target host and a browser, either of which
// asks for it alone, in the CLI's words (agentry-desktop#812, #878).
func TestContractDialogNamesATargetHostAndABrowser(t *testing.T) {
	plan := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0", NeedsCapabilityConfirm: true,
		Capabilities: installed.Snapshot(manifest.Capabilities{TargetHosts: []string{"server"}, Browser: &manifest.Browser{Sites: []string{"site"}}})}
	got := contractConsentText(plan, nil)
	for _, want := range []string{
		"• target hosts: server (asks for a server when it runs, and can run any command on it as the account you sign in with)",
		"• browser: a web browser that can open the site you name for site and its subdomains",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dialog:\n%s\nwant %q", got, want)
		}
	}
}
