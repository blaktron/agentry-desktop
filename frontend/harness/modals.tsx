// Run-surface previews (agentry-desktop#39, extended for In Agentry in
// #77): RunPanel and ToolsPanel are modals and SessionChat is a chat pane.
// Mocks the bindings and selects one surface by location.hash so each gets
// a clean screenshot. Not shipped.
//
//   #runpanel              the mode choice, In Agentry offered
//   #runpanel-hosted       In Agentry chosen: the hold, the balance, the caps
//   #runpanel-unavailable  In Agentry offered, unavailable with the reason
//   #runpanel-hosted-earns In Agentry chosen for a release whose publisher is
//                          in Publisher Earnings: the line under the cost
//                          (agentry-desktop#613)
//   #runpanel-hosted-followed  Review and start In Agentry while a run In
//                          Agentry is followed in its card: Run held, and
//                          the line above it says why (agentry-desktop#466)
//   (a run In Agentry, once started, is the run's flow's since
//   agentry-desktop#466: runs.html#hosted-* shows it queued, running, over,
//   failed and with hostile output)
//   #runpanel-keys         pr-review-brief's shape: keys unsaved, no intake —
//                          the line above Run names the keys, Input files
//                          collapsed (agentry-desktop#167)
//   #runpanel-file-intake  a release asking for a file up front
//   #runpanel-unscanned    a release the registry admitted without a scan:
//                          its own mark and consent, In Agentry not offered
//   #install-unscanned     the install wizard on the same release (M23)
//   #install-mcp-env       the install review of a release declaring two MCP
//                          servers, one with plain env (agentry-desktop#62)
//   #toolspanel | #chat    the other two run surfaces
//
// The guided run setup (agentry-desktop#226) shows one step at a time, so
// each state opens on the step that shows it (STEP_OF below), and four
// more show the steps themselves:
//   #runpanel-step-files    Files: the chosen files, Add files / Add folder
//   #runpanel-step-tools    Connect tools on pr-review-brief's shape: the
//                           declared MCP server and the unsaved key
//   #runpanel-step-details  Details: the intake question, then the task
//   #runpanel-step-review   Review and start: the recap, output, consents
//   #runpanel-voice         Review and start with this computer's voice on:
//                           Write in my voice, ticked
//   #runpanel-chat-review   Review and start for a chat on this computer:
//                           Advanced has no timeout, and says why
//   #runpanel-schedule      Review and start with When on Repeat (Schedule
//                           a new run, agentry-desktop#743): the check ready
//   #runpanel-schedule-gaps the check names what a fire would lack
//   #runpanel-schedule-windows  the signed-out box off, with the reason
//   #runpanel-step-review-long  the same with an intake prompt as long as
//                           binary-reverse-engineer's: the label wraps and
//                           every value keeps its width (agentry-desktop#420)
//
// The author's guide (manifest v2's guide block, agentry-desktop#228), on
// pr-review-brief's shape with a guide:
//   #runpanel-guided          Before you start: the overview, then the
//                             author's "What to have ready"
//   #runpanel-guided-files    Files: the author's line on what to add
//   #runpanel-guided-tools    Connect tools: the key's help under its field,
//                             its link handed to the system browser
//   #runpanel-guided-details  Details: the author's task placeholder
//   #runpanel-guided-hostile  a guide carrying raw HTML, a script, an image,
//                             javascript: and file: links and hidden
//                             characters, on Before you start
//   #runpanel-guided-hostile-tools  the same guide's key help on Connect tools
//
// Knowledge attached to a run on this machine (plan knowledge.md §4.5,
// agentry-desktop#273):
//   #runpanel-step-knowledge      the Knowledge step: the packs kept on this
//                                 computer, each with its scan state
//   #runpanel-local-finished-files a local run given files, finished (shots.mjs
//                                 clicks Run): the receipt's decode line,
//                                 ClamAV's and Hybrid Analysis's outcomes
//                                 (agentry-desktop#530); -nokey: no Hybrid
//                                 Analysis key, so "not checked: no key"
//   #runpanel-knowledge-finished  a local run with a pack attached, finished
//                                 (shots.mjs clicks Run): the receipt's
//                                 knowledge line under the result
//
// A release that ships a pack and asks for one of the person's own
// (agentry-desktop#277, plan knowledge.md §4.6):
//   #runpanel-step-knowledge-ships  the Knowledge step: the release's ask,
//                                   required; its shipped pack; the packs
//                                   kept here, none ticked, so Next is held
//   #install-ships-knowledge        the install review: the contract's
//                                   knowledge row with the pack's size
//
// Knowledge attached to a run In Agentry (agentry-desktop#275):
//   #runpanel-hosted-knowledge          the Knowledge step In Agentry: packs
//                                       kept and scanned in Agentry can be
//                                       ticked; one kept only here offers
//                                       Copy to Cloud; one being scanned
//                                       there says so
//   (a run In Agentry that is over is the run's flow's since
//   agentry-desktop#466: runs.html#hosted-finished shows its knowledge line)
//
// A run In Agentry of a release that declares a secret (agentry-desktop#300):
//   #runpanel-hosted-secrets      Review and start In Agentry: the note on
//                                 what the start asks for; shots.mjs clicks
//                                 Run In Agentry and the registry answers
//                                 secret-missing, so the ask shows, Keep it
//                                 for later runs unticked
//
// Talk to it In Agentry, a hosted session (agentry-desktop#403), on an
// interactive-only copy of the release:
//   #runpanel-session-offer    Before you start with Talk to it In Agentry
//                              chosen: what a session costs, from the
//                              registry's terms
//   #runpanel-session-tier     a payer on Standard: the choice offered,
//                              unavailable, with the reason and See Pro
//   #runpanel-session-nohost   no sandbox host answering: the choice offered,
//                              unavailable, with the reason (#437)
//   (a chat, once started, is the chat's flow's since agentry-desktop#467:
//   runs.html#chat-* shows it here and In Agentry, going and over)
//
// A run on this machine, finished (shots.mjs clicks Run on the review):
//   #runpanel-local-finished      its Run details name the operator policy
//                                 it enforced, the runner home's default
//                                 (agentry-cli#152, agentry-desktop#269)
//   #runpanel-local-finished-env  the same run with the runner home
//                                 relocated by AGENTRY_RUNNER_HOME
//
// A release with a remote server that signs in by OAuth, Google's Gmail
// server (agentry-desktop#666), on Connect tools:
//   #runpanel-signin             not connected: Connect, and what it asks for
//   #runpanel-signin-connecting  Connect clicked: the wait, the consent
//                                address, and the field for the address a
//                                browser on another computer ended on
//   #runpanel-signin-own-client  Connect clicked: Google lets no app register
//                                itself, so Agentry's Google steps (the
//                                Developer Preview and Test users steps
//                                among them) and the client form
//   #runpanel-signin-connected   connected, with Connect again and Disconnect
//   #runpanel-signin-review      Review and start: Run held until connected
//   #runpanel-signin-hosted      Connect tools In Agentry (agentry-desktop
//                                #667): Agentry's own connections, made on
//                                the site, and Connect in your browser now
//   #runpanel-signin-hosted-connect  Review and start In Agentry: shots.mjs
//                                clicks Run In Agentry, the registry answers
//                                connection-missing, Connect tools opens in
//                                the browser and the start waits
import { createRoot } from "react-dom/client";
import "../src/style.css";

