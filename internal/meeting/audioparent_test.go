package meeting

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// memTracks keeps what each track is given.
type memTracks struct {
	mu     sync.Mutex
	tracks map[string]*memTrack
}

type memTrack struct {
	mu      sync.Mutex
	label   Label
	samples int
	ended   bool
}

func (m *memTracks) Open(label Label, name string) (Track, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tracks == nil {
		m.tracks = map[string]*memTrack{}
	}
	t := &memTrack{label: label}
	m.tracks[name] = t
	return t, nil
}

func (m *memTracks) get(name string) (samples int, ended, ok bool) {
	m.mu.Lock()
	t, ok := m.tracks[name]
	m.mu.Unlock()
	if !ok {
		return 0, false, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.samples, t.ended, true
}

func (t *memTrack) Write(s []int16) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.samples += len(s)
}

func (t *memTrack) End() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ended = true
}

// inProcessChild runs the child in this process over pipes. crash, when
// set, ends the child's output as a crashed process's would.
type inProcessChild struct {
	f       *fakeAudio
	spawned atomic.Int32
	mu      sync.Mutex
	crash   []func()
}

func (c *inProcessChild) spawn() (*childProc, error) {
	c.spawned.Add(1)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		captureMain(inR, outW, func() (audioSystem, error) { return c.f, nil }, "")
		_ = outW.Close()
	}()
	crash := func() { _ = outW.CloseWithError(io.ErrUnexpectedEOF); _ = inW.Close() }
	c.mu.Lock()
	c.crash = append(c.crash, crash)
	c.mu.Unlock()
	stop := func() {
		_ = inW.Close()
		go func() { _, _ = io.Copy(io.Discard, outR) }()
		<-done
	}
	return &childProc{in: inW, out: outR, stop: stop}, nil
}

func (c *inProcessChild) crashLast() {
	c.mu.Lock()
	f := c.crash[len(c.crash)-1]
	c.mu.Unlock()
	f()
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A recording through the child writes both sides to their tracks, nothing
// while paused, and ends every track on Stop.
func TestARecordingThroughTheChild(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	child := &inProcessChild{f: f}
	tracks := &memTracks{}
	c, err := childRecorder{spawn: child.spawn}.Record(context.Background(), Call{App: "Zoom Meetings", Binary: "zoom.exe", PID: "4100"}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	if c.FarSide() != FarSideAppDevice || c.Call().Binary != "zoom.exe" {
		t.Fatalf("far side %q, call %+v", c.FarSide(), c.Call())
	}
	eventually(t, "both tracks written", func() bool {
		mic, _, ok1 := tracks.get("mic")
		far, _, ok2 := tracks.get("far")
		return ok1 && ok2 && mic > 0 && far > 0
	})
	if err := c.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the devices let go", func() bool { return f.openCount() == 0 })
	held, _, _ := tracks.get("mic")
	time.Sleep(100 * time.Millisecond)
	if now, _, _ := tracks.get("mic"); now != held {
		t.Fatalf("written while paused: %d then %d", held, now)
	}
	if err := c.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	eventually(t, "written again", func() bool { n, _, _ := tracks.get("mic"); return n > held })
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mic", "far"} {
		if _, ended, _ := tracks.get(name); !ended {
			t.Errorf("%s not ended", name)
		}
	}
	if n := f.openCount(); n != 0 {
		t.Fatalf("%d streams open after Stop", n)
	}
}

// Audio that reaches the app while the recording is paused (sent before
// the child let go of the devices) is not written.
func TestAudioArrivingWhilePausedIsDropped(t *testing.T) {
	tracks := &memTracks{}
	c := &childCapture{tracks: tracks}
	c.write(wireMic, []byte{1, 0, 2, 0})
	c.paused = true
	c.write(wireMic, []byte{3, 0, 4, 0})
	if n, _, _ := tracks.get("mic"); n != 2 {
		t.Fatalf("%d samples written, want the 2 sent before the pause", n)
	}
}

// A far side that plays nothing has its track all the same, so the
// transcript says it was silent.
func TestASilentFarSideStillHasItsTrack(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	f.gone["headset"] = true // the loopback delivers nothing
	tracks := &memTracks{}
	c, err := childRecorder{spawn: (&inProcessChild{f: f}).spawn}.Record(context.Background(), Call{PID: "4100"}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	if _, _, ok := tracks.get("far"); !ok {
		t.Fatal("no far track while the far side is silent")
	}
}

// A child that dies is started again, and the recording goes on on the same
// tracks.
func TestACrashedChildIsStartedAgain(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	child := &inProcessChild{f: f}
	tracks := &memTracks{}
	c, err := childRecorder{spawn: child.spawn}.Record(context.Background(), Call{Binary: "zoom.exe", PID: "4100"}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	eventually(t, "written", func() bool { n, _, _ := tracks.get("far"); return n > 0 })
	child.crashLast()
	eventually(t, "a second child", func() bool { return child.spawned.Load() == 2 })
	before, _, _ := tracks.get("far")
	eventually(t, "written by the second child", func() bool { n, _, _ := tracks.get("far"); return n > before })
	select {
	case <-c.Ended():
		t.Fatal("the recording ended at a crash")
	default:
	}
}

// The child's start failing is the recording's error, with its reason.
func TestAChildThatCannotRecordFailsTheRecording(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	f.refuse["headset-mic"] = io.ErrClosedPipe
	child := &inProcessChild{f: f}
	_, err := childRecorder{spawn: child.spawn}.Record(context.Background(), Call{PID: "4100"}, &memTracks{})
	if err == nil || !strings.Contains(err.Error(), "microphone could not be recorded") {
		t.Fatalf("err %v", err)
	}
}

// The detector reports a call as the child's sessions show it, and its end.
func TestTheDetectorReportsCallsFromTheChild(t *testing.T) {
	f := newFakeAudio()
	child := &inProcessChild{f: f}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan CallEvent, 4)
	done := make(chan error, 1)
	go func() { done <- childDetector{spawn: child.spawn, endWait: 50 * time.Millisecond}.Watch(ctx, events) }()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	var got []string
	select {
	case ev := <-events:
		got = append(got, ev.Call.AppKey()+" started")
	case <-time.After(10 * time.Second):
		t.Fatal("no call")
	}
	f.set()
	select {
	case ev := <-events:
		if !ev.Ended {
			t.Fatalf("%+v", ev)
		}
		got = append(got, ev.Call.AppKey()+" ended")
	case <-time.After(10 * time.Second):
		t.Fatal("no end")
	}
	if !slices.Equal(got, []string{"zoom.exe started", "zoom.exe ended"}) {
		t.Fatalf("%v", got)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func waitForLong(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(200 * time.Millisecond)
	}
}
