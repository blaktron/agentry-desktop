package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Run defaults (plan settings-cards.md D2, D13; agentry-desktop#347): the
// timeout and max turns in the runner policy the CLI shares, and where runs
// go and the output folder in desktop.yaml, written only by Go. The saved
// output folder is admitted at run time because Go reads it from its own
// settings file, as an output folder only (D-044); no default is a consent.

// runDefaultsApp is an App whose directory picker answers dir, under a
// settings file and a runner home of the test's own.
func runDefaultsApp(t *testing.T, dir string) (*App, *SettingsAPI) {
	t.Helper()
	t.Setenv(configdir.EnvVar, t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	app := &App{
		ctx:           context.Background(),
		confirm:       func(context.Context, string, string) (bool, error) { return false, nil },
		pickDirectory: func(context.Context, string) (string, error) { return dir, nil },
	}
	return app, &SettingsAPI{app: app}
}

func TestTheSavedOutputFolderIsAdmittedAsAnOutputFolderOnly(t *testing.T) {
	saved := t.TempDir()
	app, s := runDefaultsApp(t, saved)
	v, err := s.ChooseDefaultOutputDir()
	if err != nil {
		t.Fatal(err)
	}
	if v.OutputDir != saved {
		t.Fatalf("the saved folder reads back as %q, want %q", v.OutputDir, saved)
	}
	st, _ := settings.Load()
	if st.DefaultOutputDir != saved {
		t.Fatalf("desktop.yaml holds %q, want the picker's %q", st.DefaultOutputDir, saved)
	}

	// Signed in to a registry nobody answers at, so a run In Agentry gets
	// past its sign-in check to the path gate and fails, if it passes the
	// gate, only on the upload.
	const reg = "http://127.0.0.1:1"
	st.Upsert(settings.Registry{URL: reg, Token: "agy_run_defaults_test"})
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}

	r := &RunAPI{app: app, inFlightDir: t.TempDir()}
	refusedAsTyped := func(what string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
			t.Errorf("%s: err = %v, want the picker refusal", what, err)
		}
	}
	passedTheGate := func(what string, err error) {
		t.Helper()
		if err != nil && strings.Contains(err.Error(), "not chosen through the file dialog") {
			t.Errorf("%s: the saved output folder was refused: %v", what, err)
		}
	}

	// As the output folder of a run, a session or a run In Agentry, the
	// saved folder passes the picked-path gate in a session that never
	// picked it.
	passedTheGate("the gates, output", r.runGates("acme/x@1.0.0", nil, saved, false, false, false))
	_, err = r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, saved, nil, 0, 0, false, nil, false, false, nil)
	passedTheGate("StartSession", err)
	_, err = r.StartHostedRun(reg, "acme/x@1.0.0", "go", nil, saved, nil, nil)
	passedTheGate("StartHostedRun", err)
	if err == nil || strings.Contains(err.Error(), "sign in") {
		t.Fatalf("StartHostedRun stopped before the path gate: %v", err)
	}
	_, err = r.AttachHostedRun(reg, saved)
	passedTheGate("AttachHostedRun", err)

	// Never as an input: a file inside it, or the folder itself, is refused
	// unless a picker returned it in this session.
	refusedAsTyped("an input inside the saved folder",
		r.runGates("acme/x@1.0.0", []string{filepath.Join(saved, "notes.txt")}, saved, false, false, false))
	refusedAsTyped("the saved folder as an input",
		r.runGates("acme/x@1.0.0", []string{saved}, "", false, false, false))
	_, err = r.StartHostedRun(reg, "acme/x@1.0.0", "go", []string{filepath.Join(saved, "notes.txt")}, saved, nil, nil)
	refusedAsTyped("an input inside the saved folder, In Agentry", err)

	// A path the page sends as "the default" that is not the saved one is
	// refused: a folder under it, a sibling, a traversal.
	for _, p := range []string{filepath.Join(saved, "sub"), saved + "-sibling", filepath.Join(saved, "..", "elsewhere")} {
		refusedAsTyped("output "+p, r.runGates("acme/x@1.0.0", nil, p, false, false, false))
	}

	// Cleared, the folder is a typed path again.
	if _, err := s.ClearDefaultOutputDir(); err != nil {
		t.Fatal(err)
	}
	refusedAsTyped("the cleared folder", r.runGates("acme/x@1.0.0", nil, saved, false, false, false))
}

func TestChoosingTheOutputFolderDoesNotPickItForInputs(t *testing.T) {
	// The Run defaults picker chooses an output folder: it records nothing in
	// this session's picked set, so it admits no input either.
	saved := t.TempDir()
	app, s := runDefaultsApp(t, saved)
	if _, err := s.ChooseDefaultOutputDir(); err != nil {
		t.Fatal(err)
	}
	if err := app.requirePicked("input", filepath.Join(saved, "a.txt")); err == nil {
		t.Fatal("choosing the default output folder admitted an input inside it")
	}
}

func TestACancelledPickerKeepsTheSavedFolder(t *testing.T) {
	saved := t.TempDir()
	app, s := runDefaultsApp(t, saved)
	if _, err := s.ChooseDefaultOutputDir(); err != nil {
		t.Fatal(err)
	}
	app.pickDirectory = func(context.Context, string) (string, error) { return "", nil }
	v, err := s.ChooseDefaultOutputDir()
	if err != nil {
		t.Fatal(err)
	}
	if v.OutputDir != saved {
		t.Fatalf("a cancelled picker changed the folder to %q", v.OutputDir)
	}
}

