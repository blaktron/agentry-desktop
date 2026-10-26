package meeting

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The live tests record through this Mac's own Core Audio. They need
// AGENTRY_MEETING_LIVE set, clang (the Command Line Tools) to build the
// stand-in meeting app from testdata/standin_darwin.m, and
// AGENTRY_MEETING_STAND_IN_WAV naming the speech it plays (whisper.cpp's
// samples/jfk.wav, 11 s). The recording test also takes the sidecar
// (AGENTRY_MEETING_SIDECAR) and a folder holding the models
// (AGENTRY_MEETING_MODELS).
//
// macOS gives a process it has not allowed only silence from the
// microphone and from a tap, so where nobody has allowed the test binary
// both, the recording test checks that the silence was caught and said.

const (
	standInLead = 4 * time.Second
	standInGap  = 8 * time.Second
)

func liveOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	if s := PlatformSupport(); s != "" {
		t.Skip(s)
	}
}

// startStandIn builds the stand-in and starts it; it ends when the test
// does, or when stop is called.
func startStandIn(t *testing.T) (stop func()) {
	t.Helper()
	wav := os.Getenv("AGENTRY_MEETING_STAND_IN_WAV")
	if wav == "" {
		t.Skip("AGENTRY_MEETING_STAND_IN_WAV is not set")
	}
	bin := filepath.Join(t.TempDir(), "testmeet")
	build := exec.Command("clang", "-fobjc-arc", "-o", bin, filepath.Join("testdata", "standin_darwin.m"),
		"-framework", "Foundation", "-framework", "AVFoundation", "-framework", "AudioToolbox")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the stand-in: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, wav)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	stop = func() {
		if done {
			return
		}
		done = true
		_ = in.Close()
		_ = cmd.Wait()
	}
	t.Cleanup(stop)
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "ready") {
		t.Fatalf("the stand-in did not start: %q %v", line, err)
	}
	return stop
}

func TestLiveCoreAudioDetection(t *testing.T) {
	liveOnly(t)
	events := make(chan CallEvent, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- CoreAudioDetector{endWait: 3 * time.Second, list: listProcesses}.Watch(ctx, events) }()
	began := time.Now()
	stop := startStandIn(t)
	ev := nextEvent(t, events, errc, 10*time.Second)
	if ev.Ended || ev.Call.App != "testmeet" || ev.Call.AppKey() != "testmeet" {
		t.Fatalf("the first event %+v, want the stand-in's call", ev)
	}
	t.Logf("the call was seen %.2f s after the stand-in started: %+v", ev.At.Sub(began).Seconds(), ev.Call)
	gone := time.Now()
	stop()
	ev = nextEvent(t, events, errc, 15*time.Second)
	if !ev.Ended || ev.Call.AppKey() != "testmeet" {
		t.Fatalf("the next event %+v, want the call's end", ev)
	}
	t.Logf("the call ended %.2f s after the stand-in went (end wait 3 s)", ev.At.Sub(gone).Seconds())
}

func nextEvent(t *testing.T, events <-chan CallEvent, errc <-chan error, within time.Duration) CallEvent {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case err := <-errc:
		t.Fatalf("the detector stopped: %v", err)
	case <-time.After(within):
		t.Fatalf("no call event within %s", within)
	}
	return CallEvent{}
}

