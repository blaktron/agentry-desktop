//go:build linux

// Package wayland is computer use on a Wayland desktop, through the desktop
// portal: the ScreenCast portal shares the window the person picks (or the
// display, in screen scope) as a PipeWire stream, and the RemoteDesktop
// portal posts the input. How much of app scope the compositor holds
// differs by desktop, and the backend says which in its Enforced line:
//
//   - On a compositor whose remote control shares one window, the pointer
//     is posted absolute within that window's stream, so the compositor
//     holds both the capture and the pointer to the window; keys go to the
//     focused window, which a click in it makes.
//   - On KDE Plasma, whose remote control shares the whole screen, the
//     capture is still the one window the person shared, and each input is
//     posted only after KWin's own account (package kwin) says that window
//     is active and the point is inside it.
//
// A compositor without the RemoteDesktop portal is refused.
package wayland

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
	"github.com/blaktron/agentry-desktop/internal/computeruse/kwin"
	"github.com/blaktron/agentry-desktop/internal/computeruse/pipewire"
	"github.com/blaktron/agentry-desktop/internal/computeruse/portal"
)

// App is an application the run may use: its name and the window classes
// (Wayland app ids) its windows carry.
type App struct {
	Name    string
	Classes []string
}

// Tokens keeps the portals' restore tokens between sessions.
type Tokens interface {
	Get(key string) string
	Set(key, token string)
}

// Options shape a backend.
type Options struct {
	Scope string
	// Apps are the applications the run bound; empty in app scope means
	// the window the person picks, whatever it is.
	Apps []App
	// Tokens keeps the restore tokens; nil keeps none.
	Tokens Tokens
	// ScriptDir is where KWin's script is written while it is loaded.
	ScriptDir string
	// Desktop is the desktop's name ($XDG_CURRENT_DESKTOP); "" reads it.
	Desktop string
}

type mode int

const (
	modeHeld   mode = iota // the compositor holds the pointer to the shared stream
	modeKWin               // KWin's account checks each input
	modeScreen             // the whole display
)

// Backend is computer use through the portals for one binding.
type Backend struct {
	opt     Options
	desktop string
	mode    mode
	conn    *portal.Conn
	capture *portal.Session
	input   *portal.Session
	stream  *pipewire.Stream
	node    uint32
	// inputNode is the stream absolute motion is relative to.
	inputNode uint32
	tracker   *kwin.Tracker

	mu sync.Mutex
	// win is the KWin window the shared stream shows, and origin its
	// place in the compositor's space (modeKWin).
	win    string
	origin image.Point
	app    string
	size   image.Point
	closed bool
	// client says the shared stream shows the window's client area rather
	// than its frame (modeKWin).
	client bool
	// clicked says a click was posted in the shared window: on a
	// compositor that holds the pointer to it, keys go to it only once a
	// click there has focused it (modeHeld).
	clicked bool
	// input is when the last input was posted: a capture soon after waits
	// for a picture the window drew since.
	lastInput time.Time
}

var _ computeruse.Backend = (*Backend)(nil)

// IsKDE says the desktop is KDE Plasma.
func IsKDE(desktop string) bool {
	for _, d := range strings.Split(desktop, ":") {
		if strings.EqualFold(strings.TrimSpace(d), "KDE") {
			return true
		}
	}
	return false
}

