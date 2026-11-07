package computeruse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
)

// DefaultMaxEdge is the longest edge of a screenshot, in pixels: a larger
// window is scaled down to it, and positions are pixels of that image.
const DefaultMaxEdge = 1280

// Options shape an Agent.
type Options struct {
	// Scope is ScopeApp or ScopeScreen.
	Scope string
	// MaxEdge is the longest edge of a screenshot; 0 is DefaultMaxEdge.
	MaxEdge int
	// Keep is the folder where each screenshot is saved, when the person
	// keeps what the agent saw; empty keeps none.
	Keep string
	// Sleep waits d or until ctx ends; nil is a timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// Gate is asked before each input: a refusal (the person paused or
	// stopped the run) stops the action there. Nil lets every input
	// through.
	Gate func() error
}

// Refusal is a call the scope or the window system refused: Reason is one
// of the receipt's desktop-refused details.
type Refusal struct {
	Reason  string
	Message string
}

func (r *Refusal) Error() string { return r.Message }

func outside(format string, args ...any) *Refusal {
	return &Refusal{Reason: receipt.DesktopOutsideWindow, Message: fmt.Sprintf(format, args...)}
}

// Agent answers one binding's computer-use tools through a backend.
type Agent struct {
	b   Backend
	opt Options

	mu sync.Mutex
	// shots are each window's last screenshot: its size and its scale
	// against the window, which every position of a later call is read in.
	shots map[string]shot
	// last is the window of the last screenshot.
	last string
	kept int
}

type shot struct {
	size  image.Point
	scale float64
}

// New makes an agent over a backend.
func New(b Backend, opt Options) *Agent {
	if opt.MaxEdge <= 0 {
		opt.MaxEdge = DefaultMaxEdge
	}
	if opt.Sleep == nil {
		opt.Sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-t.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return &Agent{b: b, opt: opt, shots: map[string]shot{}}
}

// MaxEdge is the longest edge of a screenshot.
func (a *Agent) MaxEdge() int { return a.opt.MaxEdge }

// Call answers one tool call. A refusal of the scope is a *Refusal; any
// other error is the window system's, answered to the agent as an error.
// The answer's Window names the window the call saw or acted in, for the
// run's call log.
func (a *Agent) Call(ctx context.Context, tool string, raw json.RawMessage) (desktoplink.Answer, error) {
	var args callArgs
	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&args); err != nil {
			return desktoplink.Answer{}, fmt.Errorf("the arguments of %s are not ones it takes: %v", tool, err)
		}
	}
	switch tool {
	case "screenshot":
		return a.screenshot(ctx, args.Window)
	case "zoom":
		return a.zoom(ctx, args.Region)
	case "left_click", "right_click", "middle_click", "double_click", "triple_click":
		return a.click(ctx, tool, args)
	case "left_click_drag":
		return a.drag(ctx, args)
	case "mouse_move":
		return a.move(ctx, args)
	case "cursor_position":
		return a.cursor(ctx)
	case "scroll":
		return a.scroll(ctx, args)
	case "type":
		return a.typeText(ctx, args.Text)
	case "key":
		return a.key(ctx, args.Text)
	case "hold_key":
		return a.holdKey(ctx, args.Text, args.Duration)
	case "wait":
		return a.wait(ctx, args.Duration)
	case "windows":
		return a.windows(ctx)
	case "focus_window":
		return a.focus(ctx, args.Window)
	}
	return desktoplink.Answer{}, fmt.Errorf("%q is not a computer-use tool", tool)
}

// callArgs are every tool's arguments; each tool reads its own.
type callArgs struct {
	Window          string  `json:"window"`
	Region          []int   `json:"region"`
	Coordinate      []int   `json:"coordinate"`
	StartCoordinate []int   `json:"start_coordinate"`
	Text            *string `json:"text"`
	ScrollDirection string  `json:"scroll_direction"`
	ScrollAmount    int     `json:"scroll_amount"`
	Duration        float64 `json:"duration"`
}

