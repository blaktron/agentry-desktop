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
const installedRecs = [
  { registry: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: "1.2.0", target: "agents-md", dir: "/home/me/work/oncall", managed: true, installedAt: "2026-09-02T09:00:00Z", capabilities: { egressHosts: ["api.pagerduty.com"], fsRead: "workspace", fsWrite: "paths", secrets: ["PAGERDUTY_TOKEN"], execCommands: [] } },
  { registry: "https://secureagentry.net", namespace: "acme", name: "pr-triage", version: "0.4.1", target: "claude-skill", dir: "/home/me/work/acme", managed: true, installedAt: "2026-08-30T14:00:00Z", capabilities: { egressHosts: ["api.github.com"], fsRead: "workspace", fsWrite: "none", secrets: [], execCommands: ["gh"] } },
];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
g.go = { main: {
  BrowseAPI: {
    Search: async () => ({ total: hits.length, results: hits }),
    Agent: async () => detail,
    Readme: async () => "<h1>incident-summarizer</h1><p>Summarises an incident channel into a single report card. Reads the channel, redacts anything that looks like a secret, and writes <code>report.md</code>. Declares a strict egress allowlist and no filesystem write outside <code>reports/</code>.</p><h2>Usage</h2><pre><code>agentry run demo1/incident-summarizer</code></pre>",
    Report: async () => report,
    Provenance: async () => prov,
  },
  InstallAPI: { ListInstalled: async () => installedRecs, CheckUpdates: async () => [] },
  RunAPI: { GatewayRequirements: async () => ({ toolRows: [], servers: [], vulnerabilities: [], dataAccess: [], secretNames: [], embeddedTools: [], refusal: "", vulnerabilityLine: "" }) },
} };
g.runtime = { EventsOn: () => () => {}, EventsOff: () => {} };

import Browse from "../src/pages/Browse";
import AgentPage from "../src/pages/AgentPage";
import Installed from "../src/pages/Installed";

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
  </div>,
);
