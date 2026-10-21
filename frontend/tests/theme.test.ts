// The themes Settings → Home offers, and putting one on the window.
import { test } from "node:test";
import assert from "node:assert/strict";
import { applyTheme, isTheme, THEMES, themeOn } from "../src/theme.ts";

const root = () => ({ dataset: {} as Record<string, string | undefined> });

test("Settings lists System first, then Light, Dark, Slate, Forest, Midnight and High contrast", () => {
  assert.deepEqual(THEMES.map((t) => t.title), ["System", "Light", "Dark", "Slate", "Forest", "Midnight", "High contrast"]);
  for (const t of THEMES) assert.ok(t.line.endsWith("."), t.id);
});

test("a theme is put on the root element; System is no attribute", () => {
  const r = root();
  assert.equal(applyTheme("slate", r), "slate");
  assert.equal(r.dataset.theme, "slate");
  assert.equal(themeOn(r), "slate");
  assert.equal(applyTheme("system", r), "system");
  assert.equal("theme" in r.dataset, false);
  assert.equal(themeOn(r), "system");
});

test("a name this app lacks is System", () => {
  const r = root();
  r.dataset.theme = "midnight";
  assert.equal(applyTheme("aurora", r), "system");
  assert.equal("theme" in r.dataset, false);
  assert.equal(isTheme("aurora"), false);
  assert.equal(themeOn({ dataset: { theme: "aurora" } }), "system");
});
