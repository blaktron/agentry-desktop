package meeting

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMicFollowGivesWayToTheDefaultAndTakesThePinnedOneBack(t *testing.T) {
	usb := Microphone{ID: "usb", Name: "USB Speakerphone"}
	builtin := Microphone{ID: "builtin", Name: "Built-in Audio"}
	f := micFollow{pinned: "usb"}
	f.started("usb", "USB Speakerphone")
	t0 := time.Unix(1000, 0)

	// Unplugged: the default is read at once, as one change.
	listed := []Microphone{builtin}
	if !f.gone(listed) {
		t.Fatal("the unplugged microphone is not gone")
	}
	f.lost(t0)
	id, name := f.target(listed, "builtin")
	if id != "builtin" {
		t.Fatalf("target %q, want the default", id)
	}
	f.reading(id, name, t0)

	// Plugged back in: the pinned one is read again.
	listed = []Microphone{builtin, usb}
	if f.gone(listed) {
		t.Fatal("the default is gone while it is listed")
	}
	id, name = f.target(listed, "builtin")
	f.reading(id, name, t0.Add(time.Minute))

	want := []MicChange{
		{At: t0, Lost: "USB Speakerphone", Now: "Built-in Audio"},
		{At: t0.Add(time.Minute), Now: "USB Speakerphone"},
	}
	if got := f.list(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("changes %+v, want %+v", got, want)
	}
}

func TestMicFollowReadsNoneUntilAMicrophoneIsConnected(t *testing.T) {
	f := micFollow{pinned: "usb"}
	f.started("usb", "USB Speakerphone")
	t0 := time.Unix(1000, 0)
	// Nothing else is connected, and the default is an output's monitor:
	// nothing is read.
	f.lost(t0)
	if id, _ := f.target(nil, "speakers.monitor"); id != "" {
		t.Fatalf("target %q, want none", id)
	}
	f.reading("", "", t0)
	// Another microphone is connected and made the default.
	headset := Microphone{ID: "headset", Name: "Headset"}
	id, name := f.target([]Microphone{headset}, "headset")
	f.reading(id, name, t0.Add(30*time.Second))
	got := f.list()
	if len(got) != 2 || got[0].Lost != "USB Speakerphone" || got[0].Now != "" || got[1].Now != "Headset" {
		t.Fatalf("changes %+v", got)
	}
}

func TestMicFollowFollowsTheDefaultWhenNothingIsPinned(t *testing.T) {
	f := micFollow{}
	f.started("a", "A")
	id, _ := f.target([]Microphone{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}, "b")
	if id != "b" {
		t.Fatalf("target %q, want the new default", id)
	}
}

func TestAMicrophoneThatGoesAwayIsSaidAndTheTranscriptMarksTheGap(t *testing.T) {
	h := newHarness(t)
	h.rec.micName = "USB Speakerphone"
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	h.eng.byFile["room.wav"] = []Segment{{Start: time.Second, End: 2 * time.Second, Text: "Hello."}}
	mt, err := h.m.RecordRoom(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	c := h.rec.capture

	c.change(MicChange{Lost: "USB Speakerphone"})
	waitFor(t, "the loss said", func() bool { return countTitled(h.noticesNow(), "Microphone disconnected") == 1 })
	if n := lastTitled(h.noticesNow(), "Microphone disconnected"); !strings.Contains(n.Body, "USB Speakerphone was disconnected") || !strings.Contains(n.Body, "no other microphone could be read, so nothing is recorded") || n.Tag != tagMic {
		t.Fatalf("notice %+v", n)
	}
	if st := h.m.Status(); st.Recording == nil || !st.Recording.MicNone || st.Recording.MicLost != "USB Speakerphone" {
		t.Fatalf("status %+v, want no microphone shown", st.Recording)
	}

	c.change(MicChange{Now: "USB Speakerphone"})
	waitFor(t, "the return said", func() bool { return countTitled(h.noticesNow(), "Microphone connected again") == 1 })
	if st := h.m.Status(); st.Recording == nil || st.Recording.MicNone || st.Recording.MicLost != "" {
		t.Fatalf("status %+v, want the microphone back", st.Recording)
	}
	if n := countTitled(h.noticesNow(), "Recording another microphone"); n != 0 {
		t.Fatalf("the change was said %d more times", n)
	}

	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	got := h.meeting(t, mt.ID)
	if len(got.Gaps) != 1 || got.Gaps[0].Reason != "microphone disconnected" || got.Gaps[0].To < got.Gaps[0].From {
		t.Fatalf("gaps %+v, want the stretch with no microphone", got.Gaps)
	}
	raw, _ := os.ReadFile(got.Transcript)
	for _, want := range []string{"reason: microphone disconnected", "USB Speakerphone was disconnected, and no other microphone could be read", "USB Speakerphone was recorded from then on."} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("transcript lacks %q:\n%s", want, raw)
		}
	}
}

