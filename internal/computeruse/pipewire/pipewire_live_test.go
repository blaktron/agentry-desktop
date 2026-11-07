//go:build linux && cgo

package pipewire

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// findNode returns the id of the PipeWire node with the given node.name.
func findNode(name string) (uint32, bool) {
	out, err := exec.Command("pw-dump").Output()
	if err != nil {
		return 0, false
	}
	var objs []struct {
		ID   uint32 `json:"id"`
		Info struct {
			Props map[string]any `json:"props"`
		} `json:"info"`
	}
	if json.NewDecoder(bytes.NewReader(out)).Decode(&objs) != nil {
		return 0, false
	}
	for _, o := range objs {
		if o.Info.Props["node.name"] == name {
			return o.ID, true
		}
	}
	return 0, false
}

func TestLiveCaptureSolidColour(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_PIPEWIRE") != "1" {
		t.Skip("set AGENTRY_LIVE_PIPEWIRE=1 to capture from the user's PipeWire daemon")
	}
	const name = "agentry-capture-test"
	prod := exec.Command("gst-launch-1.0", "-q", "videotestsrc", "pattern=solid-color",
		"foreground-color="+strconv.Itoa(0xff3366cc), "is-live=true", "!",
		"video/x-raw,format=BGRx,width=320,height=200,framerate=10/1", "!",
		"pipewiresink", "stream-properties=props,media.class=Video/Source,node.name="+name)
	prod.Stderr = os.Stderr
	if err := prod.Start(); err != nil {
		t.Skipf("no gst-launch-1.0: %v", err)
	}
	defer func() { _ = prod.Process.Kill(); _ = prod.Wait() }()

	var node uint32
	deadline := time.Now().Add(10 * time.Second)
	for {
		if id, ok := findNode(name); ok {
			node = id
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("producer node did not appear")
		}
		time.Sleep(200 * time.Millisecond)
	}

	s, err := Open(-1, node)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	img, err := s.Frame(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Size(); got.X != 320 || got.Y != 200 {
		t.Fatalf("size = %v, want 320x200", got)
	}
	if sz := s.Size(); sz.X != 320 || sz.Y != 200 {
		t.Fatalf("Size() = %v", sz)
	}
	near := func(a, b uint8) bool { return int(a)-int(b) <= 6 && int(b)-int(a) <= 6 }
	for _, p := range [][2]int{{0, 0}, {160, 100}, {319, 199}} {
		c := img.RGBAAt(p[0], p[1])
		t.Logf("pixel %v = %v", p, c)
		// foreground-color is ARGB 0xff3366cc.
		if !near(c.R, 0x33) || !near(c.G, 0x66) || !near(c.B, 0xcc) || c.A != 255 {
			t.Errorf("pixel %v = %v, want 33 66 cc ff", p, c)
		}
	}
	if _, err := s.Frame(ctx, true); err != nil {
		t.Fatalf("fresh frame: %v", err)
	}

	done := make(chan struct{})
	go func() { _ = s.Close(); _ = s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	if _, err := s.Frame(context.Background(), false); err == nil {
		t.Error("Frame after Close succeeded")
	}
}
