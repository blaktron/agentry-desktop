// At 200% text size the smallest window (900 by 600) is 450 by 300 CSS
// pixels. There the sidebar folds under a Pages button, every page reflows
// with no sideways scroll, and the folded rows open, take a choice and
// close from the keyboard.
import { expect, test } from "@playwright/test";
import { axeViolations, report } from "./axe";
import { SETTINGS } from "./states";

test.use({ viewport: { width: 450, height: 300 } });

const PAGES = [
  "/pages.html",
  ...SETTINGS.map((s) => `/pages.html?settings=${s}`),
  "/knowledge.html#knowledge-local", "/knowledge.html#knowledge-cloud",
  "/reviews.html#validate", "/reviews.html#report-held",
  "/runs.html#run-finished", "/runs.html#run-failed",
  "/drafts.html#draft-landed",
];

for (const url of PAGES) {
  test(`${url} reflows at 200% with no sideways scroll`, async ({ page }) => {
    await page.goto(url);
    await page.waitForLoadState("networkidle");
    await page.waitForTimeout(500);
    const wide = await page.evaluate(() => [...document.querySelectorAll(".content")]
      .filter((c) => c.scrollWidth > c.clientWidth + 2)
      .map((c) => `${c.closest("[data-preview]")?.getAttribute("data-preview") ?? "page"}: ${c.scrollWidth} in ${c.clientWidth}`));
    expect(wide).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  });
}

test("the pages at 200% have no accessibility violations", async ({ page }) => {
  const vs = await axeViolations(page, "/pages.html");
  expect(vs, report(vs)).toEqual([]);
});

test("the folded sidebar opens with its Pages button, closes on a choice and on Escape", async ({ page }) => {
  await page.goto("/pages.html");
  const shell = page.locator('[data-preview="library"]');
  const toggle = shell.locator(".nav-toggle");
  const nav = shell.locator(".sidebar-nav");
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(nav).toBeHidden();

  await toggle.focus();
  await page.keyboard.press("Enter");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(nav).toBeVisible();
  await page.keyboard.press("Tab");
  await page.keyboard.press("Escape");
  await expect(nav).toBeHidden();
  await expect(toggle).toBeFocused();

  await page.keyboard.press("Enter");
  await nav.getByRole("button", { name: "Browse", exact: true }).click();
  await expect(nav).toBeHidden();
});

test("a wide window shows the rows and no Pages button", async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 800 });
  await page.goto("/pages.html");
  const shell = page.locator('[data-preview="library"]');
  await expect(shell.locator(".nav-toggle")).toBeHidden();
  await expect(shell.locator(".sidebar-nav")).toBeVisible();
});
