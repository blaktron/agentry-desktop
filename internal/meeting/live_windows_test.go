package meeting

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/blaktron/agentry-desktop/internal/wasapi"
)

// standInEnv, when set to a WAV file, makes this test binary a stand-in
// meeting app: it reads the default microphone, waits standInLead, plays
// the file, waits standInGap, plays it again, and goes on reading the
// microphone until it is ended or its standard input closes, which happens
// when the test that started it ends however it ends. It plays at a tenth
// of full volume, through the default output.
const standInEnv = "AGENTRY_MEETING_STAND_IN"

const (
	standInLead = 4 * time.Second
	standInGap  = 8 * time.Second
)

// TestMain also runs this binary as the audio child, which the recorder
// starts as this executable with CaptureFlag, and as the stand-in.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == CaptureFlag {
		os.Exit(CaptureMain(os.Stdin, os.Stdout))
	}
	if wav := os.Getenv(standInEnv); wav != "" {
		os.Exit(standIn(wav))
	}
	os.Exit(m.Run())
}

func standIn(wav string) int {
	winmm := windows.NewLazySystemDLL("winmm.dll")
	play := winmm.NewProc("PlaySoundW")
	volume := winmm.NewProc("waveOutSetVolume")
	s, err := wasapi.Open()
	if err != nil {
		return 1
	}
	defer s.Close()
	ep, err := s.Default(wasapi.Capture)
	if err != nil {
		return 1
	}
	mic, err := s.Record(ep, wasapi.Capture)
	if err != nil {
		return 1
	}
	defer mic.Close()
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}()
	go func() {
		const sndFilename, sndNoDefault = 0x00020000, 0x0002
		p, _ := windows.UTF16PtrFromString(wav)
		_, _, _ = volume.Call(0, 0x19991999)
		time.Sleep(standInLead)
		_, _, _ = play.Call(uintptr(unsafe.Pointer(p)), 0, sndFilename|sndNoDefault)
		time.Sleep(standInGap)
		_, _, _ = play.Call(uintptr(unsafe.Pointer(p)), 0, sndFilename|sndNoDefault)
	}()
	for {
		if _, err := mic.Read(); err != nil {
			return 1
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// startStandIn starts a copy of this binary named testmeet.exe as the
// stand-in: a copy, since this app's own executable is never a call.
func startStandIn(t *testing.T) *exec.Cmd {
	t.Helper()
	wav := filepath.Join(os.Getenv("AGENTRY_MEETING_AUDIO"), "jfk.wav")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "testmeet.exe")
	in, err := os.Open(self) //nolint:gosec // this test binary
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(exe) //nolint:gosec // a file in the test's folder
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, in)
	_ = in.Close()
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe) //nolint:gosec // the copy made above
	cmd.Env = append(os.Environ(), standInEnv+"="+wav)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// Held open while the test runs: its end, a crash included, ends the
	// stand-in.
	if _, err := cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

