package meeting

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDetector hands the manager the calls a test sends, and counts its
// watches.
type fakeDetector struct {
	mu      sync.Mutex
	watches int
	active  int
	events  chan CallEvent
}

func newFakeDetector() *fakeDetector { return &fakeDetector{events: make(chan CallEvent)} }

func (d *fakeDetector) Watch(ctx context.Context, events chan<- CallEvent) error {
	d.mu.Lock()
	d.watches++
	d.active++
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.active--
		d.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-d.events:
			select {
			case events <- ev:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (d *fakeDetector) watching() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active > 0
}

var zoom = Call{App: "Zoom Meeting", Binary: "zoom", PID: "4242"}

// detecting is a harness whose manager watches a fake detector, with offers
// that lapse after lapse.
func detecting(t *testing.T, lapse time.Duration) (*harness, *fakeDetector) {
	t.Helper()
	d := newFakeDetector()
	h := newHarnessWith(t, func(m *Manager) {
		m.Detector = d
		m.OfferFor = lapse
	})
	waitFor(t, "the detector to be watched", d.watching)
	return h, d
}

func (h *harness) offer() *Offer { return h.m.Status().Offer }

func (h *harness) lastRaised(tag string) (Notice, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.raised) - 1; i >= 0; i-- {
		if h.raised[i].Tag == tag {
			return h.raised[i], true
		}
	}
	return Notice{}, false
}

func (h *harness) withdrew(tag string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, w := range h.withdrawn {
		if w == tag {
			return true
		}
	}
	return false
}

// lastShown is what the tray was last told, and whether it was told at all.
func (h *harness) lastShown() (*RecordingStatus, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.shown) == 0 {
		return nil, false
	}
	return h.shown[len(h.shown)-1], true
}

func actionIDs(n Notice) []string {
	var out []string
	for _, a := range n.Actions {
		out = append(out, a.ID)
	}
	return out
}

func TestADetectedCallIsOnlyOfferedAndLapses(t *testing.T) {
	h, d := detecting(t, 150*time.Millisecond)
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the offer", func() bool { return h.offer() != nil })
	n, ok := h.lastRaised(tagOffer)
	if !ok || !strings.Contains(n.Title, "Zoom Meeting") || !strings.Contains(n.Body, "consent") {
		t.Fatalf("the offer's notification = %+v, want one naming the app with the consent sentence", n)
	}
	o := h.offer()
	if ids := actionIDs(n); len(ids) != 2 || ids[0] != ActionRecord+":"+o.ID || ids[1] != ActionNotNow+":"+o.ID {
		t.Fatalf("the offer's buttons = %v, want Record and Not now", ids)
	}
	waitFor(t, "the offer to lapse", func() bool { return h.offer() == nil })
	if !h.withdrew(tagOffer) {
		t.Fatal("the lapsed offer's notification was not taken away")
	}
	if n := h.rec.count(); n != 0 {
		t.Fatalf("the recorder was asked %d times for a call nobody chose to record", n)
	}
	if ms, _ := h.m.Meetings(); len(ms) != 0 {
		t.Fatalf("a lapsed offer made a meeting: %+v", ms)
	}
	// The same call again, as the detector would report a stream changing:
	// it is not offered twice.
	d.events <- CallEvent{Call: zoom}
	time.Sleep(50 * time.Millisecond)
	if h.offer() != nil {
		t.Fatal("a call was offered again before it ended")
	}
	// Once it has ended, the next call is a new meeting and is offered.
	d.events <- CallEvent{Call: zoom, Ended: true}
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the next call's offer", func() bool { return h.offer() != nil })
	if _, err := h.m.AcceptOffer(context.Background(), o.ID); !errors.Is(err, ErrNoOffer) {
		t.Fatalf("Record on a lapsed offer: %v, want ErrNoOffer", err)
	}
	if n := h.rec.count(); n != 0 {
		t.Fatalf("an answer to a lapsed offer recorded (%d)", n)
	}
}

