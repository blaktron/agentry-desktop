package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-desktop/internal/version"
)

// App carries the Wails lifecycle context the bound services share.
type App struct {
	ctx context.Context
}

// NewApp creates the application struct.
func NewApp() *App {
	return &App{}
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
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
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
