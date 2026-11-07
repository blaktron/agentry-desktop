package x11

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// startXvfb runs a private X server on a display it picks itself and
// returns the display's name.
func startXvfb(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("Xvfb")
	if err != nil {
		t.Skip("Xvfb is not installed")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	cmd := exec.Command(path, "-displayfd", "3", "-screen", "0", "1280x800x24", "-nolisten", "tcp", "-noreset")
	cmd.ExtraFiles = []*os.File{w}
	if err := cmd.Start(); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	_ = w.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		t.Fatalf("Xvfb did not report a display: %v", err)
	}
	return ":" + strings.TrimSpace(line)
}

type fixture struct {
	display string
	conn    *xgb.Conn
	root    xproto.Window
	blender xproto.Window
	konsole xproto.Window
	events  chan xgb.Event
}

var fill = color.RGBA{R: 200, G: 30, B: 90, A: 255}

func setProp(t *testing.T, c *xgb.Conn, w xproto.Window, name string, typ xproto.Atom, format byte, data []byte) {
	t.Helper()
	a, err := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
	if err != nil {
		t.Fatal(err)
	}
	n := uint32(len(data))
	if format == 32 {
		n /= 4
	}
	if err := xproto.ChangePropertyChecked(c, xproto.PropModeReplace, w, a.Atom, typ, format, n, data).Check(); err != nil {
		t.Fatal(err)
	}
}

func newWindow(t *testing.T, c *xgb.Conn, root xproto.Window, x, y, w, h int, instance, class, title string, pid uint32) xproto.Window {
	t.Helper()
	id, err := xproto.NewWindowId(c)
	if err != nil {
		t.Fatal(err)
	}
	screen := xproto.Setup(c).DefaultScreen(c)
	mask := uint32(xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskPointerMotion |
		xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease | xproto.EventMaskExposure)
	if err := xproto.CreateWindowChecked(c, screen.RootDepth, id, root, int16(x), int16(y), uint16(w), uint16(h), 0,
		xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{screen.WhitePixel, mask}).Check(); err != nil {
		t.Fatal(err)
	}
	setProp(t, c, id, "WM_CLASS", xproto.AtomString, 8, []byte(instance+"\x00"+class+"\x00"))
	setProp(t, c, id, "_NET_WM_PID", xproto.AtomCardinal, 32, []byte{byte(pid), byte(pid >> 8), byte(pid >> 16), byte(pid >> 24)})
	utf8, _ := xproto.InternAtom(c, false, 11, "UTF8_STRING").Reply()
	setProp(t, c, id, "_NET_WM_NAME", utf8.Atom, 8, []byte(title))
	if err := xproto.MapWindowChecked(c, id).Check(); err != nil {
		t.Fatal(err)
	}
	return id
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	display := startXvfb(t)
	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	f := &fixture{display: display, conn: conn, root: root, events: make(chan xgb.Event, 256)}
	f.blender = newWindow(t, conn, root, 200, 100, 400, 300, "blender", "Blender", "Cube - Blender é", 4242)
	f.konsole = newWindow(t, conn, root, 700, 100, 200, 200, "konsole", "Konsole", "shell", 4343)

	gc, _ := xproto.NewGcontextId(conn)
	pixel := uint32(fill.R)<<16 | uint32(fill.G)<<8 | uint32(fill.B)
	if err := xproto.CreateGCChecked(conn, gc, xproto.Drawable(f.blender), xproto.GcForeground, []uint32{pixel}).Check(); err != nil {
		t.Fatal(err)
	}
	if err := xproto.PolyFillRectangleChecked(conn, xproto.Drawable(f.blender), gc, []xproto.Rectangle{{X: 0, Y: 0, Width: 400, Height: 300}}).Check(); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			ev, err := conn.WaitForEvent()
			if ev == nil && err == nil {
				close(f.events)
				return
			}
			if ev != nil {
				f.events <- ev
			}
		}
	}()
	return f
}