// In Agentry (hosted runs M6). The view is the panel's: what a run holds,
// where it stands, and what it produced — the registry's numbers.
const view = location.hash.replace("#", "") || "runpanel";
// #install-mcp-env: two declared servers, one with plain env and a tool
// allowlist, one with neither (agentry-desktop#62).
const mcpEnvServers = [
  { id: "npm:@modelcontextprotocol/server-github@2025.4.8", tools: ["get_issue", "list_issues", "search_issues"],
    env: [{ name: "GITHUB_API_URL", value: "https://api.github.com" }, { name: "GITHUB_TOOLSETS", value: "issues,pull_requests" }] },
  { id: "pypi:mcp-server-fetch@2025.4.7", tools: [], env: [] },
];
const hostedState = view.startsWith("runpanel-") ? view.slice("runpanel-".length) : "";

// The run form asks only for what the release declares (agentry-desktop#167).
// #runpanel-keys is agentry/pr-review-brief's shape: one GitHub MCP server
// with GITHUB_TOKEN, no intake, no filesystem — the keys unsaved, so the
// line above Run names them and nothing else; #runpanel-file-intake a
// release that asks for a file up front — the intake form shows and the
// unanswered question is what the line names.
const guidedShape = view.startsWith("runpanel-guided") || view === "toolspanel-hostile";
const keysShape = view === "runpanel-keys" || view === "runpanel-keys-saved" || view === "runpanel-step-tools" || guidedShape;
// A Gmail-reading release whose one server is Google's remote Gmail server,
// signing in by OAuth with the person's own client (agentry-desktop#666).
const signInShape = view.startsWith("runpanel-signin");
const hostedSignIn = view.startsWith("runpanel-signin-hosted");
const gmailServer = "https://gmailmcp.googleapis.com/mcp/v1";
const signInRelease = {
  ref: "acme/inbox-brief@0.1.0", digest: "sha256:7f3a2b9c1d4e5f60",
  servers: [{
    server: gmailServer, what: gmailServer, scopes: ["https://www.googleapis.com/auth/gmail.readonly"], sharesGrantWith: [],
    ...(view === "runpanel-signin-connected"
      ? { state: "connected", issuer: "https://accounts.google.com", client: "own", createdAt: "2026-10-06T14:02:00Z", lastUsedAt: "2026-10-06T14:05:00Z", granted: ["https://www.googleapis.com/auth/gmail.readonly"] }
      : { state: "none", granted: [] }),
  }],
};
// Agentry's Google steps, as Go words them (internal/run OwnClientSteps).
const googleGuide = {
  issuer: "https://accounts.google.com", vendor: "Google", known: true, docs: "https://docs.secureagentry.net/agentry/google-client/",
  steps: [
    "In the Google Cloud console (console.cloud.google.com), create a project or pick one of yours.",
    "APIs & Services → Library: enable the Gmail API and the Gmail MCP API (gmailmcp.googleapis.com).",
    "Google's remote MCP servers are in Google's Workspace Developer Preview: enrol this project at developers.google.com/workspace/preview. Google asks for a Google Workspace account and can take a couple of days. Until the project is enrolled, every tool call answers \"The caller does not have permission\". A QuickAgent that uses the workspace-mcp server instead reaches Gmail through the Gmail API and needs no enrolment.",
    "Google Auth Platform → Branding: give the app a name and your email address.",
    "Audience: choose Internal if your account is in a Google Workspace organization; otherwise External, then Publish app so it is In production. A client left in Testing gets sign-ins that stop working after 7 days.",
    "If the app stays in Testing: Audience → Test users → Add users, and add the Google account you will sign in with. Google refuses the sign-in of any account a Testing app does not list.",
    "Data Access → Add or remove scopes: add the scopes this QuickAgent asks for (https://www.googleapis.com/auth/gmail.readonly).",
    "Clients → Create client → Application type: Desktop app. Copy its client ID and client secret into the fields below.",
  ],
  note: "Google may show an \"unverified app\" screen when you sign in with a client of your own: the app is yours, and Google says so because it has not reviewed it. Every QuickAgent you connect to Google from this computer uses the client you keep here; each version still asks for its own consent.",
};
const hostileGuide = view.startsWith("runpanel-guided-hostile") || view === "toolspanel-hostile";
// What pr-review-brief's author could write in its guide (DEV-141).
const guide = hostileGuide
  ? {
    overview:
      "# Reviews your PR\n\nIt is **careful**. <script>alert('x')</script><b onclick=\"steal()\">raw html</b>\n\n" +
      "![tracking pixel](https://evil.example/p.gif) [open me](javascript:alert(1)) [docs](https://example.com/docs)",
    prepare: ["A pay\u200Bpal token <b onclick=\"steal()\">now</b>", "javascript:alert(1)"],
    files: "<img src=\"https://evil.example/p.gif\"> none",
    task: "e.g. <script>alert(1)</script>",
    connect: {
      GITHUB_TOKEN: "Paste it <b onclick=\"steal()\">here</b>. Not javascript:alert(1), file:///etc/passwd or https://github.com@evil.example/x but https://github.com/settings/tokens\u202E, or the look-alike https://g\u0456thub.com/new.\n\n" +
        // As markdown (agentry-desktop#343): an image, a javascript: link, link
        // text that claims another host, bold link text, and a host hidden in
        // percent-encoding each keep their real host beside them.
        "![pixel](https://evil.example/p.gif) [open me](javascript:alert(1)) [GitHub settings](https://evil.example/steal) [**github.com**](https://github.co/) <https://github.com%2Eevil.example/>",
    },
  }
  : {
    overview:
      "Reads one pull request and returns a **review brief**: what changed, what to look at first, and the questions to ask its author.\n\n" +
      "It reads the pull request through the GitHub API and never pushes, comments or approves.",
    prepare: ["The pull request's URL", "A GitHub token that can read that repository's pull requests"],
    files: "None needed: it reads the pull request from GitHub.",
    task: "e.g. https://github.com/acme/api/pull/482, and look hardest at the migration",
    connect: {
      GITHUB_TOKEN: "A fine-grained token with read access to pull requests: https://github.com/settings/personal-access-tokens/new.",
    },
  };