func TestNotNowRecordsNothingAndTheEndWithdrawsTheOffer(t *testing.T) {
	h, d := detecting(t, time.Minute)
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the offer", func() bool { return h.offer() != nil })
	if err := h.m.HandleAction(ActionNotNow + ":" + h.offer().ID); err != nil {
		t.Fatal(err)
	}
	if h.offer() != nil || !h.withdrew(tagOffer) || h.rec.count() != 0 {
		t.Fatalf("after Not now: offer %+v, withdrawn %v, recorder asked %d times", h.offer(), h.withdrew(tagOffer), h.rec.count())
	}
	d.events <- CallEvent{Call: zoom, Ended: true}
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the next call's offer", func() bool { return h.offer() != nil })
	h.mu.Lock()
	h.withdrawn = nil
	h.mu.Unlock()
	d.events <- CallEvent{Call: zoom, Ended: true}
	waitFor(t, "the ended call's offer to go", func() bool { return h.offer() == nil && h.withdrew(tagOffer) })
	if n := h.rec.count(); n != 0 {
		t.Fatalf("the recorder was asked %d times", n)
	}
}

func TestRecordOnTheOfferRecordsThatCallAndShowsIt(t *testing.T) {
	h, d := detecting(t, time.Minute)
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(1600)}}
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the offer", func() bool { return h.offer() != nil })
	if err := h.m.HandleAction(ActionRecord + ":" + h.offer().ID); err != nil {
		t.Fatal(err)
	}
	st := h.m.Status().Recording
	if st == nil || st.How != FromOffer || st.App != "Zoom Meeting" {
		t.Fatalf("recording = %+v, want Zoom from the offer", st)
	}
	h.rec.mu.Lock()
	asked := append([]Call(nil), h.rec.asked...)
	h.rec.mu.Unlock()
	if len(asked) != 1 || asked[0] != zoom {
		t.Fatalf("the recorder was asked for %+v, want the detected call", asked)
	}
	if h.offer() != nil || !h.withdrew(tagOffer) {
		t.Fatal("the answered offer still stands")
	}
}

func TestAnAppOnTheAutomaticListIsRecordedVisibly(t *testing.T) {
	h, d := detecting(t, time.Minute)
	h.set(func(s *Settings) { s.Automatic = []string{"zoom"} })
	d.events <- CallEvent{Call: Call{App: "Google Chrome", Binary: "chrome", PID: "7"}}
	waitFor(t, "Chrome's offer", func() bool { return h.offer() != nil })
	_ = h.m.DeclineOffer(h.offer().ID)
	d.events <- CallEvent{Call: zoom}
	// The recording shows in the status as it starts, and is announced once
	// the capture runs.
	waitFor(t, "the automatic recording's announcement", func() bool { _, ok := h.lastRaised(tagRecording); return ok })
	st := h.m.Status().Recording
	if st == nil || st.How != Automatically {
		t.Fatalf("recording = %+v, want automatic", st)
	}
	n, ok := h.lastRaised(tagRecording)
	if !ok || !strings.Contains(n.Title, "automatically") || !strings.Contains(n.Body, "Zoom Meeting") || !strings.Contains(n.Body, "consent") {
		t.Fatalf("the automatic recording's notification = %+v", n)
	}
	if shown, ok := h.lastShown(); !ok || shown == nil || shown.How != Automatically {
		t.Fatalf("the tray was told %+v, want the automatic recording", shown)
	}
	if apps := h.m.Apps(); len(apps) != 2 || apps[0].Key != "zoom" || apps[1].Key != "chrome" {
		t.Fatalf("known apps = %+v, want zoom then chrome", apps)
	}
}

