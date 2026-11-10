// The first-run wizard passes an automated accessibility check on its sign-in
// step, and its step dots say which step this is.
import { renderHarness, report, violations, window } from "./a11y.ts";
import { test } from "node:test";
import assert from "node:assert/strict";

const root = await renderHarness("wizard", "1");

test("the wizard's steps are a list a screen reader reads, with the current step marked", () => {
  const steps = window.document.querySelector('ol[aria-label="Setup steps"]');
  assert.ok(steps, "no steps list");
  const current = steps.querySelector('[aria-current="step"]');
  assert.match(current?.textContent ?? "", /^Step 2 of \d+: /);
});

test("the wizard has no accessibility violations axe can find", async () => {
  const vs = await violations(root);
  assert.equal(vs.length, 0, report(vs));
});