const fileIntakeShape = view === "runpanel-file-intake";
// An intake prompt as long as blaktron/binary-reverse-engineer@0.1.0's,
// which once took the recap's whole width (agentry-desktop#420).
const longPromptShape = view === "runpanel-step-review-long";
const unsafeShape = view.startsWith("runpanel-unsafe");
// A finished local run: no intake question, so Run on the review is enabled.
const localFinished = view.startsWith("runpanel-local-finished") || view === "runpanel-knowledge-finished" || view === "runpanel-unsafe-finished";
const hostedKnowledge = view.startsWith("runpanel-hosted-knowledge");
const hostedSecrets = view === "runpanel-hosted-secrets";
const hostedEarns = view === "runpanel-hosted-earns";
const sessionShape = view.startsWith("runpanel-session");
// The first start of #runpanel-hosted-secrets is refused secret-missing;
// the one after the ask is saved is queued.
let hostedStarts = 0;
const shipsShape = view === "runpanel-step-knowledge-ships" || view === "install-ships-knowledge";
const knowledgeShape = view === "runpanel-step-knowledge" || view === "runpanel-knowledge-finished" || hostedKnowledge || shipsShape;
const knowledgePacks = knowledgeShape
  ? [
    { name: "incident-runbooks", pages: 131, bytes: 6_902_112, scan: "scanned in Agentry, 2 findings", scanState: "scanned", local: true, inAgentry: true, agentryReady: true, changed: false, where: "this computer and Agentry", key: "your account's key at secureagentry.net, which Agentry holds", wrapper: "registry", registry: "secureagentry.net", opensOffline: false },
    { name: "oncall-notes", pages: 12, bytes: 184_320, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, agentryReady: false, changed: false, where: "this computer", key: "your account's key at secureagentry.net, which Agentry holds", wrapper: "registry", registry: "secureagentry.net", opensOffline: true },
    { name: "supplier-contracts", pages: 27, bytes: 1_203_000, scan: "scanned in Agentry, 0 findings", scanState: "scanned", local: false, inAgentry: true, agentryReady: true, changed: false, where: "Agentry", opensOffline: false },
    ...(hostedKnowledge
      ? [{ name: "vendor-sla", pages: 3, bytes: 41_000, scan: "being scanned in Agentry", scanState: "scanning", local: false, inAgentry: true, agentryReady: false, changed: false, where: "Agentry", opensOffline: false }]
      : []),
  ]
  : [];