// Every way into a recording shows it: the tray is told and the recording's
// notification, with Pause and Stop, is raised.
func TestEveryWayIntoARecordingIsShown(t *testing.T) {
	ways := map[How]func(h *harness, d *fakeDetector){
		ByHand: func(h *harness, _ *fakeDetector) {
			if _, err := h.m.Record(context.Background()); err != nil {
				t.Fatal(err)
			}
		},
		FromOffer: func(h *harness, d *fakeDetector) {
			d.events <- CallEvent{Call: zoom}
			waitFor(t, "the offer", func() bool { return h.offer() != nil })
			if _, err := h.m.AcceptOffer(context.Background(), h.offer().ID); err != nil {
				t.Fatal(err)
			}
		},
		Automatically: func(h *harness, d *fakeDetector) {
			h.set(func(s *Settings) { s.Automatic = []string{"zoom"} })
			d.events <- CallEvent{Call: zoom}
		},
	}
	for how, start := range ways {
		t.Run(string(how), func(t *testing.T) {
			h, d := detecting(t, time.Minute)
			start(h, d)
			waitFor(t, "the recording", func() bool { return h.m.Status().Recording != nil })
			waitFor(t, "the tray", func() bool { st, _ := h.lastShown(); return st != nil && st.How == how })
			n, ok := h.lastRaised(tagRecording)
			if !ok || !n.Resident || strings.Join(actionIDs(n), ",") != ActionPause+","+ActionStop {
				t.Fatalf("the recording's notification = %+v, want a resident one with Pause and Stop", n)
			}
			if err := h.m.HandleAction(ActionPause); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the paused notification", func() bool {
				n, _ := h.lastRaised(tagRecording)
				return strings.Join(actionIDs(n), ",") == ActionResume+","+ActionStop
			})
			if st, _ := h.lastShown(); st == nil || !st.Paused {
				t.Fatalf("the tray was told %+v after Pause", st)
			}
			if err := h.m.HandleAction(ActionStop); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the tray to be told it stopped", func() bool { st, ok := h.lastShown(); return ok && st == nil })
			if !h.withdrew(tagRecording) {
				t.Fatal("the recording's notification stayed after Stop")
			}
		})
	}
}

// The microphone is opened only for a call the detector reported, or at the
// person's own Record: the manager never asks the recorder for anything
// else.
func TestTheMicrophoneWaitsForTheMeetingApp(t *testing.T) {
	d := newFakeDetector()
	// Zoom is on the automatic list from the start: still nothing is
	// recorded until its call is reported.
	h := newHarnessWith(t, func(m *Manager) {
		m.Detector = d
		settings := m.Settings
		m.Settings = func() Settings {
			s := settings()
			s.Automatic = []string{"zoom"}
			return s
		}
	})
	waitFor(t, "the detector to be watched", d.watching)
	time.Sleep(50 * time.Millisecond)
	if n := h.rec.count(); n != 0 {
		t.Fatalf("the recorder was asked %d times before any call", n)
	}
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the automatic recording", func() bool { return h.m.Status().Recording != nil })
	_ = h.m.Stop()
	if _, err := h.m.Record(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.rec.mu.Lock()
	asked := append([]Call(nil), h.rec.asked...)
	h.rec.mu.Unlock()
	if len(asked) != 2 || asked[0] != zoom || asked[1] != (Call{}) {
		t.Fatalf("the recorder was asked for %+v, want the detected call, then the person's own Record", asked)
	}
}

func TestNothingIsWatchedWhileTheFeatureIsOff(t *testing.T) {
	d := newFakeDetector()
	h := newHarnessWith(t, func(m *Manager) { m.Detector = d })
	waitFor(t, "the detector", d.watching)
	h.set(func(s *Settings) { s.On = false })
	h.m.Refresh()
	waitFor(t, "the detector to stop", func() bool { return !d.watching() })
	select {
	case d.events <- CallEvent{Call: zoom}:
		t.Fatal("a call was taken while the feature is off")
	case <-time.After(50 * time.Millisecond):
	}
	h.set(func(s *Settings) { s.On = true })
	h.m.Refresh()
	waitFor(t, "the detector again", d.watching)
}

func TestARecordingStopsAtItsLongestLength(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.MaxLength = 1200 * time.Millisecond })
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(1600)}}
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the recording to stop", func() bool { return h.m.Status().Recording == nil })
	if st := h.meeting(t, mt.ID).State; st == StateRecording {
		t.Fatalf("state = %s", st)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if last := h.notices[len(h.notices)-1]; !strings.Contains(last, "Agentry stopped recording after") {
		t.Fatalf("the last notice = %q, want the stop explained", last)
	}
	if lengthWords(DefaultMaxLength) != "4 hours" || lengthWords(time.Hour) != "1 hour" || lengthWords(30*time.Minute) != "30 minutes" {
		t.Fatalf("lengthWords: %q %q %q", lengthWords(DefaultMaxLength), lengthWords(time.Hour), lengthWords(30*time.Minute))
	}
}

