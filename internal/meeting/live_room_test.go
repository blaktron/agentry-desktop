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
// recorded, alone on a Room track with no far side, and that a chosen one
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
	chosen := mics[len(mics)-1]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracks := &memTracks{}
	c, err := rr.Record(ctx, Call{Room: true, Mic: chosen.ID}, tracks)
	if err != nil {
		t.Fatal(err)
	}
	if c.FarSide() != FarSideNone {
		t.Errorf("far side %q, want none", c.FarSide())
	}
	if name, missing := c.(MicReporter).Microphone(); name != chosen.Name || missing {
		t.Errorf("recording %q (missing %v), want %q", name, missing, chosen.Name)
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
		t.Fatalf("tracks %+v, want one Room track", tracks.tracks)
	}
	if samples, ended, _ := tracks.get("room"); samples == 0 || !ended {
		t.Fatalf("the room track has %d samples, ended %v", samples, ended)
	}
	c, err = rr.Record(ctx, Call{Room: true, Mic: "no such microphone"}, &memTracks{})
	if err != nil {
		t.Fatal(err)
	}
	name, missing := c.(MicReporter).Microphone()
	_ = c.Stop()
	if !missing || name == "" {
		t.Fatalf("a missing choice recorded %q, missing %v", name, missing)
	}
}
