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
		name, desc  string
		missing     bool
	}{
		{"alsa_input.usb", "alsa_input.pci", "alsa_input.usb", "USB Speakerphone", false},
		{"", "alsa_input.pci", "alsa_input.pci", "Built-in Audio", false},
		{"unplugged", "alsa_input.pci", "alsa_input.pci", "Built-in Audio", true},
		{"unplugged", "", "", "", true},
		{"", "alsa_output.pci.monitor", "", "", false},
	}
	for _, c := range cases {
		name, desc, missing := pickMic(mics, c.chosen, c.def)
		if name != c.name || desc != c.desc || missing != c.missing {
			t.Errorf("pickMic(%q, %q) = %q, %q, %v; want %q, %q, %v", c.chosen, c.def, name, desc, missing, c.name, c.desc, c.missing)
		}
	}
}
