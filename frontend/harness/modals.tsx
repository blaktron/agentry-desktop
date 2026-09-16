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
//   #toolspanel | #chat    the other two run surfaces
import { createRoot } from "react-dom/client";
import "../src/style.css";

const profile = {
  ref: "demo1/incident-summarizer@1.2.0", digest: "sha256:9c356fd0d20fef07",
  capabilityLines: ["network egress: api.pagerduty.com", "filesystem: read workspace, write reports/", "secrets: PAGERDUTY_TOKEN"],
  declaredSecrets: ["PAGERDUTY_TOKEN"], storedSecretNames: ["PAGERDUTY_TOKEN"], exists: true, keep: true,
  modes: ["autonomous", "interactive"], intake: [{ id: "since", prompt: "Summarise incidents since when?", type: "text", required: true, choices: [] }], opening: "What incident should I look at?",
};
const setup = { dockerOk: true, imagesReady: true, timeoutSeconds: 900, maxTurns: 60, policyExists: true, provider: "anthropic", model: "claude-sonnet-4-5" };
const mcp = { refusal: "", vulnerabilityLine: "have no known high or critical advisories", toolRows: [], servers: [], vulnerabilities: [], dataAccess: [], secretNames: [], embeddedTools: [] };

// In Agentry (hosted runs M6). The view is the panel's: what a run holds,
// where it stands, and what it produced — the registry's numbers.
const view = location.hash.replace("#", "") || "runpanel";
const hostedState = view.startsWith("runpanel-") ? view.slice("runpanel-".length) : "";

const availability = {
  offered: true, available: hostedState !== "unavailable",
  reason: hostedState === "unavailable"
    ? "The token stored for this registry does not carry run:read and run:write, which a run In Agentry needs. It was minted before this app asked for them."
    : "",
  remedy: hostedState === "unavailable" ? "Sign in again from Settings to mint a token that carries them." : "",
  signedIn: true, extension: true, enabled: true, workersOnline: 1, queued: 2,
  tier: "standard", holdCredits: 20, tokensPerCredit: 1000,
  caps: { timeoutSeconds: 300, maxTurns: 20, maxTokens: 20000 },
  balance: 46, balanceKnown: true, upgradeUrl: "https://secureagentry.net/settings/subscription?upgrade=pro",
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
    ProfileStatus: async () => profile, SaveProfile: async () => profile,
    HostedAvailability: async () => availability,
    AttachHostedRun: async () => ({ run: hostedRun }),
    StartHostedRun: async () => ({ run: hostedRuns.queued }),
    CancelHostedRun: async () => ({ ...hostedRuns.queued, cancelRequested: true, statusLine: "cancelling" }),
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
import ToolsPanel from "../src/pages/ToolsPanel";
import { Wordmark } from "../src/ui";

const att = { state: "verified", keyId: "a1b2c3d4", detail: "DSSE verified" } as any;

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
if (view === "chat") {
  createRoot(el).render(<ChatSample />);
} else if (view === "toolspanel") {
  createRoot(el).render(<><Backdrop /><ToolsPanel registry="https://secureagentry.net" ns="demo1" name="incident-summarizer" version="1.2.0" onClose={() => {}} /></>);
} else {
  createRoot(el).render(
    <><Backdrop />
      <RunPanel
        registry="https://secureagentry.net" ns="demo1" name="incident-summarizer" version="1.2.0"
        attestation={att} onClose={() => {}}
        initialMode={hostedState === "hosted" || hostedState === "unavailable" ? "hosted" : undefined}
      />
    </>,
  );
}
