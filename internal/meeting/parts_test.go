package meeting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTheCatalogueIsWhole(t *testing.T) {
	c, err := LoadCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"english", "multilingual"} {
		if c.Profile(id).ID != id {
			t.Errorf("no %s profile", id)
		}
	}
	if got := c.Choices("english", TierCPU); !slices.Equal(got, []string{"small.en-q5_1", "base.en-q5_1"}) {
		t.Errorf("English on the CPU: %v", got)
	}
	if got := c.Choices("multilingual", TierCPU); !slices.Equal(got, []string{"small-q5_1", "base-q5_1"}) {
		t.Errorf("Multilingual on the CPU: %v", got)
	}
	if c.Profile("klingon").ID != DefaultProfile {
		t.Error("an unknown profile is not the default")
	}
	if c.Allowed("english", "small-q5_1") {
		t.Error("English offers the multilingual small model")
	}
	if c.Profile("multilingual").Language != "auto" || c.Profile("english").Language != "en" {
		t.Error("the profiles' languages are wrong")
	}
	for id, a := range c.Models {
		if !strings.HasPrefix(a.URL, "https://huggingface.co/ggerganov/whisper.cpp/resolve/") || !strings.HasSuffix(a.URL, "/"+a.File) {
			t.Errorf("model %s comes from %s", id, a.URL)
		}
	}
}

func TestACatalogueNamingAMissingModelIsRefused(t *testing.T) {
	c, _ := LoadCatalogue()
	c.Profiles = append(c.Profiles, Profile{ID: "x", Tiers: map[string][]string{TierCPU: {"nope"}}})
	if err := c.check(); err == nil {
		t.Fatal("a profile naming a missing model passed")
	}
	c, _ = LoadCatalogue()
	a := c.Models["base-q5_1"]
	a.URL = "http://huggingface.co/x"
	c.Models["base-q5_1"] = a
	if err := c.check(); err == nil {
		t.Fatal("a model from plain http passed")
	}
}

func asset(body []byte, file string, url string) Asset {
	sum := sha256.Sum256(body)
	return Asset{File: file, URL: url, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}
}

func TestAPinnedDownloadIsKept(t *testing.T) {
	body := []byte("a model of the right bytes")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	m := &Models{Dir: t.TempDir(), Client: srv.Client()}
	a := asset(body, "m.bin", srv.URL+"/m.bin")
	var last int64
	p, err := m.Ensure(context.Background(), a, func(done, _ int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != string(body) || last != int64(len(body)) {
		t.Fatalf("kept %q, progress %d", got, last)
	}
}

func TestACorruptedDownloadIsRefusedAndDeleted(t *testing.T) {
	body := []byte("a model of the right bytes")
	// The right length, all zeros: what a broken download was once seen to
	// deliver.
	zeros := make([]byte, len(body))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(zeros) }))
	defer srv.Close()
	dir := t.TempDir()
	m := &Models{Dir: dir, Client: srv.Client()}
	a := asset(body, "m.bin", srv.URL+"/m.bin")
	if _, err := m.Ensure(context.Background(), a, nil); !errors.Is(err, ErrDamaged) {
		t.Fatalf("a zero-filled download: %v, want ErrDamaged", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the damaged download was left behind: %v", entries)
	}
}

