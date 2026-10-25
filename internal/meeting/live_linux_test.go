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
//	                         side) and intruder.wav (another app's sound)
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