// Open starts the portal sessions for a binding: the person is asked
// through the portal's own dialogs unless a restore token answers.
func Open(ctx context.Context, opt Options) (*Backend, error) {
	if opt.Desktop == "" {
		opt.Desktop = os.Getenv("XDG_CURRENT_DESKTOP")
	}
	desk := opt.Desktop
	if desk == "" {
		desk = "this desktop"
	}
	if !pipewire.Available() {
		return nil, fmt.Errorf("%w: this build of Agentry Desktop cannot read a window's stream on Wayland", computeruse.ErrNotGranted)
	}
	conn, err := portal.Connect()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", computeruse.ErrNotGranted, err)
	}
	b := &Backend{opt: opt, desktop: desk, conn: conn}
	ok := false
	defer func() {
		if !ok {
			_ = b.Close()
		}
	}()
	if conn.Version("org.freedesktop.portal.RemoteDesktop") == 0 {
		return nil, fmt.Errorf("%w: %s offers no remote control portal (org.freedesktop.portal.RemoteDesktop), so Agentry Desktop cannot post input to a window there", computeruse.ErrNotGranted, desk)
	}
	if conn.Version("org.freedesktop.portal.ScreenCast") == 0 {
		return nil, fmt.Errorf("%w: %s offers no screen sharing portal (org.freedesktop.portal.ScreenCast)", computeruse.ErrNotGranted, desk)
	}
	switch {
	case opt.Scope == computeruse.ScopeScreen:
		b.mode = modeScreen
		err = b.openShared(ctx, portal.SourceMonitor, "remotedesktop-monitor")
	case IsKDE(opt.Desktop):
		b.mode = modeKWin
		err = b.openKWin(ctx)
	default:
		b.mode = modeHeld
		err = b.openShared(ctx, portal.SourceWindow, "remotedesktop-window")
	}
	if err != nil {
		return nil, err
	}
	ok = true
	return b, nil
}

func (b *Backend) token(key string) string {
	if b.opt.Tokens == nil {
		return ""
	}
	return b.opt.Tokens.Get(key)
}

func (b *Backend) keep(key, tok string) {
	if b.opt.Tokens != nil && tok != "" {
		b.opt.Tokens.Set(key, tok)
	}
}

func notGranted(err error, what string) error {
	if errors.Is(err, portal.ErrCancelled) {
		return fmt.Errorf("%w: %s was not allowed in the portal's dialog", computeruse.ErrNotGranted, what)
	}
	return fmt.Errorf("%w: %s: %v", computeruse.ErrNotGranted, what, err)
}

// openShared starts one RemoteDesktop session that shares a stream too:
// absolute motion is posted within that stream.
func (b *Backend) openShared(ctx context.Context, source uint32, key string) error {
	s, err := b.conn.RemoteDesktop(ctx, portal.RemoteOptions{Restore: b.token(key), Share: portal.ShareOptions{Types: source, CursorHidden: true}})
	if err != nil {
		return notGranted(err, "sharing and controlling the window")
	}
	b.input, b.capture = s, s
	b.keep(key, s.Restore)
	streams := s.Streams()
	if len(streams) != 1 {
		return fmt.Errorf("%w: the portal shared %d streams, and computer use takes one", computeruse.ErrNotGranted, len(streams))
	}
	st := streams[0]
	if source == portal.SourceWindow && st.Source != portal.SourceWindow {
		return fmt.Errorf("%w: %s's remote control shared the whole screen rather than one window, so Agentry Desktop cannot hold the input to a window there", computeruse.ErrNotGranted, b.desktop)
	}
	b.node, b.inputNode = st.Node, st.Node
	return b.openStream(ctx)
}

// openKWin shares the window the person picks for the capture, starts the
// remote control for the input alone, and follows KWin's account of the
// windows to check each input.
func (b *Backend) openKWin(ctx context.Context) error {
	key := b.windowKey()
	sc, err := b.conn.ScreenCast(ctx, portal.ShareOptions{Types: portal.SourceWindow, Restore: b.token(key), CursorHidden: true})
	if err != nil {
		return notGranted(err, "sharing the window")
	}
	b.capture = sc
	b.keep(key, sc.Restore)
	streams := sc.Streams()
	if len(streams) != 1 {
		return fmt.Errorf("%w: the portal shared %d windows, and computer use takes one", computeruse.ErrNotGranted, len(streams))
	}
	b.node = streams[0].Node
	rd, err := b.conn.RemoteDesktop(ctx, portal.RemoteOptions{Restore: b.token("remotedesktop")})
	if err != nil {
		return notGranted(err, "controlling the pointer and the keyboard")
	}
	b.input = rd
	b.keep("remotedesktop", rd.Restore)
	if err := b.openStream(ctx); err != nil {
		return err
	}
	t, err := kwin.Start(ctx, b.conn.Bus(), b.opt.ScriptDir)
	if err != nil {
		return fmt.Errorf("%w: %v", computeruse.ErrNotGranted, err)
	}
	b.tracker = t
	return b.matchWindow(t.State())
}