// next waits for an event accepted by pick, skipping the others.
func (f *fixture) next(t *testing.T, pick func(xgb.Event) bool) xgb.Event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-f.events:
			if !ok {
				t.Fatal("connection closed")
			}
			if pick(ev) {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for an event")
		}
	}
}

func (f *fixture) open(t *testing.T, scope string) *Backend {
	t.Helper()
	b, err := Open(context.Background(), Options{
		Display: f.display,
		Scope:   scope,
		Match: func(instance, class string, pid int) (string, bool) {
			if instance == "blender" && class == "Blender" && pid == 4242 {
				return "Blender", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestAppScopeListsOnlyTheMatchedWindows(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	ws, err := b.Windows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 {
		t.Fatalf("got %d windows, want 1: %+v", len(ws), ws)
	}
	w := ws[0]
	if w.ID != fmt.Sprint(uint32(f.blender)) || w.App != "Blender" || w.Title != "Cube - Blender é" || w.Size != image.Pt(400, 300) {
		t.Fatalf("unexpected window %+v", w)
	}
	if b.Session() != "Linux, X11" {
		t.Fatalf("session %q", b.Session())
	}
}

func TestScreenScopeListsTheDisplay(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeScreen)
	ws, err := b.Windows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0].ID != "screen" || ws[0].Size != image.Pt(1280, 800) || !ws[0].Focused {
		t.Fatalf("unexpected windows %+v", ws)
	}
	img, err := b.Capture(context.Background(), "screen")
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Size() != image.Pt(1280, 800) {
		t.Fatalf("capture size %v", img.Bounds())
	}
}

func TestCaptureReturnsTheWindowPixels(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	id := fmt.Sprint(uint32(f.blender))
	img, err := b.Capture(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 400, 300) {
		t.Fatalf("bounds %v", img.Bounds())
	}
	if _, ok := img.(*image.RGBA); !ok {
		t.Fatalf("got %T, want *image.RGBA", img)
	}
	for _, p := range []image.Point{{0, 0}, {200, 150}, {399, 299}} {
		if got := color.RGBAModel.Convert(img.At(p.X, p.Y)); got != color.Color(fill) {
			t.Fatalf("pixel %v = %v, want %v", p, got, fill)
		}
	}
}

func TestAWindowOutsideTheScopeIsRefused(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	id := fmt.Sprint(uint32(f.konsole))
	ctx := context.Background()
	if _, err := b.Capture(ctx, id); !errors.Is(err, computeruse.ErrNoWindow) {
		t.Fatalf("capture: %v", err)
	}
	if err := b.Move(ctx, id, image.Pt(5, 5)); !errors.Is(err, computeruse.ErrNoWindow) {
		t.Fatalf("move: %v", err)
	}
	if err := b.Focus(ctx, id); !errors.Is(err, computeruse.ErrNoWindow) {
		t.Fatalf("focus: %v", err)
	}
	if _, _, err := b.Cursor(ctx, id); err == nil {
		t.Fatal("cursor accepted a window outside the scope")
	}
}

func TestMoveAndButtonsReachTheWindowAtTheRightPosition(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	ctx := context.Background()
	id := fmt.Sprint(uint32(f.blender))

	if err := b.Move(ctx, id, image.Pt(120, 80)); err != nil {
		t.Fatal(err)
	}
	ev := f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.MotionNotifyEvent); return ok }).(xproto.MotionNotifyEvent)
	if ev.EventX != 120 || ev.EventY != 80 {
		t.Fatalf("motion at %d,%d, want 120,80", ev.EventX, ev.EventY)
	}
	p, inside, err := b.Cursor(ctx, id)
	if err != nil || !inside || p != image.Pt(120, 80) {
		t.Fatalf("cursor %v inside=%v err=%v", p, inside, err)
	}
	if err := b.Button(ctx, computeruse.ButtonRight, true); err != nil {
		t.Fatal(err)
	}
	if err := b.Button(ctx, computeruse.ButtonRight, false); err != nil {
		t.Fatal(err)
	}
	press := f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.ButtonPressEvent); return ok }).(xproto.ButtonPressEvent)
	if press.Detail != 3 || press.EventX != 120 || press.EventY != 80 {
		t.Fatalf("press %+v", press)
	}
	rel := f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.ButtonReleaseEvent); return ok }).(xproto.ButtonReleaseEvent)
	if rel.Detail != 3 {
		t.Fatalf("release %+v", rel)
	}
	if err := b.Move(ctx, id, image.Pt(400, 10)); err == nil {
		t.Fatal("a point outside the window was accepted")
	}
	if err := b.Move(ctx, id, image.Pt(-1, 10)); err == nil {
		t.Fatal("a negative point was accepted")
	}
	// Moved off the window, the cursor is reported outside it.
	if err := b.Move(ctx, id, image.Pt(399, 299)); err != nil {
		t.Fatal(err)
	}
	if _, inside, _ := b.Cursor(ctx, id); !inside {
		t.Fatal("corner pixel should be inside")
	}
}