// micInUse lists the executables the microphone's consent store says are
// using it now, which is what Windows' microphone icon shows.
func micInUse() []string {
	k, err := registry.OpenKey(registry.CURRENT_USER, consentKeyNonPackaged, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer func() { _ = k.Close() }()
	names, _ := k.ReadSubKeyNames(-1)
	var out []string
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		start, _, _ := sk.GetIntegerValue("LastUsedTimeStart")
		stop, _, _ := sk.GetIntegerValue("LastUsedTimeStop")
		_ = sk.Close()
		if start != 0 && stop == 0 {
			out = append(out, filepath.Base(strings.ReplaceAll(n, "#", `\`)))
		}
	}
	return out
}

const consentKeyNonPackaged = `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone\NonPackaged`

// TestLiveWASAPIRecording records a stand-in meeting app on this computer
// through the audio child and transcribes it with the real sidecar. It runs
// only when AGENTRY_MEETING_LIVE is set, with AGENTRY_MEETING_SIDECAR (the
// whisper-cli.exe), AGENTRY_MEETING_MODELS (ggml-base.en-q5_1.bin and
// ggml-silero-v6.2.0.bin) and AGENTRY_MEETING_AUDIO (whisper.cpp's
// samples/jfk.wav). The sample is played quietly through the default
// output, so it is heard in the room.
//
// It asks that the call is found and named, that the far side is the
// app's device, that both sides land on their tracks, that the second
// playing keeps its place after the silence between them, and that no audio
// outlives the transcription.
func TestLiveWASAPIRecording(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	startStandIn(t)
	time.Sleep(1500 * time.Millisecond)
	cat, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	m := &Manager{
		Root: t.TempDir(), Catalogue: cat, Recorder: NewRecorder(),
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
	began := time.Now()
	mt, err := m.Record(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mt.App != "testmeet" || mt.FarSide != FarSideAppDevice {
		t.Fatalf("recording %+v, want testmeet on its device", mt)
	}
	time.Sleep(5 * time.Second)
	t.Logf("the microphone in use, as Windows' consent store has it: %v", micInUse())
	// Both playings of the 11 s sample, counted from the stand-in's start
	// 1.5 s before began, and a little more.
	time.Sleep(time.Until(began.Add(standInLead + 2*11*time.Second + standInGap + 2*time.Second)))
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
	var them []string
	var me int
	for _, line := range strings.Split(doc, "\n") {
		switch {
		case strings.Contains(line, "] Them: "):
			them = append(them, line)
		case strings.Contains(line, "] Me: "):
			me++
		}
	}
	t.Logf("tracks %+v; %d Me lines (not shown: the room's microphone); Them:\n%s", got.Tracks, me, strings.Join(them, "\n"))
	if !strings.Contains(doc, "far_side: app-device") {
		t.Errorf("the transcript does not say the far side is the app's device")
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
	// The second playing begins standInGap after the first ends: the
	// silence between them kept its length.
	if gap := at[1] - at[0]; gap < 11+int(standInGap/time.Second)-3 || gap > 11+int(standInGap/time.Second)+3 {
		t.Errorf("the two playings are %d s apart, want about %d", gap, 11+int(standInGap/time.Second))
	}
	if m.HasAudio(got) {
		t.Error("audio outlived its transcription")
	}
}

var clockLine = regexp.MustCompile(`^\[(\d\d):(\d\d):(\d\d)\]`)

func clockSeconds(t *testing.T, line string) int {
	t.Helper()
	m := clockLine.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no time in %q", line)
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	return h*3600 + mi*60 + s
}

// TestLiveWASAPIDetection watches this computer's audio sessions for calls
// through the audio child. It runs only when AGENTRY_MEETING_LIVE is set,
// with AGENTRY_MEETING_AUDIO. The stand-in is a call from its first session
// until 30 s after it ends.
func TestLiveWASAPIDetection(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan CallEvent, 8)
	errc := make(chan error, 1)
	go func() { errc <- NewDetector().Watch(ctx, events) }()
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
	time.Sleep(2 * time.Second)
	app := startStandIn(t)
	began := time.Now()
	ev := expect("the call's start", 15*time.Second)
	if ev.Ended || ev.Call.Binary != "testmeet.exe" {
		t.Fatalf("the call's start: %+v", ev)
	}
	t.Logf("the call was seen %s after the stand-in started (it plays %s after it opens the microphone)", time.Since(began).Round(10*time.Millisecond), standInLead)
	_ = app.Process.Kill()
	gone := time.Now()
	ev = expect("the call's end", callEndWait+15*time.Second)
	if !ev.Ended || ev.Call.Binary != "testmeet.exe" {
		t.Fatalf("the call's end: %+v", ev)
	}
	if d := time.Since(gone); d < callEndWait {
		t.Fatalf("the call ended %s after the app went, before the wait", d)
	}
	t.Logf("the call ended %s after the app went", time.Since(gone).Round(100*time.Millisecond))
}