const localResult = {
  status: "ok", exitCode: 0, runId: "01K5ZQ4W7X2M9N3P6R8T0V1Y2B", ref: "demo1/incident-summarizer@1.2.0",
  digest: "sha256:9c356fd0d20fef07", resultJson: JSON.stringify("INC-4821 began at 02:14 UTC with elevated 5xx on the checkout service. Draft report written to reports/report.md."),
  outDir: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-01K5ZQ4W",
  receiptsDir: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-01K5ZQ4W/receipts",
  runPlanSha: "5d41402abc4b2a76b9719d911017c592ae2f1c3e8b6d7a9f0e1d2c3b4a596877",
  inputTokens: 5120, outputTokens: 1430, costUsd: 0.0182, wallMs: 41200, turns: 4, attestationState: "verified",
  policy: {
    ...(view === "runpanel-local-finished-env"
      ? { path: "/srv/agentry-runner/policy.yaml", source: "env" }
      : { path: "/home/chris/.agentry/runner/policy.yaml", source: "default" }),
    sha256: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
  },
  knowledge: view === "runpanel-knowledge-finished"
    ? "incident-runbooks mounted read-only at /knowledge/incident-runbooks (sha256:3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855, 131 pages, 6902112 bytes, scan: a scan record this client has not confirmed with the registry); tools knowledge_list, knowledge_search and knowledge_read, on the tools container only"
    : undefined,
  decode: view === "runpanel-local-finished-files-nokey"
    ? "the inputs were decoded before the run, in agentry-decode:6edfce5958ccc415 (built on ghcr.io/blaktron/agentry-files:2026.10.04.3@sha256:f8da034b365cd86c1498b041e87ee06faeb4264dfc6e25bd9c0d3013cf58a306) with no network, every capability dropped and the inputs read-only, by bsdcat, bsdtar, clamscan, file, java, pandoc, pdftotext and tesseract (clamav-scanner-1.4.3-r0, pandoc-cli-3.6.4-r0, poppler-utils-25.04.0-r0, tika-app-3.3.2) and the runner's own spreadsheet, slide, zip and tar readers: 1 decoded, 1 read as text; each file read and each archive unpacked was checked by ClamAV (clamscan, PUA off) before its text was extracted, against the signatures of 2026-10-05 06:24 UTC (daily 28144, main 63; ghcr.io/blaktron/agentry-clamav-db:latest@sha256:df8194fd3a27): 2 clean; Hybrid Analysis (CrowdStrike), asked by SHA-256 alone and never sent a file, on the files of a type it is asked about: 1 not checked: no key"
    : view === "runpanel-local-finished-files"
    ? "the inputs were decoded before the run, in agentry-decode:6edfce5958ccc415 (built on ghcr.io/blaktron/agentry-files:2026.10.04.3@sha256:f8da034b365cd86c1498b041e87ee06faeb4264dfc6e25bd9c0d3013cf58a306) with no network, every capability dropped and the inputs read-only, by bsdcat, bsdtar, clamscan, file, java, pandoc, pdftotext and tesseract (clamav-scanner-1.4.3-r0, pandoc-cli-3.6.4-r0, poppler-utils-25.04.0-r0, tika-app-3.3.2) and the runner's own spreadsheet, slide, zip and tar readers: 1 decoded, 1 read as text; each file read and each archive unpacked was checked by ClamAV (clamscan, PUA off) before its text was extracted, against the signatures of 2026-10-05 06:24 UTC (daily 28144, main 63; ghcr.io/blaktron/agentry-clamav-db:latest@sha256:df8194fd3a27): 2 clean; Hybrid Analysis (CrowdStrike), asked by SHA-256 alone and never sent a file, on the files of a type it is asked about: 1 clean"
    : undefined,
  unsafeExec: view === "runpanel-unsafe-finished"
    ? "UNSAFE MODE: run_command restricted to the argv0 allowlist {7z, analyzeHeadless, binwalk, decompyle3, file, objdump, pyinstxtractor, r2, strings}; the tools container is the Unsafe Mode image ghcr.io/blaktron/agentry-unsafe-kali:2026.09.30.2@sha256:87fa4ae406a9621cef3a4cffcdccf2a512da03141ca21727ce663ca715a191da with the declared commands installed one at a time, from Kali Linux's repository or, for a pip extra, PyPI (7z ← apt, analyzeHeadless ← link ghidra:/usr/share/ghidra/support/analyzeHeadless, binwalk ← apt, decompyle3 ← pip decompyle3==3.9.3, file ← apt, objdump ← apt, r2 ← apt, strings ← apt — apt packages: 7zip, binutils, binwalk, file, ghidra, radare2); this machine's exec catalogue was not consulted; the image is not minimized, and a declared command runs with the image's own PATH after its shims, so it can reach every program the image holds; not available in Unsafe Mode's image: pyinstxtractor; packages resolved at image build: binutils=2.45-3, file=1:5.46-5, ghidra=11.4.2-0kali1, radare2=6.0.2+dfsg-1; NOT installed at image build (the commands named are absent from this run unless the base image already carries them): binwalk (E: Unable to locate package binwalk) for binwalk"
    : undefined,
};
const profile = signInShape
  ? {
    ref: signInRelease.ref, digest: signInRelease.digest,
    capabilityLines: ["network egress: none", "filesystem: read none, write none", "MCP: https://gmailmcp.googleapis.com/mcp/v1 (remote; signs in by OAuth)"],
    declaredSecrets: [], storedSecretNames: [], exists: false, keep: true,
    modes: ["autonomous"], intake: [], opening: "", shipsKnowledge: [],
  }
  : keysShape
  ? {
    ref: "agentry/pr-review-brief@1.2.1", digest: "sha256:4e6d650976b7a1b2",
    capabilityLines: ["network egress: none", "filesystem: read none, write none", "MCP: github-mcp-server-js (api.github.com; secrets: GITHUB_TOKEN)", "dataAccess: code"],
    declaredSecrets: hostileGuide ? ["GITHUB_TOKEN", "constructor"] : ["GITHUB_TOKEN", "SEARCH_KEY"], storedSecretNames: [], exists: false, keep: true,
    // SEARCH_KEY is optional (agentry-desktop#655): marked so in the keys
    // form, and the line above Run names GITHUB_TOKEN alone.
    optionalSecrets: hostileGuide ? [] : ["SEARCH_KEY"],
    modes: ["autonomous"], intake: [], opening: "",
    ...(guidedShape ? { guide } : {}),
  }
  : {
    ref: "demo1/incident-summarizer@1.2.0", digest: "sha256:9c356fd0d20fef07",
    capabilityLines: ["network egress: api.pagerduty.com", "filesystem: read workspace, write reports/", "secrets: PAGERDUTY_TOKEN"],
    declaredSecrets: ["PAGERDUTY_TOKEN"], storedSecretNames: ["PAGERDUTY_TOKEN"], exists: true, keep: true,
    modes: sessionShape ? ["interactive"] : ["autonomous", "interactive"],
    intake: fileIntakeShape
      ? [{ id: "channel_export", prompt: "The incident channel's export (a file)", type: "file", required: true, choices: [] }]
      : localFinished || view === "runpanel-hosted-secrets" || sessionShape ? []
      : longPromptShape
      ? [{ id: "focus", prompt: "Focus area (optional): a function, embedded payload, or part of the binary to prioritize; leave blank for a full pass.", type: "text", required: false, choices: [] }]
      : [{ id: "since", prompt: "Summarise incidents since when?", type: "text", required: true, choices: [] }],
    opening: "What incident should I look at?",
    ...(shipsShape
      ? { shipsKnowledge: ["pagerduty-docs"], acceptsKnowledge: { prompt: "Your team's own incident runbooks", required: true } }
      : { shipsKnowledge: [] }),
  };
