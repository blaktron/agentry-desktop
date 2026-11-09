//go:build linux

package linkservice

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
	"github.com/blaktron/agentry-desktop/internal/computeruse/kwin"
	"github.com/blaktron/agentry-desktop/internal/computeruse/wayland"
	"github.com/blaktron/agentry-desktop/internal/computeruse/x11"
)

// DisplayVariable, set to x11, has computer use work through the X server
// (Xwayland under a Wayland session) rather than the desktop portal.
const DisplayVariable = "AGENTRY_DESKTOP_COMPUTER_USE_DISPLAY"

// tokenPath is where the portal's restore tokens are kept; set by the app.
var tokenPath string

// SetTokenPath sets the file the desktop portal's restore tokens are kept
// in.
func SetTokenPath(path string) { tokenPath = path }

// openComputer opens computer use on this session: through the desktop
// portal on Wayland, through the X server on X11.
func openComputer(ctx context.Context, req ComputerRequest) (computeruse.Backend, error) {
	wl := os.Getenv("WAYLAND_DISPLAY") != ""
	if strings.EqualFold(os.Getenv(DisplayVariable), "x11") {
		wl = false
	}
	if wl {
		apps := make([]wayland.App, 0, len(req.Apps))
		for _, a := range req.Apps {
			apps = append(apps, wayland.App{Name: a.Name, Classes: a.Classes})
		}
		return wayland.Open(ctx, wayland.Options{Scope: req.Scope, Apps: apps, Tokens: NewTokens(tokenPath), ScriptDir: kwin.ScriptDir()})
	}
	if os.Getenv("DISPLAY") == "" {
		return nil, fmt.Errorf("%w: no graphical session is open for Agentry Desktop (neither WAYLAND_DISPLAY nor DISPLAY is set)", computeruse.ErrNotGranted)
	}
	opt := x11.Options{Scope: req.Scope}
	if req.Scope == computeruse.ScopeApp {
		if len(req.Apps) == 0 {
			return nil, fmt.Errorf("%w: on X11, computer use is held to the windows of the applications a QuickAgent uses, and this one names none", computeruse.ErrNotGranted)
		}
		opt.Match = func(instance, class string, _ int) (string, bool) {
			for _, a := range req.Apps {
				for _, c := range a.Classes {
					if strings.EqualFold(c, instance) || strings.EqualFold(c, class) {
						return a.Name, true
					}
				}
			}
			return "", false
		}
	}
	return x11.Open(ctx, opt)
}

// ComputerSupport says what computer use can do on this session, for
// Plugins → Your computer: the display, whether it is ready, what is
// missing, and what the window system holds.
func ComputerSupport() Support {
	wl := os.Getenv("WAYLAND_DISPLAY") != "" && !strings.EqualFold(os.Getenv(DisplayVariable), "x11")
	if wl {
		desk := os.Getenv("XDG_CURRENT_DESKTOP")
		s := Support{Session: "Wayland (" + strings.ReplaceAll(cmpOr(desk, "unknown desktop"), ":", ", ") + ")"}
		if !wayland.Supported() {
			s.Problem = "This build of Agentry Desktop cannot read a window's stream on Wayland."
			return s
		}
		rd, sc := portalVersions()
		switch {
		case sc == 0:
			s.Problem = "This desktop offers no screen sharing portal (org.freedesktop.portal.ScreenCast): install xdg-desktop-portal and your desktop's portal backend."
		case rd == 0:
			s.Problem = "This desktop offers no remote control portal (org.freedesktop.portal.RemoteDesktop), so Agentry Desktop cannot post input to a window here. Computer use is refused on it."
		default:
			s.Ready = true
		}
		if wayland.IsKDE(desk) {
			s.Holds = "Plasma shares only the window you pick when a run starts; its remote control is not held to a window, so Agentry Desktop checks with KWin before each input that the window is active and the point inside it."
		} else {
			s.Holds = "The desktop shares only the window you pick when a run starts and holds the pointer to it; keys go to the focused window."
		}
		return s
	}
	if os.Getenv("DISPLAY") == "" {
		return Support{Session: "no graphical session", Problem: "Agentry Desktop sees no graphical session (neither WAYLAND_DISPLAY nor DISPLAY is set)."}
	}
	return Support{Session: "X11", Ready: true, Holds: "X11 holds no input to a window: Agentry Desktop checks before each input that the application's window is focused and the point inside it, and captures that window alone."}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// RequestComputerPermissions has nothing to ask ahead of a run here: the
// desktop portal asks when a run starts.
func RequestComputerPermissions() Support { return ComputerSupport() }

// runningWindows reads the windows the X server shows, each with its
// WM_CLASS instance and class: on X11, and on Wayland the windows of
// applications that run through Xwayland. A Wayland application's own
// windows are not readable this way, and the person types its app id.
func runningWindows(ctx context.Context) ([]RunningWindow, error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, fmt.Errorf("this desktop's windows cannot be listed: type the application's identifier (its Wayland app id, such as org.kde.kdenlive)")
	}
	b, err := x11.Open(ctx, x11.Options{Scope: computeruse.ScopeApp, Match: func(instance, class string, _ int) (string, bool) {
		return instance + "\x00" + class, true
	}})
	if err != nil {
		return nil, fmt.Errorf("this display's windows cannot be listed (%v): type the application's identifier", err)
	}
	defer func() { _ = b.Close() }()
	ws, err := b.Windows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RunningWindow, 0, len(ws))
	for _, w := range ws {
		var ids []string
		for _, id := range strings.Split(w.App, "\x00") {
			if id != "" && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			out = append(out, RunningWindow{Title: w.Title, Identifiers: ids})
		}
	}
	if len(out) == 0 && os.Getenv("WAYLAND_DISPLAY") != "" {
		return nil, fmt.Errorf("no window Agentry Desktop can read is open: on Wayland, type the application's app id (such as org.kde.kdenlive)")
	}
	return out, nil
}
