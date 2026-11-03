package meeting

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// callHarness records a call whose microphone says one line and whose far
// side, in the tracks given, says three, with the switch for calls on.
func callHarness(t *testing.T, d Diarizer, far ...string) *harness {
	t.Helper()
	h := newHarnessWith(t, func(m *Manager) {
		if d != nil {
			m.Diarizer = d
		}
	})
	h.set(func(s *Settings) { s.CallSpeakers = true })
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	for _, name := range far {
		h.rec.tracks = append(h.rec.tracks, fakeTrackSpec{Them, name, speech(SampleRate)})
	}
	h.eng.byFile["mic.wav"] = []Segment{{Start: sec(8), End: sec(9), Text: "I agree."}}
	lines := [][]Segment{
		{{Start: sec(1), End: sec(3), Text: "Shall we start?"}, {Start: sec(12), End: sec(14), Text: "Good."}},
		{{Start: sec(4), End: sec(6), Text: "Yes, let's."}},
	}
	for i, name := range far {
		if len(far) == 1 {
			h.eng.byFile[name+".wav"] = append(lines[0], lines[1]...)
			break
		}
		h.eng.byFile[name+".wav"] = lines[i]
	}
	return h
}

func (h *harness) recordCall(t *testing.T) (*Meeting, string) {
	t.Helper()
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool {
		s := h.meeting(t, mt.ID).State
		return s == StateDone || s == StateFailed
	})
	got := h.meeting(t, mt.ID)
	if got.State != StateDone {
		t.Fatalf("transcription %s: %s", got.State, got.Error)
	}
	raw, err := os.ReadFile(got.Transcript)
	if err != nil {
		t.Fatal(err)
	}
	return got, string(raw)
}

func TestACallsFarSideIsToldApartAndMeStaysMe(t *testing.T) {
	d := twoVoices()
	h := callHarness(t, d, "them")
	mt, doc := h.recordCall(t)
	for _, want := range []string{
		"diarization: done", "speaker_count: estimated", "label: Me", "label: Speaker 1", "label: Speaker 2", "source: system-audio",
		"] Speaker 1: Shall we start?", "] Speaker 2: Yes, let's.", "] Me: I agree.", "] Speaker 1: Good.",
		`"Me" is the person who owns this computer`, "How many people were on the call was estimated",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("transcript lacks %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "] Them:") || strings.Contains(doc, "label: Them") {
		t.Errorf("a Them line or label is left:\n%s", doc)
	}
	if mt.Diarization != DiarizationDone || mt.Speakers != 2 {
		t.Errorf("meeting says %q, %d speakers", mt.Diarization, mt.Speakers)
	}
	reqs := d.requests()
	if len(reqs) != 1 || reqs[0].Speakers != 0 || filepath.Base(reqs[0].Audio) != "them.wav" {
		t.Fatalf("asked %+v", reqs)
	}
}

func TestACallsFarSideStreamsAreHeardTogether(t *testing.T) {
	d := twoVoices()
	h := callHarness(t, d, "zoom", "zoom-2")
	mt, doc := h.recordCall(t)
	for _, want := range []string{"] Speaker 1: Shall we start?", "] Speaker 2: Yes, let's.", "] Speaker 1: Good."} {
		if !strings.Contains(doc, want) {
			t.Errorf("transcript lacks %q:\n%s", want, doc)
		}
	}
	reqs := d.requests()
	if len(reqs) != 1 || filepath.Base(reqs[0].Audio) != ".far-side-mix.wav" {
		t.Fatalf("asked %+v", reqs)
	}
	if _, err := os.Stat(reqs[0].Audio); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the mix was left behind (%v)", err)
	}
	if len(mt.Tracks) != 3 {
		t.Errorf("tracks %+v", mt.Tracks)
	}
}

func TestACallsSpeakerSeparationThatCannotHelpLeavesThemAndSaysWhy(t *testing.T) {
	for _, c := range []struct {
		name string
		d    Diarizer
		want string
	}{
		{"failed", &fakeDiarizer{err: errors.New("exit status 255")}, "Telling the far side's speakers apart failed (the speaker separation stopped with an error)"},
		{"not carried", nil, "The far side's speakers could not be told apart (this copy of Agentry does not carry the speaker separation)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := callHarness(t, c.d, "them")
			_, doc := h.recordCall(t)
			for _, want := range []string{c.want, "] Them: Shall we start?", "label: Them", "] Me: I agree."} {
				if !strings.Contains(doc, want) {
					t.Errorf("transcript lacks %q:\n%s", want, doc)
				}
			}
			if strings.Contains(doc, "Speaker 1") || strings.Contains(doc, "speaker_count") {
				t.Errorf("speakers claimed:\n%s", doc)
			}
		})
	}
}

