// The app's main pages pass an automated accessibility check: the design
// harness's page previews (Home, Library, Browse, an agent's page,
// Scheduled, Build, Validate, Voice, Publisher, Settings, Agents, Plugins,
// Computer use and a persistent agent's page), rendered with its mocks.
import { renderHarness, report, violations, window } from "./a11y.ts";
import { test } from "node:test";
import assert from "node:assert/strict";

await renderHarness("pages");
const previews = Array.from(window.document.querySelectorAll("[data-preview]"));

test("the main pages render, with Browse's results", () => {
  const shown = previews.map((e) => e.getAttribute("data-preview"));
  for (const page of ["home", "library", "browse", "agent", "scheduled", "settings", "agents"]) {
    assert.ok(shown.includes(page), `no ${page} preview in ${shown.join(", ")}`);
  }
  const rows = window.document.querySelectorAll('[data-preview="browse"] .agent-row.clickable');
  assert.ok(rows.length > 0, "the Browse preview shows no results");
});

for (const preview of previews) {
  const page = preview.getAttribute("data-preview");
  test(`the ${page} page has no accessibility violations axe can find`, async () => {
    const vs = await violations(preview);
    assert.equal(vs.length, 0, report(vs));
  });
}
