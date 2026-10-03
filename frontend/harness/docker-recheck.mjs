// Real RuntimeSetup/PrereqCard components against the harness's Wails
// boundary. Start `npx vite harness --port 5199` first; PLAYWRIGHT_MODULE
// points to the estate's installed Playwright, as for shots.mjs.
import assert from "node:assert/strict";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE);
const browser = await chromium.launch();
const page = await browser.newPage({ colorScheme: process.env.SCHEME || "light" });
const base = process.env.HARNESS_URL || "http://localhost:5199";
try {
  await page.goto(`${base}/pages.html?settings=provider#docker-manual`);
  const provider = page.locator('[data-preview="settings"]');
  await provider.getByRole("button", { name: "Check again", exact: true }).waitFor();
  const field = (name, control) => provider.locator("label.field").filter({ has: page.locator(".field-label").filter({ hasText: new RegExp(`^${name}$`) }) }).locator(control);
  await field("Provider", "select").selectOption("custom");
  await field("Endpoint URL", "input").fill("https://unsaved.example/v1");
  await field("API key", "input").fill("unsaved-test-key");
  await page.evaluate(() => {
    const previous = window.go.main.RunAPI.SetupStatus;
    window.dockerChecks = 0;
    window.go.main.RunAPI.SetupStatus = async () => {
      window.dockerChecks++;
      await new Promise((resolve) => { window.finishDockerCheck = resolve; });
      return { ...(await previous()), dockerOk: true, imagesReady: true };
    };
  });
  await provider.getByRole("button", { name: "Check again", exact: true }).click();
  await provider.getByText("Checking Docker…", { exact: true }).waitFor();
  await page.evaluate(() => window.finishDockerCheck());
  await provider.getByText("docker available", { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.dockerChecks), 1);
  assert.equal(await field("Provider", "select").inputValue(), "custom");
  assert.equal(await field("Endpoint URL", "input").inputValue(), "https://unsaved.example/v1");
  assert.equal(await field("API key", "input").inputValue(), "unsaved-test-key");

  await page.goto(`${base}/pages.html?settings=run-defaults#docker-manual`);
  const defaults = page.locator('[data-preview="settings"]');
  await defaults.getByRole("button", { name: "Check again", exact: true }).waitFor();
  await page.evaluate(() => {
    window.go.main.RunAPI.PrereqStatus = async () => {
      await new Promise((resolve) => { window.finishDockerCheck = resolve; });
      return { state: "ready", dockerOk: true };
    };
  });
  await defaults.getByRole("button", { name: "Check again", exact: true }).click();
  await defaults.getByText("Checking Docker…", { exact: true }).waitFor();
  await page.evaluate(() => window.finishDockerCheck());
  await defaults.getByRole("button", { name: "Check again", exact: true }).waitFor({ state: "hidden" });
  await page.goto(`${base}/wizard.html#2-waking`);
  await page.getByText("docker available", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Prepare images now", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Prepare images now", exact: true }).isEnabled(), true);
  console.log("Docker re-check clears stale failure and preserves provider edits; prerequisites recover.");
} finally {
  await browser.close();
}
