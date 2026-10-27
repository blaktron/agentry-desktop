package meeting

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func (f *fakeAudio) unplug(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gone[id] = true
	f.mics = slices.DeleteFunc(f.mics, func(m Microphone) bool { return m.ID == id })
}

func (f *fakeAudio) plug(m Microphone) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.gone, m.ID)
	f.mics = append(f.mics, m)
}

// A room's microphone unplugged mid-recording gives way to the default at
// once, and is read again when it is plugged back in; the parent has both
// changes.
func TestARoomsUnpluggedMicrophoneGivesWayToTheDefaultAndComesBack(t *testing.T) {
	f := newFakeAudio()
	child := &inProcessChild{f: f}
	tracks := &memTracks{}
	c, err := childRecorder{spawn: child.spawn}.Record(context.Background(), Call{Room: true, Mic: "usb-mic"}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	mf := c.(MicFollower)
	f.unplug("usb-mic")
	eventually(t, "the loss", func() bool { return len(mf.MicChanges()) == 1 })
	if got := mf.MicChanges()[0]; got.Lost != "USB Speakerphone" || got.Now != "Microphone Array" {
		t.Fatalf("change %+v, want the default in its place", got)
	}
	if name, _ := c.(MicReporter).Microphone(); name != "Microphone Array" {
		t.Fatalf("microphone %q", name)
	}
	f.plug(Microphone{ID: "usb-mic", Name: "USB Speakerphone"})
	eventually(t, "the return", func() bool { return len(mf.MicChanges()) == 2 })
	if got := mf.MicChanges()[1]; got.Lost != "" || got.Now != "USB Speakerphone" {
		t.Fatalf("change %+v, want the room's microphone back", got)
	}
	if got := f.events(); !slices.Contains(got, "open mic laptop-mic") || slices.Index(got, "open mic laptop-mic") > slices.Index(got, "close mic laptop-mic") {
		t.Fatalf("opened %v", got)
	}
	if _, _, ok := tracks.get("far"); ok {
		t.Fatal("a room opened a far track")
	}
}

// With no other microphone, a loss reads none, and the next microphone
// connected as the default is read.
func TestAnUnpluggedMicrophoneWithNoOtherReadsNoneUntilOneIsConnected(t *testing.T) {
	f := newFakeAudio()
	var out bytes.Buffer
	r, now := newRecording(f, recordTarget{}, &out)
	r.room = true
	if err := r.chooseMic("usb-mic"); err != nil {
		t.Fatal(err)
	}
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	f.unplug("usb-mic")
	f.unplug("laptop-mic")
	r.collect()
	*now = now.Add(time.Second)
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	if r.mic != nil {
		t.Fatalf("reading %q with no microphone connected", r.micEP)
	}
	f.plug(Microphone{ID: "headset-mic", Name: "Headset"})
	f.mu.Lock()
	f.defaults[flowCapture] = "headset-mic"
	f.mu.Unlock()
	*now = now.Add(time.Second)
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	got := r.follow.list()
	if len(got) != 2 || got[0].Lost != "USB Speakerphone" || got[0].Now != "" || got[1].Now != "Headset" {
		t.Fatalf("changes %+v", got)
	}
	var evs []wireEvent
	rd := bufio.NewReader(&out)
	for {
		kind, _, payload, err := readFrame(rd)
		if err != nil {
			break
		}
		var ev wireEvent
		if kind == frameEvent && json.Unmarshal(payload, &ev) == nil && ev.Type == evMic {
			evs = append(evs, ev)
		}
	}
	if len(evs) != 2 || len(evs[1].MicChanges) != 2 || evs[1].MicName != "Headset" {
		t.Fatalf("mic events %+v", evs)
	}
}

// A recording with nothing chosen follows the default, and a new default
// is a change the parent hears of.
func TestRecordWithNoAppSaysANewDefaultMicrophone(t *testing.T) {
	f := newFakeAudio()
	var out bytes.Buffer
	r, _ := newRecording(f, recordTarget{}, &out)
	if err := r.chooseMic(""); err != nil {
		t.Fatal(err)
	}
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.defaults[flowCapture] = "usb-mic"
	f.mu.Unlock()
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	if got := r.follow.list(); len(got) != 1 || got[0].Lost != "" || got[0].Now != "USB Speakerphone" || r.micEP != "usb-mic" {
		t.Fatalf("changes %+v, reading %q", got, r.micEP)
	}
}

// A child started again keeps the changes the earlier one sent.
func TestARestartedChildKeepsTheChangesSoFar(t *testing.T) {
	c := &childCapture{room: true, tracks: &memTracks{}}
	var b bytes.Buffer
	w := &frameWriter{w: &b}
	t0 := time.Unix(1000, 0).UTC()
	lost := MicChange{At: t0, Lost: "USB Speakerphone", Now: "Microphone Array"}
	_ = w.event(wireEvent{Type: evMic, MicName: "Microphone Array", MicChanges: []MicChange{lost}})
	_ = w.event(wireEvent{Type: evStarted, FarSide: FarSideNone, MicName: "Microphone Array", MicMissing: true})
	back := MicChange{At: t0.Add(time.Minute), Now: "Headset"}
	_ = w.event(wireEvent{Type: evMic, MicName: "Headset", MicChanges: []MicChange{back}})
	_ = c.follow(bufio.NewReader(&b))
	got := c.MicChanges()
	if len(got) != 2 || !got[0].At.Equal(lost.At) || got[0].Lost != lost.Lost || got[1].Now != "Headset" {
		t.Fatalf("changes %+v", got)
	}
	if name, _ := c.Microphone(); name != "Headset" {
		t.Fatalf("microphone %q", name)
	}
}

// A child started again on a microphone, after the last one read none or
// another, is a change: the gap closes and the new microphone is said.
func TestAChildStartedAgainOnAMicrophoneIsAChange(t *testing.T) {
	c := &childCapture{room: true, micName: "USB Speakerphone", tracks: &memTracks{}}
	var b bytes.Buffer
	w := &frameWriter{w: &b}
	_ = w.event(wireEvent{Type: evMic, MicName: "USB Speakerphone", MicChanges: []MicChange{{At: time.Unix(1000, 0), Lost: "USB Speakerphone"}}})
	_ = w.event(wireEvent{Type: evStarted, FarSide: FarSideNone, MicName: "Microphone Array", MicMissing: true})
	_ = c.follow(bufio.NewReader(&b))
	got := c.MicChanges()
	if len(got) != 2 || got[1].Now != "Microphone Array" || got[1].Lost != "" {
		t.Fatalf("changes %+v, want the restart's microphone as a change", got)
	}
}

// A microphone that goes away during a pause is a loss, nothing is opened
// while paused, and the default is read at the resume.
func TestAMicrophoneThatGoesAwayDuringAPauseIsReadAgainAtTheResume(t *testing.T) {
	f := newFakeAudio()
	var out bytes.Buffer
	r, now := newRecording(f, recordTarget{}, &out)
	r.room = true
	if err := r.chooseMic("usb-mic"); err != nil {
		t.Fatal(err)
	}
	if err := r.scan(true); err != nil {
		t.Fatal(err)
	}
	r.setPaused(true)
	f.unplug("usb-mic")
	*now = now.Add(time.Second)
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	if n := f.openCount(); n != 0 {
		t.Fatalf("%d streams open while paused: %v", n, f.events())
	}
	if got := r.follow.list(); len(got) != 1 || got[0].Lost != "USB Speakerphone" || got[0].Now != "" {
		t.Fatalf("changes %+v while paused", got)
	}
	r.setPaused(false)
	*now = now.Add(time.Second)
	if err := r.scan(false); err != nil {
		t.Fatal(err)
	}
	if got := r.follow.list(); len(got) != 2 || got[1].Now != "Microphone Array" || r.micEP != "laptop-mic" {
		t.Fatalf("changes %+v, reading %q after the resume", got, r.micEP)
	}
	for _, ev := range f.events() {
		if strings.Contains(ev, "loopback") {
			t.Fatalf("a room opened %q", ev)
		}
	}
}
