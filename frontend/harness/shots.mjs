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

// The AI Review key's recommendation, word for word as the operator decided
// it (plan desktop-review-key-and-run-cards.md D1): spelled out here, not
// imported, so a change to the app's sentence fails the states that show it.
// The Build start page's words (agentry-desktop#598), the operator's, on
// both clients: BuildWays.tsx and agentry-web components/builder.tsx.
const BUILD_TAGLINE = "Use our AI Powered QuickAgent Drafter or Build one to your exact requirements.";
const DESCRIBE_COPY = "Build a QuickAgent using the AI Drafter - just tell it what you want an agent to do and it will figure out what tools and servers it needs to do that job and build it out in the correct format.";
const BY_HAND_COPY = "Build a QuickAgent manually so that it does exactly what you want with exactly the tools you want. This is useful if you already have a workflow to bring over, or if you know precisely how you want the agent to behave.";
const KEY_RECOMMENDATION = "Agentry's AI Review is powered by TypeSafe AI's Jev System One. We highly recommend signing up with TypeSafe and getting an API key if you want to build shareable QuickAgents or submit them to Agentry.";

// One entry per state the panel can be in, as the issue lists them.
const states = [
  { name: "runpanel-modes", url: "/modals.html#runpanel", note: "Before you start: what it does, what to have ready, where to run it (In Agentry beside the local modes)", expectText: "What to have ready" },
  { name: "runpanel-in-agentry", url: "/modals.html#runpanel-hosted", note: "In Agentry chosen: the hold, the balance, the caps", expectNoText: "Its publisher earns" },
  { name: "runpanel-in-agentry-earns", url: "/modals.html#runpanel-hosted-earns", note: "In Agentry chosen for a release whose publisher is in Publisher Earnings: the line under the cost (agentry-desktop#613)", expectText: "Its publisher earns a share of the credits runs of it spend. What you pay does not change." },
  { name: "runpanel-unavailable", url: "/modals.html#runpanel-unavailable", note: "offered, unavailable, with the reason and the remedy" },
  { name: "runpanel-hosted-knowledge", url: "/modals.html#runpanel-hosted-knowledge", note: "the Knowledge step In Agentry: packs kept and scanned in Agentry can be ticked, one kept only here offers Copy to Cloud, one being scanned says so (agentry-desktop#275)", expectText: "Attach knowledge packs Agentry keeps for your account" },
  { name: "runpanel-hosted-secrets", url: "/modals.html#runpanel-hosted-secrets", note: "In Agentry for a release that declares PAGERDUTY_TOKEN: the start is refused secret-missing and the ask shows, Keep it for later runs unticked (agentry-desktop#300)", click: 'button:has-text("Run In Agentry")', expectText: "Secrets for this run" },
  { name: "runpanel-hosted-followed", url: "/modals.html#runpanel-hosted-followed", note: "Review and start In Agentry while a run In Agentry is followed in its card: Run held, the line above it says why, the card in the corner (agentry-desktop#466)", expectText: ["A run or a chat In Agentry is under way, in its card in the corner", "Running demo1/incident-summarizer@1.2.0 In Agentry"] },
  { name: "runpanel-unscanned", url: "/modals.html#runpanel-unscanned", note: "a release admitted without a scan: its own mark and consent, In Agentry not offered (M23); the line above Run names the consent (agentry-desktop#167)", expectBlocker: "Tick \u201cRun an unscanned release\u201d", expectRunDisabled: true },
  { name: "runpanel-local-install", url: "/modals.html#runpanel-local-install", note: "a QuickAgent installed from a folder on this computer, run from Installed: local and unsigned, its consent in its own words, In Agentry not offered (agentry-desktop#425)", expectText: ["It was installed from a folder on this computer: no pipeline scanned it and no one signed it.", "The run's receipts are marked local."], expectBlocker: "Tick \u201cRun an unsigned release\u201d", expectRunDisabled: true },
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
  { name: "runpanel-voice", url: "/modals.html#runpanel-voice", note: "Review and start with this computer's Writing Voice on for every run: Write in my voice, ticked from its switch (agentry-desktop#510)", expectText: ["Write in my voice", "Here it is this computer's voice."] },
  { name: "runpanel-step-review-long", url: "/modals.html#runpanel-step-review-long", note: "Review and start with an intake prompt as long as binary-reverse-engineer's: the label wraps in its column and every value keeps a readable width (agentry-desktop#420)", expectText: ["leave blank for a full pass", "none: its own instruction runs"], expectMinWidth: [".run-recap dd", 320] },
  // The author's guide (agentry-desktop#228): each step that reads it, and a
  // hostile guide that must render as text on the two steps it reaches.
  { name: "runpanel-guided", url: "/modals.html#runpanel-guided", note: "Before you start with a guide: the author's overview, then the author's What to have ready", expectText: "review brief" },
  { name: "runpanel-guided-files", url: "/modals.html#runpanel-guided-files", note: "Files: the author's line on what to add", expectText: "None needed: it reads the pull request from GitHub." },
  { name: "runpanel-guided-tools", url: "/modals.html#runpanel-guided-tools", note: "Connect tools: the key's help under its field, the link opened in the system browser", expectText: "A fine-grained token with read access to pull requests", expectLinks: ["https://github.com/settings/personal-access-tokens/new"] },
  { name: "runpanel-guided-details", url: "/modals.html#runpanel-guided-details", note: "Details: the author's task placeholder", expectPlaceholder: "e.g. https://github.com/acme/api/pull/482, and look hardest at the migration" },
  { name: "runpanel-guided-hostile", url: "/modals.html#runpanel-guided-hostile", note: "a hostile guide on Before you start: no markup of its own, no image, no javascript: link, the hidden character shown", expectText: "U+200B", expectLinks: ["https://example.com/docs"], expectNone: ".run-step img, .run-step script, .run-step iframe, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  { name: "runpanel-guided-hostile-tools", url: "/modals.html#runpanel-guided-hostile-tools", note: "the hostile guide's key help on Connect tools: text; the https link; a URL with a user name left as text; a look-alike host linked as its punycode; the override shown; a key named constructor finds no help", expectText: ["U+202E", "The author's help for", "(evil.example)", "(github.co)", "(github.com.evil.example)"], expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new", "https://evil.example/steal", "https://github.co/", "https://github.com.evil.example/"], expectNone: ".run-step img, .run-step script, .run-step b[onclick], .run-step a[href^='javascript'], .run-step a[href^='file']" },
  // Talk to it In Agentry, a hosted session (agentry-desktop#403).
  { name: "runpanel-session-offer", url: "/modals.html#runpanel-session-offer", note: "an interactive-only release with Talk to it In Agentry chosen: what a session costs, from the registry's terms", expectText: ["Talk to it In Agentry", "A session holds nothing. Its first 10 minutes are free"], expectChecked: 'input[name="run-mode"] >> nth=-1' },
  { name: "runpanel-session-tier", url: "/modals.html#runpanel-session-tier", note: "a payer on Standard: Talk to it In Agentry offered, unavailable, with the reason and See Pro", expectText: ["part of Pro, Business and Enterprise", "See Pro"] },
  { name: "runpanel-session-nohost", url: "/modals.html#runpanel-session-nohost", note: "no sandbox host answering: Talk to it In Agentry offered, unavailable, with the reason; no line says a session would queue (agentry-desktop#437)", expectText: ["No sandbox host is answering right now", "Try again in a few minutes"] },
  // A run on this machine, finished (agentry-desktop#269): Run details
  // name the operator policy the run enforced and how its path was chosen.
  { name: "runpanel-local-finished", url: "/modals.html#runpanel-local-finished", note: "a local run finished: Run details name the operator policy, the runner home's default, and its digest abbreviated", click: ['button:text-is("Run")', "details.run-details > summary"], expectText: "/home/chris/.agentry/runner/policy.yaml" },
  { name: "runpanel-local-finished-env", url: "/modals.html#runpanel-local-finished-env", note: "the same run with the runner home relocated by AGENTRY_RUNNER_HOME: the source says so", click: ['button:text-is("Run")', "details.run-details > summary"], expectText: "AGENTRY_RUNNER_HOME, set in the environment" },
  // Knowledge on a run on this machine (agentry-desktop#273): the step that
  // lists the packs kept here, and a finished run's knowledge line.
  { name: "runpanel-step-knowledge", url: "/modals.html#runpanel-step-knowledge", note: "the Knowledge step: the packs kept on this computer, each with its scan state and whether it opens offline, and one kept in Agentry only, not attachable here", expectText: "Copy to Local on the Cloud Knowledge page" },
  // A release that ships a pack and requires one of the person's own
  // (agentry-desktop#277): the ask, the shipped pack, Next held.
  { name: "runpanel-step-knowledge-ships", url: "/modals.html#runpanel-step-knowledge-ships", note: "the Knowledge step of a release that ships pagerduty-docs and requires a pack of the person's own: its ask, its shipped pack, and Next held while none is ticked", click: ['button:has-text("Next:")'], expectText: "This release needs a knowledge pack of your own" },
  { name: "install-ships-knowledge", url: "/modals.html#install-ships-knowledge", note: "the install review of a release that ships a pack: the contract's knowledge row with its size", click: "button:has-text('Verify')", expectText: "ships pagerduty-docs (4.2 MiB)" },
  { name: "runpanel-knowledge-finished", url: "/modals.html#runpanel-knowledge-finished", note: "a local run with a pack attached, finished: the receipt's knowledge line under the result", click: ['button:text-is("Run")'], expectText: "Knowledge this run could search" },
  // The Knowledge panel (agentry-desktop#273, plan knowledge.md §4.8): the
  // list in each state, Create knowledge's steps, a build under way and
  // done, a pack's preview, and a hostile page that must render as text.
  // The two places (agentry-desktop#619, plan knowledge-cards.md §4): the
  // cards, then each place's page.
  { name: "knowledge", url: "/knowledge.html#knowledge", note: "the start page: Local Knowledge and Cloud Knowledge, each with its line and its Create knowledge", locator: '[data-preview="knowledge"]', expectText: ["Local Knowledge", "Cloud Knowledge", "5 packs ·", "4 packs · 8.3 MB of 100 MB · 2 shared by acme."] },
  { name: "knowledge-local", url: "/knowledge.html#knowledge-local", note: "Local Knowledge: kept here (not scanned, opens offline), in both places, changed since it was copied, past Agentry's cap, and acme's shared copy", locator: '[data-preview="knowledge"]', expectText: ["Encrypted to your account's key at secureagentry.net, which Agentry holds", "also in your account", "changed since it was copied", "Copy to Cloud"] },
  { name: "knowledge-cloud", url: "/knowledge.html#knowledge-cloud", note: "Cloud Knowledge: the account's packs with Copy to Local and Delete from Agentry, the allocation, acme's shared group", locator: '[data-preview="knowledge"]', expectText: ["Storage in Agentry: 8.3 MB of 100 MB.", "also on this computer", "Copy to Local", "Shared by acme"] },
  { name: "knowledge-signed-out", url: "/knowledge.html#knowledge-signed-out", note: "not signed in, the Local page: a pack under this computer's key, and why Copy to Cloud waits", locator: '[data-preview="knowledge"]', expectText: "Encrypted to this computer's key" },
  { name: "knowledge-signed-out-start", url: "/knowledge.html#knowledge-signed-out-start", note: "not signed in: the Cloud card says to sign in", locator: '[data-preview="knowledge"]', expectText: "Sign in to keep knowledge in your account." },
  { name: "knowledge-empty", url: "/knowledge.html#knowledge-empty", note: "no pack yet: the cards say so", locator: '[data-preview="knowledge"]', expectText: ["No knowledge on this computer yet.", "No knowledge in your account yet."] },
  { name: "knowledge-empty-cloud", url: "/knowledge.html#knowledge-empty-cloud", note: "no pack in the account: Create knowledge, preset to In Agentry", locator: '[data-preview="knowledge"]', expectText: "No knowledge in your account yet" },
  { name: "knowledge-hosted-where-pro-cloud", url: "/knowledge.html#hosted-where-pro-cloud", note: "Create knowledge from the Cloud page for a Standard account: In Agentry not choosable, On this computer selected (plan knowledge-cards.md D5)", expectText: "Creating knowledge in Agentry is part of Pro", expectChecked: 'input[name="knowledge-where"] >> nth=0' },
  { name: "knowledge-create-done-cloud", url: "/knowledge.html#create-done-cloud", note: "a pack made here from the Cloud page: Copy to Cloud beside Preview", expectText: ["Made handbook: 48 pages", "Copy to Cloud"] },
  { name: "knowledge-create-documents", url: "/knowledge.html#create-documents", note: "Create knowledge: the documents, from the native pickers only", expectText: "Step 1 of 3: Documents" },
  { name: "knowledge-create-name", url: "/knowledge.html#create-name", note: "Create knowledge: the name, and where a run finds it", expectText: "Step 2 of 3: Name" },
  { name: "knowledge-create-review", url: "/knowledge.html#create-review", note: "the review: before anything runs, that the Creator runs here against the policy's model and nothing it reads or writes is scanned, and that a build is a big job that may run for up to 60 minutes (agentry-desktop#352, agentry-desktop#458)", expectText: ["Nothing it reads or writes is scanned", "may run for up to 60 minutes and 60 turns"], expectMinWidth: ['.step-recap dd', 320] },
  { name: "knowledge-create-blocked", url: "/knowledge.html#create-blocked", note: "docker not ready: the sentence, Create knowledge greyed", expectText: "Docker is not ready", expectDisabledButton: "Create knowledge" },
  { name: "knowledge-create-building", url: "/knowledge.html#create-building", note: "the build under way, driven by knowledge:build-progress events: time against its limit, turns, the pages written so far, the tokens; its log folded beneath (agentry-desktop#352)", expectText: ["Pages written", "14 of up to 60"] },
  { name: "knowledge-create-building-log", url: "/knowledge.html#create-building", note: "the build under way with its log opened, streaming", click: 'summary:has-text("Build log")', expectText: "decode: storage-guide.pdf" },
  { name: "knowledge-create-done", url: "/knowledge.html#create-done", note: "what the build made: pages, the key, what was not read, a dropped page, the tokens", expectText: "Made handbook: 48 pages" },
  { name: "knowledge-hosted-where", url: "/knowledge.html#hosted-where", note: "In Agentry beside this computer (agentry-desktop#274): the choice, and what it holds", expectText: "Step 3 of 4: Where" },
  { name: "knowledge-hosted-review", url: "/knowledge.html#hosted-review", note: "the review for a build In Agentry: where the documents go, the hold with the scan's credit inside it", expectText: "plus 1 credit to scan the pack" },
  { name: "knowledge-hosted-running", url: "/knowledge.html#hosted-running", note: "a build In Agentry under way: where it stands, the hold, Cancel", expectText: "100 credits are held for this build." },
  { name: "knowledge-hosted-done", url: "/knowledge.html#hosted-done", note: "a build In Agentry done: the pages, the settlement with the scan's credit, the pack kept here with its scan", expectText: "2 settled (1 of them for the pack's scan)" },
  { name: "knowledge-add-review", url: "/knowledge.html#add-review", note: "Add documents: the review, the same warnings", expectText: ["Step 2 of 2: Review", "may run for up to 60 minutes"] },
  { name: "knowledge-preview", url: "/knowledge.html#preview", note: "a pack's index and its pages, rendered by the run-output reader", expectText: "Rotating a key" },
  { name: "knowledge-preview-hostile", url: "/knowledge.html#preview-hostile", note: "a hostile page: no markup of its own, no image, no javascript: link, the right-to-left override shown", click: '.knowledge-pages button:has-text("Buckets")', expectText: "U+202E", expectNone: ".knowledge-page img, .knowledge-page script, .knowledge-page iframe, .knowledge-page b[onclick], .knowledge-page a[href^='javascript']" },
  // Two caps (agentry-desktop#354): 1 GB on this computer, 10 MB In Agentry.
  // A brief alone and Search the web (agentry-desktop#498; plan
  // knowledge-and-drafter.md D15, D16).
  { name: "knowledge-create-documents-brief", url: "/knowledge.html#create-documents-brief", note: "Create knowledge with no documents and a brief: the warning, louder; Search the web on by default, with what it takes on this computer", expectText: ["A brief", "Specific documents make the best knowledge. Without them the Creator researches the web", "Search the web", "searches with Brave and your key"] },
  { name: "knowledge-create-documents-noweb", url: "/knowledge.html#create-documents-noweb", note: "the same with no search key on this computer: a build here runs without the web, and where to add one", expectText: "On this computer a build runs without the web: no Brave key was found" },
  { name: "knowledge-create-documents-none", url: "/knowledge.html#create-empty", note: "neither documents nor a brief: Next refused with the reason", click: 'button:has-text("Next:")', expectText: "write a brief that says what the pack should cover" },
  { name: "knowledge-create-review-brief", url: "/knowledge.html#create-review-brief", note: "the review of a brief-only build on this computer: no documents, the brief, Search the web with your own keys, the warning", expectText: ["None: the Creator works from the brief", "Yes, with your own keys", "Specific documents make the best knowledge"], expectMinWidth: ['.step-recap dd', 320] },
  { name: "knowledge-hosted-review-brief", url: "/knowledge.html#hosted-review-brief", note: "the review of a brief-only build In Agentry: Search the web through the registry, the hold", expectText: ["Yes, through the registry", "plus 1 credit to scan the pack"] },
  { name: "knowledge-hosted-done-brief", url: "/knowledge.html#hosted-done-brief", note: "a brief-only build In Agentry done: what its record counted of its searches, the settlement, the pack kept here", expectText: ["This build searched the web 17 times", "espresso-care: 20 pages"] },
  { name: "knowledge-create-documents-large", url: "/knowledge.html#create-documents-large", note: "48 MB of documents: what they come to, and the warning that the build can take a very long time and the pack stays on this computer", expectText: ["48.0 MB together", "can take a very long time", "stay on this computer"] },
  { name: "knowledge-create-documents-huge", url: "/knowledge.html#create-documents-huge", note: "documents past 1 GB: Next refused with the reason", click: 'button:has-text("Next:")', expectText: "is at most 1 GB" },
  { name: "knowledge-hosted-where-large", url: "/knowledge.html#hosted-where-large", note: "the Where step with 48 MB of documents: In Agentry not choosable, and why", expectText: "In Agentry takes at most 10 MB of documents" },
  { name: "knowledge-large-pack", url: "/knowledge.html#knowledge", note: "a pack of 220 MB kept here: no Upload, and the sentence why", locator: '[data-preview="knowledge"]', expectText: "Past the 10 MB Agentry keeps, so it stays on this computer" },
  // A running build in its corner card over the page (agentry-desktop#353).
  { name: "knowledge-card-building", url: "/knowledge.html#card-building", note: "a build on this computer shrunk to the corner card: its progress over the Knowledge page, which stays usable; Stop and Open", expectText: ["Creating handbook", "5 pages written", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "knowledge-card-done", url: "/knowledge.html#card-done", note: "the card once the build is over: what it made, Dismiss and Open", expectText: ["handbook: the build is over", "Made handbook: 48 pages", "Preview", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "knowledge-card-hosted", url: "/knowledge.html#card-hosted", note: "a build In Agentry in its card: the registry's status line and Cancel the build", expectText: ["In Agentry", "The build is running on a sandbox host.", "Cancel the build"], expectNone: ".modal-backdrop" },
  // A draft's progress dialog and its corner card (agentry-desktop#410), as
  // Create knowledge's: the dialog on this computer and In Agentry, how a
  // draft ends, and the card, alone and stacked over a knowledge build's.
  // Review Your QuickAgents locally (agentry-desktop#469): the action, the
  // review's dialog and card, and its report.
  { name: "review-validate", url: "/reviews.html#validate", note: "Validate with a folder chosen: Review Your QuickAgents locally beside it, with what it does", click: 'button:text-is("Choose…")', expectText: ["Review Your QuickAgents locally", "the registry scans again at Submit", "PASSED"] },
  { name: "review-validate-nokey", url: "/reviews.html#validate-nokey", note: "the same without an AI Review key: the action says to add one, Validate unchanged", click: 'button:text-is("Choose…")', expectText: ["AI Review needs your own TypeSafe key", "Add your AI Review key", "PASSED"], expectNone: '.review-action button:text-is("Review Your QuickAgents locally")' },
  { name: "review-validate-clean", url: "/reviews.html#validate-clean", note: "a clean folder answered with null lists, as Go sent it before agentry-desktop#534: PASSED and both scans clean, not the error page", click: 'button:text-is("Choose…")', expectText: ["PASSED", "Clean — no binaries, nested archives, or encoding problems.", "Review Your QuickAgents locally"], expectNone: 'text=stopped with an error' },
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
  // A run on this computer followed above the pages (agentry-desktop#465).
  { name: "run-running", url: "/runs.html#run-running", note: "a run on this computer under way, driven by runner:run-progress: time against its limit, turns, tokens; its log folded; Stop the run and Run in the background", expectText: ["Running demo1/incident-summarizer@1.2.0", "7 of up to 60", "48,210 input", "Stop the run", "Run in the background"] },
  { name: "run-running-log", url: "/runs.html#run-running", note: "the same with its log opened: this run's lines, and not another run's", click: 'summary:has-text("Run log")', expectText: "tool: read_input incident-channel.txt", expectNoText: "A LINE OF ANOTHER RUN" },
  { name: "run-background", url: "/runs.html#run-running", note: "Run in the background from the dialog: the dialog goes and the card takes its place", click: 'button:has-text("Run in the background")', expectText: ["Running demo1/incident-summarizer@1.2.0", "7 turns", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "run-card", url: "/runs.html#run-card", note: "the run shrunk to its corner card over Installed, which stays usable: the time and turns, Stop and Open", expectText: ["Running demo1/incident-summarizer@1.2.0", "7 turns", "Stop", "Open"], expectNone: ".modal-backdrop" },
  { name: "run-stopped", url: "/runs.html#run-stopped", note: "a run stopped from its card: the card says so, with Dismiss and Open", expectText: ["the run is over", "You stopped the run. Its receipts are written as far as it got.", "Dismiss"], expectNone: ".modal-backdrop" },
  { name: "run-finished", url: "/runs.html#run-finished", note: "the run over: the result first, the files in its folder, Run details naming the operator policy; Back to the setup and Dismiss", expectText: ["Finished.", "INC-4821 began at 02:14 UTC", "Files in the run folder", "Back to the setup", "Dismiss"] },
  { name: "run-failed", url: "/runs.html#run-failed", note: "a run that ended with no result: Go's words and its log", expectText: ["The run did not finish.", "Cannot connect to the Docker daemon", "tool: read_input incident-channel.txt"] },
  { name: "run-hostile", url: "/runs.html#run-hostile", note: "a finished run whose result, files and log carry raw HTML, a script, an image and a right-to-left override: all drawn as text (D-035)", click: "details.run-details > summary", expectText: ["U+202E", "<img src=x onerror=alert(1)>"], expectNone: ".modal img, .modal script, .modal iframe, .modal b[onclick], .modal a[href^='javascript']" },
  { name: "run-card-stacked", url: "/runs.html#run-card-stacked", note: "the run's card and a draft In Agentry's card stacked in App's corner, neither covering the other", expectText: ["Running demo1/incident-summarizer@1.2.0", "Drafting a QuickAgent In Agentry"], expectNone: ".modal-backdrop" },
  { name: "hosted-queued", url: "/runs.html#hosted-queued", note: "a run In Agentry handed to the flow at Run and waiting for a host: its place in the queue, the credits held, Cancel run and Run in the background (agentry-desktop#466)", expectText: ["Running demo1/incident-summarizer@1.2.0 In Agentry", "number 3 in the queue", "20 credits held", "Cancel run", "Run in the background", "never drives it"] },
  { name: "hosted-running", url: "/runs.html#hosted-running", note: "a run In Agentry running: time against its cap, the credits held, no turns row (the registry reports turns only once it is over)", expectText: ["running on a sandbox host", "of up to 15:00", "Credits held"], expectNoText: "Turns" },
  { name: "hosted-cancel", url: "/runs.html#hosted-cancel", note: "Cancel asked for: Agentry is stopping the run, and Cancel run is gone", expectText: "Cancel asked for: Agentry is stopping the run", expectNoText: "Cancel run" },
  { name: "hosted-finished", url: "/runs.html#hosted-finished", note: "a run In Agentry over: the result first, the files in its folder, the star ask (agentry-desktop#613), Run details with the receipt's knowledge line; Back to the setup and Dismiss", expectText: ["Finished.", "INC-4821 began at 02:14 UTC", "Files in the run folder", "If this run helped, star demo1/incident-summarizer. Stars help other people find it.", "Star this QuickAgent", "Knowledge this run could search", "Back to the setup", "Dismiss"] },
  { name: "hosted-finished-starred", url: "/runs.html#hosted-finished", note: "the star ask answered: it says so in place of the ask (agentry-desktop#613)", click: 'button:has-text("Star this QuickAgent")', expectText: "Starred demo1/incident-summarizer.", expectNoText: "If this run helped" },
  { name: "hosted-hostile", url: "/runs.html#hosted-hostile", note: "a run In Agentry whose result and files carry hostile markdown: no markup of its own, no image, no javascript: link, the hidden characters shown (D-035)", expectText: "U+202E", expectNone: ".run-result-view img, .run-result-view script, .run-result-view iframe, .run-result-view b[onclick], .run-result-view a[href^='javascript']" },
  { name: "hosted-failed", url: "/runs.html#hosted-failed", note: "a run In Agentry that failed, with no receipt kept: the registry's verdict line, and no star ask", expectText: ["The agent ran and failed", "Dismiss"], expectNoText: "If this run helped" },
  { name: "hosted-lost", url: "/runs.html#hosted-lost", note: "the app lost touch with Agentry while the run was going: nothing downloaded, Follow it again", expectText: ["lost touch with Agentry while the run was still going", "Follow it again", "nothing was downloaded"] },
  { name: "hosted-card", url: "/runs.html#hosted-card", note: "a run In Agentry already in flight when the app started, attached as its card: time, credits, Cancel run and Open", expectText: ["Running demo1/incident-summarizer@1.2.0 In Agentry", "20 credits held", "Cancel run", "Open"], expectNone: ".modal-backdrop" },
  { name: "hosted-card-finished", url: "/runs.html#hosted-card-finished", note: "a run In Agentry that ended while its dialog was closed lands in its card: Finished, Dismiss and Open", expectText: ["the run is over", "Finished.", "Dismiss", "Open"], expectNone: ".modal-backdrop" },
  { name: "hosted-card-stacked", url: "/runs.html#hosted-card-stacked", note: "a run In Agentry's card and a run on this computer's card stacked in App's corner", expectText: ["Running demo1/incident-summarizer@1.2.0 In Agentry", "20 credits held", "7 turns"], expectNone: ".modal-backdrop" },
  { name: "chat-local", url: "/runs.html#chat-local", note: "a chat on this computer followed above the pages: the dialog is the chat, the store started it, the strip, Chat in the background (agentry-desktop#467)", expectText: ["Chat with demo1/incident-summarizer@1.2.0", "INC-4821 began at 02:14 UTC", "interactive · 1 turn · 1 tool call · $0.0182", "Chat in the background"] },
  { name: "chat-local-working", url: "/runs.html#chat-local-working", note: "its corner card while a message is out: the QuickAgent is working, the turns, Finish and Open", expectText: ["The QuickAgent is working · 1 turn", "Finish", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-local-waiting", url: "/runs.html#chat-local-waiting", note: "its card between turns: waiting for you", expectText: ["Waiting for you · 1 turn", "Finish", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-local-ended", url: "/runs.html#chat-local-ended", note: "the engine ended the chat with its dialog closed: the store collected its result, and the card says how it ended", expectText: ["it is over", "Finished.", "Dismiss", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-hosted", url: "/runs.html#chat-hosted", note: "a chat In Agentry: replayed from the host, a file added, a hostile agent reply as plain text (no markup, no link), the time-and-credits strip, D2's sentence", expectText: ["input added: /inputs/pager-export.csv", "interactive in Agentry · 2 turns · 1 tool call · 12:", "free minutes left 0 · 100 credits for time · idle closes in", "Transport encrypted by the user key, messages not stored"], expectNone: ".chat-pane script, .chat-pane b[onclick], .chat-pane a" },
  { name: "chat-hosted-working", url: "/runs.html#chat-hosted-working", note: "its card while the agent answers a message", expectText: ["Chat with demo1/incident-summarizer@1.2.0 In Agentry", "The QuickAgent is working · 2 turns", "Finish", "Open"], expectNone: ".modal-backdrop" },
  { name: "chat-hosted-waiting", url: "/runs.html#chat-hosted-waiting", note: "its card between turns", expectText: ["Waiting for you · 1 turn"], expectNone: ".modal-backdrop" },
  { name: "chat-hosted-ended", url: "/runs.html#chat-hosted-ended", note: "a chat In Agentry over: the result view, the session's credits (100 for time, 4 for tokens), transcript.md among the files, and the star ask (agentry-desktop#613)", click: ["details.run-details > summary"], expectText: ["This session held nothing: its time cost 100 credits (2 minutes after the 10 free)", "receipts/transcript.md", "If this run helped, star demo1/incident-summarizer.", "Dismiss"] },
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
  { name: "settings-quickagent-updates", url: "/pages.html?settings=updates", note: "Settings: the auto-update switch, shown and disabled, with why", locator: '[data-preview="settings"] .card:has-text("Update QuickAgents automatically")' },
  // The welcome's runtime step (agentry-desktop#131): the three questions;
  // the model list Get models filled; the failed listing with the model
  // still typeable; a stored subscription token and its sentence.
  // Reached by Back from the AI Review key step (#462), as a person would.
  { name: "wizard-runtime", url: "/wizard.html#3", note: "the runtime step: provider, key, model", locator: ".wizard", click: '.wizard-foot button:has-text("Back")', expectText: "Set up the runtime" },
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
  // The card leads with the operator's recommendation and the link to
  // TypeSafe (#461): the sentence word for word in every state, and the
  // link opening the one address, read from what the app asked to open.
  { name: "settings-aireview-key", url: "/pages.html?settings=provider", note: "Settings → AI Review key: the recommendation and the TypeSafe link, then saved in the keychain, the value never shown, Test key and Forget", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', expectText: ["Saved in this computer's keychain.", KEY_RECOMMENDATION, "Get a key from TypeSafe"] },
  { name: "settings-aireview-key-none", url: "/pages.html?settings=provider#aireview-none", note: "no AI Review key: the recommendation, its link opening https://typesafe.ai and nothing else, drafting on this computer is off until one is added; Test key greyed", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', click: '[data-preview="settings"] button:has-text("Get a key from TypeSafe")', expectText: ["Drafting on this computer is off", KEY_RECOMMENDATION, "Get a key from TypeSafe", "Submitting needs no key of yours"], expectOpened: ["https://typesafe.ai"] },
  { name: "settings-aireview-key-env", url: "/pages.html?settings=provider#aireview-env", note: "the key from the variable the app was started with: the recommendation, and the line naming the variable", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', expectText: ["$TYPESAFE_API_KEY is set where this app was started", KEY_RECOMMENDATION, "Get a key from TypeSafe"] },
  { name: "settings-web-search", url: "/pages.html?settings=provider", note: "Settings → Web search (agentry-desktop#301): Brave chosen and saved in the keychain, the providers the runner policy admits, the value never shown, Test key and Forget", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', expectText: ["Brave is chosen, and its key is saved in this computer's keychain.", "under that provider's own terms", "Forget the saved Brave key"] },
  { name: "settings-web-search-none", url: "/pages.html?settings=provider#search-none", note: "no search provider: a draft reads the registry and the pages it knows; Test key greyed", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', expectText: "No search provider chosen." },
  { name: "settings-web-search-env", url: "/pages.html?settings=provider#search-env", note: "the key from the variable the app was started with", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', expectText: "$BRAVE_API_KEY is set where this app was started" },
  { name: "settings-web-search-refused", url: "/pages.html?settings=provider#search-refused", note: "Test key answered 401: Brave's refusal in plain words, and which key was tried", locator: '[data-preview="settings"] .card:has-text("Your own key for a web search service")', click: '[data-preview="settings"] .card:has-text("Your own key for a web search service") button:has-text("Test key")', expectText: "Brave refused the key (401)" },
  { name: "settings-aireview-key-refused", url: "/pages.html?settings=provider#aireview-refused", note: "Test key answered 401: TypeSafe's refusal in plain words, and which key was tried", locator: '[data-preview="settings"] .card:has-text("Your own TypeSafe Jev key")', click: '[data-preview="settings"] button:has-text("Test key")', expectText: "TypeSafe refused the key (401)" },
  { name: "settings-coding-agents", url: "/pages.html?settings=coding-agents", note: "Settings → Coding agents: Claude Code chosen, the Claude skill in ~/.claude/skills, the switch off", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-codex", url: "/pages.html?settings=coding-agents#coding-codex", note: "Codex chosen: AGENTS.md in the chosen project folder, the switch on", locator: '[data-preview="settings"] .card:has-text("Also install into")' },
  { name: "settings-coding-agents-none", url: "/pages.html?settings=coding-agents#coding-none", note: "nothing chosen, no folder: AGENTS.md needs a project, the switch cannot go on", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectDisabledSwitch: true },
  { name: "settings-coding-agents-both", url: "/pages.html?settings=coding-agents#coding-both", note: "both chosen: the Claude skill is what is written, so the switch names Claude Code alone", locator: '[data-preview="settings"] .card:has-text("Also install into")', expectSwitchLabel: "Also install into Claude Code" },
  { name: "agent-install-coding-agent", url: "/pages.html", note: "the agent page's one-off link opens the wizard preset to the Claude skill", locator: ".modal", click: '[data-preview="agent"] button:has-text("Install into Claude Code")' },
  // The Welcome's Coding agent step, on the same list Settings uses
  // (agentry-desktop#148 moved it): unchanged for the person.
  { name: "wizard-coding-agent", url: "/wizard.html#4", note: "the welcome's coding agent step after Look: Claude Code found and chosen, Codex missing", locator: ".wizard" },
  // Reached by Skip on the AI Review key step (#462), as a person would.
  { name: "wizard-coding-agent-unprobed", url: "/wizard.html#3", note: "the step as it opens, reached by Skip on the AI Review key step: what the policy records, nothing run", locator: ".wizard", click: '.wizard-foot button:has-text("Skip")', expectText: "Your coding agent" },
  // The AI Review key step (#462): Provider settings' card in the wizard,
  // with Skip only while there is no key. Reached by Continue from Runtime
  // and by Back from Coding agent; and a save the keychain refuses.
  { name: "wizard-review-key", url: "/wizard.html#2", note: "the AI Review key step with no key, reached by Continue from Runtime: the recommendation, the TypeSafe link, the field, Skip and Continue", locator: ".wizard", click: '.wizard-foot button:has-text("Continue")', expectText: ["Your AI Review key", KEY_RECOMMENDATION, "Get a key from TypeSafe", "No key saved. Drafting on this computer is off until you add one."], expectOrder: [".wizard-foot button", ["Back", "Skip", "Continue"]] },
  { name: "wizard-review-key-saved", url: "/wizard.html#4-saved", note: "the step with a key saved, reached by Back from Coding agent: saved, never shown, and no Skip", locator: ".wizard", click: '.wizard-foot button:has-text("Back")', expectText: ["Your AI Review key", "Saved in this computer's keychain."], expectOrder: [".wizard-foot button", ["Back", "Continue"]] },
  { name: "wizard-review-key-refused", url: "/wizard.html#3-refused", note: "a save the keychain refuses: the reason shown, no key saved, Skip still offered", locator: ".wizard", fill: [['.wizard input[aria-label="Jev key"]', "tsk-example-not-a-key"]], click: '.wizard button:has-text("Save")', expectText: ["store the key in the OS keychain", "No key saved."], expectOrder: [".wizard-foot button", ["Back", "Skip", "Continue"]], expectValue: ['.wizard input[aria-label="Jev key"]', "tsk-example-not-a-key"] },
  { name: "wizard-review-key-saving", url: "/wizard.html#3-slow", note: "a save in flight: Back, Skip and Continue held until it answers, as the Runtime step holds its own while images pull", locator: ".wizard", fill: [['.wizard input[aria-label="Jev key"]', "tsk-example-not-a-key"]], click: '.wizard button:has-text("Save")', expectText: "Saving…", expectFootDisabled: true },
  // Every page under the shell and every step of the first-run wizard, so a
  // change to the token table (agentry-desktop#49) is seen everywhere it
  // lands, not only in the states above.
  { name: "page-browse", url: "/pages.html", note: "Browse under the shell: Validate is not listed outside Build (agentry-desktop#422)", locator: '[data-preview="browse"]', expectOrder: ['[data-preview="browse"] .sidebar .nav-item', ["Browse", "Installed", "Knowledge", "Voice", "Build", "Settings"]] },
  { name: "page-agent", url: "/pages.html", note: "an agent's page under the shell", locator: '[data-preview="agent"]' },
  { name: "page-installed", url: "/pages.html", note: "Installed under the shell, with a QuickAgent installed from a folder: local and unsigned, Run in place of Details (agentry-desktop#425)", locator: '[data-preview="installed"]', expectText: ["me/csv-quality-check", "local, unsigned", "from a folder on this computer; no registry holds it"] },
  { name: "page-build", url: "/pages.html", note: "Build under the shell: the start page's two cards, Describe it to AI first, then Build it by hand, with the operator's words and the tagline, as the site's /build (agentry-desktop#598)", locator: '[data-preview="build"]', expectText: [BUILD_TAGLINE, DESCRIBE_COPY, BY_HAND_COPY, "Switch to Advanced"], expectOrder: [
    ['[data-preview="build"] .build-way-main', ["Describe it to AI", "Build it by hand"]],
    ['[data-preview="build"] .sidebar .nav-item', ["Browse", "Installed", "Knowledge", "Voice", "Build", "Validate", "Settings"]],
    ['[data-preview="build"] .sidebar .nav-item.active', ["Build"]],
  ] },
  // The cards open their views (#598): a press anywhere on Build it by hand
  // opens Step by step, and its Switch to Advanced opens Advanced.
  { name: "page-build-by-hand", url: "/pages.html", note: "Build it by hand pressed in the middle of the card: Step by step on Basics", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-way:nth-child(2)', expectText: "Step 1 of 8: Basics" },
  { name: "page-build-by-hand-advanced", url: "/pages.html", note: "Build it by hand's Switch to Advanced: every section at once", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-way-part', expectText: "Switch to step by step" },
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
  { name: "page-build-review", url: "/pages.html#build-review", note: "Step by step: Review, the recap with Edit and the actions", locator: '[data-preview="build"]', expectText: "Step 8 of 8: Review", expectMinWidth: ['[data-preview="build"] .step-recap dd', 320] },
  { name: "page-build-advanced", url: "/pages.html#build-advanced", note: "Advanced: every section at once", locator: '[data-preview="build"]', expectText: ["Before a run", "The first save writes it to a new folder named for it in your output folder"] },
  // Describe it, on this computer (#282): the description with the person's
  // AI Review key saved, and without one (the Settings pointer, Draft it
  // greyed); a draft on Review with the Drafter's notes; a hostile draft
  // whose notes, match, source and withheld address must stay text.
  { name: "page-build-describe", url: "/pages.html#build-describe", note: "Describe it: the description, the hints, on this computer with the policy's model, the key from the keychain, the search line and where the description goes", locator: '[data-preview="build"]', expectText: "AI Review uses your key from this computer's keychain" },
  { name: "page-build-describe-nokey", url: "/pages.html#build-describe-nokey", note: "Describe it with no AI Review key: the Settings pointer, the TypeSafe link, and Draft it greyed", locator: '[data-preview="build"]', click: '[data-preview="build"] button:has-text("Get a key from TypeSafe")', expectText: "needs your own TypeSafe Jev key", expectOpened: ["https://typesafe.ai"] },
  { name: "page-build-drafted", url: "/pages.html#build-drafted", note: "a draft on Review, How it was drafted opened: AI Review's check of the finished draft, what AI Review read and held back, the registry's match with its page, the open questions, the Drafter's notes, the sources", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["AI Review read 6 results with your key and held back 0", "The release check: flagged.", "reaching for something the contract does not declare (C3, score 3) in AGENT.md", "This finding sends the version to review when you submit"] },
  { name: "page-build-drafted-hostile", url: "/pages.html#build-drafted-hostile", note: "a hostile draft on Review, How it was drafted opened: no markup of its own, no image, no javascript: link, the withheld address and the check's hostile finding as text, the override shown", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["U+202E", "<img src=x onerror=alert(1)>"], expectNone: '[data-preview="build"] .card img, [data-preview="build"] .card script, [data-preview="build"] .card iframe, [data-preview="build"] .card b[onclick], [data-preview="build"] .card a[href^="javascript"], [data-preview="build"] .card a[href*="evil.example"]' },
  // Describe it's Knowledge pick (agentry-desktop#497): on this computer the
  // packs kept here, a pack with no scan in Agentry ticked and the warning
  // that AI Review scans it first with the person's key; a pick past 10 MB
  // refused before Draft it; In Agentry the packs Agentry keeps, one kept
  // only here greyed with why; Review naming the packs a draft read and
  // what AI Review withheld from one.
  { name: "page-build-describe-knowledge", url: "/pages.html#build-describe-knowledge", note: "Describe it on this computer with the Knowledge pick: a pack scanned in Agentry and one kept only here ticked, the total, and the warning that AI Review scans the second first with your key", locator: '[data-preview="build"]', click: ['[data-preview="build"] label:has-text("On this computer") input', '[data-preview="build"] label:has-text("m6-dev-runbook") input', '[data-preview="build"] label:has-text("team-notes") input'], expectText: ["Knowledge, if you keep some", "Picked 2 of up to 5", "team-notes has not been scanned in Agentry, so AI Review reads every file of it with your key"] },
  { name: "page-build-describe-knowledge-over", url: "/pages.html#build-describe-knowledge-over", note: "a pick past 10 MB: the sentence, and Draft it greyed", locator: '[data-preview="build"]', click: ['[data-preview="build"] label:has-text("On this computer") input', '[data-preview="build"] label:has-text("manuals") input', '[data-preview="build"] label:has-text("catalogue") input'], expectText: "These packs hold 11 MB, and a draft reads at most 10 MB of knowledge in all" },
  { name: "page-build-describe-agentry-knowledge", url: "/pages.html#build-describe-agentry-knowledge", note: "Describe it In Agentry with the Knowledge pick: the packs Agentry keeps, an organization's shared one, and a pack kept only here greyed with why", locator: '[data-preview="build"]', click: '[data-preview="build"] label:has-text("handbook") input', expectText: ["shared by acme", "kept on this computer only: upload it to Agentry to draft with it there", "Picked 1 of up to 5"] },
  { name: "page-build-drafted-knowledge", url: "/pages.html#build-drafted-knowledge", note: "a draft that read two packs, How it was drafted opened: one scanned in Agentry, one scanned with your key with a page withheld and why", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["The knowledge it read", "scanned in Agentry; nothing withheld", "scanned with your key for this draft; 1 file withheld", "wiki/topics/override.md"] },
  // Describe it In Agentry (#281): the hold in the site's words, the caps
  // and the balance; a payer short of the hold; a draft queued and
  // followed; signed out; a draft made In Agentry on Review, with Describe
  // again under the notes.
  { name: "page-build-describe-agentry", url: "/pages.html#build-describe-agentry", note: "Describe it In Agentry: the hold sentence, the balance, the caps, the honest line, On this computer beside it", locator: '[data-preview="build"]', expectText: "This draft holds 41 credits and settles from what it uses" },
  { name: "page-build-describe-agentry-short", url: "/pages.html#build-describe-agentry-short", note: "In Agentry for a payer short of the hold: the notice and Credits and upgrade", locator: '[data-preview="build"]', expectText: "fewer credits than a draft holds" },
  { name: "page-build-describe-agentry-drafting", url: "/pages.html#build-describe-agentry-drafting", note: "Describe it while a draft In Agentry runs: the form held, and Show its progress, which opens the draft's dialog (agentry-desktop#410)", locator: '[data-preview="build"]', expectText: ["A draft is being made In Agentry", "Show its progress"] },
  { name: "page-build-describe-signedout", url: "/pages.html#build-describe-signedout", note: "In Agentry signed out: offered, and it needs an account", locator: '[data-preview="build"]', expectText: "needs an account" },
  { name: "page-build-drafted-agentry", url: "/pages.html#build-drafted-agentry", note: "a draft made In Agentry on Review, How it was drafted opened: AI Review through the registry, a host the description did not name, a rule it still fails, and Describe again", locator: '[data-preview="build"]', click: '[data-preview="build"] summary:has-text("How it was drafted")', expectText: ["Hosts your description did not name", "Describe again"] },
  // The summary (plan builder-simplify.md §4.1, agentry-desktop#344): how it
  // works and every kind of capability the model declares, Submit private
  // unless switched, See it in the form; a landed draft's check in one line;
  // hostile text drawn as text; a rule still failing, with Fix it.
  { name: "page-build-summary", url: "/pages.html#build-summary", note: "the summary of a QuickAgent written in the form: how it works, its key with the author's help, its website, its MCP server with its tools, host and configuration, its file and data grants; Save, Install and Submit, and what each does (agentry-desktop#425)", locator: '[data-preview="build"]', expectText: ["How it works", "incident-summarizer", "PAGERDUTY_TOKEN, for the QuickAgent", "The author's help for PAGERDUTY_TOKEN", "api.pagerduty.com", "npm:@cyanheads/osv-advisory-mcp-server@0.1.13", "May call all of its tools.", "Configuration: LOG_LEVEL.", "api.osv.dev", "Reads the files in its workspace.", "Writes reports in its workspace.", "Touches these kinds of data: tickets.", "Save writes it to a new folder named for it in your output folder", "puts it on this computer only, in Installed", "makes it public", "It first says what the submission costs"], expectNone: '[data-preview="build"] .build-summary .step-problem, [data-preview="build"] .build-summary input[type="checkbox"]', expectOrder: [['[data-preview="build"] .build-summary .row button', ["Save", "Install", "Submit"]]] },
  // Save, Install and Submit (agentry-desktop#425): each dialog is the Go
  // side's and native, so the harness shows what the page says after a Yes.
  { name: "page-build-summary-review", url: "/pages.html#build-summary", note: "the summary's Review Your QuickAgents locally beside Save, Install and Submit, with what it does: the registry's rules here, AI Review with your key, your model, no credits, it asks first and submits nothing (agentry-desktop#469)", locator: '[data-preview="build"]', expectText: ["Review Your QuickAgents locally", "the registry scans again at Submit", "openai qwen3.8-flash-next"], expectOrder: [['[data-preview="build"] .build-summary .row button', ["Save", "Install", "Submit"]]] },
  { name: "page-build-summary-nokey", url: "/pages.html#build-summary-nokey", note: "the summary without an AI Review key: the action says to add one, with the way to Provider settings, and Submit is the same (agentry-desktop#469)", locator: '[data-preview="build"]', expectText: ["AI Review needs your own TypeSafe key", "Add your AI Review key"], expectNone: '[data-preview="build"] .review-action button:text-is("Review Your QuickAgents locally")' },
  { name: "page-build-summary-saved", url: "/pages.html#build-summary", note: "after Save: written with no file dialog to a new folder named for it under the default output folder (agentry-desktop#585)", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Save")', expectText: ["Saved to /home/me/Downloads/agentry-out/incident-summarizer", "Save writes it to /home/me/Downloads/agentry-out/incident-summarizer"] },
  { name: "page-build-summary-installed", url: "/pages.html#build-summary", note: "after Install: written to the default output folder with no file dialog (#585), then laid down on this computer, local and unsigned, with the way to Installed", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Install")', expectText: ["installed demo1/incident-summarizer@0.1.0 on this computer — local and unsigned", "No registry holds it and nothing was submitted.", "Open Installed", "Save writes it to /home/me/Downloads/agentry-out/incident-summarizer"] },
  { name: "page-build-summary-submitted", url: "/pages.html#build-summary", note: "after Submit: written to the default output folder with no file dialog (#585), then published, and public", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Submit")', expectText: ["published demo1/incident-summarizer@0.1.0", "It is public.", "Save writes it to /home/me/Downloads/agentry-out/incident-summarizer"] },
  { name: "page-build-summary-private", url: "/pages.html#build-summary-private", note: "after Submit to an agent that exists private: it stays private, why, and the way to its settings on the site (the operator's decision of 2026-09-30)", locator: '[data-preview="build"]', click: '[data-preview="build"] .build-summary button:text-is("Submit")', expectText: ["it stays private: making a QuickAgent public is its namespace administrators' act", "Its settings on the site"] },
  { name: "page-build-landed", url: "/pages.html#build-landed", note: "a draft that landed while the tab was away, opened as Build mounts: the draft stays in the form though the fresh form's NewModel answers after it (agentry-desktop#410)", locator: '[data-preview="build"]', expectText: ["csv-quality-check"], expectNone: '[data-preview="build"] .build-summary:has-text("no name yet")' },
  { name: "page-build-summary-drafted", url: "/pages.html#build-summary-drafted", note: "a draft landed on the summary: AI Review's check in one line, the host the description did not name marked, no Drafter's notes or counts", locator: '[data-preview="build"]', expectText: ["flagged something the release check looks for", "It reaches hosts your description did not name", "your description did not name it", "csv-quality-check"], expectNone: '[data-preview="build"] .build-drafted-notes, [data-preview="build"] .build-summary ul[aria-label="Keys"], [data-preview="build"] .build-summary ul[aria-label="MCP servers"]' },
  { name: "page-build-summary-hostile", url: "/pages.html#build-summary-hostile", note: "a hostile overview, key help and host on the summary: no markup of its own, no image, no javascript: link, the override shown", locator: '[data-preview="build"]', expectText: ["U+202E", "<img src=x onerror=alert(1)>"], expectNone: '[data-preview="build"] .build-summary img, [data-preview="build"] .build-summary script, [data-preview="build"] .build-summary b[onclick], [data-preview="build"] .build-summary a[href^="javascript"]' },
  { name: "page-build-summary-problem", url: "/pages.html#build-summary-problem", note: "the summary of a QuickAgent a rule still refuses: the rule named with Fix it in its step, Submit greyed", locator: '[data-preview="build"]', expectText: ["It cannot be submitted yet. Before a run:", "Fix it in Before a run"], expectDisabledSummarySubmit: true },
  { name: "page-build-summary-public", url: "/pages.html#build-summary-public", note: "a folder whose publish.include leaves out README.md: Submit waits with Fix it in Publishing, Save and Install do not, since only a submission is public (agentry-desktop#425)", locator: '[data-preview="build"]', expectText: ["It can be saved and installed, but not submitted yet: a submission is public.", "Fix it in Publishing"], expectDisabledSubmitOnly: true },
  { name: "page-build-comments", url: "/pages.html#build-comments", note: "a loaded agent.yaml with comments: the notice that a save drops them", locator: '[data-preview="build"]', expectText: "the comments are not kept" },
  { name: "page-voice", url: "/pages.html", note: "the Voice page as two cards (agentry-desktop#610): Local Voice and Cloud Voice, each a figure over its words and its status line; no Voice Writer card", locator: '[data-preview="voice"]',
    expectText: ["Local Voice", "Cloud Voice", "The voice kept on this computer. Edit it by hand or with AI for free", "AI Review checks every version before it goes live", "Voice set · updated", "· used by default.", "Live since", "a new version was flagged by AI Review"],
    expectNoText: ["Start the Voice Writer", "Submit to Agentry"],
    expectOrder: ['[data-preview="voice"] .sidebar .nav-item', ["Browse", "Installed", "Knowledge", "Voice", "Build", "Settings"]] },
  { name: "page-voice-none", url: "/pages.html#voice-none", note: "the two cards with no voice in either place", locator: '[data-preview="voice"]', expectText: ["No voice on this computer yet.", "No voice in your account yet."] },
  { name: "page-voice-signed-out", url: "/pages.html#voice-signed-out", note: "the two cards signed out: Cloud Voice asks for a sign-in", locator: '[data-preview="voice"]', expectText: ["No voice on this computer yet.", "Sign in to keep a voice in your account."] },
  { name: "page-voice-local", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")'], note: "the Local Voice page: Download is Already the same (the digests match), Upload says it asks first because the account's flagged version is newer and costs 1 credit, Edit with AI, the switch shared with Run defaults, the editor and Delete", locator: '[data-preview="voice"]',
    expectText: ["Download Cloud version", "Already the same.", "Upload to Cloud", "for 1 credit", "so you are asked first", "Edit with AI", "Use this voice for every run on this computer", "the same switch as Settings → Run defaults", "Delete this computer's voice"] },
  { name: "page-voice-local-none", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")'], note: "the Local Voice page with no voice: Write with AI, Download with no live voice, Upload with nothing to upload, the empty editor", locator: '[data-preview="voice"]',
    expectText: ["Write with AI", "Your account has no live voice yet.", "This computer keeps no voice to upload yet.", "No voice on this computer yet."], expectNoText: ["Delete this computer's voice"] },
  { name: "page-voice-dialog", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] button:has-text("Edit with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")'], note: "Edit with AI: what should change, two samples from the native picker, the cost line, Start held until the note is written", locator: '[data-preview="voice"]',
    expectText: ["What should change?", "Add writing samples (optional)", "/home/me/writing/launch-email.md", "Free, on this computer"] },
  { name: "page-voice-dialog-none", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] button:has-text("Write with AI")'], note: "Write with AI with no voice here: samples required, the text an optional brief", locator: '[data-preview="voice"]',
    expectText: ["How do you want to sound? (optional)", "Samples of your writing", "Free, on this computer"] },
  { name: "page-voice-dialog-failed", url: "/pages.html#voice-failed", click: ['[data-preview="voice"] button.build-way-main:has-text("Local Voice")', '[data-preview="voice"] button:has-text("Write with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")', '[data-preview="voice"] .modal button:text-is("Start")', '[data-preview="voice"] .modal summary:has-text("The last run\'s log")'], note: "a Write with AI run that failed: the error, and the run's log kept under it so the person can see why (found by agentry-desktop#610's live check)", locator: '[data-preview="voice"]',
    expectText: ["the Voice Writer wrote no VOICE.md", "The last run's log", "decode: 1 files"] },
  { name: "page-voice-cloud", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page, the site's Voice settings in the desktop (agentry-desktop#611): the flagged new version with its finding and Discard, the editor on it with Edit with AI In Agentry under it, the switch shared with Profile, Restore previous (reviewed again: the question set changed) and Delete; no Download or Upload here (D7)", locator: '[data-preview="voice"]',
    expectText: ["Your new version", "Flagged", "AI Review flagged this version, so your live voice stays live.", "It asks for more than a way of writing", "Discard this version", "Your voice", "An AI Edit, 2026-10-01 22:20 UTC, reviewed 2026-10-01 22:21 UTC. 316 bytes.", "The editor holds your newest version", "Edit with AI", "It runs In Agentry.", "Use this voice for every run In Agentry", "the same switch as Settings → Profile", "Restore previous", "The previous version, written by the Voice Writer, is from 2026-10-01 20:00 UTC.", "so Restore sends it to AI Review again, for 1 credit. It takes the place of the version waiting above.", "Delete voice"],
    expectNoText: ["Use Agentry's voice here", "Download Cloud version", "Upload to Cloud"] },
  { name: "page-voice-cloud-none", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page with no voice in the account: the empty editor and Write with AI, no switch, Restore or Delete", locator: '[data-preview="voice"]',
    expectText: ["Your account keeps no Writing Voice yet.", "Write with AI", "The Voice Writer writes your voice from samples of your writing. It runs In Agentry."], expectNoText: ["Use by default", "Delete voice", "Restore previous"] },
  { name: "page-voice-cloud-reviewing", url: "/pages.html#voice-reviewing", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "a first version in review with nothing live: the page follows it every 5 seconds, the switch says no run writes in it yet, no Restore", locator: '[data-preview="voice"]',
    expectText: ["Your first version", "In review", "AI Review is reading this version. It goes live when it passes, and this page updates itself.", "Your voice is not live yet", "Delete voice"], expectNoText: ["Restore previous", "Flagged"] },
  { name: "page-voice-cloud-dialog", url: "/pages.html", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")', '[data-preview="voice"] .voice-cloud-ai button:has-text("Edit with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")'], note: "Edit with AI In Agentry: what should change, two samples from the native picker with the upload limits, the price and that it waits for AI Review, Start held until the note is written", locator: '[data-preview="voice"]',
    expectText: ["What should change?", "Add writing samples (optional)", "/home/me/writing/launch-email.md", "at most 20 files, 10 MB together", "It costs 20 credits, AI Review included.", "It rewrites your newest version", "waits for AI Review"], expectNoText: ["Free, on this computer"] },
  { name: "page-voice-cloud-running", url: "/pages.html#voice-none", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")', '[data-preview="voice"] .voice-cloud-ai button:has-text("Write with AI")', '[data-preview="voice"] .modal button:has-text("Choose files")', '[data-preview="voice"] .modal button:text-is("Start")'], note: "a Write with AI run In Agentry started: its status, Cancel run, and Close, since the page keeps following it", locator: '[data-preview="voice"]',
    expectText: ["The Voice Writer is running In Agentry", "Queued: waiting for a sandbox host.", "Cancel run", "You can close this"] },
  { name: "page-voice-cloud-attached", url: "/pages.html#voice-attached", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page opened again while an AI Edit In Agentry runs: the run followed again, Show it, and Edit with AI held rather than offering a second", locator: '[data-preview="voice"]',
    expectText: ["The Voice Writer is running In Agentry: Running: the Voice Writer is reading your voice and your samples.", "Show it"] },
  { name: "page-voice-standard", url: "/pages.html#voice-standard", click: ['[data-preview="voice"] button.build-way-main:has-text("Cloud Voice")'], note: "the Cloud Voice page on Standard with this cycle's Voice Writer run used: Edit with AI held, the date the next is theirs, and the upgrade (brought back by agentry-desktop#611)", locator: '[data-preview="voice"]',
    expectText: ["this cycle's runs are used. The next is yours on 28 October 2026.", "Upgrade"] },
  { name: "page-validate", url: "/pages.html", note: "Validate under the shell: a sub-item of Build, with Build lit beside it (agentry-desktop#422)", locator: '[data-preview="validate"]', expectOrder: [
    ['[data-preview="validate"] .sidebar .nav-item', ["Browse", "Installed", "Knowledge", "Voice", "Build", "Validate", "Settings"]],
    ['[data-preview="validate"] .sidebar .nav-item.active', ["Build", "Validate"]],
    ['[data-preview="validate"] .sidebar [aria-current="page"]', ["Validate"]],
  ] },
  // Settings as five cards (agentry-desktop#346): the landing with each
  // card's status line; each section page; a card opened, a part's link
  // followed, the way back; and each deep link App gives a caller.
  { name: "page-settings", url: "/pages.html", note: "Settings under the shell: five cards, each with its sentence, its status line and its parts", locator: '[data-preview="settings"]',
    expectText: ["Registry settings", "Update settings", "Coding agents", "Run defaults", "Provider settings", "secureagentry.net, signed in.", "Agentry Desktop 0.2.0 is available.", "AI Review key saved", "Runs stop after 15 minutes, on this computer.", "Settings file:"],
    expectSection: "landing" },
  // Settings → Profile (agentry-desktop#423): offered only while the
  // active registry holds a token, first on the landing.
  { name: "settings-landing-profile", url: "/pages.html", note: "signed in: the Profile card first on the landing", locator: '[data-preview="settings"]',
    expectOrder: ['[data-preview="settings"] .settings-card-link strong', ["Profile", "Registry settings", "Update settings", "Coding agents", "Run defaults", "Provider settings"]] },
  { name: "settings-landing-signed-out", url: "/pages.html?active=anon", note: "the active registry not signed in: no Profile card", locator: '[data-preview="settings"]',
    expectOrder: ['[data-preview="settings"] .settings-card-link strong', ["Registry settings", "Update settings", "Coding agents", "Run defaults", "Provider settings"]] },
  { name: "settings-profile", url: "/pages.html?settings=profile", note: "Profile: the handle, the verified email, signed up with Google, the display name; Standard's credits, allowance, private QuickAgents, knowledge, hosted-run caps, Upgrade", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["chris · chris@example.test (verified)", "Signed up with Google", "Save profile", "40 of 50 credits · 40 submissions left this cycle", "4 of 10", "20 MB of 100 MB", "A hosted run holds 20 credits for its cap", "Upgrade"] },
  { name: "settings-profile-save", url: "/pages.html?settings=profile", note: "the display name changed and saved: the card shows the name the registry answered, and says so", locator: '[data-preview="settings"]',
    fill: [['[data-preview="settings"] input[aria-label="Display name"]', "Chris Q"]], click: '[data-preview="settings"] button:has-text("Save profile")',
    expectSection: "profile", expectText: "Display name saved.", expectNone: '[data-preview="settings"] .error-box' },
  { name: "settings-profile-promo", url: "/pages.html?settings=profile#profile-promo", note: "Promo Pro: the promo credits, the trial's code and end, Standard pending, the expiry notice", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Promo Pro", "88 of 100 promo credits", "Promo trial", "LAUNCH", "(unverified)", "Signed up with GitHub", "Standard from 29 October 2026", "You will not be charged."] },
  { name: "settings-profile-pro", url: "/pages.html?settings=profile#profile-pro", note: "Pro offered credit packs (agentry-desktop#511): the Top up credits card, each base struck and the credits larger, the bonus, the price, Buy opening the site's card", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Top up credits", "never expire", "This account holds 1,250 bought credits.", "+250 bonus", "+1,500 bonus", "US$9.99", "US$34.99", "bought credits never do", "this app never takes money"] },
  { name: "settings-profile-suspended", url: "/pages.html?settings=profile#profile-suspended", note: "Pro suspended for non-payment: what it owes and the link to the site's card, above the Top up credits card", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["This account is suspended for non-payment.", "owes 40 credits", "Top up credits on the site", "Top up credits", "-40 of 1000 credits · 0 submissions left this cycle"] },
  { name: "settings-profile-org", url: "/pages.html?settings=profile#profile-org", note: "a member of acme: the organization's tier, pool, seats, standing and knowledge pool; Manage billing opens its console", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Business", "acme covers your work", "The pool", "3960 of 5000 credits this cycle", "organization acme · admin", "in acme's pool", "Your own account", "Manage billing"] },
  { name: "settings-profile-core", url: "/pages.html?settings=profile#profile-core", note: "a registry that sells nothing: the profile, and no Subscription part, as on the site", locator: '[data-preview="settings"]',
    expectSection: "profile", expectText: ["Save profile"], expectNone: '[data-preview="settings"] #settings-part-subscription' },
  { name: "settings-registry-account", url: "/pages.html?settings=registry", note: "Registry settings keeps who the credential is and links the Profile card; the tier and credits moved there", locator: '[data-preview="settings"]',
    expectSection: "registry", expectText: ["signed in as chris", "Profile and subscription"], expectNone: '[data-preview="settings"] #settings-part-registries :text("credits")' },
  { name: "settings-open-registry", url: "/pages.html", note: "the Registry settings card opened: the registries, Secrets and Add a registry under ← Settings", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Registry settings") .settings-card-link', expectSection: "registry", expectText: ["← Settings", "Registries", "Add a registry", "Secrets for runs In Agentry"] },
  { name: "settings-open-updates", url: "/pages.html", note: "Update settings: the self-update card and the QuickAgents switch", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] .settings-card:has-text("Update settings") .settings-card-link', expectSection: "updates", expectText: ["Check for newer versions", "Update QuickAgents automatically"] },
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
  { name: "settings-docker-images", url: "/pages.html?settings=run-defaults", note: "Run defaults → Docker images: Agentry's images by name, the CLI's use of them, and Remove (agentry-desktop#424)", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: ["Docker images", "3 Docker images Agentry put on this computer", "ghcr.io/blaktron/agentry-mcp-gateway:v0.43.3 (86.3MB)", "The agentry CLI uses the same images", "Remove Agentry's Docker images"] },
  { name: "settings-docker-images-removed", url: "/pages.html?settings=run-defaults", note: "after Remove (the native Yes is the Go host's): the report, and none listed", locator: '[data-preview="settings"]',
    click: '[data-preview="settings"] button:has-text("Remove Agentry\'s Docker images")', expectSection: "run-defaults", expectText: ["Removed 3 images.", "No Docker images of Agentry's are on this computer."] },
  { name: "settings-docker-images-nodocker", url: "/pages.html?settings=run-defaults#images-nodocker", note: "Docker not answering: nothing listed, and why", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: "Docker's images could not be listed", expectNone: '[data-preview="settings"] button:has-text("Remove Agentry\'s Docker images")' },
  { name: "settings-run-defaults", url: "/pages.html?settings=run-defaults", note: "Run defaults: timeout and turns (the policy's, which the CLI shares), where to run, the output folder, Docker", locator: '[data-preview="settings"]',
    expectSection: "run-defaults", expectText: ["Timeout (seconds)", "the agentry CLI runs with them too", "Where to run", "every consent is asked on every run", "None chosen", "Docker engine"],
    expectValue: ['[data-preview="settings"] #settings-part-budgets input[type="number"] >> nth=0', "900"] },
  // The built-in agents' switch (agentry-desktop#606, D-062): off by default
  // beside the timeout it lifts, then on.
  { name: "settings-builtin-no-timeout-off", url: "/pages.html?settings=run-defaults", note: "Run defaults → Timeout and turns: the built-in agents' no-time-limit switch, off", locator: '[data-preview="settings"] #settings-part-budgets',
    expectSection: "run-defaults", expectText: ["No time limit for the built-in agents", "drafts, knowledge builds, local", "until it finishes or you stop", "keeps its own 5 minutes", "Your QuickAgents keep the timeout above"] },
  { name: "settings-builtin-no-timeout-on", url: "/pages.html?settings=run-defaults", note: "the switch turned on: drafts, builds, reviews and voices on this computer run until they finish", locator: '[data-preview="settings"] #settings-part-budgets',
    click: '[data-preview="settings"] input[data-builtin-no-timeout]', expectSection: "run-defaults", expectText: "No time limit for the built-in agents", expectChecked: '[data-preview="settings"] input[data-builtin-no-timeout]' },
  // Unsafe Mode (agentry-desktop#418): the switch off and on in Run defaults,
  // the pull under way, and the red edge over a page and over a dialog.
  // `window` shoots the window, not the locator, since the edge is the window's;
  // on pages.html, which stacks every page, it keeps the one the locator names.
  { name: "settings-unsafe-off", url: "/pages.html?settings=run-defaults", note: "Run defaults → Unsafe Mode, off: what it lifts, what stays, and what turning it on does", locator: '[data-preview="settings"] #settings-part-unsafe',
    expectSection: "run-defaults", expectText: ["Unsafe Mode", "Off", "commands this computer has not approved", "or from PyPI", "run_command still takes only the commands a QuickAgent declares", "never uses Unsafe Mode", "ghcr.io/blaktron/agentry-unsafe-kali"],
    expectNone: ".unsafe-glow, .unsafe-badge", expectUnsafeSwitch: false },
  { name: "settings-unsafe-on", url: "/pages.html?settings=run-defaults#unsafe-on", note: "Unsafe Mode on: the switch reads on, the red edge runs round the window and the sidebar's badge says Unsafe Mode", locator: '[data-preview="settings"]', window: true,
    expectSection: "run-defaults", expectText: ["Unsafe Mode", "On"], expectUnsafeSwitch: true, expectGlow: true, scrollTo: "#settings-part-unsafe" },
  { name: "settings-unsafe-running", url: "/pages.html?settings=run-defaults#unsafe-running", note: "Unsafe Mode turned off while a run compiled in it is still live (agentry-desktop#479): the switch reads off, the red edge stays and the badge says it is running", locator: '[data-preview="settings"]', window: true,
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
  // none by default (agentry-desktop#468): the control unset, then set.
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
  { name: "runpanel-chat-review", url: "/modals.html#runpanel-chat-review", note: "Review and start for a chat on this computer, Advanced open: no timeout (s), the line that says a chat here has no time limit, max turns kept (agentry-desktop#468)",
    click: "details.advanced > summary", expectText: ["Start session", "A chat here has no time limit.", "max turns (0 = unlimited)"], expectNone: 'details.advanced span:text-is("timeout (s)")', scrollTo: "[data-chat-limits]" },
  { name: "settings-deeplink-selfupdate", url: "/pages.html", note: "the sidebar's newer-version line opens Update settings", locator: '[data-preview="settings"]', click: '[data-deeplink="selfUpdate"]', expectSection: "updates", expectPart: "desktop" },
  { name: "settings-deeplink-setup", url: "/pages.html", note: "Browse's setup notice opens Provider settings", locator: '[data-preview="settings"]', click: '[data-deeplink="setupNotice"]', expectSection: "provider" },
  { name: "settings-deeplink-knowledge", url: "/pages.html", note: "Knowledge's Not signed in opens Registry settings at the registries", locator: '[data-preview="settings"]', click: '[data-deeplink="knowledgeSignIn"]', expectSection: "registry", expectPart: "registries" },
  { name: "settings-deeplink-describe", url: "/pages.html", note: "Build's Describe it without an AI Review key opens Provider settings at the key", locator: '[data-preview="settings"]', click: '[data-deeplink="describeKey"]', expectSection: "provider", expectPart: "ai-review-key" },
  { name: "settings-deeplink-unsafe", url: "/pages.html#unsafe-on", note: "the sidebar's Unsafe Mode badge opens Run defaults at the switch", locator: '[data-preview="settings"]', click: '[data-deeplink="unsafeMode"]', expectSection: "run-defaults", expectPart: "unsafe", expectUnsafeSwitch: true },
  { name: "settings-status-failed", url: "/pages.html#status-refused", note: "the landing when every status read fails: the five cards stay, their lines drop", locator: '[data-preview="settings"]',
    expectSection: "landing", expectCards: 5, expectStatusLines: 1 },
  { name: "toolspanel", url: "/modals.html#toolspanel", note: "the tools panel over the shell" },
  { name: "toolspanel-hostile", url: "/modals.html#toolspanel-hostile", note: "the hostile guide's key help in the tools panel (agentry-desktop#343): markdown, framed, each link's host beside it, nothing that acts", linksIn: ".key-help", expectText: ["The author's help for", "U+202E", "(evil.example)", "(github.com.evil.example)"], expectLinks: ["https://github.com/settings/tokens", "https://xn--gthub-n2e.com/new", "https://evil.example/steal", "https://github.co/", "https://github.com.evil.example/"], expectNone: ".key-help img, .key-help script, .key-help b[onclick], .key-help a[href^='javascript'], .key-help a[href^='file']" },
  { name: "chat", url: "/modals.html#chat", note: "a session's chat pane", locator: ".chat-pane" },
  { name: "wizard-welcome", url: "/wizard.html#0", note: "the first-run wizard: welcome", locator: ".wizard" },
  { name: "wizard-signin", url: "/wizard.html#1", note: "the first-run wizard: sign-in", locator: ".wizard" },
  // Reached by Continue from Coding agent, and the key's own Continue before
  // it (#462): the two renumbered steps forward.
  { name: "wizard-done", url: "/wizard.html#3-saved", note: "the first-run wizard: done, reached by Continue from the AI Review key step and from Coding agent, with the key's row", locator: ".wizard", click: ['.wizard-foot button:has-text("Continue")', '.wizard-foot button:has-text("Continue")'], expectText: ["You're set up", "Provider anthropic (claude-sonnet-4-5) written to the runner policy", "AI Review key saved"] },
  { name: "wizard-done-key-only", url: "/wizard.html#5-keyonly", note: "done after the AI Review key alone was saved: the policy its save wrote is not a provider set up, so the ending is still Nothing set up", locator: ".wizard", expectText: ["Nothing is set up yet", "The runner policy has only the default provider, anthropic (claude-sonnet-4-5), with its key read from $ANTHROPIC_API_KEY", "AI Review key saved"] },
  { name: "wizard-done-nothing", url: "/wizard.html#5-nothing", note: "the first-run wizard: every step skipped, the Nothing set up ending, the key's row pending", locator: ".wizard", expectText: ["Nothing is set up yet", "No AI Review key — drafting on this computer is off until you add one in Settings"] },
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
      // expectNoText: a sentence that must not show anywhere, such as a line
      // of another run's log (agentry-desktop#465).
      for (const unwanted of [state.expectNoText ?? []].flat()) {
        const text = await page.locator(state.locator ?? "body").innerText();
        if (text.includes(unwanted)) throw new Error(`${state.name}: the page shows "${unwanted}"`);
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
        // picture (agentry-desktop#421, the Build start page's ways in). One
        // [selector, texts] pair, or a list of them.
        const pairs = Array.isArray(state.expectOrder[0]) ? state.expectOrder : [state.expectOrder];
        for (const [sel, want] of pairs) {
          const got = (await page.locator(sel).allInnerTexts()).map((t) => t.trim());
          if (JSON.stringify(got) !== JSON.stringify(want)) throw new Error(`${state.name}: ${sel} reads ${got.join(" | ")}, not ${want.join(" | ")}`);
        }
      }
      if (state.expectMinWidth) {
        // A layout check, not only a picture: every matching element keeps
        // at least this width (agentry-desktop#420, a recap value squeezed
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
        // harness's App.OpenExternal records each address (#461).
        const opened = await page.evaluate(() => window.__opened ?? []);
        if (JSON.stringify(opened) !== JSON.stringify(state.expectOpened)) throw new Error(`${state.name}: opened ${JSON.stringify(opened)}, not ${JSON.stringify(state.expectOpened)}`);
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
        // click; the badge says the same in words (plan unsafe-mode.md D13).
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
        await page.locator("select").first().selectOption("custom");
        await page.locator('input[placeholder="https://llm.example.net/v1"]').fill("llm.example.net/v1");
        await page.locator('input[type="password"]').first().fill("sk-preview-key");
        await page.locator('input[placeholder^="pick one with Get models"]').fill("qwen3.8-flash");
        await page.getByRole("button", { name: "Save provider settings" }).click();
        await page.waitForTimeout(300);
        // The refusal shown, and the key kept: the field says it is stored
        // and is empty, so nothing asks for it again (agentry-desktop#141).
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
