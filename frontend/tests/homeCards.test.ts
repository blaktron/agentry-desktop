// Home's card registry: which of the layout's cards the page draws, in what
// order, at what size.
import { test } from "node:test";
import assert from "node:assert/strict";
import { HOME_CARDS, cardsToDraw } from "../src/homeCards.ts";

const all = new Set(Object.keys(HOME_CARDS));
const kinds = (cs: { kind: string }[]) => cs.map((c) => c.kind);

test("the layout's order is the page's, and a hidden card is not drawn", () => {
  const drawn = cardsToDraw([{ kind: "run-defaults" }, { kind: "installed", hidden: true }, { kind: "profile" }], all);
  assert.deepEqual(kinds(drawn), ["run-defaults", "profile"]);
});

test("a kind this app does not know, or has no renderer for yet, is skipped and not an error", () => {
  const drawable = new Set(["installed", "profile", "run-defaults"]);
  const drawn = cardsToDraw([{ kind: "credits-chart" }, { kind: "recent-runs" }, { kind: "installed" }, { kind: "" }], drawable);
  assert.deepEqual(kinds(drawn), ["installed"]);
});

test("a default card is drawn once however often the layout names it; a pinned card may repeat", () => {
  const pin = { ref: "acme/a", registry: "https://secureagentry.net" };
  const drawn = cardsToDraw([{ kind: "profile" }, { kind: "pinned", pins: [pin] }, { kind: "profile" }, { kind: "pinned", pins: [pin] }], all);
  assert.deepEqual(kinds(drawn), ["profile", "pinned", "pinned"]);
  assert.equal(new Set(drawn.map((c) => c.key)).size, drawn.length, "each drawn card has its own key");
  assert.deepEqual(drawn[1].pins, [pin]);
  assert.deepEqual(drawn[0].pins, []);
});

test("a Pinned card is drawn only while it holds a pin", () => {
  const drawn = cardsToDraw([{ kind: "pinned" }, { kind: "pinned", pins: [] }, { kind: "installed" }], all);
  assert.deepEqual(kinds(drawn), ["installed"]);
});

test("every card is Large today, whatever size the layout names", () => {
  const drawn = cardsToDraw([{ kind: "installed", size: "small" }, { kind: "profile" }], all);
  assert.deepEqual(drawn.map((c) => c.size), ["large", "large"]);
});

test("each kind has a title for Settings to name it by", () => {
  for (const d of Object.values(HOME_CARDS)) assert.ok(d.title.length > 0, d.kind);
});

// Settings → Home: one switch per default card, and
// the landing card's status line.
import { HOME_DEFAULTS, cardSwitches, homeStatus } from "../src/homeCards.ts";

test("Settings → Home has a switch for each default card, in the default order, whatever the layout's, then the Scheduled card", () => {
  const sw = cardSwitches([{ kind: "run-defaults" }, { kind: "installed", hidden: true }, { kind: "pinned", pins: [{ ref: "a/b" }] }, { kind: "recent-runs" }]);
  assert.deepEqual(sw.map((s) => s.kind), [...HOME_DEFAULTS, "scheduled"]);
  assert.deepEqual(sw.map((s) => s.title), ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults", "Scheduled"]);
  // A card the stored layout lacks is not drawn, so its switch is off;
  // Go adds it at the end when it is turned on.
  assert.deepEqual(sw.map((s) => s.shown), [true, false, false, true, false]);
});

test("a default card the layout names twice reads as shown while any of its entries is", () => {
  const sw = cardSwitches([{ kind: "profile", hidden: true }, { kind: "profile" }]);
  assert.equal(sw.find((s) => s.kind === "profile")?.shown, true);
});

const view = (cards: { kind: string; hidden?: boolean; pins?: { ref: string }[] }[], extra: { newer?: boolean; unreadable?: boolean } = {}) =>
  ({ cards, newer: false, unreadable: false, ...extra });
const defaults = HOME_DEFAULTS.map((kind) => ({ kind }));

test("the landing's line counts the cards shown and the QuickAgents pinned", () => {
  assert.equal(homeStatus(view(defaults)), "All 4 cards shown, nothing pinned.");
  assert.equal(homeStatus(view([...defaults.slice(0, 3), { kind: "run-defaults", hidden: true }, { kind: "pinned", pins: [{ ref: "a/b" }] }])),
    "3 of 4 cards shown, 1 QuickAgent pinned.");
  assert.equal(homeStatus(view([{ kind: "pinned", pins: [{ ref: "a/b" }, { ref: "a/c" }, { ref: "a/d" }, { ref: "a/e" }] }, { kind: "pinned", pins: [{ ref: "a/f" }] }])),
    "No card shown, 5 QuickAgents pinned.");
});

test("a layout this app keeps rather than uses says so, and counts nothing", () => {
  assert.equal(homeStatus(view(defaults, { newer: true })), "Arranged by a newer Agentry Desktop; the default cards are shown.");
  assert.equal(homeStatus(view(defaults, { unreadable: true })), "The arrangement cannot be read; the default cards are shown.");
});

// The Scheduled card: one Large card, pinned from the Scheduled page,
// listed in Settings → Home after the defaults.
import { scheduledOnHome } from "../src/homeCards.ts";

test("the Scheduled card is drawn once, Large, in the layout's place, and an older app skips it", () => {
  const drawn = cardsToDraw([{ kind: "profile" }, { kind: "scheduled", size: "large" }, { kind: "scheduled" }], all);
  assert.deepEqual(kinds(drawn), ["profile", "scheduled"]);
  assert.equal(drawn[1].size, "large");
  // 0.6.81 had no renderer for it: a kind without one is skipped.
  const older = new Set(["recent-runs", "installed", "profile", "run-defaults", "pinned"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "scheduled", size: "large" }, { kind: "installed" }], older)), ["installed"]);
});

test("the options menu offers Unpin while Home draws the Scheduled card, else Pin", () => {
  assert.equal(scheduledOnHome(defaults), false);
  assert.equal(scheduledOnHome([...defaults, { kind: "scheduled" }]), true);
  assert.equal(scheduledOnHome([...defaults, { kind: "scheduled", hidden: true }]), false);
});

test("Settings → Home lists the Scheduled card after the defaults, on only while it is drawn", () => {
  const sw = cardSwitches([...defaults, { kind: "scheduled" }]);
  assert.deepEqual(sw.map((s) => s.kind), [...HOME_DEFAULTS, "scheduled"]);
  assert.equal(sw[4].title, "Scheduled");
  assert.equal(sw[4].shown, true);
  assert.equal(cardSwitches(defaults)[4].shown, false);
  assert.equal(cardSwitches([...defaults, { kind: "scheduled", hidden: true }])[4].shown, false);
});

test("the landing counts the Scheduled card only while the layout holds it", () => {
  assert.equal(homeStatus(view(defaults)), "All 4 cards shown, nothing pinned.");
  assert.equal(homeStatus(view([...defaults, { kind: "scheduled" }])), "All 5 cards shown, nothing pinned.");
  assert.equal(homeStatus(view([...defaults, { kind: "scheduled", hidden: true }])), "4 of 5 cards shown, nothing pinned.");
});
