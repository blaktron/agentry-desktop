// The main flows with the keyboard alone: from Browse's search to a
// QuickAgent's page, through the run setup's steps, the install review's
// consents and the first-run wizard's first step. Every Tab lands on a
// control, never on the page itself, and a dialog keeps the focus inside it.
import { expect, test, type Page } from "@playwright/test";

/** What has the focus: its tag, its text or name, and whether it is the
 *  page itself or inside a dialog. */
async function focused(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    return {
      body: !el || el === document.body || el === document.documentElement,
      tag: el?.tagName.toLowerCase() ?? "",
      text: (el?.getAttribute("aria-label") || el?.textContent || "").replace(/\s+/g, " ").trim(),
      className: el?.className?.toString() ?? "",
      inDialog: !!el?.closest('[role="dialog"]'),
      checked: el instanceof HTMLInputElement ? el.checked : null,
    };
  });
}

/** Presses Tab until `want` holds for the focused element, failing if the
 *  focus reaches the page itself or `max` presses go by. */
async function tabTo(page: Page, want: (f: Awaited<ReturnType<typeof focused>>) => boolean, max = 40) {
  for (let i = 0; i < max; i++) {
    await page.keyboard.press("Tab");
    const f = await focused(page);
    expect(f.body, "the focus fell to the page itself").toBe(false);
    if (want(f)) return f;
  }
  throw new Error(`no control matched within ${max} Tabs`);
}

test("Browse: from the search to a QuickAgent's page with the keyboard", async ({ page }) => {
  await page.goto("/pages.html");
  await page.locator('[data-preview="browse"] input[type="search"]').focus();
  const row = await tabTo(page, (f) => f.className.includes("agent-row-open"));
  expect(row.tag).toBe("button");
  await page.keyboard.press("Enter");
  await expect(page.locator("body")).toHaveAttribute("data-open", /^agent:/);
});

test("the run setup: the dialog takes the focus, Next moves on with Enter, and Tab never leaves it", async ({ page }) => {
  await page.goto("/modals.html#runpanel");
  await page.waitForSelector('[role="dialog"]');
  expect((await focused(page)).inDialog).toBe(true);
  const heading = page.locator(".run-step-heading");
  const before = await heading.textContent();
  await tabTo(page, (f) => f.tag === "button" && f.text.startsWith("Next"));
  await page.keyboard.press("Enter");
  await expect(heading).not.toHaveText(before ?? "");
  for (let i = 0; i < 60; i++) {
    await page.keyboard.press("Tab");
    const f = await focused(page);
    expect(f.body).toBe(false);
    expect(f.inDialog, `Tab left the dialog at "${f.text}"`).toBe(true);
  }
});

test("the install review: Verify & review with Enter, each consent checked with Space, Install then reachable, and Shift+Tab stays inside", async ({ page }) => {
  await page.goto("/modals.html#install-unscanned");
  await page.waitForSelector('[role="dialog"]');
  await tabTo(page, (f) => f.tag === "button" && /^Verify/.test(f.text));
  await page.keyboard.press("Enter");
  const consent = page.locator('[role="dialog"] input[type="checkbox"]').first();
  await expect(consent).toBeVisible();
  // Each consent the release asks for, ticked from the keyboard.
  const boxes = page.locator('[role="dialog"] input[type="checkbox"]');
  const count = await boxes.count();
  for (let i = 0; i < count; i++) {
    await tabTo(page, (f) => f.tag === "input" && f.inDialog && f.checked === false);
    await page.keyboard.press("Space");
    expect(await page.evaluate(() => (document.activeElement as HTMLInputElement).checked)).toBe(true);
  }
  await expect(page.getByRole("button", { name: "Install", exact: true })).toBeEnabled();
  for (let i = 0; i < 40; i++) {
    await page.keyboard.press("Shift+Tab");
    const f = await focused(page);
    expect(f.body).toBe(false);
    expect(f.inDialog, `Shift+Tab left the dialog at "${f.text}"`).toBe(true);
  }
});

test("the first-run wizard: Set up moves to the next step with the keyboard, and its steps are read as text", async ({ page }) => {
  await page.goto("/wizard.html#0");
  await page.waitForSelector('[role="dialog"]');
  await expect(page.locator('ol[aria-label="Setup steps"] [aria-current="step"]')).toHaveText(/^Step 1 of \d+: /);
  await tabTo(page, (f) => f.tag === "button" && /^Set up/.test(f.text));
  await page.keyboard.press("Enter");
  await expect(page.locator('ol[aria-label="Setup steps"] [aria-current="step"]')).toHaveText(/^Step 2 of \d+: /);
});

test("the deliverables menu: Shift+F10 opens it, the arrows and End stay on its items, Escape gives the focus back, Tab closes it", async ({ page }) => {
  await page.goto("/pages.html");
  const icon = page.locator('[data-preview="home"] [data-deliverable]').first();
  await icon.focus();
  await page.keyboard.press("Shift+F10");
  const menu = page.locator('[data-preview="home"] .deliverable-menu[role="menu"]');
  await expect(menu).toBeVisible();
  const onItem = () => page.evaluate(() => document.activeElement?.getAttribute("role"));
  expect(await onItem()).toBe("menuitem");
  for (const k of ["ArrowDown", "ArrowUp", "End", "Home"]) {
    await page.keyboard.press(k);
    expect(await onItem(), `${k} left the menu`).toBe("menuitem");
  }
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(icon).toBeFocused();
  await page.keyboard.press("Shift+F10");
  await expect(menu).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(menu).toHaveCount(0);
});

test("an agent's conversation is a log that does not announce itself, beside the status line that says each reply", async ({ page }) => {
  await page.goto("/pages.html");
  for (const sel of ['[data-preview="persistent"] .agent-term-scroll', '[data-preview="home"] .main-card-scroll']) {
    const log = page.locator(sel).first();
    await expect(log).toHaveAttribute("role", "log");
    await expect(log).toHaveAttribute("aria-live", "off");
    await expect(log.locator("xpath=following-sibling::*[@data-chat-said][1]")).toHaveAttribute("role", "status");
  }
});

// 200% of the smallest window the app opens (900 by 600) is 450 by 300 CSS
// pixels: a dialog fits it without a sideways scroll.
test.describe("dialogs at 200%", () => {
  test.use({ viewport: { width: 450, height: 300 } });
  for (const url of ["/modals.html#runpanel", "/modals.html#install-unscanned", "/modals.html#bugreport", "/wizard.html#1"]) {
    test(`${url} reflows without a sideways scroll`, async ({ page }) => {
      await page.goto(url);
      await page.waitForSelector('[role="dialog"]');
      const wide = await page.evaluate(() => [...document.querySelectorAll('[role="dialog"], [role="dialog"] *')]
        .filter((e) => e.scrollWidth > e.clientWidth + 2 && ["auto", "scroll"].includes(getComputedStyle(e).overflowX)).length);
      expect(wide).toBe(0);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    });
  }
});
