package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// assignedDoc is org's verified document locking the quickagents row to
// list.
func assignedDoc(t *testing.T, org string, list ...managedsettings.Assignment) managed.State {
	t.Helper()
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	return managed.State{
		Registry: "https://r.example",
		Document: managedsettings.Document{Registry: "https://r.example", Org: org, Version: 2,
			Settings: map[string]managedsettings.Setting{managedsettings.QuickAgents: {Value: b, Locked: true}}},
	}
}

func required(ref string) managedsettings.Assignment {
	return managedsettings.Assignment{Ref: ref, Version: managedsettings.VersionLatest, Mode: managedsettings.AssignRequired}
}

func offered(ref string) managedsettings.Assignment {
	return managedsettings.Assignment{Ref: ref, Version: "1.2.0", Mode: managedsettings.AssignOffered}
}

func isolatedConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	return home
}

// After a real read of the settings, each required QuickAgent is installed
// through the assigned path, with the settings held at that moment; an
// offered one is not; a failure is kept for the Library and stops nothing
// else; a read that fetched nothing installs nothing. The Library is told.
func TestRequiredQuickAgentsAreInstalledAfterARealRead(t *testing.T) {
	isolatedConfig(t)
	events := recordEvents(t)
	doc := assignedDoc(t, "acme", required("acme/one"), offered("acme/menu"), required("acme/two"))
	holdManaged(t, managed.Effective{Documents: []managed.State{doc}})
	var asked []install.Assignment
	s := &SettingsAPI{app: &App{ctx: context.Background()}, installs: &InstallAPI{app: &App{ctx: context.Background()}, engine: install.NewEngine()}}
	s.installOne = func(_ context.Context, a install.Assignment) (*install.Result, error) {
		asked = append(asked, a)
		if a.Ref == "acme/one@latest" || a.Ref == "acme/one" {
			return nil, errors.New("digest mismatch")
		}
		return &install.Result{}, nil
	}

	s.installRequired(context.Background(), &doc)
	if len(asked) != 2 || asked[0].Ref != "acme/one" || asked[1].Ref != "acme/two" {
		t.Fatalf("installed %+v, want the two required ones in order", asked)
	}
	for _, a := range asked {
		if a.Organization != "acme" || a.Registry != "https://r.example" || len(a.Held.Documents) != 1 {
			t.Fatalf("assignment %+v lacks its organization, registry or held settings", a)
		}
	}
	if got := events(); len(got) != 1 || got[0] != eventLibraryChanged {
		t.Fatalf("events %q, want the Library told once", got)
	}
	views := s.ManagedAssignments()
	if len(views) != 3 {
		t.Fatalf("views %+v", views)
	}
	if !strings.Contains(views[0].Problem, "acme/one, required by acme, was not installed: digest mismatch") || views[1].Problem != "" || views[2].Problem != "" {
		t.Fatalf("problems: %q %q %q", views[0].Problem, views[1].Problem, views[2].Problem)
	}
	if views[1].Required || views[1].Mode != "offered" || !views[0].Required {
		t.Fatalf("modes: %+v", views)
	}

	// A later read that installs it clears the problem.
	s.installOne = func(context.Context, install.Assignment) (*install.Result, error) { return nil, install.ErrCurrent }
	if err := s.InstallRequired("acme/one"); err != nil {
		t.Fatal(err)
	}
	if got := s.ManagedAssignments()[0].Problem; got != "" {
		t.Fatalf("problem after a good install: %q", got)
	}
	if err := s.InstallRequired("acme/menu"); err == nil || !strings.Contains(err.Error(), "not required") {
		t.Fatalf("an offered QuickAgent was installed as required: %v", err)
	}
}

