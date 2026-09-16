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
      const file = path.join(outDir, `${state.name}-${scheme}.png`);
      const modal = page.locator(".modal").first();
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
