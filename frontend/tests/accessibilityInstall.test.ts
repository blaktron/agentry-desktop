// The install-unscanned dialog passes an automated accessibility check, and is a
// named modal dialog.
import { renderHarness, report, violations, window } from "./a11y.ts";
import { test } from "node:test";
import assert from "node:assert/strict";

const root = await renderHarness("modals", "install-unscanned");

test("the install-unscanned dialog is a named modal dialog with no accessibility violations axe can find", async () => {
  const dialog = window.document.querySelector('[role="dialog"]');
  assert.ok(dialog, "no dialog");
  assert.equal(dialog.getAttribute("aria-modal"), "true");
  const name = dialog.getAttribute("aria-label") ?? window.document.getElementById(dialog.getAttribute("aria-labelledby") ?? "")?.textContent;
  assert.ok(name && name.trim() !== "", "the dialog has no name");
  const vs = await violations(root);
  assert.equal(vs.length, 0, report(vs));
});
