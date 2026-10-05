// Home's card registry (plan desktop-home-and-library.md §4.4, D-067;
// agentry-desktop#658): which of the layout's cards the page draws, in what
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
  const drawn = cardsToDraw([{ kind: "profile" }, { kind: "pinned" }, { kind: "profile" }, { kind: "pinned" }], all);
  assert.deepEqual(kinds(drawn), ["profile", "pinned", "pinned"]);
  assert.equal(new Set(drawn.map((c) => c.key)).size, drawn.length, "each drawn card has its own key");
});

test("every card is Large today, whatever size the layout names", () => {
  const drawn = cardsToDraw([{ kind: "installed", size: "small" }, { kind: "profile" }], all);
  assert.deepEqual(drawn.map((c) => c.size), ["large", "large"]);
});

test("each kind has a title for Settings to name it by", () => {
  for (const d of Object.values(HOME_CARDS)) assert.ok(d.title.length > 0, d.kind);
});
