package meeting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRecorder records nothing from a device: each Record opens the tracks
// a test asks for and writes the samples it is given.
type fakeRecorder struct {
	mu      sync.Mutex
	calls   int
	asked   []Call
	call    Call
	far     FarSide
	tracks  []fakeTrackSpec
	capture *fakeCapture
	err     error
}

type fakeTrackSpec struct {
	label   Label
	name    string
	samples []int16
}

func (f *fakeRecorder) Record(_ context.Context, call Call, tracks Tracks) (Capture, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.asked = append(f.asked, call)
	if f.err != nil {
		return nil, f.err
	}
	got := f.call
	if call.PID != "" {
		got = call
	}
	c := &fakeCapture{call: got, far: f.far, ended: make(chan struct{})}
	for _, spec := range f.tracks {
		t, err := tracks.Open(spec.label, spec.name)
		if err != nil {
			return nil, err
		}
		t.Write(spec.samples)
		c.tracks = append(c.tracks, t)
	}
	f.capture = c
	return c, nil
}

func (f *fakeRecorder) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeCapture struct {
	mu      sync.Mutex
	call    Call
	far     FarSide
	widened bool
	warning *Warning
	tracks  []Track
	paused  []bool
	stopped bool
	ended   chan struct{}
}

func (c *fakeCapture) Call() Call { return c.call }
func (c *fakeCapture) FarSide() FarSide {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.widened {
		return FarSideWholeDevice
	}
	return c.far
}
func (c *fakeCapture) Ended() <-chan struct{} { return c.ended }
func (c *fakeCapture) Warning() *Warning {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.warning
}
func (c *fakeCapture) SetPaused(p bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = append(c.paused, p)
	return nil
}
func (c *fakeCapture) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	for _, t := range c.tracks {
		t.End()
	}
	return nil
}

// fakeEngine answers each track with the segments set for its file name.
// It lists devices as its graphics devices, and a run asked for one of
// them takes gpuTook[device] on it, or falls back to the CPU when the
// device has no time set.
type fakeEngine struct {
	mu      sync.Mutex
	byFile  map[string][]Segment
	seen    []string
	reqs    []TranscribeRequest
	err     error
	took    time.Duration
	devices []string
	gpuTook map[int]time.Duration
}

func (e *fakeEngine) Transcribe(_ context.Context, req TranscribeRequest) (Transcription, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = append(e.seen, filepath.Base(req.Audio))
	e.reqs = append(e.reqs, req)
	if e.err != nil {
		return Transcription{}, e.err
	}
	res := Transcription{Segments: e.byFile[filepath.Base(req.Audio)], Language: "en", Took: e.took, Devices: e.devices}
	if took, ok := e.gpuTook[req.Device]; req.GPU && ok {
		res.Took, res.Backend = took, fmt.Sprintf("Vulkan%d", req.Device)
	}
	return res, nil
}

func (e *fakeEngine) requests() []TranscribeRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]TranscribeRequest(nil), e.reqs...)
}

func (e *fakeEngine) files() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.seen...)
}

// testCatalogue has small stand-in models whose files a test puts in place.
func testCatalogue(t *testing.T, dir string) Catalogue {
	t.Helper()
	clip := string(wavHeader(SampleRate)) + string(make([]byte, 2*SampleRate))
	asset := func(file, body string) Asset {
		sum := sha256.Sum256([]byte(body))
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return Asset{Name: file, File: file, URL: "https://example.invalid/" + file, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
	}
	return Catalogue{
		VAD:         asset("vad.bin", "vad"),
		Calibration: asset("clip.wav", clip),
		Models: map[string]Asset{
			"small": asset("small.bin", "small model"),
			"base":  asset("base.bin", "base model"),
			"large": asset("large.bin", "large model"),
		},
		Profiles: []Profile{{ID: "english", Name: "English", Language: "en", Tiers: map[string][]string{TierCPU: {"small", "base"}, TierGPU: {"large"}}}},
	}
}

type harness struct {
	m         *Manager
	rec       *fakeRecorder
	eng       *fakeEngine
	settings  *Settings
	notices   []string
	raised    []Notice
	withdrawn []string
	shown     []*RecordingStatus
	notifyErr error
	out       string
	configure func(*Manager)
	mu        sync.Mutex
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, nil) }

