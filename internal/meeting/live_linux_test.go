package meeting

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLivePulseRecording records test streams on this computer's audio
// system and transcribes them with the real sidecar. It runs only when
// AGENTRY_MEETING_LIVE is set, with:
//
//	AGENTRY_MEETING_SIDECAR  the whisper-cli executable
//	AGENTRY_MEETING_MODELS   a folder holding ggml-base.en-q5_1.bin and
//	                         ggml-silero-v6.2.0.bin, as the catalogue pins them
//	AGENTRY_MEETING_AUDIO    a folder holding mic-long.wav (fed to the
//	                         microphone), far-pad.wav (the meeting app's far
//	                         side), intruder.wav (another app's sound) and,
//	                         for TestLivePulseDetection, far-long.wav (any
//	                         sound of at least 42 s)
//
// and three devices made beforehand so nothing plays aloud: a null sink
// agentry_test_out (the speakers), a null sink agentry_test_micfeed and a remap source agentry_test_mic
// of its monitor (the microphone).
//
// A pair of pw-record and pw-play processes that share an
// application.process.id stand in for a meeting app. The test asks that
// the microphone and the far side land on their own tracks with the right
// labels, that the far track ends when its stream goes and is never moved
// onto another app's sound, and that the microphone stops being recorded
// when the app stops reading it.
func TestLivePulseRecording(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	audio := os.Getenv("AGENTRY_MEETING_AUDIO")
	start := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command(name, args...) //nolint:gosec // the test's own tools
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	app := `{ application.process.id = "424242" application.name = "TestMeet" }`
	start("pw-play", "--target", "agentry_test_micfeed", filepath.Join(audio, "mic-long.wav"))
	appMicStream := start("pw-record", "--target", "agentry_test_mic", "-P", app, filepath.Join(t.TempDir(), "app-mic.wav"))
	appFar := start("pw-play", "--target", "agentry_test_out", "-P", app, filepath.Join(audio, "far-pad.wav"))
	time.Sleep(700 * time.Millisecond)

	cat, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	m := &Manager{
		Root: t.TempDir(), Catalogue: cat, Recorder: PulseRecorder{},
		Engine: Sidecar{Path: os.Getenv("AGENTRY_MEETING_SIDECAR")},
		Models: &Models{Dir: os.Getenv("AGENTRY_MEETING_MODELS")}, Tier: TierCPU,
		Settings: func() Settings {
			return Settings{On: true, Profile: "english", Model: "base.en-q5_1", OutputDir: out}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Shutdown()
	mt, err := m.Record(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mt.App != "TestMeet" || mt.FarSide != FarSideApp {
		t.Fatalf("recording %+v, want TestMeet's own streams", mt)
	}
	// The far side's stream ends; another app then plays on the same sink.
	_ = appFar.Wait()
	time.Sleep(1500 * time.Millisecond)
	start("pw-play", "--target", "agentry_test_out", filepath.Join(audio, "intruder.wav"))
	time.Sleep(5 * time.Second)
	// The app stops reading the microphone, which goes on being fed.
	_ = appMicStream.Process.Kill()
	time.Sleep(3 * time.Second)
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
	t.Logf("tracks %+v\n%s", got.Tracks, doc)
	var me, them []string
	for _, line := range strings.Split(doc, "\n") {
		switch {
		case strings.Contains(line, "] Me: "):
			me = append(me, strings.ToLower(line))
		case strings.Contains(line, "] Them: "):
			them = append(them, strings.ToLower(line))
		}
	}
	joinedMe, joinedThem := strings.Join(me, " "), strings.Join(them, " ")
	if !strings.Contains(joinedMe, "microphone") || strings.Contains(joinedMe, "far side") {
		t.Errorf("Me lines: %q", me)
	}
	if !strings.Contains(joinedThem, "far side") || strings.Contains(joinedThem, "microphone") {
		t.Errorf("Them lines: %q", them)
	}
	if strings.Contains(joinedThem, "pineapple") || strings.Contains(joinedMe, "pineapple") {
		t.Errorf("another app's sound was recorded: %q", them)
	}
	// The microphone was fed three times over about 17 s; the app stopped
	// reading it after about 12 s, so the third time is not all there.
	if n := strings.Count(joinedMe, "quarterly budget"); n > 2 {
		t.Errorf("the microphone was recorded after the app stopped reading it (%d copies)", n)
	}
	if m.HasAudio(got) {
		t.Error("audio outlived its transcription")
	}
}

// TestLivePulseDetection watches this computer's audio system for calls. It
// runs only when AGENTRY_MEETING_LIVE is set, with AGENTRY_MEETING_AUDIO and
// the devices TestLivePulseRecording names. A volume meter's stream beside
// a sound of the same process is not a call; a stand-in meeting app (a
// pw-record and a pw-play sharing an application.process.id) is, from its
// first stream to 30 s after it lets go of the microphone, and coming back
// within those 30 s keeps it one call.
func TestLivePulseDetection(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	audio := os.Getenv("AGENTRY_MEETING_AUDIO")
	start := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command(name, args...) //nolint:gosec // the test's own tools
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan CallEvent, 8)
	errc := make(chan error, 1)
	go func() { errc <- PulseDetector{}.Watch(ctx, events) }()
	expect := func(what string, within time.Duration) CallEvent {
		t.Helper()
		select {
		case ev := <-events:
			return ev
		case err := <-errc:
			t.Fatalf("waiting for %s, the detector stopped: %v", what, err)
		case <-time.After(within):
			t.Fatalf("no %s within %s", what, within)
		}
		return CallEvent{}
	}
	quiet := func(what string, d time.Duration) {
		t.Helper()
		select {
		case ev := <-events:
			t.Fatalf("%s: %+v", what, ev)
		case <-time.After(d):
		}
	}

	meter := `{ application.process.id = "515151" application.name = "TestMeter" stream.monitor = true }`
	m1 := start("pw-record", "--target", "agentry_test_mic", "-P", meter, filepath.Join(t.TempDir(), "meter.wav"))
	m2 := start("pw-play", "--target", "agentry_test_out", "-P", meter, filepath.Join(audio, "far-long.wav"))
	quiet("a volume meter was taken for a call", 3*time.Second)
	_ = m1.Process.Kill()
	_ = m2.Process.Kill()

	app := `{ application.process.id = "424243" application.name = "TestMeet" }`
	mic := start("pw-record", "--target", "agentry_test_mic", "-P", app, filepath.Join(t.TempDir(), "app-mic.wav"))
	start("pw-play", "--target", "agentry_test_out", "-P", app, filepath.Join(audio, "far-long.wav"))
	began := time.Now()
	ev := expect("the call's start", 5*time.Second)
	if ev.Ended || ev.Call.App != "TestMeet" || ev.Call.PID != "424243" {
		t.Fatalf("the call's start: %+v", ev)
	}
	t.Logf("the call was seen %s after its streams started", time.Since(began).Round(10*time.Millisecond))

	// Dropped and rejoined within the wait: one call.
	_ = mic.Process.Kill()
	time.Sleep(5 * time.Second)
	mic = start("pw-record", "--target", "agentry_test_mic", "-P", app, filepath.Join(t.TempDir(), "app-mic-2.wav"))
	quiet("a rejoin within 30 s was a new call", 3*time.Second)

	_ = mic.Process.Kill()
	gone := time.Now()
	quiet("the call ended before its wait", callEndWait-3*time.Second)
	ev = expect("the call's end", 10*time.Second)
	if !ev.Ended || ev.Call.App != "TestMeet" {
		t.Fatalf("the call's end: %+v", ev)
	}
	t.Logf("the call ended %s after the app let go of the microphone", time.Since(gone).Round(100*time.Millisecond))
}

// TestLivePulseRoom records a meeting in the room on this computer's audio
// system and transcribes it with the real sidecar. It runs only when
// AGENTRY_MEETING_LIVE is set, with the variables and devices
// TestLivePulseRecording names. The microphone chosen is the remap source
// agentry_test_mic; another app plays on the speakers meanwhile. The test
// asks that the chosen microphone alone is recorded, as one Room track, and
// that nothing played on the speakers is in it.
func TestLivePulseRoom(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	audio := os.Getenv("AGENTRY_MEETING_AUDIO")
	mics, err := PulseRecorder{}.Microphones()
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, mic := range mics {
		if mic.ID == "agentry_test_mic" {
			listed = true
		}
		if strings.HasSuffix(mic.ID, ".monitor") {
			t.Errorf("an output's monitor is listed as a microphone: %+v", mic)
		}
	}
	if !listed {
		t.Fatalf("microphones %+v lack agentry_test_mic", mics)
	}
	start := func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command(name, args...) //nolint:gosec // the test's own tools
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	cat, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	m := &Manager{
		Root: t.TempDir(), Catalogue: cat, Recorder: PulseRecorder{},
		Engine: Sidecar{Path: os.Getenv("AGENTRY_MEETING_SIDECAR")},
		Models: &Models{Dir: os.Getenv("AGENTRY_MEETING_MODELS")}, Tier: TierCPU,
		Settings: func() Settings {
			return Settings{On: true, Profile: "english", Model: "base.en-q5_1", OutputDir: out, Mic: "agentry_test_mic"}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Shutdown()
	mt, err := m.RecordRoom(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mt.FarSide != FarSideNone || mt.MicMissing {
		t.Fatalf("recording %+v, want the room on the chosen microphone", mt)
	}
	start("pw-play", "--target", "agentry_test_micfeed", filepath.Join(audio, "mic-long.wav"))
	start("pw-play", "--target", "agentry_test_out", filepath.Join(audio, "intruder.wav"))
	time.Sleep(12 * time.Second)
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
	if len(got.Tracks) != 1 || got.Tracks[0].Label != Room {
		t.Fatalf("tracks %+v, want one Room track", got.Tracks)
	}
	raw, err := os.ReadFile(got.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ToLower(string(raw))
	t.Logf("%s", raw)
	if !strings.Contains(doc, "] room: ") || !strings.Contains(doc, "microphone") {
		t.Errorf("the chosen microphone's words are not on Room lines")
	}
	if strings.Contains(doc, "] me: ") || strings.Contains(doc, "] them: ") || strings.Contains(doc, "pineapple") {
		t.Errorf("a room recorded more than its microphone")
	}
}

// TestLivePulseMicGone records a meeting in the room on a microphone made
// for the test (a remap source of a null sink's monitor), removes it
// mid-recording and puts it back. It runs only when AGENTRY_MEETING_LIVE is
// set and pactl is installed, and needs a default source that is a
// microphone. It asks that the loss gives way to the default at once, that
// the microphone is read again once it is back, and that both are kept as
// changes.
func TestLivePulseMicGone(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	pactl := func(args ...string) string {
		out, err := exec.Command("pactl", args...).Output() //nolint:gosec // the test's own tool
		if err != nil {
			t.Fatalf("pactl %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	feed := pactl("load-module", "module-null-sink", "sink_name=agentry_test_gonefeed")
	t.Cleanup(func() { _ = exec.Command("pactl", "unload-module", feed).Run() })
	load := func() string {
		return pactl("load-module", "module-remap-source", "master=agentry_test_gonefeed.monitor", "source_name=agentry_test_gone",
			"source_properties=device.description=Agentry-test-gone")
	}
	mod := load()
	t.Cleanup(func() { _ = exec.Command("pactl", "unload-module", mod).Run() })
	time.Sleep(time.Second)
	def := pactl("get-default-source")
	if def == "agentry_test_gone" || strings.HasSuffix(def, ".monitor") {
		t.Skipf("the default source %q is not another microphone", def)
	}
	tracks := &memTracks{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, err := PulseRecorder{}.Record(ctx, Call{Room: true, Mic: "agentry_test_gone"}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	if name, missing := c.(MicReporter).Microphone(); missing || name == "" {
		t.Fatalf("recording %q (missing %v), want the test microphone", name, missing)
	}
	time.Sleep(2 * time.Second)
	pactl("unload-module", mod)
	changes := func() []MicChange { return c.(MicFollower).MicChanges() }
	deadline := time.Now().Add(5 * time.Second)
	for len(changes()) == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	got := changes()
	if len(got) != 1 || got[0].Lost == "" || got[0].Now == "" {
		t.Fatalf("after the removal: changes %+v, want the loss and the default read in its place", got)
	}
	t.Logf("removed: %+v", got[0])
	// The default is recorded meanwhile, on the same track.
	before, _, _ := tracks.get("room")
	time.Sleep(2 * time.Second)
	if after, _, _ := tracks.get("room"); after-before < SampleRate {
		t.Fatalf("the default gave %d samples in 2 s", after-before)
	}
	mod = load()
	deadline = time.Now().Add(5 * time.Second)
	for len(changes()) < 2 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	got = changes()
	if len(got) != 2 || got[1].Now != got[0].Lost {
		t.Fatalf("after it came back: changes %+v, want the test microphone read again", got)
	}
	t.Logf("back: %+v", got[1])
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if samples, ended, _ := tracks.get("room"); samples == 0 || !ended {
		t.Fatalf("the room track has %d samples, ended %v", samples, ended)
	}
}
