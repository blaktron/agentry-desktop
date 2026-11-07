// Package stopkey binds the key that stops every run's computer use while
// one runs: on Wayland through the desktop portal's global shortcuts, which
// the desktop may ask the person to confirm, and on X11 by grabbing the key
// on the root window.
package stopkey

import "context"

// Trigger is the shortcut, in the portal's trigger syntax.
const Trigger = "CTRL+SHIFT+Escape"

// DesktopEntry is the name of the app's installed desktop entry
// (agentry-desktop.desktop), which the portal knows the app by.
const DesktopEntry = "agentry-desktop"

// Binding is a bound stop key; Close unbinds it.
type Binding interface {
	Close() error
}

// Bind binds the stop key and calls stop each time it is pressed, until
// the binding is closed. An error says the key could not be bound; the
// tray and the banner still stop.
func Bind(ctx context.Context, stop func()) (Binding, error) { return bind(ctx, stop) }