// newHarnessWith is newHarness with configure run on the manager before it
// starts.
func newHarnessWith(t *testing.T, configure func(*Manager)) *harness {
	t.Helper()
	root := t.TempDir()
	models := filepath.Join(root, "models")
	if err := os.MkdirAll(models, 0o700); err != nil {
		t.Fatal(err)
	}
	h := &harness{
		rec:       &fakeRecorder{far: FarSideApp, call: Call{App: "Zoom", PID: "42"}},
		eng:       &fakeEngine{byFile: map[string][]Segment{}},
		settings:  &Settings{On: true, Profile: "english", OutputDir: filepath.Join(root, "out")},
		out:       filepath.Join(root, "out"),
		configure: configure,
	}
	h.m = &Manager{
		Root: root, Catalogue: testCatalogue(t, models), Recorder: h.rec, Engine: h.eng,
		Models: &Models{Dir: models}, Tier: TierCPU,
		Settings: func() Settings {
			h.mu.Lock()
			defer h.mu.Unlock()
			return *h.settings
		},
		Notify: func(n Notice) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.notices = append(h.notices, n.Title+": "+n.Body)
			h.raised = append(h.raised, n)
			return h.notifyErr
		},
		Withdraw: func(tag string) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.withdrawn = append(h.withdrawn, tag)
		},
		Indicate: func(st *RecordingStatus) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.shown = append(h.shown, st)
		},
	}
	if h.configure != nil {
		h.configure(h.m)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { h.m.Shutdown(); cancel() })
	h.m.Start(ctx)
	return h
}

func (h *harness) set(f func(*Settings)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f(h.settings)
}

func speech(n int) []int16 {
	s := make([]int16, n)
	for i := range s {
		if i%2 == 0 {
			s[i] = 8000
		} else {
			s[i] = -8000
		}
	}
	return s
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *harness) meeting(t *testing.T, id string) *Meeting {
	t.Helper()
	m, err := h.m.store().Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNothingRecordsWhileTheFeatureIsOff(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.On = false })
	if _, err := h.m.Record(context.Background()); !errors.Is(err, ErrOff) {
		t.Fatalf("Record with the feature off: %v, want ErrOff", err)
	}
	if n := h.rec.count(); n != 0 {
		t.Fatalf("the recorder was asked %d times with the feature off", n)
	}
	if err := h.m.Retry("anything"); !errors.Is(err, ErrOff) {
		t.Fatalf("Retry with the feature off: %v, want ErrOff", err)
	}
	if ms, _ := h.m.Meetings(); len(ms) != 0 {
		t.Fatalf("a meeting was made with the feature off: %+v", ms)
	}
}

func TestARecordingIsAnnouncedAndOneRunsAtATime(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(1600)}}
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mt.App != "Zoom" || mt.FarSide != FarSideApp {
		t.Fatalf("meeting = %+v", mt)
	}
	h.mu.Lock()
	notices := append([]string(nil), h.notices...)
	h.mu.Unlock()
	if len(notices) != 1 || !strings.Contains(notices[0], "Zoom") || !strings.Contains(notices[0], "consent") {
		t.Fatalf("notices = %q, want one naming Zoom with the consent sentence", notices)
	}
	if st := h.m.Status(); st.Recording == nil || st.Recording.ID != mt.ID {
		t.Fatalf("status = %+v, want the recording", st)
	}
	if _, err := h.m.Record(context.Background()); err == nil {
		t.Fatal("a second recording started while one ran")
	}
}

