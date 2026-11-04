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

// Settings → Home: one switch per card, the cards first and the shortcuts
// after, and the landing card's status line.
import { HOME_STARTS, HOME_SWITCHES, cardSwitches, homeStatus, kindSize, startLayout, isShortcut } from "../src/homeCards.ts";

const CARDS = ["get-started", "main", "deliverables", "recent-runs", "installed", "profile", "run-defaults", "runhost", "knowledge", "learn", "scheduled"];
const SHORTCUTS = ["shortcut-run", "shortcut-draft", "shortcut-knowledge", "shortcut-voice", "shortcut-agent", "shortcut-setup"];

test("Settings → Home has a switch for every card but Pinned, the cards then the shortcuts, whatever the layout's order", () => {
  const sw = cardSwitches([{ kind: "run-defaults" }, { kind: "installed", hidden: true }, { kind: "pinned", pins: [{ ref: "a/b" }] }, { kind: "recent-runs" }, { kind: "shortcut-draft" }]);
  assert.deepEqual(sw.map((s) => s.kind), [...CARDS, ...SHORTCUTS]);
  assert.deepEqual(sw.filter((s) => s.shortcut).map((s) => s.kind), SHORTCUTS);
  // A card the stored layout lacks is not drawn, so its switch is off;
  // Go adds it at the end when it is turned on.
  assert.deepEqual(sw.filter((s) => s.shown).map((s) => s.kind), ["recent-runs", "run-defaults", "shortcut-draft"]);
});

test("Settings → Home has the Meeting notes card and shortcut only where this copy records meetings", () => {
  assert.equal(cardSwitches([{ kind: "meetings" }]).some((s) => s.kind === "meetings" || s.kind === "shortcut-meetings"), false);
  const sw = cardSwitches([{ kind: "meetings" }], true);
  assert.deepEqual(sw.map((s) => s.kind), HOME_SWITCHES);
  assert.equal(sw.find((s) => s.kind === "meetings")?.shown, true);
  assert.equal(cardSwitches([{ kind: "meetings", hidden: true }], true).find((s) => s.kind === "meetings")?.shown, false);
});

test("a card the layout names twice reads as shown while any of its entries is", () => {
  const sw = cardSwitches([{ kind: "profile", hidden: true }, { kind: "profile" }]);
  assert.equal(sw.find((s) => s.kind === "profile")?.shown, true);
});

const view = (cards: { kind: string; hidden?: boolean; pins?: { ref: string }[] }[], extra: { newer?: boolean; unreadable?: boolean } = {}) =>
  ({ cards, newer: false, unreadable: false, ...extra });
const defaults = ["main", "deliverables", "recent-runs", "installed", "profile", "run-defaults"].map((kind) => ({ kind }));

test("the landing's line counts the cards the layout holds and shows, and the QuickAgents pinned", () => {
  assert.equal(homeStatus(view(defaults)), "All 6 cards shown, nothing pinned.");
  assert.equal(homeStatus(view(startLayout("local"))), `All ${HOME_STARTS.local.length} cards shown, nothing pinned.`);
  assert.equal(homeStatus(view([...defaults.slice(0, 5), { kind: "run-defaults", hidden: true }, { kind: "pinned", pins: [{ ref: "a/b" }] }])),
    "5 of 6 cards shown, 1 QuickAgent pinned.");
  assert.equal(homeStatus(view([{ kind: "pinned", pins: [{ ref: "a/b" }, { ref: "a/c" }, { ref: "a/d" }, { ref: "a/e" }] }, { kind: "pinned", pins: [{ ref: "a/f" }] }])),
    "No card shown, 5 QuickAgents pinned.");
  // A kind this app does not know is not counted.
  assert.equal(homeStatus(view([...defaults, { kind: "credits-chart" }])), "All 6 cards shown, nothing pinned.");
});

test("a layout this app keeps rather than uses says so, and counts nothing", () => {
  assert.equal(homeStatus(view(defaults, { newer: true })), "Arranged by a newer Agentry Desktop; the starting cards are shown.");
  assert.equal(homeStatus(view(defaults, { unreadable: true })), "The arrangement cannot be read; the starting cards are shown.");
});

// The Scheduled card: one Large card, pinned from the Scheduled page.
import { scheduledOnHome } from "../src/homeCards.ts";

test("the Scheduled card is drawn once, Large, in the layout's place, and an older app skips it", () => {
  const drawn = cardsToDraw([{ kind: "profile" }, { kind: "scheduled", size: "large" }, { kind: "scheduled" }], all);
  assert.deepEqual(kinds(drawn), ["profile", "scheduled"]);
  assert.equal(drawn[1].size, "large");
  // 0.6.81 had no renderer for it: a kind without one is skipped.
  const older = new Set(["deliverables", "recent-runs", "installed", "profile", "run-defaults", "pinned"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "scheduled", size: "large" }, { kind: "installed" }], older)), ["installed"]);
});

