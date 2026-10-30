package app

import (
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/highlights"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func whatsNewEntries(versions ...string) func() ([]highlights.Entry, error) {
	return func() ([]highlights.Entry, error) {
		var es []highlights.Entry
		for _, v := range versions {
			es = append(es, highlights.Entry{Version: v, Highlights: []string{"What " + v + " brings."}})
		}
		return es, nil
	}
}

func cardVersions(v *WhatsNewView) string {
	var s []string
	for _, c := range v.Cards {
		s = append(s, c.Version)
	}
	return strings.Join(s, " ")
}

// seenFile writes desktop.yaml with the last-seen version, or with none.
func seenFile(t *testing.T, seen string) {
	t.Helper()
	if err := settings.Save(settings.Settings{DefaultRegistry: "https://example.test", WhatsNewSeen: seen}); err != nil {
		t.Fatal(err)
	}
}

func settingsFile(t *testing.T) string {
	t.Helper()
	path, err := settings.Path()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "absent"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestWhatsNewOnStart(t *testing.T) {
	all := whatsNewEntries("0.6.20", "0.6.19", "0.6.18", "0.6.17", "0.6.16", "0.6.15", "0.6.14", "0.6.13")
	cases := []struct {
		name     string
		file     bool
		seen     string
		running  string
		cards    string
		earlier  int
		recorded string
	}{
		{name: "a fresh install shows nothing and records the version", running: "0.6.20", recorded: "0.6.20"},
		{name: "a version that kept no last-seen version shows its own card", file: true, running: "0.6.18-dev.2", cards: "0.6.18", recorded: ""},
		{name: "one version on", file: true, seen: "0.6.19", running: "0.6.20", cards: "0.6.20", recorded: "0.6.19"},
		{name: "several versions on", file: true, seen: "0.6.16", running: "0.6.20", cards: "0.6.20 0.6.19 0.6.18 0.6.17", recorded: "0.6.16"},
		{name: "more than five on", file: true, seen: "0.6.12", running: "0.6.20", cards: "0.6.20 0.6.19 0.6.18 0.6.17 0.6.16", earlier: 3, recorded: "0.6.12"},
		{name: "the same version shows nothing", file: true, seen: "0.6.20", running: "0.6.20-dev.3", recorded: "0.6.20"},
		{name: "an older version than the one seen shows nothing and records it", file: true, seen: "0.6.20", running: "0.6.18", recorded: "0.6.18"},
		{name: "a version with no entry shows nothing and records it", file: true, seen: "0.6.20", running: "0.6.21", recorded: "0.6.21"},
		{name: "a build that names no release shows and records nothing", file: true, seen: "0.6.20", running: "0.0.0-dev", recorded: "0.6.20"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AGENTRY_CONFIG_DIR", t.TempDir())
			if c.file {
				seenFile(t, c.seen)
			}
			w := newWhatsNewAPI()
			w.version, w.entries = c.running, all
			v, err := w.Pending()
			if err != nil {
				t.Fatal(err)
			}
			if got := cardVersions(v); got != c.cards || v.Earlier != c.earlier {
				t.Fatalf("cards %q +%d, want %q +%d", got, v.Earlier, c.cards, c.earlier)
			}
			if v.ReleasesURL != desktopReleasesURL {
				t.Fatalf("releases link %q", v.ReleasesURL)
			}
			st, err := settings.Load()
			if err != nil {
				t.Fatal(err)
			}
			if st.WhatsNewSeen != c.recorded {
				t.Fatalf("recorded %q, want %q", st.WhatsNewSeen, c.recorded)
			}
		})
	}
}

// The cards the screen shows stay until the person closes them: quitting
// with the screen open shows them again on the next start.
func TestWhatsNewIsRecordedOnlyWhenClosed(t *testing.T) {
	t.Setenv("AGENTRY_CONFIG_DIR", t.TempDir())
	seenFile(t, "0.6.18")
	before := settingsFile(t)
	w := newWhatsNewAPI()
	w.version, w.entries = "0.6.20-dev.1", whatsNewEntries("0.6.20", "0.6.19", "0.6.18")
	for range 2 {
		v, err := w.Pending()
		if err != nil {
			t.Fatal(err)
		}
		if cardVersions(v) != "0.6.20 0.6.19" {
			t.Fatalf("cards %q", cardVersions(v))
		}
		if settingsFile(t) != before {
			t.Fatal("showing the cards wrote desktop.yaml")
		}
	}
	if _, err := w.Recent(); err != nil {
		t.Fatal(err)
	}
	if settingsFile(t) != before {
		t.Fatal("Settings' What's new wrote desktop.yaml")
	}
	if err := w.Dismiss(); err != nil {
		t.Fatal(err)
	}
	if v, err := w.Pending(); err != nil || len(v.Cards) != 0 {
		t.Fatalf("after Dismiss: %+v, %v", v, err)
	}
	st, _ := settings.Load()
	if st.WhatsNewSeen != "0.6.20" {
		t.Fatalf("Dismiss recorded %q", st.WhatsNewSeen)
	}
	if st.DefaultRegistry != "https://example.test" {
		t.Fatal("Dismiss changed another setting")
	}
}

func TestSettingsWhatsNewShowsTheNewestUpToThisVersion(t *testing.T) {
	t.Setenv("AGENTRY_CONFIG_DIR", t.TempDir())
	w := newWhatsNewAPI()
	w.version, w.entries = "0.6.19", whatsNewEntries("0.6.20", "0.6.19", "0.6.18", "0.6.17", "0.6.16", "0.6.15", "0.6.14")
	v, err := w.Recent()
	if err != nil {
		t.Fatal(err)
	}
	if cardVersions(v) != "0.6.19 0.6.18 0.6.17 0.6.16 0.6.15" || v.Earlier != 1 || v.Current != "0.6.19" {
		t.Fatalf("%+v", v)
	}
	if settingsFile(t) != "absent" {
		t.Fatal("Recent wrote desktop.yaml")
	}
}
