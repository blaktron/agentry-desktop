package tray

import (
	"bytes"
	"image/png"
	"testing"

	_ "embed"
)

//go:embed icons/tray.png
var testIcon []byte

func TestTheRecordingIconCarriesARedDot(t *testing.T) {
	out := withDot(testIcon)
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := png.Decode(bytes.NewReader(testIcon))
	if img.Bounds() != orig.Bounds() {
		t.Fatalf("bounds %v, want %v", img.Bounds(), orig.Bounds())
	}
	b := img.Bounds()
	r := b.Dx() * 3 / 16
	cr, cg, cb, ca := img.At(b.Max.X-r-1, b.Max.Y-r-1).RGBA()
	if cr>>8 != 0xe0 || cg>>8 != 0x24 || cb>>8 != 0x24 || ca>>8 != 0xff {
		t.Fatalf("the dot's centre is %x %x %x %x, want red", cr>>8, cg>>8, cb>>8, ca>>8)
	}
	if got := withDot([]byte("not a png")); string(got) != "not a png" {
		t.Fatal("an unreadable icon was not kept as it was")
	}
}

func TestMeetingLabels(t *testing.T) {
	if s, p := MeetingLabels(Meeting{Recording: true, What: "Zoom"}); s != "Recording Zoom" || p != "Pause recording" {
		t.Fatalf("recording: %q %q", s, p)
	}
	if s, p := MeetingLabels(Meeting{Recording: true, Paused: true}); s != "Recording paused: a meeting" || p != "Resume recording" {
		t.Fatalf("paused: %q %q", s, p)
	}
}
