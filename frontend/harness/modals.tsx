// Run-surface previews (agentry-desktop#39, extended for In Agentry in
// #77): RunPanel and ToolsPanel are modals and SessionChat is a chat pane.
// Mocks the bindings and selects one surface by location.hash so each gets
// a clean screenshot. Not shipped.
//
//   #runpanel              the mode choice, In Agentry offered
//   #runpanel-hosted       In Agentry chosen: the hold, the balance, the caps
//   #runpanel-unavailable  In Agentry offered, unavailable with the reason
//   #runpanel-queued       a hosted run queued, with its position
//   #runpanel-streaming    a hosted run streaming its log
//   #runpanel-finished     a hosted run finished: receipt and artefacts
//   #runpanel-keys         pr-review-brief's shape: keys unsaved, no intake —
//                          the line above Run names the keys, Input files
//                          collapsed (agentry-desktop#167)
//   #runpanel-file-intake  a release asking for a file up front
//   #runpanel-unscanned    a release the registry admitted without a scan:
//                          its own mark and consent, In Agentry not offered
//   #install-unscanned     the install wizard on the same release (M23)
//   #toolspanel | #chat    the other two run surfaces
import { createRoot } from "react-dom/client";
import "../src/style.css";

// In Agentry (hosted runs M6). The view is the panel's: what a run holds,
// where it stands, and what it produced — the registry's numbers.
const view = location.hash.replace("#", "") || "runpanel";
const hostedState = view.startsWith("runpanel-") ? view.slice("runpanel-".length) : "";

// The run form asks only for what the release declares (agentry-desktop#167).
// #runpanel-keys is agentry/pr-review-brief's shape: one GitHub MCP server
// with GITHUB_TOKEN, no intake, no filesystem — the keys unsaved, so the
// line above Run names them and nothing else; #runpanel-file-intake a
// release that asks for a file up front — the intake form shows and the
// unanswered question is what the line names.
const keysShape = view === "runpanel-keys" || view === "runpanel-keys-saved";
const fileIntakeShape = view === "runpanel-file-intake";
const profile = keysShape
  ? {
    ref: "agentry/pr-review-brief@1.2.1", digest: "sha256:4e6d650976b7a1b2",
    capabilityLines: ["network egress: none", "filesystem: read none, write none", "MCP: github-mcp-server-js (api.github.com; secrets: GITHUB_TOKEN)", "dataAccess: code"],
    declaredSecrets: ["GITHUB_TOKEN"], storedSecretNames: [], exists: false, keep: true,
    modes: ["autonomous"], intake: [], opening: "",
  }
  : {
    ref: "demo1/incident-summarizer@1.2.0", digest: "sha256:9c356fd0d20fef07",
    capabilityLines: ["network egress: api.pagerduty.com", "filesystem: read workspace, write reports/", "secrets: PAGERDUTY_TOKEN"],
    declaredSecrets: ["PAGERDUTY_TOKEN"], storedSecretNames: ["PAGERDUTY_TOKEN"], exists: true, keep: true,
    modes: ["autonomous", "interactive"],
    intake: fileIntakeShape
      ? [{ id: "channel_export", prompt: "The incident channel's export (a file)", type: "file", required: true, choices: [] }]
      : [{ id: "since", prompt: "Summarise incidents since when?", type: "text", required: true, choices: [] }],
    opening: "What incident should I look at?",
  };
const setup = { dockerOk: true, imagesReady: true, timeoutSeconds: 900, maxTurns: 60, policyExists: true, provider: "anthropic", model: "claude-sonnet-4-5" };
const mcp = {
  refusal: "", vulnerabilityLine: "have no known high or critical advisories", toolRows: [],
  servers: keysShape ? ["github-mcp-server-js"] : [], vulnerabilities: [], dataAccess: keysShape ? ["code"] : [],
  secretNames: keysShape ? ["GITHUB_TOKEN"] : [], embeddedTools: [],
};

const availability = {
  offered: true, available: hostedState !== "unavailable",
  reason: hostedState === "unavailable"
    ? "The token stored for this registry does not carry run:read and run:write, which a run In Agentry needs. It was minted before this app asked for them."
    : "",
  remedy: hostedState === "unavailable" ? "Sign in again from Settings to mint a token that carries them." : "",
  signedIn: true, extension: true, enabled: true, workersOnline: 1, queued: 2,
  tier: "standard", holdCredits: 20, tokensPerCredit: 1000,
  caps: { timeoutSeconds: 300, maxTurns: 20, maxTokens: 20000 },
  balance: 46, balanceKnown: true,
  maxInputFiles: 20, maxInputBytes: 10 * 1024 * 1024,
};

const hostedBase = {
  id: "0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42", ref: "demo1/incident-summarizer@1.2.0",
  digest: "sha256:9c356fd0d20fef07", holdCredits: 20, caps: availability.caps,
  outputs: [], turns: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0, costUsd: 0, wallMs: 0,
  settledCredits: 0, settled: false, cancelRequested: false, final: false,
  createdAt: "2026-09-16T10:04:00Z",
};

