package settings

import (
	"fmt"
	"os"
	"regexp"
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

// With no home key, Home is the default cards in their order, and
// nothing is written by reading it.
func TestHomeAbsentIsTheDefaults(t *testing.T) {
	isolate(t)
	l, state, err := homeLayoutState()
	if err != nil {
		t.Fatal(err)
	}
	if state != homeOK {
		t.Fatalf("an absent layout reads as %q", state)
	}
	if got, want := strings.Join(kinds(l), ","), "main,deliverables,recent-runs,installed,profile,run-defaults"; got != want {
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
	if got, want := strings.Join(kinds(l), ","), "main,deliverables,recent-runs,installed(hidden),profile,run-defaults"; got != want {
		t.Fatalf("after hiding = %s, want %s", got, want)
	}
	if err := SetHomeCardShown(HomeInstalled, true); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,deliverables,recent-runs,installed,profile,run-defaults"; got != want {
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
	writeRaw(t, "home:\n  version: 1\n  cards:\n    - kind: profile\n    - kind: deliverables\n")
	if err := SetHomeCardShown(HomeRunDefaults, true); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,profile,deliverables,run-defaults"; got != want {
		t.Fatalf("layout = %s, want %s", got, want)
	}
}

// Only the default cards are shown or hidden by kind: a pinned card is one
// of several and goes by its pins, and a kind this app does not know is
// refused rather than written.
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
// one arranged.
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
	// Main's card was put first when the layout was read, and saved with
	// the change.
	if cards[0].(map[string]any)["kind"] != HomeMain {
		t.Fatalf("Main's card was not put first: %v", cards[0])
	}
	cards = cards[1:]
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
	if got, want := strings.Join(kinds(l), ","), "main,deliverables,recent-runs,installed,profile,run-defaults"; got != want {
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
	if state != homeOK || len(l.Cards) != 6 {
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
// change to it is refused, and Reset clears it.
func TestHomeUnreadableNeverBreaksTheFile(t *testing.T) {
	for name, home := range map[string]string{
		"a number":           "home: 5\n",
		"a string version":   "home:\n  version: \"2\"\n",
		"cards as a map":     "home:\n  cards: {a: 1}\n",
		"a card as a scalar": "home:\n  cards: [x]\n",
		"an object size":     "home:\n  version: 1\n  cards:\n    - kind: installed\n      size: {w: 2}\n",
		// A Pinned card's pins, malformed three ways.
		"pins as a scalar":  "home:\n  version: 1\n  cards:\n    - kind: pinned\n      pins: x\n",
		"a pin as a scalar": "home:\n  version: 1\n  cards:\n    - kind: pinned\n      pins: [x]\n",
		"an object ref":     "home:\n  version: 1\n  cards:\n    - kind: pinned\n      pins:\n        - ref: {a: 1}\n",
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
			if state != homeUnreadable || len(l.Cards) != 6 {
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

// The Scheduled card: pinned from the Scheduled page, at most one, Large,
// after every other card, and never one of the 16 QuickAgent pins.
func TestPinScheduledAddsOneLargeCard(t *testing.T) {
	isolate(t)
	if scheduledShown(t) {
		t.Fatal("the defaults hold a Scheduled card")
	}
	for range 2 {
		if err := PinScheduledToHome(); err != nil {
			t.Fatal(err)
		}
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,deliverables,recent-runs,installed,profile,run-defaults,scheduled"; got != want {
		t.Fatalf("layout = %s, want %s", got, want)
	}
	if c := l.Cards[6]; c.Size != HomeLarge || c.Hidden || len(c.Pins) != 0 {
		t.Fatalf("the Scheduled card = %+v, want Large, shown, no pins", c)
	}
	if !scheduledShown(t) {
		t.Fatal("ScheduledOnHome after the pin")
	}
	// It is a card, not a pin: all 16 pins still fit.
	for i := range MaxPins {
		if err := PinToHome(fmt.Sprintf("acme/a%d", i), "https://r.example"); err != nil {
			t.Fatalf("pin %d with the Scheduled card on Home: %v", i, err)
		}
	}
}

// Settings → Home hides and shows it in its place like a default card;
// shown when it is not there, it is added at the end. Pin to Home on the
// page shows a hidden one again, and Unpin removes it.
func TestScheduledCardShownHiddenAndUnpinned(t *testing.T) {
	isolate(t)
	if err := PinScheduledToHome(); err != nil {
		t.Fatal(err)
	}
	if err := SetHomeCardShown(HomeScheduled, false); err != nil {
		t.Fatal(err)
	}
	if scheduledShown(t) {
		t.Fatal("a hidden Scheduled card reads as on Home")
	}
	if err := PinScheduledToHome(); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if n := len(l.Cards); n != 7 || l.Cards[6].Hidden {
		t.Fatalf("Pin did not show the hidden card in its place: %+v", l.Cards)
	}
	if err := UnpinScheduledFromHome(); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,deliverables,recent-runs,installed,profile,run-defaults"; got != want {
		t.Fatalf("after Unpin = %s, want %s", got, want)
	}
	if err := SetHomeCardShown(HomeScheduled, true); err != nil {
		t.Fatal(err)
	}
	if l, _, _ = homeLayoutState(); l.Cards[len(l.Cards)-1].Kind != HomeScheduled || l.Cards[len(l.Cards)-1].Size != HomeLarge {
		t.Fatalf("Settings' switch did not add it at the end, Large: %+v", l.Cards)
	}
	if err := ResetHome(); err != nil {
		t.Fatal(err)
	}
	if scheduledShown(t) {
		t.Fatal("Reset Home kept the Scheduled card")
	}
}

// A file that holds the Scheduled card stays a version 1 layout, so an
// older Agentry Desktop reads it as its own rather than as a newer
// arrangement: it keeps the card it does not know through its own saves
// (TestHomeKeepsWhatItDoesNotKnow) and its page skips the kind.
func TestScheduledCardKeepsTheLayoutReadableByAnOlderApp(t *testing.T) {
	isolate(t)
	if err := PinScheduledToHome(); err != nil {
		t.Fatal(err)
	}
	raw := readRaw(t)
	if !strings.Contains(raw, "version: 1\n") || !regexp.MustCompile(`- kind: scheduled\n\s+size: large\n`).MatchString(raw) {
		t.Fatalf("the file =\n%s", raw)
	}
	// What 0.6.81 did with it: hide a default card, pin a QuickAgent.
	if err := SetHomeCardShown(HomeProfile, false); err != nil {
		t.Fatal(err)
	}
	if err := PinToHome("acme/a", ""); err != nil {
		t.Fatal(err)
	}
	if !scheduledShown(t) {
		t.Fatal("another change dropped the Scheduled card")
	}
}

// Unpin of a layout this app keeps rather than uses is refused like any
// other change to it.
func TestScheduledCardLeavesANewerLayoutAlone(t *testing.T) {
	isolate(t)
	raw := "home:\n  version: 2\n  cards:\n    - kind: scheduled\n"
	writeRaw(t, raw)
	if scheduledShown(t) {
		t.Fatal("a newer layout's card reads as on Home")
	}
	if err := PinScheduledToHome(); err == nil {
		t.Fatal("Pin changed a newer layout")
	}
	if err := UnpinScheduledFromHome(); err == nil {
		t.Fatal("Unpin changed a newer layout")
	}
	if readRaw(t) != raw {
		t.Fatal("the newer layout was written over")
	}
}

// scheduledShown reports whether the stored layout draws the Scheduled
// card: one there and not hidden, in a layout this app uses.
func scheduledShown(t *testing.T) bool {
	t.Helper()
	l, st, err := homeLayoutState()
	if err != nil {
		t.Fatal(err)
	}
	if st != homeOK {
		return false
	}
	for _, c := range l.Cards {
		if c.Kind == HomeScheduled && !c.Hidden {
			return true
		}
	}
	return false
}

// A Home with no stored layout has the Deliverables card after Main's. One
// arranged before the card existed has it appended after every card, once:
// the order the person made never moves, hiding the card keeps its entry,
// and a save by anything else writes it down so it is not added again.
func TestDeliverablesCardIsFirstInTheDefaultsAndAppendedToAStoredLayout(t *testing.T) {
	isolate(t)
	l, _, _ := homeLayoutState()
	if l.Cards[1].Kind != HomeDeliverables {
		t.Fatalf("the defaults' second card is %s, want the Deliverables card", l.Cards[1].Kind)
	}

	writeRaw(t, "home:\n  version: 1\n  cards:\n    - kind: profile\n    - kind: pinned\n      size: large\n      pins:\n        - ref: acme/a\n    - kind: installed\n      hidden: true\n")
	l, state, _ := homeLayoutState()
	if state != homeOK {
		t.Fatalf("state = %q", state)
	}
	if got, want := strings.Join(kinds(l), ","), "main,profile,pinned,installed(hidden),deliverables"; got != want {
		t.Fatalf("an arranged Home reads as %s, want %s", got, want)
	}
	if strings.Contains(readRaw(t), "deliverables") || strings.Contains(readRaw(t), "main") {
		t.Fatal("reading the layout wrote a card")
	}

	// A change to something else saves the card where it was appended.
	if err := SetHomeCardShown(HomeProfile, false); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,profile(hidden),pinned,installed(hidden),deliverables"; got != want {
		t.Fatalf("after a change: %s, want %s", got, want)
	}
	// Hiding it keeps its entry and its place; showing it puts it back.
	if err := SetHomeCardShown(HomeDeliverables, false); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,profile(hidden),pinned,installed(hidden),deliverables(hidden)"; got != want {
		t.Fatalf("after hiding: %s, want %s", got, want)
	}
	if n := strings.Count(readRaw(t), "kind: deliverables"); n != 1 {
		t.Fatalf("the card is stored %d times", n)
	}
	if err := SetHomeCardShown(HomeDeliverables, true); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,profile(hidden),pinned,installed(hidden),deliverables"; got != want {
		t.Fatalf("after showing: %s, want %s", got, want)
	}
}

// Main's card is first in a Home with no stored layout, and put first, once,
// in one arranged before the card existed: hiding it keeps its entry, so it
// is never put back, and showing it again leaves it where it is.
func TestMainCardIsFirstAndPrependedOnce(t *testing.T) {
	isolate(t)
	l, _, _ := homeLayoutState()
	if l.Cards[0].Kind != HomeMain {
		t.Fatalf("the defaults start with %s, want Main's card", l.Cards[0].Kind)
	}
	writeRaw(t, "home:\n  version: 1\n  cards:\n    - kind: installed\n    - kind: deliverables\n")
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,installed,deliverables"; got != want {
		t.Fatalf("an arranged Home reads as %s, want %s", got, want)
	}
	if err := SetHomeCardShown(HomeMain, false); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main(hidden),installed,deliverables"; got != want {
		t.Fatalf("after hiding: %s, want %s", got, want)
	}
	if n := strings.Count(readRaw(t), "kind: main"); n != 1 {
		t.Fatalf("Main's card is stored %d times", n)
	}
	if err := SetHomeCardShown(HomeMain, true); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,installed,deliverables"; got != want {
		t.Fatalf("after showing: %s, want %s", got, want)
	}
}