func TestCallsAreReadFromTheStreams(t *testing.T) {
	const me = "100"
	mic := func(pid, binary string) paStream {
		return paStream{Index: 1, Device: 5, App: "x input", Binary: binary, PID: pid}
	}
	play := func(pid, binary, app string) paStream {
		return paStream{Index: 2, Device: 9, App: app, Binary: binary, PID: pid}
	}
	cases := []struct {
		name string
		snap paSnapshot
		want []Call
	}{
		{"mic and playback of one process", paSnapshot{Outputs: []paStream{mic("7", "zoom")}, Inputs: []paStream{play("7", "zoom", "ZOOM VoiceEngine")}},
			[]Call{{App: "ZOOM VoiceEngine", Binary: "zoom", PID: "7"}}},
		{"two processes of one executable", paSnapshot{Outputs: []paStream{mic("7", "chrome")}, Inputs: []paStream{play("8", "chrome", "Google Chrome")}},
			[]Call{{App: "Google Chrome", Binary: "chrome", PID: "7"}}},
		{"a mic with nothing playing", paSnapshot{Outputs: []paStream{mic("7", "zoom")}}, nil},
		{"a corked mic", paSnapshot{Outputs: []paStream{func() paStream { o := mic("7", "zoom"); o.Corked = true; return o }()}, Inputs: []paStream{play("7", "zoom", "Z")}}, nil},
		{"a monitor, not a mic", paSnapshot{Outputs: []paStream{func() paStream { o := mic("7", "obs"); o.OnMonitor = true; return o }()}, Inputs: []paStream{play("7", "obs", "OBS")}}, nil},
		{"a volume meter", paSnapshot{Outputs: []paStream{func() paStream { o := mic("7", "plasmashell"); o.Meter = true; return o }()}, Inputs: []paStream{play("7", "plasmashell", "Plasma")}}, nil},
		{"this app's own streams", paSnapshot{Outputs: []paStream{mic(me, "agentry-desktop")}, Inputs: []paStream{play(me, "agentry-desktop", "Agentry Desktop")}}, nil},
		{"two processes of different apps", paSnapshot{Outputs: []paStream{mic("7", "zoom")}, Inputs: []paStream{play("8", "chrome", "Google Chrome")}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := findCalls(c.snap, me)
			if len(got) != len(c.want) || (len(got) > 0 && got[0] != c.want[0]) {
				t.Fatalf("calls = %+v, want %+v", got, c.want)
			}
		})
	}
	if !isMeter("peaks", nil) || !isMeter("", map[string]string{"stream.monitor": "true"}) || !isMeter("", map[string]string{"resample.peaks": "true"}) || isMeter("speex-float-1", map[string]string{}) {
		t.Fatal("isMeter misreads a stream")
	}
}

// A call dropped and rejoined within the end wait stays one call; one gone
// longer has ended.
func TestADropAndRejoinStaysOneCall(t *testing.T) {
	call := paSnapshot{
		Outputs: []paStream{{Index: 1, Device: 5, Binary: "zoom", PID: "7"}},
		Inputs:  []paStream{{Index: 2, Device: 9, App: "Zoom", Binary: "zoom", PID: "7"}},
	}
	none := paSnapshot{}
	tr := newCallTracker("100")
	t0 := time.Unix(1_000_000, 0)
	at := func(d time.Duration) time.Time { return t0.Add(d) }
	if ev := tr.update(call, at(0)); len(ev) != 1 || ev[0].Ended {
		t.Fatalf("the call's start: %+v", ev)
	}
	if ev := tr.update(none, at(time.Second)); len(ev) != 0 || !tr.pending() {
		t.Fatalf("a drop: %+v, pending %v", ev, tr.pending())
	}
	if ev := tr.update(none, at(20*time.Second)); len(ev) != 0 {
		t.Fatalf("20 s after the drop: %+v", ev)
	}
	if ev := tr.update(call, at(25*time.Second)); len(ev) != 0 || tr.pending() {
		t.Fatalf("a rejoin within the wait: %+v, pending %v", ev, tr.pending())
	}
	tr.update(none, at(26*time.Second))
	if ev := tr.update(none, at(26*time.Second+callEndWait)); len(ev) != 1 || !ev[0].Ended || ev[0].Call.Binary != "zoom" {
		t.Fatalf("the end: %+v", ev)
	}
	if ev := tr.update(call, at(time.Hour)); len(ev) != 1 || ev[0].Ended {
		t.Fatalf("the next call: %+v", ev)
	}
}