func TestScrollSendsWheelButtons(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	ctx := context.Background()
	if err := b.Move(ctx, fmt.Sprint(uint32(f.blender)), image.Pt(50, 50)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		d    computeruse.Direction
		code byte
	}{{computeruse.ScrollUp, 4}, {computeruse.ScrollDown, 5}, {computeruse.ScrollLeft, 6}, {computeruse.ScrollRight, 7}} {
		if err := b.Scroll(ctx, c.d, 2); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			ev := f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.ButtonPressEvent); return ok }).(xproto.ButtonPressEvent)
			if byte(ev.Detail) != c.code {
				t.Fatalf("%s: button %d, want %d", c.d, ev.Detail, c.code)
			}
		}
	}
}

// keysymOf reads the keysym a keycode carries at a column.
func keysymOf(t *testing.T, c *xgb.Conn, code xproto.Keycode, col int) uint32 {
	t.Helper()
	r, err := xproto.GetKeyboardMapping(c, code, 1).Reply()
	if err != nil {
		t.Fatal(err)
	}
	return uint32(r.Keysyms[col])
}

func TestFocusedFollowsTheInputFocus(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	ctx := context.Background()
	focused := func() bool {
		ws, err := b.Windows(ctx)
		if err != nil || len(ws) != 1 {
			t.Fatalf("windows %v %v", ws, err)
		}
		return ws[0].Focused
	}
	if focused() {
		t.Fatal("focused before any focus was set")
	}
	if err := xproto.SetInputFocusChecked(f.conn, xproto.InputFocusParent, f.blender, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	if !focused() {
		t.Fatal("not focused with the input focus on the window")
	}
	if err := xproto.SetInputFocusChecked(f.conn, xproto.InputFocusParent, f.konsole, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	if focused() {
		t.Fatal("focused with the input focus elsewhere")
	}
	if err := b.Focus(ctx, fmt.Sprint(uint32(f.blender))); err != nil {
		t.Fatal(err)
	}
	if !focused() {
		t.Fatal("Focus did not give the window the keyboard")
	}
}

func TestKeyDeliversTheKeysymToTheFocusedWindow(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	ctx := context.Background()
	if err := xproto.SetInputFocusChecked(f.conn, xproto.InputFocusParent, f.blender, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	if err := b.Key(ctx, 'a', true); err != nil {
		t.Fatal(err)
	}
	if err := b.Key(ctx, 'a', false); err != nil {
		t.Fatal(err)
	}
	press := f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.KeyPressEvent); return ok }).(xproto.KeyPressEvent)
	if got := keysymOf(t, f.conn, press.Detail, 0); got != 'a' {
		t.Fatalf("keysym 0x%x, want 'a'", got)
	}
	f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.KeyReleaseEvent); return ok })

	if b.NeedsShift('a') || !b.NeedsShift('A') {
		t.Fatalf("NeedsShift a=%v A=%v", b.NeedsShift('a'), b.NeedsShift('A'))
	}
	if err := b.Key(ctx, 'A', true); err != nil {
		t.Fatal(err)
	}
	if err := b.Key(ctx, 'A', false); err != nil {
		t.Fatal(err)
	}
	press = f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.KeyPressEvent); return ok }).(xproto.KeyPressEvent)
	if got := keysymOf(t, f.conn, press.Detail, 1); got != 'A' {
		t.Fatalf("shifted column keysym 0x%x, want 'A'", got)
	}
}