func TestLiveCoreAudioRecording(t *testing.T) {
	liveOnly(t)
	stop := startStandIn(t)
	time.Sleep(1500 * time.Millisecond)
	cat, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	m := &Manager{
		Root: t.TempDir(), Catalogue: cat, Recorder: NewRecorder(),
		Engine: Sidecar{Path: os.Getenv("AGENTRY_MEETING_SIDECAR")},
		Models: &Models{Dir: os.Getenv("AGENTRY_MEETING_MODELS")}, Tier: HardwareTier(),
		Settings: func() Settings {
			return Settings{On: true, Profile: "english", Model: "base.en-q5_1", OutputDir: out}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Shutdown()
	began := time.Now()
	mt, err := m.Record(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mt.App != "testmeet" || mt.FarSide != FarSideApp {
		t.Fatalf("recording %+v, want testmeet's own sound", mt)
	}
	// Both playings of the 11 s sample, counted from the stand-in's start
	// 1.5 s before began, and long enough for the silence check.
	time.Sleep(time.Until(began.Add(max(standInLead+2*11*time.Second+standInGap+2*time.Second, silenceAfter+3*time.Second))))
	warning := m.Status().Recording.Warning
	t.Logf("the recording's warning: %+v", warning)
	// The stand-in going lets go of the microphone at once; the recording
	// itself ends callEndWait later, or at Stop.
	stop()
	time.Sleep(2 * time.Second)
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitForLong(t, func() bool {
		got, _ := m.store().Get(mt.ID)
		return got != nil && (got.State == StateDone || got.State == StateFailed)
	})
	got, _ := m.store().Get(mt.ID)
	if got.State != StateDone {
		t.Fatalf("transcription %s: %s", got.State, got.Error)
	}
	raw, err := os.ReadFile(got.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	if !strings.Contains(doc, "far_side: app") {
		t.Errorf("the transcript does not say the far side is the app's own")
	}
	var them []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, "] Them: ") {
			them = append(them, line)
		}
	}
	t.Logf("tracks %+v; Them:\n%s", got.Tracks, strings.Join(them, "\n"))
	if m.HasAudio(got) {
		t.Error("audio outlived its transcription")
	}
	if len(them) == 0 {
		// Nothing was heard of the meeting: macOS withheld it, and the
		// person was told so, with where to allow it.
		if warning == nil || (warning.Settings != SettingsAudio && warning.Settings != SettingsPrivacy) {
			t.Fatalf("the far side was silent and the warning is %+v, want one naming Screen & System Audio Recording", warning)
		}
		t.Log("macOS gave the tap only silence (the test binary is not allowed to record system audio); the warning said so")
		return
	}
	var at []int
	for _, l := range them {
		if strings.Contains(strings.ToLower(l), "country") {
			at = append(at, clockSeconds(t, l))
		}
	}
	if len(at) != 2 {
		t.Fatalf("the sample was heard %d times on the far side, want 2", len(at))
	}
	if gap := at[1] - at[0]; gap < 11+int(standInGap/time.Second)-3 || gap > 11+int(standInGap/time.Second)+3 {
		t.Errorf("the two playings are %d s apart, want about %d", gap, 11+int(standInGap/time.Second))
	}
}

var clockLine = regexp.MustCompile(`^\[(\d\d):(\d\d):(\d\d)\]`)

func clockSeconds(t *testing.T, line string) int {
	t.Helper()
	m := clockLine.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no clock in %q", line)
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	return h*3600 + mi*60 + s
}

// TestLiveTranscriberOnMetal runs the sidecar this Mac carries on the
// accelerated model its tier defaults to, on the sample itself.
func TestLiveTranscriberOnMetal(t *testing.T) {
	liveOnly(t)
	wav, sidecar, models := os.Getenv("AGENTRY_MEETING_STAND_IN_WAV"), os.Getenv("AGENTRY_MEETING_SIDECAR"), os.Getenv("AGENTRY_MEETING_MODELS")
	if wav == "" || sidecar == "" || models == "" {
		t.Skip("AGENTRY_MEETING_STAND_IN_WAV, AGENTRY_MEETING_SIDECAR and AGENTRY_MEETING_MODELS are needed")
	}
	cat, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	tier := HardwareTier()
	id := cat.Choices("english", tier)[0]
	model := filepath.Join(models, cat.Models[id].File)
	if _, err := os.Stat(model); err != nil {
		t.Skipf("%s is not downloaded to %s", id, models)
	}
	tr, err := Sidecar{Path: sidecar}.Transcribe(context.Background(), TranscribeRequest{
		Audio: wav, Model: model, VADModel: filepath.Join(models, cat.VAD.File), Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, s := range tr.Segments {
		text = append(text, s.Text)
	}
	all := strings.Join(text, " ")
	t.Logf("tier %s, model %s, %s: %s", tier, id, tr.Took, all)
	if !strings.Contains(strings.ToLower(all), "ask not what your country can do for you") {
		t.Fatalf("the sample was transcribed as %q", all)
	}
}
