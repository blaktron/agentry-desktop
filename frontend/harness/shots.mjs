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
  { name: "runpanel-hosted-knowledge", url: "/modals.html#runpanel-hosted-knowledge", note: "the Knowledge step In Agentry: packs kept and scanned in Agentry can be ticked, one kept only here offers Upload to Agentry, one being scanned says so (agentry-desktop#275)", expectText: "Attach knowledge packs Agentry keeps for your account" },
  { name: "runpanel-hosted-secrets", url: "/modals.html#runpanel-hosted-secrets", note: "In Agentry for a release that declares PAGERDUTY_TOKEN: the start is refused secret-missing and the ask shows, Keep it for later runs unticked (agentry-desktop#300)", click: 'button:has-text("Run In Agentry")', expectText: "Secrets for this run" },
  { name: "runpanel-hosted-knowledge-finished", url: "/modals.html#runpanel-hosted-knowledge-finished", note: "a run In Agentry with a pack attached, done — pass: the receipt's knowledge line under the result (agentry-desktop#275)", expectText: "Knowledge this run could search" },
  { name: "runpanel-failed", url: "/modals.html#runpanel-failed", note: "done — fail: the outcome's line, credits settled; a run the registry kept no receipt for" },
  { name: "runpanel-unscanned", url: "/modals.html#runpanel-unscanned", note: "a release admitted without a scan: its own mark and consent, In Agentry not offered (M23); the line above Run names the consent (agentry-desktop#167)", expectBlocker: "Tick \u201cRun an unscanned release\u201d", expectRunDisabled: true },
  // The form asks only for what the release declares (agentry-desktop#167):
  // the keys line names the server and its secret with Run grey; a file
  // intake question shows as the form; Input files stays a disclosure.
  { name: "runpanel-keys", url: "/modals.html#runpanel-keys", note: "pr-review-brief's shape, on Review and start: keys unsaved, the line above Run names them", expectBlocker: "Save the keys for github-mcp-server-js: GITHUB_TOKEN", expectRunDisabled: true },
  { name: "runpanel-keys-saved", url: "/modals.html#runpanel-keys-saved", note: "Connect tools: the key saved, shown by name only", click: 'button:has-text("Save keys")', expectText: "Stored for this exact release: GITHUB_TOKEN" },
  { name: "runpanel-file-intake", url: "/modals.html#runpanel-file-intake", note: "a release that asks for a file up front: on Review and start the line names the unanswered question", expectBlocker: "The incident channel's export (a file)", expectRunDisabled: true },
  // Run defaults in the run setup (agentry-desktop#347): In Agentry chosen by
  // default where it is offered; a release that cannot run there starts here
  // and says why; the saved folder filled in on Review and start.
  { name: "runpanel-default-agentry", url: "/modals.html#runpanel-default-agentry", note: "where to run saved as In Agentry: the Where step opens with it chosen, the credits it holds shown", expectText: ["What to have ready", "This run holds 20 credits"], expectChecked: 'input[name="run-mode"] >> nth=-1' },
  { name: "runpanel-default-agentry-unscanned", url: "/modals.html#runpanel-default-agentry-unscanned", note: "the same default for a release admitted without a scan: it starts on this computer, and the step says why", expectText: "Your Run defaults say In Agentry, but a release admitted without a scan is not run In Agentry from this app, so this run starts on this computer." },
  { name: "runpanel-default-agentry-signedout", url: "/modals.html#runpanel-default-agentry-signedout", note: "the same default, signed out: it starts on this computer and names the sign-in", expectText: "a run In Agentry needs you signed in to this registry" },
  { name: "runpanel-default-folder", url: "/modals.html#runpanel-default-folder", note: "a saved output folder, filled in on Review and start; Choose… still picks another", expectText: "Review and start", expectValue: ['.modal input.mono[readonly]', "/home/me/agentry-runs"] },
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
  { name: "runpanel-guided-hostile-tools", url: "/modals.html#runpanel-guided-hostile-tools", note: "the hostile guide's key help on Connect tools: text; the https link; a URL with a user name left as text; a look-alike host linked as its punycode; the override shown; a key named constructor finds no help", expectText: ["U+202E", "The author's help for", "(evil.example)", "(github.co)", "(github.com.evil.example)"], expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new", "https://evil.example/steal", "https://github.co/", "https://github.com.evil.example/"], expectNone: ".run-step img, .run-step script, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  // A run on this machine, finished (agentry-desktop#269): Run details
  // name the operator policy the run enforced and how its path was chosen.
  { name: "runpanel-local-finished", url: "/modals.html#runpanel-local-finished", note: "a local run finished: Run details name the operator policy, the runner home's default, and its digest abbreviated", click: ['button:text-is("Run")', "details.run-details > summary"], expectText: "/home/chris/.agentry/runner/policy.yaml" },
  { name: "runpanel-local-finished-env", url: "/modals.html#runpanel-local-finished-env", note: "the same run with the runner home relocated by AGENTRY_RUNNER_HOME: the source says so", click: ['button:text-is("Run")', "details.run-details > summary"], expectText: "AGENTRY_RUNNER_HOME, set in the environment" },
  // Knowledge on a run on this machine (agentry-desktop#273): the step that
  // lists the packs kept here, and a finished run's knowledge line.
  { name: "runpanel-step-knowledge", url: "/modals.html#runpanel-step-knowledge", note: "the Knowledge step: the packs kept on this computer, each with its scan state and whether it opens offline, and one kept in Agentry only, not attachable here", expectText: "download it on the Knowledge page" },
  // A release that ships a pack and requires one of the person's own
  // (agentry-desktop#277): the ask, the shipped pack, Next held.
  { name: "runpanel-step-knowledge-ships", url: "/modals.html#runpanel-step-knowledge-ships", note: "the Knowledge step of a release that ships pagerduty-docs and requires a pack of the person's own: its ask, its shipped pack, and Next held while none is ticked", click: ['button:has-text("Next:")'], expectText: "This release needs a knowledge pack of your own" },
  { name: "install-ships-knowledge", url: "/modals.html#install-ships-knowledge", note: "the install review of a release that ships a pack: the contract's knowledge row with its size", click: "button:has-text('Verify')", expectText: "ships pagerduty-docs (4.2 MiB)" },
  { name: "runpanel-knowledge-finished", url: "/modals.html#runpanel-knowledge-finished", note: "a local run with a pack attached, finished: the receipt's knowledge line under the result", click: ['button:text-is("Run")'], expectText: "Knowledge this run could search" },
  // The Knowledge panel (agentry-desktop#273, plan knowledge.md §4.8): the
  // list in each state, Create knowledge's steps, a build under way and
  // done, a pack's preview, and a hostile page that must render as text.
  { name: "knowledge", url: "/knowledge.html#knowledge", note: "signed in: kept here (not scanned, opens offline), in both places (scanned in Agentry, 2 findings), changed since upload, in Agentry only, being scanned", locator: '[data-preview="knowledge"]', expectText: "Encrypted to your account's key at secureagentry.net, which Agentry holds" },
  { name: "knowledge-signed-out", url: "/knowledge.html#knowledge-signed-out", note: "not signed in: a pack under this computer's key, and why Agentry's copies are not listed", locator: '[data-preview="knowledge"]', expectText: "Encrypted to this computer's key" },
  { name: "knowledge-empty", url: "/knowledge.html#knowledge-empty", note: "no pack yet: Create knowledge", locator: '[data-preview="knowledge"]', expectText: "No knowledge packs yet" },
  { name: "knowledge-create-documents", url: "/knowledge.html#create-documents", note: "Create knowledge: the documents, from the native pickers only", expectText: "Step 1 of 3: Documents" },
  { name: "knowledge-create-name", url: "/knowledge.html#create-name", note: "Create knowledge: the name, and where a run finds it", expectText: "Step 2 of 3: Name" },
  { name: "knowledge-create-review", url: "/knowledge.html#create-review", note: "the review: before anything runs, that the Creator runs here against the policy's model and nothing it reads or writes is scanned, and that a build is a big job that may run for up to 30 minutes (agentry-desktop#352)", expectText: ["Nothing it reads or writes is scanned", "may run for up to 30 minutes and 60 turns"] },
  { name: "knowledge-create-blocked", url: "/knowledge.html#create-blocked", note: "docker not ready: the sentence, Create knowledge greyed", expectText: "Docker is not ready", expectDisabledButton: "Create knowledge" },
  { name: "knowledge-create-building", url: "/knowledge.html#create-building", note: "the build under way, driven by knowledge:build-progress events: time against its limit, turns, the pages written so far, the tokens; its log folded beneath (agentry-desktop#352)", expectText: ["Pages written", "14 of up to 60"] },
  { name: "knowledge-create-building-log", url: "/knowledge.html#create-building", note: "the build under way with its log opened, streaming", click: 'summary:has-text("Build log")', expectText: "decode: storage-guide.pdf" },
  { name: "knowledge-create-done", url: "/knowledge.html#create-done", note: "what the build made: pages, the key, what was not read, a dropped page, the tokens", expectText: "Made handbook: 48 pages" },
  { name: "knowledge-hosted-where", url: "/knowledge.html#hosted-where", note: "In Agentry beside this computer (agentry-desktop#274): the choice, and what it holds", expectText: "Step 3 of 4: Where" },
  { name: "knowledge-hosted-review", url: "/knowledge.html#hosted-review", note: "the review for a build In Agentry: where the documents go, the hold with the scan's credit inside it", expectText: "plus 1 credit to scan the pack" },
  { name: "knowledge-hosted-running", url: "/knowledge.html#hosted-running", note: "a build In Agentry under way: where it stands, the hold, Cancel", expectText: "100 credits are held for this build." },
  { name: "knowledge-hosted-done", url: "/knowledge.html#hosted-done", note: "a build In Agentry done: the pages, the settlement with the scan's credit, the pack kept here with its scan", expectText: "2 settled (1 of them for the pack's scan)" },
  { name: "knowledge-add-review", url: "/knowledge.html#add-review", note: "Add documents: the review, the same warnings", expectText: ["Step 2 of 2: Review", "may run for up to 30 minutes"] },
  { name: "knowledge-preview", url: "/knowledge.html#preview", note: "a pack's index and its pages, rendered by the run-output reader", expectText: "Rotating a key" },
  { name: "knowledge-preview-hostile", url: "/knowledge.html#preview-hostile", note: "a hostile page: no markup of its own, no image, no javascript: link, the right-to-left override shown", click: '.knowledge-pages button:has-text("Buckets")', expectText: "U+202E", expectNone: ".knowledge-page img, .knowledge-page script, .knowledge-page iframe, .knowledge-page b[onclick], .knowledge-page a[href^='javascript']" },
  // Two caps (agentry-desktop#354): 1 GB on this computer, 10 MB In Agentry.
  { name: "knowledge-create-documents-large", url: "/knowledge.html#create-documents-large", note: "48 MB of documents: what they come to, and the warning that the build can take a very long time and the pack stays on this computer", expectText: ["48.0 MB together", "can take a very long time", "stay on this computer"] },
  { name: "knowledge-create-documents-huge", url: "/knowledge.html#create-documents-huge", note: "documents past 1 GB: Next refused with the reason", click: 'button:has-text("Next:")', expectText: "is at most 1 GB" },
  { name: "knowledge-hosted-where-large", url: "/knowledge.html#hosted-where-large", note: "the Where step with 48 MB of documents: In Agentry not choosable, and why", expectText: "In Agentry takes at most 10 MB of documents" },
  { name: "knowledge-large-pack", url: "/knowledge.html#knowledge", note: "a pack of 220 MB kept here: no Upload, and the sentence why", locator: '[data-preview="knowledge"]', expectText: "Past the 10 MB Agentry keeps, so it stays on this computer" },
  // A running build in its corner card over the page (agentry-desktop#353).
  { name: "knowledge-card-building", url: "/knowledge.html#card-building", note: "a build on this computer shrunk to the corner card: its progress over the Knowledge page, which stays usable; Stop and Open", expectText: ["Creating handbook", "5 pages written", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "knowledge-card-done", url: "/knowledge.html#card-done", note: "the card once the build is over: what it made, Dismiss and Open", expectText: ["handbook: the build is over", "Made handbook: 48 pages", "Preview", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "knowledge-card-hosted", url: "/knowledge.html#card-hosted", note: "a build In Agentry in its card: the registry's status line and Cancel the build", expectText: ["In Agentry", "The build is running on a sandbox host.", "Cancel the build"], expectNone: ".modal-backdrop" },
  { name: "knowledge-create-building-background", url: "/knowledge.html#create-building", note: "the building view offers Stop the build and Run in the background", expectText: ["Stop the build", "Run in the background"] },
  // A pack a build left short, and a render fault's way back (agentry-desktop#351).
  { name: "knowledge-preview-empty", url: "/knowledge.html#preview-empty", note: "a pack with its index and no page, the list answered as null: the preview says no pages yet, never a blank window", expectText: "No pages yet" },
  { name: "render-fault", url: "/knowledge.html#render-fault", note: "a page that throws while rendering: what went wrong and Try again, under a sidebar that still works", expectText: "This page stopped with an error" },
  { name: "render-fault-recovered", url: "/knowledge.html#render-fault", note: "Try again after the fault clears: the page renders afresh", click: 'button:has-text("Try again")', locator: '[data-preview="knowledge"]', expectText: "Knowledge packs a run can search" },
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
  { name: "settings-secrets", url: "/pages.html?settings=registry", note: "Settings: Secrets for runs In Agentry — save a kept secret by name, delete one, the list on the registry's Secrets page (agentry-desktop#300)", locator: '[data-preview="settings"] .card:has-text("Secrets for runs In Agentry")', expectText: "neither this app nor Agentry shows it again" },
  { name: "settings-briefagent-updates", url: "/pages.html?settings=updates", note: "Settings: the auto-update switch, shown and disabled, with why", locator: '[data-preview="settings"] .card:has-text("Update BriefAgents automatically")' },
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
  { name: "settings-agent-runs-advanced", url: "/pages.html?settings=provider", note: "Settings → Provider settings with Advanced open: the variable and Forget", locator: '[data-preview="settings"] .card:has(details.advanced)', openAdvanced: true },
  { name: "settings-agent-runs-custom-http", url: "/pages.html?settings=provider#setup-custom-http", note: "Settings → Provider settings, a custom endpoint over plain http off this machine: saved, and the warning under the URL recommends https (agentry-desktop#317)", locator: '[data-preview="settings"] .card:has-text("Endpoint URL")', expectText: "uses plain http: the API key and the prompts travel unencrypted to gpu-box.lan" },
  // Settings → Coding agents (agentry-desktop#148, D6): the coding agent,
  // the export it implies, its folder, the switch off by default; and
  // the agent page's one-off link, named for the coding agent, opening
  // the wizard on that export.
  // Settings → AI Review key (#282): saved, never shown; none saved; and
  // Test key answered with TypeSafe's refusal in plain words.
  { name: "settings-aireview-key", url: "/pages.html?settings=provider", note: "Settings → AI Review key: saved in the keychain, the value never shown, Test key and Forget", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', expectText: "Saved in this computer's keychain." },
  { name: "settings-aireview-key-none", url: "/pages.html?settings=provider#aireview-none", note: "no AI Review key: drafting on this computer is off until one is added; Test key greyed", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', expectText: "Drafting on this computer is off" },
  { name: "settings-aireview-key-refused", url: "/pages.html?settings=provider#aireview-refused", note: "Test key answered 401: TypeSafe's refusal in plain words, and which key was tried", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', click: '[data-preview="settings"] button:has-text("Test key")', expectText: "TypeSafe refused the key (401)" },
  { name: "settings-coding-agents", url: "/pages.html?settings=coding-agents", note: "Settings → Coding agents: Claude Code chosen, the Claude skill in ~/.claude/skills, the switch off", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-codex", url: "/pages.html?settings=coding-agents#coding-codex", note: "Codex chosen: AGENTS.md in the chosen project folder, the switch on", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-none", url: "/pages.html?settings=coding-agents#coding-none", note: "nothing chosen, no folder: AGENTS.md needs a project, the switch cannot go on", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectDisabledSwitch: true },
  { name: "settings-coding-agents-both", url: "/pages.html?settings=coding-agents#coding-both", note: "both chosen: the Claude skill is what is written, so the switch names Claude Code alone", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectSwitchLabel: "Also install into Claude Code" },
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
  { name: "page-build", url: "/pages.html", note: "Build under the shell: the start page, Step by step and Advanced", locator: '[data-preview="build"]', expectText: "Step by step" },
  // The Build tab's views (plan builder-rework.md §5, agentry-desktop#280):
  // a step with its reach summary and the lines it writes, the preview of
  // what a person sees before a run, Review's recap, and Advanced.
  { name: "page-build-steps", url: "/pages.html#build-steps", note: "Step by step: Basics", locator: '[data-preview="build"]', expectText: "Step 1 of 8: Basics" },
  { name: "page-build-tools", url: "/pages.html#build-tools", note: "Step by step: Tools and web access, the reach summary, the server's configuration and the registry's check of it (never the word safe)", locator: '[data-preview="build"]', expectText: "No known advisories as of 2026-09-25 11:13 UTC." },
  { name: "page-build-instructions", url: "/pages.html#build-instructions", note: "Step by step: Instructions, with the size against the quota", locator: '[data-preview="build"]', expectText: "Step 2 of 8: Instructions" },
  { name: "page-build-access", url: "/pages.html#build-access", note: "Step by step: Files, commands and data", locator: '[data-preview="build"]', expectText: "Step 4 of 8: Files, commands and data" },
  { name: "page-build-questions", url: "/pages.html#build-questions", note: "Step by step: Questions", locator: '[data-preview="build"]', expectText: "Step 5 of 8: Questions" },
  { name: "page-build-publish", url: "/pages.html#build-publish", note: "Step by step: Publishing", locator: '[data-preview="build"]', expectText: "Step 7 of 8: Publishing" },
  { name: "page-build-validate", url: "/pages.html#build-validate", note: "Review after Save + Validate: the card names the V-rule problem and the finding's rule and place", locator: '[data-preview="build"]', click: '[data-preview="build"] button:has-text("Save + Validate")', expectText: "S4-prompt-injection-001" },
  { name: "page-build-guide", url: "/pages.html#build-guide", note: "Step by step: Before a run, with its preview", locator: '[data-preview="build"]', expectText: "What a person sees before a run" },
  { name: "page-build-guide-hostile", url: "/pages.html#build-guide-hostile", note: "Before a run's preview with a hostile key help (agentry-desktop#343): markdown, framed as the author's; no markup of its own, no image, no javascript: link; each link's host shown, a look-alike as punycode; a link cut at the hidden override", locator: '[data-preview="build"]', linksIn: '[data-preview="build"] .key-help', expectText: ["The author's help for", "U+202E", "(evil.example)", "(xn--pgerduty-16g.com)", "(pagerduty.com.evil.example)"], expectLinks: ["https://evil.example/pd", "https://support.pagerduty.com/keys", "https://xn--pgerduty-16g.com/new", "https://pagerduty.com.evil.example/"], expectNone: '[data-preview="build"] .key-help img, [data-preview="build"] .key-help script, [data-preview="build"] .key-help b[onclick], [data-preview="build"] .key-help a[href^="javascript"]' },
  { name: "page-build-review-hostile", url: "/pages.html#build-review-hostile", note: "Review with the same hostile key help beneath Before a run", locator: '[data-preview="build"]', linksIn: '[data-preview="build"] .key-help', expectText: ["The author's help for", "U+202E", "(evil.example)", "(pagerduty.com.evil.example)"], expectLinks: ["https://evil.example/pd", "https://support.pagerduty.com/keys", "https://xn--pgerduty-16g.com/new", "https://pagerduty.com.evil.example/"], expectNone: '[data-preview="build"] .key-help img, [data-preview="build"] .key-help script, [data-preview="build"] .key-help b[onclick], [data-preview="build"] .key-help a[href^="javascript"]' },
  { name: "page-build-review", url: "/pages.html#build-review", note: "Step by step: Review, the recap with Edit and the actions", locator: '[data-preview="build"]', expectText: "Step 8 of 8: Review" },
  { name: "page-build-advanced", url: "/pages.html#build-advanced", note: "Advanced: every section at once", locator: '[data-preview="build"]', expectText: "Before a run" },
  // Describe it, on this computer (#282): the description with the person's
  // AI Review key saved, and without one (the Settings pointer, Draft it
  // greyed); a draft on Review with the Drafter's notes; a hostile draft
  // whose notes, match, source and withheld address must stay text.
  { name: "page-build-describe", url: "/pages.html#build-describe", note: "Describe it: the description, the hints, on this computer with the policy's model, the key from the keychain, the search line and where the description goes", locator: '[data-preview="build"]', expectText: "AI Review uses your key from this computer's keychain" },
  { name: "page-build-describe-nokey", url: "/pages.html#build-describe-nokey", note: "Describe it with no AI Review key: the Settings pointer, and Draft it greyed", locator: '[data-preview="build"]', expectText: "needs your own TypeSafe Jev key" },
  { name: "page-build-drafted", url: "/pages.html#build-drafted", note: "a draft on Review: what AI Review read and held back, the registry's match with its page, the open questions, the Drafter's notes, the sources", locator: '[data-preview="build"]', expectText: "AI Review read 6 results with your key and held back 0" },
  { name: "page-build-drafted-hostile", url: "/pages.html#build-drafted-hostile", note: "a hostile draft on Review: no markup of its own, no image, no javascript: link, the withheld address as text, the override shown", locator: '[data-preview="build"]', expectText: "U+202E", expectNone: '[data-preview="build"] .card img, [data-preview="build"] .card script, [data-preview="build"] .card iframe, [data-preview="build"] .card b[onclick], [data-preview="build"] .card a[href^="javascript"], [data-preview="build"] .card a[href*="evil.example"]' },
  // Describe it In Agentry (#281): the hold in the site's words, the caps
  // and the balance; a payer short of the hold; a draft queued and
  // followed; signed out; a draft made In Agentry on Review, with Describe
  // again under the notes.
  { name: "page-build-describe-agentry", url: "/pages.html#build-describe-agentry", note: "Describe it In Agentry: the hold sentence, the balance, the caps, the honest line, On this computer beside it", locator: '[data-preview="build"]', expectText: "This draft holds 41 credits and settles from what it uses" },
  { name: "page-build-describe-agentry-short", url: "/pages.html#build-describe-agentry-short", note: "In Agentry for a payer short of the hold: the notice and Credits and upgrade", locator: '[data-preview="build"]', expectText: "fewer credits than a draft holds" },
  { name: "page-build-describe-agentry-drafting", url: "/pages.html#build-describe-agentry-drafting", note: "a draft In Agentry followed: queued, its place, the hold, Cancel", locator: '[data-preview="build"]', expectText: "41 credits are held for this draft." },
  { name: "page-build-describe-signedout", url: "/pages.html#build-describe-signedout", note: "In Agentry signed out: offered, and it needs an account", locator: '[data-preview="build"]', expectText: "needs an account" },
  { name: "page-build-drafted-agentry", url: "/pages.html#build-drafted-agentry", note: "a draft made In Agentry on Review: AI Review through the registry, a host the description did not name, a rule it still fails, and Describe again", locator: '[data-preview="build"]', expectText: "Hosts your description did not name" },
  { name: "page-build-comments", url: "/pages.html#build-comments", note: "a loaded agent.yaml with comments: the notice that a save drops them", locator: '[data-preview="build"]', expectText: "the comments are not kept" },
  { name: "page-validate", url: "/pages.html", note: "Validate under the shell", locator: '[data-preview="validate"]' },
  // Settings as five cards (agentry-desktop#346): the landing with each
  // card's status line; each section page; a card opened, a part's link
  // followed, the way back; and each deep link App gives a caller.
  { name: "page-settings", url: "/pages.html", note: "Settings under the shell: five cards, each with its sentence, its status line and its parts", locator: '[data-preview="settings"]',
    expectText: ["Registry settings", "Update settings", "Coding agents", "Run defaults", "Provider settings", "secureagentry.net, signed in.", "Agentry Desktop 0.2.0 is available.", "AI Review key saved", "Runs stop after 15 minutes, on this computer.", "Settings file:"],
    expectSection: "landing" },
  { name: "settings-open-registry", url: "/pages.html", note: "the Registry settings card opened: the registries, Secrets and Add a registry under ← Settings", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Registry settings") .settings-card-link', expectSection: "registry", expectText: ["← Settings", "Registries", "Add a registry", "Secrets for runs In Agentry"] },
  { name: "settings-open-updates", url: "/pages.html", note: "Update settings: the self-update card and the BriefAgents switch", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Update settings") .settings-card-link', expectSection: "updates", expectText: ["Check for newer versions", "Update BriefAgents automatically"] },
  { name: "settings-open-coding-agents", url: "/pages.html", note: "Coding agents: the chooser and the export switch", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Coding agents") .settings-card-link', expectSection: "coding-agents", expectText: ["Also install into"] },
  { name: "settings-open-run-defaults", url: "/pages.html", note: "Run defaults: the Docker engine's state (M4 adds the defaults)", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Run defaults") .settings-card-link', expectSection: "run-defaults", expectText: ["Docker engine"] },
  { name: "settings-open-provider", url: "/pages.html", note: "Provider settings: the model provider form and the AI Review key", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Provider settings") .settings-card-link', expectSection: "provider", expectText: ["Model provider", "Save provider settings", "Your own TypeSafe Jev key"] },
  { name: "settings-part-link", url: "/pages.html", note: "a part's link on the landing opens its section at that part", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card-parts button:has-text("AI Review key")', expectSection: "provider", expectPart: "ai-review-key" },
  { name: "settings-back-registry", url: "/pages.html?settings=registry", note: "← Settings on Registry settings returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-updates", url: "/pages.html?settings=updates", note: "← Settings on Update settings returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-coding-agents", url: "/pages.html?settings=coding-agents", note: "← Settings on Coding agents returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-run-defaults", url: "/pages.html?settings=run-defaults", note: "← Settings on Run defaults returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-provider", url: "/pages.html?settings=provider", note: "← Settings on Provider settings returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  // Run defaults (agentry-desktop#347): the four defaults and Docker; a
  // budget saved, In Agentry chosen and a folder picked, as Go answers.
  { name: "settings-run-defaults", url: "/pages.html?settings=run-defaults", note: "Run defaults: timeout and turns (the policy's, which the CLI shares), where to run, the output folder, Docker", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: ["Timeout (seconds)", "the agentry CLI runs with them too", "Where to run", "every consent is asked on every run", "None chosen", "Docker engine"],
    expectValue: ['[data-preview="settings"] #settings-part-budgets input[type="number"] >> nth=0', "900"] },
  { name: "settings-run-defaults-saved", url: "/pages.html?settings=run-defaults", note: "a timeout of 1800 s saved, In Agentry chosen, a folder picked", locator: '[data-preview="settings"]',
    fill: [['[data-preview="settings"] #settings-part-budgets input[type="number"] >> nth=0', "1800"]],
    click: ['[data-preview="settings"] input[name="run-where"] >> nth=1', '[data-preview="settings"] #settings-part-output button:has-text("Choose")', '[data-preview="settings"] #settings-part-budgets button:has-text("Save")'],
    expectSection: "run-defaults", expectText: ["Saved.", "/home/me/agentry-runs"], expectChecked: '[data-preview="settings"] input[name="run-where"] >> nth=1' },
  { name: "settings-deeplink-selfupdate", url: "/pages.html", note: "the sidebar's newer-version line opens Update settings", locator: '[data-preview="settings"]', click: '[data-deeplink="selfUpdate"]', expectSection: "updates", expectPart: "desktop" },
  { name: "settings-deeplink-setup", url: "/pages.html", note: "Browse's setup notice opens Provider settings", locator: '[data-preview="settings"]', click: '[data-deeplink="setupNotice"]', expectSection: "provider" },
  { name: "settings-deeplink-knowledge", url: "/pages.html", note: "Knowledge's Not signed in opens Registry settings at the registries", locator: '[data-preview="settings"]', click: '[data-deeplink="knowledgeSignIn"]', expectSection: "registry", expectPart: "registries" },
  { name: "settings-deeplink-describe", url: "/pages.html", note: "Build's Describe it without an AI Review key opens Provider settings at the key", locator: '[data-preview="settings"]', click: '[data-deeplink="describeKey"]', expectSection: "provider", expectPart: "ai-review-key" },
  { name: "settings-status-failed", url: "/pages.html#status-refused", note: "the landing when every status read fails: the five cards stay, their lines drop", locator: '[data-preview="settings"]',
    expectSection: "landing", expectCards: 5, expectStatusLines: 1 },
  { name: "toolspanel", url: "/modals.html#toolspanel", note: "the tools panel over the shell" },
  { name: "toolspanel-hostile", url: "/modals.html#toolspanel-hostile", note: "the hostile guide's key help in the tools panel (agentry-desktop#343): markdown, framed, each link's host beside it, nothing that acts", linksIn: ".key-help", expectText: ["The author's help for", "U+202E", "(evil.example)", "(github.com.evil.example)"], expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new", "https://evil.example/steal", "https://github.co/", "https://github.com.evil.example/"], expectNone: ".key-help img, .key-help script, .key-help b[onclick], .key-help a[href^='javascript'], .key-help a[href^='file']" },
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
      // A field filled first, as a person would type it.
      for (const [sel, val] of state.fill ?? []) {
        await page.locator(sel).first().fill(val);
      }
      // A state behind a click — a tab, a wizard's next step — or a few in
      // order (click as a list: Run, then Run details) is reached the way a
      // person reaches it.
      for (const sel of [state.click ?? []].flat()) {
        await page.locator(sel).first().click();
        await page.waitForTimeout(400);
      }
      // expectText may name several sentences, each of which must show.
      for (const want of [state.expectText ?? []].flat()) {
        const text = await page.locator(state.locator ?? "body").innerText();
        if (!text.includes(want)) throw new Error(`${state.name}: the page does not show "${want}"`);
      }
      if (state.expectNone) {
        // Agent output makes no markup of its own (agentry-desktop#227).
        const n = await page.locator(state.expectNone).count();
        if (n !== 0) throw new Error(`${state.name}: ${n} element(s) matching ${state.expectNone}`);
      }
      if (state.expectDisabledButton) {
        // A blocker greys the button; the script says so, not only the shot.
        const b = page.locator(".modal").getByRole("button", { name: state.expectDisabledButton }).first();
        if (await b.isEnabled()) throw new Error(`${state.name}: ${state.expectDisabledButton} is enabled with a blocker showing`);
      }
      if (state.expectPlaceholder) {
        // The task field's placeholder is the author's (agentry-desktop#228).
        const ph = await page.locator(".run-step textarea").first().getAttribute("placeholder");
        if (ph !== state.expectPlaceholder) throw new Error(`${state.name}: the task placeholder reads "${ph}"`);
      }
      if (state.expectLinks) {
        // Exactly these links, and nothing else a click could follow.
        const hrefs = await page.locator(`${state.linksIn ?? ".run-step"} a[href]`).evaluateAll((as) => as.map((a) => a.getAttribute("href")));
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
      if (state.expectChecked) {
        if (!(await page.locator(state.expectChecked).first().isChecked())) throw new Error(`${state.name}: ${state.expectChecked} is not checked`);
      }
      if (state.expectValue) {
        const [sel, want] = state.expectValue;
        const got = await page.locator(sel).first().inputValue();
        if (got !== want) throw new Error(`${state.name}: ${sel} holds "${got}", not "${want}"`);
      }
      if (state.expectSection) {
        // The route, not only the shot: which Settings page is showing.
        const at = await page.locator('[data-preview="settings"] [data-settings-section]').first().getAttribute("data-settings-section");
        if (at !== state.expectSection) throw new Error(`${state.name}: Settings shows "${at}", not "${state.expectSection}"`);
      }
      if (state.expectPart) {
        // A part the link named is scrolled into view.
        const seen = await page.locator(`#settings-part-${state.expectPart}`).evaluate((el) => {
          const r = el.getBoundingClientRect();
          return r.top < window.innerHeight && r.bottom > 0;
        });
        if (!seen) throw new Error(`${state.name}: the part ${state.expectPart} is not in view`);
      }
      if (state.expectCards) {
        const n = await page.locator('[data-preview="settings"] .settings-card').count();
        if (n !== state.expectCards) throw new Error(`${state.name}: ${n} cards, not ${state.expectCards}`);
      }
      if (state.expectStatusLines !== undefined) {
        // Only the registry's line, which reads no binding, is left.
        const n = await page.locator('[data-preview="settings"] .settings-card-status').count();
        if (n !== state.expectStatusLines) throw new Error(`${state.name}: ${n} status lines, not ${state.expectStatusLines}`);
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