// An automatic recording that no notification can show is let go: nothing
// records unseen without the person's Record.
func TestAnAutomaticRecordingNobodyCanSeeIsNotKept(t *testing.T) {
	h, d := detecting(t, time.Minute)
	h.set(func(s *Settings) { s.Automatic = []string{"zoom"} })
	h.mu.Lock()
	h.notifyErr = errors.New("no notification server")
	h.mu.Unlock()
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "the recorder to be asked", func() bool { return h.rec.count() == 1 })
	waitFor(t, "the capture to be let go", func() bool {
		h.rec.mu.Lock()
		defer h.rec.mu.Unlock()
		h.rec.capture.mu.Lock()
		defer h.rec.capture.mu.Unlock()
		return h.rec.capture.stopped
	})
	if st := h.m.Status().Recording; st != nil {
		t.Fatalf("an unseen automatic recording runs: %+v", st)
	}
	if ms, _ := h.m.Meetings(); len(ms) != 0 {
		t.Fatalf("an unseen automatic recording was kept: %+v", ms)
	}
}

// A call of an app on the automatic list takes the place of an offer that
// stands; the offered call waits its turn and is offered once nothing
// records.
func TestAnAutomaticAppTakesThePlaceOfAStandingOffer(t *testing.T) {
	h, d := detecting(t, time.Minute)
	h.set(func(s *Settings) { s.Automatic = []string{"zoom"} })
	chrome := Call{App: "Google Chrome", Binary: "chrome", PID: "7"}
	d.events <- CallEvent{Call: chrome}
	waitFor(t, "Chrome's offer", func() bool { return h.offer() != nil })
	d.events <- CallEvent{Call: zoom}
	waitFor(t, "Zoom's automatic recording", func() bool {
		st := h.m.Status().Recording
		return st != nil && st.How == Automatically
	})
	if h.offer() != nil || !h.withdrew(tagOffer) {
		t.Fatal("Chrome's offer still stands while Zoom records")
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Chrome offered again", func() bool { o := h.offer(); return o != nil && o.App == "Google Chrome" })
	// Zoom's call, stopped by the person, is not recorded again.
	time.Sleep(50 * time.Millisecond)
	if st := h.m.Status().Recording; st != nil {
		t.Fatalf("a call the person stopped was recorded again: %+v", st)
	}
}

// A call that starts while a meeting records is offered once the recording
// ends, if it is still under way.
func TestACallThatStartedDuringARecordingIsOfferedAfter(t *testing.T) {
	h, d := detecting(t, time.Minute)
	if _, err := h.m.Record(context.Background()); err != nil {
		t.Fatal(err)
	}
	d.events <- CallEvent{Call: zoom}
	time.Sleep(50 * time.Millisecond)
	if h.offer() != nil {
		t.Fatal("a call was offered while a meeting records")
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the waiting call's offer", func() bool { o := h.offer(); return o != nil && o.App == "Zoom Meeting" })
}

// A Stop while the capture is starting lets the capture go: no device stays
// open for a recording nobody sees.
func TestAStopWhileTheCaptureStartsLetsItGo(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	h.m.Recorder = recorderFunc(func(ctx context.Context, call Call, tracks Tracks) (Capture, error) {
		<-gate
		return h.rec.Record(ctx, call, tracks)
	})
	errc := make(chan error, 1)
	go func() { _, err := h.m.Record(context.Background()); errc <- err }()
	waitFor(t, "the recording to begin", func() bool { return h.m.Status().Recording != nil })
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	close(gate)
	if err := <-errc; err == nil {
		t.Fatal("a recording stopped as it started went on")
	}
	h.rec.mu.Lock()
	c := h.rec.capture
	h.rec.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		t.Fatal("the capture of a stopped recording was left running")
	}
}

type recorderFunc func(context.Context, Call, Tracks) (Capture, error)

func (f recorderFunc) Record(ctx context.Context, call Call, tracks Tracks) (Capture, error) {
	return f(ctx, call, tracks)
}
