// Page previews (agentry-desktop#37+). Mocks the Wails bindings
// (window.go / window.runtime) with fixture data so the REAL page
// components render in a plain browser for screenshots — the bindings
// resolve through window.go at call time, so setting it before render is
// enough. Not shipped (outside tsconfig's include and the app's build).
import { useState } from "react";
import { createRoot } from "react-dom/client";
import "../src/style.css";
import { Wordmark } from "../src/ui";

const hits = [
  { namespace: "demo1", name: "incident-summarizer", summary: "Summarises an incident channel into a report card, redacting secrets first.", modes: ["autonomous"], quarantine: "", installs30d: 42 },
  { namespace: "acme", name: "pr-triage", summary: "Reads an open PR and drafts a triage note under a strict egress allowlist.", modes: ["autonomous", "interactive"], quarantine: "", installs30d: 7 },
  { namespace: "labs", name: "log-explainer", summary: "An interactive session over a log bundle; no filesystem write, no network.", modes: ["interactive"], quarantine: "", installs30d: 0 },
];
// #agent-unscanned (M23): the latest version was admitted without a scan
// — its own verdict, its own mark, no attestation to verify.
const unscanned = window.location.hash === "#agent-unscanned";
const detail = {
  agent: { namespace: "demo1", name: "incident-summarizer", summary: "Summarises an incident channel into a report card, redacting secrets first.", modes: ["autonomous"], quarantine: "" },
  latest: "1.2.0", latestDigest: "sha256:9c356fd0d20fef07123ba7c4a1b2c3d4e5f6a7b8",
  latestCapabilities: { network: { egress: ["api.pagerduty.com"] }, filesystem: { read: "workspace", write: { paths: ["reports"] } }, secrets: ["PAGERDUTY_TOKEN"] },
  dependencyAlerts: [],
  latestScanStatus: unscanned ? "skipped" : "pipeline",
  versions: [
    { version: "1.2.0", channel: "stable", status: "published", publishedAt: "2026-09-01T10:00:00Z", createdAt: "2026-09-01T10:00:00Z", archiveDigest: "sha256:9c356fd0", yanked: false, scanStatus: unscanned ? "skipped" : "pipeline" },
    { version: "1.1.0", channel: "stable", status: "published", publishedAt: "2026-08-20T10:00:00Z", createdAt: "2026-08-20T10:00:00Z", archiveDigest: "sha256:d20fef07", yanked: false },
  ],
};
const report = {
  findingCounts: { medium: 1, low: 2 }, versionStatus: "published", rulesetVersion: "2026.09.1",
  finishedAt: "2026-09-01T10:04:00Z", judgeModel: "claude-sonnet-4-5",
  findings: [
    { severity: "medium", scanner: "S4", title: "Prompt-injection signature in a bundled sample", filePath: "samples/incident.md", lineStart: 12, excerpt: "Ignore previous instructions and…", verdict: "open", verdictReason: "" },
    { severity: "low", scanner: "S1", title: "Large vendored file", filePath: "vendor/data.bin", lineStart: 0, excerpt: "", verdict: "open", verdictReason: "" },
  ],
};
const prov = unscanned
  ? { state: "unscanned", detail: "the registry admitted this version without a scan: no findings, no report and no attestation stand behind it" }
  : { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified", rulesetVersion: "2026.09.1", judgeModel: "claude-sonnet-4-5" };
// The Installed page's three sections (agentry-desktop#63): two installs this
// app manages, a run consent the CLI wrote into the same file, and a record
// whose directory is gone.
const installedRecs = [
  { registry: "https://secureagentry.net", origin: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: "1.2.0", target: "agents-md", dir: "/home/me/work/oncall", managed: true, installedAt: "2026-09-02T09:00:00Z", capabilities: { egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsWrite: "paths", secrets: ["PAGERDUTY_TOKEN"], execCommands: [] }, present: true, manageable: true },
  { registry: "https://secureagentry.net", origin: "https://secureagentry.net", namespace: "acme", name: "pr-triage", version: "0.4.1", target: "claude-skill", dir: "/home/me/work/acme", managed: true, installedAt: "2026-08-30T14:00:00Z", capabilities: { egressHosts: ["api.github.com"], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: ["gh"] }, present: true, manageable: true },
  { registry: "runner", origin: "https://secureagentry.net", namespace: "agentry", name: "changelog-from-git", version: "1.2.1", target: "runner", dir: "/home/me/.agentry/runner/agents/agentry/changelog-from-git", managed: true, installedAt: "2026-09-11T16:20:00Z", capabilities: { egressHosts: [], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: ["git"] }, present: true, manageable: true },
  { registry: "https://secureagentry.net", origin: "https://secureagentry.net", namespace: "acme", name: "release-notes", version: "0.2.0", target: "claude-skill", dir: "/home/me/old-project", managed: true, installedAt: "2026-07-14T10:00:00Z", capabilities: { egressHosts: [], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: [] }, present: false, manageable: true },
];
// pages.html#fresh previews a first run: nothing installed, and a registry
// that sent no total (a core before agentry#290) gets a list with no count.
const fresh = window.location.hash === "#fresh";
// The agent page's one-click Install (agentry-desktop#146, D-031):
// #agent-installed has the runner-home record for the shown QuickAgent at
// its latest (Update, disabled); #agent-installed-older at an older
// version (Update to <latest>); #agent-install-refused makes the one-click
// call fail the way a declined gate does; #agent-installing never answers,
// so the button reads Installing….
const agentInstalled = window.location.hash.startsWith("#agent-installed");
const agentInstalledOlder = window.location.hash === "#agent-installed-older";
// #agent-installed-unreachable: the record's registry cannot be asked, so
// Update stays disabled and the page says why (agentry-desktop#147).
const agentInstalledUnreachable = window.location.hash === "#agent-installed-unreachable";
// #installed-updates: the launch check found two newer versions — the
// number on Installed in the sidebar (agentry-desktop#147).
const installedUpdates = window.location.hash === "#installed-updates" ? 2 : 0;
// #installed-update-warning: the runner-home record has a newer version,
// and its Update completes with a warning — the policy it could not read
// (agentry-desktop#235); the page shows the engine's sentence.
const installedUpdateWarning = window.location.hash === "#installed-update-warning";
// Set once that Update answers, so the list reads back the record at 1.3.0
// with nothing newer, as the real refresh would.
let updateApplied = false;
const agentInstallRefused = window.location.hash === "#agent-install-refused";
const agentInstalling = window.location.hash === "#agent-installing";
// Settings → Coding agents (agentry-desktop#148): the hash picks the
// coding agent the policy records. #coding-codex has Codex alone, so the
// export is AGENTS.md in the chosen project folder, and the switch on;
// #coding-none has nothing chosen and no folder, so the switch cannot go
// on; anything else has Claude Code, the Claude skill, the switch off —
// which is also what names the agent page's one-off link.
const codingState = (window.location.hash.match(/^#coding-(\w+)/) || [])[1] || "claude";
const claudeCode = { name: "claude", displayName: "Claude Code", executable: "claude", installUrl: "https://claude.ai/code", found: true, path: "/home/me/.local/bin/claude", pathFromPolicy: true, version: "2.1.263 (Claude Code)", versionMatches: true, chosen: true, mcpCommand: "claude mcp add agentry -- node /path/to/agentry-mcp/dist/index.js" };
const codex = { name: "codex", displayName: "Codex", executable: "codex", installUrl: "https://github.com/openai/codex", found: true, path: "/usr/local/bin/codex", pathFromPolicy: true, version: "0.42.0", versionMatches: true, chosen: true, mcpCommand: "codex mcp add agentry -- node /path/to/agentry-mcp/dist/index.js" };
// #coding-both has both chosen: only the Claude skill is written, so the
// switch and the link name Claude Code alone.
const codingChosen = codingState === "codex" ? [codex] : codingState === "none" ? [] : codingState === "both" ? [claudeCode, codex] : [claudeCode];
let codingSwitch = codingState === "codex";
const codingView = () => ({
  chosen: codingChosen,
  export: codingState === "codex"
    ? { target: "agents-md", dir: "/home/me/work/oncall", needsDir: false, enabled: codingSwitch }
    : codingState === "none"
      ? { target: "agents-md", dir: "", needsDir: true, enabled: false }
      : { target: "claude-skill", dir: "/home/me/.claude/skills", needsDir: false, enabled: codingSwitch },
});
const runnerRecForShown = {
  registry: "runner", origin: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: agentInstalledOlder ? "1.1.0" : "1.2.0",
  target: "runner", dir: "/home/me/.agentry/runner/agents/demo1/incident-summarizer", managed: true, installedAt: "2026-09-21T12:00:00Z",
  capabilities: { egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsWrite: "paths", secrets: ["PAGERDUTY_TOKEN"], execCommands: [] }, present: true, manageable: true,
};


const buildModel = {
  name: "incident-summarizer", namespace: "demo1", modes: ["autonomous"], version: "1.2.0",
  license: "Apache-2.0", summary: "Summarises an incident channel into a report card, redacting secrets first.",
  tags: ["oncall", "reporting"], instructionFile: "AGENT.md",
  egressMode: "hosts", egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsReadPaths: [],
  fsWrite: "paths", fsWritePaths: ["reports"], execCommands: [], secrets: ["PAGERDUTY_TOKEN"],
  mcpServers: [{ source: "npm", package: "@cyanheads/osv-advisory-mcp-server", version: "0.1.13", tools: [], secrets: [], egress: ["api.osv.dev"], env: [{ name: "LOG_LEVEL", value: "info" }] }],
  dataAccess: ["tickets"], intake: [], opening: "", idleTimeoutSeconds: 0,
  guide: { overview: "Reads the incident channel and returns a one-page report card.", prepare: ["The incident number", "Read access to the channel"], files: "", task: "Summarise INC-1234", connect: [{ secret: "PAGERDUTY_TOKEN", help: window.location.hash.endsWith("-hostile") && window.location.hash.startsWith("#build-") && !window.location.hash.startsWith("#build-drafted")
    // agentry-desktop#343: a key's help is markdown (DEV-167). A hostile one
    // on Before a run's preview and on Review shows as text there.
    ? "1. Open **Integrations → API Access Keys**.\n2. Create a *read-only* key.\n\n<b onclick=\"steal()\">now</b> ![pixel](https://evil.example/p.gif) [open me](javascript:alert(1)) [PagerDuty](https://evil.example/pd) https://support.pagerduty.com/keys\u202E and https://p\u0430gerduty.com/new <https://pagerduty.com%2Eevil.example/>"
    : "Create a read-only API key under Integrations" }] },
  authors: [], runtimeModels: [], runtimeInterfaces: ["claude-skill", "agents-md"], dependencies: [], publishChannel: "", publishInclude: [],
  hadComments: window.location.hash === "#build-comments",
  instructions: "You summarise an incident channel. Read it, redact secrets, and write reports/report.md.",
};
// BuildAPI.Check's answer for the model above (agentry-desktop#280): every
// step passes, so each shows the lines it writes and Review the file.
const buildCheck = {
  yaml: 'schemaVersion: "2"\nname: incident-summarizer\nnamespace: demo1\nmodes: [autonomous]\nversion: 1.2.0\nsummary: Summarises an incident channel into a report card, redacting secrets first.\nlicense: Apache-2.0\n',
  fragments: {
    basics: "name: incident-summarizer\nmodes: [autonomous]\nsummary: Summarises an incident channel into a report card, redacting secrets first.",
    instructions: "entrypoints:\n  instruction: AGENT.md",
    tools: "capabilities:\n  network:\n    egress: [api.pagerduty.com]\n  mcpServers:\n    - npm: '@cyanheads/osv-advisory-mcp-server'\n      version: 0.1.13\n      egress: [api.osv.dev]\n      env:\n        LOG_LEVEL: info",
    access: "capabilities:\n  filesystem:\n    read: workspace\n    write: { paths: [reports] }\n  exec: none\n  secrets: [PAGERDUTY_TOKEN]\n  dataAccess: [tickets]",
    guide: "guide:\n  overview: Reads the incident channel and returns a one-page report card.\n  prepare: [The incident number, Read access to the channel]\n  task: Summarise INC-1234\n  connect:\n    PAGERDUTY_TOKEN: Create a read-only API key under Integrations",
    publish: "namespace: demo1\nversion: 1.2.0\nlicense: Apache-2.0\ntags: [oncall, reporting]",
  },
  problems: {}, first: null, warnings: [],
};
// pages.html#build-* opens the Build preview on a view and a step.
const buildHash = (window.location.hash.match(/^#build-([\w-]+)/) || [])[1] || "";
const buildView = buildHash === "" ? "start" : buildHash === "advanced" || buildHash === "comments" ? "advanced" : "steps";
const buildStep = ({ instructions: 1, tools: 2, access: 3, questions: 4, guide: 5, "guide-hostile": 5, publish: 6, review: 7, "review-hostile": 7, validate: 7, drafted: 7, "drafted-hostile": 7, "drafted-agentry": 7 } as Record<string, number>)[buildHash] ?? 0;
// Describe it (#282): #build-describe with the person's AI Review key saved,
// #build-describe-nokey without one; #build-drafted opens Review with a
// draft the Drafter made and its notes, and #build-drafted-hostile with
// notes, a match, a source and a withheld address that carry markup, an
// image, a javascript: link and a right-to-left override.
const buildDescribe = buildHash.startsWith("describe");
const hostileDraft = buildHash === "drafted-hostile";
// Describe it In Agentry (#281): #build-describe-agentry offers it with the
// hold, the caps and the balance; #build-describe-agentry-short is a payer
// short of the hold; #build-describe-agentry-drafting follows one queued;
// #build-describe-signedout offers it but asks for an account;
// #build-drafted-agentry opens Review on a draft made In Agentry, with
// Describe again under the notes.
const describeAgentry = buildHash.startsWith("describe-agentry");
const describeSignedOut = buildHash === "describe-signedout";
const agentryDraft = buildHash === "drafted-agentry";
const describeHosted = describeSignedOut
  ? { offered: true, available: false, signedIn: false, reason: "Drafting In Agentry spends credits, so it needs an account.", remedy: "Sign in from Settings.",
      holdCredits: 0, inputsSentence: "", timeoutSeconds: 0, maxTurns: 0, maxPages: 0, balance: 0, balanceKnown: false, unmetered: false, short: false }
  : { offered: true, available: true, signedIn: true, holdCredits: 41,
      holdSentence: "This draft holds 41 credits and settles from what it uses: the tokens its model reads and writes, up to 10 web searches at 1 credit each, and 1 credit for AI Review, never more than it holds.",
      inputsSentence: "Your description and the Drafter's searches go to a search provider, to AI Review (TypeSafe's Jev) and to the sites the Drafter reads, so the description should hold no secret. AI Review reads every result before the Drafter's model does, and holds back what it flags.",
      timeoutSeconds: 1200, maxTurns: 60, maxPages: 40, balance: buildHash === "describe-agentry-short" ? 12 : 496, balanceKnown: true, unmetered: false,
      short: buildHash === "describe-agentry-short" };
const hostedDrafting = buildHash === "describe-agentry-drafting"
  ? { id: "01a0e424-0000-0000-0000-000000000000", status: "queued", state: "queued", statusLine: "waiting for a runner — number 1 in the queue",
      queuePosition: 1, cancelRequested: false, holdCredits: 41, creditsLine: "41 credits are held for this draft.", final: false }
  : null;
const draftedView = buildHash.startsWith("drafted") ? {
  where: agentryDraft ? "agentry" : "here", description: "Check a CSV file I add for missing values, duplicate rows and mixed formats.", hints: [],
  unmentioned: agentryDraft ? ["api.csvlint.io"] : [], moreProblems: 0,
  problems: agentryDraft ? ["V14 guide.prepare: 13 prepare items; the maximum is 10"] : [],
  model: { ...buildModel, name: "csv-quality-check", namespace: "demo1", version: "0.1.0",
    summary: "Checks a CSV you add for missing values, duplicate rows and mixed formats, per column.",
    egressMode: "none", egressHosts: [], fsRead: "none", fsWrite: "none", fsWritePaths: [], secrets: [], mcpServers: [], dataAccess: ["files"],
    intake: [{ id: "file", prompt: "The CSV file to check", type: "file", required: true, choices: [] }],
    guide: { overview: "Reads the CSV you add and reports its problems per column.", prepare: ["The CSV file"], files: "One CSV file.", task: "Check sales.csv", connect: [] },
    instructions: "# CSV quality check\n\nRead the CSV under /inputs. For each column, report missing values, duplicate rows and values whose format differs from the rest.\n",
    hadComments: false },
  decodeNotes: hostileDraft ? ["The form cannot hold these fields of the draft, so they were left out: x-run-this."] : [],
  notes: hostileDraft
    ? "## Notes\n\n<script>alert(1)</script><b onclick=\"steal()\">click</b> ![logo](https://evil.example/pixel.png) [open](javascript:alert(1)) and \u202eevil.exe\n"
    : "## How I read the task\n\nA local check of one CSV file: no network, no command, no key. The registry already has **agentry/csv-data-quality-check**, which does more than asked.\n\n## What I could not settle\n\n- Whether dates should be ISO 8601.\n",
  matches: [{ ref: hostileDraft ? "javascript:alert(1)" : "agentry/csv-data-quality-check@1.2.1", why: hostileDraft ? "<img src=x onerror=alert(1)> does it" : "Profiles a CSV for nulls, duplicates and format violations, with the same contract." }],
  sources: [{ url: hostileDraft ? "javascript:alert(document.cookie)" : "https://www.rfc-editor.org/rfc/rfc4180", used: "the CSV format", withheld: false },
    ...(hostileDraft ? [{ url: "https://evil.example/inject", used: "", withheld: true }] : [])],
  openQuestions: ["Should dates be ISO 8601?", "One file per run, or several?"],
  aiReview: { scan: agentryDraft ? "registry" : "jev", read: hostileDraft ? 7 : 6, passed: hostileDraft ? 6 : 6, withheld: hostileDraft ? 1 : 0, notScanned: 0,
    withheldSources: hostileDraft ? [{ tool: "web_read", url: "https://evil.example/inject\u202e", reason: "AI Review flagged it (injection)" }] : [] },
  manifestError: "", drafterModel: "openai/qwen3.8-flash-next", runId: "0192a3", outDir: "/home/me/Downloads/agentry-out/agentry-drafter-0192a3",
} : null;
// Settings → Provider settings → AI Review key: saved by default; #aireview-none has none;
// #aireview-refused answers Test key with TypeSafe's 401.
const jevKeyView = window.location.hash === "#aireview-none" || buildHash === "describe-nokey"
  ? { stored: false, envName: "TYPESAFE_API_KEY", envSet: false, ready: false }
  : { stored: true, source: "keyring", envName: "TYPESAFE_API_KEY", envSet: false, ready: true };
const validateResult = {
  passed: true, errorCount: 0, warnCount: 0, inventoryFailing: 0, fileCount: 6,
  bundleVersion: "2026.09.1", bundleSource: "embedded", manifestName: "demo1/incident-summarizer",
  manifestModes: ["autonomous"], manifestVersion: "1.2.0", manifestError: "",
  issues: [], inventory: [], findings: [], readErrors: [],
};

// Settings → Run defaults (agentry-desktop#347): the policy's budgets,
// runs on this computer, no folder saved.
let runDefaultsView = { timeoutSeconds: 900, maxTurns: 60, policyPath: "~/.agentry/runner/policy.yaml", where: "here", outputDir: "" };

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
g.go = { main: {
  BrowseAPI: {
    Search: async () => (fresh ? { results: hits } : { total: hits.length, results: hits }),
    Agent: async () => detail,
    Readme: async () => "<h1>incident-summarizer</h1><p>Summarises an incident channel into a single report card. Reads the channel, redacts anything that looks like a secret, and writes <code>report.md</code>. Declares a strict egress allowlist and no filesystem write outside <code>reports/</code>.</p><h2>Usage</h2><pre><code>agentry run demo1/incident-summarizer</code></pre>",
    Report: async () => (unscanned ? { versionStatus: "published", scanStatus: "skipped", verdict: "unscanned", findings: [] } : report),
    Provenance: async () => prov,
  },
  InstallAPI: {
    ListInstalled: async () => (fresh ? [] : agentInstalled ? [...installedRecs, runnerRecForShown]
      : updateApplied ? installedRecs.map((r, i) => (i === 2 ? { ...r, version: "1.3.0" } : r)) : installedRecs),
    CheckUpdates: async () => [],
    Forget: async () => undefined,
    CheckUpdate: async () => {
      if (agentInstalledUnreachable) return { key: {}, current: "1.2.0", error: "dial tcp 127.0.0.1:1: connect: connection refused" };
      return { key: {}, current: runnerRecForShown.version, latest: "1.2.0", updateAvailable: agentInstalledOlder };
    },
    UpdatesStatus: async () => installedUpdateWarning && !updateApplied
      ? { checked: "2026-09-21T12:00:00Z", available: 1, updates: [{ key: { registry: "runner", namespace: "agentry", name: "changelog-from-git", target: "runner", dir: installedRecs[2].dir }, current: "1.2.1", latest: "1.3.0", updateAvailable: true }] }
      : { checked: "2026-09-21T12:00:00Z", available: installedUpdates, updates: [] },
    InstallRunner: async () => {
      if (agentInstalling) await new Promise(() => {});
      if (agentInstallRefused) throw new Error("not confirmed: Accept what demo1/incident-summarizer may do?");
      if (installedUpdateWarning) {
        updateApplied = true;
        const warning = "operator policy /home/me/.agentry/runner/policy.yaml could not be read (parse /home/me/.agentry/runner/policy.yaml: yaml: line 3: mapping values are not allowed in this context) — the export beside the install takes the generic AGENTS.md shape";
        return { message: "installed agentry/changelog-from-git@1.3.0 into the runner home — warning: " + warning, dest: installedRecs[2].dir, warning };
      }
      return { message: "installed demo1/incident-summarizer@1.2.0 into the runner home", dest: runnerRecForShown.dir };
    },
  },
  // The native folder picker, as Build's first save asks it (D-030).
  App: { ChooseDirectory: async () => "/home/me/quickagents/incident-summarizer" },
  BuildAPI: {
    NewModel: async () => buildModel, Exists: async () => false, Check: async () => buildCheck,
    Save: async () => undefined,
    // pages.html#build-validate: a validate run that failed, so the card
    // shows a V-rule problem and a content finding with their place.
    Validate: async () => ({ ...validateResult, passed: false, errorCount: 1,
      issues: [{ rule: "V5", path: "capabilities.network.egress[0]", message: "\"http://api.pagerduty.com\" is not a valid DNS host name", warning: false }],
      findings: [{ ruleId: "S4-prompt-injection-001", scanner: "S4", severity: "high", confidence: "medium", title: "instruction override phrasing", filePath: "AGENT.md", lineStart: 12, lineEnd: 12, excerpt: "ignore previous instructions" }] }),
    ServerChecks: async () => [{ ref: "npm:@cyanheads/osv-advisory-mcp-server@0.1.13", status: "safe", advisories: [], checkedAt: "2026-09-25T11:13:00Z", refuses: false }],
    DescribeStatus: async () => ({ key: jevKeyView, search: "no-provider", modelName: "openai qwen3.8-flash-next",
      searchSentence: "It has no web search, since your runner policy names no search provider, so it reads the registry and the pages it knows." }),
    DescribeOnThisComputer: async () => { await new Promise(() => {}); },
    DescribeInAgentryStatus: async () => describeHosted,
    AttachHostedDraft: async () => (hostedDrafting ? { draft: hostedDrafting } : {}),
    DescribeInAgentry: async () => { await new Promise(() => {}); },
    CancelHostedDraft: async () => ({ ...hostedDrafting, cancelRequested: true }),
    DescribeAgainText: async () => "What the person wants changed now:\n…",
  },
  ValidateAPI: { Run: async () => validateResult },
  // The account line on a signed-in registry's card (agentry-desktop#74,
  // and the organization of agentry-desktop#79). The hash picks which
  // state Settings previews: #account-org is a member of an organization,
  // whose tier, pool, seats and standing are the organization's;
  // #account-pending is a fold-in an admin has yet to approve, who still
  // spends from their own account; anything else is a person on Standard.
  // The page the Upgrade button opens is the registry's answer, never a
  // path built here, so the fixture answers one as a registry would.
  SettingsAPI: {
    Account: async () => {
      const page = "https://secureagentry.net/settings/subscription";
      if (location.hash === "#account-org") {
        return {
          user: {
            handle: "chris", displayName: "Chris", email: "chris@acme.example",
            orgs: [{ handle: "acme", role: "admin", state: "active" }],
          },
          subscription: {
            tier: "business", standing: "active", memberOf: "acme",
            seats: { used: 3, max: 10 },
            balance: 3960, allowance: 5000, submissionCost: 10, periodEnd: "2026-10-11T00:00:00Z",
            privateAgents: { max: 0, used: 4 },
            upgrade: { available: false, manage: true, url: "https://secureagentry.net/orgs/acme/admin" },
          },
        };
      }
      if (location.hash === "#account-pending") {
        return {
          user: {
            handle: "chris", displayName: "Chris", email: "chris@acme.example",
            orgs: [{ handle: "acme", role: "member", state: "pending" }],
            pendingOrg: { handle: "acme", displayName: "Acme", via: "domain" },
          },
          subscription: {
            tier: "standard", balance: 40, allowance: 50, submissionCost: 10, periodEnd: "2026-10-11T00:00:00Z",
            privateAgents: { max: 10, used: 4 },
            upgrade: { available: true, manage: false, url: page },
          },
        };
      }
      return {
        user: { handle: "chris", displayName: "Chris", email: "chris@example.test" },
        subscription: {
          tier: "standard", balance: 40, allowance: 50, submissionCost: 10, periodEnd: "2026-10-11T00:00:00Z",
          privateAgents: { max: 10, used: 4 },
          upgrade: { available: true, manage: false, url: page },
        },
      };
    },
    OpenUpgrade: async () => undefined,
    CodingAgents: async () => codingView(),
    // Settings → Run defaults (agentry-desktop#347): the saves answer as Go
    // would, and the picker "returns" a folder.
    RunDefaults: async () => runDefaultsView,
    SetRunBudgets: async (t: number, m: number) => (runDefaultsView = { ...runDefaultsView, timeoutSeconds: t, maxTurns: m }),
    SetRunWhere: async (w: string) => (runDefaultsView = { ...runDefaultsView, where: w }),
    ChooseDefaultOutputDir: async () => (runDefaultsView = { ...runDefaultsView, outputDir: "/home/me/agentry-runs" }),
    ClearDefaultOutputDir: async () => (runDefaultsView = { ...runDefaultsView, outputDir: "" }),
    SetInstallIntoCodingAgent: async (on: boolean) => { codingSwitch = on; return codingView(); },
    ChooseCodingAgentProjectDir: async () => codingView(),
  },
  // The "a newer Agentry Desktop" card (agentry-desktop#87). The hash picks
  // which state Settings previews: #selfupdate-none (nothing newer),
  // #selfupdate-nochannel (a build of the open core), #selfupdate-failed (the
  // last check could not be believed); anything else offers a release.
  SelfUpdateAPI: {
    SelfUpdateStatus: async () => {
      const state = (window.location.hash.match(/^#selfupdate-([\w-]+)/) || [])[1] || "available";
      const base = { channel: "https://dev.secureagentry.net/downloads/manifest.json", current: "0.1.0", enabled: true, checked: "2026-09-20T11:00:00Z" };
      switch (state) {
        case "nochannel": return { channel: "", current: "0.1.0", enabled: true };
        case "none": return base;
        case "failed": return { ...base, err: "the release the channel offers is not one this build accepts: the signature was made by a key this build does not trust: it claims key 7B0FF069B8E68A7B, and this build carries dev (BE6E950FCAA6286C)" };
        case "retired": return { ...base, downloadUrl: "https://dev.secureagentry.net/download", available: { version: "0.2.0", commit: "a7d2e603", signedBy: "dev", retired: true } };
        default: return { ...base, downloadUrl: "https://dev.secureagentry.net/download", available: { version: "0.2.0", commit: "a7d2e603", signedBy: "prod", retired: false } };
      }
    },
    SelfUpdateCheckNow: async () => ({ channel: "https://dev.secureagentry.net/downloads/manifest.json", current: "0.1.0", enabled: true, checked: new Date().toISOString() }),
    SetSelfUpdateCheckEnabled: async (on: boolean) => ({ channel: "https://dev.secureagentry.net/downloads/manifest.json", current: "0.1.0", enabled: on, checked: "2026-09-20T11:00:00Z" }),
    DismissSelfUpdate: async () => ({ channel: "https://dev.secureagentry.net/downloads/manifest.json", current: "0.1.0", enabled: true, dismissed: true }),
  },
  RunAPI: {
    // Settings' Secrets card (agentry-desktop#300): the registry keeps
    // secrets; a save and a delete answer as the vault does.
    KeepsSecrets: async () => true,
    SetHostedSecret: async () => {},
    DeleteHostedSecret: async () => {},
    SecretsPageURL: async () => "https://secureagentry.net/settings/secrets",
    ChosenHarnesses: async () => codingChosen,
    HarnessStatus: async () => [claudeCode, codex].map((h) => ({ ...h, chosen: codingChosen.some((c) => c.name === h.name) })),
    ChooseHarness: async () => [claudeCode, codex].map((h) => ({ ...h, chosen: true })),
    ForgetHarness: async () => [],
    // pages.html#agent-mcp-env: the MCP tab of a release declaring two
    // servers, one with plain env beneath its line (agentry-desktop#62).
    GatewayRequirements: async () => (window.location.hash === "#agent-mcp-env"
      ? {
        toolRows: [
          { tool: "write_report", grantedBy: "always granted — reports land in the run's output folder" },
          { tool: "list_inputs", grantedBy: "always granted — operator-provided input files, read-only" },
          { tool: "read_input", grantedBy: "always granted — operator-provided input files, read-only" },
        ],
        servers: [
          "QuickAgent Tools — the runner's companion server, served through a per-run Docker MCP Gateway; its tool surface is exactly the table above",
          "npm:@modelcontextprotocol/server-github@2025.4.8 — declared by the manifest; installed per-signature from the operator's configured registries; tools filtered to get_issue, list_issues, search_issues",
          "pypi:mcp-server-fetch@2025.4.7 — declared by the manifest; installed per-signature from the operator's configured registries",
        ],
        declaredServers: [
          { id: "npm:@modelcontextprotocol/server-github@2025.4.8", tools: ["get_issue", "list_issues", "search_issues"],
            env: [{ name: "GITHUB_API_URL", value: "https://api.github.com" }, { name: "GITHUB_TOOLSETS", value: "issues,pull_requests" }] },
          { id: "pypi:mcp-server-fetch@2025.4.7", tools: [], env: [] },
        ],
        vulnerabilities: [], dataAccess: [], secretNames: ["GITHUB_TOKEN"], embeddedTools: [], refusal: "", vulnerabilityLine: "",
      }
      : { toolRows: [], servers: [], declaredServers: [], vulnerabilities: [], dataAccess: [], secretNames: [], embeddedTools: [], refusal: "", vulnerabilityLine: "" }),
    // pages.html#docker-<state> previews the docker card in that state
    // (agentry-desktop#64): installable | groupMissing | pendingRestart | manual.
    PrereqStatus: async () => {
      const state = (window.location.hash.match(/^#docker-(\w+)/) || [])[1] || "installable";
      const base = { os: "linux", distro: "fedora", downloadUrl: "https://docs.docker.com/engine/install/", command: "dnf install -y moby-engine && systemctl enable --now docker && usermod -aG docker me", note: "After installation, log out and back in (or reboot) so your user joins the docker group.", groupCommand: "usermod -aG docker me", dockerErr: "permission denied while trying to connect to the docker daemon socket" };
      switch (state) {
        case "groupMissing": return { ...base, state, canInstall: true, engineInstalled: true, inDockerGroup: false, dockerOk: false };
        case "pendingRestart": return { ...base, state, canInstall: true, engineInstalled: true, inDockerGroup: true, dockerOk: false };
        case "manual": return { ...base, state, canInstall: false, engineInstalled: true, inDockerGroup: true, dockerOk: false, dockerErr: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?" };
        default: return { ...base, state: "installable", canInstall: true, engineInstalled: false, inDockerGroup: false, dockerOk: false };
      }
    },
    AddToDockerGroup: async () => ({ state: "pendingRestart" }),
    RestartNow: async () => undefined,
    SetupStatus: async () => (window.location.hash === "#setup-custom-http"
      ? { dockerOk: true, imagesReady: true, policyExists: true, policyPath: "~/.agentry/runner/policy.yaml", provider: "custom", model: "qwen3.8-flash", apiKeyEnv: "OPENAI_API_KEY", apiKeySource: "keyring", keysStored: ["custom"], baseUrl: "http://gpu-box.lan:8000/v1", timeoutSeconds: 900, maxTurns: 60 }
      : { dockerOk: !window.location.hash.startsWith("#docker-"), imagesReady: true, policyExists: true, policyPath: "~/.agentry/runner/policy.yaml", provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY", apiKeySource: "keyring", keysStored: ["anthropic"], baseUrl: "", timeoutSeconds: 900, maxTurns: 60 }),
    // The CLI's sentence (setup.PlainHTTPWarning) for plain http off this
    // machine; "" for https and loopback (agentry-desktop#317).
    EndpointWarning: async (u: string) => {
      const m = /^http:\/\/([^/:]+)/i.exec(u.trim());
      if (!m || m[1] === "localhost" || m[1].startsWith("127.")) return "";
      return `${u.trim()} uses plain http: the API key and the prompts travel unencrypted to ${m[1]}. Use https if the endpoint supports it.`;
    },
    ListModels: async () => ({ provider: "anthropic", kind: "api", notice: "", models: [{ id: "claude-opus-5", displayName: "Claude Opus 5" }, { id: "claude-sonnet-4-5", displayName: "Claude Sonnet 4.5" }] }),
    JevKeyStatus: async () => jevKeyView,
    StoreJevKey: async () => ({ stored: true, source: "keyring", envName: "TYPESAFE_API_KEY", envSet: false, ready: true }),
    ForgetJevKey: async () => ({ stored: false, envName: "TYPESAFE_API_KEY", envSet: false, ready: false }),
    TestJevKey: async () => (window.location.hash === "#aireview-refused"
      ? { ok: false, message: "TypeSafe refused the key (401). Check that you pasted the whole key and that it has not been revoked.", from: "the keychain" }
      : { ok: true, message: "The key works: TypeSafe answered one question with it.", from: "the keychain" }),
  },
} };
// pages.html#status-refused: every binding the Settings landing reads a
// status line from refuses, so the shot shows the cards without them.
if (window.location.hash === "#status-refused") {
  const refuse = async () => { throw new Error("refused (harness)"); };
  g.go.main.SelfUpdateAPI.SelfUpdateStatus = refuse;
  g.go.main.SettingsAPI.CodingAgents = refuse;
  g.go.main.RunAPI.SetupStatus = refuse;
  g.go.main.RunAPI.JevKeyStatus = refuse;
  g.go.main.SettingsAPI.RunDefaults = refuse;
}
g.runtime = {
  EventsOn: () => () => {}, EventsOnMultiple: () => () => {}, EventsOnce: () => () => {},
  EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import Browse from "../src/pages/Browse";
import AgentPage from "../src/pages/AgentPage";
import Installed from "../src/pages/Installed";
import Build from "../src/pages/Build";
import Validate from "../src/pages/Validate";
import Settings, { SETTINGS_FOR, SettingsPart, SettingsSection } from "../src/pages/Settings";

const settingsView = {
  defaultRegistry: "https://secureagentry.net", configPath: "~/.config/agentry/config.yaml",
  registries: [
    { url: "https://secureagentry.net", isDefault: true, hasToken: true, tokenSource: "keyring" },
    { url: "http://agentry:8080", isDefault: false, hasToken: false, tokenSource: "" },
  ],
};
const REG = "https://secureagentry.net";
function Shell({ active, children }: { active: string; children: React.ReactNode }) {
  const nav = ["Browse", "Installed", "Knowledge", "Build", "Validate", "Settings"];
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        {nav.map((n) => (
          <button key={n} className={`nav-item ${n === active ? "active" : ""}`}>
            {n}
            {n === "Installed" && installedUpdates > 0 && <span className="badge warn nav-count">{installedUpdates}</span>}
          </button>
        ))}
        <div className="foot"><div>v0.1.0</div><div className="nowrap">secureagentry.net</div></div>
      </div>
      <div className="content">{children}</div>
    </div>
  );
}

// Settings as five cards (agentry-desktop#346): the preview keeps its own
// route, as App does, so a shot can open a card, follow a part's link, come
// back by "← Settings", and follow each deep link App gives the places that
// open Settings. ?settings=<section> opens on a section page; the hash keeps
// picking the states the cards themselves show.
function SettingsPreview() {
  const q = new URLSearchParams(window.location.search);
  const [at, setAt] = useState<{ section?: SettingsSection; part?: SettingsPart }>({ section: (q.get("settings") || undefined) as SettingsSection | undefined });
  return (
    <>
      <Settings settings={settingsView as any} activeRegistry={REG} onChanged={async () => settingsView as any} onSelectRegistry={() => {}}
        onRunSetup={() => {}} section={at.section} part={at.part} onOpen={(section, part) => setAt({ section, part })} />
      <div className="harness-deeplinks muted small" style={{ marginTop: 24 }}>
        Deep links (harness only):{" "}
        {Object.entries(SETTINGS_FOR).map(([name, to]) => (
          <button key={name} type="button" data-deeplink={name} onClick={() => setAt(to)}>{name}</button>
        ))}
      </div>
    </>
  );
}

createRoot(document.getElementById("root")!).render(
  <div>
    <div data-preview="browse"><Shell active="Browse"><Browse registry={REG} onOpenAgent={() => {}} /></Shell></div>
    <div data-preview="agent"><Shell active="Browse"><AgentPage registry={REG} ns="demo1" name="incident-summarizer" onBack={() => {}} /></Shell></div>
    <div data-preview="installed"><Shell active="Installed"><Installed onBrowse={() => {}} onOpenAgent={() => {}} /></Shell></div>
    <div data-preview="build"><Shell active="Build"><Build registry={REG} initialView={buildDescribe ? "describe" : buildView as "start" | "steps" | "advanced"} initialStep={buildStep}
      initialDraft={draftedView as any} onOpenAgent={() => {}} onOpenSettings={() => {}} /></Shell></div>
    <div data-preview="validate"><Shell active="Validate"><Validate /></Shell></div>
    <div data-preview="settings"><Shell active="Settings"><SettingsPreview /></Shell></div>
  </div>,
);
