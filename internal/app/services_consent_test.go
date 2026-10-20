package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// The renderer is not a principal. A consent the page passes is asked again
// in a native dialog the page cannot answer, and a path the page names is
// refused unless a native picker in this process returned it.

func TestConsentIsANativeDialogNotAPageFlag(t *testing.T) {
	asked := []string{}
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, _ string) (bool, error) {
		asked = append(asked, title)
		return false, nil // the person says No
	}}
	// The consent-free run below reaches beginRun: its crash-path record
	// goes to a temp dir, not the real runner home.
	r := &RunAPI{app: app, runnerHomeDir: t.TempDir()}
	// A draft reaches its dialog only with a Jev key of the person's, here
	// the variable's, under a runner home of the test's own.
	t.Setenv(policy.HomeEnv, t.TempDir())
	t.Setenv(policy.DefaultJudgeKeyEnv, "jev-consent-test")
	// A settings file of the test's own: Unsafe Mode is off.
	t.Setenv(configdir.EnvVar, t.TempDir())
	for name, call := range map[string]func() error{
		"RestartNow":       func() error { return r.RestartNow() },
		"InstallEngine":    func() error { _, err := r.InstallEngine(); return err },
		"AddToDockerGroup": func() error { _, err := r.AddToDockerGroup(); return err },
		"RunAgent unsigned": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"InstallAPI.Confirm unscanned": func() error {
			i := &InstallAPI{app: app}
			_, err := i.Confirm("plan", false, true, true)
			return err
		},
		// Describe it, on this computer: where the description goes is asked
		// before anything leaves this computer.
		"BuildAPI.DescribeOnThisComputer": func() error {
			b := &BuildAPI{app: app, run: r}
			_, err := b.DescribeOnThisComputer("https://r.example", "Summarize my meeting notes.", nil, nil, nil)
			return err
		},
	} {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "not confirmed") {
			t.Errorf("%s with the dialog answered No: err = %v, want a refusal", name, err)
		}
	}
	if len(asked) != 6 {
		t.Fatalf("dialogs raised = %v, want one per call", asked)
	}
	// A consent-free run raises no dialog: nothing to confirm.
	before := len(asked)
	_, _ = r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil)
	if len(asked) != before {
		t.Fatalf("a run with no consent flags raised a dialog: %v", asked[before:])
	}
	// A declared server the gate rates high runs here only in Unsafe Mode:
	// with the switch off, the page's consent for one is refused before any
	// dialog, since a Yes could not let it through, and the page cannot turn
	// the switch on (no binding takes it).
	before = len(asked)
	for name, call := range map[string]func() error{
		"RunAgent": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, true, false, nil, false, nil, nil, nil)
			return err
		},
		"StartSession": func() error {
			_, err := r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, false, nil, true, false, nil, false, nil, nil, nil)
			return err
		},
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "only in Unsafe Mode, and Unsafe Mode is off") {
			t.Errorf("%s with a vulnerable server's consent and Unsafe Mode off: err = %v, want the refusal", name, err)
		}
	}
	if len(asked) != before {
		t.Fatalf("a consent Unsafe Mode's absence refuses raised a dialog: %v", asked[before:])
	}
	// No dialog available at all (no window): refuse, never assume yes.
	none := &RunAPI{app: &App{ctx: context.Background()}}
	if err := none.RestartNow(); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("no dialog seam: %v", err)
	}
}

func TestPathsMustComeFromTheNativePicker(t *testing.T) {
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	r := &RunAPI{app: app}
	b := &BuildAPI{app: app}
	typed := filepath.Join(t.TempDir(), "typed")
	for name, call := range map[string]func() error{
		"RunAgent inputs": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", []string{typed}, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"RunAgent outDir": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, typed, nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"StartSession outDir": func() error {
			_, err := r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, typed, nil, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"BuildAPI.Save": func() error { return b.Save(typed, build.Model{}) },
		"BuildAPI.Submit": func() error {
			_, err := b.Submit("https://r.example", typed)
			return err
		},
		"BuildAPI.Install": func() error {
			_, err := b.Install(typed)
			return err
		},
		"BuildAPI.Validate": func() error {
			_, err := b.Validate(typed)
			return err
		},
	} {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
			t.Errorf("%s with a path the page typed: err = %v, want the picker refusal", name, err)
		}
	}
	// A picked directory admits itself and the files under it; a sibling
	// stays refused.
	picked := t.TempDir()
	app.markPicked(picked)
	if err := app.requirePicked("input", filepath.Join(picked, "notes", "a.txt")); err != nil {
		t.Fatalf("a file under a picked directory: %v", err)
	}
	if err := app.requirePicked("input", picked+"-sibling"); err == nil {
		t.Fatal("a sibling of a picked directory passed")
	}
	if err := app.requirePicked("input", filepath.Join(picked, "..", "escape")); err == nil {
		t.Fatal("a traversal out of a picked directory passed")
	}
	var nilErr error
	if err := app.requirePicked("output directory", ""); !errors.Is(err, nilErr) {
		t.Fatalf("the empty path is the binding's default and must pass: %v", err)
	}
}

// A target host's key file is chosen in a picker of its own, recorded apart
// from the run's inputs: a key picked there is never an input a run stages,
// and an input picked as one is never a key sent to a host. A cancelled pick
// records nothing.
func TestAHostKeyPickIsNeverAnInputAndAnInputNeverAKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	app.pickKeyFile = func(context.Context, runtime.OpenDialogOptions) (string, error) { return keyPath, nil }
	r := &RunAPI{app: app}
	if got, err := r.ChooseHostKeyFile("admin@server"); err != nil || got != keyPath {
		t.Fatalf("pick: %q %v", got, err)
	}
	if err := app.keyFiles.require("the key file", keyPath); err != nil {
		t.Fatalf("a picked key is refused as a key: %v", err)
	}
	if _, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", []string{keyPath}, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("a picked key was taken as an input: %v", err)
	}
	input := filepath.Join(t.TempDir(), "notes.txt")
	app.markPicked(input)
	if err := app.keyFiles.require("the key file", input); err == nil {
		t.Fatal("an input pick was taken as a key")
	}
	app.pickKeyFile = func(context.Context, runtime.OpenDialogOptions) (string, error) { return "", nil }
	other := filepath.Join(t.TempDir(), "id_rsa")
	if got, err := r.ChooseHostKeyFile(""); got != "" || err != nil || app.keyFiles.require("k", other) == nil {
		t.Fatalf("a cancelled pick: %q %v", got, err)
	}
}
