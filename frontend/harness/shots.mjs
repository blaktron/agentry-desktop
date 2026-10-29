// Screenshots of the run surfaces from the design harness, light and dark.
// It starts the harness itself, shoots each state into harness/shots/
// (ignored by git), and stops it again.
//
//   cd frontend && node harness/shots.mjs
//
// Playwright is not a dependency of this app: the harness is dev-only and
// CI never shoots it. Point PLAYWRIGHT_MODULE at an installation that has
// it:
//
//   PLAYWRIGHT_MODULE=/path/to/node_modules/playwright/index.mjs \
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

// The AI Review key's recommendation, word for word: spelled out here, not
// imported, so a change to the app's sentence fails the states that show it.
// The Build start page's words, the same on both clients: BuildWays.tsx and
// the Build page of the web app.
const BUILD_TAGLINE = "Use our AI Powered QuickAgent Drafter or Build one to your exact requirements.";
const DESCRIBE_COPY = "Build a QuickAgent using the AI Drafter - just tell it what you want an agent to do and it will figure out what tools and servers it needs to do that job and build it out in the correct format.";
const BY_HAND_COPY = "Build a QuickAgent manually so that it does exactly what you want with exactly the tools you want. This is useful if you already have a workflow to bring over, or if you know precisely how you want the agent to behave.";
const KEY_RECOMMENDATION = "Agentry's AI Review is powered by TypeSafe AI's Jev System One. We highly recommend signing up with TypeSafe and getting an API key if you want to build shareable QuickAgents or submit them to Agentry.";

