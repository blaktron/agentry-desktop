// Package x11 is the computer-use backend for an X server: a real X11
// session or Xwayland. It reads windows from the server, captures them with
// GetImage (through a Composite window pixmap when the server has one, so a
// covered window still captures), and sends input with the XTEST extension.
//
// X11 holds no input to a window: XTEST events go to whatever is under the
// pointer or has the focus. The backend refuses a capture or a move for a
// window the scope does not allow, and checks a move against the window's
// size; the check that an allowed window is focused before each input is the
// caller's.
package x11

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/composite"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// screenID is the one window of screen scope.
const screenID = "screen"

// Options configures Open.
type Options struct {
	// Display is the X display to connect to; empty means $DISPLAY.
	Display string
	// Scope is computeruse.ScopeApp or computeruse.ScopeScreen.
	Scope string
	// Match says whether a top-level window belongs to an application the
	// run may use, and that application's name, from the window's WM_CLASS
	// instance and class and its _NET_WM_PID (0 when unset). App scope only.
	Match func(instance, class string, pid int) (app string, ok bool)
}

// Backend implements computeruse.Backend on an X server.
type Backend struct {
	mu      sync.Mutex
	conn    *xgb.Conn
	setup   *xproto.SetupInfo
	root    xproto.Window
	opt     Options
	session string

	hasComposite bool
	atoms        atoms
	// remapped holds the spare keycode temporarily given to a keysym that
	// no keycode carries, until that key is released.
	remapped map[uint32]xproto.Keycode
}

type atoms struct {
	clientList, activeWindow, supported       xproto.Atom
	wmName, netWMName, utf8, wmState, wmClass xproto.Atom
	wmPID                                     xproto.Atom
}

var _ computeruse.Backend = (*Backend)(nil)

// Open connects to the X server. The error wraps computeruse.ErrNotGranted
// when there is no server to reach or it lacks the XTEST extension.
func Open(ctx context.Context, opt Options) (*Backend, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opt.Scope != computeruse.ScopeApp && opt.Scope != computeruse.ScopeScreen {
		return nil, fmt.Errorf("x11: unknown scope %q", opt.Scope)
	}
	if opt.Scope == computeruse.ScopeApp && opt.Match == nil {
		return nil, errors.New("x11: app scope needs a Match function")
	}
	conn, err := xgb.NewConnDisplay(opt.Display)
	if err != nil {
		return nil, fmt.Errorf("%w: no X server to connect to: %v", computeruse.ErrNotGranted, err)
	}
	b := &Backend{conn: conn, opt: opt, remapped: map[uint32]xproto.Keycode{}}
	if err := b.init(); err != nil {
		conn.Close()
		return nil, err
	}
	return b, nil
}

func (b *Backend) init() error {
	b.setup = xproto.Setup(b.conn)
	if len(b.setup.Roots) == 0 {
		return fmt.Errorf("%w: the X server has no screen", computeruse.ErrNotGranted)
	}
	b.root = b.setup.DefaultScreen(b.conn).Root
	if err := xtest.Init(b.conn); err != nil {
		return fmt.Errorf("%w: the X server has no XTEST extension", computeruse.ErrNotGranted)
	}
	if composite.Init(b.conn) == nil {
		if r, err := composite.QueryVersion(b.conn, 0, 4).Reply(); err == nil && (r.MajorVersion > 0 || r.MinorVersion >= 2) {
			b.hasComposite = true
		}
	}
	intern := func(name string) xproto.Atom {
		r, err := xproto.InternAtom(b.conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			return 0
		}
		return r.Atom
	}
	b.atoms = atoms{
		clientList:   intern("_NET_CLIENT_LIST"),
		activeWindow: intern("_NET_ACTIVE_WINDOW"),
		supported:    intern("_NET_SUPPORTED"),
		wmName:       xproto.AtomWmName,
		netWMName:    intern("_NET_WM_NAME"),
		utf8:         intern("UTF8_STRING"),
		wmState:      intern("WM_STATE"),
		wmClass:      xproto.AtomWmClass,
		wmPID:        intern("_NET_WM_PID"),
	}
	xwayland := strings.Contains(strings.ToLower(b.setup.Vendor), "xwayland") ||
		os.Getenv("XDG_SESSION_TYPE") == "wayland" || os.Getenv("WAYLAND_DISPLAY") != ""
	if xwayland {
		b.session = "Linux, Xwayland"
	} else {
		b.session = "Linux, X11"
	}
	return nil
}

