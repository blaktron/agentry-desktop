package meeting

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeCoreAudio is a Mac's Core Audio as the capture sees it: the process
// list, the default devices, the microphone permission, and every source
// started, in order.
type fakeCoreAudio struct {
	mu        sync.Mutex
	procs     []caProcess
	out, in   uint32
	allowed   bool
	appTapErr error
	calls     []string
	sources   []*fakeSource
}

type fakeSource struct {
	mu      sync.Mutex
	kind    string // "app", "global", "mic"
	objects []uint32
	frames  uint64
	heard   uint64
	pending int
	stopped bool
}

func (s *fakeSource) Rate() float64 { return SampleRate }
func (s *fakeSource) Read(buf []float32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := min(s.pending, len(buf))
	for i := range n {
		buf[i] = 0.1
	}
	s.pending -= n
	return n
}
func (s *fakeSource) Frames() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.frames }
func (s *fakeSource) Heard() uint64  { s.mu.Lock(); defer s.mu.Unlock(); return s.heard }
func (s *fakeSource) Stop()          { s.mu.Lock(); defer s.mu.Unlock(); s.stopped = true }

// deliver is a second of audio from every running source, heard or silent.
func (f *fakeCoreAudio) deliver(heard bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sources {
		s.mu.Lock()
		if !s.stopped {
			s.frames += SampleRate
			s.pending += SampleRate
			if heard {
				s.heard += SampleRate
			}
		}
		s.mu.Unlock()
	}
}

func (f *fakeCoreAudio) Processes() ([]caProcess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.procs), nil
}
func (f *fakeCoreAudio) DefaultDevice(input bool) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if input {
		return f.in
	}
	return f.out
}
func (f *fakeCoreAudio) SelfObject() uint32 { return 1 }
func (f *fakeCoreAudio) MicAllowed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.allowed
}
func (f *fakeCoreAudio) AskMic() { f.mu.Lock(); defer f.mu.Unlock(); f.calls = append(f.calls, "ask") }
func (f *fakeCoreAudio) StartTap(objects []uint32, global bool) (caSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kind := "app"
	if global {
		kind = "global"
	} else if f.appTapErr != nil {
		f.calls = append(f.calls, "app-refused")
		return nil, f.appTapErr
	}
	s := &fakeSource{kind: kind, objects: slices.Clone(objects)}
	f.sources = append(f.sources, s)
	f.calls = append(f.calls, fmt.Sprintf("%s%v", kind, objects))
	return s, nil
}
func (f *fakeCoreAudio) StartMic() (caSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &fakeSource{kind: "mic"}
	f.sources = append(f.sources, s)
	f.calls = append(f.calls, "mic")
	return s, nil
}

// running are the kinds of the sources running now.
func (f *fakeCoreAudio) running() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, s := range f.sources {
		s.mu.Lock()
		if !s.stopped {
			out = append(out, fmt.Sprintf("%s%v", s.kind, s.objects))
		}
		s.mu.Unlock()
	}
	slices.Sort(out)
	return out
}

func (f *fakeCoreAudio) set(ps ...caProcess) { f.mu.Lock(); f.procs = ps; f.mu.Unlock() }

// A browser call: its helper (object 9) records and plays; its main
// process (object 7) has its own process object.
func browserCall(input bool) []caProcess {
	return []caProcess{
		{Object: 7, PID: 100, Path: chromeApp},
		{Object: 9, PID: 120, Input: input, Output: true, Path: chromeHelper},
		{Object: 11, PID: 300, Output: true, Path: musicApp},
	}
}

func startFake(t *testing.T, f *fakeCoreAudio, call Call) (*caCapture, *memTracks, time.Time) {
	t.Helper()
	tracks := &memTracks{}
	t0 := time.Unix(1000, 0)
	c, err := startCoreAudio(f, call, tracks, t0)
	if err != nil {
		t.Fatal(err)
	}
	close(c.quit)
	close(c.done)
	return c, tracks, t0
}

func TestADetectedCallTapsTheAppAndItsMicrophoneAfterAskingNotBefore(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	c, _, _ := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	if want := []string{"ask", "app[7 9]", "mic"}; !slices.Equal(f.calls, want) {
		t.Fatalf("Core Audio was asked %v, want %v: the prompt before any device, then the app's processes alone, then the microphone", f.calls, want)
	}
	if c.FarSide() != FarSideApp {
		t.Fatalf("far side %s, want the app's own", c.FarSide())
	}
}

