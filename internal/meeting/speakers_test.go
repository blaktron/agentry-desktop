package meeting

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden transcripts")

// fakeDiarizer answers spans, or an error, and keeps what it was asked.
type fakeDiarizer struct {
	mu    sync.Mutex
	spans []SpeakerSpan
	err   error
	block bool
	asked []DiarizeRequest
}

func (d *fakeDiarizer) Diarize(ctx context.Context, req DiarizeRequest) ([]SpeakerSpan, error) {
	d.mu.Lock()
	d.asked = append(d.asked, req)
	spans, err, block := d.spans, d.err, d.block
	d.mu.Unlock()
	if req.Progress != nil {
		req.Progress(50)
	}
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return spans, err
}

func (d *fakeDiarizer) requests() []DiarizeRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DiarizeRequest(nil), d.asked...)
}

// roomHarness records a meeting in the room whose one track is said as
// three lines, and tells its two voices apart with d.
func roomHarness(t *testing.T, d Diarizer, configure func(*Manager)) *harness {
	t.Helper()
	h := newHarnessWith(t, func(m *Manager) {
		if d != nil {
			m.Diarizer = d
		}
		if configure != nil {
			configure(m)
		}
	})
	h.rec.micName = "USB Speakerphone"
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	h.eng.byFile["room.wav"] = []Segment{
		{Start: sec(1), End: sec(3), Text: "Shall we start?"},
		{Start: sec(4), End: sec(6), Text: "Yes, let's."},
		{Start: sec(12), End: sec(14), Text: "Good."},
	}
	return h
}

func twoVoices() *fakeDiarizer {
	return &fakeDiarizer{spans: []SpeakerSpan{{sec(0.5), sec(3.2), 4}, {sec(3.8), sec(6.5), 1}, {sec(11), sec(15), 4}}}
}

func (h *harness) recordRoom(t *testing.T, people int) string {
	t.Helper()
	mt, err := h.m.RecordRoom(context.Background(), people)
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
	return string(raw)
}

func TestARoomsSpeakersAreToldApartAndNumberedByWhoSpokeFirst(t *testing.T) {
	d := twoVoices()
	h := roomHarness(t, d, nil)
	doc := h.recordRoom(t, 0)
	for _, want := range []string{
		"diarization: done", "speaker_count: estimated", "label: Speaker 1", "label: Speaker 2",
		"] Speaker 1: Shall we start?", "] Speaker 2: Yes, let's.", "] Speaker 1: Good.",
		"was estimated, so one person may appear under several numbers",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("transcript lacks %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "] Room:") || strings.Contains(doc, "label: Room") {
		t.Errorf("a Room line or label is left:\n%s", doc)
	}
	reqs := d.requests()
	if len(reqs) != 1 || reqs[0].Speakers != 0 || filepath.Base(reqs[0].Audio) != "room.wav" ||
		filepath.Base(reqs[0].Segmentation) != "segmentation.onnx" || filepath.Base(reqs[0].Embedding) != "embedding.onnx" {
		t.Fatalf("asked %+v", reqs)
	}
}

func TestACountGivenAtRecordInTheRoomFixesTheVoices(t *testing.T) {
	d := twoVoices()
	h := roomHarness(t, d, nil)
	doc := h.recordRoom(t, 3)
	if reqs := d.requests(); len(reqs) != 1 || reqs[0].Speakers != 3 {
		t.Fatalf("asked %+v, want 3 speakers", reqs)
	}
	if !strings.Contains(doc, "speaker_count: given") || !strings.Contains(doc, "said 3 people were in the room") {
		t.Errorf("transcript:\n%s", doc)
	}
	for _, n := range []int{1, 21, -1} {
		if _, err := h.m.RecordRoom(context.Background(), n); err == nil {
			t.Errorf("a count of %d was taken", n)
		}
	}
}

