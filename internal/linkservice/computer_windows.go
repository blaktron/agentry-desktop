//go:build windows

package linkservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
	"github.com/blaktron/agentry-desktop/internal/computeruse/win32"
)

// SetTokenPath has nothing to keep on this system.
func SetTokenPath(string) {}

// openComputer opens computer use through the Windows window manager.
func openComputer(ctx context.Context, req ComputerRequest) (computeruse.Backend, error) {
	opt := win32.Options{Scope: req.Scope}
	if req.Scope == computeruse.ScopeApp {
		if len(req.Apps) == 0 {
			return nil, fmt.Errorf("%w: on Windows, computer use is held to the windows of the applications a QuickAgent uses, and this one names none", computeruse.ErrNotGranted)
		}
		opt.Match = func(exe string) (string, bool) {
			for _, a := range req.Apps {
				for _, c := range a.Classes {
					if strings.EqualFold(c, exe) {
						return a.Name, true
					}
				}
			}
			return "", false
		}
	}
	return win32.Open(ctx, opt)
}

// ComputerSupport says what computer use can do here. Windows asks for no
// permission; what it does not let through is input to an application run
// as administrator.
func ComputerSupport() Support {
	return Support{Session: "Windows", Ready: true,
		Holds: "Windows captures only the window of the application the run uses; it holds no input to a window, so Agentry Desktop checks before each input that the window is in the foreground, the point inside it and no other window above it there. An application run as administrator cannot take Agentry Desktop's input, and a run that uses one is refused when it starts."}
}

// RequestComputerPermissions has nothing to ask on Windows.
func RequestComputerPermissions() Support { return ComputerSupport() }

// runningWindows is not read here: the person types the identifier.
func runningWindows(context.Context) ([]RunningWindow, error) {
	return nil, errors.New("this system's windows cannot be listed yet: type the application's identifier (its bundle identifier on macOS, its executable's file name on Windows)")
}