func TestNoDefaultIsAConsent(t *testing.T) {
	// With every default saved, a run that needs a consent still asks it in
	// the native dialog, and the dialog's No still refuses.
	saved := t.TempDir()
	app, s := runDefaultsApp(t, saved)
	if _, err := s.ChooseDefaultOutputDir(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRunWhere(settings.RunWhereAgentry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRunBudgets(300, 10); err != nil {
		t.Fatal(err)
	}
	asked := 0
	app.confirm = func(context.Context, string, string) (bool, error) { asked++; return false, nil }
	r := &RunAPI{app: app, inFlightDir: t.TempDir()}
	for name, flags := range map[string][3]bool{
		"unsigned": {true, false, false}, "vulnerable MCP": {false, true, false}, "unscanned": {false, false, true},
	} {
		err := r.runGates("acme/x@1.0.0", nil, saved, flags[0], flags[1], flags[2])
		if err == nil || !strings.Contains(err.Error(), "not confirmed") {
			t.Errorf("%s with every default saved: err = %v, want the dialog's refusal", name, err)
		}
	}
	if asked != 3 {
		t.Fatalf("dialogs raised = %d, want one per consent", asked)
	}
}

func TestWhereToRunIsOneOfTwo(t *testing.T) {
	_, s := runDefaultsApp(t, "")
	v, err := s.RunDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if v.Where != settings.RunWhereHere {
		t.Fatalf("with nothing saved, runs go %q, want this computer", v.Where)
	}
	if v, err = s.SetRunWhere(settings.RunWhereAgentry); err != nil || v.Where != settings.RunWhereAgentry {
		t.Fatalf("SetRunWhere(agentry) = %+v, %v", v, err)
	}
	if st, _ := settings.Load(); st.RunWhere != settings.RunWhereAgentry {
		t.Fatalf("desktop.yaml holds %q", st.RunWhere)
	}
	for _, bad := range []string{"", "cloud", "Agentry", "here;rm"} {
		if _, err := s.SetRunWhere(bad); err == nil {
			t.Errorf("SetRunWhere(%q) was accepted", bad)
		}
	}
	if st, _ := settings.Load(); st.RunWhere != settings.RunWhereAgentry {
		t.Fatalf("a refused value changed desktop.yaml to %q", st.RunWhere)
	}
}

func TestRunDefaultsReadTheBudgetsThePolicyHolds(t *testing.T) {
	_, s := runDefaultsApp(t, "")
	v, err := s.RunDefaults()
	if err != nil {
		t.Fatal(err)
	}
	def := policy.Default().Budgets
	if v.TimeoutSeconds != def.TimeoutSeconds || v.MaxTurns != def.MaxTurns {
		t.Fatalf("with no policy, the defaults read %d s / %d turns, want the policy's %d / %d", v.TimeoutSeconds, v.MaxTurns, def.TimeoutSeconds, def.MaxTurns)
	}
	if v, err = s.SetRunBudgets(1200, 0); err != nil || v.TimeoutSeconds != 1200 || v.MaxTurns != 0 {
		t.Fatalf("SetRunBudgets(1200, 0) = %+v, %v", v, err)
	}
	if _, err := s.SetRunBudgets(0, 5); err == nil {
		t.Fatal("a zero timeout was accepted")
	}
}

func TestEachRunGetsItsOwnFolderInsideTheSavedOne(t *testing.T) {
	// Every run shares the saved folder, so none is handed the folder
	// itself: the agent's output mount is read-write, and one run must not
	// see or change what another left (review of agentry-desktop#347). A
	// folder picked for one run is that run's, as before.
	saved := t.TempDir()
	app, s := runDefaultsApp(t, saved)
	if _, err := s.ChooseDefaultOutputDir(); err != nil {
		t.Fatal(err)
	}
	var got []run.Request
	r := &RunAPI{app: app, inFlightDir: t.TempDir(),
		engineRun: func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
			got = append(got, req)
			return &run.Result{}, nil
		}}
	if _, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, saved, nil, 0, 0, false, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	picked := t.TempDir()
	app.markPicked(picked)
	if _, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, picked, nil, 0, 0, false, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].OutDirIsRoot || got[1].OutDirIsRoot {
		t.Fatalf("OutDirIsRoot = %v, want the saved folder a root and the picked one not", got)
	}
}

func TestARunInAgentryLandsInItsOwnFolderInsideTheSavedOne(t *testing.T) {
	saved := t.TempDir()
	_, s := runDefaultsApp(t, saved)
	if _, err := s.ChooseDefaultOutputDir(); err != nil {
		t.Fatal(err)
	}
	run := &hostedrun.Run{ID: "3f2a9c1d", Ref: "acme/notes@1.0.0"}
	got, err := hostedOutDir(saved, run)
	if err != nil || got != filepath.Join(saved, "acme-notes-3f2a9c1d") {
		t.Fatalf("hostedOutDir(saved) = %q, %v; want acme-notes-3f2a9c1d inside it", got, err)
	}
	picked := t.TempDir()
	if got, err := hostedOutDir(picked, run); err != nil || got != picked {
		t.Fatalf("hostedOutDir(picked) = %q, %v; want the picked folder as it is", got, err)
	}
}
