package meeting

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAudio is an audio stack: sessions and defaults the test sets, and
// streams that deliver a steady tone until their device is gone.
type fakeAudio struct {
	mu       sync.Mutex
	sessions []audioSession
	defaults map[endpointFlow]string
	gone     map[string]bool
	refuse   map[string]error
	log      []string
	open     map[string]int
	sessErr  error
}

func newFakeAudio() *fakeAudio {
	return &fakeAudio{
		defaults: map[endpointFlow]string{flowRender: "speakers", flowCapture: "laptop-mic"},
		gone:     map[string]bool{}, refuse: map[string]error{}, open: map[string]int{},
	}
}

func (f *fakeAudio) set(s ...audioSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = s
}

func (f *fakeAudio) Sessions() ([]audioSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sessions), f.sessErr
}

func (f *fakeAudio) Default(flow endpointFlow) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.defaults[flow], nil
}

func flowName(flow endpointFlow) string {
	if flow == flowCapture {
		return "mic"
	}
	return "loopback"
}

func (f *fakeAudio) Record(ep string, flow endpointFlow) (audioStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.refuse[ep]; err != nil {
		return nil, err
	}
	f.log = append(f.log, fmt.Sprintf("open %s %s", flowName(flow), ep))
	f.open[ep]++
	return &fakeStream{f: f, ep: ep, flow: flow}, nil
}

func (f *fakeAudio) Changed(timeout time.Duration) { time.Sleep(min(timeout, 5*time.Millisecond)) }
func (f *fakeAudio) Close()                        {}

func (f *fakeAudio) events() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.log)
}

func (f *fakeAudio) openCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.open {
		n += c
	}
	return n
}

type fakeStream struct {
	f      *fakeAudio
	ep     string
	flow   endpointFlow
	closed bool
}

func (s *fakeStream) Read() ([]int16, error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	if s.f.gone[s.ep] {
		return nil, fmt.Errorf("reading: %w", errDeviceGone)
	}
	v := int16(1000)
	if s.flow == flowRender {
		v = 2000
	}
	out := make([]int16, 320)
	for i := range out {
		out[i] = v
	}
	return out, nil
}

func (s *fakeStream) Close() {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.f.open[s.ep]--
		s.f.log = append(s.f.log, fmt.Sprintf("close %s %s", flowName(s.flow), s.ep))
	}
}

const zoomExe = `C:\Users\p\AppData\Roaming\Zoom\bin\Zoom.exe`

func zoomSession(ep string, flow endpointFlow, active bool) audioSession {
	return audioSession{Endpoint: ep, Flow: flow, PID: 4100, Exe: zoomExe, App: "Zoom Meetings", Active: active}
}

// A call is an app reading a microphone that also plays; this app's own
// sessions, the audio child's included, are never one.
func TestSessionsAreCallsAsOnEveryPlatform(t *testing.T) {
	self := `C:\Program Files\Agentry\agentry-desktop.exe`
	chromeMic := audioSession{Endpoint: "headset-mic", Flow: flowCapture, PID: 7, Exe: `C:\Chrome\chrome.exe`, App: "Google Chrome", Active: true}
	chromeOut := audioSession{Endpoint: "headset", Flow: flowRender, PID: 8, Exe: `C:\Chrome\chrome.exe`, App: "Google Chrome", Active: true}
	ours := audioSession{Endpoint: "laptop-mic", Flow: flowCapture, PID: 9, Exe: self, Active: true}
	oursOut := audioSession{Endpoint: "speakers", Flow: flowRender, PID: 9, Exe: self, Active: true}
	for _, tc := range []struct {
		name     string
		sessions []audioSession
		want     []string
	}{
		{"an app reading and playing", []audioSession{zoomSession("mic", flowCapture, true), zoomSession("speakers", flowRender, true)}, []string{"zoom.exe Zoom Meetings 4100"}},
		{"a microphone not running", []audioSession{zoomSession("mic", flowCapture, false), zoomSession("speakers", flowRender, true)}, nil},
		{"reading but playing nothing", []audioSession{zoomSession("mic", flowCapture, true)}, nil},
		{"another process of the same executable plays", []audioSession{chromeMic, chromeOut}, []string{"chrome.exe Google Chrome 7"}},
		{"this app", []audioSession{ours, oursOut}, nil},
	} {
		var got []string
		for _, c := range findCalls(sessionSnapshot(withoutExe(tc.sessions, strings.ToUpper(self))), "") {
			got = append(got, c.Binary+" "+c.App+" "+c.PID)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: calls %v, want %v", tc.name, got, tc.want)
		}
	}
}

func newRecording(f *fakeAudio, target recordTarget, out io.Writer) (*childRecording, *time.Time) {
	now := time.Unix(1000, 0)
	r := &childRecording{sys: f, out: &frameWriter{w: out}, target: target, logged: map[string]bool{}, now: func() time.Time { return now }}
	return r, &now
}