func TestAMicrophoneThatGoesAwayGivesWayToTheDefaultAndSaysSo(t *testing.T) {
	h := newHarness(t)
	h.rec.micName = "USB Speakerphone"
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	h.eng.byFile["room.wav"] = []Segment{{Start: time.Second, End: 2 * time.Second, Text: "Hello."}}
	mt, err := h.m.RecordRoom(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	h.rec.capture.change(MicChange{Lost: "USB Speakerphone", Now: "Built-in Audio"})
	waitFor(t, "the loss said", func() bool { return countTitled(h.noticesNow(), "Microphone disconnected") == 1 })
	if n := lastTitled(h.noticesNow(), "Microphone disconnected"); !strings.Contains(n.Body, "now records Built-in Audio instead") {
		t.Fatalf("notice %+v", n)
	}
	st := h.m.Status()
	if st.Recording == nil || st.Recording.MicNone || st.Recording.MicLost != "USB Speakerphone" || st.Recording.Microphone != "Built-in Audio" {
		t.Fatalf("status %+v", st.Recording)
	}
	// One notice for one change: not also "Recording another microphone".
	time.Sleep(1200 * time.Millisecond)
	if n := countTitled(h.noticesNow(), "Recording another microphone"); n != 0 {
		t.Fatalf("the change was said %d more times", n)
	}
	if err := h.m.Stop(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the transcript", func() bool { return h.meeting(t, mt.ID).State == StateDone })
	got := h.meeting(t, mt.ID)
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps %+v: another microphone was recorded throughout", got.Gaps)
	}
	raw, _ := os.ReadFile(got.Transcript)
	if !strings.Contains(string(raw), "USB Speakerphone was disconnected, and Built-in Audio was recorded instead.") {
		t.Errorf("transcript lacks the change:\n%s", raw)
	}
}

func lastTitled(ns []Notice, title string) Notice {
	var out Notice
	for _, n := range ns {
		if n.Title == title {
			out = n
		}
	}
	return out
}

func TestMicFollowKeepsTheOneReadWhenNeitherThePinnedOneNorTheDefaultIsListed(t *testing.T) {
	f := micFollow{pinned: "usb"}
	f.started("builtin", "Built-in Audio")
	// The default became an output's monitor, and the pinned one is not
	// back: the microphone read is still there and is kept.
	gone, id, _ := f.look([]Microphone{{ID: "builtin", Name: "Built-in Audio"}}, "speakers.monitor", time.Unix(1000, 0))
	if gone || id != "builtin" {
		t.Fatalf("gone %v, next %q, want the one read kept", gone, id)
	}
}

func TestANewMicrophoneSaysTheChosenOneIsStillMissing(t *testing.T) {
	h := newHarness(t)
	h.set(func(s *Settings) { s.Mic = "gone" })
	h.rec.micName, h.rec.micMissing = "Built-in Audio", true
	h.rec.tracks = []fakeTrackSpec{{Room, "room", speech(SampleRate)}}
	if _, err := h.m.RecordRoom(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	h.rec.capture.change(MicChange{Now: "Headset"})
	waitFor(t, "the change said", func() bool { return countTitled(h.noticesNow(), "Recording another microphone") == 1 })
	if n := lastTitled(h.noticesNow(), "Recording another microphone"); !strings.Contains(n.Body, "Headset") || !strings.Contains(n.Body, "was not found") {
		t.Fatalf("notice %+v", n)
	}
}