func (b *Backend) openStream(ctx context.Context) error {
	fd, err := b.capture.PipeWire(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", computeruse.ErrNotGranted, err)
	}
	s, err := pipewire.Open(fd, b.node)
	if err != nil {
		return fmt.Errorf("%w: the shared stream: %v", computeruse.ErrNotGranted, err)
	}
	b.stream = s
	fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	img, err := s.Frame(fctx, false)
	if err != nil {
		return fmt.Errorf("%w: the shared stream sent no picture: %v", computeruse.ErrNotGranted, err)
	}
	b.size = img.Bounds().Size()
	return nil
}

// windowKey is the restore token's key for the window share of the bound
// applications: a token kept for one application's window is never offered
// for another's.
func (b *Backend) windowKey() string {
	var classes []string
	for _, a := range b.opt.Apps {
		classes = append(classes, a.Classes...)
	}
	slices.Sort(classes)
	return "screencast-window:" + strings.Join(classes, ",")
}

// matchWindow finds the KWin window the shared stream shows: the one window
// of the bound applications whose frame or client area has the stream's
// size. The portal does not say which window it shared, so where another
// window has that size too, which one the person shared cannot be told,
// and the bind is refused rather than guessed; and a stream that shows no
// such window, because the person shared another one, is refused.
func (b *Backend) matchWindow(s kwin.State) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var match *kwin.Window
	var app string
	client := false
	others := 0
	for i, w := range s.Windows {
		sized := b.size == w.Frame.Size() || b.size == w.Client.Size()
		if !sized {
			continue
		}
		a, ok := b.appOf(w)
		if !ok || match != nil {
			others++
			continue
		}
		match, app, client = &s.Windows[i], a, b.size != w.Frame.Size()
	}
	if match != nil && others > 0 {
		return fmt.Errorf("%w: another open window has the same size as %s's, so Agentry Desktop cannot tell which one was shared: resize or minimise the other window and run again", computeruse.ErrNotGranted, app)
	}
	if match != nil {
		b.win, b.app, b.client = match.ID, app, client
		b.origin = match.Frame.Min
		if client {
			b.origin = match.Client.Min
		}
		return nil
	}
	names := make([]string, 0, len(b.opt.Apps))
	for _, a := range b.opt.Apps {
		names = append(names, a.Name)
	}
	if len(names) == 0 {
		return fmt.Errorf("%w: Agentry Desktop could not find the shared window among KWin's windows", computeruse.ErrNotGranted)
	}
	return fmt.Errorf("%w: the window shared in the portal's dialog is not %s's: run again and pick %s's window", computeruse.ErrNotGranted, strings.Join(names, " or "), names[0])
}

// appOf is the bound application a KWin window belongs to.
func (b *Backend) appOf(w kwin.Window) (string, bool) {
	if len(b.opt.Apps) == 0 {
		return w.Class, true
	}
	for _, a := range b.opt.Apps {
		for _, c := range a.Classes {
			if strings.EqualFold(c, w.Class) || strings.EqualFold(c, w.Name) {
				return a.Name, true
			}
		}
	}
	return "", false
}

// Session names the display.
func (b *Backend) Session() string {
	if IsKDE(b.opt.Desktop) {
		return "Linux, Wayland (KDE Plasma)"
	}
	return "Linux, Wayland (" + b.desktop + ")"
}