// The microphone is opened only on the device the app reads, only once it
// reads it, and let go the moment it stops; the call ends after the wait.
func TestTheMicrophoneFollowsTheApp(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true))
	var out bytes.Buffer
	r, now := newRecording(f, recordTarget{PID: 4100, Binary: "zoom.exe"}, &out)
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	if got := f.events(); !slices.Equal(got, []string{"open loopback headset"}) {
		t.Fatalf("before the app reads a microphone: %v", got)
	}
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, false))
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	want := []string{"open loopback headset", "open mic headset-mic", "close mic headset-mic"}
	if got := f.events(); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	*now = now.Add(callEndWait - time.Second)
	_ = r.scan(false)
	if r.ended {
		t.Fatal("ended before the wait was over")
	}
	*now = now.Add(2 * time.Second)
	_ = r.scan(false)
	if !r.ended || !strings.Contains(out.String(), `"type":"ended"`) {
		t.Fatalf("not ended after the wait: %q", out.String())
	}
	if r.farSide != FarSideAppDevice {
		t.Fatalf("far side %q, want the app's device", r.farSide)
	}
}

// The far side follows the app to another device once two scans agree, and
// the new stream is open before the old one closes.
func TestTheFarSideFollowsTheAppToAnotherDevice(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("speakers", flowRender, true), zoomSession("laptop-mic", flowCapture, true))
	var out bytes.Buffer
	r, _ := newRecording(f, recordTarget{PID: 4100, Binary: "zoom.exe"}, &out)
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	_ = r.scan(false)
	if r.farEP != "speakers" {
		t.Fatalf("followed after one scan: %s", r.farEP)
	}
	_ = r.scan(false)
	want := []string{
		"open mic laptop-mic", "open loopback speakers",
		"close mic laptop-mic", "open mic headset-mic",
		"open loopback headset", "close loopback speakers",
	}
	if got := f.events(); !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if r.farSide != FarSideAppDevice {
		t.Fatalf("far side %q", r.farSide)
	}
}

// An app found playing nowhere is recorded from the default output, which
// the recording says is the whole device; a later move to the app's own
// device does not take that back.
func TestAnAppPlayingNowhereRecordsTheDefaultOutput(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("laptop-mic", flowCapture, true))
	var out bytes.Buffer
	r, _ := newRecording(f, recordTarget{PID: 4100}, &out)
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	if r.farSide != FarSideWholeDevice || r.farEP != "speakers" {
		t.Fatalf("far side %q on %q", r.farSide, r.farEP)
	}
	f.set(zoomSession("laptop-mic", flowCapture, true), zoomSession("headset", flowRender, true))
	_ = r.scan(false)
	_ = r.scan(false)
	if r.farEP != "headset" || r.farSide != FarSideWholeDevice {
		t.Fatalf("far side %q on %q", r.farSide, r.farEP)
	}
}

// An app with a session on an output it is not playing to yet is recorded
// from that output, not the whole device; a microphone it holds but does
// not read is left alone.
func TestAnIdleOutputIsStillTheApps(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, false), zoomSession("headset-mic", flowCapture, false))
	var out bytes.Buffer
	r, _ := newRecording(f, recordTarget{PID: 4100}, &out)
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	if r.farEP != "headset" || r.farSide != FarSideAppDevice || r.mic != nil {
		t.Fatalf("far side %q on %q, mic %v", r.farSide, r.farEP, r.mic)
	}
}

// A device that goes away is closed, said in the log, and taken up again at
// the next scan.
func TestADeviceThatGoesIsTakenUpAgain(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	var out bytes.Buffer
	r, _ := newRecording(f, recordTarget{PID: 4100}, &out)
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.gone["headset"] = true
	f.mu.Unlock()
	r.collect()
	if r.far != nil || !r.scanNow || !strings.Contains(out.String(), "a device went away") {
		t.Fatalf("far %v, scanNow %v, out %q", r.far, r.scanNow, out.String())
	}
	f.mu.Lock()
	f.gone["headset"] = false
	f.mu.Unlock()
	_ = r.scan(false)
	if r.farEP != "headset" || r.farSide != FarSideAppDevice {
		t.Fatalf("far side %q on %q after the device came back", r.farSide, r.farEP)
	}
}

// While paused no device is open and none is opened, and the call is still
// watched.
func TestAPauseHoldsNoDevice(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	var out bytes.Buffer
	r, now := newRecording(f, recordTarget{PID: 4100}, &out)
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	r.setPaused(true)
	_ = r.scan(false)
	if n := f.openCount(); n != 0 {
		t.Fatalf("%d streams open while paused", n)
	}
	f.set(zoomSession("headset", flowRender, true))
	_ = r.scan(false)
	*now = now.Add(callEndWait + time.Second)
	_ = r.scan(false)
	if !r.ended || f.openCount() != 0 {
		t.Fatalf("ended %v, %d open", r.ended, f.openCount())
	}
}

