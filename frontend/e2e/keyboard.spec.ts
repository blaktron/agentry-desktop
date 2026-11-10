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
  const heading = page.locator(".run-step-heading, .step-heading").first();
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