// Session names the display for the receipt.
func (b *Backend) Session() string { return b.session }

// Enforced says what the window system holds of the scope.
func (b *Backend) Enforced() string {
	if b.opt.Scope == computeruse.ScopeScreen {
		return "the whole display; X11 holds nothing to a window, and in screen scope nothing is checked"
	}
	return "the capture is of the allowed windows alone, read from the X server; X11 holds no input to a window, so Agentry Desktop checks before each input that an allowed window is focused and the point inside it"
}

// Windows lists the windows the scope allows.
func (b *Backend) Windows(ctx context.Context) ([]computeruse.Window, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.windows()
}

func (b *Backend) windows() ([]computeruse.Window, error) {
	if b.conn == nil {
		return nil, errClosed
	}
	if b.opt.Scope == computeruse.ScopeScreen {
		g, err := xproto.GetGeometry(b.conn, xproto.Drawable(b.root)).Reply()
		if err != nil {
			return nil, fmt.Errorf("x11: reading the root window: %w", err)
		}
		return []computeruse.Window{{ID: screenID, App: "the display", Size: image.Pt(int(g.Width), int(g.Height)), Focused: true}}, nil
	}
	active := b.activeWindow()
	var out []computeruse.Window
	for _, w := range b.clientWindows() {
		attr, err := xproto.GetWindowAttributes(b.conn, w).Reply()
		if err != nil || attr.MapState != xproto.MapStateViewable {
			continue
		}
		instance, class := b.wmClassOf(w)
		app, ok := b.opt.Match(instance, class, b.pidOf(w))
		if !ok {
			continue
		}
		g, err := xproto.GetGeometry(b.conn, xproto.Drawable(w)).Reply()
		if err != nil {
			continue
		}
		focused := false
		if active != 0 {
			focused = active == w
		} else {
			focused = b.inputFocusWithin(w)
		}
		out = append(out, computeruse.Window{
			ID:      strconv.FormatUint(uint64(w), 10),
			App:     app,
			Title:   b.titleOf(w),
			Size:    image.Pt(int(g.Width), int(g.Height)),
			Focused: focused,
		})
	}
	return out, nil
}

// clientWindows are the top-level client windows: the window manager's
// client list when it keeps one, else the root's children, descending into a
// reparenting window manager's frames to the window that has WM_STATE.
func (b *Backend) clientWindows() []xproto.Window {
	if ids := b.windowList(b.root, b.atoms.clientList); len(ids) > 0 {
		return ids
	}
	tree, err := xproto.QueryTree(b.conn, b.root).Reply()
	if err != nil {
		return nil
	}
	var out []xproto.Window
	for _, child := range tree.Children {
		if b.hasProp(child, b.atoms.wmState) {
			out = append(out, child)
			continue
		}
		if sub, err := xproto.QueryTree(b.conn, child).Reply(); err == nil {
			found := false
			for _, c := range sub.Children {
				if b.hasProp(c, b.atoms.wmState) {
					out = append(out, c)
					found = true
					break
				}
			}
			if found {
				continue
			}
		}
		if b.hasProp(child, b.atoms.wmClass) {
			out = append(out, child)
		}
	}
	return out
}

func (b *Backend) prop(w xproto.Window, name, typ xproto.Atom) *xproto.GetPropertyReply {
	if name == 0 {
		return nil
	}
	r, err := xproto.GetProperty(b.conn, false, w, name, typ, 0, 1024).Reply()
	if err != nil || r.ValueLen == 0 {
		return nil
	}
	return r
}

func (b *Backend) hasProp(w xproto.Window, name xproto.Atom) bool {
	return b.prop(w, name, xproto.GetPropertyTypeAny) != nil
}

func (b *Backend) windowList(w xproto.Window, name xproto.Atom) []xproto.Window {
	r := b.prop(w, name, xproto.AtomWindow)
	if r == nil || r.Format != 32 {
		return nil
	}
	out := make([]xproto.Window, 0, len(r.Value)/4)
	for i := 0; i+4 <= len(r.Value); i += 4 {
		out = append(out, xproto.Window(xgb.Get32(r.Value[i:])))
	}
	return out
}