func TestStopTranscribesBothSidesAndDeletesTheAudio(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{
		{Me, "mic", speech(SampleRate)},
		{Them, "far-7", speech(SampleRate)},
		{Them, "far-8", make([]int16, SampleRate)}, // a silent stream
	}
	h.eng.byFile["mic.wav"] = []Segment{{Start: 2 * time.Second, End: 3 * time.Second, Text: "Morning, can everyone hear me?"}}
	h.eng.byFile["far-7.wav"] = []Segment{{Start: 4 * time.Second, End: 6 * time.Second, Text: "Yes, loud and clear."}}
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	got := h.meeting(t, mt.ID)
	if files := h.eng.files(); len(files) != 2 || strings.Contains(strings.Join(files, ","), "far-8") {
		t.Fatalf("transcribed %v, want mic and far-7 and never the silent far-8", files)
	}
	raw, err := os.ReadFile(got.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, want := range []string{"[00:00:02] Me: Morning, can everyone hear me?", "[00:00:04] Them: Yes, loud and clear.", "app: Zoom", "far_side: app", "model: small"} {
		if !strings.Contains(doc, want) {
			t.Errorf("transcript lacks %q:\n%s", want, doc)
		}
	}
	if filepath.Dir(got.Transcript) != h.out {
		t.Errorf("transcript at %s, want in %s", got.Transcript, h.out)
	}
	if h.m.HasAudio(got) || !got.AudioUntil.IsZero() {
		t.Fatalf("with retention off the audio outlived its transcription: %+v", got)
	}
}

func TestKeptAudioStaysForSevenDaysThenGoes(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.KeepAudio = true })
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	mt, _ := h.m.Record(context.Background())
	_ = h.m.Stop()
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	got := h.meeting(t, mt.ID)
	if !h.m.HasAudio(got) || time.Until(got.AudioUntil) < KeepFor-time.Minute {
		t.Fatalf("kept audio: has=%v until=%v", h.m.HasAudio(got), got.AudioUntil)
	}
	st := h.m.store()
	if err := st.Sweep(time.Now().Add(KeepFor-time.Hour), true); err != nil || !st.HasAudio(h.meeting(t, mt.ID)) {
		t.Fatalf("the sweep took kept audio early (err %v)", err)
	}
	if err := st.Sweep(time.Now().Add(KeepFor+time.Hour), true); err != nil || st.HasAudio(h.meeting(t, mt.ID)) {
		t.Fatalf("the sweep left audio past seven days (err %v)", err)
	}
}

func TestTurningRetentionOffDeletesKeptAudioOnTheNextSweep(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.KeepAudio = true })
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	mt, _ := h.m.Record(context.Background())
	_ = h.m.Stop()
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	if err := h.m.store().Sweep(time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if h.m.HasAudio(h.meeting(t, mt.ID)) {
		t.Fatal("audio kept after retention went off")
	}
}

func TestAFailedTranscriptionKeepsTheAudioForAnotherTry(t *testing.T) {
	h := newHarness(t)
	h.eng.err = errors.New("the sidecar crashed")
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	mt, _ := h.m.Record(context.Background())
	_ = h.m.Stop()
	waitFor(t, "the failure", func() bool { return h.meeting(t, mt.ID).State == StateFailed })
	got := h.meeting(t, mt.ID)
	if !h.m.HasAudio(got) || !strings.Contains(got.Error, "crashed") {
		t.Fatalf("failed meeting = %+v, audio %v", got, h.m.HasAudio(got))
	}
	h.eng.mu.Lock()
	h.eng.err = nil
	h.eng.mu.Unlock()
	if err := h.m.Retry(mt.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the retry", func() bool { return h.meeting(t, mt.ID).State == StateDone })
}

func TestPauseIsAGapInTheTranscript(t *testing.T) {
	h := newHarness(t)
	now := time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	h.m.Now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
	advance := func(d time.Duration) { clockMu.Lock(); now = now.Add(d); clockMu.Unlock() }
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	h.eng.byFile["mic.wav"] = []Segment{{Start: 0, End: time.Second, Text: "Hello."}}
	mt, _ := h.m.Record(context.Background())
	advance(10 * time.Second)
	if err := h.m.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	advance(20 * time.Second)
	if err := h.m.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	if got := h.rec.capture.paused; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("the capture was told %v, want pause then resume", got)
	}
	_ = h.m.Stop()
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	raw, _ := os.ReadFile(h.meeting(t, mt.ID).Transcript)
	if !strings.Contains(string(raw), "from: \"00:00:10\"") || !strings.Contains(string(raw), "reason: paused") {
		t.Fatalf("the pause is not in the transcript:\n%s", raw)
	}
}