// Enforced says what the compositor holds.
func (b *Backend) Enforced() string {
	switch b.mode {
	case modeScreen:
		return "the whole display, shared through the desktop portal; nothing is held to a window in screen scope"
	case modeKWin:
		return "the capture is held by Plasma to the window shared in the portal's dialog; Plasma's remote control is not held to a window, so Agentry Desktop checks with KWin before each input that the window is active and the point inside it"
	}
	return "the capture and the pointer are held by the compositor to the window shared in the portal's dialog; keys go to the focused window, so Agentry Desktop sends them only after a click in the shared window, which focuses it, and the person can focus another window between"
}

// Windows is the shared window, or the display.
func (b *Backend) Windows(ctx context.Context) ([]computeruse.Window, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.mode {
	case modeScreen:
		return []computeruse.Window{{ID: "screen", App: "the display", Size: b.size, Focused: true}}, nil
	case modeHeld:
		app := "the shared window"
		if len(b.opt.Apps) == 1 {
			app = b.opt.Apps[0].Name
		}
		// The compositor holds the pointer to the window; the keyboard goes
		// to it once a click there has focused it.
		return []computeruse.Window{{ID: "shared", App: app, Size: b.size, Focused: b.clicked}}, nil
	}
	if !b.tracker.Alive() {
		return nil, fmt.Errorf("%w: KWin restarted, so Agentry Desktop no longer knows where the window is", computeruse.ErrNotGranted)
	}
	s := b.tracker.State()
	for _, w := range s.Windows {
		if w.ID != b.win {
			continue
		}
		if w.Minimized {
			return nil, nil
		}
		// The window moves and resizes; its place and size are read again
		// on every account, the stream following it.
		r := w.Frame
		if b.client {
			r = w.Client
		}
		b.origin, b.size = r.Min, r.Size()
		return []computeruse.Window{{ID: w.ID, App: b.app, Title: w.Caption, Size: b.size, Focused: w.Active}}, nil
	}
	return nil, nil
}

// Covered says another window is above the shared one at the point, by
// KWin's stacking order (modeKWin); elsewhere the compositor posts the
// pointer within the window's own stream and nothing covers it.
func (b *Backend) Covered(ctx context.Context, id string, p image.Point) (bool, error) {
	if b.mode != modeKWin {
		return false, nil
	}
	if _, err := b.Windows(ctx); err != nil {
		return false, err
	}
	b.mu.Lock()
	at := b.origin.Add(p)
	win := b.win
	b.mu.Unlock()
	s := b.tracker.State()
	stack := -1
	for _, w := range s.Windows {
		if w.ID == win {
			stack = w.Stack
		}
	}
	for _, w := range s.Windows {
		if w.ID == win || w.Minimized || w.Stack <= stack {
			continue
		}
		if at.In(w.Frame) {
			return true, nil
		}
	}
	return false, nil
}

func (b *Backend) allowed(id string) error {
	ws, err := b.Windows(context.Background())
	if err != nil {
		return err
	}
	for _, w := range ws {
		if w.ID == id {
			return nil
		}
	}
	return computeruse.ErrNoWindow
}

// Focus brings the window to the front.
func (b *Backend) Focus(ctx context.Context, id string) error {
	if err := b.allowed(id); err != nil {
		return err
	}
	if b.mode != modeKWin {
		return nil
	}
	return b.tracker.Activate(ctx, id)
}

// Capture is the shared stream's picture.
func (b *Backend) Capture(ctx context.Context, id string) (image.Image, error) {
	if err := b.allowed(id); err != nil {
		return nil, err
	}
	wait := 150 * time.Millisecond
	b.mu.Lock()
	if time.Since(b.lastInput) < 2*time.Second {
		wait = time.Second
	}
	b.mu.Unlock()
	fctx, cancel := context.WithTimeout(ctx, wait)
	img, err := b.stream.Frame(fctx, true)
	cancel()
	if err != nil {
		// A window that has not changed sends no new picture: the newest
		// one is what it shows.
		fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if img, err = b.stream.Frame(fctx, false); err != nil {
			return nil, err
		}
	}
	b.mu.Lock()
	b.size = img.Bounds().Size()
	b.mu.Unlock()
	return img, nil
}