// A new pin on an arranged Home opens its Pinned card after the Deliverables
// card that was appended, and no card moves.
func TestAPinOnAnArrangedHomeComesAfterTheAppendedCard(t *testing.T) {
	isolate(t)
	writeRaw(t, "home:\n  version: 1\n  cards:\n    - kind: run-defaults\n    - kind: recent-runs\n")
	if err := PinToHome("acme/a", "https://r.example"); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "main,run-defaults,recent-runs,deliverables,pinned"; got != want {
		t.Fatalf("layout = %s, want %s", got, want)
	}
}

// The Deliverables switch defaults on, and off only when the file says so.
func TestDeliverablesDefaultOn(t *testing.T) {
	isolate(t)
	s, _ := Load()
	if !s.DeliverablesEnabled() {
		t.Fatal("deliverables are off in a fresh install")
	}
	off := false
	s.Deliverables = &off
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	s, _ = Load()
	if s.DeliverablesEnabled() {
		t.Fatal("a saved off switch reads as on")
	}
}

func sizes(l HomeLayout) []string {
	out := make([]string, 0, len(l.Cards))
	for _, c := range l.Cards {
		out = append(out, fmt.Sprintf("%s:%dx%d", c.Kind, c.Cols, c.Rows))
	}
	return out
}

// A move puts the card at its new place and keeps the others' order, hidden
// cards included; a pinned card moves with its pins.
func TestHomeMoveKeepsTheOthersInOrder(t *testing.T) {
	isolate(t)
	if err := SetHomeCardShown(HomeInstalled, false); err != nil {
		t.Fatal(err)
	}
	if err := PinToHome("acme/one", "https://dev.example"); err != nil {
		t.Fatal(err)
	}
	// main,deliverables,recent-runs,installed(hidden),profile,run-defaults,pinned
	if err := MoveHomeCard(6, HomePinned, 0); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "pinned,main,deliverables,recent-runs,installed(hidden),profile,run-defaults"; got != want {
		t.Fatalf("after moving the pinned card first = %s, want %s", got, want)
	}
	if len(l.Cards[0].Pins) != 1 || l.Cards[0].Pins[0].Ref != "acme/one" {
		t.Fatalf("the pinned card lost its pin: %+v", l.Cards[0])
	}
	if err := MoveHomeCard(1, HomeMain, 5); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	if got, want := strings.Join(kinds(l), ","), "pinned,deliverables,recent-runs,installed(hidden),profile,main,run-defaults"; got != want {
		t.Fatalf("after moving main = %s, want %s", got, want)
	}
}

