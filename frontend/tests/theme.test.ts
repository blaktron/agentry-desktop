// The themes Settings → Home offers, and putting one on the window.
import { test } from "node:test";
import assert from "node:assert/strict";
import { applyTheme, isTheme, THEME_KEY, THEMES, themeOn } from "../src/theme.ts";

const root = () => ({ dataset: {} as Record<string, string | undefined> });
const store = () => {
  const kept = new Map<string, string>();
  return { kept, setItem: (k: string, v: string) => { kept.set(k, v); } };
};

test("Settings lists System first, then Light, Dark, Slate, Forest, Midnight and High contrast", () => {
  assert.deepEqual(THEMES.map((t) => t.title), ["System", "Light", "Dark", "Slate", "Forest", "Midnight", "High contrast"]);
  for (const t of THEMES) assert.ok(t.line.endsWith("."), t.id);
});

test("a theme is put on the root element and kept for the next start; System is no attribute", () => {
  const r = root();
  const s = store();
  assert.equal(applyTheme("slate", r, s), "slate");
  assert.equal(r.dataset.theme, "slate");
  assert.equal(s.kept.get(THEME_KEY), "slate");
  assert.equal(themeOn(r), "slate");
  assert.equal(applyTheme("system", r, s), "system");
  assert.equal("theme" in r.dataset, false);
  assert.equal(s.kept.get(THEME_KEY), "system");
  assert.equal(themeOn(r), "system");
});

test("a name this app lacks is System, and storage that refuses is not an error", () => {
  const r = root();
  r.dataset.theme = "midnight";
  assert.equal(applyTheme("aurora", r, { setItem: () => { throw new Error("quota"); } }), "system");
  assert.equal("theme" in r.dataset, false);
  assert.equal(isTheme("aurora"), false);
  assert.equal(themeOn({ dataset: { theme: "aurora" } }), "system");
});
