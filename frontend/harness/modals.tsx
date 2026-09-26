// Run-surface previews (agentry-desktop#39, extended for In Agentry in
// #77): RunPanel and ToolsPanel are modals and SessionChat is a chat pane.
// Mocks the bindings and selects one surface by location.hash so each gets
// a clean screenshot. Not shipped.
//
//   #runpanel              the mode choice, In Agentry offered
//   #runpanel-hosted       In Agentry chosen: the hold, the balance, the caps
//   #runpanel-unavailable  In Agentry offered, unavailable with the reason
//   #runpanel-queued       a hosted run queued, with its position
//   #runpanel-running      a hosted run running: its state, no log
//   #runpanel-finished     a hosted run done — pass: the receipt the
//                          registry kept, and the outputs
//   #runpanel-failed       a pushed run done — fail: no receipt kept
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
//
// The result first (agentry-desktop#227): -finished and -failed show the
// result view over their Run details, and
//   #runpanel-hostile       a finished run whose result and files carry
//                           hostile markdown (raw HTML, a script, an image,
//                           a javascript: link) and a file name with a
//                           right-to-left override: none of it becomes
//                           markup, and the hidden characters show
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
//   #runpanel-knowledge-finished  a local run with a pack attached, finished
//                                 (shots.mjs clicks Run): the receipt's
//                                 knowledge line under the result
//
// A run on this machine, finished (shots.mjs clicks Run on the review):
//   #runpanel-local-finished      its Run details name the operator policy
//                                 it enforced, the runner home's default
//                                 (agentry-cli#152, agentry-desktop#269)
//   #runpanel-local-finished-env  the same run with the runner home
//                                 relocated by AGENTRY_RUNNER_HOME
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
const guidedShape = view.startsWith("runpanel-guided");
const keysShape = view === "runpanel-keys" || view === "runpanel-keys-saved" || view === "runpanel-step-tools" || guidedShape;
const hostileGuide = view.startsWith("runpanel-guided-hostile");
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
      GITHUB_TOKEN: "Paste it <b onclick=\"steal()\">here</b>. Not javascript:alert(1), file:///etc/passwd or https://github.com@evil.example/x but https://github.com/settings/tokens\u202E, or the look-alike https://g\u0456thub.com/new.",
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
// A finished local run: no intake question, so Run on the review is enabled.
const localFinished = view.startsWith("runpanel-local-finished") || view === "runpanel-knowledge-finished";
const knowledgeShape = view === "runpanel-step-knowledge" || view === "runpanel-knowledge-finished";
const knowledgePacks = knowledgeShape
  ? [
    { name: "incident-runbooks", pages: 131, bytes: 6_902_112, scan: "scanned in Agentry, 2 findings", scanState: "scanned", local: true, inAgentry: true, changed: false, where: "this computer and Agentry", key: "your account's key at secureagentry.net, which Agentry holds", wrapper: "registry", registry: "secureagentry.net", opensOffline: false },
    { name: "oncall-notes", pages: 12, bytes: 184_320, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, changed: false, where: "this computer", key: "your account's key at secureagentry.net, which Agentry holds", wrapper: "registry", registry: "secureagentry.net", opensOffline: true },
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
};
const profile = keysShape
  ? {
    ref: "agentry/pr-review-brief@1.2.1", digest: "sha256:4e6d650976b7a1b2",
    capabilityLines: ["network egress: none", "filesystem: read none, write none", "MCP: github-mcp-server-js (api.github.com; secrets: GITHUB_TOKEN)", "dataAccess: code"],
    declaredSecrets: hostileGuide ? ["GITHUB_TOKEN", "constructor"] : ["GITHUB_TOKEN"], storedSecretNames: [], exists: false, keep: true,
    modes: ["autonomous"], intake: [], opening: "",
    ...(guidedShape ? { guide } : {}),
  }
  : {
    ref: "demo1/incident-summarizer@1.2.0", digest: "sha256:9c356fd0d20fef07",
    capabilityLines: ["network egress: api.pagerduty.com", "filesystem: read workspace, write reports/", "secrets: PAGERDUTY_TOKEN"],
    declaredSecrets: ["PAGERDUTY_TOKEN"], storedSecretNames: ["PAGERDUTY_TOKEN"], exists: true, keep: true,
    modes: ["autonomous", "interactive"],
    intake: fileIntakeShape
      ? [{ id: "channel_export", prompt: "The incident channel's export (a file)", type: "file", required: true, choices: [] }]
      : localFinished ? []
      : [{ id: "since", prompt: "Summarise incidents since when?", type: "text", required: true, choices: [] }],
    opening: "What incident should I look at?",
  };
const setup = { dockerOk: true, imagesReady: true, timeoutSeconds: 900, maxTurns: 60, policyExists: true, provider: "anthropic", model: "claude-sonnet-4-5" };
const mcp = {
  refusal: "", vulnerabilityLine: "have no known high or critical advisories", toolRows: [],
  servers: keysShape ? ["github-mcp-server-js"] : [], vulnerabilities: [], dataAccess: keysShape ? ["code"] : [],
  declaredServers: keysShape
    ? [{ id: "npm:github-mcp-server-js@0.1.7", tools: ["get_pull_request", "list_pull_request_files", "list_pull_request_commits"], env: [{ name: "GITHUB_PERMISSION", value: "read-only" }] }]
    : [],
  secretNames: keysShape ? ["GITHUB_TOKEN"] : [], embeddedTools: [],
};