func (b *Backend) activeWindow() xproto.Window {
	r := b.prop(b.root, b.atoms.activeWindow, xproto.AtomWindow)
	if r == nil || r.Format != 32 || len(r.Value) < 4 {
		return 0
	}
	return xproto.Window(xgb.Get32(r.Value))
}

// inputFocusWithin says the input focus is w or one of its descendants.
func (b *Backend) inputFocusWithin(w xproto.Window) bool {
	f, err := xproto.GetInputFocus(b.conn).Reply()
	if err != nil {
		return false
	}
	cur := f.Focus
	for i := 0; i < 64 && cur > 1 && cur != b.root; i++ {
		if cur == w {
			return true
		}
		t, err := xproto.QueryTree(b.conn, cur).Reply()
		if err != nil {
			return false
		}
		cur = t.Parent
	}
	return false
}

func (b *Backend) wmClassOf(w xproto.Window) (instance, class string) {
	r := b.prop(w, b.atoms.wmClass, xproto.AtomString)
	if r == nil {
		return "", ""
	}
	parts := strings.Split(string(r.Value), "\x00")
	instance = parts[0]
	if len(parts) > 1 {
		class = parts[1]
	}
	return instance, class
}

func (b *Backend) pidOf(w xproto.Window) int {
	r := b.prop(w, b.atoms.wmPID, xproto.AtomCardinal)
	if r == nil || r.Format != 32 || len(r.Value) < 4 {
		return 0
	}
	return int(xgb.Get32(r.Value))
}

func (b *Backend) titleOf(w xproto.Window) string {
	if r := b.prop(w, b.atoms.netWMName, b.atoms.utf8); r != nil {
		return string(r.Value)
	}
	if r := b.prop(w, b.atoms.wmName, xproto.GetPropertyTypeAny); r != nil {
		// WM_NAME is Latin-1 unless the client says otherwise.
		if r.Type == b.atoms.utf8 {
			return string(r.Value)
		}
		rs := make([]rune, len(r.Value))
		for i, c := range r.Value {
			rs[i] = rune(c)
		}
		return string(rs)
	}
	return ""
}

// target resolves an id the scope allows to its X window and current size.
func (b *Backend) target(id string) (xproto.Window, image.Point, error) {
	ws, err := b.windows()
	if err != nil {
		return 0, image.Point{}, err
	}
	for _, w := range ws {
		if w.ID != id {
			continue
		}
		if id == screenID {
			return b.root, w.Size, nil
		}
		n, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			break
		}
		return xproto.Window(n), w.Size, nil
	}
	return 0, image.Point{}, fmt.Errorf("x11: window %q is not one the run may use: %w", id, computeruse.ErrNoWindow)
}

// Focus gives the window the keyboard and raises it.
func (b *Backend) Focus(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, _, err := b.target(id)
	if err != nil {
		return err
	}
	if w == b.root {
		return nil
	}
	if b.wmSupportsActivation() {
		ev := xproto.ClientMessageEvent{
			Format: 32,
			Window: w,
			Type:   b.atoms.activeWindow,
			Data:   xproto.ClientMessageDataUnionData32New([]uint32{2, 0, 0, 0, 0}),
		}
		mask := uint32(xproto.EventMaskSubstructureRedirect | xproto.EventMaskSubstructureNotify)
		return xproto.SendEventChecked(b.conn, false, b.root, mask, string(ev.Bytes())).Check()
	}
	if err := xproto.ConfigureWindowChecked(b.conn, w, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove}).Check(); err != nil {
		return fmt.Errorf("x11: raising the window: %w", err)
	}
	if err := xproto.SetInputFocusChecked(b.conn, xproto.InputFocusPointerRoot, w, xproto.TimeCurrentTime).Check(); err != nil {
		return fmt.Errorf("x11: focusing the window: %w", err)
	}
	return nil
}

func (b *Backend) wmSupportsActivation() bool {
	r := b.prop(b.root, b.atoms.supported, xproto.AtomAtom)
	if r == nil || r.Format != 32 {
		return false
	}
	for i := 0; i+4 <= len(r.Value); i += 4 {
		if xproto.Atom(xgb.Get32(r.Value[i:])) == b.atoms.activeWindow {
			return true
		}
	}
	return false
}

