// Settings → Home, with its Accessibility part, passes the automated
// accessibility check, and the part is a labelled group of choices.
import { renderHarness, report, violations, window } from "./a11y.ts";
import { test } from "node:test";
import assert from "node:assert/strict";

await renderHarness("pages", "", "settings=home&textsize=125");
const preview = window.document.querySelector('[data-preview="settings"]');

test("Settings → Home shows its Accessibility part with the kept text size chosen", () => {
  const part = window.document.getElementById("settings-part-accessibility");
  assert.ok(part, "no Accessibility part");
  assert.equal(part.querySelector("h2")?.textContent, "Accessibility");
  assert.equal(part.querySelector<HTMLInputElement>('input[name="text-size"]:checked')?.value, "125");
  assert.ok(part.querySelector('[role="radiogroup"][aria-label="Motion"]'));
});

test("Settings → Home has no accessibility violations axe can find", async () => {
  assert.ok(preview);
  const vs = await violations(preview);
  assert.equal(vs.length, 0, report(vs));
});
