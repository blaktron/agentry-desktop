// Pins on Home: what the page reads back from the layout Go keeps.
import { test } from "node:test";
import assert from "node:assert/strict";
import { homeFullSentence, isPinned, latestRunFor, pinOffer, pinRecord, pinsOf, registryHost } from "../src/pins.ts";

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
  // Compared as Go compares it: case is not folded, so the page never shows
  // as pinned what Go's Unpin would not find.
  assert.ok(!isPinned(cards, "acme/a", REG.toUpperCase()));
});

test("the Pin button offers Unpin, Pin, or says Home is full in Go's limits", () => {
  const roomy = { maxPins: 16, pinsPerCard: 4 };
  const full3 = { maxPins: 3, pinsPerCard: 2 };
  assert.deepEqual(pinOffer(cards, roomy, "acme/a", REG), { pinned: true, full: false });
  assert.deepEqual(pinOffer(cards, roomy, "acme/c", REG), { pinned: false, full: false });
  const full = pinOffer(cards, full3, "acme/c", REG);
  assert.equal(full.full, true);
  assert.equal(full.why, "Home holds 3 pinned QuickAgents, 2 to a Pinned card. Unpin one to pin another.");
  assert.equal(homeFullSentence(cards, full3), full.why);
  assert.equal(homeFullSentence(cards, roomy), null);
  // A pinned QuickAgent can always be unpinned, full or not.
  assert.deepEqual(pinOffer(cards, full3, "acme/b", REG), { pinned: true, full: false });
});

const rec = (over: Partial<{ namespace: string; name: string; origin: string; local: boolean; present: boolean; version: string }>) =>
  ({ namespace: "acme", name: "a", origin: REG, local: false, present: true, version: "1.0.0", ...over });

test("a pin shows the record from its registry, else another from a registry; a folder pin only a folder install", () => {
  const other = rec({ origin: "https://other.example", version: "2.0.0" });
  const mine = rec({ version: "1.0.0" });
  assert.equal(pinRecord([other, mine], { ref: "acme/a", registry: REG })?.version, "1.0.0");
  assert.equal(pinRecord([other], { ref: "acme/a", registry: REG })?.version, "2.0.0");
  assert.equal(pinRecord([rec({ present: false })], { ref: "acme/a", registry: REG }), undefined);
  const local = rec({ origin: "", local: true, version: "0.1.0" });
  assert.equal(pinRecord([mine, local], { ref: "acme/a" }), local);
  assert.equal(pinRecord([mine], { ref: "acme/a" }), undefined, "a folder pin never shows a registry install");
  assert.equal(pinRecord([local], { ref: "acme/a", registry: REG }), undefined, "a registry pin never shows a folder install");
});

test("a pin's latest run is the newest run or chat of its ref from its registry, never a built-in's job", () => {
  const rows = [
    { id: "1", kind: "draft", ref: "acme/a@1.0.0", registry: REG },
    { id: "2", kind: "run", ref: "acme/ab@1.0.0", registry: REG },
    { id: "5", kind: "run", ref: "acme/a@1.0.0", registry: "https://other.example" },
    { id: "3", kind: "chat", ref: "acme/a@1.1.0", registry: REG + "/" },
    { id: "4", kind: "run", ref: "acme/a@1.0.0", registry: REG },
  ];
  assert.equal(latestRunFor(rows, { ref: "acme/a", registry: REG })?.id, "3");
  assert.equal(latestRunFor(rows, { ref: "acme/a", registry: "https://other.example" })?.id, "5");
  // A folder install's run records whatever registry the app had: by ref.
  assert.equal(latestRunFor(rows, { ref: "acme/a" })?.id, "5");
  assert.equal(latestRunFor(rows, { ref: "acme/never", registry: REG }), undefined);
});

test("Settings → Home names a pin's registry by its host, never a userinfo part", () => {
  assert.equal(registryHost("https://secureagentry.net/"), "secureagentry.net");
  assert.equal(registryHost("http://localhost:8080"), "localhost:8080");
  assert.equal(registryHost("https://chris:s3cret@registry.example"), "registry.example");
  assert.equal(registryHost("not a url"), "");
});
