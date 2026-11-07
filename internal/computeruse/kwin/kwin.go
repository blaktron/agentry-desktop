// Package kwin follows KDE Plasma's window manager, KWin, through its
// scripting interface: which windows are open, where each is, which one is
// active and where the pointer is. On Wayland no client can ask for any of
// that, and Plasma's remote control posts input to the whole screen, so a
// check that an input lands only in an allowed window needs KWin's own
// account. A script loaded into KWin reports each change back over the
// session bus to this process, and only reports sent by KWin are read.
package kwin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	kwinName   = "org.kde.KWin"
	scripting  = "org.kde.kwin.Scripting"
	scriptIfc  = "org.kde.kwin.Script"
	reportPath = dbus.ObjectPath("/org/secureagentry/KWinState")
	reportIfc  = "org.secureagentry.KWinState"
)

// Window is a window as KWin reports it.
type Window struct {
	ID        string
	Class     string
	Name      string
	Caption   string
	PID       int
	Frame     image.Rectangle
	Client    image.Rectangle
	Active    bool
	Minimized bool
	// Stack is the window's place in KWin's stacking order: a higher one
	// is above a lower one.
	Stack int
}

// State is KWin's account of the windows and the pointer.
type State struct {
	Windows []Window
	Cursor  image.Point
	// Seq counts the reports.
	Seq uint64
}

// Active is the active window, if any.
func (s State) Active() (Window, bool) {
	for _, w := range s.Windows {
		if w.Active {
			return w, true
		}
	}
	return Window{}, false
}

// Tracker follows KWin's windows for one binding.
type Tracker struct {
	bus   *dbus.Conn
	dir   string
	name  string
	nonce string
	owner string

	mu      sync.Mutex
	state   State
	changed chan struct{}
	closed  bool
}

// ScriptDir is where this process writes KWin's scripts while they load:
// the user's runtime folder where there is one.
func ScriptDir() string {
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		return filepath.Join(rt, "agentry-desktop-kwin")
	}
	return filepath.Join(os.TempDir(), "agentry-desktop-kwin")
}

// Available says KWin's scripting interface answers on the bus.
func Available(bus *dbus.Conn) bool {
	var owner string
	return bus.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, kwinName).Store(&owner) == nil && owner != ""
}

// Start loads the reporting script into KWin and waits for its first
// report. dir holds the script file while it is loaded.
func Start(ctx context.Context, bus *dbus.Conn, dir string) (*Tracker, error) {
	var owner string
	if err := bus.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, kwinName).Store(&owner); err != nil {
		return nil, fmt.Errorf("KWin does not answer on the session bus: %w", err)
	}
	nonce, err := randomHex()
	if err != nil {
		return nil, err
	}
	t := &Tracker{bus: bus, dir: dir, name: "agentry-computer-use-" + nonce[:12], nonce: nonce, owner: owner, changed: make(chan struct{})}
	if err := bus.Export(t, reportPath, reportIfc); err != nil {
		return nil, err
	}
	if err := t.load(ctx, t.name, reportScript(bus.Names()[0], nonce)); err != nil {
		_ = bus.Export(nil, reportPath, reportIfc)
		return nil, err
	}
	if _, err := t.Wait(ctx, func(s State) bool { return s.Seq > 0 }); err != nil {
		t.Close()
		return nil, fmt.Errorf("KWin did not report its windows: %w", err)
	}
	return t, nil
}

