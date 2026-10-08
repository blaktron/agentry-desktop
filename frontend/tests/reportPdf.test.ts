import { test } from "node:test";
import assert from "node:assert/strict";
import { pdfActions, REPORT_THEMES, themeSavedSentence } from "../src/reportPdf.ts";

// D15, D16: only a PDF the run's export made opens, in the app or the
// system's viewer; any other is Save only, and says why.
test("an exported PDF opens in the app and the system viewer, and can be saved", () => {
  const a = pdfActions({ exported: true, pages: 3, theme: "letter" });
  assert.equal(a.view, true);
  assert.equal(a.openInSystemViewer, true);
  assert.equal(a.save, true);
  assert.equal(a.line, "A PDF the run exported (3 pages, letter theme).");
  assert.equal(pdfActions({ exported: true, pages: 1 }).line, "A PDF the run exported (1 page).");
  assert.equal(pdfActions({ exported: true }).line, "A PDF the run exported.");
});

test("a PDF the export did not make is Save only, and says why", () => {
  const a = pdfActions({ exported: false, pages: 9, theme: "plain" });
  assert.equal(a.view, false);
  assert.equal(a.openInSystemViewer, false);
  assert.equal(a.save, true);
  assert.match(a.line, /^Not a PDF this run's PDF export made/);
});

test("the five built-in themes, in order, and what a save says", () => {
  assert.deepEqual(REPORT_THEMES.map((t) => t.name), ["plain", "formal", "brief", "technical", "letter"]);
  assert.equal(themeSavedSentence("formal"), "Saved. Your runs' PDF reports use the Formal theme.");
  assert.match(themeSavedSentence(""), /no report theme: runs from this app use Plain/);
});
