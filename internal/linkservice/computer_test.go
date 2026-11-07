package linkservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// screenBackend is a window system with Blender's window, focused, at
// 1600×900; it records the inputs it is sent and whether it was closed.
type screenBackend struct {
	mu      sync.Mutex
	onKey   func(n int)
	inputs  []string
	closed  bool
	focused bool
	req     ComputerRequest
}

func (s *screenBackend) Session() string  { return "Linux, test" }
func (s *screenBackend) Enforced() string { return "the test holds nothing" }
func (s *screenBackend) Windows(context.Context) ([]computeruse.Window, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []computeruse.Window{{ID: "w", App: "Blender", Size: image.Pt(1600, 900), Focused: s.focused}}, nil
}
func (s *screenBackend) Focus(context.Context, string) error { return nil }
func (s *screenBackend) Capture(context.Context, string) (image.Image, error) {
	img := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	return img, nil
}
func (s *screenBackend) Cursor(context.Context, string) (image.Point, bool, error) {
	return image.Point{}, false, nil
}
func (s *screenBackend) rec(f string, a ...any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("closed")
	}
	s.inputs = append(s.inputs, fmt.Sprintf(f, a...))
	return nil
}
func (s *screenBackend) Move(_ context.Context, id string, p image.Point) error {
	return s.rec("move %s %v", id, p)
}
func (s *screenBackend) Button(_ context.Context, b computeruse.Button, down bool) error {
	return s.rec("button %d %v", b, down)
}
func (s *screenBackend) Scroll(_ context.Context, d computeruse.Direction, n int) error {
	return s.rec("scroll %s %d", d, n)
}
func (s *screenBackend) Key(_ context.Context, k uint32, down bool) error {
	err := s.rec("key %#x %v", k, down)
	s.mu.Lock()
	n, f := len(s.inputs), s.onKey
	s.mu.Unlock()
	if f != nil {
		f(n)
	}
	return err
}
func (s *screenBackend) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
func (s *screenBackend) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.inputs...)
}

// computerRig is a link service with computer use on or off, keeping
// screenshots or not, over a screenBackend.
func computerRig(t *testing.T, on, keep bool) (*rig, *screenBackend) {
	t.Helper()
	r := newRig(t)
	sb := &screenBackend{focused: true}
	r.s.opt.Computer = ComputerOptions{
		Enabled: func() bool { return on },
		Keep:    func() bool { return keep },
		Open: func(_ context.Context, req ComputerRequest) (computeruse.Backend, error) {
			sb.req = req
			return sb, nil
		},
		StopKey: "the stop key",
	}
	return r, sb
}

func bindComputer(t *testing.T, r *rig, reports string) desktop.Bound {
	t.Helper()
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Ref: "acme/scene@1.0.0",
		Servers:     []desktop.BindServer{{ID: "blender", Tools: []string{"get_scene_info"}}},
		ComputerUse: "app", ComputerTools: desktoplink.ComputerToolNames(), Reports: reports})
	if ref != nil {
		t.Fatalf("the bind was refused: %+v", ref)
	}
	return b
}

