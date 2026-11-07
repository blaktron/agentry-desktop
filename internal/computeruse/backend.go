// Package computeruse answers a run's computer-use tools on the person's
// computer: it sees and acts only in the windows the scope allows. In app
// scope those are the windows of the applications the run bound; in screen
// scope, the whole display. A Backend is one window system's capture and
// input; this package holds what is the same on every one: the tools'
// arguments, the scale between a screenshot and its window, the check
// before each input that the window is focused and the point inside it,
// and the screenshots the person asked to keep.
package computeruse

import (
	"context"
	"errors"
	"image"
)

// Scopes, as a run declares them.
const (
	ScopeApp    = "app"
	ScopeScreen = "screen"
)

// Window is one window the scope allows, as a backend sees it.
type Window struct {
	// ID names the window to the backend and to the agent, stable while
	// the window lives.
	ID string
	// App is the application the window belongs to, as people know it.
	App string
	// Title is the window's title, or empty where the window system does
	// not say.
	Title string
	// Size is the window's own size in its pixels: what a capture of it
	// returns.
	Size image.Point
	// Focused says the window has the keyboard and is the one an input
	// goes to. A backend whose window system holds the input to the window
	// itself reports every window as focused.
	Focused bool
}

// Button is a mouse button.
type Button int

// The buttons.
const (
	ButtonLeft Button = iota + 1
	ButtonMiddle
	ButtonRight
)

// Direction is a scroll's direction.
type Direction string

// The directions.
const (
	ScrollUp    Direction = "up"
	ScrollDown  Direction = "down"
	ScrollLeft  Direction = "left"
	ScrollRight Direction = "right"
)

// Backend is one window system's capture and input, for one binding. Every
// position is in the window's own pixels (Window.Size). A backend refuses
// what its window system cannot do with ErrNotGranted or another error;
// the checks of scope are this package's, and a backend adds its own where
// the window system gives one.
type Backend interface {
	// Session names the display the backend works on, for the receipt:
	// "Linux, X11", "Linux, Wayland (KDE Plasma)".
	Session() string
	// Enforced says, for the receipt, which of the scope the window system
	// holds and which only Agentry Desktop's check before each input does.
	Enforced() string
	// Windows are the windows the scope allows, as they are now.
	Windows(ctx context.Context) ([]Window, error)
	// Focus brings the window to the front and gives it the keyboard.
	Focus(ctx context.Context, id string) error
	// Capture is the window's image at its own size.
	Capture(ctx context.Context, id string) (image.Image, error)
	// Cursor is the pointer's position in the window, and whether it is
	// inside it.
	Cursor(ctx context.Context, id string) (image.Point, bool, error)
	// Move puts the pointer at a position in the window.
	Move(ctx context.Context, id string, p image.Point) error
	// Button presses or releases a button where the pointer is.
	Button(ctx context.Context, b Button, down bool) error
	// Scroll scrolls by steps of the wheel where the pointer is.
	Scroll(ctx context.Context, d Direction, steps int) error
	// Key presses or releases the key that types the X keysym.
	Key(ctx context.Context, keysym uint32, down bool) error
	// Close ends what the backend holds for the binding.
	Close() error
}

// Coverer is a backend that can say another window is above an allowed
// one at a point, so an input there would land in that window instead.
type Coverer interface {
	Covered(ctx context.Context, id string, p image.Point) (bool, error)
}

// ErrNotGranted says the window system has not granted what the scope
// needs, or cannot: the bind is refused desktop-not-granted.
var ErrNotGranted = errors.New("the window system has not granted it")

// ErrNoWindow says the scope allows no window now: the application is
// closed or none of its windows is mapped.
var ErrNoWindow = errors.New("no window the run may use is open")