// With no app named, the call under way is recorded; with none, the default
// devices, which the recording calls the whole device.
func TestRecordWithNoAppTakesTheCallOrTheDefaults(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	frames := runChild(t, f, wireCommand{Cmd: "record"})
	started := firstEvent(t, frames, "started")
	if started.Call == nil || started.Call.Binary != "zoom.exe" || started.FarSide != FarSideAppDevice {
		t.Fatalf("started %+v", started)
	}
	f = newFakeAudio()
	frames = runChild(t, f, wireCommand{Cmd: "record"})
	started = firstEvent(t, frames, "started")
	if started.Call != nil || started.FarSide != FarSideWholeDevice {
		t.Fatalf("started %+v", started)
	}
	if got := f.events(); !slices.Contains(got, "open mic laptop-mic") || !slices.Contains(got, "open loopback speakers") {
		t.Fatalf("opened %v", got)
	}
}

// A microphone Windows refuses fails the recording's start with the reason.
func TestARefusedMicrophoneFailsTheStart(t *testing.T) {
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true), zoomSession("headset-mic", flowCapture, true))
	f.refuse["headset-mic"] = errors.New("Windows refused access")
	frames := runChild(t, f, wireCommand{Cmd: "record", PID: "4100"})
	ev := firstEvent(t, frames, "error")
	if !strings.Contains(ev.Message, "Windows refused access") {
		t.Fatalf("error %q", ev.Message)
	}
	if n := f.openCount(); n != 0 {
		t.Fatalf("%d streams left open", n)
	}
}

// The sessions the detector reports never include this app's own, its
// audio child's recording among them.
func TestTheDetectorLeavesThisAppOut(t *testing.T) {
	self := `C:\Program Files\Agentry\agentry-desktop.exe`
	f := newFakeAudio()
	f.set(zoomSession("headset", flowRender, true),
		audioSession{Endpoint: "laptop-mic", Flow: flowCapture, PID: 9, Exe: self, Active: true})
	ev := firstEvent(t, runChildAs(t, f, wireCommand{Cmd: "detect"}, self), "sessions")
	for _, s := range ev.Sessions {
		if s.Exe == self {
			t.Fatalf("this app's session was reported: %+v", ev.Sessions)
		}
	}
	if len(ev.Sessions) != 1 {
		t.Fatalf("sessions %+v, want Zoom's alone", ev.Sessions)
	}
}

// runChild runs the child for one command until it has said something,
// then closes its input.
func runChild(t *testing.T, f *fakeAudio, cmd wireCommand) []wireEvent {
	t.Helper()
	return runChildAs(t, f, cmd, "")
}

// runChildAs is runChild for a child whose own executable is self.
func runChildAs(t *testing.T, f *fakeAudio, cmd wireCommand, self string) []wireEvent {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- captureMain(inR, outW, func() (audioSystem, error) { return f, nil }, self)
		_ = outW.Close()
	}()
	b, _ := json.Marshal(cmd)
	if _, err := inW.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
	var events []wireEvent
	rd := bufio.NewReader(outR)
	for len(events) == 0 {
		kind, _, payload, err := readFrame(rd)
		if err != nil {
			break
		}
		var ev wireEvent
		if kind == frameEvent && json.Unmarshal(payload, &ev) == nil && ev.Type != "log" {
			events = append(events, ev)
		}
	}
	_ = inW.Close()
	go func() { _, _ = io.Copy(io.Discard, outR) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the child did not end when its input closed")
	}
	return events
}

func firstEvent(t *testing.T, events []wireEvent, typ string) wireEvent {
	t.Helper()
	for _, ev := range events {
		if ev.Type == typ {
			return ev
		}
	}
	t.Fatalf("no %q event in %+v", typ, events)
	return wireEvent{}
}

// A frame claiming more than a frame can hold is refused, not allocated.
func TestAnOversizedFrameIsRefused(t *testing.T) {
	var b bytes.Buffer
	w := &frameWriter{w: &b}
	if err := w.audio(wireFar, []int16{1, -1}); err != nil {
		t.Fatal(err)
	}
	kind, track, payload, err := readFrame(bufio.NewReader(&b))
	if err != nil || kind != frameAudio || track != wireFar || !bytes.Equal(payload, []byte{1, 0, 0xff, 0xff}) {
		t.Fatalf("%c %d %v %v", kind, track, payload, err)
	}
	bad := []byte{frameAudio, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(bad[2:], maxFrame+1)
	bad = append(bad, make([]byte, maxFrame+1)...)
	if _, _, _, err := readFrame(bufio.NewReader(bytes.NewReader(bad))); err == nil || !strings.Contains(err.Error(), "a frame of") {
		t.Fatalf("a frame over the bound: %v", err)
	}
}

var _ = context.Background
