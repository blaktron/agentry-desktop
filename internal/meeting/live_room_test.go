//go:build windows || darwin

package meeting

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveRoom records a meeting in the room through this computer's own
// audio stack for a few seconds, with no transcription. It runs only when
// AGENTRY_MEETING_LIVE is set. It asks that the microphones are listed,
// none of them this app's own device, that the one chosen is the one
// recorded, alone on a Room track with no far side (or, where the system
// will not start it, that the start fails), and that a chosen one
// that is missing gives way to the default and says so. The microphone
// hears the room: what it records is counted, never kept.
func TestLiveRoom(t *testing.T) {
	if os.Getenv("AGENTRY_MEETING_LIVE") == "" {
		t.Skip("AGENTRY_MEETING_LIVE is not set")
	}
	if s := PlatformSupport(); s != "" {
		t.Skip(s)
	}
	rr, ok := NewRecorder().(RoomRecorder)
	if !ok {
		t.Fatal("this computer's recorder does not record a room")
	}
	mics, err := rr.Microphones()
	if err != nil || len(mics) == 0 {
		t.Fatalf("microphones %+v, %v", mics, err)
	}
	for _, m := range mics {
		t.Logf("microphone %q (%s)", m.Name, m.ID)
		if m.Name == "Agentry Meeting notes" {
			t.Fatalf("this app's own device is listed as a microphone: %+v", m)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorded := 0
	for _, chosen := range mics {
		tracks := &memTracks{}
		c, err := rr.Record(ctx, Call{Room: true, Mic: chosen.ID}, tracks)
		if err != nil {
			// A device the system will not start fails the start, never
			// a recording of nothing.
			t.Logf("%q refused: %v", chosen.Name, err)
			continue
		}
		if c.FarSide() != FarSideNone {
			t.Errorf("%q: far side %q, want none", chosen.Name, c.FarSide())
		}
		if name, missing := c.(MicReporter).Microphone(); name != chosen.Name || missing {
			t.Errorf("%q: recording %q (missing %v)", chosen.Name, name, missing)
		}
		time.Sleep(3 * time.Second)
		if err := c.Stop(); err != nil {
			t.Fatal(err)
		}
		tracks.mu.Lock()
		room := tracks.tracks["room"]
		n := len(tracks.tracks)
		tracks.mu.Unlock()
		if n != 1 || room == nil || room.label != Room {
			t.Fatalf("%q: tracks %+v, want one Room track", chosen.Name, tracks.tracks)
		}
		if samples, ended, _ := tracks.get("room"); samples == 0 || !ended {
			t.Fatalf("%q: the room track has %d samples, ended %v", chosen.Name, samples, ended)
		}
		recorded++
	}
	if recorded == 0 {
		t.Fatal("no microphone recorded")
	}
	c, err := rr.Record(ctx, Call{Room: true, Mic: "no such microphone"}, &memTracks{})
	if err != nil {
		t.Fatal(err)
	}
	name, missing := c.(MicReporter).Microphone()
	_ = c.Stop()
	if !missing || name == "" {
		t.Fatalf("a missing choice recorded %q, missing %v", name, missing)
	}
}