const setup = { dockerOk: true, imagesReady: true, timeoutSeconds: 900, maxTurns: 60, policyExists: true, provider: "anthropic", model: "claude-sonnet-4-5" };
const mcp = {
  refusal: "", vulnerabilityLine: "have no known high or critical advisories", toolRows: [],
  servers: keysShape ? ["github-mcp-server-js"] : [], vulnerabilities: [], dataAccess: keysShape ? ["code"] : [],
  declaredServers: signInShape
    ? [{ id: gmailServer, tools: ["search_threads", "get_thread"], env: [], remote: true, signsIn: true }]
    : keysShape
    ? [{ id: "npm:github-mcp-server-js@0.1.7", tools: ["get_pull_request", "list_pull_request_files", "list_pull_request_commits"], env: [{ name: "GITHUB_PERMISSION", value: "read-only" }] }]
    : [],
  secretNames: keysShape ? ["GITHUB_TOKEN"] : [], embeddedTools: [],
};

// Settings → Run defaults (agentry-desktop#347): #runpanel-default-agentry
// has where to run saved as In Agentry, and the setup opens with it chosen;
// #runpanel-default-agentry-unscanned the same for a release admitted without
// a scan, which starts on this computer and says why; #runpanel-default-folder
// a saved output folder, filled in on Review and start.
// Unsafe Mode (agentry-desktop#418): #runpanel-unsafe-review is Review and
// start with Run defaults' switch on, for a release shaped like
// blaktron/binary-reverse-engineer: the line names the declared commands
// and the one the image cannot provide, the red edge frames the dialog, and
// the sidebar behind it carries the badge. #runpanel-unsafe-finished is
// that run finished, its result naming what was installed and what was not.
const unsafeNotice = {
  on: unsafeShape,
  line: unsafeShape
    ? "Unsafe Mode is on: every command this QuickAgent declares is installed from Kali Linux's repository or PyPI without asking this computer's approved list, and each runs with the image's own PATH, so it can start any program that image holds. run_command still takes only the declared commands. As resolved, it declares: 7z, analyzeHeadless, binwalk, decompyle3, file, objdump, pyinstxtractor, r2, strings. Not available in Unsafe Mode's image: pyinstxtractor. The run goes on without it."
    : "",
  unplaced: unsafeShape ? ["pyinstxtractor"] : [],
};
const runDefaults = {
  unsafeMode: unsafeShape,
  timeoutSeconds: 900, maxTurns: 60, policyPath: "~/.agentry/runner/policy.yaml",
  where: view.startsWith("runpanel-default-agentry") ? "agentry" : "here",
  outputDir: view === "runpanel-default-folder" || view.startsWith("runpanel-schedule") ? "/home/me/agentry-runs" : "",
  chatIdleTimeoutSeconds: 0,
};

// #runpanel-default-agentry-signedout: signed out, where Go offers no run In
// Agentry at all and says the app is not signed in.
const signedOut = hostedState === "default-agentry-signedout";

const availability = {
  // An interactive-only release is not offered the autonomous run In Agentry.
  offered: !signedOut && !sessionShape, available: hostedState !== "unavailable" && !signedOut && !sessionShape,
  reason: hostedState === "unavailable"
    ? "The token stored for this registry does not carry run:read and run:write, which a run In Agentry needs. It was minted before this app asked for them."
    : "",
  remedy: hostedState === "unavailable" ? "Sign in again from Settings to mint a token that carries them." : "",
  signedIn: !signedOut, extension: true, enabled: true, runnersOnline: 1, queued: 2,
  tier: "standard", holdCredits: 20,
  caps: { timeoutSeconds: 300, maxTurns: 20 },
  balance: sessionShape ? 460 : 46, balanceKnown: true,
  maxInputFiles: 20, maxInputBytes: 10 * 1024 * 1024,
  knowledgeAttach: hostedKnowledge, maxPacks: hostedKnowledge ? 5 : 0,
  declaredSecrets: hostedSecrets ? ["PAGERDUTY_TOKEN"] : [], vault: true,
  publisherEarns: hostedEarns,
  // Talk to it In Agentry: offered on the sessions' own terms; a Standard
  // payer is offered it unavailable, with the way up.
  session: sessionShape
    ? {
      offered: true, available: view !== "runpanel-session-tier" && view !== "runpanel-session-nohost",
      reason: view === "runpanel-session-tier" ? "Talking to an agent In Agentry is part of Pro, Business and Enterprise."
        : view === "runpanel-session-nohost" ? "No sandbox host is answering right now, so a session In Agentry cannot start." : "",
      remedy: view === "runpanel-session-tier" ? "On Standard, talk to it on this computer, for no credits; or upgrade to Pro."
        : view === "runpanel-session-nohost" ? "Try again in a few minutes, or talk to it on this computer." : "",
      upgrade: view === "runpanel-session-tier",
      freeMinutes: 10, creditsPerMinute: 50, minCredits: 50, idleSeconds: 300,
      transport: "Transport encrypted by the user key, messages not stored; the transcript comes back only in your sealed output.",
    }
    : undefined,
};

const hostedBase = {
  id: "0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42", ref: "demo1/incident-summarizer@1.2.0",
  digest: "sha256:9c356fd0d20fef07", holdCredits: 20, caps: availability.caps,
  outputs: [], turns: 0, wallMs: 0,
  settledCredits: 0, settled: false, cancelRequested: false, final: false,
  createdAt: "2026-09-16T10:04:00Z",
};