// Computer use binds only when the person turned it on, holds the backend
// to the bound applications' windows, and answers with what it bound for
// the receipt; with the switch off the bind is refused desktop-not-granted.
func TestComputerUseBindsOnlyWhenTurnedOn(t *testing.T) {
	off, _ := computerRig(t, false, false)
	_, ref := off.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"look"}}}, ComputerUse: "app", ComputerTools: []string{"screenshot"}})
	if ref == nil || ref.Reason != receipt.DesktopNotGranted || !strings.Contains(ref.Message, "Settings, Your computer") {
		t.Fatalf("with computer use off: %+v", ref)
	}
	if len(off.s.Runs()) != 0 {
		t.Fatal("a refused bind left a run bound")
	}
	r, sb := computerRig(t, true, false)
	b := bindComputer(t, r, "")
	c := b.Computer
	if c == nil || c.Scope != "app" || c.MaxEdge != 1280 || c.Enforced != "the test holds nothing" || c.Session != "Linux, test" || c.KeepScreenshots || c.StopKey == "" || len(c.Windows) != 1 || c.Windows[0] != "Blender" {
		t.Fatalf("bound %+v", c)
	}
	if len(sb.req.Apps) != 1 || sb.req.Apps[0].Name != "Blender" || !contains(sb.req.Apps[0].Classes, "blender") {
		t.Fatalf("the backend was opened for %+v", sb.req)
	}
	runs := r.s.Runs()
	if len(runs) != 1 || runs[0].ComputerUse != "app" || runs[0].ComputerApps[0] != "Blender" {
		t.Fatalf("runs %+v", runs)
	}
	// A tool not asked for at bind is refused.
	if _, ref := call(r, b.Token, "computer", "not_a_tool", `{}`); ref == nil || ref.Reason != receipt.DesktopNotGranted {
		t.Fatalf("an unknown tool: %+v", ref)
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// In app scope an input aimed outside the window is refused outside-window
// and nothing reaches the window system; one inside is sent.
func TestAppScopeRefusesAnInputOutsideTheWindow(t *testing.T) {
	r, sb := computerRig(t, true, false)
	b := bindComputer(t, r, "")
	if _, ref := call(r, b.Token, "computer", "screenshot", `{}`); ref != nil {
		t.Fatalf("screenshot: %+v", ref)
	}
	if _, ref := call(r, b.Token, "computer", "left_click", `{"coordinate":[2000,10]}`); ref == nil || ref.Reason != receipt.DesktopOutsideWindow || ref.Server != "computer" {
		t.Fatalf("a click outside: %+v", ref)
	}
	if len(sb.sent()) != 0 {
		t.Fatalf("inputs were sent: %v", sb.sent())
	}
	sb.mu.Lock()
	sb.focused = false
	sb.mu.Unlock()
	if _, ref := call(r, b.Token, "computer", "key", `{"text":"Return"}`); ref == nil || ref.Reason != receipt.DesktopOutsideWindow {
		t.Fatalf("a key with the window not focused: %+v", ref)
	}
	sb.mu.Lock()
	sb.focused = true
	sb.mu.Unlock()
	if _, ref := call(r, b.Token, "computer", "left_click", `{"coordinate":[640,360]}`); ref != nil {
		t.Fatalf("a click inside: %+v", ref)
	}
	if got := sb.sent(); len(got) != 3 || got[0] != "move w (800,450)" {
		t.Fatalf("inputs %v", got)
	}
}

// Stop ends computer use: every later call is refused desktop-stopped,
// the window system's session is closed, and Pause refuses until resumed.
func TestStopRefusesEveryLaterComputerCall(t *testing.T) {
	r, sb := computerRig(t, true, false)
	b := bindComputer(t, r, "")
	if err := r.s.Pause("R"); err != nil {
		t.Fatal(err)
	}
	if _, ref := call(r, b.Token, "computer", "screenshot", `{}`); ref == nil || ref.Reason != receipt.DesktopPaused {
		t.Fatalf("paused: %+v", ref)
	}
	if err := r.s.Resume("R"); err != nil {
		t.Fatal(err)
	}
	if _, ref := call(r, b.Token, "computer", "screenshot", `{}`); ref != nil {
		t.Fatalf("resumed: %+v", ref)
	}
	if err := r.s.Stop("R"); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"screenshot", "left_click", "type"} {
		if _, ref := call(r, b.Token, "computer", tool, `{}`); ref == nil || ref.Reason != receipt.DesktopStopped {
			t.Fatalf("%s after Stop: %+v", tool, ref)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		sb.mu.Lock()
		closed := sb.closed
		sb.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Stop left the window system's session open")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A whole-screen scope is bound only when the person says yes in a native
// dialog at bind, whatever allowed it where the run was compiled.
func TestScreenScopeIsAskedAtBind(t *testing.T) {
	r, _ := computerRig(t, true, false)
	req := desktop.BindRequest{Run: "R", Ref: "acme/watch@1.0.0", ComputerUse: "screen", ComputerTools: []string{"screenshot"}}
	if _, ref := r.s.Bind(context.Background(), req); ref == nil || ref.Reason != receipt.DesktopNotGranted {
		t.Fatalf("with no one to ask: %+v", ref)
	}
	var asked []string
	r.s.opt.Computer.AskScreen = func(_ context.Context, who string) error { asked = append(asked, who); return errors.New("no") }
	if _, ref := r.s.Bind(context.Background(), req); ref == nil || ref.Reason != receipt.DesktopDeclined || len(asked) != 1 || asked[0] != "acme/watch@1.0.0" {
		t.Fatalf("declined: %+v, asked %v", ref, asked)
	}
	r.s.opt.Computer.AskScreen = func(context.Context, string) error { return nil }
	b, ref := r.s.Bind(context.Background(), req)
	if ref != nil || b.Computer == nil || b.Computer.Scope != "screen" {
		t.Fatalf("allowed: %+v %+v", b.Computer, ref)
	}
}

// A pause stops an action in flight at its next input: a long text stops
// being typed, and the call is refused paused.
func TestAPauseStopsAnActionInFlight(t *testing.T) {
	r, sb := computerRig(t, true, false)
	b := bindComputer(t, r, "")
	sb.onKey = func(n int) {
		if n == 6 {
			_ = r.s.Pause("R")
		}
	}
	_, ref := call(r, b.Token, "computer", "type", `{"text":"abcdefghijklmnopqrstuvwxyz"}`)
	if ref == nil || ref.Reason != receipt.DesktopPaused {
		t.Fatalf("a pause mid-type: %+v", ref)
	}
	if n := len(sb.sent()); n > 8 {
		t.Fatalf("typing went on after the pause: %d inputs", n)
	}
}

// Nothing of the screen rests anywhere unless the person keeps it: with the
// switch off no file is written in the run's folder; with it on each
// screenshot is saved there and the bind says so.
func TestScreenshotsAreNotKeptByDefault(t *testing.T) {
	dir := t.TempDir()
	reports := filepath.Join(dir, "reports", "desktop")
	r, _ := computerRig(t, true, false)
	b := bindComputer(t, r, reports)
	if b.Computer.KeepScreenshots {
		t.Fatal("the bind says screenshots are kept with the switch off")
	}
	a, ref := r.s.Call(context.Background(), desktop.CallRequest{Run: "R", Token: b.Token, Server: "computer", Tool: "screenshot", Arguments: json.RawMessage(`{}`)})
	if ref != nil || len(a.Content) == 0 || a.Content[0].Type != "image" || a.Window != "Blender" {
		t.Fatalf("screenshot %+v %+v", a, ref)
	}
	if _, err := os.Stat(reports); !os.IsNotExist(err) {
		t.Fatalf("the reports folder exists with the switch off: %v", err)
	}
	k, _ := computerRig(t, true, true)
	kb := bindComputer(t, k, reports)
	if !kb.Computer.KeepScreenshots {
		t.Fatal("the bind does not say screenshots are kept")
	}
	if _, ref := call(k, kb.Token, "computer", "screenshot", `{}`); ref != nil {
		t.Fatal(ref)
	}
	kept, _ := filepath.Glob(filepath.Join(reports, "*.png"))
	if len(kept) != 1 {
		t.Fatalf("kept %v", kept)
	}
}