func (c callArgs) text() string {
	if c.Text == nil {
		return ""
	}
	return *c.Text
}

// target is the window an input goes to: the allowed window that is
// focused, refused outside-window when none is.
func (a *Agent) target(ctx context.Context) (Window, error) {
	if a.opt.Gate != nil {
		if err := a.opt.Gate(); err != nil {
			return Window{}, err
		}
	}
	ws, err := a.b.Windows(ctx)
	if err != nil {
		return Window{}, err
	}
	if len(ws) == 0 {
		return Window{}, outside("none of the windows this run may use is open")
	}
	for _, w := range ws {
		if w.Focused {
			return w, nil
		}
	}
	return Window{}, outside("none of the windows this run may use is focused, so the input was not sent: bring one to the front with focus_window")
}

// check is the check before each input: the window is still allowed and
// focused, and the point is inside it.
func (a *Agent) check(ctx context.Context, id string, p image.Point) (Window, error) {
	w, err := a.target(ctx)
	if err != nil {
		return Window{}, err
	}
	if w.ID != id {
		return Window{}, outside("the focus moved to another window before the input was sent, so it was not: take a new screenshot")
	}
	if p.X < 0 || p.Y < 0 || p.X >= w.Size.X || p.Y >= w.Size.Y {
		return Window{}, outside("the point is outside %s's window, so the input was not sent", w.App)
	}
	if c, ok := a.b.(Coverer); ok && a.opt.Scope == ScopeApp {
		covered, err := c.Covered(ctx, id, p)
		if err != nil {
			return Window{}, err
		}
		if covered {
			return Window{}, outside("another window is above %s's window at that point, so the input was not sent: take a new screenshot", w.App)
		}
	}
	return w, nil
}

// toWindow reads a position of the window's last screenshot as a point of
// the window.
func (a *Agent) toWindow(w Window, c []int, name string) (image.Point, error) {
	if len(c) != 2 {
		return image.Point{}, fmt.Errorf("%s is [x, y]", name)
	}
	a.mu.Lock()
	s, ok := a.shots[w.ID]
	a.mu.Unlock()
	if !ok {
		return image.Point{}, outside("there is no screenshot of %s's window yet, and positions are pixels of one: take a screenshot first", w.App)
	}
	x, y := c[0], c[1]
	if x < 0 || y < 0 || x >= s.size.X || y >= s.size.Y {
		return image.Point{}, outside("[%d, %d] is outside %s's window, which the last screenshot showed at %d×%d", x, y, w.App, s.size.X, s.size.Y)
	}
	return image.Pt(int(math.Floor(float64(x)/s.scale)), int(math.Floor(float64(y)/s.scale))), nil
}

// fromWindow reads a point of the window as a position of its last
// screenshot.
func (a *Agent) fromWindow(id string, p image.Point) image.Point {
	a.mu.Lock()
	s, ok := a.shots[id]
	a.mu.Unlock()
	if !ok {
		return p
	}
	return image.Pt(int(math.Round(float64(p.X)*s.scale)), int(math.Round(float64(p.Y)*s.scale)))
}

func label(w Window) string {
	if w.Title == "" || w.Title == w.App {
		return w.App
	}
	return w.App + " — " + w.Title
}

