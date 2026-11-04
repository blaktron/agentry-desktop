package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"

	"github.com/blaktron/agentry-desktop/internal/install"
)

// persistentStub is a registry that serves one unsigned persistent release,
// acme/helper@1.0.0, with a persona, the clock and a subagent,
// acme/summarize, whose newest version within ^1.0.0 runs autonomously and
// reaches one host.
func persistentStub(t *testing.T) string {
	t.Helper()
	files := map[string]string{
		"agent.yaml": "schemaVersion: \"2\"\nname: helper\nnamespace: acme\nmodes: [persistent]\nversion: 1.0.0\nsummary: Keeps notes\nlicense: MIT\n" +
			"persona:\n  name: Pip\ncapabilities:\n  persistent:\n    clock: true\n" +
			"dependencies:\n  agents:\n    - ref: acme/summarize\n      version: ^1.0.0\n      why: Summarises the PDFs you share\n" +
			"publish:\n  include: [agent.yaml, AGENT.md, PERSONA.md, README.md, LICENSE]\n",
		"AGENT.md":   "Keep notes on what the person shares.\n",
		"PERSONA.md": "You are Pip, a careful note keeper.\n",
		"README.md":  "# Helper\n",
		"LICENSE":    "MIT\n",
	}
	ws := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(ws, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manifest.Parse([]byte(files["agent.yaml"]))
	if err != nil {
		t.Fatal(err)
	}
	walked, err := pkgarchive.WalkWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if _, err := pkgarchive.BuildArchive(ws, walked, &archive); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	digest := "sha256:" + hex.EncodeToString(sum[:])
	sub := map[string]any{
		"schemaVersion": "2", "name": "summarize", "namespace": "acme", "version": "1.2.0", "summary": "Summarises",
		"modes": []string{"autonomous"}, "license": "MIT", "publish": map[string]any{"include": []string{"agent.yaml"}},
		"capabilities": map[string]any{"network": map[string]any{"egress": []string{"api.example.com"}}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resolve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Query().Get("ref"), "acme/summarize") {
			_ = json.NewEncoder(w).Encode(map[string]any{"archiveUrl": "/none", "digest": digest, "manifest": sub,
				"namespace": "acme", "name": "summarize", "version": "1.2.0", "yanked": false, "scanStatus": "pipeline"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"archiveUrl": "/archive.tar.zst", "attestationUrl": "/att", "digest": digest,
			"manifest": m, "namespace": "acme", "name": "helper", "version": "1.0.0", "yanked": false, "scanStatus": "pipeline"})
	})
	mux.HandleFunc("/archive.tar.zst", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive.Bytes()) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// Installing a persistent release asks once, in one dialog that names what
// it may do, its persona and its subagent pinned with that version's reach;
// it is then one of the agents, enabled and stopped, its subagent pinned,
// and the release laid down in the runner home with its consent record.
// Uninstalling keeps its data, and removes the release and the record.
func TestAPersistentReleaseInstallsWithOneConsent(t *testing.T) {
	rig := newPersistentRig(t)
	reg := persistentStub(t)
	rig.p.installs = &InstallAPI{app: rig.p.app, engine: install.NewEngine()}

	v, err := rig.p.Install(reg, "acme/helper")
	if err != nil {
		t.Fatal(err)
	}
	asked := rig.dialogs()
	if len(asked) != 1 {
		t.Fatalf("dialogs: %q", asked)
	}
	for _, want := range []string{
		"Install acme/helper@1.0.0 as a persistent agent?",
		"a release with no attestation",
		", from " + reg + ", runs as a persistent agent",
		"keeps its conversation, its files and a share folder",
		"the time on each turn, its own tasks and the heartbeat that fires them",
		"persona Pip",
		"acme/summarize@1.2.0 (^1.0.0), Summarises the PDFs you share. It reaches network egress.",
	} {
		if !strings.Contains(asked[0], want) {
			t.Errorf("the dialog does not say %q:\n%s", want, asked[0])
		}
	}
	if !v.Published || v.Ref != "acme/helper" || v.Version != "1.0.0" || v.Persona != "Pip" || v.State != agentStopped {
		t.Fatalf("the agent: %+v", v)
	}
	if len(v.Subagents) != 1 || v.Subagents[0].Version != "1.2.0" || v.Subagents[0].Reach != "network egress" {
		t.Fatalf("its subagents: %+v", v.Subagents)
	}
	if !rig.p.shows(v.Name) {
		t.Fatalf("%s is not listed among %v", v.Name, rig.p.shownNames())
	}
	key := installdir.ConsentKey(rig.home, "acme", "helper")
	if st, _ := installed.Load(); st.Find(key) == nil {
		t.Fatal("no consent record for the release")
	}

	rig.asked = nil
	if err := rig.p.Uninstall(v.Name, false); err != nil {
		t.Fatal(err)
	}
	if d := rig.dialogs(); len(d) != 1 || !strings.Contains(d[0], "are kept") {
		t.Fatalf("the uninstall dialogs: %q", d)
	}
	if rig.p.shows(v.Name) {
		t.Fatal("still listed after its uninstall")
	}
	if st, _ := installed.Load(); st.Find(key) != nil {
		t.Fatal("its consent record outlived the uninstall")
	}
	if _, err := os.Stat(installdir.AgentDir(rig.home, "acme", "helper")); !os.IsNotExist(err) {
		t.Fatalf("its release folder is still there: %v", err)
	}
	if _, err := os.Stat(persistent.Dir(rig.home, v.Name)); err != nil {
		t.Fatalf("its state folder went with a keep-data uninstall: %v", err)
	}
}

// A No to the one dialog installs nothing.
func TestAPersistentInstallDeclinedInstallsNothing(t *testing.T) {
	rig := newPersistentRig(t)
	reg := persistentStub(t)
	rig.p.installs = &InstallAPI{app: rig.p.app, engine: install.NewEngine()}
	rig.answer = false
	if _, err := rig.p.Install(reg, "acme/helper"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if _, err := persistent.LoadRelease(rig.home, persistent.Key("acme", "helper")); err == nil {
		t.Fatal("a No recorded the install")
	}
	if _, err := os.Stat(installdir.AgentDir(rig.home, "acme", "helper")); !os.IsNotExist(err) {
		t.Fatal("a No laid the release down")
	}
}

// What cannot be a persistent agent is refused before anyone is asked.
func TestPersistentInstallableRefuses(t *testing.T) {
	rig := newPersistentRig(t)
	base := func() *manifest.Manifest {
		return &manifest.Manifest{SchemaVersion: "2", Namespace: "acme", Name: "helper", Modes: []manifest.Mode{manifest.ModePersistent}}
	}
	if err := rig.p.persistentInstallable(rig.home, base()); err != nil {
		t.Fatalf("a plain persistent release: %v", err)
	}
	for name, change := range map[string]func(*manifest.Manifest){
		"not persistent": func(m *manifest.Manifest) { m.Modes = []manifest.Mode{manifest.ModeAutonomous} },
		"a built-in":     func(m *manifest.Manifest) { m.Namespace, m.Name = "agentry", "assistant" },
		"files": func(m *manifest.Manifest) {
			m.Capabilities.Filesystem.Read = manifest.FSAccess{Mode: manifest.FSModeWorkspace}
		},
		"a target host": func(m *manifest.Manifest) { m.Capabilities.TargetHosts = []string{"build"} },
	} {
		m := base()
		change(m)
		if err := rig.p.persistentInstallable(rig.home, m); err == nil {
			t.Errorf("%s is installable", name)
		}
	}
	if len(rig.dialogs()) != 0 {
		t.Fatalf("a refusal asked: %q", rig.dialogs())
	}
}

// A start beside three running agents warns first, and a No starts
// nothing.
func TestAFourthRunningAgentWarnsFirst(t *testing.T) {
	rig := newPersistentRig(t)
	cfg, err := persistent.LoadConfig(rig.home, assistantAgent)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	if err := persistent.SaveConfig(rig.home, assistantAgent, cfg); err != nil {
		t.Fatal(err)
	}
	rig.p.mu.Lock()
	rig.p.sups = map[string]agentSupervisor{"acme.a": newFakeAgent(), "acme.b": newFakeAgent(), coderAgent: newFakeAgent()}
	rig.p.mu.Unlock()
	rig.answer = false
	if _, err := rig.p.Start(assistantAgent); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("the start: %v", err)
	}
	d := rig.dialogs()
	if len(d) == 0 || !strings.HasPrefix(d[0], "Start another persistent agent?\n3 persistent agents are running already.") {
		t.Fatalf("dialogs: %q", d)
	}
	if rig.started != 0 {
		t.Fatal("a No started the agent")
	}
	rig.p.mu.Lock()
	rig.p.sups = nil
	rig.p.mu.Unlock()
}

// An agent's subagent runs are read from every segment's record, a start
// and its end made one run, newest first.
func TestSubagentRunsAreReadFromEverySegment(t *testing.T) {
	home := t.TempDir()
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	write := func(segment string, lines ...map[string]any) {
		dir := filepath.Join(persistent.Dir(home, "acme.helper"), "segments", segment, "receipts")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		for _, l := range lines {
			_ = json.NewEncoder(&b).Encode(l)
		}
		b.WriteString("not json\n")
		if err := os.WriteFile(filepath.Join(dir, "subagents.ndjson"), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("run-1",
		map[string]any{"time": t0, "event": "started", "run": "sa-000000000001", "subagent": "acme/summarize@1.2.0"},
		map[string]any{"time": t0.Add(time.Minute), "event": "ended", "run": "sa-000000000001", "subagent": "acme/summarize@1.2.0", "outcome": "ok", "runId": "r-1", "copied": []string{"subagents/sa-000000000001/summary.md"}})
	write("run-2",
		map[string]any{"time": t0.Add(time.Hour), "event": "started", "run": "sa-000000000002", "subagent": "acme/summarize@1.2.0"})
	runs := subagentRunViews(home, "acme.helper")
	if len(runs) != 2 || runs[0].ID != "sa-000000000002" || runs[0].Outcome != "" {
		t.Fatalf("runs: %+v", runs)
	}
	if r := runs[1]; r.Outcome != "ok" || r.RunID != "r-1" || !r.Ended.Equal(t0.Add(time.Minute)) || len(r.Copied) != 1 {
		t.Fatalf("the ended run: %+v", r)
	}
	if got := subagentRunViews(home, "nobody"); len(got) != 0 {
		t.Fatalf("an agent with no segments: %+v", got)
	}
}

// A plain install of a release installed as a persistent agent is refused:
// it would move the version the agent is pinned to.
func TestAPlainInstallOfAPersistentAgentsReleaseIsRefused(t *testing.T) {
	_, home := isolateHomes(t)
	if err := persistentAgentsRelease("acme/helper"); err != nil {
		t.Fatalf("nothing installed: %v", err)
	}
	if err := os.MkdirAll(persistent.Dir(home, "acme.helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	rec := "ref: acme/helper\nversion: 1.0.0\ndigest: sha256:" + strings.Repeat("0", 64) + "\nsource: registry\ninstalledAt: 2026-11-02T09:00:00Z\n"
	if err := os.WriteFile(filepath.Join(persistent.Dir(home, "acme.helper"), "release.yaml"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := persistentAgentsRelease("acme/helper@1.1.0"); err == nil || !strings.Contains(err.Error(), "installed as a persistent agent") {
		t.Fatalf("a plain install: %v", err)
	}

}

// A contract that declares no filesystem access says nothing about files in
// the consent: "none" is not something the release may do.
func TestTheConsentNamesNoFilesystemDeclaredAsNone(t *testing.T) {
	plan := &install.Plan{Version: "1.0.0", NeedsCapabilityConfirm: true}
	plan.Capabilities.FSRead, plan.Capabilities.FSWrite = "none", "none"
	for _, l := range declaredLines(plan) {
		if strings.Contains(l, "filesystem") {
			t.Fatalf("a filesystem of none is listed: %q", l)
		}
	}
	plan.Capabilities.FSRead = "workspace"
	if got := declaredLines(plan); len(got) != 1 || !strings.HasPrefix(got[0], "filesystem read: workspace") {
		t.Fatalf("a declared read: %q", got)
	}
}
