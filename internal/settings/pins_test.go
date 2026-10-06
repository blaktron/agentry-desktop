package settings

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const pinReg = "https://dev.secureagentry.net"

// pinCards is each Pinned card's pins, in the layout's order, as
// "a,b|c" (a card per |), the layout re-read from the file.
func pinCards(t *testing.T) string {
	t.Helper()
	l, _, _, err := HomeLayoutNow()
	if err != nil {
		t.Fatal(err)
	}
	var cards []string
	for _, c := range l.Cards {
		if c.Kind != HomePinned {
			continue
		}
		var refs []string
		for _, p := range c.Pins {
			refs = append(refs, strings.TrimPrefix(p.Ref, "acme/"))
		}
		cards = append(cards, strings.Join(refs, ","))
	}
	return strings.Join(cards, "|")
}

func pin(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := PinToHome("acme/"+n, pinReg); err != nil {
			t.Fatalf("pin %s: %v", n, err)
		}
	}
}

// The first pin opens a Pinned card after the default cards, Large, and a
// pin already there is not pinned twice (plan desktop-home-and-library.md
// D5, D11, D12).
func TestAFirstPinOpensAPinnedCardAfterTheDefaults(t *testing.T) {
	isolate(t)
	pin(t, "a", "a")
	l, _, _, err := HomeLayoutNow()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(kinds(l), ","); got != "recent-runs,installed,profile,run-defaults,pinned" {
		t.Fatalf("cards = %s", got)
	}
	last := l.Cards[len(l.Cards)-1]
	if last.Size != "large" || len(last.Pins) != 1 || last.Pins[0].Ref != "acme/a" || last.Pins[0].Registry != pinReg {
		t.Fatalf("the Pinned card = %+v", last)
	}
	if !IsPinned("acme/a", pinReg+"/") || IsPinned("acme/a", "https://other.example") || IsPinned("acme/b", pinReg) {
		t.Fatal("IsPinned does not hold to the ref and the registry")
	}
}

// Four pins share a card; the fifth opens a second (D5, D11).
func TestTheFifthPinOpensASecondCard(t *testing.T) {
	isolate(t)
	pin(t, "a", "b", "c", "d")
	if got := pinCards(t); got != "a,b,c,d" {
		t.Fatalf("four pins: %s", got)
	}
	pin(t, "e")
	if got := pinCards(t); got != "a,b,c,d|e" {
		t.Fatalf("five pins: %s", got)
	}
}

// Home holds 16 pins: the 17th is refused with a sentence saying so, and
// the layout is left as it was (D5).
func TestTheSeventeenthPinIsRefused(t *testing.T) {
	isolate(t)
	for i := range MaxPins {
		pin(t, fmt.Sprintf("q%02d", i))
	}
	before := readRaw(t)
	err := PinToHome("acme/q16", pinReg)
	if !errors.Is(err, ErrHomeFull) || !strings.Contains(err.Error(), "16") {
		t.Fatalf("the 17th pin: %v", err)
	}
	if readRaw(t) != before {
		t.Fatal("a refused pin changed the settings file")
	}
	if n := strings.Count(pinCards(t), "|") + 1; n != MaxPinnedCards {
		t.Fatalf("%d Pinned cards", n)
	}
}

// Unpinning leaves its gap and moves no other pin; the next pin fills the
// first card with room; an emptied card goes (D11).
func TestUnpinLeavesTheGapTheNextPinFillsIt(t *testing.T) {
	isolate(t)
	pin(t, "a", "b", "c", "d", "e", "f")
	if err := UnpinFromHome("acme/b", pinReg); err != nil {
		t.Fatal(err)
	}
	if got := pinCards(t); got != "a,c,d|e,f" {
		t.Fatalf("after unpinning b: %s", got)
	}
	pin(t, "g")
	if got := pinCards(t); got != "a,c,d,g|e,f" {
		t.Fatalf("g did not fill the first card's gap: %s", got)
	}
	for _, n := range []string{"e", "f"} {
		if err := UnpinFromHome("acme/"+n, pinReg); err != nil {
			t.Fatal(err)
		}
	}
	if got := pinCards(t); got != "a,c,d,g" {
		t.Fatalf("the emptied card stayed: %s", got)
	}
	if err := UnpinFromHome("acme/never", pinReg); err != nil {
		t.Fatalf("unpinning what is not pinned: %v", err)
	}
}

// UnpinRef removes a ref's pins from every registry, for an uninstall,
// or its folder install's pin alone.
func TestUnpinRefRemovesEveryRegistrysPinOrTheFolders(t *testing.T) {
	isolate(t)
	pin(t, "a", "b")
	for _, reg := range []string{"https://other.example", ""} {
		if err := PinToHome("acme/a", reg); err != nil {
			t.Fatal(err)
		}
	}
	if err := UnpinRef("acme/a", false); err != nil {
		t.Fatal(err)
	}
	if got := pinCards(t); got != "b,a" || !IsPinned("acme/a", "") {
		t.Fatalf("after UnpinRef from registries: %s", got)
	}
	if err := UnpinRef("acme/a", true); err != nil {
		t.Fatal(err)
	}
	if got := pinCards(t); got != "b" {
		t.Fatalf("after UnpinRef from a folder: %s", got)
	}
}

