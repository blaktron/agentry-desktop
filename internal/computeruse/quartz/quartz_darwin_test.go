//go:build darwin

package quartz

import (
	"context"
	"errors"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// The window list reads without any permission: every window on screen,
// front to back, each with its process and a size. A process outside a
// login session (one started over SSH) reaches no window server, and the
// test has nothing to read there.
func TestTheWindowListReadsWithoutPermission(t *testing.T) {
	ws, err := all()
	if err != nil {
		t.Skipf("no window server for this process (not in a login session): %v", err)
	}
	t.Logf("%d windows on screen; permissions %+v; ScreenCaptureKit %v", len(ws), Granted(), CaptureKit())
	for _, w := range ws {
		if w.pid <= 0 {
			t.Errorf("window %d has no process", w.id)
		}
	}
}

// Without Screen Recording and Accessibility, opening computer use is
// refused as not granted, naming what to turn on.
func TestOpenNamesTheMissingPermissions(t *testing.T) {
	p := Granted()
	_, err := Open(context.Background(), Options{Scope: computeruse.ScopeApp, Match: func(string, string) (string, bool) { return "", false }})
	if p.Screen && p.Accessibility {
		if err != nil {
			t.Fatalf("both granted, Open refused: %v", err)
		}
		return
	}
	if !errors.Is(err, computeruse.ErrNotGranted) {
		t.Fatalf("Open with %+v: %v", p, err)
	}
}

// A window in front holding the point covers the allowed one; one behind
// it, or one that does not hold the point, does not.
func TestCoveredAtIsTheWindowsInFront(t *testing.T) {
	ws := []win{
		{id: 1, x: 0, y: 0, w: 100, h: 30, alpha: 1},
		{id: 2, x: 0, y: 0, w: 800, h: 600, alpha: 1},
		{id: 3, x: 0, y: 0, w: 800, h: 600, alpha: 1},
	}
	if !coveredAt(ws, 2, 10, 10) {
		t.Error("a window in front at the point does not cover")
	}
	if coveredAt(ws, 2, 400, 300) {
		t.Error("the point outside the front window is covered")
	}
	ws[0].alpha = 0
	if coveredAt(ws, 2, 10, 10) {
		t.Error("an invisible window covers")
	}
	if !coveredAt(ws, 9, 10, 10) {
		t.Error("a window no longer listed is not refused")
	}
}
