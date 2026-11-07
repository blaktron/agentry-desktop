// Package quartz is the computer-use backend for macOS. It lists windows
// with Quartz's window list, captures one window with ScreenCaptureKit
// (macOS 14 and later; the older window image call before that), and posts
// input with Quartz events.
//
// ScreenCaptureKit captures exactly the window it is given, so the capture
// is held to the allowed window by the system. Input is not: a Quartz event
// goes to whatever is under the pointer or has the keyboard, so the check
// that an allowed window is frontmost and the point inside it and not under
// another window, before each input, is the caller's, with Covered beside
// it. Capturing needs Screen Recording and posting needs Accessibility, both
// granted by the person in System Settings; macOS asks for Screen Recording
// once and wants the app restarted after.
package quartz

/*
#cgo CFLAGS: -fobjc-arc -x objective-c
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include <stdlib.h>
#include "quartz_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// screenID is the one window of screen scope.
const screenID = "screen"

// Options configures Open.
type Options struct {
	// Scope is computeruse.ScopeApp or computeruse.ScopeScreen.
	Scope string
	// Match says whether a window's application is one the run may use,
	// and its name, from the application's bundle identifier
	// ("org.blenderfoundation.blender") and its process name. App scope
	// only.
	Match func(bundle, owner string) (app string, ok bool)
}

// Permissions are the privacy permissions computer use needs here.
type Permissions struct {
	// Screen is Screen Recording, which a capture needs.
	Screen bool
	// Accessibility is what posting input and raising a window need.
	Accessibility bool
}

// Granted reads the two permissions as they are now.
func Granted() Permissions {
	return Permissions{Screen: C.qz_screen_granted() != 0, Accessibility: C.qz_ax_granted() != 0}
}

// Request asks macOS for whichever permission is missing, through its own
// prompts: once for Screen Recording (later asks open nothing; the person
// turns it on in System Settings), and the Accessibility prompt that points
// to System Settings.
func Request() Permissions {
	p := Granted()
	if !p.Screen {
		C.qz_request_screen()
	}
	if !p.Accessibility {
		C.qz_request_ax()
	}
	return Granted()
}

// CaptureKit says whether captures go through ScreenCaptureKit.
func CaptureKit() bool { return C.qz_capture_kit() != 0 }

// Backend implements computeruse.Backend on macOS.
type Backend struct {
	mu  sync.Mutex
	opt Options
	// held are the modifiers pressed and not released: their flags go on
	// every event, and a character pressed under them is its key.
	held map[uint32]bool
	// px and py are where the last Move put the pointer, in the display
	// space, and moved says one has: a press or a scroll is posted there,
	// and refused before any move.
	px, py float64
	moved  bool
}

var (
	_ computeruse.Backend = (*Backend)(nil)
	_ computeruse.Coverer = (*Backend)(nil)
)

// Open opens computer use for one binding. It refuses with
// computeruse.ErrNotGranted when Screen Recording or Accessibility is not
// allowed for Agentry Desktop.
func Open(ctx context.Context, opt Options) (*Backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opt.Scope != computeruse.ScopeApp && opt.Scope != computeruse.ScopeScreen {
		return nil, fmt.Errorf("quartz: unknown scope %q", opt.Scope)
	}
	if opt.Scope == computeruse.ScopeApp && opt.Match == nil {
		return nil, errors.New("quartz: app scope needs a Match function")
	}
	if err := Missing(Granted()); err != nil {
		return nil, err
	}
	return &Backend{opt: opt, held: map[uint32]bool{}}, nil
}

// Missing is the refusal for permissions not granted, nil when both are.
func Missing(p Permissions) error {
	switch {
	case !p.Screen && !p.Accessibility:
		return fmt.Errorf("%w: macOS has not allowed Agentry Desktop Screen Recording or Accessibility: turn both on in System Settings, Privacy & Security, then quit and reopen Agentry Desktop", computeruse.ErrNotGranted)
	case !p.Screen:
		return fmt.Errorf("%w: macOS has not allowed Agentry Desktop Screen Recording: turn it on in System Settings, Privacy & Security, Screen & System Audio Recording, then quit and reopen Agentry Desktop", computeruse.ErrNotGranted)
	case !p.Accessibility:
		return fmt.Errorf("%w: macOS has not allowed Agentry Desktop Accessibility, which posting a click or a key needs: turn it on in System Settings, Privacy & Security, Accessibility", computeruse.ErrNotGranted)
	}
	return nil
}

// Session names the display.
func (b *Backend) Session() string { return "macOS" }

// Enforced says what macOS holds of the scope.
func (b *Backend) Enforced() string {
	if b.opt.Scope == computeruse.ScopeScreen {
		return "the main display; macOS holds no input to a window, and in screen scope nothing is checked"
	}
	how := "ScreenCaptureKit captures the allowed window alone"
	if !CaptureKit() {
		how = "macOS's window image call captures the allowed window alone"
	}
	return how + "; macOS holds no input to a window, so Agentry Desktop checks before each input that an allowed window is frontmost, the point inside it and no other window above it there"
}

type win struct {
	id                 uint32
	pid                int32
	app, title, bundle string
	x, y, w, h         float64
	scale              float64
	layer              int32
	alpha              float64
}

func (w win) size() image.Point {
	return image.Pt(int(math.Round(w.w*w.scale)), int(math.Round(w.h*w.scale)))
}

// all is every window on screen, front to back.
func all() ([]win, error) {
	buf := make([]C.qz_win, 256)
	for {
		n := int(C.qz_windows(&buf[0], C.int(len(buf))))
		if n < 0 {
			return nil, errors.New("macOS did not list the windows on screen")
		}
		if n > len(buf) {
			buf = make([]C.qz_win, n+32)
			continue
		}
		out := make([]win, 0, n)
		for i := range buf[:n] {
			c := &buf[i]
			out = append(out, win{id: uint32(c.id), pid: int32(c.pid),
				app: C.GoString(&c.owner[0]), title: C.GoString(&c.title[0]), bundle: C.GoString(&c.bundle[0]),
				x: float64(c.x), y: float64(c.y), w: float64(c.w), h: float64(c.h), layer: int32(c.layer), alpha: float64(c.alpha)})
		}
		return out, nil
	}
}

// allowed are the windows the scope allows, front to back, each with its
// display's scale.
func (b *Backend) allowed() ([]win, []win, error) {
	ws, err := all()
	if err != nil {
		return nil, nil, err
	}
	var out []win
	for _, w := range ws {
		if w.layer != 0 || w.w < 1 || w.h < 1 || w.alpha <= 0 {
			continue
		}
		app, ok := b.opt.Match(w.bundle, w.app)
		if !ok {
			continue
		}
		w.app = app
		w.scale = float64(C.qz_scale(C.double(w.x), C.double(w.y), C.double(w.w), C.double(w.h)))
		if w.scale <= 0 {
			w.scale = 1
		}
		out = append(out, w)
	}
	return out, ws, nil
}

func (w win) idString() string { return strconv.FormatUint(uint64(w.id), 10) }

func mainDisplay() win {
	var x, y, w, h C.double
	C.qz_main_display(&x, &y, &w, &h)
	d := win{x: float64(x), y: float64(y), w: float64(w), h: float64(h)}
	d.scale = float64(C.qz_scale(x, y, w, h))
	if d.scale <= 0 {
		d.scale = 1
	}
	return d
}

// Windows lists the windows the scope allows. The frontmost window of the
// frontmost application is the focused one.
func (b *Backend) Windows(ctx context.Context) ([]computeruse.Window, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.opt.Scope == computeruse.ScopeScreen {
		return []computeruse.Window{{ID: screenID, App: "the screen", Size: mainDisplay().size(), Focused: true}}, nil
	}
	ws, everything, err := b.allowed()
	if err != nil {
		return nil, err
	}
	front := frontWindow(everything, int32(C.qz_frontmost_pid()))
	out := make([]computeruse.Window, 0, len(ws))
	for _, w := range ws {
		out = append(out, computeruse.Window{ID: w.idString(), App: w.app, Title: w.title, Size: w.size(), Focused: w.id == front})
	}
	return out, nil
}

// frontWindow is the id of the frontmost application's front window,
// passing over the tiny or invisible helper windows an application keeps.
func frontWindow(ws []win, pid int32) uint32 {
	for _, w := range ws {
		if w.pid == pid && w.layer == 0 && w.alpha > 0 && w.w >= 2 && w.h >= 2 {
			return w.id
		}
	}
	return 0
}

func (b *Backend) find(id string) (win, []win, error) {
	if b.opt.Scope == computeruse.ScopeScreen {
		if id != screenID {
			return win{}, nil, computeruse.ErrNoWindow
		}
		return mainDisplay(), nil, nil
	}
	ws, everything, err := b.allowed()
	if err != nil {
		return win{}, nil, err
	}
	for _, w := range ws {
		if w.idString() == id {
			return w, everything, nil
		}
	}
	return win{}, nil, computeruse.ErrNoWindow
}

// Focus brings the window's application to the front and raises the
// window.
func (b *Backend) Focus(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, _, err := b.find(id)
	if err != nil || b.opt.Scope == computeruse.ScopeScreen {
		return err
	}
	if C.qz_activate(C.int32_t(w.pid), C.double(w.x), C.double(w.y), C.double(w.w), C.double(w.h)) != 0 {
		return fmt.Errorf("macOS did not bring %s to the front", w.app)
	}
	return nil
}

// Capture is the window's image at its size in pixels.
func (b *Backend) Capture(ctx context.Context, id string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The window is found under the lock; the capture, which may wait on
	// ScreenCaptureKit, runs without it, so a stop is not held behind it.
	b.mu.Lock()
	w, _, err := b.find(id)
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}
	sz := w.size()
	var buf *C.uint8_t
	var ow, oh C.int
	errBuf := make([]byte, 512)
	var rc C.int
	if id == screenID {
		rc = C.qz_capture_display(C.int(sz.X), C.int(sz.Y), &buf, &ow, &oh, (*C.char)(unsafe.Pointer(&errBuf[0])), C.int(len(errBuf)))
	} else {
		rc = C.qz_capture_window(C.uint32_t(w.id), C.int(sz.X), C.int(sz.Y), &buf, &ow, &oh, (*C.char)(unsafe.Pointer(&errBuf[0])), C.int(len(errBuf)))
	}
	if rc != 0 {
		msg := C.GoString((*C.char)(unsafe.Pointer(&errBuf[0])))
		if !Granted().Screen {
			return nil, fmt.Errorf("%w: %s", computeruse.ErrNotGranted, msg)
		}
		return nil, fmt.Errorf("the capture failed: %s", msg)
	}
	defer C.qz_free(unsafe.Pointer(buf))
	width, height := int(ow), int(oh)
	img := image.NewRGBA(image.Rect(0, 0, sz.X, sz.Y))
	src := unsafe.Slice((*byte)(unsafe.Pointer(buf)), width*height*4)
	// The capture is drawn at the size asked; a display that changed scale
	// between the list and the capture is clipped to the window's size.
	for y := 0; y < min(height, sz.Y); y++ {
		copy(img.Pix[y*img.Stride:y*img.Stride+min(width, sz.X)*4], src[y*width*4:y*width*4+min(width, sz.X)*4])
	}
	return img, nil
}

// global is a window position in pixels as a point of the display space.
func (w win) global(p image.Point) (float64, float64) {
	return w.x + float64(p.X)/w.scale, w.y + float64(p.Y)/w.scale
}

// Cursor is the pointer's position in the window, in its pixels.
func (b *Backend) Cursor(ctx context.Context, id string) (image.Point, bool, error) {
	if err := ctx.Err(); err != nil {
		return image.Point{}, false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, _, err := b.find(id)
	if err != nil {
		return image.Point{}, false, err
	}
	var x, y C.double
	if C.qz_cursor(&x, &y) != 0 {
		return image.Point{}, false, errors.New("macOS did not say where the pointer is")
	}
	p := image.Pt(int(math.Floor((float64(x)-w.x)*w.scale)), int(math.Floor((float64(y)-w.y)*w.scale)))
	sz := w.size()
	return p, p.X >= 0 && p.Y >= 0 && p.X < sz.X && p.Y < sz.Y, nil
}

// Move puts the pointer at a position in the window.
func (b *Backend) Move(ctx context.Context, id string, p image.Point) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, _, err := b.find(id)
	if err != nil {
		return err
	}
	sz := w.size()
	if p.X < 0 || p.Y < 0 || p.X >= sz.X || p.Y >= sz.Y {
		return fmt.Errorf("the point %v is outside the window", p)
	}
	x, y := w.global(p)
	if C.qz_move(C.double(x), C.double(y), C.uint64_t(b.flags())) != 0 {
		return errors.New("macOS did not move the pointer")
	}
	b.px, b.py, b.moved = x, y, true
	return nil
}

// Covered says another window is above the allowed one at the point: one
// earlier in the front-to-back list, visible, whose bounds hold it.
func (b *Backend) Covered(ctx context.Context, id string, p image.Point) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, everything, err := b.find(id)
	if err != nil || id == screenID {
		return false, err
	}
	x, y := w.global(p)
	return coveredAt(everything, w.id, x, y), nil
}

// coveredAt reports a visible window in front of id whose bounds hold the
// point.
func coveredAt(ws []win, id uint32, x, y float64) bool {
	for _, o := range ws {
		if o.id == id {
			return false
		}
		if o.alpha > 0 && o.layer >= 0 && x >= o.x && x < o.x+o.w && y >= o.y && y < o.y+o.h {
			return true
		}
	}
	return true
}

// clickInterval is how soon a press at the same place counts as the next
// click of a double or triple click.
const clickInterval = 500 * time.Millisecond

// clicks counts a press at the same place soon after another as a double
// or triple click, as macOS wants it said on the event.
var clicks struct {
	sync.Mutex
	n    int
	x, y float64
	b    computeruse.Button
	at   time.Time
}

// Button presses or releases a button where the pointer is.
func (b *Backend) Button(ctx context.Context, btn computeruse.Button, down bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	x, y, moved := b.px, b.py, b.moved
	b.mu.Unlock()
	if !moved {
		return errors.New("the pointer was not placed in the window before the press, so it was not sent")
	}
	clicks.Lock()
	if down {
		if clicks.b == btn && clicks.x == x && clicks.y == y && clicks.n < 3 && time.Since(clicks.at) < clickInterval {
			clicks.n++
		} else {
			clicks.n, clicks.x, clicks.y, clicks.b = 1, x, y, btn
		}
		clicks.at = time.Now()
	}
	n := clicks.n
	clicks.Unlock()
	b.mu.Lock()
	flags := b.flags()
	b.mu.Unlock()
	if C.qz_button(C.int(btn), boolInt(down), C.double(x), C.double(y), C.uint64_t(flags), C.int(n)) != 0 {
		return fmt.Errorf("macOS did not take the button %d", btn)
	}
	return nil
}

func boolInt(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

// Scroll scrolls by lines where the pointer is.
func (b *Backend) Scroll(ctx context.Context, d computeruse.Direction, steps int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var dy, dx int
	switch d {
	case computeruse.ScrollUp:
		dy = 1
	case computeruse.ScrollDown:
		dy = -1
	case computeruse.ScrollLeft:
		dx = 1
	case computeruse.ScrollRight:
		dx = -1
	default:
		return fmt.Errorf("no such direction %q", d)
	}
	b.mu.Lock()
	x, y, moved, flags := b.px, b.py, b.moved, b.flags()
	b.mu.Unlock()
	if !moved {
		return errors.New("the pointer was not placed in the window before the scroll, so it was not sent")
	}
	for i := 0; i < steps; i++ {
		if C.qz_scroll(C.int32_t(dy), C.int32_t(dx), C.double(x), C.double(y), C.uint64_t(flags)) != 0 {
			return errors.New("macOS did not take the scroll")
		}
	}
	return nil
}

// Quartz's modifier flags.
const (
	flagShift   = 0x00020000
	flagControl = 0x00040000
	flagAlt     = 0x00080000
	flagCommand = 0x00100000
)

// flags are the held modifiers as Quartz's event flags.
func (b *Backend) flags() uint64 {
	var f uint64
	for k := range b.held {
		switch computeruse.ModifierOf(k) {
		case computeruse.ModShift:
			f |= flagShift
		case computeruse.ModControl:
			f |= flagControl
		case computeruse.ModAlt:
			f |= flagAlt
		case computeruse.ModSuper:
			f |= flagCommand
		}
	}
	return f
}

// Key presses or releases a key. A named key or a modifier is its key code;
// a character is typed as itself unless a modifier is held, when it is the
// US layout's key for it so the combination reaches the application.
func (b *Backend) Key(ctx context.Context, keysym uint32, down bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	combo := len(b.held) > 0
	if computeruse.IsModifier(keysym) {
		if down {
			b.held[keysym] = true
		} else {
			delete(b.held, keysym)
		}
	}
	flags := b.flags()
	b.mu.Unlock()
	if code, ok := computeruse.MacKeyOf(keysym, combo); ok {
		if C.qz_key(C.uint16_t(code), boolInt(down), C.uint64_t(flags)) != 0 {
			return errors.New("macOS did not take the key")
		}
		return nil
	}
	r, ok := computeruse.KeysymRune(keysym)
	if !ok {
		return fmt.Errorf("the key %#x has no key on macOS", keysym)
	}
	if C.qz_unicode(C.uint32_t(r), boolInt(down), C.uint64_t(flags)) != 0 {
		return errors.New("macOS did not take the character")
	}
	return nil
}

// Close releases every modifier the binding still holds.
func (b *Backend) Close() error {
	b.mu.Lock()
	held := b.held
	b.held = map[uint32]bool{}
	b.mu.Unlock()
	for k := range held {
		if code, ok := computeruse.MacKeyOf(k, false); ok {
			C.qz_key(C.uint16_t(code), 0, 0)
		}
	}
	return nil
}