// The sync installs only after a read that fetched, and the sign-in's read
// installs too.
func TestSyncInstallsRequiredOnlyAfterAFetch(t *testing.T) {
	s := twoSignInApp(t)
	recordManaged(t)
	doc := assignedDoc(t, "acme", required("acme/one"))
	holdManaged(t, managed.Effective{Documents: []managed.State{doc}})
	calls := 0
	s.installs = &InstallAPI{app: s.app, engine: install.NewEngine()}
	s.installOne = func(context.Context, install.Assignment) (*install.Result, error) {
		calls++
		return &install.Result{}, nil
	}
	if _, err := settings.StoreToken("https://secureagentry.net", "agy_secure"); err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Upsert(settings.Registry{URL: "https://secureagentry.net"})
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	fetched := false
	managedCalls.sync = func(context.Context, *http.Client, string, string, time.Duration) (*managed.State, bool, error) {
		return &doc, fetched, nil
	}
	s.syncManaged(context.Background())
	if calls != 0 {
		t.Fatalf("installed %d times after a read that fetched nothing", calls)
	}
	fetched = true
	s.syncManaged(context.Background())
	if calls != 1 {
		t.Fatalf("installed %d times after a fetch, want 1", calls)
	}
}

// Uninstall is refused, with the settings' sentence, while a held document
// requires the QuickAgent; once none does (an offered one, another
// QuickAgent, or the member left the organization) it goes like any other.
func TestUninstallIsRefusedWhileARequiredQuickAgentIsHeld(t *testing.T) {
	isolatedConfig(t)
	i := &InstallAPI{app: &App{ctx: context.Background()}, engine: install.NewEngine()}
	key := installed.Key{Registry: installdir.ConsentRegistry, Namespace: "acme", Name: "one", Target: installdir.Target, Dir: filepath.Join(t.TempDir(), "acme", "one")}

	holdManaged(t, managed.Effective{Documents: []managed.State{assignedDoc(t, "acme", required("acme/one"), offered("acme/menu"))}})
	_, err := i.Uninstall(key)
	if err == nil || !strings.Contains(err.Error(), "acme/one is required by acme's managed settings (quickagents) and cannot be uninstalled while it is") {
		t.Fatalf("uninstall of a required QuickAgent: %v", err)
	}
	other := key
	other.Name = "menu"
	if _, err := i.Uninstall(other); err == nil || strings.Contains(err.Error(), "required by") {
		t.Fatalf("an offered QuickAgent was refused as required: %v", err)
	}
	// Leaving the organization keeps the install as the person's own: the
	// document is gone, so nothing refuses it any more.
	holdManaged(t, managed.Effective{})
	if _, err := i.Uninstall(key); err == nil || strings.Contains(err.Error(), "required by") {
		t.Fatalf("after leaving the organization: %v", err)
	}
}

// The Library's section lists what the documents give, with what is
// installed here.
func TestManagedAssignmentsListWhatIsInstalled(t *testing.T) {
	home := isolatedConfig(t)
	dir := installdir.AgentDir(home, "acme", "one")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	st := installed.State{}
	st.UpsertAssigned(installed.Key{Registry: installdir.ConsentRegistry, Namespace: "acme", Name: "one", Target: installdir.Target, Dir: dir}, "1.4.0", true, "acme")
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	pinned := offered("acme/menu")
	pinned.Preset = &managedsettings.Preset{TimeoutSeconds: 120, MaxTurns: 8}
	pinned.PresetLocked = true
	holdManaged(t, managed.Effective{Documents: []managed.State{assignedDoc(t, "acme", required("acme/one"), pinned)}})
	got := (&SettingsAPI{}).ManagedAssignments()
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if one := got[0]; !one.Installed || one.InstalledVersion != "1.4.0" || !one.Required || one.Selector != "acme/one" || one.Organization != "acme" {
		t.Fatalf("required: %+v", one)
	}
	if menu := got[1]; menu.Installed || menu.Required || menu.Selector != "acme/menu@1.2.0" || menu.Preset == nil || !menu.Preset.Locked || menu.Preset.TimeoutSeconds != 120 {
		t.Fatalf("offered: %+v", menu)
	}
}