// Capture returns the window's image at its own size.
func (b *Backend) Capture(ctx context.Context, id string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, size, err := b.target(id)
	if err != nil {
		return nil, err
	}
	if size.X <= 0 || size.Y <= 0 {
		return nil, errors.New("x11: the window has no size")
	}
	if w != b.root && b.hasComposite {
		if img, err := b.captureNamed(w, size); err == nil {
			return img, nil
		}
	}
	return b.getImage(xproto.Drawable(w), 0, 0, size)
}

// captureNamed reads the window from its Composite pixmap, or from its
// frame's pixmap cropped to the window when a compositing window manager
// redirected the frame. It fails when the window is not redirected.
func (b *Backend) captureNamed(w xproto.Window, size image.Point) (image.Image, error) {
	if img, err := b.readNamed(w, 0, 0, size); err == nil {
		return img, nil
	}
	frame := w
	for i := 0; i < 16; i++ {
		t, err := xproto.QueryTree(b.conn, frame).Reply()
		if err != nil {
			return nil, err
		}
		if t.Parent == b.root || t.Parent == 0 {
			break
		}
		frame = t.Parent
	}
	if frame == w {
		return nil, errors.New("x11: no window pixmap")
	}
	off, err := xproto.TranslateCoordinates(b.conn, w, frame, 0, 0).Reply()
	if err != nil {
		return nil, err
	}
	return b.readNamed(frame, int(off.DstX), int(off.DstY), size)
}

func (b *Backend) readNamed(w xproto.Window, x, y int, size image.Point) (image.Image, error) {
	pid, err := xproto.NewPixmapId(b.conn)
	if err != nil {
		return nil, err
	}
	if err := composite.NameWindowPixmapChecked(b.conn, w, pid).Check(); err != nil {
		return nil, err
	}
	defer xproto.FreePixmap(b.conn, pid)
	return b.getImage(xproto.Drawable(pid), x, y, size)
}

func (b *Backend) getImage(d xproto.Drawable, x, y int, size image.Point) (image.Image, error) {
	r, err := xproto.GetImage(b.conn, xproto.ImageFormatZPixmap, d, int16(x), int16(y), uint16(size.X), uint16(size.Y), 0xffffffff).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11: reading the window's image: %w", err)
	}
	if r.Depth != 24 && r.Depth != 32 || bitsPerPixel(b.setup, r.Depth) != 32 {
		return nil, fmt.Errorf("x11: the window's depth %d is not one the capture reads", r.Depth)
	}
	if len(r.Data) < size.X*size.Y*4 {
		return nil, errors.New("x11: the server returned a short image")
	}
	img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	msb := b.setup.ImageByteOrder == xproto.ImageOrderMSBFirst
	for i := 0; i < size.X*size.Y; i++ {
		s := r.Data[i*4 : i*4+4]
		d := img.Pix[i*4 : i*4+4]
		if msb {
			d[0], d[1], d[2] = s[1], s[2], s[3]
		} else {
			d[0], d[1], d[2] = s[2], s[1], s[0]
		}
		d[3] = 255
	}
	return img, nil
}

func bitsPerPixel(s *xproto.SetupInfo, depth byte) int {
	for _, f := range s.PixmapFormats {
		if f.Depth == depth {
			return int(f.BitsPerPixel)
		}
	}
	return 0
}

// Cursor is the pointer's position in the window and whether it is inside.
func (b *Backend) Cursor(ctx context.Context, id string) (image.Point, bool, error) {
	if err := ctx.Err(); err != nil {
		return image.Point{}, false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, size, err := b.target(id)
	if err != nil {
		return image.Point{}, false, err
	}
	q, err := xproto.QueryPointer(b.conn, w).Reply()
	if err != nil {
		return image.Point{}, false, fmt.Errorf("x11: reading the pointer: %w", err)
	}
	p := image.Pt(int(q.WinX), int(q.WinY))
	return p, q.SameScreen && p.In(image.Rectangle{Max: size}), nil
}

// Move puts the pointer at a position in the window.
func (b *Backend) Move(ctx context.Context, id string, p image.Point) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	w, size, err := b.target(id)
	if err != nil {
		return err
	}
	if !p.In(image.Rectangle{Max: size}) {
		return fmt.Errorf("x11: point %v is outside the %dx%d window", p, size.X, size.Y)
	}
	t, err := xproto.TranslateCoordinates(b.conn, w, b.root, int16(p.X), int16(p.Y)).Reply()
	if err != nil {
		return fmt.Errorf("x11: locating the window: %w", err)
	}
	return b.fake(xproto.MotionNotify, 0, t.DstX, t.DstY)
}

