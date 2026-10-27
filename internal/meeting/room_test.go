package meeting

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// plainRecorder is a recorder that cannot record a meeting in the room.
type plainRecorder struct{ r *fakeRecorder }

func (p plainRecorder) Record(ctx context.Context, call Call, tracks Tracks) (Capture, error) {
	return p.r.Record(ctx, call, tracks)
}

func (h *harness) noticesNow() []Notice {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Notice(nil), h.raised...)
}

func (h *harness) withdrawnNow() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.withdrawn...)
}

func countTitled(ns []Notice, title string) int {
	n := 0
	for _, x := range ns {
		if x.Title == title {
			n++
		}
	}
	return n
}

func TestARoomIsRecordedThroughTheChosenMicrophoneAlone(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.Mic = "usb-speakerphone" })
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	h.rec.micName = "USB Speakerphone"
	mt, err := h.m.RecordRoom(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := h.rec.lastCall(); !got.Room || got.Mic != "usb-speakerphone" || got.PID != "" {
		t.Fatalf("the recorder was asked for %+v, want the room on the chosen microphone and no app", got)
	}
	if mt.FarSide != FarSideNone || mt.App != "" || mt.Microphone != "USB Speakerphone" {
		t.Fatalf("meeting = %+v, want no far side, no app, the microphone named", mt)
	}
	if st := h.m.Status(); st.Recording == nil || st.Recording.How != InRoom {
		t.Fatalf("status = %+v, want a recording in the room", st.Recording)
	}
	ns := h.noticesNow()
	if len(ns) != 1 || !strings.Contains(ns[0].Body, "your microphone in this room") || !strings.Contains(ns[0].Body, "consent") || strings.Contains(ns[0].Body, "plays") {
		t.Fatalf("notices = %+v, want one saying the microphone in this room alone, with the consent sentence", ns)
	}
	h.eng.byFile["room.wav"] = []Segment{
		{Start: 2 * time.Second, End: 3 * time.Second, Text: "Shall we start?"},
		{Start: 3 * time.Second, End: 4 * time.Second, Text: "Yes, go ahead."},
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	raw, err := os.ReadFile(h.meeting(t, mt.ID).Transcript)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, want := range []string{"[00:00:02] Room: Shall we start? Yes, go ahead.", "setting: in-room", "far_side: none", "label: Room", "microphone: USB Speakerphone", "everyone in the room"} {
		if !strings.Contains(doc, want) {
			t.Errorf("transcript lacks %q:\n%s", want, doc)
		}
	}
	for _, unwanted := range []string{"Them", "Me:", "silent:"} {
		if strings.Contains(doc, unwanted) {
			t.Errorf("transcript of a room says %q:\n%s", unwanted, doc)
		}
	}
	if !strings.Contains(h.meeting(t, mt.ID).Transcript, "meeting-in-the-room") {
		t.Errorf("transcript named %s, want it to say it was a meeting in the room", h.meeting(t, mt.ID).Transcript)
	}
}

func TestOnlyRecordInTheRoomRecordsARoom(t *testing.T) {
	h := newHarness(t)
	for _, how := range []How{ByHand, FromOffer, Automatically, InWindow} {
		if _, err := h.m.startRecording(context.Background(), Call{Room: true}, how, nil); err == nil {
			t.Fatalf("a room was recorded by %q", how)
		}
	}
	if _, err := h.m.startRecording(context.Background(), Call{}, InRoom, nil); err == nil {
		t.Fatal("Record in the room recorded something other than the room")
	}
	if n := h.rec.count(); n != 0 {
		t.Fatalf("the recorder was asked %d times", n)
	}
	if _, err := h.m.Record(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.rec.lastCall().Room {
		t.Fatal("Record asked the recorder for a room")
	}
}

func TestARoomNeedsARecorderThatRecordsRooms(t *testing.T) {
	h := newHarnessWith(t, func(m *Manager) { m.Recorder = plainRecorder{&fakeRecorder{}} })
	if h.m.RecordsRooms() {
		t.Fatal("a recorder without rooms is said to record them")
	}
	if _, err := h.m.RecordRoom(context.Background()); !errors.Is(err, ErrNoRooms) {
		t.Fatalf("RecordRoom = %v, want ErrNoRooms", err)
	}
	if mics, err := h.m.Microphones(); err != nil || mics != nil {
		t.Fatalf("Microphones = %v, %v; want none", mics, err)
	}
}

func TestARoomIsNotRecordedWhileTheFeatureIsOff(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.On = false })
	if _, err := h.m.RecordRoom(context.Background()); !errors.Is(err, ErrOff) {
		t.Fatalf("RecordRoom with the feature off: %v, want ErrOff", err)
	}
	if n := h.rec.count(); n != 0 {
		t.Fatalf("the recorder was asked %d times with the feature off", n)
	}
}