// A move or a size names the card it means: a card of another kind at that
// place, or a place Home does not have, is refused and nothing is written.
func TestHomeMoveAndSizeRefuseAStaleOrMissingCard(t *testing.T) {
	isolate(t)
	if err := MoveHomeCard(0, HomeProfile, 3); err == nil {
		t.Fatal("a move naming the wrong kind was accepted")
	}
	if err := MoveHomeCard(0, HomeMain, 6); err == nil {
		t.Fatal("a move past the last place was accepted")
	}
	if err := MoveHomeCard(-1, HomeMain, 0); err == nil {
		t.Fatal("a move from before the first card was accepted")
	}
	if err := SizeHomeCard(9, HomeMain, 2, 2); err == nil {
		t.Fatal("a size of a card Home lacks was accepted")
	}
	if path, _ := Path(); fileExists(path) {
		t.Fatal("a refused change wrote the settings file")
	}
}

// A card is 1 to 4 columns wide and 1 to 3 rows tall; its size is kept
// through a move, and Reset puts the default sizes back.
func TestHomeSizeIsBoundedKeptAndReset(t *testing.T) {
	isolate(t)
	for _, bad := range [][2]int{{0, 1}, {5, 1}, {1, 0}, {1, 4}} {
		if err := SizeHomeCard(0, HomeMain, bad[0], bad[1]); err == nil {
			t.Fatalf("a %dx%d card was accepted", bad[0], bad[1])
		}
	}
	if err := SizeHomeCard(2, HomeRecentRuns, 4, 3); err != nil {
		t.Fatal(err)
	}
	if err := MoveHomeCard(2, HomeRecentRuns, 0); err != nil {
		t.Fatal(err)
	}
	l, _, _ := homeLayoutState()
	if got, want := strings.Join(sizes(l)[:2], ","), "recent-runs:4x3,main:0x0"; got != want {
		t.Fatalf("after size and move = %s, want %s", got, want)
	}
	if !strings.Contains(readRaw(t), "cols: 4") || !strings.Contains(readRaw(t), "rows: 3") {
		t.Fatalf("the size was not written:\n%s", readRaw(t))
	}
	if err := ResetHome(); err != nil {
		t.Fatal(err)
	}
	l, _, _ = homeLayoutState()
	for _, c := range l.Cards {
		if c.Cols != 0 || c.Rows != 0 {
			t.Fatalf("Reset kept a size: %+v", c)
		}
	}
}

