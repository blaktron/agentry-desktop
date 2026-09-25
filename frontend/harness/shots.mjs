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
  { name: "runpanel-modes", url: "/modals.html#runpanel", note: "Before you start: what it does, what to have ready, where to run it (In Agentry beside the local modes)", expectText: "What to have ready" },
  { name: "runpanel-in-agentry", url: "/modals.html#runpanel-hosted", note: "In Agentry chosen: the hold, the balance, the caps" },
  { name: "runpanel-unavailable", url: "/modals.html#runpanel-unavailable", note: "offered, unavailable, with the reason and the remedy" },
  { name: "runpanel-queued", url: "/modals.html#runpanel-queued", note: "queued with its position" },
  { name: "runpanel-running", url: "/modals.html#runpanel-running", note: "running: its state, no log (agentry-desktop#182)" },
  { name: "runpanel-finished", url: "/modals.html#runpanel-finished", note: "done — pass: the result first, the files in the run folder with previews, Run details collapsed", expectText: "Files in the run folder" },
  { name: "runpanel-hostile", url: "/modals.html#runpanel-hostile", note: "hostile output: no markup of its own, no image, no javascript: link, the hidden characters shown", expectText: "U+202E", expectNone: ".run-result-view img, .run-result-view script, .run-result-view iframe, .run-result-view b[onclick], .run-result-view a[href^='javascript']" },
  { name: "runpanel-failed", url: "/modals.html#runpanel-failed", note: "done — fail: the outcome's line, credits settled; a run the registry kept no receipt for" },
  { name: "runpanel-unscanned", url: "/modals.html#runpanel-unscanned", note: "a release admitted without a scan: its own mark and consent, In Agentry not offered (M23); the line above Run names the consent (agentry-desktop#167)", expectBlocker: "Tick \u201cRun an unscanned release\u201d", expectRunDisabled: true },
  // The form asks only for what the release declares (agentry-desktop#167):
  // the keys line names the server and its secret with Run grey; a file
  // intake question shows as the form; Input files stays a disclosure.
  { name: "runpanel-keys", url: "/modals.html#runpanel-keys", note: "pr-review-brief's shape, on Review and start: keys unsaved, the line above Run names them", expectBlocker: "Save the keys for github-mcp-server-js: GITHUB_TOKEN", expectRunDisabled: true },
  { name: "runpanel-keys-saved", url: "/modals.html#runpanel-keys-saved", note: "Connect tools: the key saved, shown by name only", click: 'button:has-text("Save keys")', expectText: "Stored for this exact release: GITHUB_TOKEN" },
  { name: "runpanel-file-intake", url: "/modals.html#runpanel-file-intake", note: "a release that asks for a file up front: on Review and start the line names the unanswered question", expectBlocker: "The incident channel's export (a file)", expectRunDisabled: true },
  // The guided run setup (agentry-desktop#226): one state per step.
  { name: "runpanel-step-files", url: "/modals.html#runpanel-step-files", note: "Files: no file chosen yet, Add files and Add folder", expectText: "Step 2 of" },
  { name: "runpanel-step-tools", url: "/modals.html#runpanel-step-tools", note: "Connect tools: the declared MCP server's pin and tools, then its key", expectText: "Step 3 of 5: Connect tools" },
  { name: "runpanel-step-details", url: "/modals.html#runpanel-step-details", note: "Details: the intake question, then the task", expectText: ": Details" },
  { name: "runpanel-step-review", url: "/modals.html#runpanel-step-review", note: "Review and start: the recap with Edit links, the output folder, Advanced, Run", expectText: "Review and start" },
  // The author's guide (agentry-desktop#228): each step that reads it, and a
  // hostile guide that must render as text on the two steps it reaches.
  { name: "runpanel-guided", url: "/modals.html#runpanel-guided", note: "Before you start with a guide: the author's overview, then the author's What to have ready", expectText: "review brief" },
  { name: "runpanel-guided-files", url: "/modals.html#runpanel-guided-files", note: "Files: the author's line on what to add", expectText: "None needed: it reads the pull request from GitHub." },
  { name: "runpanel-guided-tools", url: "/modals.html#runpanel-guided-tools", note: "Connect tools: the key's help under its field, the link opened in the system browser", expectText: "A fine-grained token with read access to pull requests", expectLinks: ["https://github.com/settings/personal-access-tokens/new"] },
  { name: "runpanel-guided-details", url: "/modals.html#runpanel-guided-details", note: "Details: the author's task placeholder", expectPlaceholder: "e.g. https://github.com/acme/api/pull/482, and look hardest at the migration" },
  { name: "runpanel-guided-hostile", url: "/modals.html#runpanel-guided-hostile", note: "a hostile guide on Before you start: no markup of its own, no image, no javascript: link, the hidden character shown", expectText: "U+200B", expectLinks: ["https://example.com/docs"], expectNone: ".run-step img, .run-step script, .run-step iframe, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  { name: "runpanel-guided-hostile-tools", url: "/modals.html#runpanel-guided-hostile-tools", note: "the hostile guide's key help on Connect tools: text; the https link; a URL with a user name left as text; a look-alike host linked as its punycode; the override shown; a key named constructor finds no help", expectText: "U+202E", expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new"], expectNone: ".run-step img, .run-step script, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  // A run on this machine, finished (agentry-desktop#269): Run details
  // name the operator policy the run enforced and how its path was chosen.
  { name: "runpanel-local-finished", url: "/modals.html#runpanel-local-finished", note: "a local run finished: Run details name the operator policy, the runner home's default, and its digest abbreviated", clicks: ['button:text-is("Run")', "details.run-details > summary"], expectText: "/home/chris/.agentry/runner/policy.yaml" },
  { name: "runpanel-local-finished-env", url: "/modals.html#runpanel-local-finished-env", note: "the same run with the runner home relocated by AGENTRY_RUNNER_HOME: the source says so", clicks: ['button:text-is("Run")', "details.run-details > summary"], expectText: "AGENTRY_RUNNER_HOME, set in the environment" },
  { name: "agent-mcp-env", url: "/pages.html#agent-mcp-env", note: "the agent page's MCP tab: a declared server's plain env beneath its line, a server without env unchanged (agentry-desktop#62)", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button.tab:has-text("MCP servers")' },
  { name: "install-mcp-env", url: "/modals.html#install-mcp-env", note: "the install review: each declared server's pin, tools and plain env (agentry-desktop#62)", click: "button:has-text('Verify')" },
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
  { name: "installed-update-warning", url: "/pages.html#installed-update-warning", note: "a runner-home Update that completes with a warning: the engine's sentence on Installed", locator: '[data-preview="installed"]', click: '[data-preview="installed"] button:text-is("Update")', expectText: "agentry/changelog-from-git: operator policy" },
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
  // Settings → Coding agents (agentry-desktop#148, D6): the coding agent,
  // the export it implies, its folder, the switch off by default; and
  // the agent page's one-off link, named for the coding agent, opening
  // the wizard on that export.
  { name: "settings-coding-agents", url: "/pages.html", note: "Settings → Coding agents: Claude Code chosen, the Claude skill in ~/.claude/skills, the switch off", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-codex", url: "/pages.html#coding-codex", note: "Codex chosen: AGENTS.md in the chosen project folder, the switch on", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-none", url: "/pages.html#coding-none", note: "nothing chosen, no folder: AGENTS.md needs a project, the switch cannot go on", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectDisabledSwitch: true },
  { name: "settings-coding-agents-both", url: "/pages.html#coding-both", note: "both chosen: the Claude skill is what is written, so the switch names Claude Code alone", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectSwitchLabel: "Also install into Claude Code" },
  { name: "agent-install-coding-agent", url: "/pages.html", note: "the agent page's one-off link opens the wizard preset to the Claude skill", locator: ".modal", click: '[data-preview="agent"] button:has-text("Install into Claude Code")' },
  // The Welcome's Coding agent step, on the same list Settings uses
  // (agentry-desktop#148 moved it): unchanged for the person.
  { name: "wizard-coding-agent", url: "/wizard.html#3", note: "the welcome's coding agent step after Look: Claude Code found and chosen, Codex missing", locator: ".wizard" },
  { name: "wizard-coding-agent-unprobed", url: "/wizard.html#3-unprobed", note: "the step as it opens: what the policy records, nothing run", locator: ".wizard" },
  // Every page under the shell and every step of the first-run wizard, so a
  // change to the token table (agentry-desktop#49) is seen everywhere it
  // lands, not only in the states above.
  { name: "page-browse", url: "/pages.html", note: "Browse under the shell", locator: '[data-preview="browse"]' },
  { name: "page-agent", url: "/pages.html", note: "an agent's page under the shell", locator: '[data-preview="agent"]' },
  { name: "page-installed", url: "/pages.html", note: "Installed under the shell", locator: '[data-preview="installed"]' },
  { name: "page-build", url: "/pages.html", note: "Build under the shell", locator: '[data-preview="build"]' },
  { name: "page-validate", url: "/pages.html", note: "Validate under the shell", locator: '[data-preview="validate"]' },
  { name: "page-settings", url: "/pages.html", note: "Settings under the shell", locator: '[data-preview="settings"]' },
  { name: "toolspanel", url: "/modals.html#toolspanel", note: "the tools panel over the shell" },
  { name: "chat", url: "/modals.html#chat", note: "a session's chat pane", locator: ".chat-pane" },
  { name: "wizard-welcome", url: "/wizard.html#0", note: "the first-run wizard: welcome", locator: ".wizard" },
  { name: "wizard-signin", url: "/wizard.html#1", note: "the first-run wizard: sign-in", locator: ".wizard" },
  { name: "wizard-done", url: "/wizard.html#4", note: "the first-run wizard: done", locator: ".wizard" },
  { name: "wizard-done-nothing", url: "/wizard.html#4-nothing", note: "the first-run wizard: every step skipped, the Nothing set up ending", locator: ".wizard" },
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
      // A state behind a click — a tab, a wizard's next step — or a few
      // (Run, then Run details) is reached the way a person reaches it.
      for (const sel of state.clicks ?? (state.click ? [state.click] : [])) {
        await page.locator(sel).first().click();
        await page.waitForTimeout(400);
      }
      if (state.expectText) {
        const text = await page.locator(state.locator ?? "body").innerText();
        if (!text.includes(state.expectText)) throw new Error(`${state.name}: the page does not show "${state.expectText}"`);
      }
      if (state.expectNone) {
        // Agent output makes no markup of its own (agentry-desktop#227).
        const n = await page.locator(state.expectNone).count();
        if (n !== 0) throw new Error(`${state.name}: ${n} element(s) matching ${state.expectNone}`);
      }
      if (state.expectPlaceholder) {
        // The task field's placeholder is the author's (agentry-desktop#228).
        const ph = await page.locator(".run-step textarea").first().getAttribute("placeholder");
        if (ph !== state.expectPlaceholder) throw new Error(`${state.name}: the task placeholder reads "${ph}"`);
      }
      if (state.expectLinks) {
        // Exactly these links, and nothing else a click could follow.
        const hrefs = await page.locator(".run-step a[href]").evaluateAll((as) => as.map((a) => a.getAttribute("href")));
        if (JSON.stringify(hrefs) !== JSON.stringify(state.expectLinks)) throw new Error(`${state.name}: links ${JSON.stringify(hrefs)}`);
      }
      if (state.expectBlocker) {
        // The line above Run is the same condition the button reads.
        const line = await page.locator(".run-blockers").innerText();
        if (!line.includes(state.expectBlocker)) throw new Error(`${state.name}: the line above Run reads "${line}"`);
        if (state.expectRunDisabled) {
          const run = page.getByRole("button", { name: /^Run$|^Start session$/ }).first();
          if (await run.isEnabled()) throw new Error(`${state.name}: Run is enabled with a blocker showing`);
        }
      }
      if (state.expectNoBlocker) {
        if (await page.locator(".run-blockers").count() !== 0) throw new Error(`${state.name}: a blocker line is showing`);
        if (state.expectRunEnabled && !(await page.getByRole("button", { name: /^Run$/ }).first().isEnabled())) throw new Error(`${state.name}: Run is disabled with nothing missing`);
      }
      if (state.expectCollapsed) {
        if (await page.locator(state.expectCollapsed).first().getAttribute("open") !== null) throw new Error(`${state.name}: ${state.expectCollapsed} is open with nothing chosen`);
      }
      if (state.expectSwitchLabel) {
        const label = await page.locator(state.locator).locator('label:has(input[type="checkbox"])').last().innerText();
        if (label.trim() !== state.expectSwitchLabel) throw new Error(`${state.name}: the switch reads "${label.trim()}"`);
      }
      if (state.expectDisabledSwitch) {
        // The switch cannot go on while AGENTS.md has nowhere to go: the
        // script says so, not only the shot.
        const box = page.locator(state.locator).locator('input[type="checkbox"]').last();
        if (await box.isEnabled()) throw new Error(`${state.name}: the switch is enabled with no project folder`);
      }
      if (state.name === "agent-install-coding-agent") {
        const checked = await page.locator('.modal input[type="radio"]:checked').getAttribute("value").catch(() => null);
        const label = await page.locator('.modal label.check-row:has(input:checked)').innerText();
        if (!label.includes("Claude skill")) throw new Error(`${state.name}: the wizard did not open on the Claude skill (${label}, ${checked})`);
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
