package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// resolveStub answers /api/v1/resolve with one version for any ref: the
// registry of a test that wants an answer, not a download.
func resolveStub(t *testing.T, version string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resolve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": "/att",
			"digest":    "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			"manifest":  map[string]any{"schemaVersion": "1", "name": "demo", "namespace": "acme", "kind": "skill", "version": version},
			"namespace": "acme", "name": "demo", "version": version, "yanked": false, "scanStatus": "pipeline",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// The last check's result is a snapshot the chrome reads (agentry-
// desktop#147, D8): a read never starts a check; a record whose registry
// cannot be reached is that record's line and counts nothing; only a row
// this app manages, whose directory is there, counts toward the badge;
// one BriefAgent's check on the agent page refreshes its row in place;
// an install, an update or a removal refreshes its row at once.
func TestInstallUpdatesStatusIsASnapshot(t *testing.T) {
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	t.Setenv("HOME", t.TempDir())
	newer := resolveStub(t, "2.0.0")
	// The runner record resolves through the app's default registry: the
	// stub, so no test reaches a real one.
	if err := settings.Save(settings.Settings{DefaultRegistry: newer, Registries: []settings.Registry{{URL: newer}}}); err != nil {
		t.Fatal(err)
	}
	i := &InstallAPI{app: &App{ctx: context.Background()}, engine: install.NewEngine()}

	before := i.UpdatesStatus()
	if before.Checked != "" || before.Available != 0 || before.Updates == nil || len(before.Updates) != 0 {
		t.Fatalf("before any check: %+v", before)
	}

	present := t.TempDir()
	var st installed.State
	st.Upsert(installed.Key{Registry: "http://127.0.0.1:1", Namespace: "acme", Name: "unreachable", Target: "claude-skill", Dir: present}, "1.0.0", installed.CapSnapshot{}, true)
	st.Upsert(installed.Key{Registry: newer, Namespace: "acme", Name: "gone", Target: "claude-skill", Dir: filepath.Join(t.TempDir(), "deleted")}, "1.0.0", installed.CapSnapshot{}, true)
	st.Upsert(installed.Key{Registry: newer, Namespace: "acme", Name: "skill", Target: "claude-skill", Dir: present}, "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	if _, err := i.CheckUpdates(); err != nil {
		t.Fatal(err)
	}
	after := i.UpdatesStatus()
	// Three rows; one counts: the present, manageable record with a newer
	// version. The gone directory's and the unreachable registry's do not.
	if after.Checked == "" || len(after.Updates) != 3 || after.Available != 1 {
		t.Fatalf("after the check: available=%d rows=%d %+v", after.Available, len(after.Updates), after.Updates)
	}
	for _, u := range after.Updates {
		switch u.Key.Name {
		case "unreachable":
			if u.Error == "" || u.UpdateAvailable {
				t.Fatalf("unreachable: %+v", u)
			}
		case "gone":
			if !u.UpdateAvailable || u.Present {
				t.Fatalf("gone: %+v", u)
			}
		case "skill":
			if !u.UpdateAvailable || u.Latest != "2.0.0" || !u.Manageable || !u.Present {
				t.Fatalf("skill: %+v", u)
			}
		}
	}

	// The page's own check for a BriefAgent with no runner record: nothing.
	info, err := i.CheckUpdate("acme", "skill")
	if err != nil || info != nil {
		t.Fatalf("no runner record: %+v %v", info, err)
	}
	// A runner record installed while the app runs (the CLI's install):
	// the page's check adds its row and the badge counts it.
	st, _ = installed.Load()
	if err := os.MkdirAll(installdir.AgentDir(home, "acme", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	st.Upsert(installdir.ConsentKey(home, "acme", "demo"), "1.0.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	info, err = i.CheckUpdate("acme", "demo")
	if err != nil || info == nil || !info.UpdateAvailable || info.Latest != "2.0.0" {
		t.Fatalf("runner record: %+v %v", info, err)
	}
	if v := i.UpdatesStatus(); len(v.Updates) != 4 || v.Available != 2 {
		t.Fatalf("snapshot after the page's check: available=%d rows=%d", v.Available, len(v.Updates))
	}
	// Asked again, the row is replaced, never duplicated.
	if _, err := i.CheckUpdate("acme", "demo"); err != nil {
		t.Fatal(err)
	}
	if v := i.UpdatesStatus(); len(v.Updates) != 4 || v.Available != 2 {
		t.Fatalf("a second check duplicated a row: rows=%d", len(v.Updates))
	}
	// A removal drops the row at once.
	if err := i.Forget(installed.Key{Registry: newer, Namespace: "acme", Name: "gone", Target: "claude-skill", Dir: filepath.Join(filepath.Dir(present), "nothing-here")}); err == nil {
		t.Fatal("Forget of an unknown record succeeded")
	}
	if _, err := i.Uninstall(installdir.ConsentKey(home, "acme", "demo")); err != nil {
		t.Fatal(err)
	}
	if v := i.UpdatesStatus(); len(v.Updates) != 3 || v.Available != 1 {
		t.Fatalf("snapshot after uninstall: available=%d rows=%d", v.Available, len(v.Updates))
	}
}
