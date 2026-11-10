// Under Windows High Contrast (forced colours) the pages and the wizard pass
// axe, and the states drawn by a fill keep a visible difference: the current
// step's dot from the others, the open page's sidebar row from the rest.
import { expect, test } from "@playwright/test";
import { axeViolations, report } from "./axe";

test.use({ forcedColors: "active" });

for (const url of ["/pages.html", "/pages.html?settings=home", "/wizard.html#1", "/modals.html#runpanel"]) {
  test(`${url} has no accessibility violations under forced colours`, async ({ page }) => {
    const vs = await axeViolations(page, url);
    expect(vs, report(vs)).toEqual([]);
  });
}

test("the wizard's current step dot stays apart from the steps to come", async ({ page }) => {
  await page.goto("/wizard.html#1");
  const look = (sel: string) => page.locator(sel).first().evaluate((el) => {
    const s = getComputedStyle(el);
    return { fill: s.backgroundColor, width: s.width, edge: s.borderTopStyle };
  });
  const current = await look(".wizard-dot.active");
  const later = await look(".wizard-dot:not(.active):not(.done)");
  expect(current.edge).toBe("solid");
  expect(later.edge).toBe("solid");
  expect(current.fill).not.toBe(later.fill);
  expect(current.width).not.toBe(later.width);
});

test("the open page's sidebar row is outlined", async ({ page }) => {
  await page.goto("/pages.html");
  const row = page.locator('[data-preview="library"] .nav-item.active').first();
  await expect(row).toHaveCSS("outline-style", "solid");
});

test("badges keep an edge, and the keyboard focus is drawn in the highlight colour", async ({ page }) => {
  await page.goto("/pages.html");
  await expect(page.locator('[data-preview="browse"] .badge').first()).toHaveCSS("border-top-style", "solid");
  await page.locator('[data-preview="browse"] input[type="search"]').focus();
  await page.keyboard.press("Tab");
  const ring = await page.evaluate(() => {
    const el = document.activeElement as HTMLElement;
    return { style: getComputedStyle(el).outlineStyle, colour: getComputedStyle(el).outlineColor };
  });
  expect(ring.style).not.toBe("none");
});
