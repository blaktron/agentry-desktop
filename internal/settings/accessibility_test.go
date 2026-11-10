package settings

import (
	"strings"
	"testing"
)

// The text size is the default until one is chosen, an offered size is
// kept, the default is stored as none, a size this app does not offer is
// refused on the way in and read as the default on the way out, and Reset
// Home leaves it alone.
func TestTextSizeIsKeptAndTheDefaultIsStoredAsNone(t *testing.T) {
	isolate(t)
	if got, err := TextSizeNow(); err != nil || got != TextSizeDefault {
		t.Fatalf("nothing stored reads %d, %v", got, err)
	}
	if err := SetTextSize(150); err != nil {
		t.Fatal(err)
	}
	if got, _ := TextSizeNow(); got != 150 {
		t.Fatalf("after 150 = %d", got)
	}
	if !strings.Contains(readRaw(t), "textSize: 150") {
		t.Fatalf("not written:\n%s", readRaw(t))
	}
	for _, bad := range []int{0, 90, 133, 300} {
		if err := SetTextSize(bad); err == nil {
			t.Fatalf("%d%% was accepted", bad)
		}
	}
	if err := ResetHome(); err != nil {
		t.Fatal(err)
	}
	if got, _ := TextSizeNow(); got != 150 {
		t.Fatalf("Reset Home changed the text size to %d", got)
	}
	if err := SetTextSize(TextSizeDefault); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readRaw(t), "textSize") {
		t.Fatalf("the default was stored:\n%s", readRaw(t))
	}
	writeRaw(t, "textSize: 333\n")
	if got, _ := TextSizeNow(); got != TextSizeDefault {
		t.Fatalf("a size this app does not offer reads %d", got)
	}
}

// A step moves one offered size up or down, holds at the ends, and 0 is
// the default.
func TestTextSizeStepsThroughTheOfferedSizes(t *testing.T) {
	for _, c := range []struct{ from, step, want int }{
		{100, 1, 110}, {110, 1, 125}, {200, 1, 200},
		{125, -1, 110}, {100, -1, 100},
		{175, 0, 100}, {333, 1, 110}, {333, -1, 100},
	} {
		if got := TextSizeStep(c.from, c.step); got != c.want {
			t.Errorf("TextSizeStep(%d, %d) = %d, want %d", c.from, c.step, got, c.want)
		}
	}
}

// Motion follows the computer until Reduce is chosen; System is stored as
// none and anything else is refused.
func TestMotionIsSystemUntilReduceIsChosen(t *testing.T) {
	isolate(t)
	if got, _ := MotionNow(); got != MotionSystem {
		t.Fatalf("nothing stored reads %q", got)
	}
	if err := SetMotion(MotionReduce); err != nil {
		t.Fatal(err)
	}
	if got, _ := MotionNow(); got != MotionReduce || !strings.Contains(readRaw(t), "motion: reduce") {
		t.Fatalf("reduce: %q\n%s", got, readRaw(t))
	}
	if err := SetMotion("fast"); err == nil {
		t.Fatal("a motion choice this app lacks was accepted")
	}
	if err := SetMotion(MotionSystem); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readRaw(t), "motion") {
		t.Fatalf("System was stored:\n%s", readRaw(t))
	}
}