func TestACallWithASilentFarSideIsNotToldApart(t *testing.T) {
	d := twoVoices()
	h := callHarness(t, d)
	_, doc := h.recordCall(t)
	if len(d.requests()) != 0 || strings.Contains(doc, "diarization") {
		t.Fatalf("a call with no far side was told apart:\n%s", doc)
	}
}

func TestTheFarSidesEchoOnTheMicrophoneIsDroppedWhenItsVoicesAreToldApart(t *testing.T) {
	them := []Segment{{Start: sec(1), End: sec(3), Text: "Shall we start the review now?"}}
	me := []Segment{
		{Start: sec(1.5), End: sec(3), Text: "Shall we start the review now?"},
		{Start: sec(10), End: sec(11), Text: "Yes."},
	}
	speakers := []Segment{{Label: SpeakerLabel(1), Start: sec(1), End: sec(3), Text: "Shall we start the review now?"}}
	got := MergeSpeakers(me, them, speakers)
	if len(got) != 2 || got[0].Label != SpeakerLabel(1) || got[1].Label != Me || got[1].Text != "Yes." {
		t.Fatalf("merged %+v", got)
	}
}

func TestMixWAVSumsTracksAndClips(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, s []int16) string {
		p := filepath.Join(dir, name)
		w, err := newWAVTrack(p, time.Now(), time.Now)
		if err != nil {
			t.Fatal(err)
		}
		w.writeSamples(s)
		w.End()
		return p
	}
	long := make([]int16, SampleRate/4)
	for i := range long {
		long[i] = 20000
	}
	short := []int16{100, -30000, 20000}
	out := filepath.Join(dir, "mix.wav")
	if err := MixWAV(out, []string{write("a.wav", long), write("b.wav", short)}); err != nil {
		t.Fatal(err)
	}
	got, err := readPCM16(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(long) {
		t.Fatalf("%d samples, want %d", len(got), len(long))
	}
	if got[0] != 20100 || got[1] != -10000 || got[2] != 32767 || got[3] != 20000 || got[len(got)-1] != 20000 {
		t.Errorf("mixed %v … %v", got[:4], got[len(got)-1])
	}
}

// The transcripts of a call, as they are written: the far side as Them,
// told apart, and told apart on the whole device's sound.
func TestCallTranscriptsMatchTheirGoldenFiles(t *testing.T) {
	started := time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)
	base := TranscriptInfo{
		Title: "Meeting 2026-11-03 15:00", Started: started, Ended: started.Add(30 * time.Minute),
		App: "Zoom", FarSide: FarSideApp, Language: "en", Profile: "english", Model: "small.en-q5_1",
	}
	me := []Segment{{Start: sec(8), End: sec(9), Text: "I agree."}}
	them := []Segment{
		{Start: sec(1), End: sec(3), Text: "Shall we start?"},
		{Start: sec(4), End: sec(6), Text: "Yes, let's."},
		{Start: sec(12), End: sec(14), Text: "Good."},
	}
	speakers := []Segment{
		{Label: SpeakerLabel(1), Start: sec(1), End: sec(3), Text: "Shall we start?"},
		{Label: SpeakerLabel(2), Start: sec(4), End: sec(6), Text: "Yes, let's."},
		{Label: SpeakerLabel(1), Start: sec(12), End: sec(14), Text: "Good."},
	}
	told := base
	told.Diarization, told.Speakers = DiarizationDone, 2
	whole := told
	whole.App, whole.FarSide = "", FarSideWholeDevice
	failed := base
	failed.Diarization, failed.DiarizationError = DiarizationFailed, "the speaker separation stopped with an error"
	for name, c := range map[string]struct {
		info  TranscriptInfo
		lines []Segment
	}{
		"call-them":                  {base, Merge(me, them)},
		"call-speakers":              {told, MergeSpeakers(me, them, speakers)},
		"call-speakers-whole-device": {whole, MergeSpeakers(me, them, speakers)},
		"call-speakers-failed":       {failed, Merge(me, them)},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Render(c.info, c.lines)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "transcripts", name+".md")
			if *updateGolden {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to write it)", err)
			}
			if string(got) != string(want) {
				t.Errorf("%s differs from its golden file:\n%s", name, got)
			}
		})
	}
}
