package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The menu bar carries the editing commands a text field's shortcuts need,
// and zoom, and nothing that reloads the page or leaves the app.
func TestTheAppMenuCarriesEditingAndZoomButNoReload(t *testing.T) {
	m := appMenu()
	for _, role := range []application.Role{
		application.Undo, application.Redo, application.Cut, application.Copy,
		application.Paste, application.SelectAll,
		application.ResetZoom, application.ZoomIn, application.ZoomOut,
	} {
		if m.FindByRole(role) == nil {
			t.Errorf("the app menu has no item for role %d", role)
		}
	}
	for _, role := range []application.Role{application.Reload, application.ForceReload, application.OpenDevTools} {
		if m.FindByRole(role) != nil {
			t.Errorf("the app menu offers role %d", role)
		}
	}
	if m.FindByLabel("Help") != nil || m.FindByLabel("Learn More") != nil {
		t.Error("the app menu has a Help menu")
	}
}