// A preset is a default the member may change, or, locked, fixed: a locked
// preset refuses a different timeout, turns, place or answer, and fills what
// was left unset; an unlocked one only fills.
func TestApplyPreset(t *testing.T) {
	preset := &managedsettings.Preset{TimeoutSeconds: 300, MaxTurns: 12, Where: managedsettings.WhereLocal, Inputs: map[string]string{"region": "eu"}}
	held := func(locked bool) managed.Effective {
		a := offered("acme/menu")
		a.Preset, a.PresetLocked = preset, locked
		return managed.Effective{Documents: []managed.State{assignedDoc(t, "acme", a)}}
	}
	const local = managedsettings.WhereLocal

	// Nothing set: every default fills, the answers copied not shared.
	given := map[string]string{"other": "x"}
	to, mt, in, err := applyPreset(held(false), "acme/menu", local, 0, -1, given)
	if err != nil || to != 300 || mt != 12 || in["region"] == "" || in["other"] != "x" {
		t.Fatalf("defaults: %d %d %v %v", to, mt, in, err)
	}
	if _, ok := given["region"]; ok {
		t.Fatal("the caller's answers were changed")
	}
	// A default the member changed stands.
	to, mt, in, err = applyPreset(held(false), "acme/menu", managedsettings.WhereHosted, 60, 0, map[string]string{"region": "us"})
	if err != nil || to != 60 || mt != 0 || in["region"] != "us" {
		t.Fatalf("a changed default: %d %d %v %v", to, mt, in, err)
	}
	// Locked: the same values pass, a different one is refused with the
	// organization and what it fixes.
	if _, _, _, err := applyPreset(held(true), "acme/menu", local, 300, 12, map[string]string{"region": "eu"}); err != nil {
		t.Fatalf("a locked preset refused its own values: %v", err)
	}
	for name, run := range map[string]func() error{
		"timeout": func() error { _, _, _, e := applyPreset(held(true), "acme/menu", local, 60, 12, nil); return e },
		"turns":   func() error { _, _, _, e := applyPreset(held(true), "acme/menu", local, 300, 0, nil); return e },
		"place": func() error {
			_, _, _, e := applyPreset(held(true), "acme/menu", managedsettings.WhereHosted, 300, 12, nil)
			return e
		},
		"answer": func() error {
			_, _, _, e := applyPreset(held(true), "acme/menu", local, 300, 12, map[string]string{"region": "us"})
			return e
		},
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "conflicts with acme's managed settings") || !strings.Contains(err.Error(), "acme/menu (quickagents)") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A QuickAgent no document lists, or one with no preset, is untouched.
	if to, mt, in, err := applyPreset(held(true), "acme/else", local, 0, -1, nil); err != nil || to != 0 || mt != -1 || in != nil {
		t.Fatalf("another QuickAgent: %d %d %v %v", to, mt, in, err)
	}
	// The place a locked preset names is the only one: a runhost run is
	// refused as the CLI refuses --on for a local preset.
	if _, _, _, err := applyPreset(held(true), "acme/menu", managedsettings.WhereRunhost, 300, 12, nil); err == nil || !strings.Contains(err.Error(), "a runhost") {
		t.Fatalf("runhost under a local preset: %v", err)
	}
}

// The consent is headed by the organization and lists the contract.
func TestRequiredConsentText(t *testing.T) {
	text := requiredConsentText("acme/one", &run.ProfileInfo{RequiredBy: "acme", CapabilityLines: []string{"network.egress: api.example"}})
	for _, want := range []string{"Required by acme", "acme/one declares:", "network.egress: api.example", "Running it accepts this contract."} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

// The run paths hold a run to a locked preset before anything starts: a
// local run, a chat, a runhost run and a run In Agentry each refuse a value
// the preset fixes.
func TestRunPathsRefuseAConflictWithALockedPreset(t *testing.T) {
	a := offered("acme/menu")
	a.Preset = &managedsettings.Preset{TimeoutSeconds: 300, MaxTurns: 12, Where: managedsettings.WhereHosted}
	a.PresetLocked = true
	holdManaged(t, managed.Effective{Documents: []managed.State{assignedDoc(t, "acme", a)}})
	r := &RunAPI{app: &App{ctx: context.Background()}}
	conflict := func(name string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "conflicts with acme's managed settings") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// This computer is not where the preset sends runs, whatever the other
	// values are.
	_, err := r.RunAgent("https://r.example", "acme/menu", "", nil, "", nil, 300, 12, false, nil, false, false, nil, false, nil, nil, nil, "", "")
	conflict("run", err)
	_, err = r.StartSession("https://r.example", "acme/menu", "", nil, "", nil, 12, false, nil, false, false, nil, false, nil, nil, nil, "", "")
	conflict("chat", err)
	_, err = r.RunOnRunhost("rh", "https://r.example", "acme/menu", "", nil, "", 300, 12, false, nil, false, "")
	conflict("runhost", err)
}