const availability = {
  offered: true, available: hostedState !== "unavailable",
  reason: hostedState === "unavailable"
    ? "The token stored for this registry does not carry run:read and run:write, which a run In Agentry needs. It was minted before this app asked for them."
    : "",
  remedy: hostedState === "unavailable" ? "Sign in again from Settings to mint a token that carries them." : "",
  signedIn: true, extension: true, enabled: true, runnersOnline: 1, queued: 2,
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
  queued: { ...hostedBase, status: "queued", state: "queued", statusLine: "queued", queuePosition: 2 },
  running: { ...hostedBase, status: "running", state: "running", statusLine: "running" },
  finished: {
    ...hostedBase, status: "finished", state: "done — pass", statusLine: "ran to the end and wrote its result",
    outcome: "succeeded", outcomeTone: "ok", outcomeLine: "ran to the end and wrote its result",
    exitClass: "ok", attestationState: "verified", adapter: "claude-code",
    turns: 4, inputTokens: 5120, outputTokens: 1430, totalTokens: 6550, costUsd: 0.0182, wallMs: 41200,
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
hostedRuns.failed = {
  ...hostedRuns.finished, state: "done — fail", statusLine: "the agent ran and failed — the receipt and the outputs say where",
  outcome: "failed", outcomeTone: "bad", outcomeLine: "the agent ran and failed — the receipt and the outputs say where",
  exitClass: "agent-error", settledCredits: 3, turns: 2, inputTokens: 2210, outputTokens: 380, totalTokens: 2590, wallMs: 18400,
  // A run the registry kept no receipt for (pushed before agentry-enterprise#121).
  receiptKept: false, capabilitiesEnforced: {}, attestationState: "", adapter: "",
  resultJson: "",
  outputs: [hostedRuns.finished.outputs[2]],
};
hostedRuns.hostile = {
  ...hostedRuns.finished,
  resultJson: JSON.stringify(
    "# Summary\n\nThe report is **ready**. <script>alert('x')</script><b onclick=\"steal()\">raw html</b>\n\n" +
      "![tracking pixel](https://evil.example/p.gif) [open me](javascript:alert(1)) [docs](https://example.com/docs)\n\n" +
      "pay\u200Bpal invoice attached.",
  ),
};
const hostedRun = hostedRuns[hostedState] ?? null;

// What the result view lists from the run folder (RunAPI.RunOutputs).
const runOutputs = view === "runpanel-hostile"
  ? {
    files: [
      { path: "outputs/invoice\u202Efdp.exe", bytes: 1024, previewKind: "none", noPreview: "binary" },
      { path: "outputs/notes.md", bytes: 120, previewKind: "markdown", preview: "## Notes\n\n<iframe src=\"https://evil.example\"></iframe>Plain notes." },
    ],
    skipped: 1,
  }
  : {
    files: [
      { path: "outputs/summary.csv", bytes: 64, previewKind: "text", preview: "incident,started,severity\nINC-4821,02:14,high\n" },
      { path: "reports/report.md", bytes: 3482, previewKind: "markdown", preview: "## INC-4821\n\nElevated 5xx on **checkout** from 02:14 UTC.\n\n| step | when |\n|---|---|\n| detected | 02:14 |\n| mitigated | 02:41 |\n" },
    ],
    skipped: 0,
  };

// Which step each state opens on: the blockers and Run are on the review,
// the key form on Connect tools, the rest where the view says.
const STEP_OF: Record<string, "start" | "files" | "knowledge" | "tools" | "details" | "review"> = {
  "runpanel-keys": "review",
  "runpanel-keys-saved": "tools",
  "runpanel-file-intake": "review",
  "runpanel-unscanned": "review",
  "runpanel-step-files": "files",
  "runpanel-step-tools": "tools",
  "runpanel-step-details": "details",
  "runpanel-step-review": "review",
  "runpanel-guided-files": "files",
  "runpanel-guided-tools": "tools",
  "runpanel-guided-details": "details",
  "runpanel-guided-hostile-tools": "tools",
  "runpanel-local-finished": "review",
  "runpanel-step-knowledge": "knowledge",
  "runpanel-knowledge-finished": "review",
  "runpanel-local-finished-env": "review",
};

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
    RunAgent: async () => localResult,
    RunOutputs: async () => runOutputs,
    ShowRunFolder: async () => {},
  },
  InstallAPI: {
    // #install-unscanned: the plan the wizard shows for a release the
    // registry admitted without a scan.
    Prepare: async () => ({
      id: "plan-1", registry: "https://secureagentry.net", namespace: "demo1", name: "incident-summarizer", version: "1.2.0",
      modes: ["autonomous"], summary: "Summarises an incident channel into a report card, redacting secrets first.",
      digest: "sha256:9c356fd0d20fef07123ba7c4a1b2c3d4e5f6a7b8", yanked: false,
      attestation: att, unsigned: false, unscanned: view === "install-unscanned",
      capabilities: { network: { egress: ["api.pagerduty.com"] }, filesystem: { read: "workspace", write: { paths: ["reports"] } }, secrets: ["PAGERDUTY_TOKEN"] },
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
  App: { OpenExternal: async () => {} },
} };
// The app's EventsOn goes through EventsOnMultiple (the wails runtime
// shim). A run In Agentry sends no log (agentry-desktop#182), so nothing
// is scripted on runner:run-log.
const subscribe = (_name: string, _cb: (v: any) => void) => () => {};
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
        {["Browse", "Installed", "Knowledge", "Build", "Validate", "Settings"].map((n) => <button key={n} className={`nav-item ${n === "Browse" ? "active" : ""}`}>{n}</button>)}
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
if (view === "install-unscanned" || view === "install-mcp-env") {
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
        summary={keysShape
          ? "Turns one GitHub pull request into the reviewer's brief: what changed, the risks, what to test and questions for the author."
          : "Summarises an incident channel into a report card, redacting secrets first."}
        initialMode={hostedState === "hosted" || hostedState === "unavailable" ? "hosted" : undefined}
        initialStep={STEP_OF[view]}
      />
    </>,
  );
}