func TestASpeakerSeparationThatCannotHelpLeavesTheLinesRoomAndSaysWhy(t *testing.T) {
	for _, c := range []struct {
		name      string
		d         Diarizer
		configure func(*Manager)
		off       bool
		want      []string
	}{
		{"failed", &fakeDiarizer{err: errors.New("exit status 255")}, nil, false,
			[]string{"diarization: failed", "Telling the speakers apart failed (the speaker separation stopped with an error)"}},
		{"heard nothing", &fakeDiarizer{spans: nil}, nil, false,
			[]string{"diarization: failed", "no word could be put to a voice"}},
		{"timed out", &fakeDiarizer{block: true}, func(m *Manager) { m.DiarizeTimeout = 50 * time.Millisecond }, false,
			[]string{"diarization: failed", "and was stopped"}},
		{"not carried", nil, nil, false,
			[]string{"diarization: unavailable", "does not carry the speaker separation"}},
		{"turned off", twoVoices(), nil, true,
			[]string{`diarization: "off"`, "Telling speakers apart is turned off in the settings"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := roomHarness(t, c.d, c.configure)
			if c.off {
				h.set(func(s *Settings) { s.SpeakersOff = true })
			}
			doc := h.recordRoom(t, 0)
			for _, want := range append(c.want, "] Room: Shall we start?", "label: Room") {
				if !strings.Contains(doc, want) {
					t.Errorf("transcript lacks %q:\n%s", want, doc)
				}
			}
			if strings.Contains(doc, "Speaker 1") || strings.Contains(doc, "speaker_count") {
				t.Errorf("speakers claimed:\n%s", doc)
			}
			if fd, ok := c.d.(*fakeDiarizer); ok && c.off && len(fd.requests()) != 0 {
				t.Error("the separation ran while turned off")
			}
		})
	}
}

func TestACorruptedSpeakerModelIsRefusedAndTheLinesStayRoom(t *testing.T) {
	d := twoVoices()
	h := roomHarness(t, d, nil)
	if err := os.WriteFile(h.m.Models.Path(h.m.Catalogue.Diarization.Embedding), []byte("not the embedding model"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := h.recordRoom(t, 0)
	if !strings.Contains(doc, "diarization: unavailable") || !strings.Contains(doc, "its models could not be downloaded or checked") || strings.Contains(doc, "example.invalid") {
		t.Errorf("transcript:\n%s", doc)
	}
	if len(d.requests()) != 0 {
		t.Error("the separation ran on a model that is not the pinned one")
	}
}

func TestACallIsNeverToldApart(t *testing.T) {
	d := twoVoices()
	h := newHarnessWith(t, func(m *Manager) { m.Diarizer = d })
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	h.eng.byFile["mic.wav"] = []Segment{{Start: sec(1), End: sec(2), Text: "Hello."}}
	mt, err := h.m.Record(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	raw, _ := os.ReadFile(h.meeting(t, mt.ID).Transcript)
	if len(d.requests()) != 0 || strings.Contains(string(raw), "diarization") {
		t.Fatalf("a call was told apart:\n%s", raw)
	}
}

// The transcripts of a meeting in the room, as they are written: speakers
// estimated, given, and the separation failed.
func TestRoomTranscriptsMatchTheirGoldenFiles(t *testing.T) {
	started := time.Date(2026, 10, 27, 14, 0, 0, 0, time.UTC)
	base := TranscriptInfo{
		Title: "Meeting in the room 2026-10-27 14:00", Started: started, Ended: started.Add(20 * time.Minute),
		FarSide: FarSideNone, Language: "en", Profile: "english", Model: "small.en-q5_1", InRoom: true, Microphone: "USB Speakerphone",
	}
	spoken := []Segment{
		{Label: SpeakerLabel(1), Start: sec(1), End: sec(3), Text: "Shall we start?"},
		{Label: SpeakerLabel(2), Start: sec(4), End: sec(6), Text: "Yes, let's."},
		{Label: SpeakerLabel(1), Start: sec(12), End: sec(14), Text: "Good."},
	}
	room := MergeRoom([]Segment{
		{Start: sec(1), End: sec(3), Text: "Shall we start?"},
		{Start: sec(4), End: sec(6), Text: "Yes, let's."},
		{Start: sec(12), End: sec(14), Text: "Good."},
	})
	estimated := base
	estimated.Diarization, estimated.Speakers = DiarizationDone, 2
	given := estimated
	given.People = 2
	failed := base
	failed.Diarization, failed.DiarizationError = DiarizationFailed, "the speaker separation stopped with an error"
	for name, c := range map[string]struct {
		info  TranscriptInfo
		lines []Segment
	}{
		"room-speakers-estimated": {estimated, spoken},
		"room-speakers-given":     {given, spoken},
		"room-speakers-failed":    {failed, room},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Render(c.info, c.lines)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "transcripts", name+".md")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
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

func TestQuittingWhileTheSpeakersAreToldApartLeavesTheMeetingForTheNextLaunch(t *testing.T) {
	d := &fakeDiarizer{block: true}
	h := roomHarness(t, d, nil)
	mt, err := h.m.RecordRoom(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the separation", func() bool { return len(d.requests()) == 1 })
	h.m.Shutdown()
	got := h.meeting(t, mt.ID)
	if got.State != StatePending || got.Transcript != "" {
		t.Fatalf("after quitting: state %s, transcript %q", got.State, got.Transcript)
	}
}
