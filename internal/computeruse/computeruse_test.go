package computeruse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// fakeBackend is a window system of two windows the scope allows, Blender's
// 2560×1440 and a second 800×600 one; it records every input it is sent.
type fakeBackend struct {
	mu      sync.Mutex
	windows []Window
	inputs  []string
	pointer image.Point
	ptrIn   string
	// moveFocus, when set, moves the focus away after that many inputs.
	moveFocus int
	closed    bool
}

func newFake() *fakeBackend {
	return &fakeBackend{windows: []Window{
		{ID: "1", App: "Blender", Title: "scene.blend", Size: image.Pt(2560, 1440), Focused: true},
		{ID: "2", App: "Blender", Title: "Preferences", Size: image.Pt(800, 600)},
	}}
}

func (f *fakeBackend) Session() string  { return "fake" }
func (f *fakeBackend) Enforced() string { return "nothing" }

func (f *fakeBackend) Windows(context.Context) ([]Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Window(nil), f.windows...), nil
}

func (f *fakeBackend) Focus(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.windows {
		f.windows[i].Focused = f.windows[i].ID == id
	}
	return nil
}

func (f *fakeBackend) Capture(_ context.Context, id string) (image.Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.windows {
		if w.ID == id {
			img := image.NewRGBA(image.Rectangle{Max: w.Size})
			for y := 0; y < w.Size.Y; y++ {
				for x := 0; x < w.Size.X; x++ {
					img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 7, A: 255})
				}
			}
			return img, nil
		}
	}
	return nil, ErrNoWindow
}

func (f *fakeBackend) Cursor(_ context.Context, id string) (image.Point, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pointer, f.ptrIn == id, nil
}

func (f *fakeBackend) record(s string) {
	f.inputs = append(f.inputs, s)
	if f.moveFocus > 0 && len(f.inputs) == f.moveFocus {
		for i := range f.windows {
			f.windows[i].Focused = false
		}
	}
}

func (f *fakeBackend) Move(_ context.Context, id string, p image.Point) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pointer, f.ptrIn = p, id
	f.record(fmt.Sprintf("move %s %d,%d", id, p.X, p.Y))
	return nil
}

func (f *fakeBackend) Button(_ context.Context, b Button, down bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("button %d %v", b, down))
	return nil
}

func (f *fakeBackend) Scroll(_ context.Context, d Direction, steps int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("scroll %s %d", d, steps))
	return nil
}

func (f *fakeBackend) Key(_ context.Context, k uint32, down bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("key %#x %v", k, down))
	return nil
}

func (f *fakeBackend) Close() error { f.closed = true; return nil }

func (f *fakeBackend) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inputs...)
}

func call(t *testing.T, a *Agent, tool, args string) (string, error) {
	t.Helper()
	ans, err := a.Call(context.Background(), tool, json.RawMessage(args))
	if err != nil {
		return "", err
	}
	return ans.Text(), nil
}

func refusedOutside(t *testing.T, err error) {
	t.Helper()
	var r *Refusal
	if err == nil || !errors.As(err, &r) || r.Reason != receipt.DesktopOutsideWindow {
		t.Fatalf("want an outside-window refusal, got %v", err)
	}
}

// A screenshot is the focused allowed window scaled to the long edge, and
// every later position is read in that image: a click at the image's
// centre lands at the window's centre.
func TestPositionsArePixelsOfTheScreenshot(t *testing.T) {
	f := newFake()
	a := New(f, Options{Scope: ScopeApp})
	ans, err := a.Call(context.Background(), "screenshot", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ans.Content) != 2 || ans.Content[0].Type != "image" || ans.Content[0].MimeType != "image/png" || ans.Window != "Blender — scene.blend" {
		t.Fatalf("answer %+v", ans)
	}
	raw, _ := base64.StdEncoding.DecodeString(ans.Content[0].Data)
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil || img.Bounds().Dx() != 1280 || img.Bounds().Dy() != 720 {
		t.Fatalf("the screenshot is %v (%v)", img.Bounds(), err)
	}
	if !strings.Contains(ans.Content[1].Text, "1280×720") || !strings.Contains(ans.Content[1].Text, "scaled from the window's 2560×1440") {
		t.Errorf("the note: %q", ans.Content[1].Text)
	}
	if _, err := call(t, a, "left_click", `{"coordinate":[640,360]}`); err != nil {
		t.Fatal(err)
	}
	got := f.sent()
	if len(got) != 3 || got[0] != "move 1 1280,720" || got[1] != "button 1 true" || got[2] != "button 1 false" {
		t.Fatalf("inputs %v", got)
	}
}

