//go:build darwin

package linkservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
	"github.com/blaktron/agentry-desktop/internal/computeruse/quartz"
)

// SetTokenPath has nothing to keep on this system: macOS remembers what
// the person allowed in its own privacy settings.
func SetTokenPath(string) {}

// openComputer opens computer use through Quartz and ScreenCaptureKit.
func openComputer(ctx context.Context, req ComputerRequest) (computeruse.Backend, error) {
	opt := quartz.Options{Scope: req.Scope}
	if req.Scope == computeruse.ScopeApp {
		if len(req.Apps) == 0 {
			return nil, fmt.Errorf("%w: on macOS, computer use is held to the windows of the applications a QuickAgent uses, and this one names none", computeruse.ErrNotGranted)
		}
		opt.Match = func(bundle, owner string) (string, bool) {
			return matchApp(req.Apps, bundle, owner)
		}
	}
	return quartz.Open(ctx, opt)
}

// matchApp is the application a window belongs to, by any of the names the
// system gives it.
func matchApp(apps []ComputerApp, names ...string) (string, bool) {
	for _, a := range apps {
		for _, c := range a.Classes {
			for _, n := range names {
				if n != "" && strings.EqualFold(c, n) {
					return a.Name, true
				}
			}
		}
	}
	return "", false
}

// ComputerSupport says what computer use can do here: the two privacy
// permissions it needs, and what macOS holds of app scope.
func ComputerSupport() Support {
	p := quartz.Granted()
	s := Support{Session: "macOS", Ready: p.Screen && p.Accessibility, Permissions: []Permission{
		{Key: PermissionScreen, Name: "Screen & System Audio Recording", Granted: p.Screen, Why: "to capture the window of the application a QuickAgent uses"},
		{Key: PermissionAccessibility, Name: "Accessibility", Granted: p.Accessibility, Why: "to post its clicks and keys, and to hear the stop key"},
	}}
	if err := quartz.Missing(p); err != nil {
		s.Problem = trimNotGranted(err)
	}
	how := "ScreenCaptureKit captures only the window of the application the run uses"
	if !quartz.CaptureKit() {
		how = "macOS captures only the window of the application the run uses"
	}
	s.Holds = how + "; macOS holds no input to a window, so Agentry Desktop checks before each input that the window is frontmost, the point inside it and no other window above it there."
	return s
}

// RequestComputerPermissions asks macOS for what computer use needs and
// says what is granted now. macOS asks for Screen Recording once, and only
// a restart of Agentry Desktop takes a grant made in System Settings.
func RequestComputerPermissions() Support {
	quartz.Request()
	return ComputerSupport()
}

// runningWindows is not read here: the person types the identifier.
func runningWindows(context.Context) ([]RunningWindow, error) {
	return nil, errors.New("this system's windows cannot be listed yet: type the application's identifier (its bundle identifier on macOS, its executable's file name on Windows)")
}
