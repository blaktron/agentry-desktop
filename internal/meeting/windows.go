package meeting

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Armed windows: the Assistant asks, from the person's calendar, that a
// meeting be recorded. A window keeps the meeting's title and invitees and
// runs from a little before its start to a little after its end; a call
// detected inside it is recorded at once and shown like any recording, and
// a window in which no call is detected records nothing.

// ArmAhead is the furthest from now a window may open when it is armed, so
// a calendar read hours earlier never holds a recording open for a meeting
// that moved.
const ArmAhead = time.Hour

// maxWindows bounds the windows kept at once.
const maxWindows = 20

// keepEnded is how long a window that has closed is still listed, so the
// Assistant can see whether it recorded.
const keepEnded = 12 * time.Hour

// Window is an armed window.
type Window struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Invitees are the calendar's attendees, as it lists them: who was
	// invited, not who spoke.
	Invitees []string `json:"invitees,omitempty"`
	// App is the meeting app the calendar names ("zoom", "teams"), if any.
	App   string    `json:"app,omitempty"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Opens and Closes are when a detected call is recorded.
	Opens  time.Time `json:"opens"`
	Closes time.Time `json:"closes"`
	// By is the agent that armed it, as the person calls it.
	By    string    `json:"by,omitempty"`
	Armed time.Time `json:"armed"`
	// Meetings are the recordings made in it.
	Meetings []string `json:"meetings,omitempty"`
}

// Open says whether a call detected at t is recorded in the window.
func (w Window) Open(t time.Time) bool { return !t.Before(w.Opens) && t.Before(w.Closes) }

// Takes says whether the window records a call of this app: any call when
// the calendar names no app it knows, else that app's or a browser's, since
// every one of them can be joined from a browser.
func (w Window) Takes(c Call) bool {
	hint := knownApp(w.App)
	if hint == "" {
		return true
	}
	key := strings.ToLower(c.AppKey() + " " + c.App)
	return strings.Contains(key, hint) || BrowserApp(c.AppKey())
}

// knownApp is the part of an app's name a call of a meeting app the
// calendar names carries, "" for one this does not know.
func knownApp(hint string) string {
	h := strings.ToLower(strings.TrimSpace(hint))
	for _, a := range []string{"zoom", "teams", "webex", "slack", "discord", "skype", "jitsi"} {
		if strings.Contains(h, a) {
			return a
		}
	}
	return ""
}

// BrowserApp says whether an app's key is a web browser's: a browser's
// calls come from any site.
func BrowserApp(key string) bool {
	k := strings.ToLower(key)
	for _, b := range []string{"chrome", "chromium", "firefox", "brave", "msedge", "edge", "opera", "vivaldi", "epiphany", "librewolf", "zen"} {
		if strings.Contains(k, b) {
			return true
		}
	}
	return false
}

// The ways arming is refused.
var (
	ErrTooFarAhead = errors.New("the window would open more than an hour from now: arm it within the hour before the meeting starts")
	ErrOver        = errors.New("that meeting is over")
	ErrNoWindow    = errors.New("no armed window has that id")
	ErrRecordingIn = errors.New("that window is recording a call now: the person can stop it in Agentry")
)

func (m *Manager) windowsPath() string { return filepath.Join(m.Root, "windows.json") }

// readWindowsLocked reads the windows kept, those that closed more than
// keepEnded ago left out.
func (m *Manager) readWindowsLocked() []Window {
	var ws []Window
	if raw, err := os.ReadFile(m.windowsPath()); err == nil {
		_ = json.Unmarshal(raw, &ws)
	}
	now := m.now()
	return slices.DeleteFunc(ws, func(w Window) bool { return now.Sub(w.Closes) > keepEnded })
}

// writeWindowsLocked replaces windows.json whole.
func (m *Manager) writeWindowsLocked(ws []Window) error {
	if ws == nil {
		ws = []Window{}
	}
	raw, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return err
	}
	tmp := m.windowsPath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.windowsPath())
}

// Arm keeps a window from start-lead to end+tail for the meeting w names,
// refused while the feature is off, when it would open more than ArmAhead
// from now, or when it has closed already. A window armed again for the
// same meeting (its start and title) is the one kept, not another.
func (m *Manager) Arm(w Window, lead, tail time.Duration) (Window, error) {
	if !m.Settings().On {
		return Window{}, ErrOff
	}
	now := m.now()
	w.Opens, w.Closes = w.Start.Add(-lead), w.End.Add(tail)
	switch {
	case !w.End.After(w.Start):
		return Window{}, errors.New("the meeting ends before it starts")
	case !w.Closes.After(now):
		return Window{}, ErrOver
	case w.Opens.After(now.Add(ArmAhead)):
		return Window{}, ErrTooFarAhead
	}
	m.mu.Lock()
	ws := m.readWindowsLocked()
	for _, have := range ws {
		if have.Start.Equal(w.Start) && have.Title == w.Title {
			m.mu.Unlock()
			return have, nil
		}
	}
	if len(ws) >= maxWindows {
		m.mu.Unlock()
		return Window{}, fmt.Errorf("at most %d windows are armed at once", maxWindows)
	}
	w.ID, w.Armed, w.Meetings = "w-"+offerID(), now, nil
	ws = append(ws, w)
	err := m.writeWindowsLocked(ws)
	m.mu.Unlock()
	if err != nil {
		return Window{}, err
	}
	m.watchWindow(w)
	m.changed()
	m.consider()
	return w, nil
}

// watchWindow considers the calls under way again when the window opens,
// for a call already going then, and when it closes, which ends a
// recording made in it.
func (m *Manager) watchWindow(w Window) {
	now := m.now()
	if d := w.Opens.Sub(now); d > 0 {
		time.AfterFunc(d, m.consider)
	}
	if d := w.Closes.Sub(now); d > 0 {
		time.AfterFunc(d, func() { m.windowClosed(w.ID) })
	}
}

// windowClosed ends the recording a window started once the window closes.
func (m *Manager) windowClosed(id string) {
	m.mu.Lock()
	r := m.rec
	in := r != nil && r.window == id && !r.stopping
	m.mu.Unlock()
	if in {
		_ = m.finish(r, true)
	}
	m.changed()
}

// Windows are the windows kept, the soonest first.
func (m *Manager) Windows() []Window {
	m.mu.Lock()
	ws := m.readWindowsLocked()
	m.mu.Unlock()
	slices.SortStableFunc(ws, func(a, b Window) int { return a.Opens.Compare(b.Opens) })
	return ws
}

// CancelWindow takes a window away, so nothing is recorded in it from now
// on; one recording now is refused, since its recording is the person's to
// stop.
func (m *Manager) CancelWindow(id string) error {
	m.mu.Lock()
	if r := m.rec; r != nil && r.window == id && !r.stopping {
		m.mu.Unlock()
		return ErrRecordingIn
	}
	ws := m.readWindowsLocked()
	n := len(ws)
	ws = slices.DeleteFunc(ws, func(w Window) bool { return w.ID == id })
	if len(ws) == n {
		m.mu.Unlock()
		return ErrNoWindow
	}
	err := m.writeWindowsLocked(ws)
	m.mu.Unlock()
	if err == nil {
		m.changed()
	}
	return err
}

// windowFor is the open window that takes c at now, if one does.
func (m *Manager) windowForLocked(c Call, now time.Time) (Window, bool) {
	for _, w := range m.readWindowsLocked() {
		if w.Open(now) && w.Takes(c) {
			return w, true
		}
	}
	return Window{}, false
}

// recordedIn adds a meeting to the window it was recorded in.
func (m *Manager) recordedInLocked(windowID, meetingID string) {
	ws := m.readWindowsLocked()
	for i := range ws {
		if ws[i].ID == windowID {
			ws[i].Meetings = append(ws[i].Meetings, meetingID)
		}
	}
	_ = m.writeWindowsLocked(ws)
}

// rearm watches the windows an earlier session kept.
func (m *Manager) rearm() {
	m.mu.Lock()
	ws := m.readWindowsLocked()
	_ = m.writeWindowsLocked(ws)
	m.mu.Unlock()
	for _, w := range ws {
		m.watchWindow(w)
	}
}