func TestTheAppGoingEndsTheRecording(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	mt, _ := h.m.Record(context.Background())
	close(h.rec.capture.ended)
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	if h.m.Status().Recording != nil {
		t.Fatal("still recording after the capture ended")
	}
}

// A manager shut down for an update that did not start records again once
// started again.
func TestAManagerShutDownStartsAgain(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	h.m.Shutdown()
	h.m.Start(context.Background())
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatalf("recording after starting again: %v", err)
	}
	_ = h.m.Stop()
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
}

func TestShutdownKeepsTheRecordingForTheNextLaunch(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	h.eng.byFile["mic.wav"] = []Segment{{Start: 0, End: time.Second, Text: "Kept."}}
	mt, _ := h.m.Record(context.Background())
	h.m.Shutdown()
	if got := h.meeting(t, mt.ID); got.State != StatePending || !h.m.HasAudio(got) {
		t.Fatalf("after quitting: %+v", got)
	}
	next := &Manager{Root: h.m.Root, Catalogue: h.m.Catalogue, Recorder: h.rec, Engine: h.eng, Models: h.m.Models, Tier: TierCPU, Settings: h.m.Settings}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next.Start(ctx)
	defer next.Shutdown()
	waitFor(t, "the next launch's transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
}

func TestARecordingCutShortByACrashIsRepairedAndTranscribed(t *testing.T) {
	h := newHarness(t)
	st := h.m.store()
	started := time.Date(2026, 10, 24, 9, 30, 0, 0, time.UTC)
	mt := &Meeting{ID: st.NewID(started), Started: started, State: StateRecording, Title: "Meeting", Tracks: []TrackRecord{{File: "mic.wav", Label: Me}}}
	if err := st.Create(mt); err != nil {
		t.Fatal(err)
	}
	// A header never finished, as a crash leaves it.
	body := append(make([]byte, wavHeaderSize), int16Bytes(speech(SampleRate))...)
	if err := os.WriteFile(st.TrackPath(mt, mt.Tracks[0]), body, 0o600); err != nil {
		t.Fatal(err)
	}
	h.eng.byFile["mic.wav"] = []Segment{{Start: 0, End: time.Second, Text: "Recovered."}}
	next := &Manager{Root: h.m.Root, Catalogue: h.m.Catalogue, Recorder: h.rec, Engine: h.eng, Models: h.m.Models, Tier: TierCPU, Settings: h.m.Settings}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next.Start(ctx)
	defer next.Shutdown()
	waitFor(t, "the recovered transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
}

func int16Bytes(s []int16) []byte {
	b := make([]byte, 2*len(s))
	for i, v := range s {
		b[2*i] = byte(v)
		b[2*i+1] = byte(uint16(v) >> 8)
	}
	return b
}

func TestAFailedRecorderLeavesNoMeeting(t *testing.T) {
	h := newHarness(t)
	h.rec.err = errors.New("no audio system")
	if _, err := h.m.Record(context.Background()); err == nil {
		t.Fatal("Record succeeded with a failing recorder")
	}
	if ms, _ := h.m.Meetings(); len(ms) != 0 {
		t.Fatalf("a failed start left %d meetings", len(ms))
	}
	if h.m.Status().Recording != nil {
		t.Fatal("a failed start left a recording")
	}
}

func TestCalibrationFallsBackToTheFasterModel(t *testing.T) {
	h := newHarness(t)
	if got := h.m.ChosenModel(h.m.Settings()); got != "small" {
		t.Fatalf("before calibration: %s, want small", got)
	}
	h.eng.took = 90 * time.Second // 90 s for about a minute of speech: slower than real time
	h.m.Prepare(context.Background())
	waitFor(t, "the calibration", func() bool {
		st := h.m.Status()
		return !st.Prepare.Busy && h.m.readCalibration().RTF["small"] > 0
	})
	if st := h.m.Status(); st.Prepare.Error != "" {
		t.Fatal(st.Prepare.Error)
	}
	if got := h.m.ChosenModel(h.m.Settings()); got != "base" {
		t.Fatalf("after a slow calibration: %s, want base", got)
	}
	h.set(func(s *Settings) { s.Model = "small" })
	if got := h.m.ChosenModel(h.m.Settings()); got != "small" {
		t.Fatalf("the person's choice: %s, want small", got)
	}
	h.set(func(s *Settings) { s.Model = "not-in-the-catalogue" })
	if got := h.m.ChosenModel(h.m.Settings()); got != "base" {
		t.Fatalf("a model the profile does not offer: %s, want the calibrated base", got)
	}
}

func TestAFastCalibrationKeepsTheDefault(t *testing.T) {
	h := newHarness(t)
	h.eng.took = 3 * time.Second
	h.m.Prepare(context.Background())
	waitFor(t, "the calibration", func() bool { return !h.m.Status().Prepare.Busy && h.m.readCalibration().RTF["small"] > 0 })
	if got := h.m.ChosenModel(h.m.Settings()); got != "small" {
		t.Fatalf("after a fast calibration: %s, want small", got)
	}
	if _, measured := h.m.readCalibration().RTF["base"]; measured {
		t.Fatal("the fallback was measured though the default was fast enough")
	}
}

func TestACapturesWarningIsSaidOnceAndShownWithTheRecording(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Them, "far-1", speech(SampleRate)}}
	if _, err := h.m.Record(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := &Warning{Text: "macOS is giving Agentry only silence from your microphone.", Settings: SettingsMicrophone}
	h.rec.capture.mu.Lock()
	h.rec.capture.warning = w
	h.rec.capture.mu.Unlock()
	waitFor(t, "the warning's notification", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return len(h.notices) == 2 && strings.Contains(h.notices[1], "only silence from your microphone")
	})
	if got := h.m.Status().Recording; got == nil || got.Warning == nil || *got.Warning != *w {
		t.Fatalf("the recording's status = %+v, want the warning", got)
	}
	// The same warning is not said again.
	time.Sleep(2500 * time.Millisecond)
	h.mu.Lock()
	n := len(h.notices)
	h.mu.Unlock()
	if n != 2 {
		t.Fatalf("%d notices after the warning stood for two more seconds, want 2", n)
	}
	_ = h.m.Stop()
}

