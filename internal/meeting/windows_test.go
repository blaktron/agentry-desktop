package meeting

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// armNow arms a window that is open now: its meeting started a minute ago
// and ends in an hour.
func armNow(t *testing.T, h *harness, title, app string, invitees ...string) Window {
	t.Helper()
	now := time.Now()
	w, err := h.m.Arm(Window{Title: title, App: app, Invitees: invitees, Start: now.Add(-time.Minute), End: now.Add(time.Hour), By: "the Assistant"}, 5*time.Minute, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Arming is refused while the feature is off, for a window that would open
// more than an hour from now, and for a meeting that is over; the same
// meeting armed twice is one window.
func TestArmingIsRefusedOffTooFarAheadAndOver(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	h.set(func(s *Settings) { s.On = false })
	if _, err := h.m.Arm(Window{Title: "Standup", Start: now.Add(10 * time.Minute), End: now.Add(40 * time.Minute)}, 5*time.Minute, 15*time.Minute); !errors.Is(err, ErrOff) {
		t.Fatalf("armed while off: %v", err)
	}
	h.set(func(s *Settings) { s.On = true })
	if _, err := h.m.Arm(Window{Title: "Later", Start: now.Add(66 * time.Minute), End: now.Add(90 * time.Minute)}, 5*time.Minute, 15*time.Minute); !errors.Is(err, ErrTooFarAhead) {
		t.Fatalf("a window opening 61 minutes from now: %v", err)
	}
	if _, err := h.m.Arm(Window{Title: "Gone", Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour)}, 5*time.Minute, 15*time.Minute); !errors.Is(err, ErrOver) {
		t.Fatalf("a meeting that is over: %v", err)
	}
	a, err := h.m.Arm(Window{Title: "Standup", Start: now.Add(64 * time.Minute), End: now.Add(90 * time.Minute)}, 5*time.Minute, 15*time.Minute)
	if err != nil {
		t.Fatalf("a window opening 59 minutes from now: %v", err)
	}
	b, err := h.m.Arm(Window{Title: "Standup", Start: a.Start, End: a.End}, 5*time.Minute, 15*time.Minute)
	if err != nil || b.ID != a.ID || len(h.m.Windows()) != 1 {
		t.Fatalf("armed again: %+v %v, %d windows; want the one window", b, err, len(h.m.Windows()))
	}
	if !a.Opens.Equal(a.Start.Add(-5*time.Minute)) || !a.Closes.Equal(a.End.Add(15*time.Minute)) {
		t.Fatalf("window %s to %s for a meeting %s to %s", a.Opens, a.Closes, a.Start, a.End)
	}
}

// A call detected inside an open window is recorded at once, shown as the
// window's, and its meeting takes the calendar's title and invitees, which
// the transcript says came from the calendar.
func TestACallInsideAnArmedWindowIsRecordedAndShown(t *testing.T) {
	h, d := detecting(t, time.Minute)
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	h.eng.byFile["mic.wav"] = []Segment{{Start: time.Second, End: 2 * time.Second, Text: "Shall we start?"}}
	w := armNow(t, h, "Budget sync", "zoom", "Sam Li", "priya@example.com")
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the recording", func() bool { return h.m.Status().Recording != nil })
	st := h.m.Status().Recording
	if st.How != InWindow || st.Title != "Budget sync" {
		t.Fatalf("recording = %+v, want one made in the window, with its title", st)
	}
	if h.offer() != nil {
		t.Fatal("a call inside an armed window was offered too")
	}
	waitFor(t, "the recording's notification", func() bool { _, ok := h.lastRaised(tagRecording); return ok })
	n, _ := h.lastRaised(tagRecording)
	if !strings.Contains(n.Title, "Budget sync") || !strings.Contains(n.Body, ConsentSentence) || len(n.Actions) != 2 {
		t.Fatalf("the recording's notification = %+v, want the title, the consent sentence, Pause and Stop", n)
	}
	waitFor(t, "the tray", func() bool { shown, ok := h.lastShown(); return ok && shown != nil })
	if ws := h.m.Windows(); len(ws) != 1 || len(ws[0].Meetings) != 1 || ws[0].Meetings[0] != st.ID {
		t.Fatalf("the window = %+v, want it to name the meeting", ws)
	}
	if err := h.m.CancelWindow(w.ID); !errors.Is(err, ErrRecordingIn) {
		t.Fatalf("a window recording now was cancelled: %v", err)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, st.ID).State == StateDone })
	mt := h.meeting(t, st.ID)
	if mt.Title != "Budget sync" || mt.Window != w.ID || strings.Join(mt.Invitees, ",") != "Sam Li,priya@example.com" {
		t.Fatalf("meeting = %+v", mt)
	}
	raw, err := os.ReadFile(mt.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, want := range []string{"title: Budget sync", "title_source: calendar", "participants:\n    - Sam Li\n    - priya@example.com", "# Budget sync"} {
		if !strings.Contains(doc, want) {
			t.Errorf("transcript lacks %q:\n%s", want, doc)
		}
	}
	if !strings.HasSuffix(mt.Transcript, "-budget-sync.md") {
		t.Errorf("transcript named %s, want it named for the meeting", mt.Transcript)
	}
}