func (a *Agent) screenshot(ctx context.Context, id string) (desktoplink.Answer, error) {
	ws, err := a.b.Windows(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	var w Window
	switch {
	case id != "":
		found := false
		for _, c := range ws {
			if c.ID == id {
				w, found = c, true
			}
		}
		if !found {
			return desktoplink.Answer{}, outside("%q is not a window this run may see: windows lists them", id)
		}
	case len(ws) == 0:
		return desktoplink.Answer{}, outside("none of the windows this run may use is open")
	default:
		w = ws[0]
		for _, c := range ws {
			if c.Focused {
				w = c
				break
			}
		}
	}
	img, err := a.b.Capture(ctx, w.ID)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	scaled, scale := fit(img, a.opt.MaxEdge, 1)
	a.mu.Lock()
	a.shots[w.ID] = shot{size: scaled.Bounds().Size(), scale: scale}
	a.last = w.ID
	a.mu.Unlock()
	data, err := a.encode(scaled, "screenshot")
	if err != nil {
		return desktoplink.Answer{}, err
	}
	sz := scaled.Bounds().Size()
	note := fmt.Sprintf("%s (window %s), %d×%d pixels", label(w), w.ID, sz.X, sz.Y)
	if scale != 1 {
		ws := img.Bounds().Size()
		note += fmt.Sprintf(", scaled from the window's %d×%d", ws.X, ws.Y)
	}
	note += ". Positions in the other tools are pixels of this image."
	if !w.Focused {
		note += " The window is not focused: focus_window brings it to the front before an input."
	}
	return desktoplink.Answer{Content: []desktoplink.Content{{Type: "image", Data: data, MimeType: "image/png"}, {Type: "text", Text: note}}, Window: label(w)}, nil
}

func (a *Agent) zoom(ctx context.Context, region []int) (desktoplink.Answer, error) {
	if len(region) != 4 {
		return desktoplink.Answer{}, errors.New("region is [x0, y0, x1, y1]")
	}
	a.mu.Lock()
	id := a.last
	s, ok := a.shots[id]
	a.mu.Unlock()
	if !ok {
		return desktoplink.Answer{}, outside("there is no screenshot yet, and a region is pixels of one: take a screenshot first")
	}
	x0, y0, x1, y1 := region[0], region[1], region[2], region[3]
	if x0 >= x1 || y0 >= y1 || x0 < 0 || y0 < 0 || x1 > s.size.X || y1 > s.size.Y {
		return desktoplink.Answer{}, outside("the region [%d, %d, %d, %d] is not inside the last screenshot, %d×%d", x0, y0, x1, y1, s.size.X, s.size.Y)
	}
	ws, err := a.b.Windows(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	var w Window
	for _, c := range ws {
		if c.ID == id {
			w = c
		}
	}
	if w.ID == "" {
		return desktoplink.Answer{}, outside("the window of the last screenshot is gone")
	}
	img, err := a.b.Capture(ctx, id)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	r := image.Rect(int(float64(x0)/s.scale), int(float64(y0)/s.scale), int(math.Ceil(float64(x1)/s.scale)), int(math.Ceil(float64(y1)/s.scale))).Add(img.Bounds().Min).Intersect(img.Bounds())
	crop := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			crop.Set(x, y, img.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	scaled, _ := fit(crop, a.opt.MaxEdge, 4)
	data, err := a.encode(scaled, "zoom")
	if err != nil {
		return desktoplink.Answer{}, err
	}
	sz := scaled.Bounds().Size()
	note := fmt.Sprintf("%s, the region [%d, %d, %d, %d] of the last screenshot, shown at %d×%d. Positions in the other tools are still pixels of the last screenshot.", label(w), x0, y0, x1, y1, sz.X, sz.Y)
	return desktoplink.Answer{Content: []desktoplink.Content{{Type: "image", Data: data, MimeType: "image/png"}, {Type: "text", Text: note}}, Window: label(w)}, nil
}

// encode makes the PNG, and keeps it where the person asked.
func (a *Agent) encode(img image.Image, kind string) (string, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return "", err
	}
	if a.opt.Keep != "" {
		a.mu.Lock()
		a.kept++
		n := a.kept
		a.mu.Unlock()
		if err := os.MkdirAll(a.opt.Keep, 0o700); err == nil {
			_ = os.WriteFile(filepath.Join(a.opt.Keep, fmt.Sprintf("%s-%04d.png", kind, n)), buf.Bytes(), 0o600)
		}
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Kept is how many images were saved where the person asked.
func (a *Agent) Kept() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.kept
}

// withModifiers holds the keys of text down around act, releasing them
// whatever act does.
func (a *Agent) withModifiers(ctx context.Context, w Window, text string, act func() error) error {
	var mods []uint32
	if strings.TrimSpace(text) != "" {
		keys, err := parseCombo(text)
		if err != nil {
			return err
		}
		for _, k := range keys {
			if !modifierKeys[k] {
				return fmt.Errorf("text holds modifier keys during the action (shift, ctrl, alt), and %q is not one", text)
			}
			if a.opt.Scope == ScopeApp && clickModifierTaken(k) {
				return outside("the desktop takes a click with super or alt held (to move or resize windows), so it is not sent in this scope")
			}
		}
		mods = keys
	}
	pressed := 0
	defer func() {
		for i := pressed - 1; i >= 0; i-- {
			_ = a.b.Key(context.WithoutCancel(ctx), mods[i], false)
		}
	}()
	for _, k := range mods {
		if err := a.b.Key(ctx, k, true); err != nil {
			return err
		}
		pressed++
	}
	return act()
}

func (a *Agent) click(ctx context.Context, tool string, args callArgs) (desktoplink.Answer, error) {
	w, err := a.target(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	var p image.Point
	if args.Coordinate != nil {
		if p, err = a.toWindow(w, args.Coordinate, "coordinate"); err != nil {
			return desktoplink.Answer{}, err
		}
	} else {
		cp, in, err := a.b.Cursor(ctx, w.ID)
		if err != nil {
			return desktoplink.Answer{}, err
		}
		if !in {
			return desktoplink.Answer{}, outside("the pointer is outside %s's window, so the click was not sent", w.App)
		}
		p = cp
	}
	button, times := ButtonLeft, 1
	switch tool {
	case "right_click":
		button = ButtonRight
	case "middle_click":
		button = ButtonMiddle
	case "double_click":
		times = 2
	case "triple_click":
		times = 3
	}
	err = a.withModifiers(ctx, w, args.text(), func() error {
		if _, err := a.check(ctx, w.ID, p); err != nil {
			return err
		}
		if err := a.b.Move(ctx, w.ID, p); err != nil {
			return err
		}
		for i := 0; i < times; i++ {
			if _, err := a.check(ctx, w.ID, p); err != nil {
				return err
			}
			if err := a.press(ctx, button); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return desktoplink.Answer{}, err
	}
	at := a.fromWindow(w.ID, p)
	return a.done(w, fmt.Sprintf("%s at [%d, %d] in %s.", strings.ReplaceAll(tool, "_", " "), at.X, at.Y, label(w))), nil
}

// press clicks a button: down then up, the up sent whatever happens.
func (a *Agent) press(ctx context.Context, b Button) error {
	if err := a.b.Button(ctx, b, true); err != nil {
		return err
	}
	return a.b.Button(context.WithoutCancel(ctx), b, false)
}

func (a *Agent) done(w Window, text string) desktoplink.Answer {
	return desktoplink.Answer{Content: []desktoplink.Content{{Type: "text", Text: text}}, Window: label(w)}
}

func (a *Agent) drag(ctx context.Context, args callArgs) (desktoplink.Answer, error) {
	w, err := a.target(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	from, err := a.toWindow(w, args.StartCoordinate, "start_coordinate")
	if err != nil {
		return desktoplink.Answer{}, err
	}
	to, err := a.toWindow(w, args.Coordinate, "coordinate")
	if err != nil {
		return desktoplink.Answer{}, err
	}
	err = a.withModifiers(ctx, w, args.text(), func() error {
		if _, err := a.check(ctx, w.ID, from); err != nil {
			return err
		}
		if err := a.b.Move(ctx, w.ID, from); err != nil {
			return err
		}
		if err := a.b.Button(ctx, ButtonLeft, true); err != nil {
			return err
		}
		// The button is released whatever happens next: a press left down
		// is the one state a stop could not undo.
		defer func() { _ = a.b.Button(context.WithoutCancel(ctx), ButtonLeft, false) }()
		const steps = 8
		for i := 1; i <= steps; i++ {
			p := image.Pt(from.X+(to.X-from.X)*i/steps, from.Y+(to.Y-from.Y)*i/steps)
			if _, err := a.check(ctx, w.ID, p); err != nil {
				return err
			}
			if err := a.b.Move(ctx, w.ID, p); err != nil {
				return err
			}
			if err := a.opt.Sleep(ctx, 15*time.Millisecond); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return desktoplink.Answer{}, err
	}
	f, t := a.fromWindow(w.ID, from), a.fromWindow(w.ID, to)
	return a.done(w, fmt.Sprintf("dragged from [%d, %d] to [%d, %d] in %s.", f.X, f.Y, t.X, t.Y, label(w))), nil
}

func (a *Agent) move(ctx context.Context, args callArgs) (desktoplink.Answer, error) {
	w, err := a.target(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	p, err := a.toWindow(w, args.Coordinate, "coordinate")
	if err != nil {
		return desktoplink.Answer{}, err
	}
	if _, err := a.check(ctx, w.ID, p); err != nil {
		return desktoplink.Answer{}, err
	}
	if err := a.b.Move(ctx, w.ID, p); err != nil {
		return desktoplink.Answer{}, err
	}
	at := a.fromWindow(w.ID, p)
	return a.done(w, fmt.Sprintf("the pointer is at [%d, %d] in %s.", at.X, at.Y, label(w))), nil
}

func (a *Agent) cursor(ctx context.Context) (desktoplink.Answer, error) {
	w, err := a.target(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	p, in, err := a.b.Cursor(ctx, w.ID)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	if !in {
		return a.done(w, fmt.Sprintf("the pointer is outside %s's window.", label(w))), nil
	}
	at := a.fromWindow(w.ID, p)
	return a.done(w, fmt.Sprintf("the pointer is at [%d, %d] in %s.", at.X, at.Y, label(w))), nil
}

func (a *Agent) scroll(ctx context.Context, args callArgs) (desktoplink.Answer, error) {
	d := Direction(args.ScrollDirection)
	switch d {
	case ScrollUp, ScrollDown, ScrollLeft, ScrollRight:
	default:
		return desktoplink.Answer{}, errors.New("scroll_direction is up, down, left or right")
	}
	if args.ScrollAmount < 1 || args.ScrollAmount > 50 {
		return desktoplink.Answer{}, errors.New("scroll_amount is 1 to 50 steps")
	}
	w, err := a.target(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	p, err := a.toWindow(w, args.Coordinate, "coordinate")
	if err != nil {
		return desktoplink.Answer{}, err
	}
	err = a.withModifiers(ctx, w, args.text(), func() error {
		if _, err := a.check(ctx, w.ID, p); err != nil {
			return err
		}
		if err := a.b.Move(ctx, w.ID, p); err != nil {
			return err
		}
		if _, err := a.check(ctx, w.ID, p); err != nil {
			return err
		}
		return a.b.Scroll(ctx, d, args.ScrollAmount)
	})
	if err != nil {
		return desktoplink.Answer{}, err
	}
	at := a.fromWindow(w.ID, p)
	return a.done(w, fmt.Sprintf("scrolled %s %d at [%d, %d] in %s.", d, args.ScrollAmount, at.X, at.Y, label(w))), nil
}

// MaxTypeLength bounds one type call's text, in characters.
const MaxTypeLength = 4096

// shifter is a backend whose keyboard needs shift held for some keysyms.
type shifter interface {
	NeedsShift(keysym uint32) bool
}

func (a *Agent) typeText(ctx context.Context, text *string) (desktoplink.Answer, error) {
	if text == nil || *text == "" {
		return desktoplink.Answer{}, errors.New("text is what to type")
	}
	rs := []rune(*text)
	if len(rs) > MaxTypeLength {
		return desktoplink.Answer{}, fmt.Errorf("text is at most %d characters a call", MaxTypeLength)
	}
	keys := make([]uint32, len(rs))
	for i, r := range rs {
		k, err := runeKeysym(r)
		if err != nil {
			return desktoplink.Answer{}, err
		}
		keys[i] = k
	}
	w, err := a.target(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	sh, _ := a.b.(shifter)
	for i, k := range keys {
		if _, err := a.focused(ctx, w.ID); err != nil {
			var r *Refusal
			if i == 0 || !errors.As(err, &r) || r.Reason != receipt.DesktopOutsideWindow {
				return desktoplink.Answer{}, err
			}
			return desktoplink.Answer{}, outside("the focus left %s's window after %d of %d characters, so the rest were not typed", w.App, i, len(keys))
		}
		shift := sh != nil && sh.NeedsShift(k)
		if err := a.tap(ctx, k, shift); err != nil {
			return desktoplink.Answer{}, err
		}
	}
	return a.done(w, fmt.Sprintf("typed %d characters in %s.", len(keys), label(w))), nil
}

// focused is the check before a key: the window is still allowed and
// focused.
func (a *Agent) focused(ctx context.Context, id string) (Window, error) {
	w, err := a.target(ctx)
	if err != nil {
		return Window{}, err
	}
	if w.ID != id {
		return Window{}, outside("the focus moved to another window, so the key was not sent")
	}
	return w, nil
}

// tap presses and releases a key, with shift held around it when asked;
// every release is sent whatever happens.
func (a *Agent) tap(ctx context.Context, k uint32, shift bool) error {
	bg := context.WithoutCancel(ctx)
	if shift {
		if err := a.b.Key(ctx, keyShiftL, true); err != nil {
			return err
		}
		defer func() { _ = a.b.Key(bg, keyShiftL, false) }()
	}
	if err := a.b.Key(ctx, k, true); err != nil {
		return err
	}
	return a.b.Key(bg, k, false)
}

func (a *Agent) combo(ctx context.Context, text *string) ([]uint32, Window, error) {
	if text == nil {
		return nil, Window{}, errors.New("text names the key or the combination")
	}
	keys, err := parseCombo(*text)
	if err != nil {
		return nil, Window{}, err
	}
	w, err := a.target(ctx)
	if err != nil {
		return nil, Window{}, err
	}
	if a.opt.Scope == ScopeApp && systemCombo(keys) {
		return nil, Window{}, outside("%s is a key the desktop takes, not %s's window, so it is not sent in this scope", *text, w.App)
	}
	return keys, w, nil
}

// pressCombo presses the keys in order and releases them in reverse,
// every release sent whatever happens; hold waits between.
func (a *Agent) pressCombo(ctx context.Context, w Window, keys []uint32, hold time.Duration) error {
	if _, err := a.focused(ctx, w.ID); err != nil {
		return err
	}
	bg := context.WithoutCancel(ctx)
	pressed := 0
	defer func() {
		for i := pressed - 1; i >= 0; i-- {
			_ = a.b.Key(bg, keys[i], false)
		}
	}()
	for _, k := range keys {
		if err := a.b.Key(ctx, k, true); err != nil {
			return err
		}
		pressed++
	}
	if hold > 0 {
		return a.opt.Sleep(ctx, hold)
	}
	return nil
}

func (a *Agent) key(ctx context.Context, text *string) (desktoplink.Answer, error) {
	keys, w, err := a.combo(ctx, text)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	if err := a.pressCombo(ctx, w, keys, 0); err != nil {
		return desktoplink.Answer{}, err
	}
	return a.done(w, fmt.Sprintf("pressed %s in %s.", *text, label(w))), nil
}

func (a *Agent) holdKey(ctx context.Context, text *string, seconds float64) (desktoplink.Answer, error) {
	if seconds <= 0 || seconds > desktoplink.MaxHoldKeySeconds {
		return desktoplink.Answer{}, fmt.Errorf("duration is more than 0 and at most %d seconds", desktoplink.MaxHoldKeySeconds)
	}
	keys, w, err := a.combo(ctx, text)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	if err := a.pressCombo(ctx, w, keys, time.Duration(seconds*float64(time.Second))); err != nil {
		return desktoplink.Answer{}, err
	}
	return a.done(w, fmt.Sprintf("held %s for %g seconds in %s.", *text, seconds, label(w))), nil
}

func (a *Agent) wait(ctx context.Context, seconds float64) (desktoplink.Answer, error) {
	if seconds <= 0 || seconds > desktoplink.MaxWaitSeconds {
		return desktoplink.Answer{}, fmt.Errorf("duration is more than 0 and at most %d seconds", desktoplink.MaxWaitSeconds)
	}
	if err := a.opt.Sleep(ctx, time.Duration(seconds*float64(time.Second))); err != nil {
		return desktoplink.Answer{}, err
	}
	return desktoplink.TextAnswer(fmt.Sprintf("waited %g seconds.", seconds), false), nil
}

// windowLine is one window as the windows tool lists it.
type windowLine struct {
	ID      string `json:"id"`
	App     string `json:"application"`
	Title   string `json:"title,omitempty"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Focused bool   `json:"focused"`
}

func (a *Agent) windows(ctx context.Context) (desktoplink.Answer, error) {
	ws, err := a.b.Windows(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	lines := make([]windowLine, 0, len(ws))
	for _, w := range ws {
		lines = append(lines, windowLine{ID: w.ID, App: w.App, Title: w.Title, Width: w.Size.X, Height: w.Size.Y, Focused: w.Focused})
	}
	b, err := json.Marshal(map[string]any{"windows": lines})
	if err != nil {
		return desktoplink.Answer{}, err
	}
	text := string(b)
	if len(ws) == 0 {
		text += "\nNone of the windows this run may use is open."
	}
	return desktoplink.TextAnswer(text, false), nil
}

func (a *Agent) focus(ctx context.Context, id string) (desktoplink.Answer, error) {
	if id == "" {
		return desktoplink.Answer{}, errors.New("window is the id windows gives")
	}
	ws, err := a.b.Windows(ctx)
	if err != nil {
		return desktoplink.Answer{}, err
	}
	for _, w := range ws {
		if w.ID == id {
			if err := a.b.Focus(ctx, id); err != nil {
				return desktoplink.Answer{}, err
			}
			return a.done(w, fmt.Sprintf("%s is in front and has the keyboard; take a screenshot before an input.", label(w))), nil
		}
	}
	return desktoplink.Answer{}, outside("%q is not a window this run may use: windows lists them", id)
}

// fit scales img so its longest edge is at most maxEdge, and up by at most
// up times to reach it; it returns the image and its scale against img.
func fit(img image.Image, maxEdge int, up float64) (image.Image, float64) {
	b := img.Bounds()
	long := b.Dx()
	if b.Dy() > long {
		long = b.Dy()
	}
	if long == 0 {
		return img, 1
	}
	scale := math.Min(float64(maxEdge)/float64(long), up)
	if scale >= 1 && up <= 1 {
		return img, 1
	}
	w := int(math.Max(1, math.Round(float64(b.Dx())*scale)))
	h := int(math.Max(1, math.Round(float64(b.Dy())*scale)))
	if w == b.Dx() && h == b.Dy() {
		return img, 1
	}
	return resample(img, w, h), float64(w) / float64(b.Dx())
}

// resample scales img to w×h: an area average when shrinking, the nearest
// pixel when enlarging.
func resample(img image.Image, w, h int) *image.RGBA {
	src := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	for y := 0; y < src.Rect.Dy(); y++ {
		for x := 0; x < src.Rect.Dx(); x++ {
			src.Set(x, y, img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y))
		}
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, (y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, (x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, n uint32
			for yy := y0; yy < y1 && yy < sh; yy++ {
				i := yy*src.Stride + x0*4
				for xx := x0; xx < x1 && xx < sw; xx++ {
					r += uint32(src.Pix[i])
					g += uint32(src.Pix[i+1])
					b += uint32(src.Pix[i+2])
					n++
					i += 4
				}
			}
			if n == 0 {
				n = 1
			}
			j := y*dst.Stride + x*4
			dst.Pix[j], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return dst
}
