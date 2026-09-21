// Screenshots of the run surfaces from the design harness, light and dark
// (agentry-desktop#77). It starts the harness itself, shoots each state
// into harness/shots/, and stops it again, so the images in the repo can
// be re-made by one command rather than by hand.
//
//   cd frontend && node harness/shots.mjs
//
// Playwright is not a dependency of this app — the harness is dev-only and
// CI never shoots it. Point PLAYWRIGHT_MODULE at an installation that has
// it (the site's checkout has one):
//
//   PLAYWRIGHT_MODULE=../../agentry-web/node_modules/playwright/index.mjs \
//     node harness/shots.mjs
//
// STATES limits the run to a comma-separated subset of the names below.
import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.join(here, "shots");
const port = Number(process.env.HARNESS_PORT ?? 5199);
const base = `http://localhost:${port}`;

// One entry per state the panel can be in, as the issue lists them.
const states = [
  { name: "runpanel-modes", url: "/modals.html#runpanel", note: "the mode choice, In Agentry beside the local modes" },
  { name: "runpanel-in-agentry", url: "/modals.html#runpanel-hosted", note: "In Agentry chosen: the hold, the balance, the caps" },
  { name: "runpanel-unavailable", url: "/modals.html#runpanel-unavailable", note: "offered, unavailable, with the reason and the remedy" },
  { name: "runpanel-queued", url: "/modals.html#runpanel-queued", note: "queued with its position" },
  { name: "runpanel-streaming", url: "/modals.html#runpanel-streaming", note: "streaming the log" },
  { name: "runpanel-finished", url: "/modals.html#runpanel-finished", note: "finished: outcome, receipt, downloaded artefacts" },
  { name: "runpanel-unscanned", url: "/modals.html#runpanel-unscanned", note: "a release admitted without a scan: its own mark and consent, In Agentry not offered (M23)" },
  { name: "install-unscanned", url: "/modals.html#install-unscanned", note: "the install wizard's review of a release admitted without a scan: the mark and the consent (M23)", click: "button:has-text('Verify')" },
  { name: "agent-unscanned", url: "/pages.html#agent-unscanned", note: "the agent page's provenance tab: the unscanned badge and the mark on the card (M23)", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button.tab:has-text("provenance")' },
  { name: "agent-unscanned-report", url: "/pages.html#agent-unscanned", note: "the agent page's report tab: unscanned as its own verdict, never 'scanned, 0 findings' (M23)", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button.tab:has-text("security report")' },
  // The agent page's one-click Install (agentry-desktop#146, D-031): the
  // four states of the one button, and the wizard behind its link.
  { name: "agent-install", url: "/pages.html", note: "not installed: Install <latest> in one click", locator: '[data-preview="agent"]' },
  { name: "agent-installing", url: "/pages.html#agent-installing", note: "installing: the button held", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button:has-text("Install 1.2.0")' },
  { name: "agent-installed", url: "/pages.html#agent-installed", note: "installed at the latest: Update, disabled", locator: '[data-preview="agent"]' },
  { name: "agent-installed-older", url: "/pages.html#agent-installed-older", note: "an older version installed: Update to <latest>", locator: '[data-preview="agent"]' },
  { name: "agent-install-refused", url: "/pages.html#agent-install-refused", note: "a declined gate: the refusal in place, the button back to Install", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button:has-text("Install 1.2.0")' },
  // The checks (agentry-desktop#147): the registry unreachable on opening
  // the page; the sidebar's number after the launch check; the switch.
  { name: "agent-installed-unreachable", url: "/pages.html#agent-installed-unreachable", note: "installed, the registry could not be asked: Update disabled, the reason in the page", locator: '[data-preview="agent"]' },
  { name: "installed-badge", url: "/pages.html#installed-updates", note: "the launch check found newer versions: the number on Installed", locator: '[data-preview="installed"] .sidebar' },
  { name: "settings-briefagent-updates", url: "/pages.html", note: "Settings: the auto-update switch, shown and disabled, with why", locator: '[data-preview="settings"] .card:has-text("Update BriefAgents automatically")' },
  // The welcome's runtime step (agentry-desktop#131): the three questions;
  // the model list Get models filled; the failed listing with the model
  // still typeable; a stored subscription token and its sentence.
  { name: "wizard-runtime", url: "/wizard.html#2", note: "the runtime step: provider, key, model", locator: ".wizard" },
  { name: "wizard-runtime-fresh", url: "/wizard.html#2-nothing", note: "a fresh machine: no policy, nothing stored — the key field is there", locator: ".wizard" },
  { name: "wizard-runtime-models", url: "/wizard.html#2-models", note: "after Get models: the endpoint's list", locator: ".wizard", getModels: true },
  { name: "wizard-runtime-nomodels", url: "/wizard.html#2-nomodels", note: "Get models failed: the answer shown, the model typeable", locator: ".wizard", getModels: true },
  { name: "wizard-runtime-subscription", url: "/wizard.html#2-subscription", note: "a stored Anthropic subscription token: what it drives", locator: ".wizard" },
  // The save refused after the key was stored (agentry-desktop#141): the
  // custom provider, a URL the backend refuses — the refusal shown, the
  // key field saying the key is on this machine, Save still offered.
  { name: "wizard-runtime-refused", url: "/wizard.html#2-refused", note: "a refused save after the key was stored: the key stays, no re-typing", locator: ".wizard", saveCustom: true },
  // The image pull (agentry-desktop#145): the step holds its Continue and
  // Back while it runs, and releases them when it ends or is refused.
  { name: "wizard-runtime-preparing", url: "/wizard.html#2-preparing", note: "images preparing: Continue and Back held until the pull ends", locator: ".wizard", prepare: "held" },
  { name: "wizard-runtime-prepared", url: "/wizard.html#2-prepared", note: "images prepared: Continue and Back released", locator: ".wizard", prepare: "released" },
  { name: "wizard-runtime-prepare-refused", url: "/wizard.html#2-prepare-refused", note: "the pull refused: the reason shown, Continue and Back released", locator: ".wizard", prepare: "released" },
  { name: "settings-agent-runs-advanced", url: "/pages.html", note: "Settings → Agent runs with Advanced open: the variable and Forget", locator: '[data-preview="settings"] .card:has(details.advanced)', openAdvanced: true },
];

const wanted = process.env.STATES ? process.env.STATES.split(",").map((s) => s.trim()) : null;

async function loadPlaywright() {
  const mod = process.env.PLAYWRIGHT_MODULE ?? "playwright";
  try {
    return await import(mod.startsWith(".") ? path.resolve(process.cwd(), mod) : mod);
  } catch (err) {
    console.error(`playwright could not be loaded from ${mod}: ${err.message}`);
    console.error("Set PLAYWRIGHT_MODULE to an installation that has it.");
    process.exit(1);
  }
}

async function waitFor(url, tries = 100) {
  for (let i = 0; i < tries; i++) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`the harness never answered at ${url}`);
}

const { chromium } = await loadPlaywright();
await mkdir(outDir, { recursive: true });

const vite = spawn("npx", ["vite", "harness", "--port", String(port), "--strictPort"], {
  cwd: path.join(here, ".."),
  stdio: ["ignore", "pipe", "inherit"],
});
vite.stdout.on("data", () => {});

try {
  await waitFor(`${base}/modals.html`);
  const browser = await chromium.launch();
  for (const scheme of ["light", "dark"]) {
    const ctx = await browser.newContext({
      colorScheme: scheme,
      // Tall enough that the whole modal fits in its own box: .modal is
      // capped at the viewport height and scrolls inside, so a short
      // viewport would shoot only the top of the panel.
      viewport: { width: 1180, height: 2600 },
      deviceScaleFactor: 1.5,
    });
    for (const state of states) {
      if (wanted && !wanted.includes(state.name)) continue;
      // A fresh page per state: the states differ only by the hash the
      // harness reads at load, and a hash change alone does not reload.
      const page = await ctx.newPage();
      await page.goto(base + state.url, { waitUntil: "networkidle" });
      // The panel fills itself from the mocked bindings and the scripted
      // log; a beat is enough for both.
      await page.waitForTimeout(500);
      if (state.openAdvanced) {
        await page.locator("details.advanced > summary").first().click();
        await page.waitForTimeout(200);
      }
      if (state.click) {
        // A state behind one click — a tab, a wizard's next step — is
        // reached the way a person reaches it.
        await page.locator(state.click).first().click();
        await page.waitForTimeout(400);
      }
      if (state.saveCustom) {
        // The custom provider, its URL, a key and a model typed, then
        // Save: the mock stores the key and refuses the save.
        await page.locator("select").first().selectOption("custom");
        await page.locator('input[placeholder="https://llm.example.net/v1"]').fill("llm.example.net/v1");
        await page.locator('input[type="password"]').first().fill("sk-preview-key");
        await page.locator('input[placeholder^="pick one with Get models"]').fill("qwen3.8-flash");
        await page.getByRole("button", { name: "Save provider settings" }).click();
        await page.waitForTimeout(300);
      }
      if (state.prepare) {
        // Prepare images now, then a look at the foot: held while the
        // mock's pull runs, released once it ends or is refused. The
        // script says which, so the shot is not the only record.
        await page.getByRole("button", { name: "Prepare images now" }).click();
        await page.waitForTimeout(state.prepare === "held" ? 300 : 1200);
        const foot = await page.locator(".wizard-foot button").evaluateAll((bs) => bs.map((b) => `${b.textContent.trim()}:${b.disabled ? "disabled" : "enabled"}`));
        console.log(`  wizard-foot (${state.prepare}): ${foot.join(" ")}`);
        const want = state.prepare === "held" ? ":disabled" : ":enabled";
        if (!foot.every((f) => f.endsWith(want))) throw new Error(`a wizard-foot button is not ${want.slice(1)} in state ${state.name}`);
      }
      if (state.getModels) {
        // The listing needs a key typed and the button pressed, as a
        // person would; the mock answers by the hash.
        await page.locator('input[type="password"]').first().fill("sk-ant-api03-preview");
        await page.getByRole("button", { name: "Get models" }).click();
        await page.waitForTimeout(300);
      }
      const file = path.join(outDir, `${state.name}-${scheme}.png`);
      const modal = page.locator(state.locator ?? ".modal").first();
      await ((await modal.count()) ? modal : page).screenshot({ path: file });
      console.log(`${file}  — ${state.note}`);
      await page.close();
    }
    await ctx.close();
  }
  await browser.close();
} finally {
  vite.kill("SIGTERM");
}
