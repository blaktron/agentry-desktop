import { test } from "node:test";
import assert from "node:assert/strict";
import { fallbackSentence, renderSentence } from "../src/reportDesign.ts";

// Render says how many pages the runner laid out and which cover it drew,
// and says so plainly when the design's own cover was not the one drawn.
test("a render's line names its pages, its cover and the logo", () => {
  assert.equal(renderSentence({ pages: 5, cover: "framed", logo: false, fallback: false }),
    "5 pages of the sample report, laid out by the runner's Typst on this computer as a run's PDF is, with a framed cover page.");
  assert.equal(renderSentence({ pages: 1, cover: "", logo: true, fallback: false }),
    "One page of the sample report, laid out by the runner's Typst on this computer as a run's PDF is, with no cover page and your logo.");
  assert.equal(renderSentence({ pages: 4, cover: "largeType", logo: false, fallback: false }),
    "4 pages of the sample report, laid out by the runner's Typst on this computer as a run's PDF is, with a large type cover page.");
});

test("a custom cover that was not drawn is said, with the cover drawn instead", () => {
  assert.equal(fallbackSentence({ pages: 5, cover: "band", logo: false, fallback: true }),
    "Your custom cover was not drawn: this render uses the band cover in its place, as a run's PDF would. The cover's code is kept with the design.");
  assert.equal(fallbackSentence({ pages: 5, cover: "custom", logo: false, fallback: false }), "");
  assert.equal(fallbackSentence({ pages: 5, cover: "split", logo: false, fallback: false }), "");
});