// load writes a script, loads it into KWin under name and runs it.
func (t *Tracker) load(ctx context.Context, name, js string) error {
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(t.dir, name+".js")
	if err := os.WriteFile(path, []byte(js), 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	obj := t.bus.Object(kwinName, "/Scripting")
	_ = obj.CallWithContext(ctx, scripting+".unloadScript", 0, name).Err
	var id int32
	if err := obj.CallWithContext(ctx, scripting+".loadScript", 0, path, name).Store(&id); err != nil {
		return fmt.Errorf("KWin did not load the script: %w", err)
	}
	if id < 0 {
		return errors.New("KWin refused the script")
	}
	paths := []dbus.ObjectPath{dbus.ObjectPath("/Scripting/Script" + strconv.Itoa(int(id))), dbus.ObjectPath("/" + strconv.Itoa(int(id)))}
	var runErr error
	for _, p := range paths {
		if runErr = t.bus.Object(kwinName, p).CallWithContext(ctx, scriptIfc+".run", 0).Err; runErr == nil {
			return nil
		}
	}
	return fmt.Errorf("KWin did not run the script: %w", runErr)
}

// unload takes a script out of KWin.
func (t *Tracker) unload(name string) {
	_ = t.bus.Object(kwinName, "/Scripting").Call(scripting+".unloadScript", 0, name).Err
}

// Report is the script's report of the windows and the pointer, called by
// KWin; a call from any other sender, or with another nonce, is ignored.
func (t *Tracker) Report(sender dbus.Sender, nonce, body string) *dbus.Error {
	if string(sender) != t.owner || nonce != t.nonce {
		return nil
	}
	var r struct {
		W []struct {
			ID      string    `json:"id"`
			Cls     string    `json:"cls"`
			Name    string    `json:"name"`
			Caption string    `json:"caption"`
			PID     int       `json:"pid"`
			F       []float64 `json:"f"`
			C       []float64 `json:"c"`
			Active  bool      `json:"active"`
			Min     bool      `json:"min"`
			Stack   int       `json:"stack"`
		} `json:"w"`
		CX float64 `json:"cx"`
		CY float64 `json:"cy"`
	}
	if json.Unmarshal([]byte(body), &r) != nil {
		return nil
	}
	s := State{Cursor: image.Pt(int(r.CX), int(r.CY))}
	for _, w := range r.W {
		s.Windows = append(s.Windows, Window{ID: w.ID, Class: w.Cls, Name: w.Name, Caption: w.Caption, PID: w.PID, Frame: rect(w.F), Client: rect(w.C), Active: w.Active, Minimized: w.Min, Stack: w.Stack})
	}
	t.publish(func(st *State) {
		s.Seq = st.Seq + 1
		*st = s
	})
	return nil
}

// Cursor is the script's report of the pointer alone.
func (t *Tracker) Cursor(sender dbus.Sender, nonce, xy string) *dbus.Error {
	if string(sender) != t.owner || nonce != t.nonce {
		return nil
	}
	x, y, ok := strings.Cut(xy, ",")
	if !ok {
		return nil
	}
	fx, err1 := strconv.ParseFloat(x, 64)
	fy, err2 := strconv.ParseFloat(y, 64)
	if err1 != nil || err2 != nil {
		return nil
	}
	t.publish(func(st *State) {
		st.Cursor = image.Pt(int(fx), int(fy))
		st.Seq++
	})
	return nil
}

func (t *Tracker) publish(f func(*State)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	f(&t.state)
	close(t.changed)
	t.changed = make(chan struct{})
}

func rect(v []float64) image.Rectangle {
	if len(v) != 4 {
		return image.Rectangle{}
	}
	x, y := int(v[0]), int(v[1])
	return image.Rect(x, y, x+int(v[2]), y+int(v[3]))
}

// Alive says the KWin that reports is still the one on the bus: a KWin
// that restarted runs no script of this tracker, and its last account is
// stale.
func (t *Tracker) Alive() bool {
	var owner string
	return t.bus.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, kwinName).Store(&owner) == nil && owner == t.owner
}

// State is KWin's latest account.
func (t *Tracker) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Wait waits for an account that satisfies ok.
func (t *Tracker) Wait(ctx context.Context, ok func(State) bool) (State, error) {
	for {
		t.mu.Lock()
		s, ch, closed := t.state, t.changed, t.closed
		t.mu.Unlock()
		if ok(s) {
			return s, nil
		}
		if closed {
			return s, errors.New("the window tracker was closed")
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return s, ctx.Err()
		}
	}
}

