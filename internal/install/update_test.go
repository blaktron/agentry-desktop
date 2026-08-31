package install

// Update-check tests: per-record errors are data (an unreachable registry
// for one install must not hide another's update), semver ordering with a
// string-inequality fallback, and the Prepare-dir reconstruction that
// makes "Update" land on the original install key.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/installed"
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