// Cursor is the pointer's place in the window.
func (b *Backend) Cursor(ctx context.Context, id string) (image.Point, bool, error) {
	if err := b.allowed(id); err != nil {
		return image.Point{}, false, err
	}
	if b.mode != modeKWin {
		return image.Point{}, false, errors.New("this desktop does not say where the pointer is")
	}
	b.mu.Lock()
	origin, size := b.origin, b.size
	b.mu.Unlock()
	p := b.tracker.State().Cursor.Sub(origin)
	return p, p.In(image.Rectangle{Max: size}), nil
}

// Move puts the pointer at a point of the window.
func (b *Backend) Move(ctx context.Context, id string, p image.Point) error {
	if err := b.allowed(id); err != nil {
		return err
	}
	b.mu.Lock()
	origin, size := b.origin, b.size
	b.mu.Unlock()
	if !p.In(image.Rectangle{Max: size}) {
		return fmt.Errorf("the point [%d, %d] is outside the window", p.X, p.Y)
	}
	if b.mode != modeKWin {
		return b.input.MotionAbsolute(ctx, b.inputNode, float64(p.X)+0.5, float64(p.Y)+0.5)
	}
	// Plasma's remote control takes no absolute motion without a stream
	// of its own session, so the pointer is moved by the distance to the
	// target and read back from KWin until it is there.
	want := origin.Add(p)
	for range 4 {
		at := b.tracker.State().Cursor
		if at == want {
			return nil
		}
		d := want.Sub(at)
		if err := b.input.Motion(ctx, float64(d.X), float64(d.Y)); err != nil {
			return err
		}
		wctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		_, _ = b.tracker.Wait(wctx, func(s kwin.State) bool { return s.Cursor == want })
		cancel()
	}
	if b.tracker.State().Cursor != want {
		return errors.New("the pointer did not reach the point")
	}
	return nil
}

// posted records an input's time.
func (b *Backend) posted() {
	b.mu.Lock()
	b.lastInput = time.Now()
	b.mu.Unlock()
}

// Button presses or releases a button.
func (b *Backend) Button(ctx context.Context, btn computeruse.Button, down bool) error {
	defer b.posted()
	if down && b.mode == modeHeld {
		b.mu.Lock()
		b.clicked = true
		b.mu.Unlock()
	}
	code := map[computeruse.Button]int32{computeruse.ButtonLeft: portal.BtnLeft, computeruse.ButtonMiddle: portal.BtnMiddle, computeruse.ButtonRight: portal.BtnRight}[btn]
	if code == 0 {
		return errors.New("no such button")
	}
	return b.input.Button(ctx, code, down)
}

// Scroll scrolls by steps of the wheel.
func (b *Backend) Scroll(ctx context.Context, d computeruse.Direction, steps int) error {
	defer b.posted()
	axis, sign := uint32(0), int32(1)
	switch d {
	case computeruse.ScrollUp:
		sign = -1
	case computeruse.ScrollLeft:
		axis, sign = 1, -1
	case computeruse.ScrollRight:
		axis = 1
	}
	return b.input.Scroll(ctx, axis, sign*int32(steps))
}

// Key presses or releases a key by its keysym.
func (b *Backend) Key(ctx context.Context, keysym uint32, down bool) error {
	defer b.posted()
	return b.input.Keysym(ctx, keysym, down)
}

// Close ends the sessions and the stream.
func (b *Backend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()
	if b.tracker != nil {
		b.tracker.Close()
	}
	if b.stream != nil {
		_ = b.stream.Close()
	}
	for _, s := range slices.Compact([]*portal.Session{b.capture, b.input}) {
		if s != nil {
			_ = s.Close()
		}
	}
	return b.conn.Close()
}

// Supported says this build can read a shared window's stream.
func Supported() bool { return pipewire.Available() }