// A layout this app keeps rather than uses takes no move and no size.
func TestHomeMoveAndSizeLeaveANewerLayoutAlone(t *testing.T) {
	isolate(t)
	raw := "home:\n  version: 2\n  cards:\n    - kind: main\n    - kind: installed\n"
	writeRaw(t, raw)
	if err := MoveHomeCard(1, HomeDeliverables, 0); err == nil {
		t.Fatal("a move on a newer layout was accepted")
	}
	if err := SizeHomeCard(0, HomeMain, 2, 2); err == nil {
		t.Fatal("a size on a newer layout was accepted")
	}
	if readRaw(t) != raw {
		t.Fatalf("the newer layout was rewritten:\n%s", readRaw(t))
	}
}

// A sized layout still reads in an app that knows no sizes: its cards'
// order is the list's, and the unknown keys are what an older app keeps.
func TestHomeSizeIsPlainKeysAnOlderAppKeeps(t *testing.T) {
	isolate(t)
	if err := SizeHomeCard(0, HomeMain, 3, 2); err != nil {
		t.Fatal(err)
	}
	var older struct {
		Home struct {
			Version int `yaml:"version"`
			Cards   []struct {
				Kind string         `yaml:"kind"`
				Rest map[string]any `yaml:",inline"`
			} `yaml:"cards"`
		} `yaml:"home"`
	}
	if err := yaml.Unmarshal([]byte(readRaw(t)), &older); err != nil {
		t.Fatal(err)
	}
	if older.Home.Version != HomeVersion || older.Home.Cards[0].Kind != HomeMain {
		t.Fatalf("an older reader sees %+v", older.Home)
	}
	if older.Home.Cards[0].Rest["cols"] != 3 || older.Home.Cards[0].Rest["rows"] != 2 {
		t.Fatalf("the size is not plain keys: %+v", older.Home.Cards[0].Rest)
	}
}