const hostedRuns: Record<string, any> = {
  queued: { ...hostedBase, status: "queued", state: "queued", statusLine: "queued", queuePosition: 2 },
  running: { ...hostedBase, status: "running", state: "running", statusLine: "running" },
  finished: {
    ...hostedBase, status: "finished", state: "done — pass", statusLine: "ran to the end and wrote its result",
    outcome: "succeeded", outcomeTone: "ok", outcomeLine: "ran to the end and wrote its result",
    exitClass: "ok", attestationState: "verified", adapter: "claude-code",
    turns: 4, wallMs: 41200,
    settledCredits: 7, settled: true, final: true, receiptKept: true,
    capabilitiesEnforced: {
      "network.egress": "llm-proxy allowlist: api.pagerduty.com",
      "filesystem.read": "workspace bind mount, read-only /inputs",
      "filesystem.write": "reports/ bind mount, run-scoped",
      "mcp.gateway": "per-run gateway serving the declared contract only",
    },
    resultJson: JSON.stringify("INC-4821 began at 02:14 UTC with elevated 5xx on the checkout service. Two API tokens found in the thread were redacted before quoting. Draft report written to reports/report.md."),
    outDir: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-0199b2c4",
    outputs: [
      { name: "result.json", bytes: 214, path: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-0199b2c4/result.json" },
      { name: "reports/report.md", bytes: 3482, path: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-0199b2c4/reports/report.md" },
      { name: "receipts/run.json", bytes: 1944, path: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-0199b2c4/receipts/run.json" },
    ],
  },
};
// A session In Agentry: running twelve minutes in, two paid minutes pulled,
// and over with its tokens settled.
const sessionStarted = new Date(Date.now() - (12 * 60 + 40) * 1000).toISOString();
hostedRuns.session = {
  ...hostedRuns.running, mode: "interactive", holdCredits: 0, task: "",
  startedAt: sessionStarted,
  session: { freeMinutes: 10, creditsPerMinute: 50, paidThrough: 12, timeCredits: 100, tokenCredits: 0, tokensUnpaid: 0, outOfCredits: false, filesAdded: 1, idleSeconds: 300 },
};
const hostedRun = hostedRuns[hostedState] ?? null;

const listeners: Record<string, ((v: any) => void)[]> = {};

// What the result view lists from the run folder (RunAPI.RunOutputs).
const runOutputs = {
  files: [
    { path: "outputs/summary.csv", bytes: 64, previewKind: "text", preview: "incident,started,severity\nINC-4821,02:14,high\n" },
    { path: "reports/report.md", bytes: 3482, previewKind: "markdown", preview: "## INC-4821\n\nElevated 5xx on **checkout** from 02:14 UTC.\n\n| step | when |\n|---|---|\n| detected | 02:14 |\n| mitigated | 02:41 |\n" },
  ],
  skipped: 0,
};

// Which step each state opens on: the blockers and Run are on the review,
// the key form on Connect tools, the rest where the view says.
const STEP_OF: Record<string, StepKey> = {
  "runpanel-keys": "review",
  "runpanel-keys-saved": "tools",
  "runpanel-file-intake": "review",
  "runpanel-unscanned": "review",
  "runpanel-local-install": "review",
  "runpanel-step-files": "files",
  "runpanel-step-tools": "tools",
  "runpanel-step-details": "details",
  "runpanel-step-review": "review",
  "runpanel-voice": "review",
  "runpanel-step-review-long": "review",
  "runpanel-schedule": "review",
  "runpanel-schedule-gaps": "review",
  "runpanel-schedule-windows": "review",
  "runpanel-chat-review": "review",
  "runpanel-guided-files": "files",
  "runpanel-guided-tools": "tools",
  "runpanel-guided-details": "details",
  "runpanel-guided-hostile-tools": "tools",
  "runpanel-local-finished": "review",
  "runpanel-step-knowledge": "knowledge",
  "runpanel-step-knowledge-ships": "knowledge",
  "runpanel-knowledge-finished": "review",
  "runpanel-hosted-knowledge": "knowledge",
  "runpanel-hosted-secrets": "review",
  "runpanel-hosted-followed": "review",
  "runpanel-local-finished-env": "review",
  "runpanel-local-finished-files": "review",
  "runpanel-local-finished-files-nokey": "review",
  "runpanel-default-folder": "review",
  "runpanel-unsafe-review": "review",
  "runpanel-unsafe-finished": "review",
  "runpanel-signin": "tools",
  "runpanel-signin-connecting": "tools",
  "runpanel-signin-own-client": "tools",
  "runpanel-signin-connected": "tools",
  "runpanel-signin-review": "review",
  "runpanel-signin-hosted": "tools",
  "runpanel-signin-hosted-connect": "review",
};

const g = window as any;
g.go = { main: {
  SettingsAPI: { RunDefaults: async () => runDefaults },
  // Schedules on this computer (agentry-desktop#743): this computer can
  // schedule; on the Windows state it cannot also run signed out.
  ScheduleAPI: {
    Availability: async () => ({
      available: true, zone: "Europe/London", lingerOn: false,
      signedOut: view === "runpanel-schedule-windows"
        ? { available: false, reason: "--signed-out is not available on windows: Docker Desktop stops when you sign out" }
        : { available: true },
    }),
    Check: async () => (view === "runpanel-schedule-gaps"
      ? { ready: false, gaps: ["no model key is kept in the keychain: run `agentry setup`"], installed: true, modes: ["autonomous"], refusals: [], needsConsent: false, signedOut: { available: true } }
      : { ready: true, gaps: [], installed: true, modes: ["autonomous"], refusals: [], needsConsent: false, signedOut: { available: true }, describe: "every day at 15:00 (Europe/London)", next: "2026-10-09T14:00:00Z" }),
    Add: async () => ({ id: "ab12cd34ef" }),
  },
  RunAPI: {
    SetupStatus: async () => setup, GatewayRequirements: async () => mcp, RunRequirements: async () => mcp,
    ProfileStatus: async () => profile,
    // Saving the keys makes the profile exist: the line above Run goes and
    // Run enables (#runpanel-keys-saved, agentry-desktop#167).
    SaveProfile: async () => ({ ...profile, exists: true, storedSecretNames: profile.declaredSecrets }),
    HostedAvailability: async () => availability,
    AttachHostedRun: async () => ({ run: hostedRun }),
    AttachHostedRunAtStart: async () => ({ run: view === "runpanel-hosted-followed" ? hostedRuns.running : undefined }),
    ReplayHostedSession: async () => {},
    StartHostedSession: async () => ({ run: hostedRuns.session }),
    SendHostedTurn: async () => {}, AddHostedSessionFile: async () => {}, EndHostedSession: async () => {},
    StartHostedRun: async () => (hostedSecrets && hostedStarts++ === 0
      ? { refusal: "secret-missing", detail: "A declared secret is missing: The release declares secrets you have not set: PAGERDUTY_TOKEN.", missing: ["PAGERDUTY_TOKEN"] }
      : view === "runpanel-signin-hosted-connect"
        ? { refusal: "connection-missing", detail: `A tool is not connected: The release's tools sign in by OAuth, and you have not connected ${gmailServer} for this version. Connect them, then start the run.`, missing: [gmailServer] }
        : { run: hostedRuns.queued }),
    // A run In Agentry's sign-ins (agentry-desktop#667): the page opens in
    // the browser, which never answers here, so the start waits.
    OpenHostedConnect: async () => {},
    AwaitHostedConnections: () => new Promise(() => {}),
    StopAwaitingConnections: async () => {},
    DiscardHostedConnect: async () => {},
    SetHostedSecret: async () => {},
    DiscardHostedSecrets: async () => {},
    StarAsk: async () => ({ ask: false }),
    StarQuickAgent: async () => {},
    CancelHostedRun: async () => ({ ...hostedRuns.queued, cancelRequested: true, statusLine: "cancelling" }),
    // A run on this computer is handed to the run's flow at Run
    // (agentry-desktop#465): Go's first report says it has begun, then its
    // result comes back, and the flow shows the result the panel used to.
    RunAgent: async () => {
      const begun = { runId: localResult.runId, ref: localResult.ref, startedAt: Date.now() - 41_200, timeoutSeconds: 900, maxTurns: 60, turns: 4, inputTokens: 5120, outputTokens: 1430, prepared: true };
      for (const cb of listeners["runner:run-progress"] ?? []) cb(begun);
      return localResult;
    },
    ForgetRun: async () => {},
    UnsafeNotice: async () => unsafeNotice,
    RunOutputs: async () => runOutputs,
    ShowRunFolder: async () => {},
    // Connect tools' sign-ins (agentry-desktop#666): none for the other
    // states; on #runpanel-signin-connecting Connect sends the consent
    // address and waits for the browser, which never answers here.
    ReleaseConnections: async () => (signInShape ? signInRelease : { ref: profile.ref, digest: profile.digest, servers: [] }),
    Connect: async (_reg: string, _ref: string, server: string) => {
      if (view === "runpanel-signin-own-client") return { needsOwnClient: googleGuide };
      const url = "https://accounts.google.com/o/oauth2/v2/auth?response_type=code&client_id=123-abc.apps.googleusercontent.com&redirect_uri=http%3A%2F%2F127.0.0.1%3A41817%2Fcallback&state=Q2hlY2s&code_challenge=E9Melhoa2Owv&code_challenge_method=S256&scope=https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fgmail.readonly";
      for (const cb of listeners["connect:say"] ?? []) cb({ server, line: "The sign-in is at https://accounts.google.com (the authorization server " + server + " names), as your own OAuth client 123-abc.apps.googleusercontent.com, asking for: https://www.googleapis.com/auth/gmail.readonly." });
      for (const cb of listeners["connect:url"] ?? []) cb({ server, url });
      return new Promise(() => {});
    },
    PasteRedirect: async () => {}, CancelConnect: async () => true, KeepOwnClient: async () => {},
    Disconnect: async () => "revoked",
  },
  InstallAPI: {
    // #install-unscanned: the plan the wizard shows for a release the
    // registry admitted without a scan.
    Prepare: async () => ({
      id: "plan-1", registry: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: "1.2.0",
      modes: ["autonomous"], summary: "Summarises an incident channel into a report card, redacting secrets first.",
      digest: "sha256:9c356fd0d20fef07123ba7c4a1b2c3d4e5f6a7b8", yanked: false,
      attestation: att, unsigned: false, unscanned: view === "install-unscanned",
      capabilities: view === "install-ships-knowledge"
        ? { egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsWrite: "paths", fsWritePaths: ["reports"], secrets: ["PAGERDUTY_TOKEN"], knowledge: ["pagerduty-docs"] }
        : { network: { egress: ["api.pagerduty.com"] }, filesystem: { read: "workspace", write: { paths: ["reports"] } }, secrets: ["PAGERDUTY_TOKEN"] },
      shippedKnowledge: view === "install-ships-knowledge" ? [{ name: "pagerduty-docs", bytes: 4_404_019 }] : [],
      // Every declared secret, an optional one marked (agentry-desktop#655).
      declaredSecrets: ["PAGERDUTY_TOKEN", "STATUSPAGE_TOKEN"], optionalSecrets: ["STATUSPAGE_TOKEN"],
      needsCapabilityConfirm: true, prevVersion: "", diff: null, requiresWidenedConfirm: false,
      vulnerabilities: [], vulnerabilityLine: "have no known high or critical advisories", vulnerabilityRefusal: "",
      target: "agents-md", paths: { dest: "/home/chris/agents/incident-summarizer", managed: true, managedRoot: "/home/chris/agents", agentsMd: "/home/chris/agents/AGENTS.md" },
      mcpServers: view === "install-mcp-env" ? mcpEnvServers : [],
    }),
    Confirm: async () => ({ dir: "/home/chris/agents/incident-summarizer" }),
    Cancel: async () => {},
  },
  // The packs kept on this computer, for the Knowledge step; none unless
  // the state shows one.
  KnowledgeAPI: {
    List: async () => ({ packs: knowledgePacks, registry: "https://secureagentry.net", signedIn: true }),
  },
  // This computer's Writing Voice (agentry-desktop#510): on for every run
  // here on #runpanel-voice, so Review and start shows Write in my voice
  // ticked; none on every other state, which shows no box.
  VoiceAPI: {
    View: async () => ({
      registry: "https://secureagentry.net", signedIn: false, accountOffered: false, model: "",
      local: window.location.hash === "#runpanel-voice"
        ? { exists: true, text: "# Voice\n\nShort sentences.\n", digest: "sha256:4fa1", bytes: 28, source: "edit", phrase: "written by hand", alwaysAttach: true, created: "", updated: "" }
        : { exists: false },
    }),
  },
  App: { OpenExternal: async () => {}, ChooseFiles: async () => [] },
} };
g.go.main.SettingsAPI.OpenUpgrade = async () => {};
// The app's EventsOn goes through EventsOnMultiple (the wails runtime
// shim). A run In Agentry sends no log (agentry-desktop#182), so nothing
// is scripted on runner:run-log.
const subscribe = (name: string, cb: (v: any) => void) => {
  (listeners[name] ??= []).push(cb);
  return () => { listeners[name] = (listeners[name] ?? []).filter((c) => c !== cb); };
};
g.runtime = {
  EventsOn: subscribe,
  EventsOnMultiple: (name: string, cb: (v: any) => void) => subscribe(name, cb),
  EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import { useState } from "react";
import RunPanel from "../src/pages/RunPanel";
import RunFlow from "../src/pages/RunFlow";
import HostedRunFlow from "../src/pages/HostedRunFlow";
import { attachHostedRun } from "../src/runs";
import type { StepKey } from "../src/runSetup";
import InstallWizard from "../src/pages/InstallWizard";
import ToolsPanel from "../src/pages/ToolsPanel";
import { Wordmark } from "../src/ui";
import { SidebarNav } from "../src/nav";
import { UnsafeBadge, UnsafeGlow } from "../src/unsafe";

// #runpanel-local-install (agentry-desktop#425): a QuickAgent installed
// from a folder on this computer, run from the Library.
const localInstall = view === "runpanel-local-install";
const att = (localInstall ? { state: "local", detail: "installed from a folder on this computer" }
  : hostedState === "unscanned" || hostedState === "default-agentry-unscanned" || view === "install-unscanned"
  ? { state: "unscanned", detail: "the registry admitted this version without a scan: no findings, no report and no attestation stand behind it" }
  : { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified" }) as any;

function Backdrop() {
  return (
    <div className="shell" style={{ height: "100vh" }}>
      <div className="sidebar"><div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="browse" />
        {unsafeShape && <div className="foot"><UnsafeBadge /><div>v0.6.28</div></div>}
      </div>
      <div className="content" />
      {unsafeShape && <UnsafeGlow />}
    </div>
  );
}

function ChatSample() {
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar"><div className="brand"><Wordmark sub="Desktop" /></div></div>
      <div className="content" style={{ maxWidth: 720 }}>
        <h3>Session</h3>
        <div className="chat-pane">
          <div className="chat-msg assistant"><div className="chat-bubble"><div className="chat-who">Agent</div>What incident should I look at?</div></div>
          <div className="chat-msg user"><div className="chat-bubble"><div className="chat-who">You</div>Summarise INC-4821 from the last 24 hours.</div></div>
          <div className="chat-note">· tool: read_input</div>
          <div className="chat-msg assistant"><div className="chat-bubble"><div className="chat-who">Agent</div>INC-4821 began at 02:14 UTC with elevated 5xx on the checkout service. I redacted two API tokens found in the thread before quoting it. Draft report written to reports/report.md.</div></div>
          <div className="chat-note warn">Session ended: finished</div>
        </div>
        <p className="muted small" style={{ margin: "6px 0" }}>demo1/incident-summarizer@1.2.0 · interactive · 1 turn · 1 tool call · $0.0182 · ended (finished)</p>
      </div>
    </div>
  );
}

const el = document.getElementById("root")!;
if (view === "install-unscanned" || view === "install-mcp-env" || view === "install-ships-knowledge") {
  createRoot(el).render(<><Backdrop /><InstallWizard registry="https://secureagentry.net" agentRef="demo1/incident-summarizer@1.2.0" onClose={() => {}} /></>);
} else if (view === "chat") {
  createRoot(el).render(<ChatSample />);
} else if (view === "toolspanel" || view === "toolspanel-hostile") {
  createRoot(el).render(<><Backdrop /><ToolsPanel registry="https://secureagentry.net" ns="demo1" name="incident-summarizer" version="1.2.0" onClose={() => {}} /></>);
} else {
  // The setup closes once a run on this computer has begun, as the pages'
  // own do; the run's flow, mounted as App mounts it, shows the rest.
  // App attaches a run In Agentry already in flight as it starts.
  if (view === "runpanel-hosted-followed") void attachHostedRun("https://secureagentry.net");
  const Panel = (props: React.ComponentProps<typeof RunPanel>) => {
    const [open, setOpen] = useState(true);
    return open ? <RunPanel {...props} onClose={() => setOpen(false)} /> : null;
  };
  createRoot(el).render(
    <><Backdrop />
      <RunFlow onBackToSetup={() => {}} />
      <HostedRunFlow onBackToSetup={() => {}} />
      <Panel
        registry="https://secureagentry.net"
        ns={keysShape ? "agentry" : "demo1"} name={keysShape ? "pr-review-brief" : "incident-summarizer"} version={keysShape ? "1.2.1" : "1.2.0"}
        attestation={att} onClose={() => {}} local={localInstall}
        summary={keysShape
          ? "Turns one GitHub pull request into the reviewer's brief: what changed, the risks, what to test and questions for the author."
          : "Summarises an incident channel into a report card, redacting secrets first."}
        initialMode={hostedState === "hosted" || hostedState === "hosted-followed" || hostedState === "unavailable" || hostedKnowledge || hostedSecrets || hostedEarns || hostedSignIn ? "hosted"
          : view === "runpanel-session-offer" ? "hosted-session" : view === "runpanel-chat-review" ? "interactive" : undefined}
        initialStep={STEP_OF[view]}
        initialWhen={view.startsWith("runpanel-schedule") ? "repeat" : undefined}
      />
    </>,
  );
}
