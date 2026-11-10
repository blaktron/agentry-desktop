package app

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// zoomed records what the window's zoom was set to.
func zoomed(t *testing.T) *[]int {
	t.Helper()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	var got []int
	saved := windowZoom
	windowZoom = func(percent int) { got = append(got, percent) }
	t.Cleanup(func() { windowZoom = saved })
	return &got
}

// Setting a text size keeps it and zooms the window to it at once; a size
// not offered is refused and the window is left alone.
func TestSetTextSizeKeepsItAndZoomsTheWindow(t *testing.T) {
	got := zoomed(t)
	h := &HomeAPI{}
	if size, err := h.SetTextSize(150); err != nil || size != 150 {
		t.Fatalf("SetTextSize(150) = %d, %v", size, err)
	}
	if now, _ := h.TextSize(); now != 150 {
		t.Fatalf("kept %d", now)
	}
	if _, err := h.SetTextSize(140); err == nil {
		t.Fatal("140% was accepted")
	}
	if len(*got) != 1 || (*got)[0] != 150 {
		t.Fatalf("window zoomed to %v, want [150]", *got)
	}
}

// The zoom keys step through the offered sizes and back to the default, and
// each step is kept and put on the window.
func TestStepTextSizeStepsKeepsAndZooms(t *testing.T) {
	got := zoomed(t)
	h := &HomeAPI{}
	for _, c := range []struct{ step, want int }{{1, 110}, {1, 125}, {-1, 110}, {0, 100}, {-1, 100}} {
		if size, err := h.StepTextSize(c.step); err != nil || size != c.want {
			t.Fatalf("StepTextSize(%d) = %d, %v; want %d", c.step, size, err, c.want)
		}
	}
	if now, _ := settings.TextSizeNow(); now != 100 {
		t.Fatalf("kept %d", now)
	}
	if want := []int{110, 125, 110, 100, 100}; len(*got) != len(want) {
		t.Fatalf("window zoomed %v, want %v", *got, want)
	}
}

// The View menu's steps are the page's: kept, put on the window, and told
// to the page with the new size.
func TestTheMenusTextSizeStepIsToldToThePage(t *testing.T) {
	got := zoomed(t)
	var events []any
	saved := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev == eventTextSize {
			events = append(events, data...)
		}
	}
	t.Cleanup(func() { emitEvent = saved })
	(&Services{}).StepTextSize(1)
	if len(*got) != 1 || (*got)[0] != 110 || len(events) != 1 || events[0] != 110 {
		t.Fatalf("zoom %v, events %v", *got, events)
	}
}

// The stored text size is put on the window as the app starts; the default
// is left as the window has it.
func TestTheStoredTextSizeIsAppliedAtStart(t *testing.T) {
	got := zoomed(t)
	applyTextSize()
	if len(*got) != 0 {
		t.Fatalf("the default zoomed the window: %v", *got)
	}
	if err := settings.SetTextSize(175); err != nil {
		t.Fatal(err)
	}
	applyTextSize()
	if len(*got) != 1 || (*got)[0] != 175 {
		t.Fatalf("window zoomed %v, want [175]", *got)
	}
}

// /theme.js marks the root for reduced motion when the person chose it,
// beside the theme, and says nothing about motion otherwise.
func TestThemeScriptMarksReducedMotion(t *testing.T) {
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	if err := settings.SetMotion(settings.MotionReduce); err != nil {
		t.Fatal(err)
	}
	if got := themeJS(t, http.MethodGet, "/theme.js").Body.String(); got != "document.documentElement.dataset.motion = \"reduce\";\n" {
		t.Fatalf("reduce alone: %q", got)
	}
	if err := settings.SetTheme("contrast"); err != nil {
		t.Fatal(err)
	}
	want := "document.documentElement.dataset.theme = \"contrast\";\ndocument.documentElement.dataset.motion = \"reduce\";\n"
	if got := themeJS(t, http.MethodGet, "/theme.js").Body.String(); got != want {
		t.Fatalf("contrast and reduce: %q", got)
	}
}
