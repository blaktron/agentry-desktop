package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
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

	// The renderer is not a principal. Every bound method is reachable from
	// page script, so what the page asserts — a ticked consent box, a path
	// it typed — is a claim, not a decision. Two things stand between a
	// claim and an action: a consent is asked again in a native dialog the
	// page cannot answer (confirm), and a filesystem path is accepted only
	// when a native picker in this process returned it, or Go made it for
	// the Build tab's Save (picked).
	confirm func(ctx context.Context, title, message string) (bool, error)
	// pickDirectory is the native directory picker ("" when cancelled);
	// a test substitutes one, the way it does for confirm.
	pickDirectory func(ctx context.Context, title string) (string, error)
	mu            sync.Mutex
	picked        map[string]bool
	// keyFiles are the private keys the native picker returned for a
	// target host's sign-in (ChooseHostKeyFile), kept apart from picked:
	// a key is never an input, and an input never a key.
	keyFiles keyFilePicks
	// pickKeyFile is the native file picker ChooseHostKeyFile opens; nil
	// is the OS's, and a test substitutes one.
	pickKeyFile func(ctx context.Context, opts runtime.OpenDialogOptions) (string, error)
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

// errNotConfirmed is askConsent's answer to a No, for a binding whose No
// is an outcome rather than a failure.
var errNotConfirmed = errors.New("not confirmed")

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
		return fmt.Errorf("%w: %s", errNotConfirmed, title)
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
//
// The path must lie under a picked root both as written and once links,
// and on Windows directory junctions, are resolved on both sides: a link
// inside a picked folder that leads out of it would otherwise let the page
// name any file the link reaches, while a picked folder that is itself
// reached through a link (a linked home directory) still admits what lies
// under it. A path that does not exist yet is judged by its nearest
// existing parent. A ".." element is refused outright: the system applies
// it after following a link, while a lexical clean removes it before, so
// the two would judge different files.
func (a *App) requirePicked(what, p string) error {
	if p == "" {
		return nil
	}
	notPicked := fmt.Errorf("%s %q was not chosen through the file dialog; pick it there", what, p)
	if hasDotDot(p) {
		return notPicked
	}
	clean := filepath.Clean(p)
	var roots []string
	a.mu.Lock()
	for root := range a.picked {
		if root == clean || within(root, clean) {
			roots = append(roots, root)
		}
	}
	a.mu.Unlock()
	if len(roots) == 0 {
		return notPicked
	}
	real, err := resolveExisting(clean)
	if err != nil {
		return fmt.Errorf("%s %q could not be followed to the file it names (%w); pick it again", what, p, err)
	}
	resolved := false
	for _, root := range roots {
		realRoot, err := resolveExisting(root)
		if err != nil {
			continue
		}
		resolved = true
		if real == realRoot || within(realRoot, real) {
			return nil
		}
	}
	if !resolved {
		return fmt.Errorf("the folder %s %q was chosen from can no longer be read; pick it again", what, p)
	}
	return fmt.Errorf("%s %q reaches outside the folder it was chosen from through a link; pick the file itself", what, p)
}

// hasDotDot reports whether any element of p is "..".
func hasDotDot(p string) bool {
	for _, el := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if el == ".." {
			return true
		}
	}
	return false
}

// resolveExisting resolves every link in the clean path p, junctions
// included on Windows. A path that does not exist resolves its nearest
// existing parent and keeps the rest as written: nothing below that parent
// exists, so nothing there is a link. A link whose target is missing is an
// error, not an absent name, because a write through it would land
// wherever it points.
func resolveExisting(p string) (string, error) {
	real, err := resolveLinks(p)
	if err == nil {
		return real, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if _, lerr := os.Lstat(p); lerr == nil {
		return "", fmt.Errorf("a link whose target is missing: %w", err)
	}
	parent := filepath.Dir(p)
	if parent == p {
		return "", err
	}
	realParent, err := resolveExisting(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(realParent, filepath.Base(p)), nil
}

// requireOutputDir is requirePicked for a run's output folder, which may
// also be the one Run defaults saved: Go reads it from its own settings
// file, so the page cannot name it, and it admits that folder exactly — as
// an output folder only, never a folder under it and never an input, which
// still needs this session's picker.
func (a *App) requireOutputDir(p string) error {
	err := a.requirePicked("output directory", p)
	if err == nil {
		return nil
	}
	if isSavedOutputDir(p) {
		return nil
	}
	return err
}

// isSavedOutputDir says whether p is Run defaults' saved output folder,
// which every run shares and so hands each run a folder of its own inside
// it, whether or not it was also picked in this session.
func isSavedOutputDir(p string) bool {
	if p == "" {
		return false
	}
	st, err := settings.Load()
	return err == nil && st.DefaultOutputDir != "" && filepath.Clean(st.DefaultOutputDir) == filepath.Clean(p)
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

// ctxOrBackground is the window's context, or a background one before it exists.
func (a *App) ctxOrBackground() context.Context {
	if a != nil && a.ctx != nil {
		return a.ctx
	}
	return context.Background()
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
// bindings that store a directory themselves (Settings → Coding tools'
// project directory), through the seam a test substitutes: what it
// returns is the picker's by construction, never a path the page passed.
func (a *App) chooseDirectory(title string) (string, error) {
	dir, err := a.pickDirectoryUnmarked(title)
	if err == nil {
		a.markPicked(dir)
	}
	return dir, err
}

// pickDirectoryUnmarked opens the native directory picker and records
// nothing in the picked set: for a folder chosen for one use only, as Run
// defaults' output folder is.
func (a *App) pickDirectoryUnmarked(title string) (string, error) {
	if a.pickDirectory == nil {
		return "", fmt.Errorf("no directory picker is available")
	}
	return a.pickDirectory(a.ctx, title)
}

// ChooseFiles opens the native multi-file picker and returns the chosen
// paths (empty when the user cancelled) — the run surface's input-files
// channel is fed from here.
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
	browserOpenURL(a.ctx, url)
	return nil
}

// browserOpenURL opens an address in the system browser; a test replaces it.
var browserOpenURL = runtime.BrowserOpenURL
