// Real RuntimeSetup/PrereqCard components against the harness's Wails
// boundary: the container engine's explicit re-check clears a stale
// failure without replacing unsaved provider edits, and the prerequisites
// card recovers. Start `npx vite harness --port 5199` first; PLAYWRIGHT_MODULE
// points to an installed Playwright, as for shots.mjs.
import assert from "node:assert/strict";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE);
const browser = await chromium.launch();
const page = await browser.newPage({ colorScheme: process.env.SCHEME || "light" });
const base = process.env.HARNESS_URL || "http://localhost:5199";
try {
  // Provider settings with the engine refused: the provider is one tile of
  // a radio group (ProviderTiles), the endpoint and key fields plain
  // labelled inputs under it.
  await page.goto(`${base}/pages.html?settings=provider#docker-manual`);
  const provider = page.locator('[data-preview="settings"]');
  await provider.getByRole("button", { name: "Check again", exact: true }).waitFor();
  const tile = (id) => provider.locator(`.provider-tiles [role="radio"][data-provider="${id}"]`);
  const field = (name) => provider.locator("label.field").filter({ has: page.locator(".field-label").filter({ hasText: new RegExp(`^${name}$`) }) }).locator("input");
  await tile("custom").click();
  assert.equal(await tile("custom").getAttribute("aria-checked"), "true");
  await field("Endpoint URL").fill("https://unsaved.example/v1");
  await field("API key").fill("unsaved-test-key");
  await page.evaluate(() => {
    const previous = window.go.app.RunAPI.SetupStatus;
    window.dockerChecks = 0;
    window.go.app.RunAPI.SetupStatus = async () => {
      window.dockerChecks++;
      await new Promise((resolve) => { window.finishDockerCheck = resolve; });
      return { ...(await previous()), dockerOk: true, imagesReady: true };
    };
  });
  await provider.getByRole("button", { name: "Check again", exact: true }).click();
  await provider.getByText("Checking the container engine…", { exact: true }).waitFor();
  await page.evaluate(() => window.finishDockerCheck());
  await provider.getByText("docker 29.7.2 available", { exact: true }).waitFor();
  assert.equal(await page.evaluate(() => window.dockerChecks), 1);
  assert.equal(await tile("custom").getAttribute("aria-checked"), "true");
  assert.equal(await field("Endpoint URL").inputValue(), "https://unsaved.example/v1");
  assert.equal(await field("API key").inputValue(), "unsaved-test-key");

  // Run defaults with the engine refused: the prerequisites card's own
  // Re-detect runs the whole detection again and, once the engine answers,
  // the card gives way to the engine found.
  await page.goto(`${base}/pages.html?settings=run-defaults#docker-manual`);
  const defaults = page.locator('[data-preview="settings"]');
  const prereq = defaults.locator(".card", { hasText: "Cannot connect to the Docker daemon" });
  await prereq.getByRole("button", { name: "Re-detect", exact: true }).waitFor();
  await page.evaluate(() => {
    const ready = { found: true, kind: "docker", cli: "docker", version: "29.7.2", endpoint: "unix:///var/run/docker.sock", rootless: false, how: "the docker CLI's current context", summary: "docker 29.7.2, rootful, at unix:///var/run/docker.sock, found as the docker CLI's current context", err: "", tried: [], warnings: [] };
    const previous = window.go.app.RunAPI.SetupStatus;
    window.go.app.RunAPI.RedetectEngine = async () => {
      await new Promise((resolve) => { window.finishDockerCheck = resolve; });
      return ready;
    };
    window.go.app.RunAPI.PrereqStatus = async () => ({ state: "ready", dockerOk: true, engine: ready, warnings: [], os: "linux" });
    window.go.app.RunAPI.SetupStatus = async () => ({ ...(await previous()), dockerOk: true, dockerErr: "", engine: ready });
  });
  await prereq.getByRole("button", { name: "Re-detect", exact: true }).click();
  await defaults.getByText("Checking the container engine…", { exact: true }).waitFor();
  await page.evaluate(() => window.finishDockerCheck());
  await defaults.locator("[data-engine-line]").waitFor();
  await defaults.getByText("Cannot connect to the Docker daemon", { exact: false }).waitFor({ state: "hidden" });

  // The wizard's engine step: a waking engine answers on the second check,
  // and preparing the images is offered.
  await page.goto(`${base}/wizard.html#3-waking`);
  await page.getByText("docker 29.7.2 available", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Prepare images now", exact: true }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Prepare images now", exact: true }).isEnabled(), true);
  console.log("The engine re-check clears a stale failure and preserves provider edits; prerequisites recover.");
} finally {
  await browser.close();
}
