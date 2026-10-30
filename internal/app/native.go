package app

// The app's side of the window toolkit: the native dialogs, the system
// browser, the window and the page's events. Every function here reaches the
// running application (application.Get) and its main window; before the
// application exists, or in a test, the callers' seams stand in. Nothing
// else in the package imports the toolkit's application package.

import (
	"context"
	"errors"
	"io/fs"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// mainWindowName is the name main.go gives the one window.
const mainWindowName = "main"

// errNoApplication is every native call's answer before the application is
// running.
var errNoApplication = errors.New("the application is not running")

// mainWindow is the app's window, nil before it exists.
func mainWindow() application.Window {
	wails := application.Get()
	if wails == nil {
		return nil
	}
	w, ok := wails.Window.GetByName(mainWindowName)
	if !ok {
		return nil
	}
	return w
}

// fileFilter is one entry of a picker's type menu.
type fileFilter struct{ Name, Pattern string }

// pickOptions describes a native file or directory picker.
type pickOptions struct {
	Title string
	// Filters limits a file picker to the types named; none shows every file.
	Filters []fileFilter
	// ShowHidden shows dot files, for a private key kept under ~/.ssh.
	ShowHidden bool
	// DefaultFilename is the name a save dialog proposes.
	DefaultFilename string
	// Directory is where the dialog opens; empty is the system's choice.
	Directory string
}

// nativeOpenFile is the OS file picker, "" when cancelled.
func nativeOpenFile(_ context.Context, o pickOptions) (string, error) {
	wails := application.Get()
	if wails == nil {
		return "", errNoApplication
	}
	d := wails.Dialog.OpenFile().SetTitle(o.Title).CanChooseFiles(true).ShowHiddenFiles(o.ShowHidden)
	if o.Directory != "" {
		d.SetDirectory(o.Directory)
	}
	for _, f := range o.Filters {
		d.AddFilter(f.Name, f.Pattern)
	}
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// nativeOpenFiles is the OS multi-file picker, empty when cancelled.
func nativeOpenFiles(_ context.Context, title string) ([]string, error) {
	wails := application.Get()
	if wails == nil {
		return nil, errNoApplication
	}
	d := wails.Dialog.OpenFile().SetTitle(title).CanChooseFiles(true)
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForMultipleSelection()
}

// nativeSaveFile is the OS save dialog, "" when cancelled.
func nativeSaveFile(_ context.Context, o pickOptions) (string, error) {
	wails := application.Get()
	if wails == nil {
		return "", errNoApplication
	}
	d := wails.Dialog.SaveFile().SetMessage(o.Title).SetFilename(o.DefaultFilename)
	for _, f := range o.Filters {
		d.AddFilter(f.Name, f.Pattern)
	}
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// nativePickDirectory is the OS directory dialog.
func nativePickDirectory(_ context.Context, title string) (string, error) {
	wails := application.Get()
	if wails == nil {
		return "", errNoApplication
	}
	d := wails.Dialog.OpenFile().SetTitle(title).CanChooseFiles(false).CanChooseDirectories(true)
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// nativeConfirm is the OS question dialog, No by default and on cancel.
func nativeConfirm(ctx context.Context, title, message string) (bool, error) {
	wails := application.Get()
	if wails == nil {
		return false, errNoApplication
	}
	answer := make(chan bool, 1)
	d := wails.Dialog.Question().SetTitle(title).SetMessage(message)
	d.AddButton("Yes").OnClick(func() { answer <- true })
	no := d.AddButton("No").OnClick(func() { answer <- false })
	d.SetDefaultButton(no).SetCancelButton(no)
	if ctx == nil {
		ctx = context.Background()
	}
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	d.Show()
	select {
	case yes := <-answer:
		return yes, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// nativeInfo is the OS information dialog, which returns once it is closed.
func nativeInfo(ctx context.Context, title, message string) {
	wails := application.Get()
	if wails == nil {
		return
	}
	closed := make(chan struct{}, 1)
	d := wails.Dialog.Info().SetTitle(title).SetMessage(message)
	ok := d.AddButton("OK").OnClick(func() { closed <- struct{}{} })
	d.SetDefaultButton(ok).SetCancelButton(ok)
	if ctx == nil {
		ctx = context.Background()
	}
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	d.Show()
	select {
	case <-closed:
	case <-ctx.Done():
	}
}

// nativeOpenURL opens an address in the system browser.
func nativeOpenURL(_ context.Context, url string) {
	if wails := application.Get(); wails != nil {
		_ = wails.Browser.OpenURL(url)
	}
}

// nativeEmit sends an event to the page.
func nativeEmit(name string, data ...any) {
	if wails := application.Get(); wails != nil {
		wails.Event.Emit(name, data...)
	}
}

// nativeShowWindow brings the window up, from hidden or minimised.
func nativeShowWindow(context.Context) {
	if w := mainWindow(); w != nil {
		w.Show()
		w.UnMinimise()
		w.Focus()
	}
}

// nativeHideWindow hides the window; the app goes on.
func nativeHideWindow(context.Context) {
	if w := mainWindow(); w != nil {
		w.Hide()
	}
}

// nativeWindowMinimised reports whether the window is minimised.
func nativeWindowMinimised() bool {
	w := mainWindow()
	return w != nil && w.IsMinimised()
}

// nativeQuit ends the application: its shutdown runs, then the process ends.
func nativeQuit(context.Context) {
	if wails := application.Get(); wails != nil {
		wails.Quit()
	}
}

// Assets is the asset server's handler: the bundle the binary embeds, with
// /theme.js (ThemeScript) served ahead of it.
func Assets(bundle fs.FS) http.Handler {
	files := application.AssetFileServerFS(bundle)
	theme := ThemeScript()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/theme.js" {
			theme.ServeHTTP(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