// One entry per state the panel can be in.
const states = [
  // Report bug: the card over the page, and the link above the version.
  { name: "bugreport", url: "/modals.html#bugreport", note: "signed in: the box, Cancel and Submit", expectText: "Tell us what went wrong" },
  { name: "bugreport-signedout", url: "/modals.html#bugreport-signedout", note: "signed out: Sign in instead of the box", expectText: "Sign in to report a bug, so we can email you when it is fixed." },
  { name: "bugreport-short", url: "/modals.html#bugreport-short", note: "a text below 10 characters: Submit held, and why", expectText: "A bug report is at least 10 characters." },
  { name: "bugreport-filed", url: "/modals.html#bugreport-filed", note: "filed: the thanks, before the card closes itself", expectText: "Thanks. Your report reached the Agentry team" },
  { name: "bugreport-limited", url: "/modals.html#bugreport-limited", note: "the registry's ten-an-hour refusal, in its words", expectText: "You have sent 10 bug reports in the last hour. Try again later." },
  { name: "bugreport-absent", url: "/modals.html#bugreport-absent", note: "a registry without the bug-reports extension", expectText: "This registry does not take bug reports." },
  { name: "runpanel-modes", url: "/modals.html#runpanel", note: "Before you start: what it does, what to have ready, where to run it (In Agentry beside the local modes)", expectText: "What to have ready" },
  { name: "runpanel-in-agentry", url: "/modals.html#runpanel-hosted", note: "In Agentry chosen: the hold, the balance, the caps", expectNoText: "Its publisher earns" },
  { name: "runpanel-in-agentry-earns", url: "/modals.html#runpanel-hosted-earns", note: "In Agentry chosen for a release whose publisher is in Publisher Earnings: the line under the cost", expectText: "Its publisher earns a share of the credits runs of it spend. What you pay does not change." },
  { name: "runpanel-unavailable", url: "/modals.html#runpanel-unavailable", note: "offered, unavailable, with the reason and the remedy" },
  { name: "runpanel-hosted-knowledge", url: "/modals.html#runpanel-hosted-knowledge", note: "the Knowledge step In Agentry: packs kept and scanned in Agentry can be ticked, one kept only here offers Copy to Cloud, one being scanned says so", expectText: "Attach knowledge packs Agentry keeps for your account" },
  { name: "runpanel-hosted-optional", url: "/modals.html#runpanel-hosted-optional", note: "In Agentry with PAGERDUTY_TOKEN required and STATUSPAGE_TOKEN optional: the ask names both, the optional one marked", click: 'button:has-text("Run In Agentry")', expectText: ["Secrets for this run", "Enter the required one to start the run", "optional: leave it empty to run without it"] },
  { name: "runpanel-hosted-optional-only", url: "/modals.html#runpanel-hosted-optional-only", note: "In Agentry with STATUSPAGE_TOKEN alone, optional: the ask offers Start with the field empty", click: 'button:has-text("Run In Agentry")', expectText: ["Secrets for this run", "It is optional: enter it for the run to use it, or start without it"] },
  { name: "runpanel-hosted-keys-tools", url: "/modals.html#runpanel-hosted-keys-tools", note: "In Agentry on Connect tools for a release with a required key and no local profile: Next goes on to Details", click: 'button:has-text("Next:")', expectText: "What should the run do?", expectNone: 'text="Save the keys to go on"' },
  { name: "runpanel-hosted-secrets", url: "/modals.html#runpanel-hosted-secrets", note: "In Agentry for a release that declares PAGERDUTY_TOKEN: the start is refused secret-missing and the ask shows, Keep it for later runs unticked", click: 'button:has-text("Run In Agentry")', expectText: "Secrets for this run" },
  { name: "runpanel-hosted-followed", url: "/modals.html#runpanel-hosted-followed", note: "Review and start In Agentry while a run In Agentry is followed in its card: Run held, the line above it says why, the card in the corner", expectText: ["A run or a chat In Agentry is under way, in its card in the corner", "Running demo1/incident-summarizer@1.2.0 In Agentry"] },
  { name: "runpanel-unscanned", url: "/modals.html#runpanel-unscanned", note: "a release admitted without a scan: its own mark and consent, In Agentry not offered; the line above Run names the consent", expectBlocker: "Tick \u201cRun an unscanned release\u201d", expectRunDisabled: true },
  { name: "runpanel-local-install", url: "/modals.html#runpanel-local-install", note: "a QuickAgent installed from a folder on this computer, run from the Library: local and unsigned, its consent in its own words, In Agentry not offered", expectText: ["It was installed from a folder on this computer: no pipeline scanned it and no one signed it.", "The run's receipts are marked local."], expectBlocker: "Tick \u201cRun an unsigned release\u201d", expectRunDisabled: true },
  // The form asks only for what the release declares:
  // the keys line names the server and its secret with Run grey; a file
  // intake question shows as the form; Input files stays a disclosure.
  { name: "runpanel-keys", url: "/modals.html#runpanel-keys", note: "pr-review-brief's shape, on Review and start: keys unsaved, the line above Run names them", expectBlocker: "Save the keys for github-mcp-server-js: GITHUB_TOKEN", expectRunDisabled: true },
  { name: "runpanel-keys-optional", url: "/modals.html#runpanel-keys-optional", note: "Connect tools with the one key optional and nothing saved: Set keys… offered", expectText: "Set keys…" },
  { name: "runpanel-keys-optional-saved", url: "/modals.html#runpanel-keys-optional-saved", note: "the optional key set at run time: Set keys…, then Save keys", click: ['button:has-text("Set keys")', 'button:has-text("Save keys")'], expectText: "Stored for this exact release: SEARCH_KEY" },
  { name: "runpanel-keys-saved", url: "/modals.html#runpanel-keys-saved", note: "Connect tools: the key saved, shown by name only", click: 'button:has-text("Save keys")', expectText: "Stored for this exact release: GITHUB_TOKEN" },
  { name: "runpanel-file-intake", url: "/modals.html#runpanel-file-intake", note: "a release that asks for a file up front: on Review and start the line names the unanswered question", expectBlocker: "The incident channel's export (a file)", expectRunDisabled: true },
  // Run defaults in the run setup: In Agentry chosen by
  // default where it is offered; a release that cannot run there starts here
  // and says why; the saved folder filled in on Review and start.
  { name: "runpanel-default-agentry", url: "/modals.html#runpanel-default-agentry", note: "where to run saved as In Agentry: the Where step opens with it chosen, the credits it holds shown", expectText: ["What to have ready", "This run holds 20 credits"], expectChecked: 'input[name="run-mode"] >> nth=-1' },
  { name: "runpanel-default-agentry-unscanned", url: "/modals.html#runpanel-default-agentry-unscanned", note: "the same default for a release admitted without a scan: it starts on this computer, and the step says why", expectText: "Your Run defaults say In Agentry, but a release admitted without a scan is not run In Agentry from this app, so this run starts on this computer." },
  { name: "runpanel-default-agentry-signedout", url: "/modals.html#runpanel-default-agentry-signedout", note: "the same default, signed out: it starts on this computer and names the sign-in", expectText: "a run In Agentry needs you signed in to this registry" },
  { name: "runpanel-default-folder", url: "/modals.html#runpanel-default-folder", note: "a saved output folder, filled in on Review and start; Choose… still picks another", expectText: "Review and start", expectValue: ['.modal input.mono[readonly]', "/home/me/agentry-runs"] },
  // The guided run setup: one state per step.
  { name: "runpanel-step-files", url: "/modals.html#runpanel-step-files", note: "Files: no file chosen yet, Add files and Add folder", expectText: "Step 2 of" },
  { name: "runpanel-step-tools", url: "/modals.html#runpanel-step-tools", note: "Connect tools: the declared MCP server's pin and tools, then its key", expectText: "Step 3 of 5: Connect tools" },
  { name: "runpanel-source-details", url: "/modals.html#runpanel-source-details", note: "Details for a release that asks for a repository and, optionally, a file at an address: the address and the branch, tag or commit, nothing answered yet", expectText: ["The repository to audit *", "Branch, tag or commit", "The advisory to check it against (a PDF or a page)"], expectNoText: "Access token for" },
  { name: "runpanel-source-saved", url: "/modals.html#runpanel-source-saved", note: "Details with the repository answered on a host whose key is saved: the run uses the saved key, another token can be typed for this run, the saved key can be removed", wait: 800, expectText: ["This run uses the key saved on this computer for github.com.", "Use another token for this run", "Remove the saved key"], expectNoText: "Access token for github.com" },
  { name: "runpanel-source-typed", url: "/modals.html#runpanel-source-typed", note: "Details with the repository answered on a host with no saved key: the token field, optional, and Save for the host", wait: 800, expectText: ["Access token for git.example.org", "Save for git.example.org", "No key is saved for git.example.org. A public repository needs none; a private one needs an access token."] },
  { name: "runpanel-source-review", url: "/modals.html#runpanel-source-review", note: "Review and start: the repository with its ref and the saved key it uses, the file's address with its query hidden, never a token", wait: 800, expectText: ["https://github.com/acme/payments, at v4.2.0, with the key saved for github.com", "https://advisories.example.com/GHSA-2026-0042.pdf?…, with no access token"], expectNoText: "sig=8f2c" },
  { name: "runpanel-source-hosted", url: "/modals.html#runpanel-source-hosted", note: "In Agentry chosen for a release given a repository with a key saved on this computer: Review says that key is sent to Agentry for this run alone.", wait: 800, expectText: "with the key saved on this computer for github.com, sent to Agentry for this run alone" },
  { name: "runpanel-host-where", url: "/modals.html#runpanel-host-where", note: "Before you start for a release that asks for a server: what to have ready names the address and the sign-in, and In Agentry is not offered, with why", wait: 800, expectText: ["The server for “The server to set up”, as user@host", "A run In Agentry is not offered for this QuickAgent: it works a server you name over SSH"] },
  { name: "runpanel-site-where", url: "/modals.html#runpanel-site-where", note: "Before you start for a release with a browser: what to have ready names the site to test, and In Agentry is not offered, with why", wait: 800, expectText: ["The address of the site to test for “Which site should I test?”", "it drives a web browser, which no run In Agentry has yet", "browser: a web browser that can open the site you name for site and its subdomains"] },
  { name: "runpanel-site-details", url: "/modals.html#runpanel-site-details", note: "Details for a release with a browser: the site question is an address with no token field, and one with no scheme is held", wait: 800, expectText: ["Which site should I test? *", "This address is not a web address. It starts with https:// or http://."], expectNoText: "Access token for" },
  { name: "runpanel-site-review", url: "/modals.html#runpanel-site-review", note: "Review and start with the site answered: the address as given, nothing fetched", wait: 800, expectText: "https://staging.example.com" },
  { name: "runpanel-host-details", url: "/modals.html#runpanel-host-details", note: "Details for a release that asks for a server: the address field and this computer's recent hosts, nothing answered yet", wait: 800, expectText: ["The server to set up *", "Recent on this computer", "admin@web-01.example.net"], expectNoText: "Sign in with" },
  { name: "runpanel-host-key", url: "/modals.html#runpanel-host-key", note: "Details with a server on port 2222 and no saved sign-in: a key file (chosen in the picker) or a password, the sudo field, Save, and the host key's dialog explained", wait: 800, expectText: ["Signs in as deploy to 203.0.113.10, port 2222.", "Private key file", "sudo password", "Save this sign-in on this computer for deploy@203.0.113.10:2222", "a dialog shows its fingerprint for you to accept"] },
  { name: "runpanel-host-saved", url: "/modals.html#runpanel-host-saved", note: "Details with a server whose sign-in is saved: the saved one is used, another can be given for this run, the saved one removed", wait: 800, expectText: ["the sign-in saved here", "This run signs in with the sign-in saved on this computer.", "Remove the saved sign-in"] },
  { name: "runpanel-host-review", url: "/modals.html#runpanel-host-review", note: "Review and start: the server and where its sign-in comes from, never a secret", wait: 800, expectText: "admin@web-01.example.net, with the sign-in saved on this computer" },
  { name: "runpanel-step-details", url: "/modals.html#runpanel-step-details", note: "Details: the intake question, then the task", expectText: ": Details" },
  { name: "runpanel-step-review", url: "/modals.html#runpanel-step-review", note: "Review and start: the recap with Edit links, the output folder, Advanced, Run", expectText: "Review and start" },
  { name: "runpanel-voice", url: "/modals.html#runpanel-voice", note: "Review and start with this computer's Writing Voice on for every run: Write in my voice, ticked from its switch", expectText: ["Write in my voice", "Here it is this computer's voice."] },
  { name: "runpanel-schedule", url: "/modals.html#runpanel-schedule", note: "Review and start opened by Schedule a new run: When on Repeat (every day, the next whole hour, this computer's zone), the signed-out box, the check ready, and the button reads Schedule; the fixture's unanswered intake question still holds it", expectText: ["When", "Also run when I'm signed out", "Ready: every day at 15:00 (Europe/London)"], expectSelector: '.run-step-foot button.primary:text-is("Schedule")' },
  { name: "runpanel-schedule-gaps", url: "/modals.html#runpanel-schedule-gaps", note: "a fire would lack the model key: the check's gap holds Schedule", expectText: ["A scheduled run would not have it: no model key is kept in the keychain"], expectSelector: '.run-step-foot button.primary:text-is("Schedule")[disabled]' },
  { name: "runpanel-schedule-hosted", url: "/modals.html#runpanel-schedule-hosted", note: "When on Repeat for a run In Agentry, on Pro: no signed-out box, each run's hold and the balance, the check Agentry makes when Schedule is pressed, no output folder, and Schedule enabled", expectText: ["A schedule In Agentry runs this exact version", "holds 20 credits while it runs, and your balance is 46", "each secret kept in your account"], expectNone: "[data-signed-out]", expectSelector: '.run-step-foot button.primary:text-is("Schedule"):not([disabled])' },
  { name: "runpanel-schedule-hosted-standard", url: "/modals.html#runpanel-schedule-hosted-standard", note: "the same on Standard: Later and Repeat are off, with the plan's sentence and the way up", expectText: ["Scheduling runs In Agentry is part of Pro, Business and Enterprise.", "On Standard, schedule it on this computer", "Credits and upgrade"], expectSelector: '[data-when-step] input[type="radio"][disabled]' },
  { name: "runpanel-schedule-hosted-gap", url: "/modals.html#runpanel-schedule-hosted-gap", note: "Schedule In Agentry refused secret-not-kept: the secret asked for here, kept, with no box to hold it for one run", click: '.run-step-foot button.primary:text-is("Schedule")', expectText: ["Secrets for this schedule", "Keep and schedule"], expectNone: 'text="Keep it for later runs"' },
  { name: "runpanel-schedule-windows", url: "/modals.html#runpanel-schedule-windows", note: "on Windows the signed-out box is off, with the reason", expectText: ["Docker Desktop stops when you sign out."], expectSelector: '[data-signed-out] input[type="checkbox"][disabled]' },
  { name: "runpanel-step-review-long", url: "/modals.html#runpanel-step-review-long", note: "Review and start with an intake prompt as long as binary-reverse-engineer's: the label wraps in its column and every value keeps a readable width", expectText: ["leave blank for a full pass", "none: its own instruction runs"], expectMinWidth: [".run-recap dd", 320] },
  // The author's guide: each step that reads it, and a
  // hostile guide that must render as text on the two steps it reaches.
  { name: "runpanel-guided", url: "/modals.html#runpanel-guided", note: "Before you start with a guide: the author's overview, then the author's What to have ready", expectText: "review brief" },
  { name: "runpanel-guided-files", url: "/modals.html#runpanel-guided-files", note: "Files: the author's line on what to add", expectText: "None needed: it reads the pull request from GitHub." },
  { name: "runpanel-signin", url: "/modals.html#runpanel-signin", note: "Connect tools for a release whose remote server signs in by OAuth (Google's Gmail server): Connect, and what it asks for", expectText: ["Sign-ins", "Not connected yet.", "a remote server, signs you in by OAuth"] },
  { name: "runpanel-signin-connecting", url: "/modals.html#runpanel-signin-connecting", note: "Connect clicked: the wait, the consent address, and the field for the address a browser on another computer ended on", click: 'button:text-is("Connect")', expectText: ["Approve the sign-in in your browser.", "accounts.google.com/o/oauth2/v2/auth", "Paste that page's whole address here"] },
  { name: "runpanel-signin-own-client", url: "/modals.html#runpanel-signin-own-client", note: "Connect clicked at Google, which lets no app register itself: Agentry's Google steps first, the Developer Preview and Test users steps among them, then the client form", click: 'button:text-is("Connect")', expectText: ["these are Agentry's steps", "Workspace Developer Preview", "Test users", "Desktop app", "Keep it and connect"] },
  { name: "runpanel-signin-own-client-help", url: "/modals.html#runpanel-signin-own-client-help", note: "the own-client ask for a release that carries its own help for that server: Agentry's steps, then the author's help, then the client form", click: 'button:text-is("Connect")', expectText: ["these are Agentry's steps", "The author's help for https://gmailmcp.googleapis.com/mcp/v1", "same Google account", "Keep it and connect"] },
  { name: "runpanel-signin-connected", url: "/modals.html#runpanel-signin-connected", note: "connected: where, as which client, since when, and that Google did not say which account; Connect again and Disconnect", expectText: ["Connected at accounts.google.com, as your own OAuth client", "Disconnect"] },
  { name: "runpanel-signin-hosted", url: "/modals.html#runpanel-signin-hosted", note: "Connect tools In Agentry: Agentry keeps its own connections for this version, made on the site; Connect in your browser now", expectText: ["Sign-ins", "uses the connections Agentry keeps for this version", "Connect in your browser now"] },
  { name: "runpanel-signin-hosted-connect", url: "/modals.html#runpanel-signin-hosted-connect", note: "Run In Agentry refused connection-missing: Connect tools opens in the browser and the start waits for Agentry to hold the connection", click: 'button:has-text("Run In Agentry")', expectText: ["Connect tools in Agentry", "Waiting for Agentry to hold the connection", "Stop waiting"] },
  { name: "runpanel-signin-review", url: "/modals.html#runpanel-signin-review", note: "Review and start: Run held until the server is connected, by name", expectText: "Connect https://gmailmcp.googleapis.com/mcp/v1 on Connect tools." },
  { name: "runpanel-guided-tools", url: "/modals.html#runpanel-guided-tools", note: "Connect tools: the key's help under its field, the link opened in the system browser", expectText: "A fine-grained token with read access to pull requests", expectLinks: ["https://github.com/settings/personal-access-tokens/new"] },
  { name: "runpanel-guided-details", url: "/modals.html#runpanel-guided-details", note: "Details: the author's task placeholder", expectPlaceholder: "e.g. https://github.com/acme/api/pull/482, and look hardest at the migration" },
  { name: "runpanel-guided-hostile", url: "/modals.html#runpanel-guided-hostile", note: "a hostile guide on Before you start: no markup of its own, no image, no javascript: link, the hidden character shown", expectText: "U+200B", expectLinks: ["https://example.com/docs"], expectNone: ".run-step img, .run-step script, .run-step iframe, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  { name: "runpanel-guided-hostile-tools", url: "/modals.html#runpanel-guided-hostile-tools", note: "the hostile guide's key help on Connect tools: text; the https link; a URL with a user name left as text; a look-alike host linked as its punycode; the override shown; a key named constructor finds no help", expectText: ["U+202E", "The author's help for", "(evil.example)", "(github.co)", "(github.com.evil.example)"], expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new", "https://evil.example/steal", "https://github.co/", "https://github.com.evil.example/"], expectNone: ".run-step img, .run-step script, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  // Talk to it In Agentry, a hosted session.
  { name: "runpanel-session-offer", url: "/modals.html#runpanel-session-offer", note: "an interactive-only release with Talk to it In Agentry chosen: what a session costs, from the registry's terms", expectText: ["Talk to it In Agentry", "A session holds nothing. Its first 10 minutes are free"], expectChecked: 'input[name="run-mode"] >> nth=-1' },
  { name: "runpanel-session-tier", url: "/modals.html#runpanel-session-tier", note: "a payer on Standard: Talk to it In Agentry offered, unavailable, with the reason and See Pro", expectText: ["part of Pro, Business and Enterprise", "See Pro"] },
  { name: "runpanel-session-nohost", url: "/modals.html#runpanel-session-nohost", note: "no sandbox host answering: Talk to it In Agentry offered, unavailable, with the reason; no line says a session would queue", expectText: ["No sandbox host is answering right now", "Try again in a few minutes"] },
  // A run on this machine, finished: Run details
  // name the operator policy the run enforced and how its path was chosen.
  { name: "runpanel-local-finished", url: "/modals.html#runpanel-local-finished", note: "a local run finished: Run details name the operator policy, the runner home's default, and its digest abbreviated", click: ['button:text-is("Run")', "details.run-details > summary"], expectText: "/home/chris/.agentry/runner/policy.yaml" },
  { name: "runpanel-local-finished-env", url: "/modals.html#runpanel-local-finished-env", note: "the same run with the runner home relocated by AGENTRY_RUNNER_HOME: the source says so", click: ['button:text-is("Run")', "details.run-details > summary"], expectText: "AGENTRY_RUNNER_HOME, set in the environment" },
  // Knowledge on a run on this machine: the step that
  // lists the packs kept here, and a finished run's knowledge line.
  { name: "runpanel-step-knowledge", url: "/modals.html#runpanel-step-knowledge", note: "the Knowledge step: the packs kept on this computer, each with its scan state and whether it opens offline, and one kept in Agentry only, not attachable here", expectText: "Copy to Local on the Cloud Knowledge page" },
  // A release that ships a pack and requires one of the person's own:
  // the ask, the shipped pack, Next held.
  { name: "runpanel-step-knowledge-ships", url: "/modals.html#runpanel-step-knowledge-ships", note: "the Knowledge step of a release that ships pagerduty-docs and requires a pack of the person's own: its ask, its shipped pack, and Next held while none is ticked", click: ['button:has-text("Next:")'], expectText: "This release needs a knowledge pack of your own" },
  { name: "install-target-host", url: "/modals.html#install-target-host", note: "the install review of a release that declares a target host: the contract's target hosts row in the CLI's consent words, and the consent names it", click: "button:has-text('Verify')", expectText: ["target hosts", "server: asks for a server when it runs, and can run any command on it as the account you sign in with", "Accept the declared target host."] },
  { name: "install-browser", url: "/modals.html#install-browser", note: "the install review of a release that declares a browser: the contract's browser row in the CLI's consent words, naming the keys it may type, and the consent names it", click: "button:has-text('Verify')", expectText: ["a web browser that can open the site you name for site and its subdomains, and fonts.googleapis.com, fonts.gstatic.com; it can type SITE_USER, SITE_PASSWORD into the site's sign-in forms (it never sees them)", "Accept the declared browser."] },
  { name: "install-ships-knowledge", url: "/modals.html#install-ships-knowledge", note: "the install review of a release that ships a pack: the contract's knowledge row with its size", click: "button:has-text('Verify')", expectText: "ships pagerduty-docs (4.2 MiB)" },
  { name: "runpanel-local-finished-files", url: "/modals.html#runpanel-local-finished-files", note: "a local run given files, finished: the receipt's decode line under the result, ClamAV's and Hybrid Analysis's outcomes", click: ['button:text-is("Run")'], expectText: ["How your files were read and checked", "Hybrid Analysis (CrowdStrike), asked by SHA-256 alone and never sent a file"] },
  { name: "runpanel-local-finished-files-nokey", url: "/modals.html#runpanel-local-finished-files-nokey", note: "the same with no Hybrid Analysis key: the decode line says the risky file was not checked, no key, and nothing else warns", click: ['button:text-is("Run")'], expectText: ["How your files were read and checked", "1 not checked: no key"] },
  { name: "runpanel-knowledge-finished", url: "/modals.html#runpanel-knowledge-finished", note: "a local run with a pack attached, finished: the receipt's knowledge line under the result", click: ['button:text-is("Run")'], expectText: "Knowledge this run could search" },
  // The Knowledge panel: the
  // list in each state, Create knowledge's steps, a build under way and
  // done, a pack's preview, and a hostile page that must render as text.
  // The two places: the
  // cards, then each place's page.
  { name: "knowledge", url: "/knowledge.html#knowledge", note: "the start page: Local Knowledge and Cloud Knowledge, each with its line and its Create knowledge", locator: '[data-preview="knowledge"]', expectText: ["Local Knowledge", "Cloud Knowledge", "5 packs ·", "4 packs · 8.3 MB of 100 MB · 2 shared by acme."] },
  { name: "knowledge-local", url: "/knowledge.html#knowledge-local", note: "Local Knowledge: kept here (not scanned, opens offline), in both places, changed since it was copied, past Agentry's cap, and acme's shared copy", locator: '[data-preview="knowledge"]', expectText: ["Encrypted to your account's key at secureagentry.net, which Agentry holds", "also in your account", "changed since it was copied", "Copy to Cloud"] },
  { name: "knowledge-cloud", url: "/knowledge.html#knowledge-cloud", note: "Cloud Knowledge: the account's packs with Copy to Local and Delete from Agentry, the allocation, acme's shared group", locator: '[data-preview="knowledge"]', expectText: ["Storage in Agentry: 8.3 MB of 100 MB.", "also on this computer", "Copy to Local", "Shared by acme"] },
  { name: "knowledge-signed-out", url: "/knowledge.html#knowledge-signed-out", note: "not signed in, the Local page: a pack under this computer's key, and why Copy to Cloud waits", locator: '[data-preview="knowledge"]', expectText: "Encrypted to this computer's key" },
  { name: "knowledge-signed-out-start", url: "/knowledge.html#knowledge-signed-out-start", note: "not signed in: the Cloud card says to sign in", locator: '[data-preview="knowledge"]', expectText: "Sign in to keep knowledge in your account." },
  { name: "knowledge-empty", url: "/knowledge.html#knowledge-empty", note: "no pack yet: the cards say so", locator: '[data-preview="knowledge"]', expectText: ["No knowledge on this computer yet.", "No knowledge in your account yet."] },
  { name: "knowledge-empty-cloud", url: "/knowledge.html#knowledge-empty-cloud", note: "no pack in the account: Create knowledge, preset to In Agentry", locator: '[data-preview="knowledge"]', expectText: "No knowledge in your account yet" },
  { name: "knowledge-hosted-where-pro-cloud", url: "/knowledge.html#hosted-where-pro-cloud", note: "Create knowledge from the Cloud page for a Standard account: In Agentry not choosable, On this computer selected", expectText: "Creating knowledge in Agentry is part of Pro", expectChecked: 'input[name="knowledge-where"] >> nth=0' },
  { name: "knowledge-create-done-cloud", url: "/knowledge.html#create-done-cloud", note: "a pack made here from the Cloud page: Copy to Cloud beside Preview", expectText: ["Made handbook: 48 pages", "Copy to Cloud"] },
  { name: "knowledge-create-documents", url: "/knowledge.html#create-documents", note: "Create knowledge: the documents, from the native pickers only", expectText: "Step 1 of 3: Documents" },
  { name: "knowledge-create-name", url: "/knowledge.html#create-name", note: "Create knowledge: the name, and where a run finds it", expectText: "Step 2 of 3: Name" },
  { name: "knowledge-create-review", url: "/knowledge.html#create-review", note: "the review: before anything runs, that the Creator runs here against the policy's model and nothing it reads or writes is scanned, and that a build is a big job that may run for up to 60 minutes", expectText: ["Nothing it reads or writes is scanned", "may run for up to 60 minutes and 60 turns"], expectMinWidth: ['.step-recap dd', 320] },
  { name: "knowledge-create-blocked", url: "/knowledge.html#create-blocked", note: "the engine not ready: the sentence, Create knowledge greyed", expectText: "The container engine is not ready", expectDisabledButton: "Create knowledge" },
  { name: "knowledge-create-building", url: "/knowledge.html#create-building", note: "the build under way, driven by knowledge:build-progress events: time against its limit, turns, the pages written so far, the tokens; its log folded beneath", expectText: ["Pages written", "14 of up to 60"] },
  { name: "knowledge-create-building-log", url: "/knowledge.html#create-building", note: "the build under way with its log opened, streaming", click: 'summary:has-text("Build log")', expectText: "decode: storage-guide.pdf" },
  { name: "knowledge-create-done", url: "/knowledge.html#create-done", note: "what the build made: pages, the key, what was not read, a dropped page, the tokens", expectText: "Made handbook: 48 pages" },
  { name: "knowledge-hosted-stopped-brief", url: "/knowledge.html#hosted-stopped-brief", note: "a brief-only build In Agentry stopped at its turn limit: Continue starts again from this brief, never documents", expectText: ["Stopped at its turn or cost limit.", "as it is now, with this brief, searching the web"] },
  { name: "knowledge-create-stopped-brief", url: "/knowledge.html#create-stopped-brief", note: "a brief-only build stopped at its turn budget: continued with the same brief, never documents", expectText: ["Made espresso: 17 pages", "Add documents with the same brief continues it."] },
  { name: "knowledge-hosted-where", url: "/knowledge.html#hosted-where", note: "In Agentry beside this computer: the choice, and what it holds", expectText: "Step 3 of 4: Where" },
  { name: "knowledge-hosted-review", url: "/knowledge.html#hosted-review", note: "the review for a build In Agentry: where the documents go, the hold with the scan's credit inside it", expectText: "plus 1 credit to scan the pack" },
  { name: "knowledge-hosted-running", url: "/knowledge.html#hosted-running", note: "a build In Agentry under way: where it stands, the hold, Cancel", expectText: "100 credits are held for this build." },
  { name: "knowledge-hosted-done", url: "/knowledge.html#hosted-done", note: "a build In Agentry done: the pages, the settlement with the scan's credit, the pack kept here with its scan", expectText: "2 settled (1 of them for the pack's scan)" },
  { name: "knowledge-add-review", url: "/knowledge.html#add-review", note: "Add documents: the review, the same warnings", expectText: ["Step 2 of 2: Review", "may run for up to 60 minutes"] },
  { name: "knowledge-preview", url: "/knowledge.html#preview", note: "a pack's index and its pages, rendered by the run-output reader", expectText: "Rotating a key" },
  { name: "knowledge-preview-hostile", url: "/knowledge.html#preview-hostile", note: "a hostile page: no markup of its own, no image, no javascript: link, the right-to-left override shown", click: '.knowledge-pages button:has-text("Buckets")', expectText: "U+202E", expectNone: ".knowledge-page img, .knowledge-page script, .knowledge-page iframe, .knowledge-page b[onclick], .knowledge-page a[href^='javascript']" },
  // Two caps: 1 GB on this computer, 10 MB In Agentry.
  // A brief alone and Search the web.
  { name: "knowledge-create-documents-brief", url: "/knowledge.html#create-documents-brief", note: "Create knowledge with no documents and a brief: the warning, louder; Search the web on by default, with what it takes on this computer", expectText: ["A brief", "Specific documents make the best knowledge. Without them the Creator researches the web", "Search the web", "searches with Brave and your key"] },
  { name: "knowledge-create-documents-noweb", url: "/knowledge.html#create-documents-noweb", note: "the same with no search key on this computer: a build here runs without the web, and where to add one", expectText: "On this computer a build runs without the web: no Brave key was found" },
  { name: "knowledge-create-documents-none", url: "/knowledge.html#create-empty", note: "neither documents nor a brief: Next refused with the reason", click: 'button:has-text("Next:")', expectText: "write a brief that says what the pack should cover" },
  { name: "knowledge-create-documents-sources", url: "/knowledge.html#create-documents-sources", note: "the Documents step with a repository and two addresses, one refused by Go's check, and the key saved for github.com", expectText: ["Git repositories", "Web addresses of files or pages", "an address is https", "the key you saved for github.com"] },
  { name: "knowledge-create-review-sources", url: "/knowledge.html#create-review-sources", note: "the review of a build from a repository and an address: their rows beside the documents", expectText: ["Repositories", "https://github.com/google/uuid@v1.6.0", "Web addresses"] },
  { name: "knowledge-create-review-brief", url: "/knowledge.html#create-review-brief", note: "the review of a brief-only build on this computer: no documents, the brief, Search the web with your own keys, the warning", expectText: ["None: the Creator works from the brief", "Yes, with your own keys", "Specific documents make the best knowledge"], expectMinWidth: ['.step-recap dd', 320] },
  { name: "knowledge-hosted-review-brief", url: "/knowledge.html#hosted-review-brief", note: "the review of a brief-only build In Agentry: Search the web through the registry, the hold", expectText: ["Yes, through the registry", "plus 1 credit to scan the pack"] },
  { name: "knowledge-hosted-done-brief", url: "/knowledge.html#hosted-done-brief", note: "a brief-only build In Agentry done: what its record counted of its searches, the settlement, the pack kept here", expectText: ["This build searched the web 17 times", "espresso-care: 20 pages"] },
  { name: "knowledge-create-documents-large", url: "/knowledge.html#create-documents-large", note: "48 MB of documents: what they come to, and the warning that the build can take a very long time and the pack stays on this computer", expectText: ["48.0 MB together", "can take a very long time", "stay on this computer"] },
  { name: "knowledge-create-documents-huge", url: "/knowledge.html#create-documents-huge", note: "documents past 1 GB: Next refused with the reason", click: 'button:has-text("Next:")', expectText: "is at most 1 GB" },
  { name: "knowledge-hosted-where-large", url: "/knowledge.html#hosted-where-large", note: "the Where step with 48 MB of documents: In Agentry not choosable, and why", expectText: "In Agentry takes at most 10 MB of documents" },
  { name: "knowledge-large-pack", url: "/knowledge.html#knowledge-local", note: "a pack of 220 MB kept here, on the Local Knowledge page: no Copy to Cloud, and the sentence why", locator: '[data-preview="knowledge"]', expectText: "Past the 10 MB Agentry keeps, so it stays on this computer", expectNone: '.knowledge-row:has-text("scanned-archive") button:has-text("Copy to Cloud")' },
  // A running build in its corner card over the page.
  { name: "knowledge-card-building", url: "/knowledge.html#card-building", note: "a build on this computer shrunk to the corner card: its progress over the Knowledge page, which stays usable; Stop and Open", expectText: ["Creating handbook", "5 pages written", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "knowledge-card-done", url: "/knowledge.html#card-done", note: "the card once the build is over: what it made, Dismiss and Open", expectText: ["handbook: the build is over", "Made handbook: 48 pages", "Preview", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "knowledge-card-hosted", url: "/knowledge.html#card-hosted", note: "a build In Agentry in its card: the registry's status line and Cancel the build", expectText: ["In Agentry", "The build is running on a sandbox host.", "Cancel the build"], expectNone: ".modal-backdrop" },
  // A draft's progress dialog and its corner card, as
  // Create knowledge's: the dialog on this computer and In Agentry, how a
  // draft ends, and the card, alone and stacked over a knowledge build's.
  // Review Your QuickAgents locally: the action, the
  // review's dialog and card, and its report.
  { name: "review-validate", url: "/reviews.html#validate", note: "Validate with a folder chosen: Review Your QuickAgents locally beside it, with what it does", click: 'button:text-is("Choose…")', expectText: ["Review Your QuickAgents locally", "the registry scans again at Submit", "PASSED"] },
  { name: "review-validate-nokey", url: "/reviews.html#validate-nokey", note: "the same without an AI Review key: the action says to add one, Validate unchanged", click: 'button:text-is("Choose…")', expectText: ["AI Review needs your own TypeSafe key", "Add your AI Review key", "PASSED"], expectNone: '.review-action button:text-is("Review Your QuickAgents locally")' },
  { name: "review-validate-clean", url: "/reviews.html#validate-clean", note: "a clean folder answered with null lists: PASSED and both scans clean, not the error page", click: 'button:text-is("Choose…")', expectText: ["PASSED", "Clean — no binaries, nested archives, or encoding problems.", "Review Your QuickAgents locally"], expectNone: 'text=stopped with an error' },
  { name: "review-running", url: "/reviews.html#review-running", note: "a review under way: the stage, time against its limit, turns, tokens; its log folded; Stop the review and Run in the background", expectText: ["Reviewing a QuickAgent on this computer", "The Examiner is reading the QuickAgent", "6 of up to 60", "31,420 input", "Stop the review", "Run in the background"] },
  { name: "review-card", url: "/reviews.html#card-running", note: "the review shrunk to its corner card: the stage, the clock and turns, Stop and Open", expectText: ["Reviewing a QuickAgent here", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "review-report-published", url: "/reviews.html#report-published", note: "a report expecting published: the outcome under the registry's policy, the three kinds, what did not run, the notice", expectText: ["Expected at Submit: published", "The registry's rules, run here", "These rules found nothing.", "AI Review, asked as the pipeline asks it", "No question scored a finding.", "The Examiner's own reading", "What did not run here", "the registry's answer is the one that counts", "Submitting is never held by this review."] },
  { name: "review-report-held", url: "/reviews.html#report-held", note: "a report expecting held for review: AI Review's injection finding, the rules' hidden comment with its file and line, a file AI Review withheld from the Examiner's model", expectText: ["Expected at Submit: held for review", "S5/C1_injection", "S3/html-comment", "AGENT.md:5", "Withheld from its model by AI Review", "package/AGENT.md"] },
  { name: "review-report-hostile", url: "/reviews.html#report-hostile", note: "a report whose titles, excerpt, summary, detail and file names carry raw HTML, a script, an image, a javascript: link and a right-to-left override: all drawn as text, the override shown", expectText: ["<img src=x onerror=alert(2)>", "<script>alert(5)</script>", "U+202E", "U+200B", "recovered from a last message"], expectNone: ".modal img, .modal script, .modal iframe, .modal b[onclick], .modal a[href^=\"javascript\"]" },
  { name: "review-report-failed", url: "/reviews.html#report-failed", note: "the Examiner's run failed: the run's error, AI Review did not answer, not determined here, the rules' findings still shown", expectText: ["Expected at Submit: not determined here", "part of the report is missing", "It did not answer here", "There is no reading", "S3/html-comment"] },
  { name: "review-card-over", url: "/reviews.html#card-over", note: "the card once the review is over: the expected outcome in its line, Dismiss and Open", expectText: ["The review is over", "Expected at Submit: held for review", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "draft-local", url: "/drafts.html#draft-local", note: "a draft on this computer under way, driven by build:draft-progress: the stage, time against its limit, turns, tokens; its log folded; Stop the draft and Run in the background", expectText: ["The Drafter is drafting", "7 of up to 60", "48,210 input", "Run in the background", "Stop the draft"] },
  { name: "draft-local-log", url: "/drafts.html#draft-local", note: "the same with its log opened", click: 'summary:has-text("Draft log")', expectText: "tool: registry_search csv quality" },
  { name: "draft-agentry", url: "/drafts.html#draft-agentry", note: "a draft In Agentry under way: the registry's status line, the hold, Cancel the draft", expectText: ["Drafting a QuickAgent In Agentry", "The Drafter is running on a sandbox host.", "41 credits are held for this draft.", "Cancel the draft"] },
  { name: "draft-landed", url: "/drafts.html#draft-landed", note: "a draft that landed: its name, and Open the draft", expectText: ["Drafted csv-quality-check", "Open the draft"] },
  { name: "draft-failed", url: "/drafts.html#draft-failed", note: "a draft that ended with no draft: the error and the log", expectText: ["No draft was made", "the final message is not valid JSON"] },
  { name: "draft-hostile", url: "/drafts.html#draft-hostile", note: "a failed draft whose error and log carry raw HTML, a script, an image and a right-to-left override: all drawn as text", expectText: ["<img src=x onerror=alert(1)>", "U+202Etxt.exe", "U+202Egnp.exe"], expectNone: ".modal img, .modal script, .modal iframe, .modal b[onclick]" },
  { name: "draft-refused", url: "/drafts.html#draft-refused", note: "Agentry refused the draft for credits: the refusal and Credits and upgrade", expectText: ["Agentry did not start the draft", "a draft holds 41", "Credits and upgrade"] },
  { name: "draft-card-local", url: "/drafts.html#card-local", note: "a draft on this computer shrunk to the corner card over Describe it, which says a draft is being made: the stage, the time and turns, Stop and Open", expectText: ["Drafting a QuickAgent", "The Drafter is drafting", "Stop", "Show its progress"], expectNone: ".modal-backdrop" },
  { name: "draft-card-agentry", url: "/drafts.html#card-agentry", note: "a draft In Agentry in its card: the status line and Cancel the draft", expectText: ["Drafting a QuickAgent In Agentry", "The Drafter is running on a sandbox host.", "Cancel the draft"], expectNone: ".modal-backdrop" },
  { name: "draft-card-landed", url: "/drafts.html#card-landed", note: "the card once a draft landed: Drafted, Open the draft and Dismiss", expectText: ["Describe it: the draft is over", "Drafted csv-quality-check", "Open the draft", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "draft-card-stacked", url: "/drafts.html#card-stacked", note: "a draft In Agentry's card and a knowledge build's card stacked in App's corner, neither covering the other", expectText: ["Drafting a QuickAgent In Agentry", "Creating handbook"], expectNone: ".modal-backdrop" },
  // A run on this computer followed above the pages.
  { name: "run-running", url: "/runs.html#run-running", note: "a run on this computer under way, driven by runner:run-progress: time against its limit, turns, tokens; its log folded; Stop the run and Run in the background", expectText: ["Running demo1/incident-summarizer@1.2.0", "7 of up to 60", "48,210 input", "Stop the run", "Run in the background"] },
  { name: "run-running-log", url: "/runs.html#run-running", note: "the same with its log opened: this run's lines, and not another run's", click: 'summary:has-text("Run log")', expectText: "tool: read_input incident-channel.txt", expectNoText: "A LINE OF ANOTHER RUN" },
  { name: "run-background", url: "/runs.html#run-running", note: "Run in the background from the dialog: the dialog goes and the card takes its place", click: 'button:has-text("Run in the background")', expectText: ["Running demo1/incident-summarizer@1.2.0", "7 turns", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "run-card", url: "/runs.html#run-card", note: "the run shrunk to its corner card over the Library, which stays usable: the time and turns, Stop and Open", expectText: ["Running demo1/incident-summarizer@1.2.0", "7 turns", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "run-stopped", url: "/runs.html#run-stopped", note: "a run stopped from its card: the card says so, with Dismiss and Open", expectText: ["the run is over", "You stopped the run. Its receipts are written as far as it got.", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "run-finished", url: "/runs.html#run-finished", note: "the run over: the result first, the files in its folder, Run details naming the operator policy; Back to the setup and Dismiss", expectText: ["Finished.", "INC-4821 began at 02:14 UTC", "Files in the run folder", "Back to the setup", "Dismiss"] },
  { name: "run-failed", url: "/runs.html#run-failed", note: "a run that ended with no result: Go's words and its log", expectText: ["The run did not finish.", "Cannot connect to the Docker daemon", "tool: read_input incident-channel.txt"] },
  // The run page, opened from Home's Recent runs.
  { name: "runpage-deliverables", url: "/runs.html#runpage-deliverables", note: "the run page lists the files the run was for first, each with its copy, Open, View and Show in folder; a type no click opens says so; a withheld file is counted", locator: '[data-preview="runpage"]',
    expectText: ["Deliverables", "Weekly digest", "Copied to /home/chris/Documents/Agentry/pr-triage", "Numbers", "A .html file can be acted on by your system", "Its contents are not what its file type says", "One file the run's record names is not shown"],
    expectOrder: [['[data-preview="runpage"] h4', ["Deliverables", "The result", "Files in the run folder"]]] },
  { name: "runpage-deliverables-off", url: "/runs.html#runpage-deliverables-off", note: "with the switch off the line says the files are in the run folder only", locator: '[data-preview="runpage"]',
    expectText: ["Deliverables are off, so this file is in the run folder only."], expectNoText: ["Copied to"] },
  { name: "runpage-deliverables-open", url: "/runs.html#runpage-deliverables", note: "Open on a deliverable asks Go to open it by run id and path", locator: '[data-preview="runpage"]',
    click: '[data-preview="runpage"] [data-deliverable="reports/numbers.xlsx"] button:has-text("Open")', expectSelector: 'body[data-opened]' },
  { name: "runpage-local", url: "/runs.html#runpage-local", note: "a run on this computer the history keeps, as a page: how it ended, the result, the files, Show in folder, Home lit", locator: '[data-preview="runpage"]',
    expectText: ["demo1/incident-summarizer", "On this computer", "Finished.", "INC-4821 began at 02:14 UTC", "reports/report.md", "Show in folder", "← back to Home"],
    expectOrder: [['[data-preview="runpage"] .sidebar .nav-item.active', ["Home"]]] },
  { name: "runpage-hostile", url: "/runs.html#runpage-hostile", note: "the run page's result and files carrying raw HTML, a script, an image and a right-to-left override: all drawn as text (invariant 2)", locator: '[data-preview="runpage"]',
    expectText: "U+202E", expectNone: ".run-result-view img, .run-result-view script, .run-result-view iframe, .run-result-view b[onclick], .run-result-view a[href^='javascript']" },
  { name: "runpage-gone", url: "/runs.html#runpage-gone", note: "a run whose folder has gone: said, never looked for, with Remove from Recent runs", locator: '[data-preview="runpage"]',
    expectText: ["It ended in an error (exit 3).", "no longer there", "does not look for", "Remove from Recent runs"], expectNoText: "Show in folder" },
  { name: "runpage-gone-removed", url: "/runs.html#runpage-gone", note: "Remove from Recent runs drops the entry and goes back", locator: '[data-preview="runpage"]',
    click: 'button:has-text("Remove from Recent runs")', expectSelector: 'body[data-removed="1"][data-open="home"]' },
  { name: "runpage-draft", url: "/runs.html#runpage-draft", note: "a draft by the Drafter: its result view and Open in Build", locator: '[data-preview="runpage"]',
    expectText: ["A draft by the Drafter", "Finished.", "Drafted demo1/csv-quality-check", "Open in Build"] },
  { name: "runpage-draft-open", url: "/runs.html#runpage-draft", note: "Open in Build loads the selected local draft into the real Builder", locator: '[data-preview="runpage"]', click: 'button:has-text("Open in Build")', expectSelector: 'body[data-open="build"]', expectText: "Checks the selected CSV draft." },
  { name: "runpage-hosted-draft-open", url: "/runs.html#runpage-hosted-draft", note: "Open in Build loads the selected hosted draft", locator: '[data-preview="runpage"]', click: 'button:has-text("Open in Build")', expectText: "Checks the selected CSV draft.", expectSelector: 'body[data-open="build"]' },
  { name: "runpage-draft-open-error", url: "/runs.html#runpage-draft-error", note: "Unavailable output stays on the selected run page", locator: '[data-preview="runpage"]', click: 'button:has-text("Open in Build")', expectText: "The draft's output is no longer available.", expectNone: 'body[data-open="build"]' },
  { name: "runpage-hosted-draft-open-error", url: "/runs.html#runpage-hosted-draft-error", note: "Unavailable hosted output stays on the selected run page", locator: '[data-preview="runpage"]', click: 'button:has-text("Open in Build")', expectText: "The draft's output is no longer available.", expectNone: 'body[data-open="build"]' },
  { name: "runpage-review", url: "/runs.html#runpage-review", note: "a review by the Examiner: its report is the page, read back from its folder, with no button elsewhere", locator: '[data-preview="runpage"]',
    expectText: ["A review by the Examiner", "Expected at Submit: published", "me/release-notes@0.1.0", "Show the folder"], expectNoText: ["Open in Build", "Open in Knowledge"] },
  { name: "runpage-knowledge", url: "/runs.html#runpage-knowledge", note: "a knowledge build that was stopped and keeps no run folder: how it ended and Open in Knowledge", locator: '[data-preview="runpage"]',
    expectText: ["A knowledge build", "The job was stopped.", "keeps no run folder", "Open in Knowledge"], expectNoText: "Show in folder" },
  { name: "runpage-hosted", url: "/runs.html#runpage-hosted", note: "a run In Agentry whose output is not on this computer: how it ended and Download output, for no credits", locator: '[data-preview="runpage"]',
    expectText: ["Ran to the end and wrote its result.", "Download output", "for no credits", "30 days"], expectNoText: "Show in folder" },
  { name: "runpage-hosted-expired", url: "/runs.html#runpage-hosted-expired", note: "a run In Agentry whose output the registry no longer holds: no Download output, and why", locator: '[data-preview="runpage"]',
    expectText: "The registry holds no output for this run: it wrote none, or the 30 days it is kept have passed.", expectNoText: "Download output" },
  { name: "runpage-hosted-downloaded", url: "/runs.html#runpage-hosted", note: "after Download output: the result view, read by the registry's run id", locator: '[data-preview="runpage"]',
    click: 'button:has-text("Download output")', expectText: ["INC-4821 began at 02:14 UTC", "Show in folder"], expectSelector: 'body[data-downloaded="1"]' },
  { name: "runpage-hosted-kept", url: "/runs.html#runpage-hosted-kept", note: "a run In Agentry whose output is here: the result view, the receipt under Run details", locator: '[data-preview="runpage"]',
    expectText: ["Ran to the end and wrote its result.", "INC-4821 began at 02:14 UTC", "Show in folder"], expectNoText: "Download output" },
  { name: "runpage-hosted-draft", url: "/runs.html#runpage-hosted-draft", note: "a draft In Agentry: how it ended and Open in Build, no download", locator: '[data-preview="runpage"]',
    expectText: ["A draft by the Drafter", "Open in Build"], expectNoText: "Download output" },
  { name: "run-hostile", url: "/runs.html#run-hostile", note: "a finished run whose result, files and log carry raw HTML, a script, an image and a right-to-left override: all drawn as text", click: "details.run-details > summary", expectText: ["U+202E", "<img src=x onerror=alert(1)>"], expectNone: ".modal img, .modal script, .modal iframe, .modal b[onclick], .modal a[href^='javascript']" },
  { name: "run-card-stacked", url: "/runs.html#run-card-stacked", note: "the run's card and a draft In Agentry's card stacked in App's corner, neither covering the other", expectText: ["Running demo1/incident-summarizer@1.2.0", "Drafting a QuickAgent In Agentry"], expectNone: ".modal-backdrop" },
  { name: "hosted-queued", url: "/runs.html#hosted-queued", note: "a run In Agentry handed to the flow at Run and waiting for a host: its place in the queue, the credits held, Cancel run and Run in the background", expectText: ["Running demo1/incident-summarizer@1.2.0 In Agentry", "number 3 in the queue", "20 credits held", "Cancel run", "Run in the background", "never drives it"] },
  { name: "hosted-running", url: "/runs.html#hosted-running", note: "a run In Agentry running: time against its cap, the credits held, no turns row (the registry reports turns only once it is over)", expectText: ["running on a sandbox host", "of up to 15:00", "Credits held"], expectNoText: "Turns" },
  { name: "hosted-cancel", url: "/runs.html#hosted-cancel", note: "Cancel asked for: Agentry is stopping the run, and Cancel run is gone", expectText: "Cancel asked for: Agentry is stopping the run", expectNoText: "Cancel run" },
  { name: "hosted-finished", url: "/runs.html#hosted-finished", note: "a run In Agentry over: the result first, the files in its folder, the star ask, Run details with the receipt's knowledge line; Back to the setup and Dismiss", expectText: ["Finished.", "INC-4821 began at 02:14 UTC", "Files in the run folder", "If this run helped, star demo1/incident-summarizer. Stars help other people find it.", "Star this QuickAgent", "Knowledge this run could search", "Back to the setup", "Dismiss"] },
  { name: "hosted-finished-starred", url: "/runs.html#hosted-finished", note: "the star ask answered: it says so in place of the ask", click: 'button:has-text("Star this QuickAgent")', expectText: "Starred demo1/incident-summarizer.", expectNoText: "If this run helped" },
  { name: "hosted-hostile", url: "/runs.html#hosted-hostile", note: "a run In Agentry whose result and files carry hostile markdown: no markup of its own, no image, no javascript: link, the hidden characters shown", expectText: "U+202E", expectNone: ".run-result-view img, .run-result-view script, .run-result-view iframe, .run-result-view b[onclick], .run-result-view a[href^='javascript']" },
  { name: "hosted-failed", url: "/runs.html#hosted-failed", note: "a run In Agentry that failed, with no receipt kept: the registry's verdict line, and no star ask", expectText: ["The agent ran and failed", "Dismiss"], expectNoText: "If this run helped" },
  { name: "hosted-lost", url: "/runs.html#hosted-lost", note: "the app lost touch with Agentry while the run was going: nothing downloaded, Follow it again", expectText: ["lost touch with Agentry while the run was still going", "Follow it again", "nothing was downloaded"] },
  { name: "hosted-card", url: "/runs.html#hosted-card", note: "a run In Agentry already in flight when the app started, attached as its card: time, credits, Cancel run and Open", expectText: ["Running demo1/incident-summarizer@1.2.0 In Agentry", "20 credits held", "Cancel run", "Open"], expectNone: ".modal-backdrop" },
  { name: "hosted-card-finished", url: "/runs.html#hosted-card-finished", note: "a run In Agentry that ended while its dialog was closed lands in its card: Finished, Dismiss and Open", expectText: ["the run is over", "Finished.", "Dismiss", "Open"], expectNone: ".modal-backdrop" },
  { name: "hosted-card-stacked", url: "/runs.html#hosted-card-stacked", note: "a run In Agentry's card and a run on this computer's card stacked in App's corner", expectText: ["Running demo1/incident-summarizer@1.2.0 In Agentry", "20 credits held", "7 turns"], expectNone: ".modal-backdrop" },
  { name: "chat-local", url: "/runs.html#chat-local", note: "a chat on this computer followed above the pages: the dialog is the chat, the store started it, the strip, Chat in the background", expectText: ["Chat with demo1/incident-summarizer@1.2.0", "INC-4821 began at 02:14 UTC", "interactive · 1 turn · 1 tool call · $0.0182", "Chat in the background"] },
  { name: "chat-local-working", url: "/runs.html#chat-local-working", note: "its corner card while a message is out: the QuickAgent is working, the turns, Finish and Open", expectText: ["The QuickAgent is working · 1 turn", "Finish", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-local-waiting", url: "/runs.html#chat-local-waiting", note: "its card between turns: waiting for you", expectText: ["Waiting for you · 1 turn", "Finish", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-local-ended", url: "/runs.html#chat-local-ended", note: "the engine ended the chat with its dialog closed: the store collected its result, and the card says how it ended", expectText: ["it is over", "Finished.", "Dismiss", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-hosted", url: "/runs.html#chat-hosted", note: "a chat In Agentry: replayed from the host, a file added, a hostile agent reply as plain text (no markup, no link), the time-and-credits strip, the line that the transport is encrypted and messages are not stored", expectText: ["input added: /inputs/pager-export.csv", "interactive in Agentry · 2 turns · 1 tool call · 12:", "free minutes left 0 · 100 credits for time · idle closes in", "Transport encrypted by the user key, messages not stored"], expectNone: ".chat-pane script, .chat-pane b[onclick], .chat-pane a" },
  { name: "chat-hosted-working", url: "/runs.html#chat-hosted-working", note: "its card while the agent answers a message", expectText: ["Chat with demo1/incident-summarizer@1.2.0 In Agentry", "The QuickAgent is working · 2 turns", "Finish", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-hosted-waiting", url: "/runs.html#chat-hosted-waiting", note: "its card between turns", expectText: ["Waiting for you · 1 turn"], expectNone: ".modal-backdrop" },
  { name: "chat-hosted-ended", url: "/runs.html#chat-hosted-ended", note: "a chat In Agentry over: the result view, the session's credits (100 for time, 4 for tokens), transcript.md among the files, and the star ask", click: ["details.run-details > summary"], expectText: ["This session held nothing: its time cost 100 credits (2 minutes after the 10 free)", "receipts/transcript.md", "If this run helped, star demo1/incident-summarizer.", "Dismiss"] },
  { name: "knowledge-create-building-background", url: "/knowledge.html#create-building", note: "the building view offers Stop the build and Run in the background", expectText: ["Stop the build", "Run in the background"] },
  // A pack a build left short, and a render fault's way back.
  { name: "knowledge-preview-empty", url: "/knowledge.html#preview-empty", note: "a pack with its index and no page, the list answered as null: the preview says no pages yet, never a blank window", expectText: "No pages yet" },
  { name: "render-fault", url: "/knowledge.html#render-fault", note: "a page that throws while rendering: what went wrong and Try again, under a sidebar that still works", expectText: "This page stopped with an error" },
  { name: "render-fault-recovered", url: "/knowledge.html#render-fault", note: "Try again after the fault clears: the page renders afresh", click: 'button:has-text("Try again")', locator: '[data-preview="knowledge"]', expectText: "Knowledge packs a run can search" },
  { name: "agent-mcp-env", url: "/pages.html#agent-mcp-env", note: "the agent page's MCP tab: a declared server's plain env beneath its line, a server without env unchanged", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button.tab:has-text("MCP servers")' },
  { name: "install-mcp-env", url: "/modals.html#install-mcp-env", note: "the install review: each declared server's pin, tools and plain env", click: "button:has-text('Verify')" },
  { name: "install-unscanned", url: "/modals.html#install-unscanned", note: "the install wizard's review of a release admitted without a scan: the mark and the consent", click: "button:has-text('Verify')" },
  { name: "agent-unscanned", url: "/pages.html#agent-unscanned", note: "the agent page's provenance tab: the unscanned badge and the mark on the card", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button.tab:has-text("provenance")' },
  { name: "agent-unscanned-report", url: "/pages.html#agent-unscanned", note: "the agent page's report tab: unscanned as its own verdict, never 'scanned, 0 findings'", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button.tab:has-text("security report")' },
  // The agent page's one-click Install: the
  // four states of the one button, and the wizard behind its link.
  { name: "agent-install", url: "/pages.html", note: "not installed: Install <latest> in one click", locator: '[data-preview="agent"]' },
  { name: "agent-tab", url: "/pages.html", note: "the Agent tab: the README, then the release's instruction file as text under it", locator: '[data-preview="agent"]', expectText: ["Agent", "AGENT.md", "Read the incident channel export"], expectNone: '[data-preview="agent"] .agent-instructions a[href]' },
  { name: "agent-tab-hidden", url: "/pages.html#agent-instructions-hidden", note: "an instruction file whose name and text hide characters: each one a chip, in the heading, the note and the text", locator: '[data-preview="agent"]', expectText: ["AGENTU+202Emd.TXT is longer than the registry shows", "report.md.U+200B Then", "U+2066", "U+2069"] },
  { name: "agent-installing", url: "/pages.html#agent-installing", note: "installing: the button held", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button:has-text("Install 1.2.0")' },
  { name: "agent-installed", url: "/pages.html#agent-installed", note: "installed at the latest: Update, disabled; Pin to Home beside it", locator: '[data-preview="agent"]', expectSelector: '[data-preview="agent"] button[data-pin-button="pin"]' },
  { name: "agent-pin", url: "/pages.html#agent-installed", note: "Pin to Home on an installed QuickAgent's page: it turns to Unpin from Home", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button[data-pin-button="pin"]', expectSelector: '[data-preview="agent"] button[data-pin-button="unpin"]' },
  { name: "agent-open-run", url: "/pages.html#agent-open-run", note: "the page a pin's Run opens: its run setup open over it, for the latest version", locator: ".modal", expectText: ["Run demo1/incident-summarizer@1.2.0", "Before you start"] },
  { name: "agent-not-installed-no-pin", url: "/pages.html#fresh", note: "a QuickAgent not installed here offers no Pin", locator: '[data-preview="agent"]', expectNone: '[data-preview="agent"] button[data-pin-button]' },
  { name: "agent-installed-older", url: "/pages.html#agent-installed-older", note: "an older version installed: Update to <latest>", locator: '[data-preview="agent"]' },
  { name: "agent-install-refused", url: "/pages.html#agent-install-refused", note: "a declined gate: the refusal in place, the button back to Install", locator: '[data-preview="agent"]', click: '[data-preview="agent"] button:has-text("Install 1.2.0")' },
  // The checks: the registry unreachable on opening
  // the page; the sidebar's number after the launch check; the switch.
  { name: "agent-installed-unreachable", url: "/pages.html#agent-installed-unreachable", note: "installed, the registry could not be asked: Update disabled, the reason in the page", locator: '[data-preview="agent"]' },
  { name: "installed-update-warning", url: "/pages.html#installed-update-warning", note: "a runner-home Update that completes with a warning: the engine's sentence on the Library", locator: '[data-preview="library"]', click: '[data-preview="library"] button:text-is("Update")', expectText: "agentry/changelog-from-git: operator policy" },
  { name: "installed-badge", url: "/pages.html#installed-updates", note: "the launch check found newer versions: the number on Library", locator: '[data-preview="library"] .sidebar' },
  { name: "settings-secrets", url: "/pages.html?settings=registry", note: "Settings: Secrets for runs In Agentry — save a kept secret by name, delete one, the list on the registry's Secrets page", locator: '[data-preview="settings"] .card:has-text("Secrets for runs In Agentry")', expectText: "neither this app nor Agentry shows it again" },
  { name: "settings-quickagent-updates", url: "/pages.html?settings=updates", note: "Settings: the auto-update switch, shown and disabled, with why", locator: '[data-preview="settings"] .card:has-text("Update QuickAgents automatically")' },
  // The welcome's runtime step: the three questions;
  // the model list Get models filled; the failed listing with the model
  // still typeable; a stored subscription token and its sentence.
  // Reached by Back from the AI Review key step, as a person would.
  { name: "wizard-runtime", url: "/wizard.html#4", note: "the runtime step: provider, key, model", locator: ".wizard", click: '.wizard-foot button:has-text("Back")', expectText: "Set up the runtime" },
  { name: "wizard-agents", url: "/wizard.html#4", note: "the Agents step: every agent off, the Assistant recommended as the personal assistant and the Helper as its lite version, Skip offered", locator: ".wizard", expectText: ["Your agents", "Agentry Helper", "Assistant", "Coding agent", "Lite personal agent that works only with Agentry", "Recommended", "One of the two is on at a time."], expectSelector: ['[data-agent-tile="main"][aria-pressed="false"]:not([disabled])', '[data-agent-tile="assistant"][aria-pressed="false"] [data-agent-recommended]'], expectOrder: [".wizard-foot button", ["Back", "Skip", "Continue"]] },
  { name: "wizard-agents-on", url: "/wizard.html#4", note: "the Assistant's tile turned on: its Start when Agentry starts box shows, the Helper's tile held off and saying why, and Skip goes", locator: ".wizard", click: '[data-agent-tile="assistant"]', expectText: ["Start when Agentry starts", "Off while the Assistant is on"], expectSelector: ['[data-agent-tile="assistant"][aria-pressed="true"]', '[data-agent-tile="main"][aria-pressed="false"][disabled]'], expectOrder: [".wizard-foot button", ["Back", "Continue"]] },
  { name: "wizard-agents-helper", url: "/wizard.html#4", note: "the Helper's tile turned on, then the Assistant's: the Assistant turns the Helper off", locator: ".wizard", click: ['[data-agent-tile="main"]', '[data-agent-tile="assistant"]'], expectSelector: ['[data-agent-tile="assistant"][aria-pressed="true"]', '[data-agent-tile="main"][aria-pressed="false"][disabled]'] },
  { name: "wizard-agents-show-on-home", url: "/wizard.html#4", note: "the Assistant and the Coding agent on: Show on Home offers both with the Assistant drawn, and picking the Coding agent gives it the card", locator: ".wizard", click: ['[data-agent-tile="assistant"]', '[data-agent-tile="coder"]', '[data-wizard-home-agent="coder"] input'], expectText: "Show on Home", expectHomeAgent: "coder" },
  { name: "wizard-agents-coder-no", url: "/wizard.html#4-coderno", note: "the Coding agent's tile answered No in its dialog: still off, and the step says nothing was changed", locator: ".wizard", click: '[data-agent-tile="coder"]', expectText: "Nothing was changed.", expectSelector: '[data-agent-tile="coder"][aria-pressed="false"]' },
  { name: "wizard-runtime-fresh", url: "/wizard.html#3-nothing", note: "a fresh machine: no policy, nothing stored — Use your Claude subscription leads, the API key one choice away; the ten provider tiles above, Agentry credits locked until a sign-in", locator: ".wizard", expectText: ["Anthropic", "Claude plan or API key", "OpenAI", "GitHub Copilot", "OpenRouter", "Poe", "Hugging Face", "Vultr", "Google Gemini", "Custom endpoint", "Agentry credits"] },
  { name: "wizard-runtime-models", url: "/wizard.html#3-models", note: "after Get models: the endpoint's list", locator: ".wizard", getModels: true },
  { name: "wizard-runtime-nomodels", url: "/wizard.html#3-nomodels", note: "Get models failed: the answer shown, the model typeable", locator: ".wizard", getModels: true },
  { name: "wizard-runtime-subscription", url: "/wizard.html#3-subscription", note: "a stored Anthropic subscription token: what it drives", locator: ".wizard" },
  // The save refused after the key was stored: the
  // custom provider, a URL the backend refuses — the refusal shown, the
  // key field saying the key is on this machine, Save still offered.
  { name: "wizard-runtime-refused", url: "/wizard.html#3-refused", note: "a refused save after the key was stored: the key stays, no re-typing", locator: ".wizard", saveCustom: true },
  // The image pull: the step holds its Continue and
  // Back while it runs, and releases them when it ends or is refused.
  { name: "wizard-runtime-preparing", url: "/wizard.html#3-preparing", note: "images preparing: Continue and Back held until the pull ends", locator: ".wizard", prepare: "held" },
  { name: "wizard-runtime-prepared", url: "/wizard.html#3-prepared", note: "images prepared: Continue and Back released", locator: ".wizard", prepare: "released" },
  { name: "wizard-runtime-prepare-refused", url: "/wizard.html#3-prepare-refused", note: "the pull refused: the reason shown, Continue and Back released", locator: ".wizard", prepare: "released" },
  // Model sign-ins: a sign-in leads, a key second;
  // a kept sign-in says who; waiting in the browser; GitHub's code copied;
  // a connected account. No state asks anyone to paste a code.
  { name: "wizard-runtime-signin-chatgpt", url: "/wizard.html#3-chatgpt", note: "OpenAI with nothing kept: Sign in with ChatGPT leads, the API key second", locator: ".wizard", expectText: ["How to pay for the models", "Sign in with ChatGPT", "API key"] },
  { name: "wizard-runtime-signin-claude", url: "/wizard.html#3-claudekept", note: "the Claude subscription chosen and signed in: who, Remove, the model, Save", locator: ".wizard", expectText: ["Use your Claude subscription (runs Claude Code)", "Signed in as you@example.com (Example Org)", "Remove"] },
  { name: "wizard-runtime-signin-waiting", url: "/wizard.html#3-chatgpt", note: "a ChatGPT sign-in waiting in the browser: Cancel, nothing to paste", locator: ".wizard", click: '.wizard button:has-text("Sign in with ChatGPT")', expectText: ["Waiting for you in your browser…", "Cancel"] },
  { name: "wizard-runtime-signin-copilot", url: "/wizard.html#3-copilotcode", note: "Sign in with GitHub: GitHub's code shown and copied", locator: ".wizard", click: '.wizard button:has-text("Sign in with GitHub")', expectText: ["WDJB-MJHT", "it is copied"] },
  { name: "wizard-runtime-copilot-models", url: "/wizard.html#3-copilotkept", note: "a Copilot sign-in kept: the plan before the model, and the models Copilot CLI lists offered, auto first", locator: ".wizard", click: '.wizard button:has-text("List my plan\'s models")', expectText: ["Your Copilot plan", "auto — Copilot chooses", "claude-opus-5.5 — Claude Opus 5.5, 10× premium requests", "List again"] },
  { name: "wizard-runtime-signin-connected", url: "/wizard.html#3-connected", note: "an OpenRouter account connected: the model typed, Disconnect", locator: ".wizard", expectText: ["Connect your account", "Connected as you", "Disconnect"] },
  // The provider tiles: a provider only a hand-written
  // policy names lights no tile, and the line under them says so.
  // Vultr picked: its key guide above the key field, with the offer, the
  // referral disclosure, the three steps and the sign-up link.
  { name: "wizard-runtime-vultr", url: "/wizard.html#3-nothing", note: "Vultr picked: how to get a key, the new-account offer and the referral disclosure, then the key field", locator: ".wizard", click: '.provider-tile:has-text("Vultr")', expectText: ["Get a Vultr key", "US$300 of free credit", "Agentry receives a referral credit", "Serverless Inference subscription", "Sign up for Vultr with our link"] },
  { name: "wizard-runtime-credits", url: "/wizard.html#3-credits", note: "signed in: Agentry credits picked, the balance and the model in use, and no key or model to give", locator: ".wizard", click: '.provider-tile:has-text("Agentry credits")', expectText: ["Your account has 1,250 credits.", "Agentry runs them on qwen3.8-flash-next now.", "a persistent agent stays up", "How paying with credits works"], expectNoText: ["Please sign in", "Endpoint URL"], expectNone: ".key-source" },
  { name: "wizard-runtime-credits-signedout", url: "/wizard.html#3-nocredits", note: "signed out: the Agentry credits tile greyed with Please sign in across it, and the line under the tiles saying why", locator: ".wizard", expectText: ["Please sign in", "Agentry credits pay for the runs on this computer with your SecureAgentry account: sign in to choose them."], expectSelector: '.provider-tile.is-locked[data-provider="agentry"]:disabled' },
  { name: "wizard-runtime-unlisted", url: "/wizard.html#3-unlisted", note: "a policy naming a provider no tile shows: no tile chosen, the line saying it is paid by a key", locator: ".wizard", expectText: ["Your runner policy names mistral, which is not one of these"] },
  { name: "settings-agent-runs-advanced", url: "/pages.html?settings=provider", note: "Settings → Provider settings with Advanced open: the variable and Forget", locator: '[data-preview="settings"] .card:has(details.advanced)', openAdvanced: true },
  { name: "settings-model-signins-kept", url: "/pages.html?settings=provider#signins", note: "Settings → Model provider: every sign-in and connected account kept on this computer, with Remove and Disconnect", locator: '[data-preview="settings"] .kept-signins', expectText: ["Kept on this computer", "Signed in as chris@example.com", "Connected — the key works until 2026-11-11", "Disconnect"] },
  { name: "settings-agent-runs-custom-http", url: "/pages.html?settings=provider#setup-custom-http", note: "Settings → Provider settings, a custom endpoint over plain http off this machine: saved, and the warning under the URL recommends https", locator: '[data-preview="settings"] .card:has-text("Endpoint URL")', expectText: "uses plain http: the API key and the prompts travel unencrypted to gpu-box.lan" },
  // Settings → Coding tools: the coding agent,
  // the export it implies, its folder, the switch off by default; and
  // the agent page's one-off link, named for the coding agent, opening
  // the wizard on that export.
  // Settings → AI Review key: saved, never shown; none saved; and
  // Test key answered with TypeSafe's refusal in plain words.
  // The card leads with a recommendation and the link to
  // TypeSafe: the sentence word for word in every state, and the
  // link opening the one address, read from what the app asked to open.
  { name: "settings-aireview-key", url: "/pages.html?settings=provider", note: "Settings → AI Review key: the recommendation and the TypeSafe link, then saved in the keychain, the value never shown, Test key and Forget", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', expectText: ["Saved in this computer's keychain.", KEY_RECOMMENDATION, "Get a key from TypeSafe"] },
  { name: "settings-aireview-key-none", url: "/pages.html?settings=provider#aireview-none", note: "no AI Review key: the recommendation, its link opening https://typesafe.ai and nothing else, drafting on this computer is off until one is added; Test key greyed", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', click: '[data-preview="settings"] button:has-text("Get a key from TypeSafe")', expectText: ["Drafting on this computer is off", KEY_RECOMMENDATION, "Get a key from TypeSafe", "Submitting needs no key of yours"], expectOpened: ["https://typesafe.ai"] },
  { name: "settings-aireview-key-env", url: "/pages.html?settings=provider#aireview-env", note: "the key from the variable the app was started with: the recommendation, and the line naming the variable", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', expectText: ["$TYPESAFE_API_KEY is set where this app was started", KEY_RECOMMENDATION, "Get a key from TypeSafe"] },
  { name: "settings-web-search", url: "/pages.html?settings=provider", note: "Settings → Web search: Brave chosen and saved in the keychain, the providers the runner policy admits, the value never shown, Test key and Forget", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', expectText: ["Brave is chosen, and its key is saved in this computer's keychain.", "under that provider's own terms", "Forget the saved Brave key"] },
  { name: "settings-web-search-none", url: "/pages.html?settings=provider#search-none", note: "no search provider: a draft reads the registry and the pages it knows; Test key greyed", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', expectText: "No search provider chosen." },
  { name: "settings-web-search-env", url: "/pages.html?settings=provider#search-env", note: "the key from the variable the app was started with", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', expectText: "$BRAVE_API_KEY is set where this app was started" },
  { name: "settings-hybrid-key", url: "/pages.html?settings=provider", note: "Settings → Hybrid Analysis key: only fingerprints are sent, saved in the keychain, the value never shown, Test key and Forget", locator: '[data-preview="settings"] .card:has-text("Your own CrowdStrike Hybrid Analysis API key")', expectText: ["Only the fingerprints (SHA-256) of risky files are sent to Hybrid Analysis, never the files.", "Saved in this computer's keychain.", "Forget the saved key"] },
  { name: "settings-hybrid-key-none", url: "/pages.html?settings=provider#hybrid-none", note: "no Hybrid Analysis key: nothing is looked up, ClamAV still checks every file; the link opens Hybrid Analysis's sign-up; Test key greyed", locator: '[data-preview="settings"] .card:has-text("Your own CrowdStrike Hybrid Analysis API key")', click: '[data-preview="settings"] .card:has-text("Your own CrowdStrike Hybrid Analysis API key") button:has-text("Get a free key")', expectText: ["Runs on this computer do not look files up on Hybrid Analysis; ClamAV still checks every file."], expectOpened: ["https://hybrid-analysis.com/signup"] },
  { name: "settings-hybrid-key-env", url: "/pages.html?settings=provider#hybrid-env", note: "the key from the variable the app was started with", locator: '[data-preview="settings"] .card:has-text("Your own CrowdStrike Hybrid Analysis API key")', expectText: "$HYBRID_API_KEY is set where this app was started" },
  { name: "settings-hybrid-key-refused", url: "/pages.html?settings=provider#hybrid-refused", note: "Test key: the service refused the key, in plain words, and which key was tried", locator: '[data-preview="settings"] .card:has-text("Your own CrowdStrike Hybrid Analysis API key")', click: '[data-preview="settings"] .card:has-text("Your own CrowdStrike Hybrid Analysis API key") button:has-text("Test key")', expectText: "Hybrid Analysis refused the key." },
  { name: "settings-web-search-refused", url: "/pages.html?settings=provider#search-refused", note: "Test key answered 401: Brave's refusal in plain words, and which key was tried", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', click: '[data-preview="settings"] .card:has-text("Your own key for a web search service") button:has-text("Test key")', expectText: "Brave refused the key (401)" },
  { name: "settings-aireview-key-refused", url: "/pages.html?settings=provider#aireview-refused", note: "Test key answered 401: TypeSafe's refusal in plain words, and which key was tried", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', click: '[data-preview="settings"] button:has-text("Test key")', expectText: "TypeSafe refused the key (401)" },
  { name: "settings-report-design", url: "/pages.html?settings=report-design", note: "Settings → Report design: the five themes, the editor on Plain (never chosen), the contrast line, the live preview; Save offered, no Reset", locator: '[data-preview="settings"]', expectText: ["Themes", "Editor", "Preview", "Starts from Plain.", "You have not saved a design: runs from this app use Plain."], expectNone: '[data-preview="settings"] form, [data-preview="settings"] button:has-text("Reset")' },
  { name: "settings-report-design-saved", url: "/pages.html?settings=report-design#report-saved", note: "a saved design of the person's own: US Letter, a teal accent on Modern's band, the footer line and page numbers in the preview; Reset offered", locator: '[data-preview="settings"]', expectText: ["Starts from Modern.", "Your runs use House (from Modern).", "Acme Ltd · internal"] },
  { name: "settings-report-design-pale", url: "/pages.html?settings=report-design#report-pale", note: "the contrast guard: an accent under what it needs is refused with the nearest passing shade and a Use button; Save stays greyed", locator: '#settings-part-design-editor', expectText: ["so it cannot be saved", "Use #3a6aa6"] },
  { name: "settings-connections", url: "/pages.html?settings=connections", note: "Settings → Connections: each kept connection by version and server, one no longer working, with Disconnect; the person's own Google client with Forget; never a token or a secret", locator: '[data-preview="settings"]', expectText: ["acme/inbox-brief@0.1.0", "acme/mail-digest@1.0.0", "No longer works (invalid_grant: the grant was revoked)", "Your OAuth clients", "secret kept in the OS keychain"] },
  { name: "settings-connections-disconnect", url: "/pages.html?settings=connections", note: "Disconnect on a Google connection whose client another connection shares: the dialog names it, since Google revokes the whole grant (it is still revoked, after the warning)", locator: ".modal", click: '[data-preview="settings"] .card:has-text("acme/inbox-brief") button:has-text("Disconnect")', expectText: ["may revoke everything granted to that client", "Google does", "acme/mail-digest@1.0.0 → pypi:workspace-mcp"] },
  { name: "settings-persistent-agents", url: "/pages.html?settings=persistent-agents", note: "Settings → Persistent Agents: the Assistant by the name given it, stopped, with its switch, Start, Open, Configure and Reset, and its share folder", locator: '[data-preview="settings"]', expectText: ["Sandra", "Stopped", "Enabled", "Start", "Configure", "Reset", "/home/me/agentry/assistantshare"] },
  { name: "settings-persistent-agents-helper", url: "/pages.html?settings=persistent-agents", note: "Settings → Persistent Agents with the Assistant on: the Helper's switch held off, the note that the Assistant does all it does, and the two described as lite and full", locator: '[data-preview="settings"] #settings-part-persistent-agents',
    expectText: ["Agentry Helper is a lite version of it that works only with Agentry", "The lite personal agent", "Sandra is on, and does everything the Helper does", "the full version of Agentry Helper"],
    expectSelector: ['[data-preview="settings"] label:has([data-persistent-enable="main"]:disabled:not(:checked))', '[data-preview="settings"] [data-helper-superseded]'] },
  { name: "settings-persistent-agents-helper-on", url: "/pages.html?settings=persistent-agents&helper=on", note: "the Helper on and the Assistant off: the Helper's switch on and free, and the Assistant's line says turning it on turns the Helper off", locator: '[data-preview="settings"] #settings-part-persistent-agents',
    expectText: "Turning it on turns Agentry Helper off.", expectNone: '[data-preview="settings"] [data-helper-superseded]',
    expectSelector: '[data-preview="settings"] label:has([data-persistent-enable="main"]:checked:not(:disabled))' },
  { name: "settings-persistent-agents-configure", url: "/pages.html?settings=persistent-agents", note: "the Assistant's Configuration: name and personality, the share folder with its picker, knowledge packs, and the four grids of cards with their Add cards", locator: ".modal", click: '[data-preview="settings"] .persistent-agent-card button:has-text("Configure")', expectText: ["Configure the Assistant", "Personality", "Choose a folder…", "Knowledge packs", "APPS AND SERVERS", "MAIL AND CALENDAR", "ACCOUNTS AND SECRETS", "SKILLS", "ABILITIES AND FOLDERS", "Add a server", "Add a secret", "Add a skill", "Wikipedia", "Google Workspace (gog)", "Could not be installed: PyPI has no notes-mcp 0.4.1"] },
  { name: "settings-persistent-agents-mail-disable", url: "/pages.html?settings=persistent-agents", note: "the Assistant's Mail and calendar: Google Workspace's card, its menu's Disable Google, and the confirmation that the sign-in is kept", locator: ".modal", click: ['[data-preview="settings"] .persistent-agent-card button:has-text("Configure")', '.modal [aria-label="Google Workspace (gog): options"]', 'text=Disable Google'], expectText: ["MAIL AND CALENDAR", "Disable Google?", "its sign-in is kept", "Disable", "Cancel"] },
  { name: "settings-persistent-agents-tasks", url: "/pages.html?settings=persistent-agents", note: "the Assistant's Tasks view: each task's schedule in words and as written, its zone, next and last fire with that answer, Pause or Resume and Remove", locator: ".modal >> nth=-1", click: ['[data-preview="settings"] .persistent-agent-card button:has-text("Configure")', '.modal button:has-text("Tasks…")'], expectText: ["Morning summary", "on weekdays at 08:00", "0 8 * * 1-5", "Europe/Oslo", "Three meetings today", "Pause", "Resume", "Remove"] },
  { name: "settings-persistent-agents-calendar", url: "/pages.html?settings=persistent-agents", note: "the Assistant's Mail and calendar: a calendar feed's card by name, service and when its address was checked, and Add a calendar feed's name and masked address", locator: ".modal", click: ['[data-preview="settings"] .persistent-agent-card button:has-text("Configure")', '.modal .config-card-add:has-text("Add a calendar feed")'], expectText: ["Family (calendar feed)", "iCloud · checked", "Add a calendar feed", "A calendar feed is read only: Sandra reads the calendar", "Secret address in iCal format", "Read and keep the feed"], expectNoText: ["p42-caldav.icloud.com/"] },
  { name: "settings-persistent-agents-calendar-rename", url: "/pages.html?settings=persistent-agents", note: "a calendar feed's menu: Rename and Remove, and Rename's field on the card", locator: ".modal", click: ['[data-preview="settings"] .persistent-agent-card button:has-text("Configure")', '.modal [aria-label="Family (calendar feed): options"]', 'text=Rename'], expectSelector: '.modal [aria-label="Rename Family"] input[aria-label="New name"]', expectText: ["Rename", "Cancel"] },
  { name: "settings-persistent-agents-account", url: "/pages.html?settings=persistent-agents", note: "the Coding agent's Add an account: whose account first, its guidance, then the host, user, commit identity and token", locator: ".modal", click: ['[data-preview="settings"] .persistent-agent-card:has-text("Coding agent") button:has-text("Configure")', '.modal .config-card-add:has-text("Add an account")', '.modal label:has-text("Its own account")'], expectText: ["Whose account is it?", "Its own account (recommended)", "taking access away is removing a collaborator", "Commit email", "Keep the account"] },
  { name: "settings-persistent-agents-add-server", url: "/pages.html?settings=persistent-agents", note: "the Assistant's Add a server: its suggestions first, then a package or an address", locator: ".modal", click: ['[data-preview="settings"] .persistent-agent-card button:has-text("Configure")', '.modal .config-card-add:has-text("Add a server")'], expectText: ["Suggested for the Assistant", "Weather (Open-Meteo)", "Maps (OpenStreetMap)", "Add server"] },
  { name: "persistent-agent-page", url: "/pages.html", note: "the Assistant's page: the chat from its transcript in a terminal's manner, the usage panel saying 'not reported' and 'unknown' for what the harness gave nothing, and its cards, with no Add card", locator: '[data-preview="persistent"]', expectText: ["Sandra", "Could you word a note", "1 tool call: fs_read", "Usage", "not reported", "unknown", "What it has", "Share folder", "Wikipedia"] },
  { name: "persistent-agent-page-background", url: "/pages.html", note: "the Assistant's page: what it did on its own folded to one line that counts the runs, never the person's, and opened, each run's own line, its instruction, tool calls and answer", locator: '[data-preview="persistent"] .agent-term-scroll',
    click: ['[data-preview="persistent"] [data-background-runs] > summary', '[data-preview="persistent"] [data-background-run="compaction"] > summary'],
    expectText: ["On its own, task “Inbox check” ran twice, the conversation was compacted once. Latest: A reply from the neighbour came in", "Compacted the conversation: saved it to memory and went on in a fresh one", "Task “Inbox check” ran on its own: No important mail since 19:00.", "Upkeep: compact this conversation (memory-compact).", "2 tool calls: fs_read, fs_write", "ctx: the hallway note was sent"],
    expectNone: '[data-preview="persistent"] .agent-term-person:has-text("Upkeep")' },
  { name: "settings-coding-agents", url: "/pages.html?settings=coding-agents", note: "Settings → Coding tools: Claude Code chosen, the Claude skill in ~/.claude/skills, the switch off", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-codex", url: "/pages.html?settings=coding-agents#coding-codex", note: "Codex chosen: AGENTS.md in the chosen project folder, the switch on", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-none", url: "/pages.html?settings=coding-agents#coding-none", note: "nothing chosen, no folder: AGENTS.md needs a project, the switch cannot go on", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectDisabledSwitch: true },
  { name: "settings-coding-agents-both", url: "/pages.html?settings=coding-agents#coding-both", note: "both chosen: the Claude skill is what is written, so the switch names Claude Code alone", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectSwitchLabel: "Also install into Claude Code" },
  { name: "agent-install-coding-agent", url: "/pages.html", note: "the agent page's one-off link opens the wizard preset to the Claude skill", locator: ".modal", click: '[data-preview="agent"] button:has-text("Install into Claude Code")' },
  // The Welcome's Coding agent step, on the same list Settings uses
  // unchanged for the person.
  { name: "wizard-coding-agent", url: "/wizard.html#6", note: "the welcome's coding agent step after Look: Claude Code found and chosen, Codex missing", locator: ".wizard" },
  // Reached by Skip on the AI Review key step, as a person would.
  { name: "wizard-coding-agent-unprobed", url: "/wizard.html#5", note: "the step as it opens, reached by Skip on the AI Review key step: what the policy records, nothing run", locator: ".wizard", click: '.wizard-foot button:has-text("Skip")', expectText: "Your coding agent" },
  // The AI Review key step: Provider settings' card in the wizard,
  // with Skip only while there is no key. Reached by Continue from Runtime
  // and by Back from Coding agent; and a save the keychain refuses.
  { name: "wizard-review-key", url: "/wizard.html#4", note: "the AI Review key step with no key, reached by Continue from Agents: the recommendation, the TypeSafe link, the field, Skip and Continue", locator: ".wizard", click: '.wizard-foot button:has-text("Continue")', expectText: ["Your AI Review key", KEY_RECOMMENDATION, "Get a key from TypeSafe", "No key saved. Drafting on this computer is off until you add one."], expectOrder: [".wizard-foot button", ["Back", "Skip", "Continue"]] },
  { name: "wizard-review-key-saved", url: "/wizard.html#6-saved", note: "the step with a key saved, reached by Back from Coding agent: saved, never shown, and no Skip", locator: ".wizard", click: '.wizard-foot button:has-text("Back")', expectText: ["Your AI Review key", "Saved in this computer's keychain."], expectOrder: [".wizard-foot button", ["Back", "Continue"]] },
  { name: "wizard-review-key-refused", url: "/wizard.html#5-refused", note: "a save the keychain refuses: the reason shown, no key saved, Skip still offered", locator: ".wizard", fill: [['.wizard input[aria-label="Jev key"]', "tsk-example-not-a-key"]], click: '.wizard button:has-text("Save")', expectText: ["store the key in the OS keychain", "No key saved."], expectOrder: [".wizard-foot button", ["Back", "Skip", "Continue"]], expectValue: ['.wizard input[aria-label="Jev key"]', "tsk-example-not-a-key"] },
  { name: "wizard-review-key-saving", url: "/wizard.html#5-slow", note: "a save in flight: Back, Skip and Continue held until it answers, as the Runtime step holds its own while images pull", locator: ".wizard", fill: [['.wizard input[aria-label="Jev key"]', "tsk-example-not-a-key"]], click: '.wizard button:has-text("Save")', expectText: "Saving…", expectFootDisabled: true },
  // Every page under the shell and every step of the first-run wizard, so a
  // change to the token table is seen everywhere it
  // lands, not only in the states above.
  { name: "page-browse", url: "/pages.html", note: "Browse under the shell, a sub-item of Library shown only inside it; Validate is not listed outside Build", locator: '[data-preview="browse"]', expectOrder: [
    ['[data-preview="browse"] .sidebar .nav-item', ["Home", "Library", "Browse", "Schedule", "Build", "Settings"]],
    ['[data-preview="browse"] .sidebar .nav-item.active', ["Library", "Browse"]],
    ['[data-preview="browse"] .sidebar .nav-item[aria-current="page"]', ["Browse"]],
  ] },
  { name: "page-browse-query", url: "/pages.html#browse-query", note: "Browse opened from the Library's search: the words carried over and searched", locator: '[data-preview="browse"]', expectValue: ['[data-preview="browse"] input[type="search"]', "triage"], expectText: ["1 result", "acme/pr-triage"], expectNoText: "demo1/incident-summarizer" },
  { name: "page-agent", url: "/pages.html", note: "an agent's page under the shell: Library lit, Browse not listed", locator: '[data-preview="agent"]', expectOrder: [
    ['[data-preview="agent"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings"]],
    ['[data-preview="agent"] .sidebar .nav-item.active', ["Library"]],
  ] },
  // Home: the
  // landing, its three cards in the layout's order, each card the start
  // pages' card with a status line and live content in place of a figure.
  { name: "page-home", url: "/pages.html", note: "Home under the shell: the lead line, then Recent runs, Installed QuickAgents, Profile and Run defaults in the layout's order, each opening its page", locator: '[data-preview="home"]',
    expectText: ["Hello, Chris.", "4 QuickAgents on this computer.", "me/csv-quality-check", "agentry/changelog-from-git", "Standard", "40 credits", "On this computer", "15 minutes", "~/Downloads/agentry-out"],
    expectNoText: ["acme/release-notes", "Top up credits"],
    expectOrder: [
      ['[data-preview="home"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings"]],
      ['[data-preview="home"] .sidebar .nav-item[aria-current="page"]', ["Home"]],
      ['[data-preview="home"] .build-way-main', ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults"]],
    ] },
  // Recent runs: this computer's history merged with the account's runs In
  // Agentry, newest first, the built-ins' jobs included.
  { name: "page-home-runs-mixed", url: "/pages.html", note: "Recent runs signed in: runs and built-in jobs on this computer and In Agentry, newest first, five at most", locator: '[data-preview="home"] [data-home-card="recent-runs"]',
    expectText: ["A knowledge build", "In Agentry", "running", "demo1/incident-summarizer", "On this computer", "Finished", "done — pass", "A draft by the Drafter"], expectNoText: ["Sign in to see your runs In Agentry", "acme/pr-triage"],
    expectOrder: [['[data-preview="home"] [data-run-row] > button', ["A knowledge build", "demo1/incident-summarizer", "demo1/incident-summarizer", "A draft by the Drafter", "A review by the Examiner"]]] },
  { name: "page-home-runs-local", url: "/pages.html?active=anon", note: "Recent runs signed out: this computer's runs only, and a line that signing in adds runs In Agentry", locator: '[data-preview="home"] [data-home-card="recent-runs"]',
    expectText: ["demo1/incident-summarizer", "A draft by the Drafter", "A review by the Examiner", "Did not finish", "acme/pr-triage", "Stopped", "Sign in to see your runs In Agentry here too."], expectNoText: ["In Agentry ·", "A knowledge build"] },
  { name: "page-home-runs-builtin", url: "/pages.html?active=anon", note: "a built-in's row opens the run page from the history, by its id", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-run-row="0192a3b4"] button', expectSelector: 'body[data-open="run:local/0192a3b4"]' },
  { name: "page-home-runs-hosted", url: "/pages.html", note: "a run In Agentry's row opens the run page from the registry", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-run-row="0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42"] button', expectSelector: 'body[data-open="run:hosted/0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42"]' },
  { name: "page-home-deliverables", url: "/pages.html", note: "Home's Deliverables card, first: five runs ran and three left files (two PDFs and a page), so three icons, each with the QuickAgent and the day under it, no file name and no button but Open folder", locator: '[data-preview="home"] [data-home-card="deliverables"]',
    expectText: ["Deliverables", "pr-triage", "site-checker", "incident-summarizer", "Open folder"], expectNoText: ["weekly-digest", "page.html", "summary.pdf", "View", "Show in folder", "Open Deliverables folder", "On this computer"],
    expectCount: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable]', 3], expectNoHScroll: '[data-preview="home"] [data-home-card="deliverables"], [data-preview="home"] [data-home-card="deliverables"] *',
    expectOrder: [['[data-preview="home"] [data-home-card] .build-way-title', ["Deliverables", "Recent runs", "Installed QuickAgents", "Profile", "Run defaults"]]]  },
  { name: "page-home-deliverables-names", url: "/pages.html", note: "each icon's accessible name says the file, the agent and the day, and its tooltip is the file name alone", locator: '[data-preview="home"] [data-home-card="deliverables"]',
    expectSelector: ['[data-preview="home"] [data-home-card="deliverables"] button[data-deliverable="reports/weekly-digest.pdf"][title="weekly-digest.pdf"][aria-label^="weekly-digest.pdf, from pr-triage, "]', '[data-preview="home"] [data-home-card="deliverables"] button[data-deliverable="reports/page.html"][data-deliverable-kind="html"][data-deliverable-open][aria-label$="Opens in your system\'s app."]'] },
  { name: "page-home-deliverables-open", url: "/pages.html", note: "a click on a PDF asks Go to open that run's file", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/weekly-digest.pdf"]', expectSelector: 'body[data-opened="0192a3b4:reports/weekly-digest.pdf"]' },
  { name: "page-home-deliverables-open-html", url: "/pages.html", note: "a click on a web page asks Go to open it too, and nothing is shown in a folder", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/page.html"]', expectSelector: 'body[data-opened="0192a3b3:reports/page.html"]', expectNone: 'body[data-shown]' },
  { name: "page-home-deliverables-menu", url: "/pages.html", note: "a right-click on an icon offers Show in folder and opens nothing", locator: '[data-preview="home"] [data-home-card="deliverables"]',
    contextClick: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/summary.pdf"]', expectText: ["Show in folder"],
    expectSelector: '[data-preview="home"] [data-home-card="deliverables"] [role="menu"] [role="menuitem"]', expectNone: 'body[data-opened]' },
  { name: "page-home-deliverables-menu-shown", url: "/pages.html", note: "Show in folder from the menu asks Go to show the file", locator: '[data-preview="home"]',
    contextClick: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/summary.pdf"]', thenClick: '[data-preview="home"] [data-home-card="deliverables"] [role="menuitem"]', expectSelector: 'body[data-shown="9a8b7c6d:reports/summary.pdf"]' },
  { name: "page-home-deliverables-menu-first-row", url: "/pages.html#home-deliverables-many", note: "the menu of an icon in the first row is on top of the row below: Show in folder can be clicked, and the focus goes back to the icon", width: 900, locator: '[data-preview="home"] [data-home-card="deliverables"]',
    contextClick: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/r0.pdf"]',
    thenClick: '[data-preview="home"] [data-home-card="deliverables"] [role="menuitem"]', expectSelector: 'body[data-shown="many0:reports/r0.pdf"]',
    expectFocused: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/r0.pdf"]' },
  { name: "page-home-deliverables-keyboard", url: "/pages.html", note: "Shift+F10 on a focused icon offers Show in folder, the first item focused", locator: '[data-preview="home"] [data-home-card="deliverables"]',
    focusPress: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/summary.pdf"]', "Shift+F10"], expectSelector: '[data-preview="home"] [data-home-card="deliverables"] [role="menu"]', expectFocused: '[data-preview="home"] [data-home-card="deliverables"] [role="menuitem"]' },
  { name: "page-home-deliverables-menukey", url: "/pages.html", note: "the context-menu key does the same", locator: '[data-preview="home"] [data-home-card="deliverables"]',
    focusPress: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/summary.pdf"]', "ContextMenu"], expectSelector: '[data-preview="home"] [data-home-card="deliverables"] [role="menu"]', expectFocused: '[data-preview="home"] [data-home-card="deliverables"] [role="menuitem"]' },
  { name: "page-home-deliverables-kinds", url: "/pages.html#home-deliverables-kinds", note: "one icon of every kind (PDF, web page, document, spreadsheet, presentation, image, text, Markdown), and the plain icon with a folder for an executable, a macro document and a PDF that is not one", locator: '[data-preview="home"] [data-home-card="deliverables"]',
    expectCount: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable]', 11], expectSelector: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="pdf"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="html"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="document"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="spreadsheet"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="presentation"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="image"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="text"]', '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable-kind="other"]'],
    expectNoHScroll: '[data-preview="home"] [data-home-card="deliverables"], [data-preview="home"] [data-home-card="deliverables"] *' },
  { name: "page-home-deliverables-reveal", url: "/pages.html#home-deliverables-kinds", note: "a click on an executable only shows it in its folder: Go is never asked to open it", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-home-card="deliverables"] [data-deliverable="reports/setup.exe"]', expectSelector: 'body[data-shown="kind8:reports/setup.exe"]', expectNone: 'body[data-opened]' },
  { name: "page-home-deliverables-many", url: "/pages.html#home-deliverables-many", note: "twelve files, a wrapping grid", locator: '[data-preview="home"] [data-home-card="deliverables"]', expectCount: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable]', 12], expectNoHScroll: '[data-preview="home"] [data-home-card="deliverables"], [data-preview="home"] [data-home-card="deliverables"] *' },
  { name: "page-home-deliverables-narrow", url: "/pages.html#home-deliverables-many", note: "the narrowest window: the grid wraps and nothing scrolls sideways", width: 900, locator: '[data-preview="home"] [data-home-card="deliverables"]', expectCount: ['[data-preview="home"] [data-home-card="deliverables"] [data-deliverable]', 12], expectNoHScroll: '[data-preview="home"] [data-home-card="deliverables"], [data-preview="home"] [data-home-card="deliverables"] *' },
  { name: "page-home-deliverables-folder", url: "/pages.html", note: "Open folder asks Go to show the folder", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-open-deliverables-folder]', expectSelector: 'body[data-folder-opened="1"]' },
  { name: "page-home-deliverables-empty", url: "/pages.html#home-deliverables-empty", note: "no deliverable yet: the card says what will show there", locator: '[data-preview="home"] [data-home-card="deliverables"]', expectText: "Nothing yet. The files a run was for are listed here, newest first.", expectNoText: ["Reading deliverables"] },
  { name: "page-home-deliverables-off", url: "/pages.html#home-deliverables-off", note: "the switch is off: the card says where it is, and offers no folder button", locator: '[data-preview="home"] [data-home-card="deliverables"]', expectText: "Deliverables are off.", expectNoText: ["Open folder"] },
  { name: "page-home-deliverables-hostile", url: "/pages.html#home-deliverables-hostile", note: "a QuickAgent name carrying markup and a right-to-left override, a name of 160 characters and an unbroken agent name: drawn as text, the override revealed, the layout held", width: 900, locator: '[data-preview="home"] [data-home-card="deliverables"]',
    expectText: ["<img src=x onerror=alert(1)>", "U+202E"], expectSelector: '[data-preview="home"] [data-home-card="deliverables"] [aria-label*="from Q3 _gnp.exe"]', expectNone: '[data-preview="home"] [data-home-card="deliverables"] script, [data-preview="home"] [data-home-card="deliverables"] img, [data-preview="home"] [data-home-card="deliverables"] [aria-label*="\\202e"]', expectNoHScroll: '[data-preview="home"] [data-home-card="deliverables"], [data-preview="home"] [data-home-card="deliverables"] *' },
  { name: "page-home-runs-empty", url: "/pages.html#home-runs-empty", note: "no run yet: the card says what will show there", locator: '[data-preview="home"] [data-home-card="recent-runs"]', expectText: "No runs yet." },
  { name: "page-home-runs-off", url: "/pages.html?active=anon#home-runs-off", note: "Remember recent runs off, signed out: nothing is kept, and the card says where the switch is", locator: '[data-preview="home"] [data-home-card="recent-runs"]', expectText: "Remember recent runs is off in Run defaults." },
  { name: "page-home-runs-card", url: "/pages.html", note: "a press on the Recent runs card outside a row opens Run defaults' Recent runs", locator: '[data-preview="home"]', click: '[data-preview="home"] [data-home-card="recent-runs"] .build-way-title', expectSelector: 'body[data-open="run-history"]' },
  { name: "page-home-pro", url: "/pages.html#profile-pro", note: "a Pro account the registry offers packs to: its plan, its balance and Top up credits", locator: '[data-preview="home"]', expectText: ["Pro", "1,380 credits", "Top up credits"] },
  { name: "page-home-updates", url: "/pages.html#installed-updates", note: "the launch check found newer versions: the Installed card counts them", locator: '[data-preview="home"]', expectText: "4 QuickAgents on this computer, 2 with an update." },
  { name: "page-home-open-agent", url: "/pages.html", note: "a name on the Installed card opens its page, from the registry it was installed from", locator: '[data-preview="home"]', click: '[data-preview="home"] [data-home-card="installed"] .home-card-list button:has-text("demo1/incident-summarizer")', expectSelector: 'body[data-open="agent:https://secureagentry.net/demo1/incident-summarizer"]' },
  { name: "page-home-open-local", url: "/pages.html", note: "a QuickAgent installed from a folder has no page: its name opens the Library", locator: '[data-preview="home"]', click: '[data-preview="home"] [data-home-card="installed"] .home-card-list button:has-text("me/csv-quality-check")', expectSelector: 'body[data-open="library"]' },
  { name: "page-home-open-card", url: "/pages.html", note: "a press anywhere on the Run defaults card opens Settings → Run defaults", locator: '[data-preview="home"]', click: '[data-preview="home"] [data-home-card="run-defaults"] .build-way', expectSelector: 'body[data-open="run-defaults"]' },
  { name: "page-home-signed-out", url: "/pages.html?active=anon", note: "signed out: no greeting, and the Profile card is a Sign in card", locator: '[data-preview="home"]', expectText: ["Sign in to agentry:8080", "Your recent runs, what is installed on this computer", "4 QuickAgents on this computer.", "me/csv-quality-check", "On this computer", "15 minutes"], expectNoText: ["Hello,", "Reading your profile"],
    expectOrder: [['[data-preview="home"] .build-way-main', ["Recent runs", "Installed QuickAgents", "Sign in", "Run defaults"]]] },
  { name: "page-home-hidden", url: "/pages.html#home-hidden", note: "a card hidden in the layout is not drawn; the others keep their order", locator: '[data-preview="home"]',
    expectOrder: [['[data-preview="home"] .build-way-main', ["Recent runs", "Profile", "Run defaults"]]] },
  { name: "page-home-newer", url: "/pages.html#home-newer", note: "a layout a newer Agentry Desktop arranged: the default cards, and a line that it is kept", locator: '[data-preview="home"]', expectText: "Home was arranged by a newer Agentry Desktop.",
    expectOrder: [['[data-preview="home"] .build-way-main', ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults"]]] },
  { name: "page-home-broken", url: "/pages.html#home-broken", note: "a layout that cannot be read: the default cards, and the reason", locator: '[data-preview="home"]', expectText: "Home's layout could not be read, so the default cards are shown",
    expectOrder: [['[data-preview="home"] .build-way-main', ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults"]]] },
  { name: "page-home-unreadable", url: "/pages.html#home-unreadable", note: "a layout this app cannot decode: the default cards, and a line that the file is left as it is", locator: '[data-preview="home"]', expectText: "Home arrangement cannot be read by this Agentry Desktop",
    expectOrder: [['[data-preview="home"] .build-way-main', ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults"]]] },
  { name: "page-home-strict-scheduled", url: "/pages.html#home-strict-scheduled", note: "Home with the Scheduled card still starts nothing: the card's one read (HomeCard) is the only binding added, and no other is called", locator: '[data-preview="home"]', wait: 1500, expectText: ["2 schedules on this computer.", "Morning digest"], expectNone: "body[data-unexpected]" },
  { name: "page-home-strict", url: "/pages.html#home-strict", note: "Home starts nothing on load: every binding but the reads it makes throws, and none is called", locator: '[data-preview="home"]', wait: 1500, expectText: ["4 QuickAgents on this computer.", "Standard", "15 minutes"], expectNone: "body[data-unexpected]" },
  { name: "page-home-setup", url: "/pages.html#home-setup", note: "the setup notice App raises shows on Home, above the cards", locator: '[data-preview="home"]', expectText: "Setup isn't finished." },
  { name: "page-home-empty", url: "/pages.html#fresh", note: "nothing installed on this computer: the way to Browse", locator: '[data-preview="home"]', expectText: ["Nothing installed on this computer yet.", "Find more QuickAgents"] },
  { name: "page-home-unsafe", url: "/pages.html#unsafe-on", note: "Unsafe Mode on: the Run defaults card says so", locator: '[data-preview="home"]', expectText: "Unsafe Mode is on" },
  // Pins: four to a Pinned card,
  // Large, after the default cards; each row's latest run, Open latest run
  // and Run; a pin whose QuickAgent is gone offers Unpin.
  { name: "page-home-pin-one", url: "/pages.html#home-pin-one", note: "one pin: a Pinned card after the default cards, its QuickAgent's version and how its latest run ended", locator: '[data-preview="home"]',
    expectText: ["Pinned", "demo1/incident-summarizer", "v1.2.0", "Open latest run"], expectSelector: '[data-preview="home"] [data-pin="demo1/incident-summarizer"] [data-pin-run="7c1e4a90"]', expectNone: '[data-preview="home"] [data-home-card="pinned"] >> nth=1' },
  { name: "page-home-pin-never", url: "/pages.html#home-pin-never", note: "a pin whose QuickAgent has never run: Not run yet, and Run but no Open latest run", locator: '[data-preview="home"] [data-home-card="pinned"]',
    expectText: ["agentry/changelog-from-git", "Not run yet", "Run"], expectNone: '[data-preview="home"] [data-home-card="pinned"] button:has-text("Open latest run")' },
  { name: "page-home-pins-four", url: "/pages.html#home-pins-four", note: "four pins share one card: a run that finished, one stopped, one never run, a folder install", locator: '[data-preview="home"]',
    expectText: ["demo1/incident-summarizer", "acme/pr-triage", "agentry/changelog-from-git", "me/csv-quality-check", "Not run yet", "Stopped"], expectNone: '[data-preview="home"] [data-home-card="pinned"] >> nth=1' },
  { name: "page-home-pins-five", url: "/pages.html#home-pins-five", note: "a fifth pin is in a second Pinned card; this one's QuickAgent is no longer on this computer, so it offers Unpin", locator: '[data-preview="home"]',
    expectText: ["acme/release-notes", "No longer installed on this computer."], expectSelector: '[data-preview="home"] [data-home-card="pinned"] >> nth=1' },
  { name: "page-home-pin-unpin-gone", url: "/pages.html#home-pins-five", note: "Unpin on a pin whose QuickAgent is gone: the pin goes, and the card it emptied with it", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-pin="acme/release-notes"] button:has-text("Unpin")', expectNone: '[data-preview="home"] [data-home-card="pinned"] >> nth=1' },
  { name: "page-home-pin-open-run", url: "/pages.html#home-pin-one", note: "Open latest run opens the run page of the pin's newest run, by its id", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-pin="demo1/incident-summarizer"] button:has-text("Open latest run")', expectSelector: 'body[data-open="run:local/7c1e4a90"]' },
  { name: "page-home-pin-run", url: "/pages.html#home-pin-one", note: "Run opens the QuickAgent's page with its run setup, from the registry it was installed from", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-pin="demo1/incident-summarizer"] button:text-is("Run")', expectSelector: 'body[data-open="run-agent:https://secureagentry.net/demo1/incident-summarizer"]' },
  { name: "page-home-pin-run-local", url: "/pages.html#home-pins-four", note: "Run on a folder install's pin opens its run setup over Home, as the Library's does", locator: ".modal",
    click: '[data-preview="home"] [data-pin="me/csv-quality-check"] button:text-is("Run")', expectText: ["Run me/csv-quality-check@0.1.0", "It was installed from a folder on this computer"] },
  { name: "page-scheduled", url: "/pages.html", note: "the Scheduled page: October's grid with dots that ran, failed, were missed and are due, the chosen day, Upcoming, each schedule's actions and the History, which says what happened and never reads as done when nothing ran", locator: '[data-preview="scheduled"]',
    expectText: ["Schedule", "Schedule a new run", "October 2026", "Morning digest", "every day at 09:00 (Europe/London)", "Runs while you are signed out too", "Paused", "Resume", "Run now", "Missed: the computer was off", "Failed (docker, exit 13)", "Upcoming", "History"],
    expectOrder: [['[data-preview="scheduled"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings"]]],
    expectSelector: '[data-preview="scheduled"] .cal-dot.tone-bad' },
  { name: "page-scheduled-hosted", url: "/pages.html#scheduled-hosted", note: "a schedule In Agentry beside this computer's: labelled In Agentry in the list, the calendar, Upcoming and History; a fire that started a run says so, never done, and one skipped for credits says nothing ran", locator: '[data-preview="scheduled"]',
    expectText: ["Weekly report", "In Agentry", "Runs In Agentry, whether or not this computer is on", "Run started", "Skipped: there were not enough credits. Nothing ran."], expectSelector: '[data-preview="scheduled"] [data-hosted-schedule] button:text-is("Run now")' },
  { name: "page-scheduled-hosted-unreachable", url: "/pages.html#scheduled-hosted-unreachable", note: "a registry that does not answer: the sentence says so, and this computer's schedules are listed and work as they do", locator: '[data-preview="scheduled"]',
    expectText: ["Your schedules In Agentry could not be read", "Morning digest"], expectSelector: '[data-preview="scheduled"] [data-schedule="ab12cd34ef"] button:text-is("Run now"):not([disabled])' },
  { name: "page-scheduled-hosted-standard", url: "/pages.html#scheduled-hosted-standard", note: "a Standard account: the plan's sentence and Credits and upgrade; Schedule a new run stays for this computer", locator: '[data-preview="scheduled"]',
    expectText: ["Scheduling runs In Agentry is part of Pro, Business and Enterprise.", "Credits and upgrade"] },
  { name: "page-scheduled-hosted-signedout", url: "/pages.html#scheduled-hosted-signedout", note: "signed out of the registry: a line says signing in shows the schedules In Agentry, with Sign in", locator: '[data-preview="scheduled"]',
    click: '[data-preview="scheduled"] [data-hosted-signed-out] button', expectText: "Sign in to secureagentry.net to see your schedules In Agentry", expectSelector: 'body[data-open="sign-in"]' },
  { name: "page-scheduled-hosted-edit", url: "/pages.html#scheduled-hosted", note: "Edit a schedule In Agentry: its name and repeat, no signed-out box", locator: ".modal",
    click: '[data-preview="scheduled"] [data-hosted-schedule] button:text-is("Edit")', expectText: ["Edit Weekly report", "Agentry checks it again when you save"], expectNone: ".modal [data-signed-out]" },
  { name: "page-scheduled-hosted-runnow", url: "/pages.html#scheduled-hosted", note: "Run now In Agentry opens the run's own page In Agentry", locator: '[data-preview="scheduled"]',
    click: '[data-preview="scheduled"] [data-hosted-schedule] button:text-is("Run now")', expectSelector: 'body[data-open="hosted-run:0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42"]' },
  { name: "page-scheduled-pick", url: "/pages.html", note: "Schedule a new run picks an installed QuickAgent; choosing one opens its page with the setup on Repeat", locator: ".modal",
    click: '[data-preview="scheduled"] button:text-is("Schedule a new run")', expectText: ["Schedule a new run", "When on Repeat"] },
  { name: "page-scheduled-edit", url: "/pages.html", note: "Edit a schedule: its name, its repeat as it was, the signed-out box", locator: ".modal",
    click: '[data-preview="scheduled"] [data-schedule="ab12cd34ef"] button:text-is("Edit")', expectText: ["Edit Morning digest", "Also run when I'm signed out", "It keeps running the version it was made for"] },
  // Pin to Home.
  { name: "page-scheduled-menu", url: "/pages.html", note: "the Scheduled page's options menu offers Pin to Home while Home has no Scheduled card", locator: '[data-preview="scheduled"] .page-header',
    click: '[data-preview="scheduled"] [data-options-menu]', expectSelector: '[data-preview="scheduled"] [role="menu"] [data-menu-item="pin-home"]', expectText: "Pin to Home" },
  { name: "page-scheduled-nocli", url: "/pages.html#scheduled-nocli", note: "no agentry command on this computer (macOS before an install): the notice says to install it from Options, and the menu offers Install the agentry command", locator: '[data-preview="scheduled"]',
    click: '[data-preview="scheduled"] [data-options-menu]',
    expectText: ["needs the agentry command, which was not found", "To install it, choose Options, then Install the agentry command."], expectSelector: '[data-preview="scheduled"] [role="menu"] [data-menu-item="install-cli"]:text-is("Install the agentry command")' },
  { name: "page-scheduled-install-cli", url: "/pages.html#scheduled-nocli", note: "Install the agentry command: Go runs the site's install script after a native dialog; the page says where it went and what the script changed, the notice goes, and the menu now offers Reinstall", locator: '[data-preview="scheduled"]',
    click: ['[data-preview="scheduled"] [data-options-menu]', '[data-preview="scheduled"] [data-menu-item="install-cli"]', '[data-preview="scheduled"] [data-options-menu]'],
    expectText: ["Installed agentry 0.6.93 at /Users/me/.local/bin/agentry.", "Added /Users/me/.local/bin to PATH in /Users/me/.zshrc", "What the install script printed"],
    expectSelector: '[data-preview="scheduled"] [data-menu-item="install-cli"]:text-is("Reinstall the agentry command")', expectNone: '[data-preview="scheduled"] [data-scheduled-unavailable]' },
  { name: "page-scheduled-pin", url: "/pages.html", note: "Pin to Home: Go adds the Large Scheduled card at the end of Home, and the menu now offers Unpin", locator: '[data-preview="scheduled"]',
    click: ['[data-preview="scheduled"] [data-options-menu]', '[data-preview="scheduled"] [data-menu-item="pin-home"]', '[data-preview="scheduled"] [data-options-menu]'],
    expectText: "Pinned to Home: this month's calendar and the next three runs.", expectSelector: '[data-preview="scheduled"] [data-menu-item="unpin-home"]' },
  { name: "page-scheduled-unpin", url: "/pages.html#home-scheduled", note: "Unpin from Home, with the card on Home: Go removes it", locator: '[data-preview="scheduled"]',
    click: ['[data-preview="scheduled"] [data-options-menu]', '[data-preview="scheduled"] [data-menu-item="unpin-home"]', '[data-preview="scheduled"] [data-options-menu]'],
    expectText: "Unpinned from Home.", expectSelector: '[data-preview="scheduled"] [data-menu-item="pin-home"]' },
  { name: "page-home-scheduled", url: "/pages.html#home-scheduled", note: "Home's Scheduled card (Large): October's mini calendar with what ran, failed, was missed and is due, and the next three occurrences", locator: '[data-preview="home"] [data-home-card="scheduled"]',
    expectText: ["Scheduled", "October 2026", "2 schedules on this computer.", "Next", "Morning digest"],
    expectOrder: [['[data-preview="home"] .build-way-main', ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults", "Scheduled"]], ['[data-preview="home"] [data-scheduled-next] li .small:first-child', ["Sat 10 Oct, 09:00", "Sun 11 Oct, 09:00", "Mon 12 Oct, 09:00"]]],
    expectSelector: '[data-preview="home"] [data-home-card="scheduled"] [data-month-grid="mini"] .cal-dot.tone-bad' },
  // Home's grid: a card dragged onto another takes its place, a corner
  // dragged snaps the card's size, and both handles take the arrow keys.
  { name: "page-home-grid", url: "/pages.html", note: "Home's grid: four columns, each card two by two until it is sized, with its move and resize handles on hover", locator: '[data-preview="home"] .home-grid',
    expectSelector: '[data-preview="home"] .home-grid[data-home-columns="4"] [data-home-card="main"][data-cols="2"][data-rows="2"]' },
  { name: "page-home-drag", url: "/pages.html", note: "Run defaults dragged onto the agent card takes its place, and Go is asked to move it there", locator: '[data-preview="home"] .home-grid',
    drag: ['[data-preview="home"] [data-home-card="run-defaults"] .build-way-title', '[data-preview="home"] [data-home-card="main"] .build-way-title'],
    expectSelector: '[data-preview="home"] .home-grid > .home-card-cell:first-child[data-home-card="run-defaults"]' },
  { name: "page-home-resize", url: "/pages.html", note: "Recent runs' corner dragged right and down: it snaps one column wider and one row taller", locator: '[data-preview="home"] .home-grid',
    dragBy: ['[data-preview="home"] [data-home-resize="recent-runs"]', 260, 200],
    expectSelector: '[data-preview="home"] [data-home-card="recent-runs"][data-cols="3"][data-rows="3"]' },
  { name: "page-home-agent-assistant", url: "/pages.html?homeagent=assistant", note: "Home's agent card given the Assistant: its name, its own chat, and its switcher", wait: 1000, locator: '[data-preview="home"] [data-home-card="main"]',
    expectSelector: '[data-preview="home"] [data-home-agent="assistant"]', expectText: ["Sandra", "Could you word a note asking my neighbour"] },
  { name: "page-home-agent-default", url: "/pages.html", note: "the Assistant on and the Helper off: the card draws the Assistant, and its switcher does not offer the Helper", wait: 1000, locator: '[data-preview="home"] [data-home-card="main"]',
    expectSelector: '[data-preview="home"] [data-home-agent="assistant"]', expectNone: '[data-preview="home"] [data-home-agent-switch] option[value="main"]' },
  { name: "page-home-agent-helper", url: "/pages.html?helper=on", note: "the Helper on and the Assistant off: the card draws the Helper", wait: 1000, locator: '[data-preview="home"] [data-home-card="main"]',
    expectSelector: '[data-preview="home"] [data-home-agent="main"]' },
  { name: "page-home-agent-none", url: "/pages.html?helper=none", note: "neither the Helper nor the Assistant on: Home draws no agent card", wait: 1000, locator: '[data-preview="home"] .home-grid',
    expectNone: '[data-preview="home"] [data-home-card="main"]' },
  { name: "page-home-agent-switch", url: "/pages.html?helper=on", note: "the card's switcher set to the Assistant: Go is asked to turn it on and give the card the Assistant, and the card draws it", locator: '[data-preview="home"] [data-home-card="main"]',
    select: [['[data-preview="home"] [data-home-agent-switch]', "assistant"]], expectHomeAgent: "assistant", expectSelector: '[data-preview="home"] [data-home-agent="assistant"]' },
  { name: "page-home-agent-off", url: "/pages.html?homeagent=butler", note: "the card given an agent this app does not list: the Assistant, which is on, is drawn in its place", wait: 1000, locator: '[data-preview="home"] [data-home-card="main"]',
    expectSelector: '[data-preview="home"] [data-home-agent="assistant"]' },
  { name: "page-home-keys", url: "/pages.html", note: "the keyboard: the move handle's Right arrow moves the agent card one place later", locator: '[data-preview="home"] .home-grid',
    focusPress: ['[data-preview="home"] [data-home-grip="main"]', "ArrowRight"],
    expectSelector: 'body[data-home-moved="main 0->1"]' },
  { name: "page-home-keys-resize", url: "/pages.html", note: "the keyboard: the resize handle's Down arrow makes Profile one row taller", locator: '[data-preview="home"] .home-grid',
    focusPress: ['[data-preview="home"] [data-home-resize="profile"]', "ArrowDown"],
    expectSelector: '[data-preview="home"] [data-home-card="profile"][data-cols="2"][data-rows="3"]' },
  { name: "page-home-narrow", url: "/pages.html", note: "a narrow window: two columns, every card at most two wide", locator: '[data-preview="home"] .home-grid', width: 820,
    expectSelector: '[data-preview="home"] .home-grid[data-home-columns="2"]' },
  // Settings → Home → Theme: a tile per theme, and a choice that is put on
  // the window at once and kept by Go.
  { name: "settings-home-agent", url: "/pages.html?settings=home&helper=on", note: "Agent on Home: the agents as tiles, one radio group, Agentry Helper chosen; picking the Assistant asks Go to turn it on and give the card it", locator: '#settings-part-home-agent',
    click: '#settings-part-home-agent [data-agent-tile="assistant"]', expectHomeAgent: "assistant", expectSelector: '#settings-part-home-agent [data-agent-tile="assistant"][aria-checked="true"]' },
  { name: "settings-home-theme", url: "/pages.html?settings=home", note: "the Theme part first in Settings → Home: System chosen, a tile per theme with its swatch and line", locator: '[data-preview="settings"] #settings-part-theme',
    expectOrder: [['[data-preview="settings"] .theme-choice strong', ["System", "Light", "Dark", "Slate", "Forest", "Midnight", "High contrast"]]],
    expectChecked: '[data-preview="settings"] [data-theme-choice="system"] input' },
  { name: "settings-home-theme-chosen", url: "/pages.html?settings=home", note: "Slate chosen: the window is drawn in it at once and Go is asked to keep it", locator: '[data-preview="settings"]', window: true,
    click: '[data-preview="settings"] [data-theme-choice="slate"]',
    expectSelector: 'html[data-theme="slate"] body[data-theme-kept="slate"]' },
  // Every theme over Home and over a dense Settings page, in the light and
  // dark the system can be in.
  { name: "theme-light-home", url: "/pages.html?theme=light", note: "Home in the light theme", locator: '[data-preview="home"]', window: true, expectSelector: 'html[data-theme="light"]' },
  { name: "theme-light-settings", url: "/pages.html?theme=light&settings=run-defaults", note: "Run defaults, a dense page of fields, badges and notes, in the light theme", locator: '[data-preview="settings"]', window: true, expectSelector: 'html[data-theme="light"]' },
  { name: "theme-dark-home", url: "/pages.html?theme=dark", note: "Home in the dark theme", locator: '[data-preview="home"]', window: true, expectSelector: 'html[data-theme="dark"]' },
  { name: "theme-dark-settings", url: "/pages.html?theme=dark&settings=run-defaults", note: "Run defaults, a dense page of fields, badges and notes, in the dark theme", locator: '[data-preview="settings"]', window: true, expectSelector: 'html[data-theme="dark"]' },
  { name: "theme-slate-home", url: "/pages.html?theme=slate", note: "Home in the slate theme", locator: '[data-preview="home"]', window: true, expectSelector: 'html[data-theme="slate"]' },
  { name: "theme-slate-settings", url: "/pages.html?theme=slate&settings=run-defaults", note: "Run defaults, a dense page of fields, badges and notes, in the slate theme", locator: '[data-preview="settings"]', window: true, expectSelector: 'html[data-theme="slate"]' },
  { name: "theme-forest-home", url: "/pages.html?theme=forest", note: "Home in the forest theme", locator: '[data-preview="home"]', window: true, expectSelector: 'html[data-theme="forest"]' },
  { name: "theme-forest-settings", url: "/pages.html?theme=forest&settings=run-defaults", note: "Run defaults, a dense page of fields, badges and notes, in the forest theme", locator: '[data-preview="settings"]', window: true, expectSelector: 'html[data-theme="forest"]' },
  { name: "theme-midnight-home", url: "/pages.html?theme=midnight", note: "Home in the midnight theme", locator: '[data-preview="home"]', window: true, expectSelector: 'html[data-theme="midnight"]' },
  { name: "theme-midnight-settings", url: "/pages.html?theme=midnight&settings=run-defaults", note: "Run defaults, a dense page of fields, badges and notes, in the midnight theme", locator: '[data-preview="settings"]', window: true, expectSelector: 'html[data-theme="midnight"]' },
  { name: "theme-contrast-home", url: "/pages.html?theme=contrast", note: "Home in the contrast theme", locator: '[data-preview="home"]', window: true, expectSelector: 'html[data-theme="contrast"]' },
  { name: "theme-contrast-settings", url: "/pages.html?theme=contrast&settings=run-defaults", note: "Run defaults, a dense page of fields, badges and notes, in the contrast theme", locator: '[data-preview="settings"]', window: true, expectSelector: 'html[data-theme="contrast"]' },
  { name: "page-home-scheduled-open", url: "/pages.html#home-scheduled", note: "a press anywhere on the Scheduled card opens the Scheduled page", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-home-card="scheduled"] .build-way', expectSelector: 'body[data-open="scheduled"]' },
  { name: "page-home-scheduled-hosted", url: "/pages.html#home-scheduled-hosted", note: "the Scheduled card with a schedule In Agentry merged: both counted, the month's dots from both, the next three from both, the hosted one marked", locator: '[data-preview="home"] [data-home-card="scheduled"]',
    expectText: ["2 schedules on this computer, 1 In Agentry.", "Weekly report"], expectSelector: '[data-preview="home"] [data-home-card="scheduled"] [data-in-agentry]' },
  { name: "page-home-scheduled-empty", url: "/pages.html#home-scheduled-empty", note: "the Scheduled card with nothing scheduled: an empty month and where to start", locator: '[data-preview="home"] [data-home-card="scheduled"]',
    expectText: ["Nothing is scheduled on this computer yet.", "Schedule a run from the Schedule page."] },
  { name: "page-home-scheduled-nocli", url: "/pages.html#home-scheduled-nocli", note: "no agentry command on this computer: the card says why, not just an empty month", locator: '[data-preview="home"] [data-home-card="scheduled"]',
    expectText: "Scheduling a run on this computer needs the agentry command, which was not found." },
  { name: "page-library-pin", url: "/pages.html", note: "Pin to Home on a Library row: Go places the pin, and the button turns to Unpin from Home", locator: '[data-preview="library"]',
    click: '[data-preview="library"] button[data-pin-button="pin"] >> nth=0', expectSelector: '[data-preview="library"] button[data-pin-button="unpin"]' },
  { name: "page-library-pins-full", url: "/pages.html#home-pins-sixteen", note: "Home holds 16 pins: Pin is held on every row not pinned, with the sentence that says why; a pinned one still offers Unpin", locator: '[data-preview="library"]',
    expectText: ["Home holds 16 pinned QuickAgents, 4 to a Pinned card. Unpin one to pin another.", "Unpin from Home"], expectSelector: '[data-preview="library"] button[data-pin-button="full"][disabled]' },
  { name: "page-home-pins-sixteen", url: "/pages.html#home-pins-sixteen", note: "four Pinned cards of four: the most Home holds", locator: '[data-preview="home"]',
    expectSelector: '[data-preview="home"] [data-home-card="pinned"] >> nth=3', expectNone: '[data-preview="home"] [data-home-card="pinned"] >> nth=4' },
  { name: "page-library", url: "/pages.html", note: "the Library under the shell (Installed and Browse in one section): what is installed on this computer, with a QuickAgent installed from a folder (local and unsigned, Run in place of Details), the search and the way on to Browse", locator: '[data-preview="library"]', expectText: ["The QuickAgents installed on this computer.", "Installed on this computer", "me/csv-quality-check", "local, unsigned", "from a folder on this computer; no registry holds it", "Find more QuickAgents"], expectOrder: [
    ['[data-preview="library"] .sidebar .nav-item', ["Home", "Library", "Browse", "Schedule", "Build", "Settings"]],
    ['[data-preview="library"] .sidebar .nav-item[aria-current="page"]', ["Library"]],
  ] },
  { name: "page-library-run-local", url: "/pages.html", note: "Run on the QuickAgent installed from a folder opens its run setup over the Library, from its own contract", locator: ".modal", click: '[data-preview="library"] button:text-is("Run")', expectText: ["Run me/csv-quality-check@0.1.0", "Step 1 of 4: Before you start", "It was installed from a folder on this computer"] },
  { name: "page-library-search", url: "/pages.html", note: "the Library's search filters what is installed as it is typed, and the button carries the words to Browse", locator: '[data-preview="library"]', fill: [['[data-preview="library"] input[type="search"]', "acme"]], expectText: ["acme/pr-triage", "acme/release-notes", "Search the registry for \u201cacme\u201d"], expectNoText: ["demo1/incident-summarizer", "me/csv-quality-check", "This app has installed nothing"] },
  { name: "page-library-no-match", url: "/pages.html", note: "a search nothing installed matches: said so, with the way to the registry", locator: '[data-preview="library"]', fill: [['[data-preview="library"] input[type="search"]', "pagerduty"]], expectText: ["Nothing installed on this computer matches \u201cpagerduty\u201d", "The registry may have it."] },
  { name: "page-library-to-browse", url: "/pages.html", note: "the registry button hands Browse what was typed, trimmed", locator: '[data-preview="library"]', fill: [['[data-preview="library"] input[type="search"]', "  pager duty "]], click: '[data-preview="library"] .toolbar button:has-text("Search the registry")', expectSelector: 'body[data-browse="pager duty"]' },
  { name: "page-library-uninstall", url: "/pages.html", note: "Uninstall from the Library still asks first, naming what it removes", locator: "body", click: '[data-preview="library"] button:text-is("Uninstall") >> nth=0', expectText: ["Uninstall demo1/incident-summarizer?", "This removes"] },
  { name: "page-library-forget", url: "/pages.html", note: "Forget from the Library drops only the record of a directory that is gone", locator: '[data-preview="library"]', click: '[data-preview="library"] button:text-is("Forget")', expectText: "Forgot acme/release-notes; nothing on disk was touched." },
  // The Publisher section:
  // pat's QuickAgents with their badges and actions, the sidebar's Publisher
  // item for a signed-in registry, an organization pat is a member of, an
  // old sign-in, the owner actions and Delete's typed name.
  { name: "page-publisher", url: "/pages.html", note: "the Publisher section: pat's QuickAgents with Private, Retired and Fork badges, the Fork of line, a fork's Make public withheld, the counts, and Publisher in the sidebar under Build", locator: '[data-preview="publisher"]',
    expectText: ["The QuickAgents you publish", "4 QuickAgents · 2 private · 1 retired", "pat/incident-digest", "Fork of demo1/incident-summarizer@1.0.0", "stays private", "A fork stays private for good, so it cannot be made public.", "Make private", "Make public", "Restore", "Retire"],
    expectNoText: ["archive", "Archive"],
    expectOrder: [
      ['[data-preview="publisher"] .sidebar .nav-item', ["Home", "Library", "Browse", "Publisher", "Schedule", "Build", "Settings"]],
      ['[data-preview="publisher"] .sidebar .nav-item[aria-current="page"]', ["Publisher"]],
      ['[data-preview="publisher"] .agent-row:has-text("pat/incident-summarizer") .badge', ["Private", "Fork"]],
    ] },
  { name: "page-publisher-retire", url: "/pages.html", note: "Retire on a public QuickAgent: the sentence Go returns, and the row now Retired with Restore", locator: '[data-preview="publisher"]',
    click: '[data-preview="publisher"] .agent-row:has-text("pat/incident-digest") button:text-is("Retire")',
    expectText: ["pat/incident-digest is retired. It is private, takes no new versions, and Restore brings it back as it was.", "4 QuickAgents · 2 private · 2 retired"] },
  { name: "page-publisher-delete", url: "/pages.html", note: "Delete asks for the typed name before anything is sent: the button waits for it", locator: "body",
    click: '[data-preview="publisher"] .agent-row:has-text("pat/csv-quality-check") button:text-is("Delete")',
    expectText: ["Delete pat/csv-quality-check?", "Every version of pat/csv-quality-check is yanked", "no QuickAgent in pat can be called csv-quality-check again", "Type csv-quality-check to confirm"],
    expectDisabledButton: "Delete" },
  { name: "page-publisher-deleted", url: "/pages.html", note: "the name typed and Enter pressed: deleted, said so, and gone from the list", locator: '[data-preview="publisher"]',
    click: '[data-preview="publisher"] .agent-row:has-text("pat/csv-quality-check") button:text-is("Delete")',
    type: [["#publisher-delete-name", "csv-quality-check\n"]],
    expectText: ["pat/csv-quality-check is deleted.", "3 QuickAgents · 1 private · 1 retired"] },
  { name: "page-publisher-member", url: "/pages.html#publisher-crew", note: "an organization pat is a member of: Edit stays, the owner actions are the admins'", locator: '[data-preview="publisher"]',
    expectText: ["Only admins of crew can make its QuickAgents public or private, retire, restore or delete them.", "crew/standup-notes", "Edit"],
    expectNone: '[data-preview="publisher"] .agent-row button:text-is("Retire")' },
  { name: "page-publisher-old-signin", url: "/pages.html#publisher-old-signin", note: "a sign-in without agent:manage: the section says to sign in again and offers no owner action", locator: '[data-preview="publisher"]',
    expectText: "This sign-in was made before Agentry Desktop could retire, restore, delete or change who sees a QuickAgent.",
    expectNone: '[data-preview="publisher"] .agent-row button:text-is("Retire")' },
  { name: "page-publisher-signed-out", url: "/pages.html#publisher-signed-out", note: "signed out: no Publisher in the sidebar, and the page says to sign in", locator: '[data-preview="publisher"]',
    expectText: "Sign in to manage what you publish",
    expectOrder: [['[data-preview="publisher"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings"]]] },
  // Fork in the desktop: Fork and the fork count on a
  // public QuickAgent's page, the dialog with the site's three facts, a
  // taken name asking for another, a fork's own page, and the Publisher
  // section following a new fork to its published first version.
  { name: "page-agent-fork", url: "/pages.html", note: "a public QuickAgent's page, signed in: Fork beside Install, and its fork count", locator: '[data-preview="agent"]',
    expectText: ["Fork", "2 forks"] },
  { name: "page-agent-fork-dialog", url: "/pages.html", note: "Fork's dialog: the namespace (yours first), the site's three facts about a fork, and no name field until one is needed", locator: ".modal",
    click: '[data-preview="agent"] button:text-is("Fork")',
    expectText: ["Fork demo1/incident-summarizer", "Fork into", "pat (yours)", "A fork stays private for good.", "Runs on your own computer pay nobody."],
    expectNone: "#fork-name" },
  { name: "page-agent-fork-taken", url: "/pages.html#agent-fork-taken", note: "the name is taken in the namespace: the refusal in the site's words, and a name field to choose another", locator: ".modal",
    click: ['[data-preview="agent"] button:text-is("Fork")', '.modal button:has-text("Fork into")'],
    expectText: ["Not forked: pat already has a QuickAgent called incident-summarizer. Choose another name for the fork.", "Name for the fork"],
    expectSelector: "#fork-name" },
  { name: "page-agent-fork-of", url: "/pages.html#agent-fork", note: "a fork's page: Fork of its origin (a link while it can be read), its copy under way, and no Fork", locator: '[data-preview="agent"]',
    expectText: ["Fork of demo1/incident-summarizer@1.2.0", "A fork stays private for good, so it cannot be made public.", "This fork is being made.", "Its history is being copied from demo1/incident-summarizer and its first version submitted to the pipeline. This page updates by itself."],
    expectNone: '[data-preview="agent"] button:text-is("Fork")' },
  { name: "page-publisher-follow", url: "/pages.html#publisher-follow", note: "after Fork, the Publisher section on the fork's namespace, following its copy and its first version to published", locator: '[data-preview="publisher"]',
    wait: 9500,
    expectText: ["Forked demo1/incident-summarizer into pat/incident-summarizer.", "pat/incident-summarizer's first version, v1.2.0, is published. It is private, and yours to change."] },
  { name: "page-library-empty", url: "/pages.html#fresh", note: "nothing installed on this computer: the way to Browse", locator: '[data-preview="library"]', expectText: ["Nothing installed on this computer yet", "Find more QuickAgents"] },
  { name: "page-build", url: "/pages.html", note: "Build under the shell: the start page's two cards, Describe it to AI first, then Build it by hand, with the same words and tagline as the site's /build", locator: '[data-preview="build"]', expectText: [BUILD_TAGLINE, DESCRIBE_COPY, BY_HAND_COPY, "Switch to Advanced"], expectOrder: [
    ['[data-preview="build"] .build-way-main', ["Describe it to AI", "Build it by hand"]],
    ['[data-preview="build"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "QuickAgent", "Validate", "Voice", "Knowledge", "Settings"]],
    ['[data-preview="build"] .sidebar .nav-item.active', ["Build", "QuickAgent"]],
  ] },
  // The cards open their views: a press anywhere on Build it by hand
  // opens Step by step, and its Switch to Advanced opens Advanced.
  { name: "page-build-by-hand", url: "/pages.html", note: "Build it by hand pressed in the middle of the card: Step by step on Basics", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-way:nth-child(2)', expectText: "Step 1 of 8: Basics" },
  { name: "page-build-by-hand-advanced", url: "/pages.html", note: "Build it by hand's Switch to Advanced: every section at once", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-way-part', expectText: "Switch to step by step" },
  // The Build tab's views:
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
  { name: "page-build-guide-ships", url: "/pages.html#build-guide-ships", note: "Before a run's Knowledge section for shipped packs: a ready pack ticked, one still being read, one past the cap and an organization's pack for another namespace each unticked with the reason, and a pack the loaded folder already holds", locator: '[data-preview="build"]', expectText: ["Knowledge it ships", "wait until AI Review has read it", "ships only into a QuickAgent in the acme namespace, and this one is in demo1", "Ticked 2 of up to 5"] },
  { name: "page-build-guide-hostile", url: "/pages.html#build-guide-hostile", note: "Before a run's preview with a hostile key help: markdown, framed as the author's; no markup of its own, no image, no javascript: link; each link's host shown, a look-alike as punycode; a link cut at the hidden override", locator: '[data-preview="build"]', linksIn: '[data-preview="build"] .key-help', expectText: ["The author's help for", "U+202E", "(evil.example)", "(xn--pgerduty-16g.com)", "(pagerduty.com.evil.example)"], expectLinks: ["https://evil.example/pd", "https://support.pagerduty.com/keys", "https://xn--pgerduty-16g.com/new", "https://pagerduty.com.evil.example/"], expectNone: '[data-preview="build"] .key-help img, [data-preview="build"] .key-help script, [data-preview="build"] .key-help b[onclick], [data-preview="build"] .key-help a[href^="javascript"]' },
  { name: "page-build-review-hostile", url: "/pages.html#build-review-hostile", note: "Review with the same hostile key help beneath Before a run", locator: '[data-preview="build"]', linksIn: '[data-preview="build"] .key-help', expectText: ["The author's help for", "U+202E", "(evil.example)", "(pagerduty.com.evil.example)"], expectLinks: ["https://evil.example/pd", "https://support.pagerduty.com/keys", "https://xn--pgerduty-16g.com/new", "https://pagerduty.com.evil.example/"], expectNone: '[data-preview="build"] .key-help img, [data-preview="build"] .key-help script, [data-preview="build"] .key-help b[onclick], [data-preview="build"] .key-help a[href^="javascript"]' },
  { name: "page-build-review", url: "/pages.html#build-review", note: "Step by step: Review, the recap with Edit and the actions", locator: '[data-preview="build"]', expectText: "Step 8 of 8: Review", expectMinWidth: ['[data-preview="build"] .step-recap dd', 320] },
  { name: "page-build-advanced", url: "/pages.html#build-advanced", note: "Advanced: every section at once", locator: '[data-preview="build"]', expectText: ["Before a run", "The first save writes it to a new folder named for it in your output folder"] },
  // Describe it, on this computer: the description with the person's
  // AI Review key saved, and without one (the Settings pointer, Draft it
  // greyed); a draft on Review with the Drafter's notes; a hostile draft
  // whose notes, match, source and withheld address must stay text.
  { name: "page-build-describe", url: "/pages.html#build-describe", note: "Describe it: the description, the hints, on this computer with the policy's model, the key from the keychain, the search line and where the description goes", locator: '[data-preview="build"]', expectText: "AI Review uses your key from this computer's keychain" },
  { name: "page-build-describe-nokey", url: "/pages.html#build-describe-nokey", note: "Describe it with no AI Review key: the Settings pointer, the TypeSafe link, and Draft it greyed", locator: '[data-preview="build"]', click: '[data-preview="build"] button:has-text("Get a key from TypeSafe")', expectText: "needs your own TypeSafe Jev key", expectOpened: ["https://typesafe.ai"] },
  { name: "page-build-drafted", url: "/pages.html#build-drafted", note: "a draft on Review, How it was drafted opened: AI Review's check of the finished draft, what AI Review read and held back, the registry's match with its page, the open questions, the Drafter's notes, the sources", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["AI Review read 6 results with your key and held back 0", "The release check: flagged.", "reaching for something the contract does not declare (C3, score 3) in AGENT.md", "This finding sends the version to review when you submit"] },
  { name: "page-build-drafted-hostile", url: "/pages.html#build-drafted-hostile", note: "a hostile draft on Review, How it was drafted opened: no markup of its own, no image, no javascript: link, the withheld address and the check's hostile finding as text, the override shown", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["U+202E", "<img src=x onerror=alert(1)>"], expectNone: '[data-preview="build"] .card img, [data-preview="build"] .card script, [data-preview="build"] .card iframe, [data-preview="build"] .card b[onclick], [data-preview="build"] .card a[href^="javascript"], [data-preview="build"] .card a[href*="evil.example"]' },
  // Describe it's Knowledge pick: on this computer the
  // packs kept here, a pack with no scan in Agentry ticked and the warning
  // that AI Review scans it first with the person's key; a pick past 10 MB
  // refused before Draft it; In Agentry the packs Agentry keeps, one kept
  // only here greyed with why; Review naming the packs a draft read and
  // what AI Review withheld from one.
  { name: "page-build-describe-knowledge", url: "/pages.html#build-describe-knowledge", note: "Describe it on this computer with the Knowledge pick: a pack scanned in Agentry and one kept only here ticked, the total, and the warning that AI Review scans the second first with your key", locator: '[data-preview="build"]', click: ['[data-preview="build"] label:has-text("On this computer") input', '[data-preview="build"] label:has-text("m6-dev-runbook") input', '[data-preview="build"] label:has-text("team-notes") input'], expectText: ["Knowledge, if you keep some", "Picked 2 of up to 5", "team-notes has not been scanned in Agentry, so AI Review reads every file of it with your key"] },
  { name: "page-build-describe-knowledge-over", url: "/pages.html#build-describe-knowledge-over", note: "a pick past 10 MB: the sentence, and Draft it greyed", locator: '[data-preview="build"]', click: ['[data-preview="build"] label:has-text("On this computer") input', '[data-preview="build"] label:has-text("manuals") input', '[data-preview="build"] label:has-text("catalogue") input'], expectText: "These packs hold 11 MB, and a draft reads at most 10 MB of knowledge in all" },
  { name: "page-build-describe-agentry-knowledge", url: "/pages.html#build-describe-agentry-knowledge", note: "Describe it In Agentry with the Knowledge pick: the packs Agentry keeps, an organization's shared one, and a pack kept only here greyed with why", locator: '[data-preview="build"]', click: '[data-preview="build"] label:has-text("handbook") input', expectText: ["shared by acme", "kept on this computer only: upload it to Agentry to draft with it there", "Picked 1 of up to 5"] },
  { name: "page-build-drafted-knowledge", url: "/pages.html#build-drafted-knowledge", note: "a draft that read two packs, How it was drafted opened: one scanned in Agentry, one scanned with your key with a page withheld and why", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["The knowledge it read", "scanned in Agentry; nothing withheld", "scanned with your key for this draft; 1 file withheld", "wiki/topics/override.md"] },
  // Describe it In Agentry: the hold in the site's words, the caps
  // and the balance; a payer short of the hold; a draft queued and
  // followed; signed out; a draft made In Agentry on Review, with Describe
  // again under the notes.
  { name: "page-build-describe-agentry", url: "/pages.html#build-describe-agentry", note: "Describe it In Agentry: the hold sentence, the balance, the caps, the honest line, On this computer beside it", locator: '[data-preview="build"]', expectText: "This draft holds 41 credits and settles from what it uses" },
  { name: "page-build-describe-agentry-short", url: "/pages.html#build-describe-agentry-short", note: "In Agentry for a payer short of the hold: the notice and Credits and upgrade", locator: '[data-preview="build"]', expectText: "fewer credits than a draft holds" },
  { name: "page-build-describe-agentry-drafting", url: "/pages.html#build-describe-agentry-drafting", note: "Describe it while a draft In Agentry runs: the form held, and Show its progress, which opens the draft's dialog", locator: '[data-preview="build"]', expectText: ["A draft is being made In Agentry", "Show its progress"] },
  { name: "page-build-describe-signedout", url: "/pages.html#build-describe-signedout", note: "In Agentry signed out: offered, and it needs an account", locator: '[data-preview="build"]', expectText: "needs an account" },
  { name: "page-build-drafted-agentry", url: "/pages.html#build-drafted-agentry", note: "a draft made In Agentry on Review, How it was drafted opened: AI Review through the registry, a host the description did not name, a rule it still fails, and Describe again", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["Hosts your description did not name", "Describe again"] },
  // The summary: how it
  // works and every kind of capability the model declares, Submit private
  // unless switched, See it in the form; a landed draft's check in one line;
  // hostile text drawn as text; a rule still failing, with Fix it.
  { name: "page-build-summary", url: "/pages.html#build-summary", note: "the summary of a QuickAgent written in the form: how it works, its key with the author's help, its website, its MCP server with its tools, host and configuration, its file and data grants; Save, Install and Submit, and what each does", locator: '[data-preview="build"]', expectText: ["How it works", "incident-summarizer", "PAGERDUTY_TOKEN, for the QuickAgent", "The author's help for PAGERDUTY_TOKEN", "api.pagerduty.com", "npm:@cyanheads/osv-advisory-mcp-server@0.1.13", "May call all of its tools.", "Configuration: LOG_LEVEL.", "api.osv.dev", "Reads the files in its workspace.", "Writes reports in its workspace.", "Touches these kinds of data: tickets.", "Save writes it to a new folder named for it in your output folder", "puts it on this computer only, in your Library", "makes it public", "It first says what the submission costs"], expectNone: '[data-preview="build"] .build-summary .step-problem, [data-preview="build"] .build-summary input[type="checkbox"]', expectOrder: [['[data-preview="build"] .build-summary .row button', ["Save", "Install", "Submit"]]] },
  // Save, Install and Submit: each dialog is the Go
  // side's and native, so the harness shows what the page says after a Yes.
  { name: "page-build-tools-remote", url: "/pages.html#build-tools-remote", note: "Tools and web access with a remote server signed in to by OAuth, one by a key, and a package server's sign-in: the rows, the reach and the server checks", locator: '[data-preview="build"]', expectText: ["+ add remote server", "signs in with", "runs at its vendor, reached at mcp.linear.app; tools list_issues; signs in with OAuth, asking for scopes read", "signs in with the key GITHUB_TOKEN", "signs in with OAuth at https://accounts.google.com", "token files, written into the server's own folder", "It runs at its vendor, so there is no package to check."] },
  { name: "page-build-summary-remote", url: "/pages.html#build-summary-remote", note: "the summary with a remote server signed in to by OAuth, one by a key, and a package server's sign-in, each with its sentences and its template whole", locator: '[data-preview="build"]', expectText: ["https://mcp.linear.app/mcp", "Runs at its vendor. Agentry checks its address and what it asks for, not the vendor's code.", "allow these scopes: read. The run gets an access token, nothing else.", "Signs in with the key GITHUB_TOKEN, sent to the vendor as a bearer token.", "Signs in with OAuth at https://accounts.google.com", "Token file me@agentry.invalid.json:", "Token settings: WORKSPACE_MCP_CREDENTIALS_DIR={dir}."] },
  { name: "page-build-summary-review", url: "/pages.html#build-summary", note: "the summary's Review Your QuickAgents locally beside Save, Install and Submit, with what it does: the registry's rules here, AI Review with your key, your model, no credits, it asks first and submits nothing", locator: '[data-preview="build"]', expectText: ["Review Your QuickAgents locally", "the registry scans again at Submit", "openai qwen3.8-flash-next"], expectOrder: [['[data-preview="build"] .build-summary .row button', ["Save", "Install", "Submit"]]] },
  { name: "page-build-summary-nokey", url: "/pages.html#build-summary-nokey", note: "the summary without an AI Review key: the action says to add one, with the way to Provider settings, and Submit is the same", locator: '[data-preview="build"]', expectText: ["AI Review needs your own TypeSafe key", "Add your AI Review key"], expectNone: '[data-preview="build"] .review-action button:text-is("Review Your QuickAgents locally")' },
  { name: "page-build-summary-saved", url: "/pages.html#build-summary", note: "after Save: written with no file dialog to a new folder named for it under the default output folder", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Save")', expectText: ["Saved to /home/me/Downloads/agentry-out/incident-summarizer", "Save writes it to /home/me/Downloads/agentry-out/incident-summarizer"] },
  { name: "page-build-summary-installed", url: "/pages.html#build-summary", note: "after Install: written to the default output folder with no file dialog, then laid down on this computer, local and unsigned, with the way to the Library", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Install")', expectText: ["installed demo1/incident-summarizer@0.1.0 on this computer — local and unsigned", "No registry holds it and nothing was submitted.", "Open Library", "Save writes it to /home/me/Downloads/agentry-out/incident-summarizer"] },
  { name: "page-build-summary-submitted", url: "/pages.html#build-summary", note: "after Submit: written to the default output folder with no file dialog, then published, and public", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Submit")', expectText: ["published demo1/incident-summarizer@0.1.0", "It is public.", "Save writes it to /home/me/Downloads/agentry-out/incident-summarizer"] },
  { name: "page-build-summary-private", url: "/pages.html#build-summary-private", note: "after Submit to an agent that exists private: it stays private, why, and the way to its settings on the site", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Submit")', expectText: ["it stays private: making a QuickAgent public is its namespace administrators' act", "Its settings on the site"] },
  { name: "page-build-landed", url: "/pages.html#build-landed", note: "a draft that landed while the tab was away, opened as Build mounts: the draft stays in the form though the fresh form's NewModel answers after it", locator: '[data-preview="build"]', expectText: ["csv-quality-check"], expectNone: '[data-preview="build"] .build-summary:has-text("no name yet")' },
  { name: "page-build-summary-drafted", url: "/pages.html#build-summary-drafted", note: "a draft landed on the summary: AI Review's check in one line, the host the description did not name marked, no Drafter's notes or counts", locator: '[data-preview="build"]', expectText: ["flagged something the release check looks for", "It reaches hosts your description did not name", "your description did not name it", "csv-quality-check"], expectNone: '[data-preview="build"] .build-drafted-notes, [data-preview="build"] .build-summary ul[aria-label="Keys"], [data-preview="build"] .build-summary ul[aria-label="MCP servers"]' },
  { name: "page-build-summary-hostile", url: "/pages.html#build-summary-hostile", note: "a hostile overview, key help and host on the summary: no markup of its own, no image, no javascript: link, the override shown", locator: '[data-preview="build"]', expectText: ["U+202E", "<img src=x onerror=alert(1)>"], expectNone: '[data-preview="build"] .build-summary img, [data-preview="build"] .build-summary script, [data-preview="build"] .build-summary b[onclick], [data-preview="build"] .build-summary a[href^="javascript"]' },
  { name: "page-build-summary-problem", url: "/pages.html#build-summary-problem", note: "the summary of a QuickAgent a rule still refuses: the rule named with Fix it in its step, Submit greyed", locator: '[data-preview="build"]', expectText: ["It cannot be submitted yet. Before a run:", "Fix it in Before a run"], expectDisabledSummarySubmit: true },
  { name: "page-build-summary-public", url: "/pages.html#build-summary-public", note: "a folder whose publish.include leaves out README.md: Submit waits with Fix it in Publishing, Save and Install do not, since only a submission is public", locator: '[data-preview="build"]', expectText: ["It can be saved and installed, but not submitted yet: a submission is public.", "Fix it in Publishing"], expectDisabledSubmitOnly: true },
  { name: "page-build-comments", url: "/pages.html#build-comments", note: "a loaded agent.yaml with comments: the notice that a save drops them", locator: '[data-preview="build"]', expectText: "the comments are not kept" },
  { name: "page-voice", url: "/pages.html", note: "the Voice page as two cards: Local Voice and Cloud Voice, each a figure over its words and its status line; no Voice Writer card", locator: '[data-preview="voice"]',
    expectText: ["Local Voice", "Cloud Voice", "The voice kept on this computer. Edit it by hand or with AI for free", "AI Review checks every version before it goes live", "Voice set · updated", "· used by default.", "Live since", "a new version was flagged by AI Review"],
    expectNoText: ["Start the Voice Writer", "Submit to Agentry"],
    expectOrder: ['[data-preview="voice"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "QuickAgent", "Voice", "Knowledge", "Settings"]] },
  { name: "page-voice-none", url: "/pages.html#voice-none", note: "the two cards with no voice in either place", locator: '[data-preview="voice"]', expectText: ["No voice on this computer yet.", "No voice in your account yet."] },
  { name: "page-voice-signed-out", url: "/pages.html#voice-signed-out", note: "the two cards signed out: Cloud Voice asks for a sign-in", locator: '[data-preview="voice"]', expectText: ["No voice on this computer yet.", "Sign in to keep a voice in your account."] },
  { name: "page-voice-local", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")'], note: "the Local Voice page: Download is Already the same (the digests match), Upload says it asks first because the account's flagged version is newer and costs 1 credit, Edit with AI, the switch shared with Run defaults, the editor and Delete", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Download Cloud version", "Already the same.", "Upload to Cloud", "for 1 credit", "so you are asked first", "Edit with AI", "Use this voice for every run on this computer", "the same switch as Settings → Run defaults", "Delete this computer's voice"] },
  { name: "page-voice-local-none", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")'], note: "the Local Voice page with no voice: Write with AI, Download with no live voice, Upload with nothing to upload, the empty editor", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Write with AI", "Your account has no live voice yet.", "This computer keeps no voice to upload yet.", "No voice on this computer yet."], expectNoText: ["Delete this computer's voice"] },
  // The WYSIWYG editor: Save on an untouched editor sends nothing, on either page (on the Cloud page it would be a credit); hostile Markdown in a voice stays text, with no element of its own and no address to open; a voice the editor cannot hold stays in the textarea.
  { name: "page-voice-local-unchanged", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] .voice-editor button:text-is("Save")'], note: "Save on the Local page's editor, untouched: nothing is saved, and it says so", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Nothing to save: the voice is as it was."] },
  { name: "page-voice-cloud-unchanged", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")', '[data-preview="voice"] .voice-editor button:text-is("Save")'], note: "Save on the Cloud page's editor, untouched: nothing goes to AI Review and no credit is spent", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Nothing to save: the voice is as it was."] },
  { name: "page-voice-local-hostile", url: "/pages.html#voice-hostile", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")'], note: "a voice holding raw HTML, a script, an image, a javascript: link and invisible characters: all of it text in the editor, the one https link drawn with no href, each invisible character a chip, the override isolated", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg a[data-href="https://example.com/style"][data-title="Read it first"]',
    expectText: ["<script>alert(1)</script>", '<b onclick="x()">bold</b>', "![tracking pixel](https://evil.example/p.gif)", "[run me](javascript:alert(1))", "R&D stays R&D.", "U+200B", "U+202E", "U+202C", "U+E0041"],
    expectNone: '[data-preview="voice"] .voice-wysiwyg :is(img, script, b, iframe, [href], [onclick])' },
  { name: "page-voice-cloud-refresh", url: "/pages.html#voice-reviewing", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], type: [['[data-preview="voice"] .voice-wysiwyg', " Kept across a refresh."]], wait: 6000, note: "a version in review, the page reading the account every 5 seconds: what the person typed in the editor is still there after a refresh that changed nothing", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Kept across a refresh."] },
  { name: "page-voice-local-table", url: "/pages.html#voice-table", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")'], note: "a voice holding a table, which the editor cannot hold: the plain textarea stays, so no edit can drop it", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-editor[data-editor="plain"] textarea[aria-label="VOICE.md"]',
    expectText: ["This voice holds a part the editor cannot show (a table, a definition or a task list), so it opens as Markdown text, and Save keeps every part."], expectNone: '[data-preview="voice"] .voice-wysiwyg' },
  { name: "page-voice-dialog", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] button:has-text("Edit with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")'], note: "Edit with AI: what should change, two samples from the native picker, the cost line, Start held until the note is written", locator: '[data-preview="voice"]',
    expectText: ["What should change?", "Add writing samples (optional)", "/home/me/writing/launch-email.md", "Free, on this computer"] },
  { name: "page-voice-dialog-none", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] button:has-text("Write with AI")'], note: "Write with AI with no voice here: samples required, the text an optional brief", locator: '[data-preview="voice"]',
    expectText: ["How do you want to sound? (optional)", "Samples of your writing", "Free, on this computer"] },
  { name: "page-voice-dialog-failed", url: "/pages.html#voice-failed", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] button:has-text("Write with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")', '[data-preview="voice"] .modal button:text-is("Start")', '[data-preview="voice"] .modal summary:has-text("The last run\'s log")'], note: "a Write with AI run that failed: the error, and the run's log kept under it so the person can see why", locator: '[data-preview="voice"]',
    expectText: ["the Voice Writer wrote no VOICE.md", "The last run's log", "decode: 1 files"] },
  { name: "page-voice-cloud", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page, the site's Voice settings in the desktop: the flagged new version with its finding and Discard, the editor on it with Edit with AI In Agentry under it, the switch shared with Profile, Restore previous (reviewed again: the question set changed) and Delete; no Download or Upload here", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Your new version", "Flagged", "AI Review flagged this version, so your live voice stays live.", "It asks for more than a way of writing", "Discard this version", "Your voice", "An AI Edit, 2026-10-01 22:20 UTC, reviewed 2026-10-01 22:21 UTC. 316 bytes.", "The editor holds your newest version", "Edit with AI", "It runs In Agentry.", "Use this voice for every run In Agentry", "the same switch as Settings → Profile", "Restore previous", "The previous version, written by the Voice Writer, is from 2026-10-01 20:00 UTC.", "so Restore sends it to AI Review again, for 1 credit. It takes the place of the version waiting above.", "Delete voice"],
    expectNoText: ["Use Agentry's voice here", "Download Cloud version", "Upload to Cloud"] },
  { name: "page-voice-cloud-none", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page with no voice in the account: the empty editor and Write with AI, no switch, Restore or Delete", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Your account keeps no Writing Voice yet.", "Write with AI", "The Voice Writer writes your voice from samples of your writing. It runs In Agentry."], expectNoText: ["Use by default", "Delete voice", "Restore previous"] },
  { name: "page-voice-cloud-reviewing", url: "/pages.html#voice-reviewing", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "a first version in review with nothing live: the page follows it every 5 seconds, the switch says no run writes in it yet, no Restore", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["Your first version", "In review", "AI Review is reading this version. It goes live when it passes, and this page updates itself.", "Your voice is not live yet", "Delete voice"], expectNoText: ["Restore previous", "Flagged"] },
  { name: "page-voice-cloud-dialog", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")', '[data-preview="voice"] .voice-cloud-ai button:has-text("Edit with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")'], note: "Edit with AI In Agentry: what should change, two samples from the native picker with the upload limits, the price and that it waits for AI Review, Start held until the note is written", locator: '[data-preview="voice"]',
    expectText: ["What should change?", "Add writing samples (optional)", "/home/me/writing/launch-email.md", "at most 20 files, 10 MB together", "It costs 20 credits, AI Review included.", "It rewrites your newest version", "waits for AI Review"], expectNoText: ["Free, on this computer"] },
  { name: "page-voice-cloud-running", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")', '[data-preview="voice"] .voice-cloud-ai button:has-text("Write with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")', '[data-preview="voice"] .modal button:text-is("Start")'], note: "a Write with AI run In Agentry started: its status, Cancel run, and Close, since the page keeps following it", locator: '[data-preview="voice"]',
    expectText: ["The Voice Writer is running In Agentry", "Queued: waiting for a sandbox host.", "Cancel run", "You can close this"] },
  { name: "page-voice-cloud-attached", url: "/pages.html#voice-attached", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page opened again while an AI Edit In Agentry runs: the run followed again, Show it, and Edit with AI held rather than offering a second", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["The Voice Writer is running In Agentry: Running: the Voice Writer is reading your voice and your samples.", "Show it"] },
  { name: "page-voice-standard", url: "/pages.html#voice-standard", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page on Standard with this cycle's Voice Writer run used: Edit with AI held, the date the next is theirs, and the upgrade", locator: '[data-preview="voice"]', expectSelector: '[data-preview="voice"] .voice-wysiwyg',
    expectText: ["this cycle's runs are used. The next is yours on 28 October 2026.", "Upgrade"] },
  { name: "page-validate", url: "/pages.html", note: "Validate under the shell: a sub-item of Build, with Build lit beside it", locator: '[data-preview="validate"]', expectOrder: [
    ['[data-preview="validate"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "QuickAgent", "Validate", "Voice", "Knowledge", "Settings"]],
    ['[data-preview="validate"] .sidebar .nav-item.active', ["Build", "QuickAgent", "Validate"]],
    ['[data-preview="validate"] .sidebar [aria-current="page"]', ["Validate"]],
  ] },
  // Settings as five cards: the landing with each
  // card's status line; each section page; a card opened, a part's link
  // followed, the way back; and each deep link App gives a caller.
  { name: "page-settings", url: "/pages.html", note: "Settings under the shell: five cards, each with its sentence, its status line and its parts", locator: '[data-preview="settings"]',
    expectText: ["Registry settings", "Update settings", "Coding tools", "Persistent Agents", "Run defaults", "Provider settings", "secureagentry.net, signed in.", "Agentry Desktop 0.2.0 is available.", "AI Review key saved", "Runs stop after 15 minutes, on this computer.", "Settings file:"],
    expectSection: "landing" },
  // Settings → Profile: offered only while the
  // active registry holds a token, first on the landing.
  { name: "settings-landing-profile", url: "/pages.html", note: "signed in: the Profile card first on the landing", locator: '[data-preview="settings"]',
    expectOrder: ['[data-preview="settings"] .settings-card-link strong', ["Profile", "Registry settings", "Update settings", "Coding tools", "Persistent Agents", "Run defaults", "Provider settings", "Connections", "Home"]] },
  // The sidebar on Settings: the landing lights Settings and lists every
  // section beneath it; a section's row opens it and is the one marked open.
  { name: "sidebar-settings-sections", url: "/pages.html", note: "the sidebar on the Settings landing: Settings lit with its twelve sections beneath it", locator: '[data-preview="settings"] .sidebar',
    expectOrder: [
      ['[data-preview="settings"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings", "Profile", "Report design", "Registry settings", "Update settings", "Coding tools", "Persistent Agents", "Run defaults", "Provider settings", "Connections", "Destinations", "Home", "Licences"]],
      ['[data-preview="settings"] .sidebar .nav-item[aria-current="page"]', ["Settings"]],
    ] },
  { name: "sidebar-settings-section-open", url: "/pages.html", note: "a section's row in the sidebar opens that section, and only it is marked open", locator: '[data-preview="settings"]', click: '[data-preview="settings"] .sidebar [data-nav="section:run-defaults"]',
    expectSelector: '[data-preview="settings"] [data-settings-section="run-defaults"]',
    expectOrder: [
      ['[data-preview="settings"] .sidebar .nav-item[aria-current="page"]', ["Run defaults"]],
      ['[data-preview="settings"] .sidebar .nav-item.active', ["Settings", "Run defaults"]],
    ] },
  { name: "sidebar-settings-signed-out", url: "/pages.html?active=anon", note: "signed out: Profile is not listed beneath Settings, as the landing shows no Profile card", locator: '[data-preview="settings"] .sidebar',
    expectOrder: [['[data-preview="settings"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings", "Report design", "Registry settings", "Update settings", "Coding tools", "Persistent Agents", "Run defaults", "Provider settings", "Connections", "Destinations", "Home", "Licences"]]] },
  { name: "sidebar-build-collapse", url: "/pages.html", note: "Build is a section, not a page: on QuickAgent a click on Build collapses its rows and Build stays lit", locator: '[data-preview="build"] .sidebar', click: '[data-preview="build"] .sidebar [data-nav="build"]',
    expectSelector: '[data-preview="build"] .sidebar [data-nav="build"][aria-expanded="false"]',
    expectOrder: [
      ['[data-preview="build"] .sidebar .nav-item', ["Home", "Library", "Schedule", "Build", "Settings"]],
      ['[data-preview="build"] .sidebar .nav-item.active', ["Build"]],
    ] },
  { name: "settings-landing-signed-out", url: "/pages.html?active=anon", note: "the active registry not signed in: no Profile card", locator: '[data-preview="settings"]',
    expectOrder: ['[data-preview="settings"] .settings-card-link strong', ["Registry settings", "Update settings", "Coding tools", "Persistent Agents", "Run defaults", "Provider settings", "Connections", "Home"]] },
  { name: "settings-profile", url: "/pages.html?settings=profile", note: "Profile: the handle, the verified email, signed up with Google, the display name; Standard's credits, allowance, private QuickAgents, knowledge, hosted-run caps, Upgrade", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["chris · chris@example.test (verified)", "Signed up with Google", "Save profile", "40 of 50 credits · 40 submissions left this cycle", "4 of 10", "20 MB of 100 MB", "A hosted run holds 20 credits for its cap", "Upgrade"] },
  { name: "settings-profile-save", url: "/pages.html?settings=profile", note: "the display name changed and saved: the card shows the name the registry answered, and says so", locator: '[data-preview="settings"]',
    fill: [['[data-preview="settings"] input[aria-label="Display name"]', "Chris Q"]], click: '[data-preview="settings"] button:has-text("Save profile")',
    expectSection: "profile", expectText: "Display name saved.", expectNone: '[data-preview="settings"] .error-box' },
  { name: "settings-profile-promo", url: "/pages.html?settings=profile#profile-promo", note: "Promo Pro: the promo credits, the trial's code and end, Standard pending, the expiry notice", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Promo Pro", "88 of 100 promo credits", "Promo trial", "LAUNCH", "(unverified)", "Signed up with GitHub", "Standard from 29 October 2026", "You will not be charged."] },
  { name: "settings-profile-pro", url: "/pages.html?settings=profile#profile-pro", note: "Pro offered credit packs: the Top up credits card, each base struck and the credits larger, the bonus, the price, Buy opening the site's card", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Top up credits", "never expire", "This account holds 1,250 bought credits.", "+250 bonus", "+1,500 bonus", "US$9.99", "US$34.99", "bought credits never do", "this app never takes money"] },
  { name: "settings-profile-suspended", url: "/pages.html?settings=profile#profile-suspended", note: "Pro suspended for non-payment: what it owes and the link to the site's card, above the Top up credits card", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["This account is suspended for non-payment.", "owes 40 credits", "Top up credits on the site", "Top up credits", "-40 of 1000 credits · 0 submissions left this cycle"] },
  { name: "settings-profile-org", url: "/pages.html?settings=profile#profile-org", note: "a member of acme: the organization's tier, pool, seats, standing and knowledge pool; Manage billing opens its console", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Business", "acme covers your work", "The pool", "3960 of 5000 credits this cycle", "organization acme · admin", "in acme's pool", "Your own account", "Manage billing"] },
  { name: "settings-profile-core", url: "/pages.html?settings=profile#profile-core", note: "a registry that sells nothing: the profile, and no Subscription part, as on the site", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Save profile"], expectNone: '[data-preview="settings"] #settings-part-subscription' },
  { name: "settings-registry-account", url: "/pages.html?settings=registry", note: "Registry settings keeps who the credential is and links the Profile card; the tier and credits moved there", locator: '[data-preview="settings"]',
    expectSection: "registry", expectText: ["signed in as chris", "Profile and subscription"], expectNone: '[data-preview="settings"] #settings-part-registries :text("credits")' },
  { name: "settings-registry-signins", url: "/pages.html?settings=registry", note: "Registry settings names the app's two sign-ins on each registry, and a listed registry that is neither says why and offers no sign-in", locator: '[data-preview="settings"]',
    expectSection: "registry", expectText: ["Agentry Desktop keeps two sign-ins: one for SecureAgentry at secureagentry.net, and one for your self-hosted registry, agentry.example.com.", "SecureAgentry sign-in", "Self-hosted sign-in", "No sign-in", "This Agentry Desktop keeps one self-hosted registry, and it is https://agentry.example.com", "Your self-hosted registry is"] },
  { name: "settings-open-registry", url: "/pages.html", note: "the Registry settings card opened: the registries, Secrets and Add a registry under ← Settings", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Registry settings") .settings-card-link', expectSection: "registry", expectText: ["← Settings", "Registries", "Add a registry", "Secrets for runs In Agentry"] },
  { name: "settings-open-updates", url: "/pages.html", note: "Update settings: the self-update card and the QuickAgents switch", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Update settings") .settings-card-link', expectSection: "updates", expectText: ["Check for newer versions", "Update QuickAgents automatically"] },
  { name: "settings-open-coding-agents", url: "/pages.html", note: "Coding tools: the chooser and the export switch", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Coding tools") .settings-card-link', expectSection: "coding-agents", expectText: ["Also install into"] },
  { name: "settings-open-run-defaults", url: "/pages.html", note: "Run defaults: the container engine's state", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Run defaults") .settings-card-link', expectSection: "run-defaults", expectText: ["Container engine"] },
  { name: "settings-open-provider", url: "/pages.html", note: "Provider settings: the model provider form and the AI Review key", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Provider settings") .settings-card-link', expectSection: "provider", expectText: ["Model provider", "Save provider settings", "Your own TypeSafe Jev key"] },
  { name: "settings-part-link", url: "/pages.html", note: "a part's link on the landing opens its section at that part", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card-parts button:has-text("AI Review key")', expectSection: "provider", expectPart: "ai-review-key" },
  // A deep link to a part a section's own read renders, or one that read
  // pushes down, lands at the part once the page has filled in:
  // Home's Recent runs card (the sidebar's Unsafe Mode
  // badge is settings-deeplink-unsafe, below).
  { name: "settings-deeplink-run-history", url: "/pages.html", note: "Home's Recent runs card opens Run defaults at Recent runs", locator: '[data-preview="settings"]',
    click: '[data-deeplink="homeRunHistory"]', expectSection: "run-defaults", expectPart: "history" },
  { name: "settings-back-registry", url: "/pages.html?settings=registry", note: "← Settings on Registry settings returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-updates", url: "/pages.html?settings=updates", note: "← Settings on Update settings returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-coding-agents", url: "/pages.html?settings=coding-agents", note: "← Settings on Coding tools returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-run-defaults", url: "/pages.html?settings=run-defaults", note: "← Settings on Run defaults returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-back-provider", url: "/pages.html?settings=provider", note: "← Settings on Provider settings returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  // Settings → Home: the landing's card and its line, the page's
  // switches, the pins with Unpin, Reset Home behind its confirm, and the
  // ways in (the card, a part's link, Home's own link, the deep link).
  { name: "settings-landing-home", url: "/pages.html", note: "the Home card last on the landing: every default card shown, nothing pinned", locator: '[data-preview="settings"] .settings-card:has-text("Which cards Home shows")',
    expectSection: "landing", expectText: ["Home", "All 4 cards shown, nothing pinned.", "Cards", "Pinned", "Reset Home"] },
  { name: "settings-landing-home-pins", url: "/pages.html#home-pins-five", note: "the Home card's line with five QuickAgents pinned", locator: '[data-preview="settings"] .settings-card:has-text("Which cards Home shows")',
    expectText: "All 4 cards shown, 5 QuickAgents pinned." },
  { name: "settings-landing-home-hidden", url: "/pages.html#home-hidden", note: "the Home card's line with a card hidden", locator: '[data-preview="settings"] .settings-card:has-text("Which cards Home shows")',
    expectText: "3 of 4 cards shown, nothing pinned." },
  { name: "settings-open-home", url: "/pages.html", note: "the Home card opened: a switch per default card, the pins and Reset Home under ← Settings", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Which cards Home shows") .settings-card-link', expectSection: "home",
    expectText: ["← Settings", "Cards", "A hidden card keeps its place on Home", "Recent runs", "Installed QuickAgents", "Profile", "Run defaults", "Nothing is pinned.", "Reset Home…"],
    expectChecked: '[data-preview="settings"] [data-home-switch="installed"] input' },
  { name: "settings-part-link-home", url: "/pages.html", note: "the Home card's Reset Home link opens the page at Reset Home", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Which cards Home shows") .settings-card-parts button:has-text("Reset Home")', expectSection: "home", expectPart: "reset-home" },
  { name: "settings-deeplink-home", url: "/pages.html", note: "Home's own Home settings link opens Settings → Home", locator: '[data-preview="settings"]', click: '[data-deeplink="homeSettings"]', expectSection: "home" },
  { name: "settings-back-home", url: "/pages.html?settings=home", note: "← Settings on Home returns to the cards", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("← Settings")', expectSection: "landing" },
  { name: "settings-home-hide", url: "/pages.html?settings=home", note: "Installed QuickAgents turned off: Go answers with the card hidden in its place, and the switch reads what Go says", locator: '[data-preview="settings"] #settings-part-home-cards',
    click: '[data-preview="settings"] [data-home-switch="installed"] input', expectSection: "home", expectNone: '[data-preview="settings"] [data-home-switch="installed"] input:checked', expectChecked: '[data-preview="settings"] [data-home-switch="profile"] input' },
  { name: "settings-home-show", url: "/pages.html?settings=home#home-hidden", note: "a hidden card turned back on", locator: '[data-preview="settings"] #settings-part-home-cards',
    click: '[data-preview="settings"] [data-home-switch="installed"] input', expectSection: "home", expectChecked: '[data-preview="settings"] [data-home-switch="installed"] input' },
  { name: "settings-home-scheduled", url: "/pages.html?settings=home#home-scheduled", note: "the Scheduled card is listed after the default cards, on while Home draws it", locator: '[data-preview="settings"] #settings-part-home-cards',
    expectSection: "home", expectText: ["Scheduled", "This month's calendar of runs scheduled on this computer"], expectChecked: '[data-preview="settings"] [data-home-switch="scheduled"] input',
    expectOrder: [['[data-preview="settings"] [data-home-switch] strong', ["Recent runs", "Installed QuickAgents", "Profile", "Run defaults", "Scheduled"]]] },
  { name: "settings-home-scheduled-hide", url: "/pages.html?settings=home#home-scheduled", note: "the Scheduled card turned off: hidden in its place, like a default card", locator: '[data-preview="settings"] #settings-part-home-cards',
    click: '[data-preview="settings"] [data-home-switch="scheduled"] input', expectSection: "home", expectNone: '[data-preview="settings"] [data-home-switch="scheduled"] input:checked' },
  { name: "settings-home-scheduled-show", url: "/pages.html?settings=home", note: "the Scheduled card turned on from Settings: Go adds it at the end", locator: '[data-preview="settings"] #settings-part-home-cards',
    click: '[data-preview="settings"] [data-home-switch="scheduled"] input', expectSection: "home", expectChecked: '[data-preview="settings"] [data-home-switch="scheduled"] input' },
  { name: "settings-home-reset-confirm-scheduled", url: "/pages.html?settings=home#home-scheduled", note: "Reset Home with the Scheduled card: it says the card goes and the schedules stay", locator: ".modal",
    click: '[data-preview="settings"] [data-reset-home]', expectText: ["Reset Home?", "The Scheduled card goes too", "Your schedules stay as they are."] },
  { name: "settings-home-pins", url: "/pages.html?settings=home#home-pins-five", note: "five pins in two Pinned cards: where each came from, its version, one no longer installed, each with Unpin", locator: '[data-preview="settings"] #settings-part-pins',
    expectSection: "home", expectText: ["5 of 16 pinned, 4 to a Pinned card.", "Pinned card 1", "Pinned card 2", "demo1/incident-summarizer", "from secureagentry.net", "v1.2.0", "me/csv-quality-check", "from a folder", "acme/release-notes", "No longer installed on this computer."] },
  { name: "settings-home-pin-userinfo", url: "/pages.html?settings=home#home-pin-userinfo", note: "a pin whose registry address carries a user and password: only its host is named", locator: '[data-preview="settings"] #settings-part-pins',
    expectSection: "home", expectText: "from registry.example", expectNoText: ["s3cret", "chris:"] },
  { name: "settings-home-unpin", url: "/pages.html?settings=home#home-pins-five", note: "Unpin here is Home's Unpin: the pin goes, and the Pinned card it emptied with it", locator: '[data-preview="settings"] #settings-part-pins',
    click: '[data-preview="settings"] [data-pin="acme/release-notes"] button:has-text("Unpin")', expectSection: "home", expectText: ["4 of 16 pinned", "Unpinned acme/release-notes."], expectNone: '[data-preview="settings"] [data-pins-card="1"]' },
  { name: "settings-home-reset-confirm", url: "/pages.html?settings=home#home-pins-five", note: "Reset Home asks first, and says every pin goes and nothing is uninstalled", locator: ".modal",
    click: '[data-preview="settings"] [data-reset-home]', expectText: ["Reset Home?", "every pin is removed", "the 5 QuickAgents pinned there stay installed"] },
  { name: "settings-home-reset-confirm-one", url: "/pages.html?settings=home#home-pin-one", note: "Reset Home's confirm with one pin: the sentence in the singular", locator: ".modal",
    click: '[data-preview="settings"] [data-reset-home]', expectText: "the QuickAgent pinned there stays installed, and Pin to Home puts it back" },
  { name: "settings-home-reset", url: "/pages.html?settings=home#home-pins-five", note: "Reset Home confirmed: the default cards, every one on, and no pin", locator: '[data-preview="settings"]',
    click: ['[data-preview="settings"] [data-reset-home]', '[data-preview="settings"] [data-reset-home-confirm]'], expectSection: "home", expectText: ["Home is back to its default cards.", "Nothing is pinned."],
    expectChecked: '[data-preview="settings"] [data-home-switch="installed"] input', expectNone: '[data-preview="settings"] .modal' },
  { name: "settings-home-newer", url: "/pages.html?settings=home#home-newer", note: "a layout a newer Agentry Desktop arranged: the switches held, the pins not listed, Reset Home still offered", locator: '[data-preview="settings"]',
    expectSection: "home", expectText: ["Home was arranged by a newer Agentry Desktop", "Reset Home starts again from the defaults", "Reset Home…"], expectSelector: '[data-preview="settings"] [data-home-switch="installed"] input:disabled',
    expectChecked: '[data-preview="settings"] [data-home-switch="installed"] input' },
  { name: "page-home-settings-link", url: "/pages.html", note: "Home's Home settings link opens Settings → Home", locator: '[data-preview="home"]',
    click: '[data-preview="home"] [data-home-settings]', expectSelector: 'body[data-open="home-settings"]' },
  { name: "page-home-all-hidden", url: "/pages.html#home-all-hidden", note: "every card hidden and nothing pinned: a line that says so, with the way back", locator: '[data-preview="home"]',
    expectText: "Every card is hidden.", expectNone: '[data-preview="home"] [data-home-card]' },
  // Run defaults: the four defaults and Docker; a
  // budget saved, In Agentry chosen and a folder picked, as Go answers.
  { name: "settings-docker-images", url: "/pages.html?settings=run-defaults", note: "Run defaults → Container images: Agentry's images by name, the CLI's use of them, and Remove", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: ["Container images", "3 container images Agentry put on this computer", "ghcr.io/blaktron/agentry-mcp-gateway:v0.43.3 (86.3MB)", "The agentry CLI uses the same images", "Remove Agentry's container images"] },
  { name: "settings-docker-images-removed", url: "/pages.html?settings=run-defaults", note: "after Remove (the native Yes is the Go host's): the report, and none listed", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("Remove Agentry\'s container images")', expectSection: "run-defaults", expectText: ["Removed 3 images.", "No container images of Agentry's are on this computer."] },
  { name: "settings-docker-images-nodocker", url: "/pages.html?settings=run-defaults#images-nodocker", note: "the engine not answering: nothing listed, and why", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: "The engine's images could not be listed", expectNone: '[data-preview="settings"] button:has-text("Remove Agentry\'s container images")' },
  { name: "settings-run-defaults", url: "/pages.html?settings=run-defaults", note: "Run defaults: timeout and turns (the policy's, which the CLI shares), where to run, the output folder, the container engine", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: ["Timeout (seconds)", "the agentry CLI runs with them too", "Where to run", "every consent is asked on every run", "None chosen", "Container engine", "docker 29.7.2, rootful, at unix:///var/run/docker.sock, found as the docker CLI's current context", "Re-detect"],
    expectValue: ['[data-preview="settings"] #settings-part-budgets input[type="number"] >> nth=0', "900"] },
  // The container engine card, state by state (pages.html#docker-<state>;
  // every string says container engine, the engine found is named by kind
  // and version).
  { name: "settings-engine-podman", url: "/pages.html?settings=run-defaults#docker-podman", note: "Run defaults → Container engine: podman found natively on this computer, named with its version and socket, and Re-detect", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["Container engine", "podman 5.8.4, rootless, at unix:///run/user/1000/podman/podman.sock, found as podman on this computer", "Re-detect"] },
  { name: "settings-engine-redetect", url: "/pages.html?settings=run-defaults#docker-podman", note: "Re-detect pressed: the whole detection again, and what it found said", locator: '[data-preview="settings"] #settings-part-docker',
    click: '[data-preview="settings"] #settings-part-docker button:has-text("Re-detect")', expectSection: "run-defaults", expectText: ["Re-detect found podman 5.8.4, rootless"] },
  { name: "settings-engine-picker", url: "/pages.html?settings=run-defaults", note: "Run defaults → Container engine → Choose an engine…: Automatic and every engine that answers, the one in use marked, Use this on the others, and where the app looked", locator: '[data-preview="settings"] #settings-part-docker',
    click: '[data-testid="choose-engine"]', expectSection: "run-defaults", expectText: ["Automatic", "the first engine that answers", "your choice", "The docker CLI's context default", "in use", "The docker CLI's context colima", "Podman on this computer", "rootless, at unix:///run/user/1000/podman/podman.sock", "Use this", "Where the app looked"], expectSelector: '[data-engine-choice="podman"]' },
  { name: "settings-engine-picker-chosen", url: "/pages.html?settings=run-defaults", note: "Use this on podman: the choice remembered and adopted at once, said, and Use automatic offered", locator: '[data-preview="settings"] #settings-part-docker',
    click: ['[data-testid="choose-engine"]', '[data-engine-choice="podman"] button:has-text("Use this")'], expectSection: "run-defaults", expectText: ["Runs now drive podman 5.8.4", "your choice", "Use automatic"], expectSelector: '[data-testid="engine-use-automatic"]' },
  { name: "settings-engine-picker-locked", url: "/pages.html?settings=run-defaults#engines-locked", note: "an app started with its engine named in its environment: nothing to choose, and why", locator: '[data-preview="settings"] #settings-part-docker',
    click: '[data-testid="choose-engine"]', expectSection: "run-defaults", expectText: ["The app was started with an engine named in its environment"], expectNone: '[data-engine-choice="podman"]' },
  { name: "settings-engine-chosen-silent", url: "/pages.html?settings=run-defaults#docker-chosenSilent", note: "the chosen engine silent: said in its own words, nothing else offered in its place, and Use automatic detection", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["Your chosen engine, the docker CLI's context colima, is not answering", "does not switch to another engine on its own", "Use automatic detection", "Where the app looked"], expectSelector: '[data-testid="engine-automatic"]', expectNoText: ["Install podman"] },
  { name: "settings-engine-podman-installable", url: "/pages.html?settings=run-defaults#docker-installable", note: "no engine on Fedora: the distribution's own engine is podman — one package, no group, no restart; the command shown before consent", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["Container engine", "This distribution's own engine is podman", "dnf install -y podman", "no group to join and no restart", "Install podman", "Where the app looked"], expectNoText: ["usermod"] },
  { name: "settings-engine-docker-installable", url: "/pages.html?settings=run-defaults#docker-ubuntu", note: "no engine on Ubuntu: Docker Engine as before — the package, the service and the docker group", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["This distribution's own engine is Docker Engine", "apt-get install -y docker.io", "usermod -aG docker me", "Install Docker Engine"] },
  { name: "settings-engine-socket-off", url: "/pages.html?settings=run-defaults#docker-socketOff", note: "podman installed, its API socket off: one click turns it on as you, no elevation", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["podman is installed, but its API socket is off", "systemctl --user enable --now podman.socket", "Turn the socket on"] },
  { name: "settings-engine-socket-on", url: "/pages.html?settings=run-defaults#docker-socketOff", note: "the socket turned on: the card is gone, the engine answers", locator: '[data-preview="settings"] #settings-part-docker',
    click: '[data-preview="settings"] #settings-part-docker button:has-text("Turn the socket on")', expectSection: "run-defaults", expectNone: '[data-preview="settings"] #settings-part-docker button:has-text("Turn the socket on")' },
  { name: "settings-engine-group-missing", url: "/pages.html?settings=run-defaults#docker-groupMissing", note: "Docker Engine installed, the user not in the docker group: the group step alone", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["Docker Engine is installed, but your user is not in the", "Add me to the docker group"] },
  { name: "settings-engine-pending-restart", url: "/pages.html?settings=run-defaults#docker-pendingRestart", note: "Docker Engine installed, the group granted, the session not restarted: Restart now / later", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["Docker Engine is installed and your user is in the", "Restart now", "Restart later"] },
  { name: "settings-engine-manual-linux", url: "/pages.html?settings=run-defaults#docker-manual", note: "an engine installed that still refuses: its own words, and where to look for docker and podman", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["A container engine is installed, and it still refuses", "Cannot connect to the Docker daemon", "systemctl --user status podman.socket", "Re-detect"] },
  { name: "settings-engine-macos", url: "/pages.html?settings=run-defaults#docker-macos", note: "no engine on a Mac: Docker Desktop first, then Colima, Rancher Desktop and Podman Desktop, each a sentence and a link, and the licence sentence", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["No container engine answered on this computer", "Docker Desktop", "Colima", "Rancher Desktop", "Podman Desktop", "Docker Desktop is free for personal use and small companies and licensed per seat for others", "see Docker's terms"], expectNoText: ["WSL 2"] },
  { name: "settings-engine-managed-offer", url: "/pages.html?settings=run-defaults#docker-managedOffer", note: "a Mac with no engine: the one click first — Agentry's engine, the three binaries, their licences and size, where it goes, no Homebrew and no password — then the engines to install by hand and the licence sentence", locator: '[data-preview="settings"] .card:has-text("Agentry\'s engine")', expectText: ["Install Agentry's engine", "Colima 0.10.3, Lima 2.2.1 and the docker command 29.9.1", "Colima (MIT), Lima (Apache-2.0) and the docker command (Apache-2.0)", "No Homebrew, no administrator password", "Or install one of these yourself", "Docker Desktop is free for personal use"], expectSelector: '[data-testid="install-managed-engine"]' },
  { name: "settings-engine-managed-stopped", url: "/pages.html?settings=run-defaults#docker-managedStopped", note: "Agentry's engine installed and not running: Start and Remove, where it lives, and where the app looked", locator: '[data-preview="settings"] .card:has-text("Agentry\'s engine")', expectText: ["is installed under", "is not running", "Where the app looked"], expectSelector: ['[data-testid="start-managed-engine"]', '[data-testid="remove-managed-engine"]'], expectNoText: "newer versions" },
  { name: "settings-engine-managed-stale", url: "/pages.html?settings=run-defaults#docker-managedStale", note: "Agentry's engine installed by an older app: the line says this app updates it at its next start", locator: '[data-preview="settings"] .card:has-text("Agentry\'s engine")', expectText: "this app carries newer versions and updates it at its next start" },
  { name: "settings-engine-managed-starting", url: "/pages.html?settings=run-defaults#docker-managedStarting", note: "the app starting its engine: the spinner, no Start and no Remove", locator: '[data-preview="settings"] .card:has-text("Agentry\'s engine")', expectText: ["is starting", "Starting Agentry's engine…"], expectNone: '[data-testid="start-managed-engine"], [data-testid="remove-managed-engine"]' },
  { name: "settings-engine-managed-long-home", url: "/pages.html?settings=run-defaults#docker-managedLongHome", note: "a home whose socket path macOS cannot bind: the reason instead of the click, then the engines to install by hand", locator: '[data-preview="settings"] .card:has-text("cannot run an engine of its own")', expectText: ["cannot run an engine of its own on this computer", "over the 104 macOS allows", "Or install one of these yourself"], expectNone: '[data-testid="install-managed-engine"]' },
  { name: "settings-engine-managed-found", url: "/pages.html?settings=run-defaults#docker-managed", note: "Agentry's engine found: the engine line names it, Remove Agentry's engine beside Re-detect", locator: '[data-preview="settings"] .card:has([data-engine-line])', expectText: ["found as Agentry's engine (Colima)", "Remove Agentry's engine", "Re-detect"] },
  { name: "settings-engine-windows-offer", url: "/pages.html?settings=run-defaults#docker-windowsOffer", note: "Windows with no engine and no WSL: Rancher Desktop in one click, WSL first with administrator consent and a restart, then the engines to install by hand and the licence sentence", locator: '[data-preview="settings"] .card:has-text("Rancher Desktop")', expectText: ["Rancher Desktop 1.24.0 (SUSE, Apache-2.0), about 640 MB", "SHA-512", "for your account alone", "administrator consent once and then a restart", "Install WSL, then Rancher Desktop", "Or install one of these yourself", "Docker Desktop is free for personal use"], expectSelector: '[data-testid="install-windows-engine"]' },
  { name: "settings-engine-windows-offer-ready", url: "/pages.html?settings=run-defaults#docker-windowsOfferReady", note: "Windows with WSL on: the click installs Rancher Desktop alone", locator: '[data-preview="settings"] .card:has-text("Rancher Desktop")', expectText: ["Install Rancher Desktop"], expectNoText: "administrator consent once" },
  { name: "settings-engine-windows-restart", url: "/pages.html?settings=run-defaults#docker-windowsRestart", note: "WSL installed and Windows not restarted: Restart later and Restart now, and what to do after", locator: '[data-preview="settings"] .card:has-text("Windows Subsystem for Linux is installed")', expectText: ["Windows has to restart", "click Install Rancher Desktop again", "Restart later", "Restart now"] },
  { name: "settings-engine-windows-stopped", url: "/pages.html?settings=run-defaults#docker-windowsStopped", note: "Rancher Desktop installed and its engine silent: Start, and the containerd note", locator: '[data-preview="settings"] .card:has-text("Rancher Desktop")', expectText: ["is installed and its engine is not answering", "choose dockerd (moby)", "Where the app looked"], expectSelector: '[data-testid="start-windows-engine"]' },
  { name: "settings-engine-windows-starting", url: "/pages.html?settings=run-defaults#docker-windowsStarting", note: "the app starting Rancher Desktop: the spinner and no Start", locator: '[data-preview="settings"] .card:has-text("Rancher Desktop")', expectText: ["is starting", "Starting Rancher Desktop…"], expectNone: '[data-testid="start-windows-engine"]' },
  { name: "wizard-runtime-managed-offer", url: "/wizard.html#3-managedOffer", note: "the wizard's runtime step on a Mac with no engine: the same one click, before the provider form", expectText: ["Install Agentry's engine", "Set up the runtime"] },
  { name: "wizard-runtime-windows-offer", url: "/wizard.html#3-windowsOffer", note: "the wizard's runtime step on Windows with no engine: the Rancher Desktop click, before the provider form", expectText: ["Install WSL, then Rancher Desktop", "Set up the runtime"] },
  { name: "settings-engine-windows", url: "/pages.html?settings=run-defaults#docker-windows", note: "no engine on Windows: Docker Desktop and Rancher Desktop, the licence sentence, and the CLI inside WSL 2", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["Docker Desktop", "Rancher Desktop", "licensed per seat", "inside WSL 2"], expectNoText: ["Colima"] },
  { name: "settings-engine-traps", url: "/pages.html?settings=run-defaults#docker-trap", note: "Docker Desktop found through its socket, beside the two traps: a credential helper off PATH, and a socket file with nothing behind it", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["found as the socket of Docker Desktop", "docker-credential-desktop is not on PATH", "left a socket at /Users/me/.rd/docker.sock with nothing answering behind it"] },
  { name: "settings-engine-empty-socket", url: "/pages.html?settings=run-defaults#docker-emptysocket", note: "no engine, and a socket file with nothing behind it named beside the choices", locator: '[data-preview="settings"] #settings-part-docker',
    expectSection: "run-defaults", expectText: ["No container engine answered on this computer", "with nothing answering behind it"] },
  // The built-in agents' switch: on by default
  // beside the timeout and turns it lifts, then off.
  { name: "settings-builtin-no-limit-on", url: "/pages.html?settings=run-defaults", note: "Run defaults → Timeout and turns: the built-in agents' no-limit switch, on by default", locator: '[data-preview="settings"] #settings-part-budgets',
    expectSection: "run-defaults", expectText: ["No time or turn limit for the built-in agents", "On unless you turn it off", "drafts, knowledge builds, local", "until it finishes or you stop", "repair turn has no timeout too", "at most eight tool calls", "remaining turns and cost", "at most five minutes", "Your QuickAgents keep the limits above"],
    expectChecked: '[data-preview="settings"] input[data-builtin-no-limit]' },
  { name: "settings-run-history", url: "/pages.html?settings=run-defaults", note: "Run defaults → Recent runs: Remember recent runs on, how many are kept, what is and is not kept, Clear run history", locator: '[data-preview="settings"] #settings-part-history',
    expectSection: "run-defaults", expectText: ["Remember recent runs", "On. 7 runs are remembered.", "never the task, your inputs, the result or", "Clear run history"], expectChecked: '[data-preview="settings"] input[data-remember-runs]' },
  { name: "settings-run-history-cleared", url: "/pages.html?settings=run-defaults", note: "Clear run history: none kept, the switch still on", locator: '[data-preview="settings"] #settings-part-history',
    click: '[data-preview="settings"] button[data-clear-run-history]', expectSection: "run-defaults", expectText: "On. No run is remembered yet.", expectNone: '[data-preview="settings"] button[data-clear-run-history]' },
  { name: "settings-run-history-off", url: "/pages.html?settings=run-defaults", note: "Remember recent runs turned off: nothing is remembered", locator: '[data-preview="settings"] #settings-part-history',
    click: '[data-preview="settings"] input[data-remember-runs]', expectSection: "run-defaults", expectText: "Off: no run is remembered." },
  { name: "settings-builtin-no-limit-off", url: "/pages.html?settings=run-defaults", note: "the switch turned off: drafts, builds, reviews and voices on this computer keep the max turns and their floors", locator: '[data-preview="settings"] #settings-part-budgets',
    click: '[data-preview="settings"] input[data-builtin-no-limit]', expectSection: "run-defaults", expectText: "No time or turn limit for the built-in agents", expectNone: '[data-preview="settings"] input[data-builtin-no-limit]:checked' },
  { name: "settings-malware-check-off", url: "/pages.html?settings=run-defaults", note: "Run defaults → Malware check: off by default; a run checks only the programs of a QuickAgent that runs commands", locator: '[data-preview="settings"] #settings-part-malware',
    expectSection: "run-defaults", expectText: ["Check every file a run is given for malware", "Off. A run checks only the programs among its files, and only when the QuickAgent runs commands that could start them.", "Runs In Agentry are always checked"], expectNone: '[data-preview="settings"] input[data-malware-check]:checked' },
  { name: "settings-malware-check-on", url: "/pages.html?settings=run-defaults", note: "the switch turned on: every file is checked, as In Agentry", locator: '[data-preview="settings"] #settings-part-malware',
    click: '[data-preview="settings"] input[data-malware-check]', expectSection: "run-defaults", expectText: "On. Every file you give a run", expectChecked: '[data-preview="settings"] input[data-malware-check]' },
  // Unsafe Mode: the switch off and on in Run defaults,
  // the pull under way, and the red edge over a page and over a dialog.
  // `window` shoots the window, not the locator, since the edge is the window's;
  // on pages.html, which stacks every page, it keeps the one the locator names.
  { name: "settings-unsafe-off", url: "/pages.html?settings=run-defaults", note: "Run defaults → Unsafe Mode, off: what it lifts, what stays, and what turning it on does", locator: '[data-preview="settings"] #settings-part-unsafe',
    expectSection: "run-defaults", expectText: ["Unsafe Mode", "Off", "commands this computer has not approved", "or from PyPI", "run_command still takes only the commands a QuickAgent declares", "never uses Unsafe Mode", "ghcr.io/blaktron/agentry-unsafe-kali"],
    expectNone: ".unsafe-glow, .unsafe-badge", expectUnsafeSwitch: false },
  { name: "settings-unsafe-on", url: "/pages.html?settings=run-defaults#unsafe-on", note: "Unsafe Mode on: the switch reads on, the red edge runs round the window and the sidebar's badge says Unsafe Mode", locator: '[data-preview="settings"]', window: true,
    expectSection: "run-defaults", expectText: ["Unsafe Mode", "On"], expectUnsafeSwitch: true, expectGlow: true, scrollTo: "#settings-part-unsafe" },
  { name: "settings-unsafe-running", url: "/pages.html?settings=run-defaults#unsafe-running", note: "Unsafe Mode turned off while a run compiled in it is still live: the switch reads off, the red edge stays and the badge says it is running", locator: '[data-preview="settings"]', window: true,
    expectSection: "run-defaults", expectText: ["Unsafe Mode", "Off", "applies from the next run or chat", "One already running keeps Unsafe Mode"], expectUnsafeSwitch: false, expectGlow: true, expectBadge: "Unsafe Mode: running", scrollTo: "#settings-part-unsafe" },
  { name: "settings-unsafe-pulling", url: "/pages.html?settings=run-defaults#unsafe-pulling", note: "the switch clicked and the native Yes given: the image's pull under way, the switch still off until it ends", locator: '[data-preview="settings"] #settings-part-unsafe',
    click: '[data-preview="settings"] #settings-part-unsafe input[role="switch"]', expectSection: "run-defaults", expectText: ["Downloading the Unsafe Mode image", "Pulling from blaktron/agentry-unsafe-kali"], expectUnsafeSwitch: false },
  { name: "settings-landing-unsafe", url: "/pages.html#unsafe-on", note: "the Settings landing in Unsafe Mode: the Run defaults card's status line says it is on, under the red edge", locator: '[data-preview="settings"]', window: true,
    expectText: "Unsafe Mode is on.", expectGlow: true },
  { name: "unsafe-glow-browse", url: "/pages.html#unsafe-on", note: "a page in Unsafe Mode: the red edge round the window, the badge at the foot of the sidebar", locator: '[data-preview="browse"]', window: true, expectGlow: true },
  { name: "runpanel-unsafe-review", url: "/modals.html#runpanel-unsafe-review", note: "Review and start in Unsafe Mode: the line names the declared commands and the one the image cannot provide; the red edge frames the dialog too", window: true,
    expectText: ["Unsafe Mode is on", "As resolved, it declares: 7z, analyzeHeadless", "Not available in Unsafe Mode's image: pyinstxtractor"], expectGlow: true, scrollTo: "[data-unsafe-notice]" },
  { name: "runpanel-unsafe-finished", url: "/modals.html#runpanel-unsafe-finished", note: "an Unsafe Mode run finished: the result names what was installed from where and what did not install", window: true,
    click: ['button:text-is("Run")'], expectText: ["This run's commands", "At least one did not install, and the run went on without it.", "binwalk (E: Unable to locate package binwalk) for binwalk"], expectGlow: true, scrollTo: "#run-result-unsafe" },
  { name: "settings-run-defaults-saved", url: "/pages.html?settings=run-defaults", note: "a timeout of 1800 s saved, In Agentry chosen, a folder picked", locator: '[data-preview="settings"]',
    fill: [['[data-preview="settings"] #settings-part-budgets input[type="number"] >> nth=0', "1800"]],
    click: ['[data-preview="settings"] input[name="run-where"] >> nth=1', '[data-preview="settings"] #settings-part-output button:has-text("Choose")', '[data-preview="settings"] #settings-part-budgets button:has-text("Save")'],
    expectSection: "run-defaults", expectText: ["Saved.", "/home/me/agentry-runs"], expectChecked: '[data-preview="settings"] input[name="run-where"] >> nth=1' },
  // A chat on this computer has no limit but Run defaults' Chat idle timeout,
  // none by default: the control unset, then set.
  { name: "settings-chat-idle-none", url: "/pages.html?settings=run-defaults", note: "Run defaults → Chat idle timeout, none: a chat here ends when the person ends it; the CLI's chats and a chat In Agentry keep their own limits", locator: '[data-preview="settings"] #settings-part-chats',
    expectSection: "run-defaults", expectText: ["Chat idle timeout", "None. A chat on this computer ends when you end it or quit Agentry Desktop.", "The max turns above still limits", "keep the runner policy's limits"],
    expectValue: ['[data-preview="settings"] input[data-chat-idle]', ""], expectNone: '[data-preview="settings"] #settings-part-chats button:has-text("No timeout")' },
  { name: "settings-chat-idle-set", url: "/pages.html?settings=run-defaults", note: "a Chat idle timeout of 30 minutes saved: the next chat here ends after 30 minutes without a message, and No timeout takes it off", locator: '[data-preview="settings"] #settings-part-chats',
    fill: [['[data-preview="settings"] input[data-chat-idle]', "30"]], click: '[data-preview="settings"] #settings-part-chats button:has-text("Save")',
    expectSection: "run-defaults", expectText: ["Saved. The next chat uses it.", "A chat on this computer ends after 30 minutes without a message from you.", "No timeout"], expectValue: ['[data-preview="settings"] input[data-chat-idle]', "30"] },
  { name: "settings-chat-idle-cleared", url: "/pages.html?settings=run-defaults", note: "30 minutes saved, then No timeout: the field empties with the line, and No timeout goes", locator: '[data-preview="settings"] #settings-part-chats',
    fill: [['[data-preview="settings"] input[data-chat-idle]', "30"]],
    click: ['[data-preview="settings"] #settings-part-chats button:has-text("Save")', '[data-preview="settings"] #settings-part-chats button:has-text("No timeout")'],
    expectSection: "run-defaults", expectText: ["Saved. The next chat has no idle timeout.", "None. A chat on this computer ends when you end it"],
    expectValue: ['[data-preview="settings"] input[data-chat-idle]', ""], expectNone: '[data-preview="settings"] #settings-part-chats button:has-text("No timeout")' },
  { name: "runpanel-chat-review", url: "/modals.html#runpanel-chat-review", note: "Review and start for a chat on this computer, Advanced open: no timeout (s), the line that says a chat here has no time limit, max turns kept",
    click: "details.advanced > summary", expectText: ["Start session", "A chat here has no time limit.", "max turns (0 = unlimited)"], expectNone: 'details.advanced span:text-is("timeout (s)")', scrollTo: "[data-chat-limits]" },
  { name: "settings-deeplink-selfupdate", url: "/pages.html", note: "the sidebar's newer-version line opens Update settings", locator: '[data-preview="settings"]', click: '[data-deeplink="selfUpdate"]', expectSection: "updates", expectPart: "desktop" },
  { name: "settings-deeplink-setup", url: "/pages.html", note: "Home's setup notice opens Provider settings", locator: '[data-preview="settings"]', click: '[data-deeplink="setupNotice"]', expectSection: "provider" },
  { name: "settings-deeplink-knowledge", url: "/pages.html", note: "Knowledge's Not signed in opens Registry settings at the registries", locator: '[data-preview="settings"]', click: '[data-deeplink="knowledgeSignIn"]', expectSection: "registry", expectPart: "registries" },
  { name: "settings-deeplink-describe", url: "/pages.html", note: "Build's Describe it without an AI Review key opens Provider settings at the key", locator: '[data-preview="settings"]', click: '[data-deeplink="describeKey"]', expectSection: "provider", expectPart: "ai-review-key" },
  { name: "settings-deeplink-unsafe", url: "/pages.html#unsafe-on", note: "the sidebar's Unsafe Mode badge opens Run defaults at the switch", locator: '[data-preview="settings"]', click: '[data-deeplink="unsafeMode"]', expectSection: "run-defaults", expectPart: "unsafe", expectUnsafeSwitch: true },
  { name: "settings-status-failed", url: "/pages.html#status-refused", note: "the landing, signed in, when every status read fails: the eight cards stay (Profile, Connections and Home among them), their lines drop but the registry's", locator: '[data-preview="settings"]',
    expectSection: "landing", expectCards: 8, expectStatusLines: 1 },
  { name: "toolspanel", url: "/modals.html#toolspanel", note: "the tools panel over the shell" },
  { name: "toolspanel-hostile", url: "/modals.html#toolspanel-hostile", note: "the hostile guide's key help in the tools panel: markdown, framed, each link's host beside it, nothing that acts", linksIn: ".key-help", expectText: ["The author's help for", "U+202E", "(evil.example)", "(github.com.evil.example)"], expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new", "https://evil.example/steal", "https://github.co/", "https://github.com.evil.example/"], expectNone: ".key-help img, .key-help script, .key-help b[onclick], .key-help a[href^='javascript'], .key-help a[href^='file']" },
  { name: "chat", url: "/modals.html#chat", note: "a session's chat pane", locator: ".chat-pane" },
  { name: "wizard-welcome", url: "/wizard.html#0", note: "the first-run wizard: welcome", locator: ".wizard" },
  { name: "wizard-signin", url: "/wizard.html#1", note: "the first-run wizard: sign-in, naming the two sign-ins and which one the address is", locator: ".wizard", expectText: ["Agentry Desktop keeps two sign-ins", "This is your SecureAgentry sign-in, sent only to secureagentry.net."] },
  { name: "wizard-signin-other", url: "/wizard.html#1", note: "another SecureAgentry address typed: the app signs in to its own SecureAgentry registry only", locator: ".wizard", fill: [['.wizard input.mono', "https://dev.secureagentry.net"]], expectText: "Agentry Desktop signs in to SecureAgentry at secureagentry.net only.", expectSelector: '.wizard-body button:has-text("Sign in via browser"):disabled' },
  // Local or Cloud after Sign in: Cloud locked with
  // "Please sign in" across it until a registry is signed in; Local goes on
  // to the runtime; Cloud to its data page, whose Finish records it.
  { name: "wizard-where", url: "/wizard.html#1", note: "Local or Cloud, reached by Skip on Sign in: Cloud greyed, Please sign in across it in the accent's light", locator: ".wizard", click: '.wizard-foot button:has-text("Skip, browse anonymously")', expectText: ["Where will your QuickAgents run?", "Local", "Cloud", "Please sign in"], expectSelector: ".wizard-choice.is-locked button:disabled" },
  { name: "wizard-where-signedin", url: "/wizard.html#2-signedin", note: "Local or Cloud signed in: both cards open", locator: ".wizard", expectText: ["Local", "Cloud"], expectNoText: "Please sign in", expectNone: ".wizard-choice.is-locked" },
  { name: "wizard-where-local", url: "/wizard.html#2", note: "Local picked: on to the runtime, Run defaults set to this computer", locator: ".wizard", click: '.wizard-choice button:has-text("Local")', expectText: "Set up the runtime", expectRunWhere: "here" },
  { name: "wizard-cloud", url: "/wizard.html#2-signedin", note: "Cloud picked signed in: how data in Agentry's cloud is kept, Back and Finish", locator: ".wizard", click: '.wizard-choice button:has-text("Cloud")', expectText: ["Your data in Agentry's cloud", "stored encrypted", "never leaves our vault", "a vault call, recorded in its audit log", "Your hosted runs", "Back", "Finish"] },
  { name: "wizard-cloud-back", url: "/wizard.html#cloud", note: "Back from the Cloud page: the choice again, with Local's seven dots, nothing written", locator: ".wizard", click: '.wizard-foot button:has-text("Back")', expectText: "Where will your QuickAgents run?", expectDots: 8, expectRunWhere: "" },
  { name: "wizard-cloud-finish", url: "/wizard.html#cloud", note: "Finish on the Cloud page records Cloud in Run defaults", locator: ".wizard", click: '.wizard-foot button:has-text("Finish")', expectRunWhere: "agentry" },
  // Reached by Continue from Coding agent, and the key's own Continue before
  // it: the two renumbered steps forward.
  { name: "wizard-done", url: "/wizard.html#5-saved", note: "the first-run wizard: done, reached by Continue from the AI Review key step and from Coding agent, with the key's row", locator: ".wizard", click: ['.wizard-foot button:has-text("Continue")', '.wizard-foot button:has-text("Continue")'], expectText: ["You're set up", "Provider anthropic (claude-sonnet-4-5) written to the runner policy", "AI Review key saved"] },
  { name: "wizard-done-key-only", url: "/wizard.html#7-keyonly", note: "done after the AI Review key alone was saved: the policy its save wrote is not a provider set up, so the ending is still Nothing set up", locator: ".wizard", expectText: ["Nothing is set up yet", "The runner policy has only the default provider, anthropic (claude-sonnet-4-5), with its key read from $ANTHROPIC_API_KEY", "AI Review key saved"] },
  { name: "wizard-done-nothing", url: "/wizard.html#7-nothing", note: "the first-run wizard: every step skipped, the Nothing set up ending, the key's row pending", locator: ".wizard", expectText: ["Nothing is set up yet", "No AI Review key — drafting on this computer is off until you add one in Settings"] },
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
      // A window shot is taken at a window's size: the Unsafe Mode edge is
      // the window's, and the default viewport is as tall as a long panel.
      if (state.window) await page.setViewportSize({ width: 1180, height: 760 });
      // width: a narrower window than the default, for a layout check at
      // the narrowest width a page is meant to hold.
      if (state.width) await page.setViewportSize({ width: state.width, height: 900 });
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
      for (const [sel, val] of state.select ?? []) {
        await page.locator(sel).first().selectOption(val);
      }
      // A state behind a click — a tab, a wizard's next step — or a few in
      // order (click as a list: Run, then Run details) is reached the way a
      // person reaches it.
      for (const sel of [state.click ?? []].flat()) {
        await page.locator(sel).first().click();
        await page.waitForTimeout(400);
      }
      // contextClick: a right-click, as a person opens a context menu;
      // focusPress: [selector, key], the element focused and the key
      // pressed, as a keyboard user reaches the same menu.
      if (state.contextClick) {
        await page.locator(state.contextClick).first().click({ button: "right" });
        await page.waitForTimeout(300);
      }
      if (state.focusPress) {
        await page.locator(state.focusPress[0]).first().focus();
        await page.keyboard.press(state.focusPress[1]);
        await page.waitForTimeout(300);
      }
      // drag: [from, to], a press on the first element's middle, moved in
      // steps onto the second's, and released, as a person drags with a
      // mouse; dragBy: [selector, dx, dy], the same by an offset.
      const dragPath = async (x0, y0, x1, y1) => {
        await page.mouse.move(x0, y0);
        await page.mouse.down();
        for (let i = 1; i <= 12; i++) await page.mouse.move(x0 + ((x1 - x0) * i) / 12, y0 + ((y1 - y0) * i) / 12);
        await page.waitForTimeout(150);
        await page.mouse.up();
        await page.waitForTimeout(400);
      };
      const middle = async (sel) => {
        const loc = page.locator(sel).first();
        await loc.scrollIntoViewIfNeeded();
        const b = await loc.boundingBox();
        if (!b) throw new Error(`${state.name}: nothing to drag at ${sel}`);
        return [b.x + b.width / 2, b.y + b.height / 2];
      };
      if (state.drag) {
        const [x0, y0] = await middle(state.drag[0]);
        const [x1, y1] = await middle(state.drag[1]);
        await dragPath(x0, y0, x1, y1);
      }
      if (state.dragBy) {
        await page.locator(state.dragBy[0]).first().hover({ force: true });
        const [x0, y0] = await middle(state.dragBy[0]);
        await dragPath(x0, y0, x0 + state.dragBy[1], y0 + state.dragBy[2]);
      }
      // thenClick: a click made after those, on what they opened.
      for (const sel of [state.thenClick ?? []].flat()) {
        await page.locator(sel).first().click();
        await page.waitForTimeout(300);
      }
      // type: text typed into an element once the clicks are done, as a
      // person types it, then wait: a pause for the page's own timers (the
      // Cloud Voice page's 5-second refresh).
      for (const [sel, text] of state.type ?? []) {
        await page.locator(sel).first().click();
        await page.keyboard.press("End");
        await page.keyboard.type(text);
      }
      if (state.wait) await page.waitForTimeout(state.wait);
      // expectSelector: an element that must show, waited for, since it may
      // arrive with a chunk loaded by import() (the voice editor).
      for (const sel of [state.expectSelector ?? []].flat()) {
        await page.locator(sel).first().waitFor({ timeout: 5000 }).catch(() => {
          throw new Error(`${state.name}: nothing matches ${sel}`);
        });
      }
      // expectText may name several sentences, each of which must show.
      for (const want of [state.expectText ?? []].flat()) {
        const text = await page.locator(state.locator ?? "body").innerText();
        if (!text.includes(want)) throw new Error(`${state.name}: the page does not show "${want}"`);
      }
      // expectNoText: a sentence that must not show anywhere, such as a line
      // of another run's log.
      for (const unwanted of [state.expectNoText ?? []].flat()) {
        const text = await page.locator(state.locator ?? "body").innerText();
        if (text.includes(unwanted)) throw new Error(`${state.name}: the page shows "${unwanted}"`);
      }
      if (state.expectNone) {
        // Agent output makes no markup of its own.
        const n = await page.locator(state.expectNone).count();
        if (n !== 0) throw new Error(`${state.name}: ${n} element(s) matching ${state.expectNone}`);
      }
      if (state.expectDisabledButton) {
        // A blocker greys the button; the script says so, not only the shot.
        const b = page.locator(".modal").getByRole("button", { name: state.expectDisabledButton }).first();
        if (await b.isEnabled()) throw new Error(`${state.name}: ${state.expectDisabledButton} is enabled with a blocker showing`);
      }
      if (state.expectPlaceholder) {
        // The task field's placeholder is the author's.
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
      if (state.expectDisabledSubmitOnly) {
        const box = page.locator('[data-preview="build"] .build-summary');
        if (await box.getByRole("button", { name: "Submit", exact: true }).isEnabled()) throw new Error(`${state.name}: Submit is enabled with a public rule failing`);
        if (!(await box.getByRole("button", { name: "Install", exact: true }).isEnabled())) throw new Error(`${state.name}: Install waits for a rule only a submission needs`);
      }
      if (state.expectDisabledSummarySubmit) {
        for (const name of ["Submit", "Install"]) {
          const b = page.locator('[data-preview="build"] .build-summary').getByRole("button", { name, exact: true });
          if (await b.isEnabled()) throw new Error(`${state.name}: ${name} is enabled with a rule still failing`);
        }
      }
      if (state.expectCollapsed) {
        if (await page.locator(state.expectCollapsed).first().getAttribute("open") !== null) throw new Error(`${state.name}: ${state.expectCollapsed} is open with nothing chosen`);
      }
      if (state.expectChecked) {
        if (!(await page.locator(state.expectChecked).first().isChecked())) throw new Error(`${state.name}: ${state.expectChecked} is not checked`);
      }
      if (state.expectOrder) {
        // The order of a set of elements, read from the page, not the
        // picture (the Build start page's ways in). One
        // [selector, texts] pair, or a list of them.
        const pairs = Array.isArray(state.expectOrder[0]) ? state.expectOrder : [state.expectOrder];
        for (const [sel, want] of pairs) {
          const got = (await page.locator(sel).allInnerTexts()).map((t) => t.trim());
          if (JSON.stringify(got) !== JSON.stringify(want)) throw new Error(`${state.name}: ${sel} reads ${got.join(" | ")}, not ${want.join(" | ")}`);
        }
      }
      if (state.expectCount) {
        const [sel, want] = state.expectCount;
        const n = await page.locator(sel).count();
        if (n !== want) throw new Error(`${state.name}: ${n} element(s) match ${sel}, not ${want}`);
      }
      if (state.expectNoHScroll) {
        // The page and the card draw no horizontal scroll: nothing is wider
        // than the box that holds it.
        const wide = await page.evaluate((sel) => {
          const els = [document.documentElement, ...document.querySelectorAll(sel)];
          return els.filter((el) => el.scrollWidth > el.clientWidth + 1).map((el) => `${el.tagName}.${el.className}`);
        }, state.expectNoHScroll);
        if (wide.length) throw new Error(`${state.name}: wider than its box: ${wide.join(", ")}`);
      }
      if (state.expectFocused) {
        const ok = await page.evaluate((sel) => document.activeElement?.matches(sel) ?? false, state.expectFocused);
        if (!ok) throw new Error(`${state.name}: the focus is not on ${state.expectFocused}`);
      }
      if (state.expectMinWidth) {
        // A layout check, not only a picture: every matching element keeps
        // at least this width (a recap value squeezed
        // to one letter a line).
        const [sel, min] = state.expectMinWidth;
        const widths = await page.locator(sel).evaluateAll((els) => els.map((el) => el.getBoundingClientRect().width));
        if (widths.length === 0) throw new Error(`${state.name}: nothing matches ${sel}`);
        const narrow = widths.filter((w) => w < min);
        if (narrow.length) throw new Error(`${state.name}: ${narrow.length} of ${widths.length} ${sel} narrower than ${min}px (${narrow.map((w) => Math.round(w)).join(", ")})`);
      }
      if (state.expectValue) {
        const [sel, want] = state.expectValue;
        const got = await page.locator(sel).first().inputValue();
        if (got !== want) throw new Error(`${state.name}: ${sel} holds "${got}", not "${want}"`);
      }
      if (state.expectOpened) {
        // What the app asked the system browser to open, exactly: the
        // harness's App.OpenExternal records each address.
        const opened = await page.evaluate(() => window.__opened ?? []);
        if (JSON.stringify(opened) !== JSON.stringify(state.expectOpened)) throw new Error(`${state.name}: opened ${JSON.stringify(opened)}, not ${JSON.stringify(state.expectOpened)}`);
      }
      if (state.expectDots) {
        const n = await page.locator(".wizard-dot").count();
        if (n !== state.expectDots) throw new Error(`${state.name}: ${n} step dots, not ${state.expectDots}`);
      }
      if (state.expectRunWhere !== undefined) {
        // What the wizard wrote to Run defaults → Where to run: the harness's
        // SettingsAPI.SetRunWhere records it.
        const where = await page.evaluate(() => window.__runWhere ?? "");
        if (where !== state.expectRunWhere) throw new Error(`${state.name}: Where to run is "${where}", not "${state.expectRunWhere}"`);
      }
      if (state.expectHomeAgent !== undefined) {
        // The agent Home's card was given: the harness's HomeAPI.SetAgent
        // records it on the body.
        const agent = await page.evaluate(() => document.body.dataset.homeAgent ?? "");
        if (agent !== state.expectHomeAgent) throw new Error(`${state.name}: Home's agent is "${agent}", not "${state.expectHomeAgent}"`);
      }
      if (state.expectFootDisabled) {
        // The wizard's footer is held while the step's work is in flight.
        const foot = await page.locator(".wizard-foot button").evaluateAll((bs) => bs.map((b) => `${b.textContent.trim()}:${b.disabled ? "held" : "live"}`));
        if (foot.length === 0 || !foot.every((f) => f.endsWith(":held"))) throw new Error(`${state.name}: the footer is not held (${foot.join(" ")})`);
      }
      if (state.expectSection) {
        // The route, not only the shot: which Settings page is showing.
        const at = await page.locator('[data-preview="settings"] [data-settings-section]').first().getAttribute("data-settings-section");
        if (at !== state.expectSection) throw new Error(`${state.name}: Settings shows "${at}", not "${state.expectSection}"`);
      }
      if (state.expectPart) {
        // A part the link named is scrolled to: its top at the top of the
        // view (less its scroll margin), or as near as a page that cannot
        // scroll further allows. Merely "in view"
        // passed a part near the top that the scroll never reached. Read
        // after the page settles, since a section's own reads render its
        // parts after the route changes.
        // The tolerances are for rounding at a 1.5 device scale, not layout.
        await page.waitForTimeout(400);
        const where = await page.locator(`#settings-part-${state.expectPart}`).evaluate((el) => {
          const r = el.getBoundingClientRect();
          const margin = parseFloat(getComputedStyle(el).scrollMarginTop) || 0;
          const s = document.scrollingElement;
          const atBottom = s.scrollTop + window.innerHeight >= s.scrollHeight - 2;
          return { off: Math.round(r.top - margin), atBottom, inView: r.top < window.innerHeight && r.bottom > 0 };
        });
        if (!where.inView || (Math.abs(where.off) > 4 && !where.atBottom)) throw new Error(`${state.name}: the part ${state.expectPart} is not scrolled to (${where.off}px from where it should be)`);
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
      if (state.expectUnsafeSwitch !== undefined) {
        // The switch reads what Go says, not what was clicked.
        const on = await page.locator('#settings-part-unsafe input[role="switch"]').isChecked();
        if (on !== state.expectUnsafeSwitch) throw new Error(`${state.name}: the Unsafe Mode switch reads ${on ? "on" : "off"}`);
      }
      const onePage = state.window && /\[data-preview="(\w+)"\]/.exec(state.locator ?? "");
      if (onePage) {
        // pages.html stacks every page in one document; a window shot shows
        // the one page its locator names, as the app's window would.
        await page.evaluate((keep) => {
          for (const el of document.querySelectorAll("[data-preview]")) if (el.getAttribute("data-preview") !== keep) el.remove();
        }, onePage[1]);
        await page.waitForTimeout(100);
      }
      if (state.expectGlow) {
        // The edge covers the window, above every dialog, and takes no
        // click; the badge says the same in words.
        const glow = await page.locator(".unsafe-glow").first().evaluate((el) => {
          const r = el.getBoundingClientRect();
          const cs = getComputedStyle(el);
          const above = [...document.querySelectorAll(".modal-backdrop, .wizard-backdrop, .corner-cards")].every((o) => Number(getComputedStyle(o).zIndex || 0) < Number(cs.zIndex));
          return { covers: r.left === 0 && r.top === 0 && r.width === window.innerWidth && r.height === window.innerHeight, clicks: cs.pointerEvents, shadow: cs.boxShadow, above };
        });
        if (!glow.covers || glow.clicks !== "none" || !glow.shadow.includes("inset") || !glow.above) throw new Error(`${state.name}: the Unsafe Mode edge is ${JSON.stringify(glow)}`);
        const badge = (await page.locator(".sidebar .unsafe-badge").first().innerText()).trim();
        if (badge !== (state.expectBadge ?? "Unsafe Mode")) throw new Error(`${state.name}: the sidebar's badge reads "${badge}"`);
      }
      if (state.scrollTo) {
        await page.locator(state.scrollTo).first().scrollIntoViewIfNeeded();
        await page.waitForTimeout(100);
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
        await page.getByRole("radio", { name: "Custom endpoint (OpenAI-compatible)" }).click();
        await page.locator('input[placeholder="https://llm.example.net/v1"]').fill("llm.example.net/v1");
        await page.locator('input[type="password"]').first().fill("sk-preview-key");
        await page.locator('input[placeholder^="pick one with Get models"]').fill("qwen3.8-flash");
        await page.getByRole("button", { name: "Save provider settings" }).click();
        await page.waitForTimeout(300);
        // The refusal shown, and the key kept: the field says it is stored
        // and is empty, so nothing asks for it again.
        const text = await page.locator(".wizard").innerText();
        if (!text.includes("the base URL must be an absolute http(s) URL")) throw new Error(`${state.name}: the refusal is not shown`);
        const key = page.locator('input[type="password"]').first();
        const ph = await key.getAttribute("placeholder");
        const typed = await key.inputValue();
        if (!ph?.startsWith("stored on this machine") || typed !== "") throw new Error(`${state.name}: the key field reads "${ph}" / "${typed}"`);
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
      await (state.window ? page : (await modal.count()) ? modal : page).screenshot({ path: file });
      console.log(`${file}  — ${state.note}`);
      await page.close();
    }
    await ctx.close();
  }
  await browser.close();
} finally {
  vite.kill("SIGTERM");
}