func TestTheMicrophoneIsReadOnlyWhileTheAppReadsOne(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	c, _, t0 := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	f.set(browserCall(false)...)
	c.look(t0.Add(time.Second))
	if got := f.running(); !slices.Equal(got, []string{"app[7 9]"}) {
		t.Fatalf("running %v once the app let go of the microphone, want the app's tap alone", got)
	}
	f.set(browserCall(true)...)
	c.look(t0.Add(2 * time.Second))
	if got := f.running(); !slices.Equal(got, []string{"app[7 9]", "mic[]"}) {
		t.Fatalf("running %v once the app read the microphone again, want the microphone back", got)
	}
}

func TestTheCallEndsOnceTheAppLetGoOfTheMicrophoneForTheWaitEvenWhilePaused(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	c, _, t0 := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	_ = c.SetPaused(true)
	c.look(t0.Add(time.Second))
	if got := f.running(); len(got) != 0 {
		t.Fatalf("running %v while paused, want nothing", got)
	}
	f.set(browserCall(false)...)
	c.look(t0.Add(2 * time.Second))
	c.look(t0.Add(2*time.Second + callEndWait - time.Second))
	select {
	case <-c.Ended():
		t.Fatal("the call ended before the wait")
	default:
	}
	c.look(t0.Add(2*time.Second + callEndWait))
	select {
	case <-c.Ended():
	default:
		t.Fatal("a call that ended during a pause kept the recording open")
	}
}

func TestAPauseLetsGoOfEveryDeviceAndDropsWhatArrives(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	c, tracks, t0 := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	buf := make([]float32, 4096)
	f.deliver(true)
	c.drain(buf)
	before, _, _ := tracks.get("far")
	_ = c.SetPaused(true)
	f.deliver(true)
	c.drain(buf)
	c.look(t0.Add(time.Second))
	if got := f.running(); len(got) != 0 {
		t.Fatalf("running %v after a pause, want nothing", got)
	}
	if after, _, _ := tracks.get("far"); after != before {
		t.Fatalf("the far track took %d samples while paused", after-before)
	}
	_ = c.SetPaused(false)
	c.look(t0.Add(2 * time.Second))
	if got := f.running(); !slices.Equal(got, []string{"app[7 9]", "mic[]"}) {
		t.Fatalf("running %v after resuming, want the app's tap and the microphone", got)
	}
}

func TestAnAppTapThatCannotBeMadeFallsBackToTheWholeDeviceAndSaysSo(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true, appTapErr: errors.New("no tap")}
	f.set(browserCall(true)...)
	c, _, _ := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	if got := f.running(); !slices.Equal(got, []string{"global[1]", "mic[]"}) {
		t.Fatalf("running %v, want everything but this app (object 1), and the microphone", got)
	}
	if c.FarSide() != FarSideWholeDevice {
		t.Fatalf("far side %s after the fallback, want whole-device", c.FarSide())
	}
}

func TestAnAppThatWentLeavesNothingTappedAndNeverWidens(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	c, _, t0 := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	f.set(caProcess{Object: 11, PID: 300, Output: true, Path: musicApp})
	c.look(t0.Add(time.Second))
	if got := f.running(); len(got) != 0 {
		t.Fatalf("running %v once the app's processes went, want nothing", got)
	}
	if c.FarSide() != FarSideApp {
		t.Fatalf("far side %s once the app went, want still the app's own", c.FarSide())
	}
	f.set(browserCall(true)...)
	c.look(t0.Add(2 * time.Second))
	if got := f.running(); !slices.Equal(got, []string{"app[7 9]", "mic[]"}) {
		t.Fatalf("running %v once the app came back, want its tap again", got)
	}
}