func TestAMissingChosenMicrophoneIsSaid(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.Mic = "gone" })
	h.rec.micName, h.rec.micMissing = "Built-in Audio", true
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	h.eng.byFile["room.wav"] = []Segment{{Start: time.Second, End: 2 * time.Second, Text: "Hello."}}
	mt, err := h.m.RecordRoom(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ns := h.noticesNow(); len(ns) != 1 || !strings.Contains(ns[0].Body, "was not found, so the default microphone is recorded") {
		t.Fatalf("notices = %+v, want the missing microphone said", ns)
	}
	if st := h.m.Status(); st.Recording == nil || !st.Recording.MicMissing {
		t.Fatalf("status = %+v, want the missing microphone shown", st.Recording)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	raw, _ := os.ReadFile(h.meeting(t, mt.ID).Transcript)
	for _, want := range []string{"microphone: Built-in Audio (default; the one chosen was missing)", "The microphone chosen in the settings was not found"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("transcript lacks %q:\n%s", want, raw)
		}
	}
}

func TestAQuietRoomIsAskedOnceAndGoesOnRecording(t *testing.T) {
	h := newHarnessWith(t, func(m *Manager) { m.QuietAfter = 1500 * time.Millisecond })
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	if _, err := h.m.RecordRoom(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the question", func() bool { return countTitled(h.noticesNow(), "Still recording?") == 1 })
	ask := h.noticesNow()[len(h.noticesNow())-1]
	if len(ask.Actions) != 2 || ask.Actions[0].ID != ActionStop || ask.Actions[1].ID != ActionKeepGoing {
		t.Fatalf("question = %+v, want Stop and Keep going", ask)
	}
	time.Sleep(2500 * time.Millisecond)
	if n := countTitled(h.noticesNow(), "Still recording?"); n != 1 {
		t.Fatalf("asked %d times in one quiet stretch, want once", n)
	}
	if st := h.m.Status(); st.Recording == nil {
		t.Fatal("the question stopped the recording")
	}
	if err := h.m.HandleAction(ActionKeepGoing); err != nil {
		t.Fatal(err)
	}
	// Someone speaks: the stretch ends, and the next one is asked about.
	h.rec.capture.tracks[0].Write(speech(SampleRate))
	waitFor(t, "the question taken away", func() bool {
		n := 0
		for _, tag := range h.withdrawnNow() {
			if tag == tagQuiet {
				n++
			}
		}
		return n >= 2
	})
	waitFor(t, "the next question", func() bool { return countTitled(h.noticesNow(), "Still recording?") == 2 })
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestAPausedRoomIsNotAskedAbout(t *testing.T) {
	h := newHarnessWith(t, func(m *Manager) { m.QuietAfter = time.Second })
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	if _, err := h.m.RecordRoom(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.m.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond)
	if n := countTitled(h.noticesNow(), "Still recording?"); n != 0 {
		t.Fatalf("a paused room was asked about %d times", n)
	}
}

func TestACallIsNeverAskedAboutSilence(t *testing.T) {
	h := newHarnessWith(t, func(m *Manager) { m.QuietAfter = time.Second })
	h.rec.tracks = []fakeTrackSpec{{Me, "mic", speech(SampleRate)}}
	if _, err := h.m.Record(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond)
	if n := countTitled(h.noticesNow(), "Still recording?"); n != 0 {
		t.Fatalf("a call was asked about silence %d times", n)
	}
}

func TestPickMicTakesTheChosenOneOrSaysTheDefault(t *testing.T) {
	mics := []Microphone{{ID: "alsa_input.pci", Name: "Built-in Audio"}, {ID: "alsa_input.usb", Name: "USB Speakerphone"}}
	cases := []struct {
		chosen, def string
		id, name    string
		missing     bool
	}{
		{"alsa_input.usb", "alsa_input.pci", "alsa_input.usb", "USB Speakerphone", false},
		{"", "alsa_input.pci", "alsa_input.pci", "Built-in Audio", false},
		{"unplugged", "alsa_input.pci", "alsa_input.pci", "Built-in Audio", true},
		{"unplugged", "", "", "", true},
		{"", "alsa_output.pci.monitor", "", "", false},
	}
	for _, c := range cases {
		id, name, missing := pickMic(mics, c.chosen, c.def)
		if id != c.id || name != c.name || missing != c.missing {
			t.Errorf("pickMic(%q, %q) = %q, %q, %v; want %q, %q, %v", c.chosen, c.def, id, name, missing, c.id, c.name, c.missing)
		}
	}
}

func TestARoomIsNeverAskedAboutOnceItStopped(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	if _, err := h.m.RecordRoom(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.m.mu.Lock()
	r := h.m.rec
	h.m.mu.Unlock()
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	// The watch decided to ask just as the recording stopped.
	h.m.quietNotice(r, true)
	if n := countTitled(h.noticesNow(), "Still recording?"); n != 0 {
		t.Fatalf("a stopped recording was asked about %d times", n)
	}
}

func TestTheQuestionSaysFifteenMinutesAndHowItEnds(t *testing.T) {
	h := newHarness(t)
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	if _, err := h.m.RecordRoom(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.m.mu.Lock()
	r := h.m.rec
	h.m.mu.Unlock()
	h.m.quietNotice(r, true)
	ns := h.noticesNow()
	ask := ns[len(ns)-1]
	if ask.Title != "Still recording?" || !strings.Contains(ask.Body, "Nobody has spoken for 15 minutes.") || !strings.Contains(ask.Body, "until you press Stop or it has run 4 hours") {
		t.Fatalf("question = %+v", ask)
	}
}

func TestAnInRoomTranscriptReadsInFull(t *testing.T) {
	loc := time.FixedZone("EDT", -4*3600)
	info := TranscriptInfo{
		Title: "Meeting in the room 2026-10-27 09:00", Started: time.Date(2026, 10, 27, 9, 0, 5, 0, loc), Ended: time.Date(2026, 10, 27, 9, 30, 5, 0, loc),
		FarSide: FarSideNone, Language: "en", Profile: "english", Model: "small.en-q5_1",
		InRoom: true, Microphone: "USB Speakerphone",
	}
	doc, err := Render(info, MergeRoom([]Segment{
		{Start: 2 * time.Second, End: 3 * time.Second, Text: "Shall we start?"},
		{Start: 3 * time.Second, End: 4 * time.Second, Text: "Yes."},
		{Start: 90 * time.Second, End: 92 * time.Second, Text: "Next item."},
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := `---
type: meeting-transcript
format: agentry-meeting-transcript/1
title: Meeting in the room 2026-10-27 09:00
started: "2026-10-27T09:00:05-04:00"
ended: "2026-10-27T09:30:05-04:00"
duration_seconds: 1800
timezone: EDT
app: none
app_detection: none
participants: unknown
title_source: recording
speakers:
    - label: Room
      source: microphone
      note: everyone in the room; not split by person
language: en
language_profile: english
model: small.en-q5_1
transcriber: whisper.cpp v1.9.5, voice detection silero-v6.2.0
far_side: none
echo: none
output_device: none
setting: in-room
microphone: USB Speakerphone
---

# Meeting in the room 2026-10-27 09:00

Transcript of a meeting in the room, produced automatically on this computer from one microphone. "Room" is everyone in the room, not split by person. Words may be wrong. Times are from the start of the recording.

## Transcript

[00:00:02] Room: Shall we start? Yes.
[00:01:30] Room: Next item.
`
	if string(doc) != want {
		t.Fatalf("in-room transcript:\n%s\nwant:\n%s", doc, want)
	}
}

func TestAMicrophoneThatChangesWhileRecordingIsSaid(t *testing.T) {
	h := newHarness(t)
	h.rec.reportsOnly = true
	h.rec.micName = "USB Speakerphone"
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	mt, err := h.m.RecordRoom(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c := h.rec.capture
	c.mu.Lock()
	c.micName, c.micMissing = "Built-in Audio", true
	c.mu.Unlock()
	waitFor(t, "the change said", func() bool { return countTitled(h.noticesNow(), "Recording another microphone") == 1 })
	ns := h.noticesNow()
	var said Notice
	for _, n := range ns {
		if n.Title == "Recording another microphone" {
			said = n
		}
	}
	if !strings.Contains(said.Body, "Built-in Audio") || !strings.Contains(said.Body, "was not found") {
		t.Fatalf("notice %+v", said)
	}
	if got := h.meeting(t, mt.ID); got.Microphone != "Built-in Audio" || !got.MicMissing {
		t.Fatalf("meeting %+v", got)
	}
	if st := h.m.Status(); st.Recording == nil || !st.Recording.MicMissing {
		t.Fatalf("status %+v", st.Recording)
	}
}