func TestAFarSideThatWidensIsSaidAndWritten(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Them, "far-1", speech(SampleRate)}}
	h.eng.byFile["far-1.wav"] = []Segment{{Start: 0, End: time.Second, Text: "Hello."}}
	mt, _ := h.m.Record(context.Background())
	h.rec.capture.mu.Lock()
	h.rec.capture.widened = true
	h.rec.capture.mu.Unlock()
	waitFor(t, "the widening", func() bool { return h.meeting(t, mt.ID).FarSide == FarSideWholeDevice })
	waitFor(t, "its notification", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return len(h.notices) == 3 && strings.Contains(h.notices[1], "everything")
	})
	// The recording's own notification is raised again with the wider
	// words, in its place.
	h.mu.Lock()
	again := h.raised[2]
	h.mu.Unlock()
	if again.Tag != tagRecording || !strings.Contains(again.Body, "everything your computer plays") {
		t.Fatalf("the recording's notification after the widening = %+v", again)
	}
	_ = h.m.Stop()
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	raw, _ := os.ReadFile(h.meeting(t, mt.ID).Transcript)
	if !strings.Contains(string(raw), "far_side: whole-device") {
		t.Fatalf("the widening is not in the transcript:\n%s", raw)
	}
}

// probed prepares h's manager and waits for its graphics devices'
// measurement.
func probed(t *testing.T, h *harness) {
	t.Helper()
	h.m.Prepare(context.Background())
	waitFor(t, "the graphics measurement", func() bool { return !h.m.Status().Prepare.Busy && h.m.readCalibration().GPU != nil })
	if st := h.m.Status(); st.Prepare.Error != "" {
		t.Fatal(st.Prepare.Error)
	}
}

