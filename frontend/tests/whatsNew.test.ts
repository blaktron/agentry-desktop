// What's new: one card per version, newest first, walked with Back and
// Next, closed from any card; the older versions beyond the cards are
// counted on the last one.
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { earlierLine, whatsNewKey, whatsNewStep, type WhatsNewData } from "../src/whatsNew.ts";

const data = (versions: string[], earlier = 0): WhatsNewData => ({
  current: versions[0] ?? "0.6.20",
  cards: versions.map((v) => ({ version: v, highlights: [`What ${v} brings.`] })),
  earlier,
  releasesUrl: "https://github.com/blaktron/agentry-desktop/releases",
});

describe("What's new", () => {
  test("shows nothing without cards", () => {
    assert.equal(whatsNewStep(data([]), 0, "start"), null);
  });

  test("one card is Done at once, with no counter and no Back", () => {
    const s = whatsNewStep(data(["0.6.20"]), 0, "start")!;
    assert.equal(s.title, "What's new in 0.6.20");
    assert.equal(s.lead, "Agentry Desktop is now 0.6.20.");
    assert.equal(s.counter, "");
    assert.equal(s.backDisabled, true);
    assert.equal(s.nextLabel, "Done");
    assert.equal(s.last, true);
    assert.equal(s.earlier, "");
  });

  test("several cards walk newest first, and the last one counts the rest", () => {
    const d = data(["0.6.20", "0.6.19", "0.6.18", "0.6.17", "0.6.16"], 12);
    const first = whatsNewStep(d, 0, "start")!;
    assert.deepEqual([first.card.version, first.counter, first.backDisabled, first.nextLabel, first.earlier], ["0.6.20", "1 of 5", true, "Next", ""]);
    const middle = whatsNewStep(d, 2, "start")!;
    assert.deepEqual([middle.card.version, middle.counter, middle.backDisabled, middle.last, middle.nextLabel], ["0.6.18", "3 of 5", false, false, "Next"]);
    const last = whatsNewStep(d, 4, "start")!;
    assert.deepEqual([last.card.version, last.counter, last.nextLabel, last.earlier], ["0.6.16", "5 of 5", "Done", "And 12 earlier versions."]);
    assert.equal(whatsNewStep(d, 9, "start")!.card.version, "0.6.16");
    assert.equal(whatsNewStep(d, -1, "start")!.card.version, "0.6.20");
  });

  test("Settings says which version is running, not that it is new", () => {
    assert.equal(whatsNewStep(data(["0.6.19"]), 0, "settings")!.lead, "You are running Agentry Desktop 0.6.19.");
  });

  test("the count of earlier versions reads as a sentence", () => {
    assert.equal(earlierLine(0), "");
    assert.equal(earlierLine(1), "And 1 earlier version.");
    assert.equal(earlierLine(3), "And 3 earlier versions.");
  });

  test("the arrows move between the cards and Escape closes from any", () => {
    assert.deepEqual(whatsNewKey("ArrowRight", 0, 3), { at: 1 });
    assert.equal(whatsNewKey("ArrowRight", 2, 3), null);
    assert.deepEqual(whatsNewKey("ArrowLeft", 2, 3), { at: 1 });
    assert.equal(whatsNewKey("ArrowLeft", 0, 3), null);
    for (const at of [0, 1, 2]) assert.equal(whatsNewKey("Escape", at, 3), "close");
    assert.equal(whatsNewKey("Enter", 1, 3), null);
  });

  test("a line is kept exactly as written, markup and all, for the page to show as text", () => {
    const d: WhatsNewData = { current: "0.6.20", cards: [{ version: "0.6.20", highlights: ["<script>alert(1)</script> **bold**"] }], earlier: 0, releasesUrl: "" };
    assert.deepEqual(whatsNewStep(d, 0, "start")!.card.highlights, ["<script>alert(1)</script> **bold**"]);
  });
});