func TestTheTapIsBuiltAgainForNewProcessesAnOutputChangeAndAStall(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	c, _, t0 := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	now := t0
	next := func() time.Time { now = now.Add(time.Second); f.deliver(false); return now }
	// A new helper of the app.
	f.set(append(browserCall(true), caProcess{Object: 13, PID: 130, Output: true, Path: chromeHelper})...)
	c.look(next())
	if got := f.running(); !slices.Equal(got, []string{"app[13 7 9]", "mic[]"}) && !slices.Equal(got, []string{"app[7 9 13]", "mic[]"}) {
		t.Fatalf("running %v after a new helper, want its tap too", got)
	}
	taps := func() int {
		n := 0
		for _, s := range f.sources {
			if s.kind == "app" {
				n++
			}
		}
		return n
	}
	before := taps()
	f.mu.Lock()
	f.out = 4
	f.mu.Unlock()
	c.look(next())
	if taps() != before+1 {
		t.Fatal("the tap was not built again on a new default output")
	}
	// A tap that delivers nothing for the stall is built again.
	before = taps()
	for range 4 {
		now = now.Add(time.Second)
		c.look(now)
	}
	if taps() <= before {
		t.Fatal("a tap that stalled was not built again")
	}
}

func TestByHandWithNoCallRecordsTheWholeDevicesAndNeverEndsByItself(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(caProcess{Object: 11, PID: 300, Output: true, Path: musicApp})
	c, _, t0 := startFake(t, f, Call{})
	if want := []string{"ask", "global[1]", "mic"}; !slices.Equal(f.calls, want) {
		t.Fatalf("Core Audio was asked %v, want %v", f.calls, want)
	}
	if c.FarSide() != FarSideWholeDevice {
		t.Fatalf("far side %s, want whole-device", c.FarSide())
	}
	c.look(t0.Add(2 * callEndWait))
	select {
	case <-c.Ended():
		t.Fatal("a recording by hand ended by itself")
	default:
	}
}

func TestSilenceIsWarnedAndTheWarningClearsOnceTheSideIsHeard(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: false}
	f.set(browserCall(true)...)
	c, _, t0 := startFake(t, f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"})
	c.look(t0.Add(time.Second))
	if w := c.Warning(); w == nil || w.Settings != SettingsMicrophone {
		t.Fatalf("warning %+v with the microphone not allowed, want the microphone's", w)
	}
	now := t0.Add(time.Second)
	for range int(silenceAfter/time.Second) + 1 {
		now = now.Add(time.Second)
		f.deliver(false)
		c.look(now)
	}
	if w := c.Warning(); w == nil || w.Settings != SettingsPrivacy {
		t.Fatalf("warning %+v after the meeting was silent while it played, want both sides'", w)
	}
	// The person allows the microphone: it is opened again, and both sides
	// are heard.
	f.mu.Lock()
	f.allowed = true
	f.mu.Unlock()
	now = now.Add(time.Second)
	c.look(now)
	f.deliver(true)
	now = now.Add(time.Second)
	c.look(now)
	if w := c.Warning(); w != nil {
		t.Fatalf("warning %+v once both sides were heard, want none", w)
	}
}

func TestTheDetectorReportsACallFromTheProcessList(t *testing.T) {
	f := &fakeCoreAudio{}
	f.set(browserCall(true)...)
	d := CoreAudioDetector{endWait: 20 * time.Millisecond, every: 5 * time.Millisecond, list: f.Processes}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan CallEvent, 4)
	go func() { _ = d.Watch(ctx, events) }()
	ev := <-events
	if ev.Ended || ev.Call.AppKey() != "google chrome" {
		t.Fatalf("first event %+v, want Google Chrome's call", ev)
	}
	f.set(browserCall(false)...)
	select {
	case ev = <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("no end")
	}
	if !ev.Ended {
		t.Fatalf("next event %+v, want the call's end", ev)
	}
	if f.calls != nil {
		t.Fatalf("the detector asked Core Audio %v; it only reads the process list", f.calls)
	}
}

func TestARunningCaptureFillsItsTracksAndLetsGoOfEveryDeviceAtStop(t *testing.T) {
	f := &fakeCoreAudio{out: 2, in: 3, allowed: true}
	f.set(browserCall(true)...)
	tracks := &memTracks{}
	c, err := recordCoreAudio(context.Background(), f, Call{App: "Google Chrome", Binary: "google chrome", PID: "120"}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	f.deliver(true)
	waitFor(t, "both tracks to fill", func() bool {
		far, _, _ := tracks.get("far")
		mic, _, _ := tracks.get("mic")
		return far >= SampleRate-1 && mic >= SampleRate-1
	})
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := f.running(); len(got) != 0 {
		t.Fatalf("running %v after Stop, want nothing", got)
	}
	for _, name := range []string{"far", "mic"} {
		if _, ended, _ := tracks.get(name); !ended {
			t.Fatalf("the %s track did not end at Stop", name)
		}
	}
}