func TestAnOverlongDownloadIsRefused(t *testing.T) {
	body := []byte("short")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("short and then a great deal more"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := &Models{Dir: dir, Client: srv.Client()}
	if _, err := m.Ensure(context.Background(), asset(body, "m.bin", srv.URL), nil); err == nil {
		t.Fatal("a download longer than pinned was kept")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestAKeptModelThatChangedIsDeletedBeforeUse(t *testing.T) {
	dir := t.TempDir()
	body := []byte("the right model")
	a := asset(body, "m.bin", "https://example.invalid/m.bin")
	wrong := []byte("the wrong model")
	if err := os.WriteFile(filepath.Join(dir, "m.bin"), wrong, 0o600); err != nil {
		t.Fatal(err)
	}
	m := &Models{Dir: dir}
	if _, err := m.Verify(a); !errors.Is(err, ErrDamaged) {
		t.Fatalf("Verify of a changed file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the changed model was not deleted")
	}
}

func TestARedirectToPlainHTTPIsRefused(t *testing.T) {
	c := DownloadClient()
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	if err := c.CheckRedirect(req, nil); err == nil {
		t.Fatal("a redirect to http was followed")
	}
}

func TestMergeLabelsDropsEchoesAndJoinsTurns(t *testing.T) {
	me := []Segment{
		{Start: 1 * time.Second, End: 2 * time.Second, Text: "Morning, everyone."},
		// The far side's words heard again through the speakers.
		{Start: 5 * time.Second, End: 7 * time.Second, Text: "the signed build went out last night"},
		{Start: 20 * time.Second, End: 21 * time.Second, Text: "I'll take the docs."},
	}
	them := []Segment{
		{Start: 4500 * time.Millisecond, End: 7 * time.Second, Text: "The signed build went out last night."},
		{Start: 8 * time.Second, End: 9 * time.Second, Text: "Any questions?"},
	}
	got := Merge(me, them)
	want := []Segment{
		{Label: Me, Start: 1 * time.Second, End: 2 * time.Second, Text: "Morning, everyone."},
		{Label: Them, Start: 4500 * time.Millisecond, End: 9 * time.Second, Text: "The signed build went out last night. Any questions?"},
		{Label: Me, Start: 20 * time.Second, End: 21 * time.Second, Text: "I'll take the docs."},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Merge:\n got %+v\nwant %+v", got, want)
	}
}

func TestRenderIsFrontmatterAndTimedLines(t *testing.T) {
	loc := time.FixedZone("EDT", -4*3600)
	info := TranscriptInfo{
		Title: "Meeting 2026-10-24 14:00", Started: time.Date(2026, 10, 24, 14, 0, 12, 0, loc), Ended: time.Date(2026, 10, 24, 14, 47, 31, 0, loc),
		App: "Zoom", FarSide: FarSideWholeDevice, Language: "en", Profile: "english", Model: "small.en-q5_1",
		Gaps: []Gap{{From: 31 * time.Minute, To: 32 * time.Minute, Reason: "paused"}},
	}
	doc, err := Render(info, []Segment{{Label: Me, Start: 3723 * time.Second, Text: "Line\none"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	for _, want := range []string{
		"---\ntype: meeting-transcript\nformat: agentry-meeting-transcript/1\n",
		"duration_seconds: 2839\n", "far_side: whole-device\n", "participants: unknown\n", "language_profile: english\n",
		"reason: paused", "# Meeting 2026-10-24 14:00\n", "[01:02:03] Me: Line one\n", "\"Them\" is everything the computer played",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered transcript lacks %q:\n%s", want, s)
		}
	}
}

func TestATitleCannotBreakTheFrontmatter(t *testing.T) {
	doc, err := Render(TranscriptInfo{Title: "x\n---\nevil: true", Started: time.Now(), Ended: time.Now()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := strings.SplitN(string(doc), "\n---\n", 2)[0]
	if strings.Contains(head, "\nevil: true") {
		t.Fatalf("the title escaped its scalar:\n%s", doc)
	}
}

func TestSlugAndName(t *testing.T) {
	for in, want := range map[string]string{"Google Chrome": "google-chrome", "ZOOM VoiceEngine": "zoom-voiceengine", "": "meeting", "--": "meeting", "Réunion d'équipe": "r-union-d-quipe"} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := TranscriptName(time.Date(2026, 10, 24, 14, 5, 0, 0, time.UTC), "Zoom"); got != "2026-10-24-1405-zoom.md" {
		t.Errorf("TranscriptName = %q", got)
	}
}

func TestATranscriptIsLinkedIntoPlaceNeverWrittenInPlace(t *testing.T) {
	dir := t.TempDir()
	content := []byte(strings.Repeat("[00:00:01] Me: words\n", 20000))
	// A walker reads every regular file it finds while transcripts are
	// written: every file it sees by its final name is whole.
	var stop atomic.Bool
	var torn atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".") {
					continue
				}
				b, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err == nil && len(b) != len(content) {
					torn.Store(fmt.Sprintf("%s had %d of %d bytes", e.Name(), len(b), len(content)))
				}
			}
		}
	}()
	var paths []string
	for range 5 {
		p, err := WriteTranscript(dir, "2026-10-24-1400-zoom.md", content)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Base(p))
	}
	stop.Store(true)
	wg.Wait()
	if v := torn.Load(); v != nil {
		t.Fatal(v)
	}
	want := []string{"2026-10-24-1400-zoom.md", "2026-10-24-1400-zoom-2.md", "2026-10-24-1400-zoom-3.md", "2026-10-24-1400-zoom-4.md", "2026-10-24-1400-zoom-5.md"}
	if !slices.Equal(paths, want) {
		t.Fatalf("names %v, want %v (an existing transcript is never written over)", paths, want)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("a temporary file was left: %s", e.Name())
		}
	}
}

func TestAnExistingTranscriptIsNeverWrittenOver(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "2026-10-24-1400-zoom.md")
	if err := os.WriteFile(mine, []byte("the person's edits"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteTranscript(dir, "2026-10-24-1400-zoom.md", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(mine); string(b) != "the person's edits" {
		t.Fatalf("written over: %q", b)
	}
}

func TestATrackKeepsItsPlaceOnTheTimeline(t *testing.T) {
	start := time.Unix(1000, 0)
	now := start
	p := filepath.Join(t.TempDir(), "t.wav")
	tr, err := newWAVTrack(p, start, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	// The first samples arrive two seconds in.
	now = start.Add(2 * time.Second)
	tr.Write(speech(SampleRate / 10))
	// Small jitter is not padded.
	now = now.Add(150 * time.Millisecond)
	tr.Write(speech(SampleRate / 10))
	// Then nothing for five seconds (a pause).
	now = now.Add(5 * time.Second)
	tr.Write(speech(SampleRate))
	tr.End()
	if err := tr.Err(); err != nil {
		t.Fatal(err)
	}
	got, err := readWAVSamples(p)
	if err != nil {
		t.Fatal(err)
	}
	wantLen := int(now.Sub(start).Seconds() * SampleRate)
	if d := len(got) - wantLen; d < -SampleRate/20 || d > SampleRate/20 {
		t.Fatalf("track is %d samples, want about %d", len(got), wantLen)
	}
	firstSound := slices.IndexFunc(got, func(v int16) bool { return v != 0 })
	if want := 2*SampleRate - SampleRate/10; firstSound < want-10 || firstSound > want+10 {
		t.Fatalf("the first sound is at sample %d, want about %d", firstSound, want)
	}
	if tr.Silent() {
		t.Fatal("a track with speech counted as silent")
	}
}

func TestSilenceIsToldApart(t *testing.T) {
	dir := t.TempDir()
	quiet, _ := newWAVTrack(filepath.Join(dir, "q.wav"), time.Now(), time.Now)
	noise := make([]int16, 5*SampleRate)
	for i := range noise {
		noise[i] = int16(i%5 - 2) // a few counts: a quiet room
	}
	quiet.Write(noise)
	quiet.End()
	if !quiet.Silent() {
		t.Fatal("room noise counted as sound")
	}
	if s, err := SilentWAV(filepath.Join(dir, "q.wav")); err != nil || !s {
		t.Fatalf("SilentWAV of room noise: %v %v", s, err)
	}
	loud, _ := newWAVTrack(filepath.Join(dir, "l.wav"), time.Now(), time.Now)
	loud.Write(speech(SampleRate))
	loud.End()
	if s, err := SilentWAV(filepath.Join(dir, "l.wav")); err != nil || s {
		t.Fatalf("SilentWAV of speech: %v %v", s, err)
	}
}

// The sidecar's words with voice detection on, as it reported them for
// speech at 2.0 s, 11.3 s and 28.8 s: the first word after each silence is
// timed early, the last before one lasts through it, and a non-speech mark
// sits among the words.
const whisperOut = `{
 "result": {"language": "en"},
 "transcription": [
  {"offsets": {"from": 2000, "to": 2000}, "text": ""},
  {"offsets": {"from": 2000, "to": 2200}, "text": " Good"},
  {"offsets": {"from": 2200, "to": 2700}, "text": " morning."},
  {"offsets": {"from": 2700, "to": 10100}, "text": " Review."},
  {"offsets": {"from": 10100, "to": 11500}, "text": " The"},
  {"offsets": {"from": 11500, "to": 12100}, "text": " marketing"},
  {"offsets": {"from": 12100, "to": 12400}, "text": " [BLANK_AUDIO]"},
  {"offsets": {"from": 12400, "to": 27000}, "text": " spend."},
  {"offsets": {"from": 27000, "to": 29100}, "text": " Café"},
  {"offsets": {"from": 29100, "to": 29500}, "text": " ouvert?"}
 ]
}`

func TestTheSidecarsWordsBecomeTimedSentences(t *testing.T) {
	got, err := parseWhisperJSON([]byte(whisperOut))
	if err != nil {
		t.Fatal(err)
	}
	want := []Segment{
		{Start: 2000 * time.Millisecond, End: 2700 * time.Millisecond, Text: "Good morning."},
		{Start: 2700 * time.Millisecond, End: 4200 * time.Millisecond, Text: "Review."},
		{Start: 11200 * time.Millisecond, End: 13900 * time.Millisecond, Text: "The marketing spend."},
		{Start: 28800 * time.Millisecond, End: 29500 * time.Millisecond, Text: "Café ouvert?"},
	}
	if !slices.Equal(got.Segments, want) || got.Language != "en" {
		t.Fatalf("segments:\n got %+v\nwant %+v", got.Segments, want)
	}
}

func TestNonSpeechMarksAreDropped(t *testing.T) {
	for in, want := range map[string]string{" [BLANK_AUDIO]": "", " (music)": "", "Hello [ Silence ] there": "Hello there", "Yes (I think) so.": "Yes (I think) so."} {
		if got := cleanText(in); got != want {
			t.Errorf("cleanText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTheSidecarRunsWithVoiceDetectionAndNoContextAndRetriesOnTheCPU(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "mic.wav")
	var runs [][]string
	s := Sidecar{Path: "/opt/whisper-cli", Threads: 2, Run: func(_ context.Context, path string, args []string, stderr io.Writer) error {
		runs = append(runs, args)
		if len(runs) == 1 {
			return errors.New("exit status 1") // a GPU backend that failed mid-run
		}
		_, _ = io.WriteString(stderr, "whisper_print_progress_callback: progress =  50%\nwhisper_print_timings:     load time =   500.00 ms\nwhisper_print_timings:    total time =  2500.00 ms\n")
		of := args[slices.Index(args, "-of")+1]
		return os.WriteFile(of+".json", []byte(whisperOut), 0o600)
	}}
	var progress []int
	res, err := s.Transcribe(context.Background(), TranscribeRequest{Audio: audio, Model: "m.bin", VADModel: "v.bin", Language: "en", Progress: func(p int) { progress = append(progress, p) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || slices.Contains(runs[0], "-ng") || !slices.Contains(runs[1], "-ng") {
		t.Fatalf("runs %v, want one on the GPU then one with -ng", runs)
	}
	args := strings.Join(runs[0], " ")
	for _, want := range []string{"--vad -vm v.bin", "-mc 0", "-ml 1 -sow", "-oj", "-l en", "-m m.bin", "-t 2"} {
		if !strings.Contains(args, want) {
			t.Errorf("arguments %q lack %q", args, want)
		}
	}
	if res.Took != 2*time.Second || !slices.Equal(progress, []int{50}) || len(res.Segments) != 4 {
		t.Fatalf("took %v, progress %v, %d segments", res.Took, progress, len(res.Segments))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the sidecar's output was left beside the audio: %v", entries)
	}
}

func TestFindCallIsTheAppThatReadsAMicAndPlays(t *testing.T) {
	s := paSnapshot{
		Outputs: []paStream{
			{Index: 1, Device: 10, PID: "99", App: "Agentry Desktop"}, // ourselves
			{Index: 2, Device: 11, PID: "50", App: "OBS", OnMonitor: true},
			{Index: 3, Device: 10, PID: "60", App: "Recorder"}, // records, plays nothing
			{Index: 4, Device: 10, PID: "70", App: "Google Chrome input"},
		},
		Inputs: []paStream{
			{Index: 20, Device: 1, PID: "99"},
			{Index: 21, Device: 1, PID: "50"},
			{Index: 22, Device: 1, PID: "70", App: "Google Chrome", Corked: true},
			{Index: 23, Device: 1, PID: "70", App: "Google Chrome"},
		},
		MonitorOf: map[uint32]uint32{1: 11},
	}
	call, ok := findCall(s, "99")
	if !ok || call.PID != "70" || call.App != "Google Chrome" {
		t.Fatalf("findCall = %+v %v", call, ok)
	}
	if mic, ok := appMic(s, "70"); !ok || mic.Device != 10 {
		t.Fatalf("appMic = %+v %v", mic, ok)
	}
	if got := appPlayback(s, "70"); len(got) != 1 || got[0].Index != 23 {
		t.Fatalf("appPlayback = %+v, want only the uncorked stream", got)
	}
	s.Outputs[3].Corked = true
	if _, ok := findCall(s, "99"); ok {
		t.Fatal("a corked microphone stream counted as a call")
	}
}

func TestInt16s(t *testing.T) {
	if got := int16s([]byte{0x01, 0x00, 0xff, 0xff, 0x00, 0x80}); !slices.Equal(got, []int16{1, -1, -32768}) {
		t.Fatalf("int16s = %v", got)
	}
}

// readWAVSamples reads a 16-bit mono WAV's samples.
func readWAVSamples(path string) ([]int16, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // a test's own file
	if err != nil {
		return nil, err
	}
	if len(raw) < wavHeaderSize || !strings.HasPrefix(string(raw), "RIFF") {
		return nil, errors.New("not a WAV file")
	}
	return int16s(raw[wavHeaderSize:]), nil
}

func TestTheSidecarIsLookedForBesideTheAppOnly(t *testing.T) {
	cases := []struct{ dir, goos, want string }{
		{"/usr/bin", "linux", "/usr/lib/agentry-desktop/whisper/whisper-cli"},
		{"/home/me/agentry", "linux", "/home/me/agentry/whisper/whisper-cli"},
		{`C:\Program Files\Agentry`, "windows", filepath.Join(`C:\Program Files\Agentry`, "bin", "whisper", "whisper-cli.exe")},
		{"/Applications/Agentry Desktop.app/Contents/MacOS", "darwin", "/Applications/Agentry Desktop.app/Contents/Resources/whisper/whisper-cli"},
		{"/tmp/loose", "darwin", ""},
	}
	for _, c := range cases {
		if got := sidecarBeside(c.dir, c.goos); got != c.want {
			t.Errorf("sidecarBeside(%q, %s) = %q, want %q", c.dir, c.goos, got, c.want)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "whisper-cli")
	if err := CheckSidecar(p); !errors.Is(err, ErrNoSidecar) {
		t.Fatalf("a missing sidecar: %v", err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckSidecar(p); !errors.Is(err, ErrNoSidecar) {
		t.Fatalf("a sidecar that cannot be run: %v", err)
	}
	if err := os.Chmod(p, 0o700); err != nil { //nolint:gosec // a test's own stand-in
		t.Fatal(err)
	}
	if err := CheckSidecar(p); err != nil {
		t.Fatal(err)
	}
}

func TestCalibrationSpeaksForAtLeastAMinute(t *testing.T) {
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.wav")
	body := append(wavHeader(11*SampleRate), int16Bytes(speech(11*SampleRate))...)
	if err := os.WriteFile(clip, body, 0o600); err != nil {
		t.Fatal(err)
	}
	out, secs, err := calibrationAudio(clip, filepath.Join(dir, "cal"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := readWAVSamples(out)
	if err != nil {
		t.Fatal(err)
	}
	if secs < calibrationSeconds || float64(len(got))/SampleRate != secs {
		t.Fatalf("calibration audio of %.1f s (%d samples), want at least %d s", secs, len(got), calibrationSeconds)
	}
	stereo := append([]byte(nil), body...)
	stereo[22] = 2
	if err := os.WriteFile(clip, stereo, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := calibrationAudio(clip, filepath.Join(dir, "cal")); err == nil {
		t.Fatal("a stereo clip was taken")
	}
}