test("the options menu offers Unpin while Home draws the Scheduled card, else Pin", () => {
  assert.equal(scheduledOnHome(defaults), false);
  assert.equal(scheduledOnHome([...defaults, { kind: "scheduled" }]), true);
  assert.equal(scheduledOnHome([...defaults, { kind: "scheduled", hidden: true }]), false);
});

test("Settings → Home's Scheduled switch is on only while Home draws the card", () => {
  const on = (cards: { kind: string; hidden?: boolean }[]) => cardSwitches(cards).find((s) => s.kind === "scheduled")?.shown;
  assert.equal(on([...defaults, { kind: "scheduled" }]), true);
  assert.equal(on(defaults), false);
  assert.equal(on([...defaults, { kind: "scheduled", hidden: true }]), false);
});

test("the landing counts the Scheduled card only while the layout holds it", () => {
  assert.equal(homeStatus(view([...defaults, { kind: "scheduled" }])), "All 7 cards shown, nothing pinned.");
  assert.equal(homeStatus(view([...defaults, { kind: "scheduled", hidden: true }])), "6 of 7 cards shown, nothing pinned.");
});

test("the Deliverables card and Main's are drawn where the layout puts them, once, and an older app skips them", () => {
  const all2 = new Set(["deliverables", "recent-runs", "installed"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "installed" }, { kind: "deliverables" }], all2)), ["installed", "deliverables"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "deliverables" }, { kind: "installed" }], new Set(["installed"]))), ["installed"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "main" }, { kind: "installed" }, { kind: "main" }], all)), ["main", "installed"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "main", hidden: true }, { kind: "installed" }], all)), ["installed"]);
  assert.deepEqual(kinds(cardsToDraw([{ kind: "main" }, { kind: "installed" }], new Set(["installed"]))), ["installed"]);
});

// The starting Homes: one per place runs go, Go's and this module's the
// same.
test("every start opens on Get started and the shortcuts, holds the agent card and Deliverables, and only its own place's cards", () => {
  for (const [start, ks] of Object.entries(HOME_STARTS)) {
    assert.equal(ks[0], "get-started", start);
    assert.deepEqual(ks.slice(1, 5), ["shortcut-run", "shortcut-draft", "shortcut-knowledge", "shortcut-setup"], start);
    assert.ok(ks.includes("main") && ks.includes("deliverables"), start);
  }
  assert.ok(HOME_STARTS.remote.includes("runhost") && !HOME_STARTS.local.includes("runhost") && !HOME_STARTS.cloud.includes("runhost"));
  assert.ok(HOME_STARTS.cloud.includes("profile") && !HOME_STARTS.local.includes("profile"));
  assert.ok(HOME_STARTS.local.includes("shortcut-meetings") && !HOME_STARTS.cloud.includes("shortcut-meetings") && !HOME_STARTS.remote.includes("shortcut-meetings"));
});

test("a shortcut is one cell and Learn one column by two rows; every other card takes the grid's default", () => {
  for (const k of HOME_SWITCHES) {
    const size = kindSize(k);
    if (isShortcut(k)) assert.deepEqual(size, { cols: 1, rows: 1 }, k);
    else if (k === "learn") assert.deepEqual(size, { cols: 1, rows: 2 });
    else assert.deepEqual(size, {}, k);
  }
  assert.deepEqual(startLayout("cloud").find((c) => c.kind === "shortcut-voice"), { kind: "shortcut-voice", cols: 1, rows: 1 });
});

test("the starts and the switches here are the ones Go keeps, in its order", async () => {
  const { readFileSync } = await import("node:fs");
  const go = readFileSync(new URL("../../internal/settings/home.go", import.meta.url), "utf8");
  const consts = new Map([...go.matchAll(/(Home\w+)\s*=\s*"([a-z-]+)"/g)].map((x) => [x[1], x[2]]));
  const names = (list: string) => list.split(",").map((n) => n.trim()).filter(Boolean).map((n) => consts.get(n));
  const sw = /var homeSwitches = \[\]string\{([^}]*)\}/.exec(go);
  assert.ok(sw, "internal/settings/home.go names homeSwitches");
  assert.deepEqual(names(sw![1]), HOME_SWITCHES);
  for (const [goName, start] of [["HomeStartLocal", "local"], ["HomeStartRemote", "remote"], ["HomeStartCloud", "cloud"]] as const) {
    const m = new RegExp(`${goName}: \\{([^}]*)\\}`).exec(go);
    assert.ok(m, `home.go names the ${start} start`);
    assert.deepEqual(names(m![1]), HOME_STARTS[start], start);
  }
});
