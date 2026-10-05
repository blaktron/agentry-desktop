// Pins on Home (plan desktop-home-and-library.md D5, D11;
// agentry-desktop#660): what the page reads back from the layout Go keeps.
import { test } from "node:test";
import assert from "node:assert/strict";
import { isPinned, latestRunFor, pinOffer, pinRecord, pinsOf } from "../src/pins.ts";

const REG = "https://dev.secureagentry.net";
const cards = [
  { kind: "recent-runs" },
  { kind: "pinned", pins: [{ ref: "acme/a", registry: REG }, { ref: "me/local" }] },
  { kind: "installed" },
  { kind: "pinned", pins: [{ ref: "acme/b", registry: REG }] },
];

test("pins are read card by card, in the layout's order", () => {
  assert.deepEqual(pinsOf(cards).map((p) => p.ref), ["acme/a", "me/local", "acme/b"]);
});

test("a pin is its ref and its registry; a folder install's has none", () => {
  assert.ok(isPinned(cards, "acme/a", REG + "/"));
  assert.ok(!isPinned(cards, "acme/a", "https://other.example"));
  assert.ok(!isPinned(cards, "acme/a"));
  assert.ok(isPinned(cards, "me/local", ""));
  assert.ok(!isPinned(cards, "me/local", REG));
});

test("the Pin button offers Unpin, Pin, or says Home is full", () => {
  assert.deepEqual(pinOffer(cards, 16, "acme/a", REG), { pinned: true, full: false });
  assert.deepEqual(pinOffer(cards, 16, "acme/c", REG), { pinned: false, full: false });
  const full = pinOffer(cards, 3, "acme/c", REG);
  assert.equal(full.full, true);
  assert.match(full.why ?? "", /Home holds 3 pinned QuickAgents/);
  // A pinned QuickAgent can always be unpinned, full or not.
  assert.deepEqual(pinOffer(cards, 3, "acme/b", REG), { pinned: true, full: false });
});

const rec = (over: Partial<{ namespace: string; name: string; origin: string; local: boolean; present: boolean; version: string }>) =>
  ({ namespace: "acme", name: "a", origin: REG, local: false, present: true, version: "1.0.0", ...over });

test("a pin shows the record from its registry, else another of the same QuickAgent, else none", () => {
  const other = rec({ origin: "https://other.example", version: "2.0.0" });
  const mine = rec({ version: "1.0.0" });
  assert.equal(pinRecord([other, mine], { ref: "acme/a", registry: REG })?.version, "1.0.0");
  assert.equal(pinRecord([other], { ref: "acme/a", registry: REG })?.version, "2.0.0");
  assert.equal(pinRecord([rec({ present: false })], { ref: "acme/a", registry: REG }), undefined);
  const local = rec({ namespace: "me", name: "local", origin: "", local: true });
  assert.equal(pinRecord([local], { ref: "me/local" }), local);
  assert.equal(pinRecord([local], { ref: "me/local", registry: REG }), undefined);
});

test("a pin's latest run is the newest run or chat of its ref, never a built-in's job", () => {
  const rows = [
    { id: "1", kind: "draft", ref: "acme/a@1.0.0" },
    { id: "2", kind: "run", ref: "acme/ab@1.0.0" },
    { id: "3", kind: "chat", ref: "acme/a@1.1.0" },
    { id: "4", kind: "run", ref: "acme/a@1.0.0" },
  ];
  assert.equal(latestRunFor(rows, "acme/a")?.id, "3");
  assert.equal(latestRunFor(rows, "acme/never"), undefined);
});
