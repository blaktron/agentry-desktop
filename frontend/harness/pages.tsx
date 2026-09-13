// Page previews (agentry-desktop#37+). Mocks the Wails bindings
// (window.go / window.runtime) with fixture data so the REAL page
// components render in a plain browser for screenshots — the bindings
// resolve through window.go at call time, so setting it before render is
// enough. Not shipped (outside tsconfig's include and the app's build).
import { createRoot } from "react-dom/client";
import "../src/style.css";
import { Wordmark } from "../src/ui";

const hits = [
  { namespace: "demo1", name: "incident-summarizer", summary: "Summarises an incident channel into a report card, redacting secrets first.", modes: ["autonomous"], quarantine: "", installs30d: 42 },
  { namespace: "acme", name: "pr-triage", summary: "Reads an open PR and drafts a triage note under a strict egress allowlist.", modes: ["autonomous", "interactive"], quarantine: "", installs30d: 7 },
  { namespace: "labs", name: "log-explainer", summary: "An interactive session over a log bundle; no filesystem write, no network.", modes: ["interactive"], quarantine: "", installs30d: 0 },
];
const detail = {
  agent: { namespace: "demo1", name: "incident-summarizer", summary: "Summarises an incident channel into a report card, redacting secrets first.", modes: ["autonomous"], quarantine: "" },
  latest: "1.2.0", latestDigest: "sha256:9c356fd0d20fef07123ba7c4a1b2c3d4e5f6a7b8",
  latestCapabilities: { network: { egress: ["api.pagerduty.com"] }, filesystem: { read: "workspace", write: { paths: ["reports"] } }, secrets: ["PAGERDUTY_TOKEN"] },
  dependencyAlerts: [],
  versions: [
    { version: "1.2.0", channel: "stable", status: "published", publishedAt: "2026-09-01T10:00:00Z", createdAt: "2026-09-01T10:00:00Z", archiveDigest: "sha256:9c356fd0", yanked: false },
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
const prov = { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified", rulesetVersion: "2026.09.1", judgeModel: "claude-sonnet-4-5" };
// The Installed page's three sections (agentry-desktop#63): two installs this
// app manages, a run consent the CLI wrote into the same file, and a record
// whose directory is gone.
const installedRecs = [
  { registry: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: "1.2.0", target: "agents-md", dir: "/home/me/work/oncall", managed: true, installedAt: "2026-09-02T09:00:00Z", capabilities: { egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsWrite: "paths", secrets: ["PAGERDUTY_TOKEN"], execCommands: [] }, present: true, manageable: true },
  { registry: "https://secureagentry.net", namespace: "acme", name: "pr-triage", version: "0.4.1", target: "claude-skill", dir: "/home/me/work/acme", managed: true, installedAt: "2026-08-30T14:00:00Z", capabilities: { egressHosts: ["api.github.com"], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: ["gh"] }, present: true, manageable: true },
  { registry: "runner", namespace: "agentry", name: "changelog-from-git", version: "1.2.1", target: "runner", dir: "/home/me/.agentry/runner/agentry/changelog-from-git", managed: false, installedAt: "2026-09-11T16:20:00Z", capabilities: { egressHosts: [], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: ["git"] }, present: true, manageable: false },
  { registry: "https://secureagentry.net", namespace: "acme", name: "release-notes", version: "0.2.0", target: "claude-skill", dir: "/home/me/old-project", managed: true, installedAt: "2026-07-14T10:00:00Z", capabilities: { egressHosts: [], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: [] }, present: false, manageable: true },
];
// pages.html#fresh previews a first run: nothing installed, and a registry
// that sent no total (a core before agentry#290) gets a list with no count.
const fresh = window.location.hash === "#fresh";

const buildModel = {
  name: "incident-summarizer", namespace: "demo1", modes: ["autonomous"], version: "1.2.0",
  license: "Apache-2.0", summary: "Summarises an incident channel into a report card, redacting secrets first.",
  tags: ["oncall", "reporting"], instructionFile: "AGENT.md",
  egressMode: "hosts", egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsReadPaths: [],
  fsWrite: "paths", fsWritePaths: ["reports"], execCommands: [], secrets: ["PAGERDUTY_TOKEN"],
  mcpServers: [], dataAccess: ["tickets"], intake: [], opening: "",
  instructions: "You summarise an incident channel. Read it, redact secrets, and write reports/report.md.",
};
const validateResult = {
  passed: true, errorCount: 0, warnCount: 0, inventoryFailing: 0, fileCount: 6,
  bundleVersion: "2026.09.1", bundleSource: "embedded", manifestName: "demo1/incident-summarizer",
  manifestModes: ["autonomous"], manifestVersion: "1.2.0", manifestError: "",
  issues: [], inventory: [], findings: [], readErrors: [],
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
g.go = { main: {
  BrowseAPI: {
    Search: async () => (fresh ? { results: hits } : { total: hits.length, results: hits }),
    Agent: async () => detail,
    Readme: async () => "<h1>incident-summarizer</h1><p>Summarises an incident channel into a single report card. Reads the channel, redacts anything that looks like a secret, and writes <code>report.md</code>. Declares a strict egress allowlist and no filesystem write outside <code>reports/</code>.</p><h2>Usage</h2><pre><code>agentry run demo1/incident-summarizer</code></pre>",
    Report: async () => report,
    Provenance: async () => prov,
  },
  InstallAPI: { ListInstalled: async () => (fresh ? [] : installedRecs), CheckUpdates: async () => [], Forget: async () => undefined },
  BuildAPI: { NewModel: async () => buildModel, Exists: async () => false },
  ValidateAPI: { Run: async () => validateResult },
  // The account line on a signed-in registry's card (agentry-desktop#74):
  // a Standard account on a registry with subscription tiers.
  SettingsAPI: {
    Account: async () => ({
      user: { handle: "chris", displayName: "Chris", email: "chris@example.test" },
      subscription: { tier: "standard", balance: 40, allowance: 50, submissionCost: 10, periodEnd: "2026-10-11T00:00:00Z", privateAgents: { max: 10, used: 4 }, upgradeUrl: "https://secureagentry.net/settings/subscription?upgrade=pro" },
    }),
    OpenUpgrade: async () => undefined,
  },
  RunAPI: {
    GatewayRequirements: async () => ({ toolRows: [], servers: [], vulnerabilities: [], dataAccess: [], secretNames: [], embeddedTools: [], refusal: "", vulnerabilityLine: "" }),
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
    SetupStatus: async () => ({ dockerOk: !window.location.hash.startsWith("#docker-"), imagesReady: true, policyExists: true, policyPath: "~/.agentry/runner/policy.yaml", provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY", baseUrl: "", timeoutSeconds: 900, maxTurns: 60 }),
  },
} };
g.runtime = {
  EventsOn: () => () => {}, EventsOnMultiple: () => () => {}, EventsOnce: () => () => {},
  EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import Browse from "../src/pages/Browse";
import AgentPage from "../src/pages/AgentPage";
import Installed from "../src/pages/Installed";
import Build from "../src/pages/Build";
import Validate from "../src/pages/Validate";
import Settings from "../src/pages/Settings";

const settingsView = {
  defaultRegistry: "https://secureagentry.net", configPath: "~/.config/agentry/config.yaml",
  registries: [
    { url: "https://secureagentry.net", isDefault: true, hasToken: true, tokenSource: "keyring" },
    { url: "http://agentry:8080", isDefault: false, hasToken: false, tokenSource: "" },
  ],
};
const REG = "https://secureagentry.net";
function Shell({ active, children }: { active: string; children: React.ReactNode }) {
  const nav = ["Browse", "Installed", "Build", "Validate", "Settings"];
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        {nav.map((n) => <button key={n} className={`nav-item ${n === active ? "active" : ""}`}>{n}</button>)}
        <div className="foot"><div>v0.1.0</div><div className="nowrap">secureagentry.net</div></div>
      </div>
      <div className="content">{children}</div>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <div>
    <div data-preview="browse"><Shell active="Browse"><Browse registry={REG} onOpenAgent={() => {}} /></Shell></div>
    <div data-preview="agent"><Shell active="Browse"><AgentPage registry={REG} ns="demo1" name="incident-summarizer" onBack={() => {}} /></Shell></div>
    <div data-preview="installed"><Shell active="Installed"><Installed onBrowse={() => {}} onOpenAgent={() => {}} /></Shell></div>
    <div data-preview="build"><Shell active="Build"><Build registry={REG} /></Shell></div>
    <div data-preview="validate"><Shell active="Validate"><Validate /></Shell></div>
    <div data-preview="settings"><Shell active="Settings"><Settings settings={settingsView as any} activeRegistry={REG} onChanged={async () => settingsView as any} onSelectRegistry={() => {}} /></Shell></div>
  </div>,
);
