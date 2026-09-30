// A run on this computer followed above the pages (agentry-desktop#465), by
// location.hash, over the Installed page under the shell chrome, with the
// bindings mocked (the pattern drafts.tsx uses). Not shipped.
//
//   #run-running      the dialog while the run goes: time against its limit,
//                     turns and tokens over its folded log; Stop the run and
//                     Run in the background
//   #run-card         the same run shrunk to its corner card: Stop and Open
//   #run-stopped      a run stopped from its card: the card says so, Dismiss
//   #run-finished     the dialog once it is over: the result first, the
//                     files, Run details; Back to the setup and Dismiss
//   #run-failed       a run that ended with no result: Go's words, the log
//   #run-hostile      a finished run whose result, files and log carry raw
//                     HTML, a script, an image and a right-to-left override:
//                     all drawn as text
//   #run-card-stacked the run's card and a draft In Agentry's card stacked
//                     in App's corner, neither covering the other
import { useState } from "react";
import { createRoot } from "react-dom/client";
import "../src/style.css";
import { Wordmark } from "../src/ui";
import { SidebarNav } from "../src/nav";

const hash = window.location.hash.replace(/^#/, "") || "run-running";
const HOST = "secureagentry.net";
const REG = "https://secureagentry.net";
const REF = "demo1/incident-summarizer@1.2.0";
const RUN_ID = "7c1e4a90";
const hostile = hash === "run-hostile";
const started = Date.now() - 134_000;
const reports = [
  { runId: RUN_ID, ref: REF, startedAt: started, timeoutSeconds: 0, maxTurns: 0, turns: 0, inputTokens: 0, outputTokens: 0 },
  { runId: RUN_ID, ref: REF, startedAt: started, timeoutSeconds: 900, maxTurns: 60, turns: 0, inputTokens: 0, outputTokens: 0 },
  { runId: RUN_ID, ref: REF, startedAt: started, timeoutSeconds: 900, maxTurns: 60, turns: 7, inputTokens: 48_210, outputTokens: 3_904 },
];
const lines = [
  "using verified install demo1/incident-summarizer@1.2.0",
  "gateway: docker/mcp-gateway:v0.43.3 ready",
  "run 7c1e4a90: demo1/incident-summarizer@1.2.0 against openai qwen3.8-flash-next",
  "tool: read_input incident-channel.txt",
  ...(hostile ? ["tool: write_report <img src=x onerror=alert(1)><script>alert(2)</script> ‮gnp.exe"] : []),
];
const result = {
  status: hash === "run-stopped" ? "error" : "ok", exitCode: hash === "run-stopped" ? 130 : 0,
  error: hash === "run-stopped" ? "run interrupted" : "", runId: RUN_ID, ref: REF, digest: "sha256:9c356fd0d20fef07",
  resultJson: JSON.stringify(hostile
    ? "Report written. <img src=x onerror=alert(3)><script>alert(4)</script> [click](javascript:alert(5)) ‮txt.exe"
    : "INC-4821 began at 02:14 UTC with elevated 5xx on the checkout service. Draft report written to reports/report.md."),
  outDir: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-7c1e4a90",
  receiptsDir: "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-7c1e4a90/receipts",
  runPlanSha: "5d41402abc4b2a76b9719d911017c592ae2f1c3e8b6d7a9f0e1d2c3b4a596877",
  inputTokens: 48_210, outputTokens: 3_904, costUsd: 0, wallMs: 134_000, turns: 7, attestationState: "verified",
  policy: { path: "/home/chris/.agentry/runner/policy.yaml", source: "default", sha256: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" },
};
const outputs = {
  files: hostile
    ? [
      { path: "outputs/invoice\u202Efdp.exe", bytes: 1024, previewKind: "none", noPreview: "binary" },
      { path: "reports/report.md", bytes: 120, previewKind: "markdown", preview: "## Notes\n\n<iframe src=\"https://evil.example\"></iframe><b onclick=alert(6)>bold?</b> Plain notes." },
    ]
    : [
      { path: "reports/report.md", bytes: 3482, previewKind: "markdown", preview: "## INC-4821\n\nElevated 5xx on **checkout** from 02:14 UTC." },
    ],
  skipped: hostile ? 1 : 0,
  truncated: false,
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const listeners: Record<string, ((v: any) => void)[]> = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const fire = (name: string, v: any) => (listeners[name] ?? []).forEach((cb) => cb(v));
let stop: (() => void) | null = null;
g.go = { main: {
  RunAPI: {
    RunAgent: () => new Promise((resolve, reject) => {
      reports.forEach((p, i) => setTimeout(() => fire("runner:run-progress", p), 20 + 30 * i));
      setTimeout(() => lines.forEach((line) => fire("runner:run-line", { runId: RUN_ID, line })), 60);
      // A line of another run, which the flow must never show.
      setTimeout(() => fire("runner:run-line", { runId: "0thr0000", line: "A LINE OF ANOTHER RUN" }), 70);
      if (hash === "run-finished" || hostile) setTimeout(() => resolve(result), 160);
      else if (hash === "run-failed") setTimeout(() => reject("gateway/docker environment: the docker daemon did not answer: Cannot connect to the Docker daemon at unix:///var/run/docker.sock"), 160);
      else if (hash === "run-stopped") stop = () => setTimeout(() => resolve(result), 40);
    }),
    StopRun: async () => stop?.(),
    ForgetRun: async () => undefined,
    RunOutputs: async () => outputs,
    ShowRunFolder: async () => undefined,
  },
  BuildAPI: {
    DescribeInAgentry: async () => ({ draft: { id: "01a0e424-0000-0000-0000-000000000000", status: "running", state: "running",
      statusLine: "The Drafter is running on a sandbox host.", queuePosition: 0, cancelRequested: false, holdCredits: 41,
      creditsLine: "41 credits are held for this draft.", final: false } }),
    CancelHostedDraft: async () => ({}),
    AttachHostedDraft: async () => ({}),
  },
  SettingsAPI: { OpenUpgrade: async () => undefined },
  App: { OpenExternal: async () => undefined },
} };
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const subscribe = (name: string, cb: (v: any) => void) => {
  (listeners[name] ??= []).push(cb);
  return () => { listeners[name] = (listeners[name] ?? []).filter((c) => c !== cb); };
};
g.runtime = {
  EventsOn: subscribe, EventsOnMultiple: subscribe, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import RunFlow from "../src/pages/RunFlow";
import DraftFlow from "../src/pages/DraftFlow";
import { startLocalRun, stopLocalRun } from "../src/runs";
import { startDraft } from "../src/drafts";
import { CornerHost } from "../src/progress";

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="installed" />
        <div className="foot"><div>v0.6.29</div><div className="nowrap">{HOST}</div></div>
      </div>
      <div className="content">
        <h1>Installed</h1>
        <p className="muted">{children}</p>
      </div>
    </div>
  );
}

function Preview() {
  const [corner, setCorner] = useState<HTMLElement | null>(null);
  const card = hash === "run-card" || hash === "run-stopped" || hash === "run-card-stacked";
  return (
    <CornerHost.Provider value={corner}>
      <div data-preview="runs">
        <Shell>The page stays usable while the run goes on in its card.</Shell>
        <RunFlow initialMinimized={card} />
        {hash === "run-card-stacked" && <DraftFlow registry={REG} onOpenDraft={() => {}} initialMinimized />}
        <div className="corner-cards" ref={setCorner} />
      </div>
    </CornerHost.Provider>
  );
}

const form = {
  mode: "autonomous", task: "", inputs: [], pickedFiles: [], intakeValues: {}, attach: [], outDir: "", timeoutSec: "", maxTurns: "",
  keep: true, keepTouched: false, acceptUnsigned: false, acceptUnscanned: false, allowVulnerableMCP: false,
};
void startLocalRun(
  { panel: { registry: REG, ns: "demo1", name: "incident-summarizer", version: "1.2.0", attestation: null }, form },
  REF,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  () => g.go.main.RunAPI.RunAgent() as Promise<any>,
).then(() => {
  if (hash === "run-stopped") setTimeout(() => void stopLocalRun(), 120);
});
if (hash === "run-card-stacked") startDraft(REG, "agentry", "Check a CSV file I add for missing values, duplicate rows and mixed formats.", []);
createRoot(document.getElementById("root")!).render(<Preview />);