func (b *Backend) fake(typ, detail byte, x, y int16) error {
	if b.conn == nil {
		return errClosed
	}
	if err := xtest.FakeInputChecked(b.conn, typ, detail, 0, b.root, x, y, 0).Check(); err != nil {
		return fmt.Errorf("x11: sending input: %w", err)
	}
	return nil
}

// errClosed answers a call after Close.
var errClosed = errors.New("x11: the connection to the X server is closed")

// Covered says a window other than the allowed one is above it at the
// point: the root's children are its top-level windows from the bottom of
// the stack to the top, so any mapped one after the allowed window's that
// contains the point would take an input there.
func (b *Backend) Covered(ctx context.Context, id string, p image.Point) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return false, errClosed
	}
	w, _, err := b.target(id)
	if err != nil {
		return false, err
	}
	if w == b.root {
		return false, nil
	}
	top, err := b.topLevel(w)
	if err != nil {
		return false, err
	}
	t, err := xproto.TranslateCoordinates(b.conn, w, b.root, int16(p.X), int16(p.Y)).Reply()
	if err != nil {
		return false, fmt.Errorf("x11: locating the window: %w", err)
	}
	pt := image.Pt(int(t.DstX), int(t.DstY))
	tree, err := xproto.QueryTree(b.conn, b.root).Reply()
	if err != nil {
		return false, fmt.Errorf("x11: reading the stack: %w", err)
	}
	above := false
	for _, c := range tree.Children {
		if c == top {
			above = true
			continue
		}
		if !above {
			continue
		}
		attr, err := xproto.GetWindowAttributes(b.conn, c).Reply()
		if err != nil || attr.MapState != xproto.MapStateViewable || attr.Class == xproto.WindowClassInputOnly {
			continue
		}
		g, err := xproto.GetGeometry(b.conn, xproto.Drawable(c)).Reply()
		if err != nil {
			continue
		}
		r := image.Rect(int(g.X), int(g.Y), int(g.X)+int(g.Width)+2*int(g.BorderWidth), int(g.Y)+int(g.Height)+2*int(g.BorderWidth))
		if pt.In(r) {
			return true, nil
		}
	}
	return false, nil
}

// topLevel is the child of the root that holds w: the window itself, or
// the frame a window manager put it in.
func (b *Backend) topLevel(w xproto.Window) (xproto.Window, error) {
	for range 32 {
		t, err := xproto.QueryTree(b.conn, w).Reply()
		if err != nil {
			return 0, fmt.Errorf("x11: reading the window's parent: %w", err)
		}
		if t.Parent == b.root || t.Parent == 0 {
			return w, nil
		}
		w = t.Parent
	}
	return w, nil
}

