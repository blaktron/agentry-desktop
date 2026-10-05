package settings

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func kinds(l HomeLayout) []string {
	out := make([]string, 0, len(l.Cards))
	for _, c := range l.Cards {
		k := c.Kind
		if c.Hidden {
			k += "(hidden)"
		}
		out = append(out, k)
	}
	return out
}

func writeRaw(t *testing.T, raw string) {
	t.Helper()
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(strings.TrimSuffix(path, "desktop.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readRaw(t *testing.T) string {
	t.Helper()
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path) //nolint:gosec // the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// With no home key, Home is the four default cards in their order, and
// nothing is written by reading it (plan desktop-home-and-library.md §4.4).
func TestHomeAbsentIsTheDefaults(t *testing.T) {
	isolate(t)
	l, state, err := homeLayoutState()
	if err != nil {
		t.Fatal(err)
	}
	if state != homeOK {
		t.Fatalf("an absent layout reads as %q", state)
	}
	if got, want := strings.Join(kinds(l), ","), "recent-runs,installed,profile,run-defaults"; got != want {
		t.Fatalf("defaults = %s, want %s", got, want)
	}
	if l.Version != HomeVersion {
		t.Fatalf("version = %d, want %d", l.Version, HomeVersion)
	}
	if path, _ := Path(); fileExists(path) {
		t.Fatal("reading the layout wrote the settings file")
	}
}

// Hiding a card keeps its place in the order, and showing it puts it back
// where it was; every other setting in the file is left as it was.
func TestHomeSetShownKeepsThePlace(t *testing.T) {
	isolate(t)
	writeRaw(t, "defaultRegistry: https://dev.example\nonboarded: true\n")
	if err := SetHomeCardShown(HomeInstalled, false); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "recent-runs,installed(hidden),profile,run-defaults"; got != want {
		t.Fatalf("after hiding = %s, want %s", got, want)
	}
	if err := SetHomeCardShown(HomeInstalled, true); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "recent-runs,installed,profile,run-defaults"; got != want {
		t.Fatalf("after showing = %s, want %s", got, want)
	}
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultRegistry != "https://dev.example" || !s.Onboarded {
		t.Fatalf("the other settings moved: %+v", s)
	}
}

// A default card a stored layout lacks (written before the card existed) is
// added at the end when it is shown, never refused.
func TestHomeSetShownAddsAMissingDefault(t *testing.T) {
	isolate(t)
	writeRaw(t, "home:\n  version: 1\n  cards:\n    - kind: profile\n")
	if err := SetHomeCardShown(HomeRunDefaults, true); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "profile,run-defaults"; got != want {
		t.Fatalf("layout = %s, want %s", got, want)
	}
}

// Only the default cards are shown or hidden by kind: a pinned card is one
// of several and goes by its pins (M4), and a kind this app does not know
// is refused rather than written.
func TestHomeSetShownRefusesOtherKinds(t *testing.T) {
	isolate(t)
	for _, k := range []string{HomePinned, "credits-chart", ""} {
		if err := SetHomeCardShown(k, false); err == nil {
			t.Fatalf("SetHomeCardShown(%q) was accepted", k)
		}
	}
	if path, _ := Path(); fileExists(path) {
		t.Fatal("a refused change wrote the settings file")
	}
}

// A card of a kind this app does not know, and fields it does not know, are
// kept through a save it makes, so an older app never destroys what a newer
// one arranged (plan §4.4).
func TestHomeKeepsWhatItDoesNotKnow(t *testing.T) {
	isolate(t)
	writeRaw(t, `home:
  version: 1
  shape: grid
  cards:
    - kind: credits-chart
      size: small
      options:
        months: 3
    - kind: installed
      accent: violet
    - kind: profile
`)
	if err := SetHomeCardShown(HomeProfile, false); err != nil {
		t.Fatal(err)
	}
	var back struct {
		Home map[string]any `yaml:"home"`
	}
	if err := yaml.Unmarshal([]byte(readRaw(t)), &back); err != nil {
		t.Fatal(err)
	}
	if back.Home["shape"] != "grid" {
		t.Fatalf("the layout's own unknown field was dropped: %v", back.Home)
	}
	cards := back.Home["cards"].([]any)
	first := cards[0].(map[string]any)
	if first["kind"] != "credits-chart" || first["size"] != "small" || first["options"].(map[string]any)["months"] != 3 {
		t.Fatalf("the unknown card was not kept whole: %v", first)
	}
	if cards[1].(map[string]any)["accent"] != "violet" {
		t.Fatalf("a known card's unknown field was dropped: %v", cards[1])
	}
	if cards[2].(map[string]any)["hidden"] != true {
		t.Fatalf("the change asked for was not made: %v", cards[2])
	}
}

// A layout of a newer version is read as the defaults, flagged, and never
// written over: a change to it is refused, and the file keeps it.
func TestHomeNewerVersionIsKept(t *testing.T) {
	isolate(t)
	raw := "home:\n  version: 2\n  cards:\n    - kind: installed\n      size: tiny\n"
	writeRaw(t, raw)
	l, state, err := homeLayoutState()
	if err != nil {
		t.Fatal(err)
	}
	if state != homeNewer {
		t.Fatalf("a version 2 layout reads as %q, not newer", state)
	}
	if got, want := strings.Join(kinds(l), ","), "recent-runs,installed,profile,run-defaults"; got != want {
		t.Fatalf("a newer layout reads as %s, want the defaults %s", got, want)
	}
	if err := SetHomeCardShown(HomeInstalled, false); err == nil {
		t.Fatal("a change to a newer layout was accepted")
	}
	// Another setting's save keeps the newer layout as it was.
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Onboarded = true
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readRaw(t), "size: tiny") || !strings.Contains(readRaw(t), "version: 2") {
		t.Fatalf("another save rewrote the newer layout:\n%s", readRaw(t))
	}
}

// Reset removes the layout, so Home is the defaults again, and touches no
// other setting.
func TestHomeReset(t *testing.T) {
	isolate(t)
	writeRaw(t, "onboarded: true\nhome:\n  version: 2\n  cards:\n    - kind: installed\n")
	if err := ResetHome(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readRaw(t), "home:") {
		t.Fatalf("Reset left a layout:\n%s", readRaw(t))
	}
	l, state, _ := homeLayoutState()
	if state != homeOK || len(l.Cards) != 4 {
		t.Fatalf("after Reset: state=%q cards=%v", state, kinds(l))
	}
	if s, _ := Load(); !s.Onboarded {
		t.Fatal("Reset moved another setting")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// A home value this app cannot decode, of whatever shape, never makes the
// settings file unreadable: every other setting loads, Home is the defaults
// and says so, another setting's save writes the value back as it was, a
// change to it is refused, and Reset clears it (the review of
// agentry-desktop#658).
func TestHomeUnreadableNeverBreaksTheFile(t *testing.T) {
	for name, home := range map[string]string{
		"a number":           "home: 5\n",
		"a string version":   "home:\n  version: \"2\"\n",
		"cards as a map":     "home:\n  cards: {a: 1}\n",
		"a card as a scalar": "home:\n  cards: [x]\n",
		"an object size":     "home:\n  version: 1\n  cards:\n    - kind: installed\n      size: {w: 2}\n",
	} {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			writeRaw(t, "onboarded: true\n"+home)
			s, err := Load()
			if err != nil {
				t.Fatalf("Load failed on %s: %v", name, err)
			}
			if !s.Onboarded {
				t.Fatal("another setting was lost")
			}
			l, state, err := homeLayoutState()
			if err != nil {
				t.Fatal(err)
			}
			if state != homeUnreadable || len(l.Cards) != 4 {
				t.Fatalf("state=%q cards=%v, want unreadable and the defaults", state, kinds(l))
			}
			s.DefaultRegistry = "https://dev.example"
			if err := Save(s); err != nil {
				t.Fatal(err)
			}
			var before, after struct {
				Home any `yaml:"home"`
			}
			_ = yaml.Unmarshal([]byte("onboarded: true\n"+home), &before)
			_ = yaml.Unmarshal([]byte(readRaw(t)), &after)
			if !equalYAML(t, before.Home, after.Home) {
				t.Fatalf("the unreadable value was not written back as it was:\n%s", readRaw(t))
			}
			if err := SetHomeCardShown(HomeInstalled, false); err == nil {
				t.Fatal("a change to an unreadable layout was accepted")
			}
			if err := ResetHome(); err != nil {
				t.Fatal(err)
			}
			if _, state, _ := homeLayoutState(); state != homeOK {
				t.Fatalf("after Reset the state is %q", state)
			}
		})
	}
}

func equalYAML(t *testing.T, a, b any) bool {
	t.Helper()
	x, err1 := yaml.Marshal(a)
	y, err2 := yaml.Marshal(b)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	return string(x) == string(y)
}
