package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The menu bar carries the editing commands a text field's shortcuts need,
// and nothing that reloads the page or leaves the app. The
// application's own menu, which needs the running application, is left out.
func TestTheAppMenuCarriesEditingButNoReload(t *testing.T) {
	m := application.NewMenu()
	addWindowMenus(m, func(int) {})
	for _, role := range []application.Role{
		application.Undo, application.Redo, application.Cut, application.Copy,
		application.Paste, application.SelectAll,
	} {
		if m.FindByRole(role) == nil {
			t.Errorf("the app menu has no item for role %d", role)
		}
	}
	for _, role := range []application.Role{application.Reload, application.ForceReload, application.OpenDevTools, application.ZoomIn, application.ZoomOut, application.ResetZoom} {
		if m.FindByRole(role) != nil {
			t.Errorf("the app menu offers role %d", role)
		}
	}
	if m.FindByLabel("Help") != nil || m.FindByLabel("Learn More") != nil {
		t.Error("the app menu has a Help menu")
	}
}

// The View menu has the text size items, each with its zoom key. They
// step the kept text size (Services.StepTextSize's own test).
func TestTheViewMenuStepsTheTextSize(t *testing.T) {
	m := application.NewMenu()
	addWindowMenus(m, func(int) {})
	for _, label := range []string{"Zoom In", "Zoom Out", "Actual Size"} {
		item := m.FindByLabel(label)
		if item == nil {
			t.Fatalf("no %s item", label)
		}
		if item.GetAccelerator() == "" {
			t.Errorf("%s has no shortcut", label)
		}
	}
}

// The application menu is macOS's alone: on Linux it would be drawn inside
// every window, and Windows needs none.
func TestTheAppMenuIsSetOnMacOSOnly(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "linux": false, "windows": false, "freebsd": false} {
		if got := usesAppMenu(goos); got != want {
			t.Errorf("usesAppMenu(%q) = %v, want %v", goos, got, want)
		}
	}
}
