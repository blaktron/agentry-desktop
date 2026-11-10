package app

// The app's side of the window toolkit: the card windows and the system's
// dialogs behind them, the system browser, the window and the page's
// events. Every function here reaches the running application (application.Get) and its main window; before the
// application exists, or in a test, the callers' seams stand in. Nothing
// else in the package imports the toolkit's application package.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
	wailsevents "github.com/wailsapp/wails/v3/pkg/events"

	"github.com/blaktron/agentry-desktop/internal/linkservice"
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

// nativeConfirm asks a Yes/No question in a card window, No by default and
// on cancel, or in the system's message box when no card window opens.
func nativeConfirm(ctx context.Context, title, message string) (bool, error) {
	button, err := cards.ask(ctx, confirmCard(title, message))
	if !errors.Is(err, errNoCardWindow) {
		return err == nil && button == 0, err
	}
	return systemConfirm(ctx, title, message)
}

// systemConfirm is the system's question box, No by default and on cancel.
func systemConfirm(ctx context.Context, title, message string) (bool, error) {
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

// execAnswers are an exec call's answers, in the order they are offered;
// declining, the last, is the default and the cancel.
var execAnswers = []linkservice.ExecAnswer{linkservice.ExecAllowOnce, linkservice.ExecAllowRun, linkservice.ExecDecline}

// execCard is an exec call's card: what would run, quoted, and its three
// answers.
func execCard(q linkservice.ExecQuestion) CardView {
	title, message := execDialogText(q)
	return CardView{Kind: cardQuestion, Title: title, Message: message, Cancel: 2,
		Buttons: []CardButton{{Label: execRunIt, Tone: tonePrimary}, {Label: execAllowRun}, {Label: execDecline}}}
}

// nativeAskExec is an exec call's question, in a card window or, when none
// opens, in the system's message box.
func nativeAskExec(ctx context.Context, q linkservice.ExecQuestion) (linkservice.ExecAnswer, error) {
	button, err := cards.ask(ctx, execCard(q))
	if !errors.Is(err, errNoCardWindow) {
		if err != nil || button < 0 || button >= len(execAnswers) {
			return linkservice.ExecDecline, err
		}
		return execAnswers[button], nil
	}
	return systemAskExec(ctx, q)
}

// systemAskExec is an exec call's system message box.
func systemAskExec(ctx context.Context, q linkservice.ExecQuestion) (linkservice.ExecAnswer, error) {
	wails := application.Get()
	if wails == nil {
		return linkservice.ExecDecline, errNoApplication
	}
	title, message := execDialogText(q)
	answer := make(chan linkservice.ExecAnswer, 1)
	d := wails.Dialog.Question().SetTitle(title).SetMessage(message)
	d.AddButton(execRunIt).OnClick(func() { answer <- linkservice.ExecAllowOnce })
	d.AddButton(execAllowRun).OnClick(func() { answer <- linkservice.ExecAllowRun })
	no := d.AddButton(execDecline).OnClick(func() { answer <- linkservice.ExecDecline })
	d.SetDefaultButton(no).SetCancelButton(no)
	if ctx == nil {
		ctx = context.Background()
	}
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	d.Show()
	select {
	case a := <-answer:
		return a, nil
	case <-ctx.Done():
		return linkservice.ExecDecline, ctx.Err()
	}
}

// nativeInfo shows a notice in a card window, or in the system's
// information box when none opens, and returns once it is closed.
func nativeInfo(ctx context.Context, title, message string) {
	if _, err := cards.ask(ctx, noticeCard(title, message)); !errors.Is(err, errNoCardWindow) {
		return
	}
	systemInfo(ctx, title, message)
}

// systemInfo is the system's information box.
func systemInfo(ctx context.Context, title, message string) {
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

// nativeOpenCard opens the window a card is shown in: small, above the
// other windows, with no frame of its own (the card draws its own head, by
// which it is moved), centred on the screen.
func nativeOpenCard(title string, closed func()) (cardWindow, error) {
	wails := application.Get()
	if wails == nil {
		return nil, errNoApplication
	}
	w := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       cardWindowName,
		Title:                      title,
		Width:                      cardWidth,
		Height:                     cardHeight,
		AlwaysOnTop:                true,
		Frameless:                  true,
		URL:                        "/#card",
		DefaultContextMenuDisabled: true,
		InitialPosition:            application.WindowCentered,
	})
	w.RegisterHook(wailsevents.Common.WindowClosing, func(*application.WindowEvent) { closed() })
	w.Show()
	w.Focus()
	return w, nil
}

// The card window's size when it opens; the card then fits it to itself.
const (
	cardWidth  = 520
	cardHeight = 260
)

// nativeCaller is the window a bound call came from, which the webview
// layer attaches to the call.
func nativeCaller(ctx context.Context) (uint, bool) {
	w, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok || w == nil {
		return 0, false
	}
	return w.ID(), true
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

// chooseCard asks which of the candidates a release's entry is, with
// None of these last, the default and the cancel.
func chooseCard(q linkservice.ChooseQuestion) CardView {
	title, message := chooseDialogText(q)
	v := CardView{Kind: cardQuestion, Title: title, Message: message}
	for _, c := range q.Candidates {
		v.Buttons = append(v.Buttons, CardButton{Label: c.Name})
	}
	v.Buttons = append(v.Buttons, CardButton{Label: chooseNone})
	v.Cancel = len(v.Buttons) - 1
	return v
}

// nativeChoose asks which of the person's applications a release's entry
// is, one button for each of up to four candidates; more than four are
// chosen in the run setup. It asks in a card window or, when none opens, in
// the system's message box.
func nativeChoose(ctx context.Context, q linkservice.ChooseQuestion) (string, error) {
	if len(q.Candidates) > maxChoiceButtons {
		return "", fmt.Errorf("more than %d of your applications could be it: choose it in the run setup", maxChoiceButtons)
	}
	button, err := cards.ask(ctx, chooseCard(q))
	if !errors.Is(err, errNoCardWindow) {
		if err != nil {
			return "", err
		}
		if button < 0 || button >= len(q.Candidates) {
			return "", errNotConfirmed
		}
		return q.Candidates[button].ID, nil
	}
	return systemChoose(ctx, q)
}

// systemChoose is nativeChoose's system message box.
func systemChoose(ctx context.Context, q linkservice.ChooseQuestion) (string, error) {
	wails := application.Get()
	if wails == nil {
		return "", errNoApplication
	}
	title, message := chooseDialogText(q)
	answer := make(chan string, 1)
	d := wails.Dialog.Question().SetTitle(title).SetMessage(message)
	for _, c := range q.Candidates {
		id := c.ID
		d.AddButton(c.Name).OnClick(func() { answer <- id })
	}
	no := d.AddButton(chooseNone).OnClick(func() { answer <- "" })
	d.SetDefaultButton(no).SetCancelButton(no)
	if ctx == nil {
		ctx = context.Background()
	}
	if w := mainWindow(); w != nil {
		d.AttachToWindow(w)
	}
	d.Show()
	select {
	case id := <-answer:
		if id == "" {
			return "", errNotConfirmed
		}
		return id, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