// A pin's ref is namespace/name, nothing more; a folder install pins with
// no registry.
func TestAPinsRefIsNamespaceAndName(t *testing.T) {
	isolate(t)
	for _, bad := range []string{"", "acme", "acme/", "/a", "acme/a@1.0.0", "acme/a/b", "acme /a"} {
		if err := PinToHome(bad, pinReg); err == nil {
			t.Fatalf("%q was pinned", bad)
		}
	}
	if err := PinToHome("acme/local", ""); err != nil {
		t.Fatalf("a folder install's pin: %v", err)
	}
	if !IsPinned("acme/local", "") {
		t.Fatal("the folder install's pin is not read back")
	}
}

// A pin's unknown keys, and a Pinned card's, survive a pin elsewhere; a
// newer layout takes no pin (D-067).
func TestPinsKeepWhatTheyDoNotKnowAndANewerLayoutTakesNone(t *testing.T) {
	isolate(t)
	writeRaw(t, `home:
    version: 1
    cards:
        - kind: pinned
          size: large
          color: teal
          pins:
            - ref: acme/a
              registry: `+pinReg+`
              note: kept
`)
	pin(t, "b")
	raw := readRaw(t)
	if !strings.Contains(raw, "color: teal") || !strings.Contains(raw, "note: kept") {
		t.Fatalf("unknown keys lost:\n%s", raw)
	}
	if got := pinCards(t); got != "a,b" {
		t.Fatalf("pins: %s", got)
	}
	writeRaw(t, "home:\n    version: 2\n    cards: []\n")
	if err := PinToHome("acme/c", pinReg); !errors.Is(err, errNewerHome) {
		t.Fatalf("a newer layout took a pin: %v", err)
	}
	if err := UnpinFromHome("acme/c", pinReg); !errors.Is(err, errNewerHome) {
		t.Fatalf("a newer layout took an unpin: %v", err)
	}
	// An uninstall's unpin never fails on Home, and leaves a newer layout
	// as it is.
	before := readRaw(t)
	if err := UnpinRef("acme/a", false); err != nil {
		t.Fatalf("UnpinRef on a newer layout: %v", err)
	}
	if readRaw(t) != before {
		t.Fatal("UnpinRef changed a newer layout")
	}
}

// Settings → Home (agentry-desktop#661): hiding a default card between
// Pinned cards keeps its place and leaves every pin where it was; showing
// it puts it back there. A card hidden before a pin opened a Pinned card
// stays ahead of it.
func TestHidingACardKeepsItsPlaceAmongThePinnedCards(t *testing.T) {
	isolate(t)
	if err := SetHomeCardShown(HomeProfile, false); err != nil {
		t.Fatal(err)
	}
	pin(t, "a", "b", "c", "d", "e")
	if err := SetHomeCardShown(HomeRecentRuns, false); err != nil {
		t.Fatal(err)
	}
	l, _, _, _ := HomeLayoutNow()
	if got, want := strings.Join(kinds(l), ","), "recent-runs(hidden),installed,profile(hidden),run-defaults,pinned,pinned"; got != want {
		t.Fatalf("after hiding = %s, want %s", got, want)
	}
	if got := pinCards(t); got != "a,b,c,d|e" {
		t.Fatalf("hiding moved the pins: %s", got)
	}
	for _, k := range []string{HomeRecentRuns, HomeProfile} {
		if err := SetHomeCardShown(k, true); err != nil {
			t.Fatal(err)
		}
	}
	l, _, _, _ = HomeLayoutNow()
	if got, want := strings.Join(kinds(l), ","), "recent-runs,installed,profile,run-defaults,pinned,pinned"; got != want {
		t.Fatalf("after showing = %s, want %s", got, want)
	}
	if got := pinCards(t); got != "a,b,c,d|e" {
		t.Fatalf("showing moved the pins: %s", got)
	}
}

// Reset Home restores the four default cards, every one shown, and drops
// every pin with the Pinned cards (agentry-desktop#661: the page says so
// before it asks). The QuickAgents themselves are not touched: a pin is
// only the layout's.
func TestResetDropsEveryPin(t *testing.T) {
	isolate(t)
	pin(t, "a", "b", "c", "d", "e")
	if err := SetHomeCardShown(HomeInstalled, false); err != nil {
		t.Fatal(err)
	}
	if err := ResetHome(); err != nil {
		t.Fatal(err)
	}
	l, newer, unreadable, err := HomeLayoutNow()
	if err != nil || newer || unreadable {
		t.Fatalf("after Reset: err=%v newer=%v unreadable=%v", err, newer, unreadable)
	}
	if got, want := strings.Join(kinds(l), ","), "recent-runs,installed,profile,run-defaults"; got != want {
		t.Fatalf("after Reset = %s, want %s", got, want)
	}
	if got := pinCards(t); got != "" || IsPinned("acme/a", pinReg) {
		t.Fatalf("Reset kept pins: %q", got)
	}
	pin(t, "f")
	if got := pinCards(t); got != "f" {
		t.Fatalf("a pin after Reset: %q", got)
	}
}