// A window takes only a call of the app the calendar names, or a browser's;
// another app's call is offered as any call is. With no window, or one
// cancelled, nothing is recorded without a press.
func TestAWindowTakesOnlyItsAppAndACancelledOneNothing(t *testing.T) {
	h, d := detecting(t, time.Minute)
	w := armNow(t, h, "Budget sync", "zoom")
	discord := Call{App: "Discord", Binary: "discord", PID: "5150"}
	d.events <- CallEvent{Call: discord}
	waitFor(t, "the offer", func() bool { return h.offer() != nil })
	if h.rec.count() != 0 {
		t.Fatal("another app's call was recorded in the window")
	}
	if err := h.m.CancelWindow(w.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.m.CancelWindow(w.ID); !errors.Is(err, ErrNoWindow) {
		t.Fatalf("cancelled twice: %v", err)
	}
	d.events <- CallEvent{Call: zoom}
	time.Sleep(100 * time.Millisecond)
	if h.rec.count() != 0 || len(h.m.Windows()) != 0 {
		t.Fatalf("a cancelled window recorded: %d recordings, windows %+v", h.rec.count(), h.m.Windows())
	}

	if !(Window{App: "Zoom"}).Takes(Call{App: "Google Chrome", Binary: "chrome"}) {
		t.Error("a Zoom window does not take a call in a browser")
	}
	if !(Window{App: "meet"}).Takes(discord) || !(Window{}).Takes(discord) {
		t.Error("a window naming no app it knows does not take any call")
	}
	if (Window{App: "teams"}).Takes(zoom) {
		t.Error("a Teams window takes a Zoom call")
	}
}

// A window in which no call is detected records nothing, and a window that
// closes ends the recording made in it.
func TestAWindowWithNoCallRecordsNothingAndClosingEndsItsRecording(t *testing.T) {
	h, d := detecting(t, time.Minute)
	now := time.Now()
	quiet, err := h.m.Arm(Window{Title: "Quiet", Start: now.Add(-time.Minute), End: now.Add(50 * time.Millisecond)}, 5*time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if h.rec.count() != 0 {
		t.Fatal("a window with no call recorded")
	}
	if ws := h.m.Windows(); len(ws) != 1 || ws[0].ID != quiet.ID || len(ws[0].Meetings) != 0 {
		t.Fatalf("windows = %+v, want the closed one listed with no meeting", ws)
	}
	if _, err := h.m.Arm(Window{Title: "Short", Start: time.Now().Add(-time.Minute), End: time.Now().Add(300 * time.Millisecond)}, 5*time.Minute, 0); err != nil {
		t.Fatal(err)
	}
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the recording", func() bool { return h.m.Status().Recording != nil })
	id := h.m.Status().Recording.ID
	waitFor(t, "the window to end it", func() bool { return h.m.Status().Recording == nil })
	if st := h.meeting(t, id).State; st == StateRecording {
		t.Fatalf("the meeting is still %s", st)
	}
}

// A window's recording that no notification can show is let go, as an
// automatic one is: nobody pressed Record.
func TestAnUnseenWindowRecordingIsLetGo(t *testing.T) {
	h, d := detecting(t, time.Minute)
	h.mu.Lock()
	h.notifyErr = errors.New("no notification server")
	h.mu.Unlock()
	armNow(t, h, "Budget sync", "")
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the recorder to be asked", func() bool { return h.rec.count() == 1 })
	time.Sleep(50 * time.Millisecond)
	if h.m.Status().Recording != nil {
		t.Fatal("a recording nobody could see kept going")
	}
	if ms, _ := h.m.Meetings(); len(ms) != 0 {
		t.Fatalf("an unseen recording was kept: %+v", ms)
	}
}

// Windows outlive a restart: the next launch keeps watching them.
func TestWindowsAreKeptAcrossARestart(t *testing.T) {
	h := newHarness(t)
	w := armNow(t, h, "Budget sync", "zoom", "Sam Li")
	h.m.Shutdown()
	m2 := &Manager{Root: h.m.Root, Settings: h.m.Settings}
	m2.Start(context.Background())
	defer m2.Shutdown()
	ws := m2.Windows()
	if len(ws) != 1 || ws[0].ID != w.ID || ws[0].Title != "Budget sync" || ws[0].Invitees[0] != "Sam Li" || !ws[0].Opens.Equal(w.Opens) {
		t.Fatalf("after a restart: %+v, want %+v", ws, w)
	}
}
