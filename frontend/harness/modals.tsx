// Run-surface previews (agentry-desktop#39): RunPanel and ToolsPanel are
// modals and SessionChat is a chat pane. Mocks the bindings and selects one
// surface by location.hash (#runpanel | #toolspanel | #chat) so each gets a
// clean screenshot. Not shipped.
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

const g = window as any;
g.go = { main: {
  RunAPI: {
    SetupStatus: async () => setup, GatewayRequirements: async () => mcp,
    ProfileStatus: async () => profile, SaveProfile: async () => profile,
  },
} };
g.runtime = { EventsOn: () => () => {}, EventsOnMultiple: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {} };

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

const view = location.hash.replace("#", "") || "runpanel";
const el = document.getElementById("root")!;
if (view === "chat") {
  createRoot(el).render(<ChatSample />);
} else if (view === "toolspanel") {
  createRoot(el).render(<><Backdrop /><ToolsPanel registry="https://secureagentry.net" ns="demo1" name="incident-summarizer" version="1.2.0" onClose={() => {}} /></>);
} else {
  createRoot(el).render(<><Backdrop /><RunPanel registry="https://secureagentry.net" ns="demo1" name="incident-summarizer" version="1.2.0" attestation={att} onClose={() => {}} /></>);
}
