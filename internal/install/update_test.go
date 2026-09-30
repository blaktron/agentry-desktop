package install

// Update-check tests: per-record errors are data (an unreachable registry
// for one install must not hide another's update), semver ordering with a
// string-inequality fallback, and the Prepare-dir reconstruction that
// makes "Update" land on the original install key.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestNewerThan(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"2.0.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "2.0.0", false},
		// Unparsable versions fall back to string inequality: "different"
		// must surface, never "silently up to date".
		{"weird", "weird", false},
		{"weird", "1.0.0", true},
		{"1.0.0", "weird", true},
	}
	for _, c := range cases {
		if got := newerThan(c.latest, c.current); got != c.want {
			t.Errorf("newerThan(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestDirForRecord(t *testing.T) {
	if d, err := DirForRecord(installed.Record{Target: "claude-skill", Dir: "/home/u/.claude/skills/demo", Managed: true}); err != nil || d != "" {
		t.Errorf("managed claude-skill dir = %q %v, want the default layout", d, err)
	}
	if d, err := DirForRecord(installed.Record{Target: "claude-skill", Dir: "/picked/demo"}); err != nil || d != "/picked" {
		t.Errorf("unmanaged claude-skill dir = %q %v", d, err)
	}
	for _, target := range []string{"mcp", "agents-md"} {
		if d, err := DirForRecord(installed.Record{Target: target, Dir: "/proj/agents/demo", Managed: true}); err != nil || d != "/proj" {
			t.Errorf("%s dir = %q %v, want the project root", target, d, err)
		}
	}
	if _, err := DirForRecord(installed.Record{Target: "bogus"}); err == nil {
		t.Error("unknown target must refuse")
	}
}

// TestCheckUpdates: one record with an update, one already current, one
// whose registry is unreachable — three result rows, errors carried as
// per-record data.
func TestCheckUpdates(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{version: "2.0.0"})
	reg = strings.TrimRight(reg, "/")

	var st installed.State
	st.Upsert(installed.Key{Registry: reg, Namespace: "acme", Name: "demo", Target: "claude-skill", Dir: "/d1"},
		"1.0.0", installed.CapSnapshot{}, false)
	st.Upsert(installed.Key{Registry: reg, Namespace: "acme", Name: "demo", Target: "mcp", Dir: "/d2"},
		"2.0.0", installed.CapSnapshot{}, false)
	st.Upsert(installed.Key{Registry: "http://127.0.0.1:1", Namespace: "acme", Name: "unreachable", Target: "claude-skill", Dir: "/d3"},
		"1.0.0", installed.CapSnapshot{}, false)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}

	infos, err := CheckUpdates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 {
		t.Fatalf("rows = %d: %+v", len(infos), infos)
	}
	byDir := map[string]UpdateInfo{}
	for _, i := range infos {
		byDir[filepath.ToSlash(i.Key.Dir)] = i
	}
	if u := byDir["/d1"]; !u.UpdateAvailable || u.Latest != "2.0.0" || u.Error != "" {
		t.Errorf("update row = %+v", u)
	}
	if u := byDir["/d2"]; u.UpdateAvailable || u.Latest != "2.0.0" || u.Error != "" {
		t.Errorf("current row = %+v", u)
	}
	if u := byDir["/d3"]; u.Error == "" || u.UpdateAvailable || u.Latest != "" {
		t.Errorf("unreachable row must carry its error: %+v", u)
	}
}

// TestCheckUpdateForTheRunnerRecord (agentry-desktop#147): opening a
// QuickAgent's page asks about its runner-home record alone — through the
// record's origin, the app's default registry — and answers with the one
// "newer" rule (semver); no record, no question.
func TestCheckUpdateForTheRunnerRecord(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	reg := startRegistry(t, &fakeRegistry{version: "2.0.0"})
	if err := settings.Save(settings.Settings{DefaultRegistry: reg, Registries: []settings.Registry{{URL: reg}}}); err != nil {
		t.Fatal(err)
	}

	info, err := CheckUpdateFor(context.Background(), "acme", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatalf("no record, yet an answer: %+v", info)
	}
	var st installed.State
	st.Upsert(installdir.ConsentKey(home, "acme", "demo"), "1.0.0", installed.CapSnapshot{}, true)
	st.Upsert(installed.Key{Registry: reg, Namespace: "acme", Name: "demo", Target: "claude-skill", Dir: "/d1"}, "0.1.0", installed.CapSnapshot{}, true)
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}
	info, err = CheckUpdateFor(context.Background(), "acme", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || !info.UpdateAvailable || info.Latest != "2.0.0" || info.Current != "1.0.0" || info.Key.Target != installdir.Target || info.Error != "" {
		t.Fatalf("runner record's check: %+v", info)
	}
	// A registry that cannot be reached is the answer's error, not a failure.
	if err := settings.Save(settings.Settings{DefaultRegistry: "http://127.0.0.1:1", Registries: []settings.Registry{{URL: "http://127.0.0.1:1"}}}); err != nil {
		t.Fatal(err)
	}
	info, err = CheckUpdateFor(context.Background(), "acme", "demo")
	if err != nil || info == nil || info.Error == "" || info.UpdateAvailable {
		t.Fatalf("unreachable: %+v err %v", info, err)
	}
}

// TestALocalInstallAsksNoRegistry (agentry-desktop#425): a QuickAgent
// installed from a folder lists as local with no origin, and its update
// check sends nothing anywhere, the default registry included.
func TestALocalInstallAsksNoRegistry(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	reg := srv.URL
	if err := settings.Save(settings.Settings{DefaultRegistry: reg, Registries: []settings.Registry{{URL: reg}}}); err != nil {
		t.Fatal(err)
	}
	var st installed.State
	key := installdir.ConsentKey(home, "acme", "demo")
	st.Upsert(key, "1.0.0", installed.CapSnapshot{}, true)
	st.MarkLocal(key, "sha256:"+strings.Repeat("a", 64))
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}

	items, err := List()
	if err != nil || len(items) != 1 {
		t.Fatalf("List = %+v, %v", items, err)
	}
	if !items[0].Local || items[0].Origin != "" {
		t.Errorf("local item = %+v, want local with no origin", items[0])
	}
	info, err := CheckUpdateFor(context.Background(), "acme", "demo")
	if err != nil || info == nil || info.UpdateAvailable || info.Latest != "" || info.Error != "" || info.Current != "1.0.0" {
		t.Fatalf("local record's check = %+v, %v", info, err)
	}
	if _, err := CheckUpdates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the registry served %d requests for a local install", n)
	}
}