// Activate makes the window KWin's active one.
func (t *Tracker) Activate(ctx context.Context, id string) error {
	quoted, _ := json.Marshal(id)
	js := fmt.Sprintf(`for (const w of workspace.windowList()) { if (String(w.internalId) === %s) { if (w.minimized) w.minimized = false; workspace.activeWindow = w; } }`, quoted)
	name := t.name + "-activate"
	err := t.load(ctx, name, js)
	t.unload(name)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = t.Wait(ctx, func(s State) bool {
		w, ok := s.Active()
		return ok && w.ID == id
	})
	if err != nil {
		return errors.New("KWin did not bring the window to the front")
	}
	return nil
}

// Close unloads the script and stops reading its reports.
func (t *Tracker) Close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	close(t.changed)
	t.mu.Unlock()
	t.unload(t.name)
	_ = t.bus.Export(nil, reportPath, reportIfc)
}

// KeepAbove asks KWin to keep this process's windows that are at most
// maxHeight tall above the others, at the top edge of the screen they are
// on, where they cover no dialog: a Wayland client can ask for neither
// itself, and its title is not always the one it set.
func KeepAbove(ctx context.Context, bus *dbus.Conn, dir string, maxHeight int) error {
	t := &Tracker{bus: bus, dir: dir}
	js := fmt.Sprintf(`for (const w of workspace.windowList()) { if (w.pid === %d && w.frameGeometry.height <= %d) { w.keepAbove = true; w.onAllDesktops = true; w.skipTaskbar = true; w.skipSwitcher = true; const a = workspace.clientArea(KWin.MaximizeArea, w); w.frameGeometry = {x: a.x + Math.round((a.width - w.frameGeometry.width) / 2), y: a.y + 8, width: w.frameGeometry.width, height: w.frameGeometry.height}; } }`, os.Getpid(), maxHeight)
	n, err := randomHex()
	if err != nil {
		return err
	}
	name := "agentry-keep-above-" + n[:12]
	defer t.unload(name)
	return t.load(ctx, name, js)
}

func randomHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// reportScript is the script KWin runs: it reports every window's place
// and the active window on each change, and the pointer as it moves.
func reportScript(service, nonce string) string {
	svc, _ := json.Marshal(service)
	n, _ := json.Marshal(nonce)
	return fmt.Sprintf(`const SVC = %s, NONCE = %s, PATH = %q, IFACE = %q;
function geo(g) { return [g.x, g.y, g.width, g.height]; }
function report() {
  const out = [];
  const order = workspace.stackingOrder;
  for (const w of workspace.windowList()) {
    if (!w.normalWindow && !w.dialog && !w.notification && !w.utility && !w.popupWindow && !w.onScreenDisplay && !w.criticalNotification && !w.dock) continue;
    out.push({id: String(w.internalId), cls: String(w.resourceClass), name: String(w.resourceName), caption: String(w.caption), pid: w.pid || 0,
      f: geo(w.frameGeometry), c: geo(w.clientGeometry), active: workspace.activeWindow === w, min: !!w.minimized, stack: order.indexOf(w)});
  }
  callDBus(SVC, PATH, IFACE, "Report", NONCE, JSON.stringify({w: out, cx: workspace.cursorPos.x, cy: workspace.cursorPos.y}));
}
function watch(w) {
  w.frameGeometryChanged.connect(report);
  if (w.stackingOrderChanged) w.stackingOrderChanged.connect(report);
  w.captionChanged.connect(report);
  w.minimizedChanged.connect(report);
}
workspace.windowList().forEach(watch);
workspace.windowAdded.connect(function (w) { watch(w); report(); });
workspace.windowRemoved.connect(report);
workspace.windowActivated.connect(report);
// A window raised without being activated changes the stack alone; where
// this KWin has no such signal, a raise is seen at the next report.
if (workspace.stackingOrderChanged) workspace.stackingOrderChanged.connect(report);
workspace.cursorPosChanged.connect(function () { callDBus(SVC, PATH, IFACE, "Cursor", NONCE, workspace.cursorPos.x + "," + workspace.cursorPos.y); });
report();
`, svc, n, string(reportPath), reportIfc)
}
