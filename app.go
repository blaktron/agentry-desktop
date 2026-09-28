package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// App carries the Wails lifecycle context the bound services share.
type App struct {
	ctx context.Context

	// The renderer is not a principal (D-030). Every bound method is
	// reachable from page script, so what the page asserts — a ticked
	// consent box, a path it typed — is a claim, not a decision. Two
	// things stand between a claim and an action: a consent is asked
	// again in a native dialog the page cannot answer (confirm), and a
	// filesystem path is accepted only when a native picker in this
	// process returned it (picked).
	confirm func(ctx context.Context, title, message string) (bool, error)
	// pickDirectory is the native directory picker ("" when cancelled);
	// a test substitutes one, the way it does for confirm.
	pickDirectory func(ctx context.Context, title string) (string, error)
	mu            sync.Mutex
	picked        map[string]bool
}

// NewApp creates the application struct.
func NewApp() *App {
	return &App{confirm: nativeConfirm, pickDirectory: nativePickDirectory}
}

// nativePickDirectory is the OS directory dialog.
func nativePickDirectory(ctx context.Context, title string) (string, error) {
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: title})
}

// nativeConfirm is the OS question dialog, No by default and on cancel.
func nativeConfirm(ctx context.Context, title, message string) (bool, error) {
	answer, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type: runtime.QuestionDialog, Title: title, Message: message,
		Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
	})
	if err != nil {
		return false, err
	}
	return answer == "Yes", nil
}

// askConsent puts the question to the person in a native dialog and
// returns whether they said yes. A binding that would act on a consent the
// page passed calls this first; the page's own checkbox is a hint, the
// dialog is the gate. Unexported: a bound method that showed a dialog on
// demand would be a nuisance the page could raise, not a gate.
func (a *App) askConsent(title, message string) error {
	if a.confirm == nil {
		return fmt.Errorf("no consent dialog is available; refusing")
	}
	yes, err := a.confirm(a.ctx, title, message)
	if err != nil {
		return fmt.Errorf("consent dialog: %w", err)
	}
	if !yes {
		return fmt.Errorf("not confirmed: %s", title)
	}
	return nil
}

// markPicked records a path a native picker returned, so the bindings
// that take a path can tell one the person chose from one the page typed.
func (a *App) markPicked(paths ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.picked == nil {
		a.picked = map[string]bool{}
	}
	for _, p := range paths {
		if p != "" {
			a.picked[filepath.Clean(p)] = true
		}
	}
}

// requirePicked refuses a path no native picker in this process returned:
// the path itself, or a file under a directory that was picked. The empty
// path is the binding's own default and passes.
func (a *App) requirePicked(what, p string) error {
	if p == "" {
		return nil
	}
	clean := filepath.Clean(p)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.picked[clean] {
		return nil
	}
	for dir := range a.picked {
		if rel, err := filepath.Rel(dir, clean); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return nil
		}
	}
	return fmt.Errorf("%s %q was not chosen through the file dialog; pick it there", what, p)
}

// requireOutputDir is requirePicked for a run's output folder, which may
// also be the one Run defaults saved (D-044): Go reads it from its own
// settings file, so the page cannot name it, and it admits that folder
// exactly — as an output folder only, never a folder under it and never an
// input, which still needs this session's picker.
func (a *App) requireOutputDir(p string) error {
	err := a.requirePicked("output directory", p)
	if err == nil {
		return nil
	}
	if st, lerr := settings.Load(); lerr == nil && st.DefaultOutputDir != "" &&
		filepath.Clean(st.DefaultOutputDir) == filepath.Clean(p) {
		return nil
	}
	return err
}

// requirePickedResolved is requirePicked for a path whose bytes leave the
// machine (a document a build uploads, agentry-desktop#274): the path must
// be one a picker returned, or under one, both as written and once links
// are resolved. requirePicked compares the paths as text, so a link inside
// a picked folder could otherwise name a file outside it.
func (a *App) requirePickedResolved(what, p string) error {
	if err := a.requirePicked(what, p); err != nil {
		return err
	}
	clean := filepath.Clean(p)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return fmt.Errorf("%s %q: %w", what, p, err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for root := range a.picked {
		if root != clean && !within(root, clean) {
			continue
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if real == realRoot || within(realRoot, real) {
			return nil
		}
	}
	return fmt.Errorf("%s %q reaches outside the folder it was chosen from through a link; pick the file itself", what, p)
}

// within reports whether p lies under dir.
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// startup saves the runtime context.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// AppVersion returns the build-stamped version.
func (a *App) AppVersion() string {
	return version.Version
}

// ChooseDirectory opens the native directory picker and returns the chosen
// path ("" when the user cancelled).
func (a *App) ChooseDirectory(title string) (string, error) {
	return a.chooseDirectory(title)
}

// chooseDirectory is the picker behind ChooseDirectory and behind the
// bindings that store a directory themselves (Settings → Coding agents'
// project directory), through the seam a test substitutes: what it
// returns is the picker's by construction, never a path the page passed.
func (a *App) chooseDirectory(title string) (string, error) {
	if a.pickDirectory == nil {
		return "", fmt.Errorf("no directory picker is available")
	}
	dir, err := a.pickDirectory(a.ctx, title)
	if err == nil {
		a.markPicked(dir)
	}
	return dir, err
}

// ChooseFiles opens the native multi-file picker and returns the chosen
// paths (empty when the user cancelled) — the run surface's input-files
// channel (RUNNER-PLAN-2 §P4) is fed from here.
func (a *App) ChooseFiles(title string) ([]string, error) {
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []string{}
	}
	a.markPicked(files...)
	return files, nil
}

// OpenExternal opens an http(s) URL in the system browser. Anything else is
// refused — the frontend renders registry-supplied content, and a scheme
// like file: or javascript: coming out of it must not reach the OS.
func (a *App) OpenExternal(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("refusing to open non-http(s) URL")
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}
