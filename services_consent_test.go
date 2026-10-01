package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// The renderer is not a principal (D-030; security review 2026-09-20 §8.4,
// §8.6). A consent the page passes is asked again in a native dialog the
// page cannot answer, and a path the page names is refused unless a native
// picker in this process returned it.

func TestConsentIsANativeDialogNotAPageFlag(t *testing.T) {
	asked := []string{}
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, _ string) (bool, error) {
		asked = append(asked, title)
		return false, nil // the person says No
	}}
	// The consent-free run below reaches beginRun: its crash-path record
	// (agentry-desktop#198) goes to a temp dir, not the real runner home.
	r := &RunAPI{app: app, runnerHomeDir: t.TempDir()}
	// A draft reaches its dialog only with a Jev key of the person's, here
	// the variable's, under a runner home of the test's own.
	t.Setenv(policy.HomeEnv, t.TempDir())
	t.Setenv(policy.DefaultJudgeKeyEnv, "jev-consent-test")
	for name, call := range map[string]func() error{
		"RestartNow":       func() error { return r.RestartNow() },
		"InstallDocker":    func() error { _, err := r.InstallDocker(); return err },
		"AddToDockerGroup": func() error { _, err := r.AddToDockerGroup(); return err },
		"RunAgent unsigned": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil)
			return err
		},
		"StartSession vulnerable MCP": func() error {
			_, err := r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, false, nil, true, false, nil)
			return err
		},
		"InstallAPI.Confirm unscanned": func() error {
			i := &InstallAPI{app: app}
			_, err := i.Confirm("plan", false, true, true)
			return err
		},
		// Describe it, on this computer (#282): where the description goes is
		// asked before anything leaves this computer.
		"BuildAPI.DescribeOnThisComputer": func() error {
			b := &BuildAPI{app: app, run: r}
			_, err := b.DescribeOnThisComputer("https://r.example", "Summarize my meeting notes.", nil)
			return err
		},
	} {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "not confirmed") {
			t.Errorf("%s with the dialog answered No: err = %v, want a refusal", name, err)
		}
	}
	if len(asked) != 7 {
		t.Fatalf("dialogs raised = %v, want one per call", asked)
	}
	// A consent-free run raises no dialog: nothing to confirm.
	before := len(asked)
	_, _ = r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, false, false, nil)
	if len(asked) != before {
		t.Fatalf("a run with no consent flags raised a dialog: %v", asked[before:])
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
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", []string{typed}, "", nil, 0, 0, false, nil, false, false, nil)
			return err
		},
		"RunAgent outDir": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, typed, nil, 0, 0, false, nil, false, false, nil)
			return err
		},
		"StartSession outDir": func() error {
			_, err := r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, typed, nil, 0, false, nil, false, false, nil)
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