const hostedRuns: Record<string, any> = {
  queued: { ...hostedBase, status: "queued", statusLine: "queued", queuePosition: 2 },
  streaming: { ...hostedBase, status: "running", statusLine: "running" },
  finished: {
    ...hostedBase, status: "finished", statusLine: "ran to the end and wrote its result",
    outcome: "succeeded", outcomeTone: "ok", outcomeLine: "ran to the end and wrote its result",
    exitClass: "ok", attestationState: "verified", adapter: "claude-code",
    turns: 4, inputTokens: 5120, outputTokens: 1430, totalTokens: 6550, costUsd: 0.0182, wallMs: 41200,
    settledCredits: 7, settled: true, final: true,
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
const hostedRun = hostedRuns[hostedState] ?? null;

const hostedLog = [
  "uploaded incidents.csv (2841 bytes)",
  "run 0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42 queued In Agentry — demo1/incident-summarizer@1.2.0, holding 20 credits",
  "materializing demo1/incident-summarizer@1.2.0",
  "verifying digest sha256:9c356fd0d20fef07 — DSSE verified",
  "gateway up: 4 tools from the declared contract",
  "turn 1 · tool: read_input",
  "turn 2 · tool: write_report",
];

const g = window as any;
g.go = { main: {
  RunAPI: {
    SetupStatus: async () => setup, GatewayRequirements: async () => mcp,
    ProfileStatus: async () => profile,
    // Saving the keys makes the profile exist: the line above Run goes and
    // Run enables (#runpanel-keys-saved, agentry-desktop#167).
    SaveProfile: async () => ({ ...profile, exists: true, storedSecretNames: profile.declaredSecrets }),
    HostedAvailability: async () => availability,
    AttachHostedRun: async () => ({ run: hostedRun }),
    StartHostedRun: async () => ({ run: hostedRuns.queued }),
    CancelHostedRun: async () => ({ ...hostedRuns.queued, cancelRequested: true, statusLine: "cancelling" }),
  },
  InstallAPI: {
    // #install-unscanned: the plan the wizard shows for a release the
    // registry admitted without a scan.
    Prepare: async () => ({
      id: "plan-1", registry: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: "1.2.0",
      modes: ["autonomous"], summary: "Summarises an incident channel into a report card, redacting secrets first.",
      digest: "sha256:9c356fd0d20fef07123ba7c4a1b2c3d4e5f6a7b8", yanked: false,
      attestation: att, unsigned: false, unscanned: true,
      capabilities: { network: { egress: ["api.pagerduty.com"] }, filesystem: { read: "workspace", write: { paths: ["reports"] } }, secrets: ["PAGERDUTY_TOKEN"] },
      needsCapabilityConfirm: true, prevVersion: "", diff: null, requiresWidenedConfirm: false,
      vulnerabilities: [], vulnerabilityLine: "have no known high or critical advisories", vulnerabilityRefusal: "",
      target: "agents-md", paths: { dest: "/home/chris/agents/incident-summarizer", managed: true, managedRoot: "/home/chris/agents", agentsMd: "/home/chris/agents/AGENTS.md" },
    }),
    Confirm: async () => ({ dir: "/home/chris/agents/incident-summarizer" }),
    Cancel: async () => {},
  },
  App: { OpenExternal: async () => {} },
} };
// The app's EventsOn goes through EventsOnMultiple (the wails runtime
// shim), so the scripted log is delivered from there.
const subscribe = (name: string, cb: (v: any) => void) => {
  if (name === "runner:run-log" && hostedRun) {
    const lines = hostedState === "queued" ? hostedLog.slice(0, 2) : hostedLog;
    setTimeout(() => lines.forEach((l) => cb(l)), 0);
  }
  return () => {};
};
g.runtime = {
  EventsOn: subscribe,
  EventsOnMultiple: (name: string, cb: (v: any) => void) => subscribe(name, cb),
  EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import RunPanel from "../src/pages/RunPanel";
import InstallWizard from "../src/pages/InstallWizard";
import ToolsPanel from "../src/pages/ToolsPanel";
import { Wordmark } from "../src/ui";

const att = (hostedState === "unscanned" || view === "install-unscanned"
  ? { state: "unscanned", detail: "the registry admitted this version without a scan: no findings, no report and no attestation stand behind it" }
  : { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified" }) as any;

function Backdrop() {
  return (
    <div className="shell" style={{ height: "100vh" }}>
      <div className="sidebar"><div className="brand"><Wordmark sub="Desktop" /></div>
        {["Browse", "Installed", "Build", "Validate", "Settings"].map((n) => <button key={n} className={`nav-item ${n === "Browse" ? "active" : ""}`}>{n}</button>)}
      </div>
      <div className="content" />
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
if (view === "install-unscanned") {
  createRoot(el).render(<><Backdrop /><InstallWizard registry="https://secureagentry.net" agentRef="demo1/incident-summarizer@1.2.0" onClose={() => {}} /></>);
} else if (view === "chat") {
  createRoot(el).render(<ChatSample />);
} else if (view === "toolspanel") {
  createRoot(el).render(<><Backdrop /><ToolsPanel registry="https://secureagentry.net" ns="demo1" name="incident-summarizer" version="1.2.0" onClose={() => {}} /></>);
} else {
  createRoot(el).render(
    <><Backdrop />
      <RunPanel
        registry="https://secureagentry.net"
        ns={keysShape ? "agentry" : "demo1"} name={keysShape ? "pr-review-brief" : "incident-summarizer"} version={keysShape ? "1.2.1" : "1.2.0"}
        attestation={att} onClose={() => {}}
        initialMode={hostedState === "hosted" || hostedState === "unavailable" ? "hosted" : undefined}
      />
    </>,
  );
}