// Button presses or releases a mouse button where the pointer is.
func (b *Backend) Button(ctx context.Context, btn computeruse.Button, down bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if btn < computeruse.ButtonLeft || btn > computeruse.ButtonRight {
		return fmt.Errorf("x11: unknown button %d", btn)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.button(byte(btn), down)
}

func (b *Backend) button(n byte, down bool) error {
	typ := byte(xproto.ButtonRelease)
	if down {
		typ = xproto.ButtonPress
	}
	return b.fake(typ, n, 0, 0)
}

// Scroll turns the wheel by steps where the pointer is.
func (b *Backend) Scroll(ctx context.Context, d computeruse.Direction, steps int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var n byte
	switch d {
	case computeruse.ScrollUp:
		n = 4
	case computeruse.ScrollDown:
		n = 5
	case computeruse.ScrollLeft:
		n = 6
	case computeruse.ScrollRight:
		n = 7
	default:
		return fmt.Errorf("x11: unknown scroll direction %q", d)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := 0; i < steps; i++ {
		if err := b.button(n, true); err != nil {
			return err
		}
		if err := b.button(n, false); err != nil {
			return err
		}
	}
	return nil
}

type keymap struct {
	min      xproto.Keycode
	per      int
	keysyms  []xproto.Keysym
	keycodes int
}

func (b *Backend) keymap() (*keymap, error) {
	min, max := b.setup.MinKeycode, b.setup.MaxKeycode
	n := int(max) - int(min) + 1
	r, err := xproto.GetKeyboardMapping(b.conn, min, byte(n)).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11: reading the keyboard mapping: %w", err)
	}
	per := int(r.KeysymsPerKeycode)
	if per == 0 || len(r.Keysyms) < n*per {
		return nil, errors.New("x11: the keyboard mapping is empty")
	}
	return &keymap{min: min, per: per, keysyms: r.Keysyms, keycodes: n}, nil
}

func (m *keymap) at(i, col int) xproto.Keysym {
	if col >= m.per {
		return 0
	}
	return m.keysyms[i*m.per+col]
}

// find returns the keycode that carries the keysym in column col.
func (m *keymap) find(ks xproto.Keysym, col int) (xproto.Keycode, bool) {
	for i := 0; i < m.keycodes; i++ {
		if m.at(i, col) == ks {
			return m.min + xproto.Keycode(i), true
		}
	}
	return 0, false
}

// spare returns a keycode with no keysyms, from the top of the range down.
func (m *keymap) spare(taken map[uint32]xproto.Keycode) (xproto.Keycode, bool) {
outer:
	for i := m.keycodes - 1; i >= 0; i-- {
		code := m.min + xproto.Keycode(i)
		for _, t := range taken {
			if t == code {
				continue outer
			}
		}
		empty := true
		for c := 0; c < m.per; c++ {
			if m.at(i, c) != 0 {
				empty = false
				break
			}
		}
		if empty {
			return code, true
		}
	}
	return 0, false
}

// NeedsShift reports whether the keysym is reachable only at the shifted
// column of its keycode. Key presses the keycode alone: the caller holds
// Shift around such a keysym.
func (b *Backend) NeedsShift(keysym uint32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, err := b.keymap()
	if err != nil {
		return false
	}
	ks := xproto.Keysym(keysym)
	if _, ok := m.find(ks, 0); ok {
		return false
	}
	_, ok := m.find(ks, 1)
	return ok
}

// Key presses or releases the keycode that carries the keysym. A keysym no
// keycode carries is put on a spare keycode until the key is released.
func (b *Backend) Key(ctx context.Context, keysym uint32, down bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return errClosed
	}
	m, err := b.keymap()
	if err != nil {
		return err
	}
	ks := xproto.Keysym(keysym)
	code, ok := m.find(ks, 0)
	if !ok {
		code, ok = m.find(ks, 1)
	}
	if !ok {
		if !down {
			return fmt.Errorf("x11: no key carries keysym 0x%x", keysym)
		}
		spare, found := m.spare(b.remapped)
		if !found {
			return fmt.Errorf("x11: no key carries keysym 0x%x and no spare keycode is free", keysym)
		}
		syms := make([]xproto.Keysym, m.per)
		for i := range syms {
			syms[i] = ks
		}
		if err := xproto.ChangeKeyboardMappingChecked(b.conn, 1, spare, byte(m.per), syms).Check(); err != nil {
			return fmt.Errorf("x11: remapping a spare key: %w", err)
		}
		b.remapped[keysym] = spare
		code = spare
	}
	typ := byte(xproto.KeyRelease)
	if down {
		typ = xproto.KeyPress
	}
	ferr := b.fake(typ, byte(code), 0, 0)
	if !down {
		if spare, ok := b.remapped[keysym]; ok {
			if err := b.restore(spare, m.per); err != nil && ferr == nil {
				ferr = err
			}
		}
	}
	return ferr
}

func (b *Backend) restore(code xproto.Keycode, per int) error {
	for ks, c := range b.remapped {
		if c == code {
			delete(b.remapped, ks)
		}
	}
	if err := xproto.ChangeKeyboardMappingChecked(b.conn, 1, code, byte(per), make([]xproto.Keysym, per)).Check(); err != nil {
		return fmt.Errorf("x11: restoring a remapped key: %w", err)
	}
	return nil
}

// Close restores any remapped keycode and closes the connection.
func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return nil
	}
	var first error
	if len(b.remapped) > 0 {
		if m, err := b.keymap(); err == nil {
			codes := map[xproto.Keycode]bool{}
			for _, c := range b.remapped {
				codes[c] = true
			}
			for c := range codes {
				if err := b.restore(c, m.per); err != nil && first == nil {
					first = err
				}
			}
		}
	}
	b.conn.Close()
	b.conn = nil
	return first
}