// In app scope an input is sent only to an allowed window that is focused,
// at a point inside it: a point outside the screenshot, a click with no
// allowed window focused, and a focus that moves away mid-way are refused
// outside-window, and nothing more is sent.
func TestAppScopeRefusesAnInputOutsideTheWindow(t *testing.T) {
	f := newFake()
	a := New(f, Options{Scope: ScopeApp})
	if _, err := call(t, a, "screenshot", `{}`); err != nil {
		t.Fatal(err)
	}
	_, err := call(t, a, "left_click", `{"coordinate":[5000,10]}`)
	refusedOutside(t, err)
	_, err = call(t, a, "mouse_move", `{"coordinate":[-1,10]}`)
	refusedOutside(t, err)
	if len(f.sent()) != 0 {
		t.Fatalf("inputs were sent: %v", f.sent())
	}
	// No allowed window focused.
	_ = f.Focus(context.Background(), "nowhere")
	_, err = call(t, a, "left_click", `{"coordinate":[10,10]}`)
	refusedOutside(t, err)
	_, err = call(t, a, "type", `{"text":"hello"}`)
	refusedOutside(t, err)
	_, err = call(t, a, "key", `{"text":"Return"}`)
	refusedOutside(t, err)
	if len(f.sent()) != 0 {
		t.Fatalf("inputs were sent with no allowed window focused: %v", f.sent())
	}
	// The focus leaves after the second key: typing stops and says so.
	_ = f.Focus(context.Background(), "1")
	f.moveFocus = 2
	_, err = call(t, a, "type", `{"text":"hello"}`)
	refusedOutside(t, err)
	if got := f.sent(); len(got) != 2 {
		t.Fatalf("typing went on after the focus left: %v", got)
	}
	// A window without a screenshot takes no position.
	f.moveFocus = 0
	_ = f.Focus(context.Background(), "2")
	_, err = call(t, a, "left_click", `{"coordinate":[10,10]}`)
	refusedOutside(t, err)
}

// A key the desktop takes rather than the window (the super key, switching
// windows, ctrl+alt with a key) is refused in app scope and sent in screen
// scope; a key combination is pressed in order and released in reverse.
func TestDesktopKeysAreRefusedInAppScope(t *testing.T) {
	f := newFake()
	a := New(f, Options{Scope: ScopeApp})
	for _, k := range []string{"super", "super+e", "alt+Tab", "ctrl+alt+t", "alt+F2", "ctrl+alt+Delete", "Print"} {
		_, err := call(t, a, "key", `{"text":"`+k+`"}`)
		refusedOutside(t, err)
	}
	if len(f.sent()) != 0 {
		t.Fatalf("a desktop key was sent: %v", f.sent())
	}
	if _, err := call(t, a, "key", `{"text":"ctrl+s"}`); err != nil {
		t.Fatal(err)
	}
	want := []string{"key 0xffe3 true", "key 0x73 true", "key 0x73 false", "key 0xffe3 false"}
	if got := f.sent(); strings.Join(got, ";") != strings.Join(want, ";") {
		t.Fatalf("ctrl+s sent %v", got)
	}
	screen := New(newFake(), Options{Scope: ScopeScreen})
	if _, err := call(t, screen, "key", `{"text":"super"}`); err != nil {
		t.Fatalf("screen scope: %v", err)
	}
}

// A drag releases the button whatever happens: a focus lost mid-way stops
// the drag and still sends the release.
func TestADragNeverLeavesTheButtonDown(t *testing.T) {
	f := newFake()
	a := New(f, Options{Scope: ScopeApp, Sleep: func(context.Context, time.Duration) error { return nil }})
	if _, err := call(t, a, "screenshot", `{}`); err != nil {
		t.Fatal(err)
	}
	f.moveFocus = 4
	_, err := call(t, a, "left_click_drag", `{"start_coordinate":[10,10],"coordinate":[600,300]}`)
	refusedOutside(t, err)
	got := f.sent()
	if got[len(got)-1] != "button 1 false" || !strings.Contains(strings.Join(got, ";"), "button 1 true") {
		t.Fatalf("the drag left the button down: %v", got)
	}
}

