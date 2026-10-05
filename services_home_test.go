package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Home's layout through its binding (plan desktop-home-and-library.md §4.4,
// D-067; agentry-desktop#658): the defaults until something is changed,
// one change per call, and a view that names every card in order.
func TestHomeBindingShowsHidesAndResets(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	h := &HomeAPI{}
	order := func(v *HomeLayoutView) string {
		var b []string
		for _, c := range v.Cards {
			k := c.Kind
			if c.Hidden {
				k += "(hidden)"
			}
			b = append(b, k)
		}
		return strings.Join(b, ",")
	}
	v, err := h.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if got := order(v); got != "recent-runs,installed,profile,run-defaults" || v.Newer {
		t.Fatalf("Layout() = %s newer=%v", got, v.Newer)
	}
	if v, err = h.SetShown("profile", false); err != nil {
		t.Fatal(err)
	}
	if got := order(v); got != "recent-runs,installed,profile(hidden),run-defaults" {
		t.Fatalf("SetShown answered %s", got)
	}
	if _, err := h.SetShown("pinned", false); err == nil {
		t.Fatal("a pinned card was shown or hidden by kind")
	}
	if v, err = h.Reset(); err != nil {
		t.Fatal(err)
	}
	if got := order(v); got != "recent-runs,installed,profile,run-defaults" {
		t.Fatalf("Reset answered %s", got)
	}
}

// A pin is local and is not a star (plan desktop-home-and-library.md §3,
// invariant 3; agentry-desktop#660): pin and unpin write desktop.yaml and
// ask no registry. The registry here fails the test on any request.
// Only a QuickAgent installed on this computer can be pinned, and one from
// a folder pins with no registry.
func TestPinIsLocalOnlyInstalledAndNotAStar(t *testing.T) {
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("pinning asked the registry: %s %s", r.Method, r.URL.Path)
		http.Error(w, "no", http.StatusTeapot)
	}))
	defer srv.Close()
	if err := settings.Save(settings.Settings{DefaultRegistry: srv.URL, Registries: []settings.Registry{{URL: srv.URL, Token: "agy_test"}}}); err != nil {
		t.Fatal(err)
	}
	var st installed.State
	st.Upsert(installed.Key{Registry: srv.URL, Namespace: "acme", Name: "triage", Target: "claude-skill", Dir: t.TempDir()}, "1.0.0", installed.CapSnapshot{}, true)
	st.Upsert(installed.Key{Registry: srv.URL, Namespace: "acme", Name: "gone", Target: "claude-skill", Dir: filepath.Join(t.TempDir(), "deleted")}, "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	h := &HomeAPI{}
	v, err := h.Pin("acme/triage", srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if last := v.Cards[len(v.Cards)-1]; last.Kind != "pinned" || len(last.Pins) != 1 || last.Pins[0].Ref != "acme/triage" || v.MaxPins != 16 {
		t.Fatalf("after Pin: %+v (maxPins %d)", last, v.MaxPins)
	}
	for _, c := range []struct{ ref, reg string }{
		{"acme/never", srv.URL},                  // not installed
		{"acme/gone", srv.URL},                   // its directory is gone
		{"acme/triage", "https://other.example"}, // installed from another registry
		{"acme/triage", ""},                      // not a folder install
	} {
		if _, err := h.Pin(c.ref, c.reg); err == nil || !strings.Contains(err.Error(), "not installed on this computer") {
			t.Fatalf("Pin(%s, %q) = %v", c.ref, c.reg, err)
		}
	}
	if v, err = h.Unpin("acme/triage", srv.URL); err != nil {
		t.Fatal(err)
	}
	if got := v.Cards[len(v.Cards)-1].Kind; got == "pinned" {
		t.Fatal("Unpin left the emptied Pinned card")
	}
}

// Uninstalling a QuickAgent, or forgetting its gone record, removes its pin
// once no other install of it is left (D5).
func TestUninstallAndForgetRemoveThePin(t *testing.T) {
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	const reg = "https://registry.test"
	keep, drop := t.TempDir(), t.TempDir()
	triage := installed.Key{Registry: reg, Namespace: "acme", Name: "triage", Target: "claude-skill", Dir: drop}
	triage2 := installed.Key{Registry: reg, Namespace: "acme", Name: "triage", Target: "codex-skill", Dir: keep}
	gone := installed.Key{Registry: reg, Namespace: "acme", Name: "gone", Target: "claude-skill", Dir: filepath.Join(t.TempDir(), "deleted")}
	var st installed.State
	st.Upsert(triage, "1.0.0", installed.CapSnapshot{}, true)
	st.Upsert(triage2, "1.0.0", installed.CapSnapshot{}, true)
	st.Upsert(gone, "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	if err := settings.PinToHome("acme/triage", reg); err != nil {
		t.Fatal(err)
	}
	if err := settings.PinToHome("acme/gone", reg); err != nil {
		t.Fatal(err)
	}
	i := &InstallAPI{app: &App{ctx: context.Background()}, engine: install.NewEngine()}
	if _, err := i.Uninstall(triage); err != nil {
		t.Fatal(err)
	}
	if !settings.IsPinned("acme/triage", reg) {
		t.Fatal("the pin went while another install of it is on this computer")
	}
	if _, err := i.Uninstall(triage2); err != nil {
		t.Fatal(err)
	}
	if settings.IsPinned("acme/triage", reg) {
		t.Fatal("uninstalling the last install left its pin")
	}
	if err := i.Forget(gone); err != nil {
		t.Fatal(err)
	}
	if settings.IsPinned("acme/gone", reg) {
		t.Fatal("forgetting the last record left its pin")
	}
}