func TestTheGraphicsCardIsOfferedOnceAndADeclineIsKept(t *testing.T) {
	h := newHarnessWith(t, func(m *Manager) { m.ProbeGPU = true })
	h.eng.took = 3 * time.Second // the CPU keeps up with small
	h.eng.devices = []string{"Integrated", "Discrete"}
	// About a minute of speech: the integrated card at half real time is
	// too slow for the better model, the discrete one at a tenth is not.
	h.eng.gpuTook = map[int]time.Duration{0: 30 * time.Second, 1: 6 * time.Second}
	probed(t, h)

	st := h.m.Status()
	if !st.GPUOffer || st.GPU == nil || st.GPU.Name != "Discrete" || st.OnGPU {
		t.Fatalf("after measuring: offer %v, gpu %+v, on %v; want the discrete card offered, not used", st.GPUOffer, st.GPU, st.OnGPU)
	}
	if got := h.m.ChosenModel(h.m.Settings()); got != "small" {
		t.Fatalf("before the offer is answered: %s, want small on the CPU", got)
	}
	// Nothing ran on a graphics card unasked but the measurement, and the
	// CPU's own measurement ran on the CPU.
	var warmed1 int
	for _, r := range h.eng.requests() {
		if r.GPU && r.Model == "" {
			t.Fatalf("a run on the graphics card without a model: %+v", r)
		}
		if r.GPU && r.Device == 1 {
			warmed1++
		}
		if !r.GPU && filepath.Base(r.Audio) != "calibration.wav" {
			t.Fatalf("a CPU run on %s", r.Audio)
		}
	}
	if warmed1 != 2 {
		t.Fatalf("the second card ran %d times, want a warm-up and a timed run", warmed1)
	}

	h.set(func(s *Settings) { s.Backend = BackendGPU })
	st = h.m.Status()
	if st.GPUOffer || !st.OnGPU {
		t.Fatalf("taken: offer %v, on %v", st.GPUOffer, st.OnGPU)
	}
	if got := h.m.ChosenModel(h.m.Settings()); got != "large" {
		t.Fatalf("taken: %s, want large", got)
	}
	if gpu, dev := h.m.gpuFor(h.m.Settings()); !gpu || dev != 1 {
		t.Fatalf("taken: gpu %v device %d, want the discrete card", gpu, dev)
	}

	h.set(func(s *Settings) { s.Backend = BackendCPU })
	h.m.Prepare(context.Background())
	waitFor(t, "the preparation", func() bool { return !h.m.Status().Prepare.Busy })
	if st := h.m.Status(); st.GPUOffer || st.OnGPU || st.GPU == nil {
		t.Fatalf("declined: offer %v, on %v, gpu %+v; want no offer, the card still known for Settings", st.GPUOffer, st.OnGPU, st.GPU)
	}
	if got := h.m.ChosenModel(h.m.Settings()); got != "small" {
		t.Fatalf("declined: %s, want small", got)
	}
}