func TestAKeysymNoKeyCarriesUsesASpareKeycodeAndRestoresIt(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	ctx := context.Background()
	const zhe = 0x1000416 // Cyrillic Zhe, not on the default keymap
	if err := xproto.SetInputFocusChecked(f.conn, xproto.InputFocusParent, f.blender, xproto.TimeCurrentTime).Check(); err != nil {
		t.Fatal(err)
	}
	if err := b.Key(ctx, zhe, true); err != nil {
		t.Fatal(err)
	}
	press := f.next(t, func(e xgb.Event) bool { _, ok := e.(xproto.KeyPressEvent); return ok }).(xproto.KeyPressEvent)
	if got := keysymOf(t, f.conn, press.Detail, 0); got != zhe {
		t.Fatalf("keysym 0x%x while held, want 0x%x", got, zhe)
	}
	if err := b.Key(ctx, zhe, false); err != nil {
		t.Fatal(err)
	}
	if got := keysymOf(t, f.conn, press.Detail, 0); got != 0 {
		t.Fatalf("keycode still carries 0x%x after release", got)
	}
}

func TestCloseRestoresAHeldRemappedKey(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	const zhe = 0x1000416
	if err := b.Key(context.Background(), zhe, true); err != nil {
		t.Fatal(err)
	}
	code := b.remapped[zhe]
	if code == 0 {
		t.Fatal("no spare keycode was used")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if got := keysymOf(t, f.conn, code, 0); got != 0 {
		t.Fatalf("keycode still carries 0x%x after Close", got)
	}
}

func TestOpenWithoutAServerIsNotGranted(t *testing.T) {
	_, err := Open(context.Background(), Options{Display: ":199", Scope: computeruse.ScopeScreen})
	if !errors.Is(err, computeruse.ErrNotGranted) {
		t.Fatalf("got %v", err)
	}
}

// A window mapped above the allowed one covers it where they overlap, and
// only there; a call after Close is refused, never a crash.
func TestAWindowAboveTheAllowedOneCoversIt(t *testing.T) {
	f := newFixture(t)
	b := f.open(t, computeruse.ScopeApp)
	id := strconv.FormatUint(uint64(f.blender), 10)
	ctx := context.Background()
	if covered, err := b.Covered(ctx, id, image.Pt(10, 10)); err != nil || covered {
		t.Fatalf("before the dialog: covered %v, %v", covered, err)
	}
	// A dialog over the allowed window's top-left corner (blender is at
	// 200,100 on the root).
	newWindow(t, f.conn, f.root, 150, 50, 120, 120, "dialog", "Dialog", "a dialog", 9999)
	if covered, err := b.Covered(ctx, id, image.Pt(10, 10)); err != nil || !covered {
		t.Fatalf("under the dialog: covered %v, %v", covered, err)
	}
	if covered, err := b.Covered(ctx, id, image.Pt(300, 200)); err != nil || covered {
		t.Fatalf("away from the dialog: covered %v, %v", covered, err)
	}
	_ = b.Close()
	if err := b.Key(ctx, 'a', false); err == nil {
		t.Fatal("a key after Close was taken")
	}
	if err := b.Button(ctx, computeruse.ButtonLeft, false); err == nil {
		t.Fatal("a button after Close was taken")
	}
	if _, err := b.Windows(ctx); err == nil {
		t.Fatal("windows after Close answered")
	}
}
