//go:build linux

package app

import (
	"context"
	"os"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/blaktron/agentry-desktop/internal/computeruse/kwin"
	"github.com/blaktron/agentry-desktop/internal/computeruse/wayland"
)

// keepBannerAbove asks Plasma's window manager to keep the banner above
// the other windows, which a Wayland client cannot ask for itself; on any
// other desktop the window's own always-on-top stands.
func keepBannerAbove(maxHeight int) {
	if os.Getenv("WAYLAND_DISPLAY") == "" || !wayland.IsKDE(os.Getenv("XDG_CURRENT_DESKTOP")) {
		return
	}
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer func() { _ = bus.Close() }()
	if !kwin.Available(bus) {
		return
	}
	dir := kwin.ScriptDir()
	// The window maps a moment after it is made.
	for range 10 {
		time.Sleep(300 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = kwin.KeepAbove(ctx, bus, dir, maxHeight)
		cancel()
	}
}