func TestNoGraphicsOfferWithoutACardThatKeepsUp(t *testing.T) {
	for name, tc := range map[string]struct {
		probe   bool
		devices []string
		gpuTook map[int]time.Duration
		known   bool // a card is known to Settings
	}{
		"not offered on this platform":              {probe: false, devices: []string{"Card"}, gpuTook: map[int]time.Duration{0: time.Second}},
		"no graphics device":                        {probe: true},
		"every device fell back to the CPU":         {probe: true, devices: []string{"Card"}},
		"a card slower than the better model needs": {probe: true, devices: []string{"Card"}, gpuTook: map[int]time.Duration{0: 20 * time.Second}, known: true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarnessWith(t, func(m *Manager) { m.ProbeGPU = tc.probe })
			h.eng.took = 3 * time.Second
			h.eng.devices, h.eng.gpuTook = tc.devices, tc.gpuTook
			if tc.probe {
				probed(t, h)
			} else {
				h.m.Prepare(context.Background())
				waitFor(t, "the preparation", func() bool { return !h.m.Status().Prepare.Busy && h.m.readCalibration().RTF["small"] > 0 })
				for _, r := range h.eng.requests() {
					if r.GPU {
						t.Fatalf("a run on the graphics card where none is offered: %+v", r)
					}
				}
				// A measurement left from elsewhere offers nothing here.
				cal := h.m.readCalibration()
				cal.GPU = &GPUProbe{Model: "small", Devices: []GPUDevice{{Name: "Card", Backend: "Vulkan0", RTF: 0.01}}}
				if err := h.m.writeCalibration(cal); err != nil {
					t.Fatal(err)
				}
			}
			st := h.m.Status()
			if st.GPUOffer || st.OnGPU || (st.GPU != nil) != tc.known {
				t.Fatalf("offer %v, on %v, gpu %+v", st.GPUOffer, st.OnGPU, st.GPU)
			}
			// Choosing the card where none ran stays on the CPU.
			h.set(func(s *Settings) { s.Backend = BackendGPU })
			if gpu, _ := h.m.gpuFor(h.m.Settings()); gpu != tc.known {
				t.Fatalf("the graphics card chosen: gpu %v, want %v", gpu, tc.known)
			}
		})
	}
}

func TestAMeetingIsTranscribedOnTheCardTaken(t *testing.T) {
	h := newHarnessWith(t, func(m *Manager) { m.ProbeGPU = true })
	h.eng.took = 3 * time.Second
	h.eng.devices = []string{"Integrated", "Discrete"}
	h.eng.gpuTook = map[int]time.Duration{1: 6 * time.Second}
	probed(t, h)
	h.set(func(s *Settings) { s.Backend = BackendGPU })
	h.rec.tracks = []fakeTrackSpec{{Them, "far-1", speech(SampleRate)}}
	h.eng.byFile["far-1.wav"] = []Segment{{Start: 0, End: time.Second, Text: "Hello."}}
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = h.m.Stop()
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	got := h.meeting(t, mt.ID)
	if got.Model != "large" || got.Backend != "Vulkan1" {
		t.Fatalf("transcribed with %s on %q, want large on Vulkan1", got.Model, got.Backend)
	}
	var track *TranscribeRequest
	for _, r := range h.eng.requests() {
		if filepath.Base(r.Audio) == "far-1.wav" {
			track = &r
		}
	}
	if track == nil || !track.GPU || track.Device != 1 || filepath.Base(track.Model) != "large.bin" {
		t.Fatalf("the track's run = %+v", track)
	}
}

func TestACalibrationFromBeforeTheCPUWasPinnedIsTakenAgain(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(h.m.calibrationPath(), []byte(`{"rtf":{"small":5}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := h.m.ChosenModel(h.m.Settings()); got != "small" {
		t.Fatalf("an unversioned calibration was used: %s", got)
	}
	h.eng.took = 3 * time.Second
	h.m.Prepare(context.Background())
	waitFor(t, "the calibration", func() bool { return !h.m.Status().Prepare.Busy && h.m.readCalibration().RTF["small"] > 0 })
	if c := h.m.readCalibration(); c.RTF["small"] >= 1 {
		t.Fatalf("the old measurement stands: %+v", c)
	}
}