// coveredBackend is the fake with a window above Blender's left half.
type coveredBackend struct{ *fakeBackend }

func (c coveredBackend) Covered(_ context.Context, _ string, p image.Point) (bool, error) {
	return p.X < 1280, nil
}

// An input at a point another window covers is refused outside-window in
// app scope, and nothing is sent; an uncovered point goes through.
func TestACoveredPointIsRefused(t *testing.T) {
	f := newFake()
	a := New(coveredBackend{f}, Options{Scope: ScopeApp})
	if _, err := call(t, a, "screenshot", `{}`); err != nil {
		t.Fatal(err)
	}
	_, err := call(t, a, "left_click", `{"coordinate":[100,100]}`)
	refusedOutside(t, err)
	if len(f.sent()) != 0 {
		t.Fatalf("an input under another window was sent: %v", f.sent())
	}
	if _, err := call(t, a, "left_click", `{"coordinate":[1000,100]}`); err != nil {
		t.Fatal(err)
	}
}

// What the person keeps: with no folder nothing is written anywhere; with
// one, each screenshot and zoom is saved there.
func TestScreenshotsAreNotKeptByDefault(t *testing.T) {
	dir := t.TempDir()
	a := New(newFake(), Options{Scope: ScopeApp})
	for range 2 {
		if _, err := call(t, a, "screenshot", `{}`); err != nil {
			t.Fatal(err)
		}
	}
	if a.Kept() != 0 {
		t.Fatal("a screenshot was kept with no folder")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("files appeared: %v", entries)
	}
	keep := filepath.Join(dir, "reports", "desktop")
	k := New(newFake(), Options{Scope: ScopeApp, Keep: keep})
	if _, err := call(t, k, "screenshot", `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, k, "zoom", `{"region":[0,0,100,50]}`); err != nil {
		t.Fatal(err)
	}
	kept, _ := filepath.Glob(filepath.Join(keep, "*.png"))
	if len(kept) != 2 || k.Kept() != 2 {
		t.Fatalf("kept %v", kept)
	}
	fi, _ := os.Stat(kept[0])
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("a kept screenshot is %v", fi.Mode().Perm())
	}
}

// The tools read only their own arguments, and windows and focus_window
// name only allowed windows.
func TestWindowsAndFocus(t *testing.T) {
	f := newFake()
	a := New(f, Options{Scope: ScopeApp})
	text, err := call(t, a, "windows", `{}`)
	if err != nil || !strings.Contains(text, `"id":"1"`) || !strings.Contains(text, `"focused":true`) || !strings.Contains(text, `"width":2560`) {
		t.Fatalf("windows: %q %v", text, err)
	}
	if _, err := call(t, a, "focus_window", `{"window":"2"}`); err != nil {
		t.Fatal(err)
	}
	w, _ := f.Windows(context.Background())
	if !w[1].Focused || w[0].Focused {
		t.Fatalf("focus %+v", w)
	}
	_, err = call(t, a, "focus_window", `{"window":"99"}`)
	refusedOutside(t, err)
	if _, err := a.Call(context.Background(), "screenshot", json.RawMessage(`{"window":"1","extra":1}`)); err == nil {
		t.Fatal("an unknown argument was taken")
	}
	if _, err := call(t, a, "wait", `{"duration":31}`); err == nil {
		t.Fatal("a wait past the bound was taken")
	}
}

// A character outside Latin-1 types through its Unicode keysym, and a
// shifted one holds shift where the backend says it needs it.
func TestTypingSendsEachCharactersKeysym(t *testing.T) {
	f := newFake()
	a := New(f, Options{Scope: ScopeApp})
	if _, err := call(t, a, "type", `{"text":"aЖ\n"}`); err != nil {
		t.Fatal(err)
	}
	want := []string{"key 0x61 true", "key 0x61 false", "key 0x1000416 true", "key 0x1000416 false", "key 0xff0d true", "key 0xff0d false"}
	if got := f.sent(); strings.Join(got, ";") != strings.Join(want, ";") {
		t.Fatalf("typed %v", got)
	}
}
