// A run on this computer followed above the pages, by
// location.hash, over the Library under the shell chrome, with the
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
//
// A run In Agentry, followed the same way:
//   #hosted-queued      the dialog while it waits: its place in the queue,
//                       the credits held, Cancel run
//   #hosted-running     the dialog while it runs: time against its cap
//   #hosted-cancel      Cancel asked for: Agentry is stopping it
//   #hosted-finished    the dialog once it is over: the result first, the
//                       files, Run details with the receipt's knowledge line
//   #hosted-failed      over, failed, with no receipt kept
//   #hosted-hostile     over, its result and files carrying hostile
//                       markdown and a right-to-left override: all text
//   #hosted-lost        the app lost touch while it was still going:
//                       Follow it again
//   #hosted-card        a run already in flight when the app started,
//                       attached as its card
//   #hosted-card-finished a run that ended while its dialog was closed,
//                       landed in its card
//   #hosted-card-stacked its card and a run on this computer's card
//
// The run page, opened from Home's Recent runs:
//   #runpage-local          a run on this computer the history keeps: how it
//                           ended (the history's), the result, the files
//   #runpage-hostile        the same, its result and files carrying raw HTML,
//                           a script, an image and a right-to-left override
//   #runpage-gone           its folder has gone: said, Remove from Recent runs
//   #runpage-draft          a draft by the Drafter: its result view and Open in
//                           Build
//   #runpage-review         a review by the Examiner: its report on the page
//   #runpage-knowledge      a knowledge build, stopped, that keeps no folder:
//                           Open in Knowledge
//   #runpage-hosted         a run In Agentry whose output is not here:
//                           Download output
//   #runpage-hosted-expired one whose output the registry no longer holds:
//                           no Download output, and why
//   #runpage-hosted-kept    one whose output was downloaded: the result view
//   #runpage-hosted-draft   a draft In Agentry: Open in Build, no download
//
// A chat, here and In Agentry, followed the same way:
//   #chat-local           the dialog is the chat: the conversation, the strip
//   #chat-local-working   its card while a message is out: working, turns
//   #chat-local-waiting   its card between turns: waiting for you
//   #chat-local-ended     the engine ended it with the dialog closed: its
//                         result, collected by the store
//   #chat-hosted          a chat In Agentry: replayed from the host, a hostile
//                         reply as plain text, the time-and-credits strip
//   #chat-hosted-working  its card while the agent answers
//   #chat-hosted-waiting  its card between turns
//   #chat-hosted-ended    over: the result, the session's credits and
//                         transcript.md among the files
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
const hostile = hash === "run-hostile" || hash === "hosted-hostile" || hash === "runpage-hostile";
const runPage = hash.startsWith("runpage-");
const hostedOnly = (hash.startsWith("hosted-") && hash !== "hosted-card-stacked") || hash.startsWith("chat-") || runPage;
const H_ID = "0199b2c4-6f1a-7c31-9a55-2f0f7f0b1d42";
const H_DIR = "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-0199b2c4";
const hostedBase = {
  id: H_ID, ref: REF, digest: "sha256:9c356fd0d20fef07", holdCredits: 20,
  caps: { timeoutSeconds: 900, maxTurns: 60 },
  outputs: [], turns: 0, wallMs: 0,
  settledCredits: 0, settled: false, cancelRequested: false, final: false, outDir: H_DIR,
  createdAt: new Date(Date.now() - 300_000).toISOString(),
};
const hStarted = new Date(Date.now() - 222_000).toISOString();
const knowledgeLine = "incident-runbooks mounted read-only at /knowledge/incident-runbooks (sha256:3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855, 131 pages); tools knowledge_list, knowledge_search and knowledge_read, on the tools container only";
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const hosted: Record<string, any> = {
  queued: { ...hostedBase, status: "queued", state: "queued", statusLine: "queued: waiting for a sandbox host", queuePosition: 3 },
  running: { ...hostedBase, status: "running", state: "running", statusLine: "running on a sandbox host", startedAt: hStarted },
  finished: {
    ...hostedBase, status: "finished", state: "done — pass", statusLine: "ran to the end and wrote its result", startedAt: hStarted,
    outcome: "succeeded", outcomeTone: "ok", outcomeLine: "ran to the end and wrote its result",
    exitClass: "ok", attestationState: "verified", adapter: "docker-agent",
    turns: 4, wallMs: 41200,
    settledCredits: 7, settled: true, final: true, receiptKept: true,
    capabilitiesEnforced: { "network.egress": "llm-proxy allowlist: api.pagerduty.com", knowledge: knowledgeLine },
    knowledge: [{ name: "incident-runbooks", digest: "sha256:3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" }],
    knowledgeLine,
    resultJson: JSON.stringify("INC-4821 began at 02:14 UTC with elevated 5xx on the checkout service. Draft report written to reports/report.md."),
    outputs: [
      { name: "result.json", bytes: 214, path: `${H_DIR}/result.json` },
      { name: "reports/report.md", bytes: 3482, path: `${H_DIR}/reports/report.md` },
    ],
  },
};
hosted.failed = {
  ...hosted.finished, state: "done — fail", statusLine: "the agent ran and failed — the receipt and the outputs say where",
  outcome: "failed", outcomeTone: "bad", outcomeLine: "the agent ran and failed — the receipt and the outputs say where",
  exitClass: "agent-error", settledCredits: 3, receiptKept: false, capabilitiesEnforced: {}, knowledge: [], knowledgeLine: "",
  attestationState: "", resultJson: "", outputs: [],
};
hosted.hostile = {
  ...hosted.finished,
  resultJson: JSON.stringify(
    "# Summary\n\nThe report is **ready**. <script>alert('x')</script><b onclick=\"steal()\">raw html</b>\n\n" +
      "![tracking pixel](https://evil.example/p.gif) [open me](javascript:alert(1)) [docs](https://example.com/docs)\n\n" +
      "pay\u200Bpal invoice attached.",
  ),
};
// A chat In Agentry twelve minutes in, and over with its tokens settled.
const sStarted = new Date(Date.now() - (12 * 60 + 40) * 1000).toISOString();
const sAt = (sec: number) => new Date(Date.parse(sStarted) + sec * 1000).toISOString();
const sessionRecord = { freeMinutes: 10, creditsPerMinute: 50, paidThrough: 12, timeCredits: 100, tokenCredits: 0, tokensUnpaid: 0, outOfCredits: false, filesAdded: 1, idleSeconds: 300 };
hosted.session = { ...hosted.running, mode: "interactive", holdCredits: 0, task: "", startedAt: sStarted, session: sessionRecord };
hosted.sessionEnded = {
  ...hosted.finished, mode: "interactive", holdCredits: 0, settledCredits: 104, startedAt: sStarted,
  session: { ...sessionRecord, tokenCredits: 4 },
  outputs: [...hosted.finished.outputs, { name: "receipts/transcript.md", bytes: 2210, path: `${H_DIR}/receipts/transcript.md` }],
};
const terms = { offered: true, available: true, freeMinutes: 10, creditsPerMinute: 50, minCredits: 50, idleSeconds: 300,
  transport: "Transport encrypted by the user key, messages not stored; the transcript comes back only in your sealed output." };
const waitingChat = hash.endsWith("-waiting");
const sessionEvents = [
  { seq: 1, kind: "user_turn", time: sAt(20), text: "Summarise INC-4821 from the last 24 hours." },
  { seq: 2, kind: "tool_call", time: sAt(30), text: "read_input" },
  { seq: 3, kind: "assistant_turn", time: sAt(80), text: "INC-4821 began at 02:14 UTC. <script>alert('x')</script><b onclick=\"steal()\">raw html</b> [open me](javascript:alert(1)) pay\u200Bpal" },
  ...(waitingChat ? [] : [
    { seq: 4, kind: "input_added", time: sAt(400), path: "/inputs/pager-export.csv" },
    { seq: 5, kind: "user_turn", time: sAt(410), text: "I added the pager export; does it change the timeline?" },
    { seq: 6, kind: "assistant_text", time: sAt(700), text: "Yes: the first page went out at 02:09, five minutes before" },
  ]),
];
const localEvents = [
  { kind: "user_turn", time: new Date().toISOString(), text: "Summarise INC-4821 from the last 24 hours." },
  { kind: "tool_call", time: new Date().toISOString(), text: "read_input" },
  { kind: "assistant_turn", time: new Date().toISOString(), text: "INC-4821 began at 02:14 UTC with elevated 5xx on the checkout service." },
  { kind: "usage", time: new Date().toISOString(), costUsd: 0.0182 },
];
hosted.lost = {
  ...hosted.running, final: true,
  downloadError: "lost touch with the registry while the run was running (read the run: the registry answered 503 (HTTP 503)); follow it again to see how it ends",
};
const started = Date.now() - 134_000;
const reports = [
  { runId: RUN_ID, ref: REF, startedAt: started, timeoutSeconds: 0, maxTurns: 0, turns: 0, inputTokens: 0, outputTokens: 0, prepared: false },
  { runId: RUN_ID, ref: REF, startedAt: started, timeoutSeconds: 900, maxTurns: 60, turns: 0, inputTokens: 0, outputTokens: 0, prepared: true },
  { runId: RUN_ID, ref: REF, startedAt: started, timeoutSeconds: 900, maxTurns: 60, turns: 7, inputTokens: 48_210, outputTokens: 3_904, prepared: true },
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

// The run page's entries.
const RP_DIR = "/home/chris/Downloads/agentry-out/demo1-incident-summarizer-7c1e4a90";
const rpRow = (over: Record<string, unknown> = {}) => ({
  id: RUN_ID, kind: "run", where: "here", registry: REG, ref: REF,
  startedAt: new Date(Date.now() - 50 * 60_000).toISOString(), endedAt: new Date(Date.now() - 47 * 60_000).toISOString(),
  outcome: "ok", tone: "ok", kept: true, source: "local", ...over,
});
function recordedRun() {
  switch (hash) {
    case "runpage-gone":
      return { run: rpRow({ outcome: "error", exitCode: 3, tone: "bad" }), folder: RP_DIR, folderState: "gone", folderError: "the run's folder is no longer there: it was deleted or moved" };
    case "runpage-draft-error":
    case "runpage-draft":
      return { run: rpRow({ kind: "draft", ref: "" }), folder: "/home/chris/Downloads/agentry-out/agentry-drafter-0192a3", folderState: "present",
        resultJson: JSON.stringify("Drafted demo1/csv-quality-check: three tools, one intake question.") };
    case "runpage-review":
      return { run: rpRow({ kind: "review", ref: "" }), folder: "/home/chris/Downloads/agentry-out/agentry-examiner-5e1f0004", folderState: "present", resultJson: "" };
    case "runpage-knowledge":
      return { run: rpRow({ kind: "knowledge", ref: "", outcome: "stopped", tone: "warn" }), folderState: "none" };
    default:
      return { run: rpRow(), folder: RP_DIR, folderState: "present", resultJson: result.resultJson };
  }
}
function hostedRecord(kept: boolean) {
  if (hash === "runpage-hosted-draft" || hash === "runpage-hosted-draft-error") {
    return { run: { ...hosted.finished, ref: "agentry/drafter@1.8.0", outputs: [], resultJson: "", outDir: "" }, kind: "draft", kept: false, folderState: "none", downloadable: false, followed: false };
  }
  if (hash === "runpage-hosted-expired") {
    return { run: { ...hosted.finished, outDir: "", resultJson: "", outputs: [] }, kind: "run", kept: false, folderState: "none", downloadable: false, followed: false,
      noOutput: "the registry holds no output for this run: it wrote none, or the 30 days it is kept have passed" };
  }
  return kept
    ? { run: hosted.finished, kind: "run", kept: true, folderState: "present", downloadable: false, followed: false }
    : { run: { ...hosted.finished, outDir: "", resultJson: "", outputs: [{ name: "output.tar", bytes: 4096 }] }, kind: "run", kept: false, folderState: "none", downloadable: true, followed: false };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const listeners: Record<string, ((v: any) => void)[]> = {};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const fire = (name: string, v: any) => (listeners[name] ?? []).forEach((cb) => cb(v));
let stop: (() => void) | null = null;
g.go = { app: {
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
    // The star ask after a run In Agentry that ended well, of someone
    // else's QuickAgent not yet starred.
    StarAsk: async () => ({ ask: true, name: "demo1/incident-summarizer" }),
    StarQuickAgent: async () => {},
    StartHostedRun: async () => {
      const first = hash === "hosted-queued" ? hosted.queued : hosted.running;
      if (hash === "hosted-finished" || hash === "hosted-failed" || hash === "hosted-lost" || hash === "hosted-hostile") {
        setTimeout(() => fire("runner:hosted-run", hosted[hash.slice("hosted-".length)]), 120);
      }
      return { run: first };
    },
    AttachHostedRunAtStart: async () => {
      if (hash === "hosted-card-finished") setTimeout(() => fire("runner:hosted-run", hosted.finished), 120);
      return { run: hosted.running };
    },
    StartSession: async () => {
      setTimeout(() => localEvents.forEach((ev) => fire("runner:session-event", ev)), 40);
      setTimeout(() => fire("runner:run-log", "session 5b18c7d1: demo1/incident-summarizer@1.2.0 against openai qwen3.8-flash-next"), 30);
      if (hash === "chat-local-ended") setTimeout(() => fire("runner:session-event", { kind: "session_end", reason: "idle", time: new Date().toISOString() }), 120);
      return {};
    },
    SendTurn: () => new Promise(() => {}),
    EndSession: async () => ({ ...result, runId: "5b18c7d1" }),
    ReplayHostedSession: async () => {
      fire("runner:hosted-session-state", { status: "running", startedAt: sStarted, session: sessionRecord });
      sessionEvents.forEach((ev) => fire("runner:hosted-session-event", ev));
      if (hash === "chat-hosted-ended") setTimeout(() => fire("runner:hosted-run", hosted.sessionEnded), 60);
    },
    SendHostedTurn: async () => undefined,
    EndHostedSession: async () => undefined,
    CancelHostedRun: async () => ({ ...hosted.running, cancelRequested: true, statusLine: "cancelling" }),
    ForgetRun: async () => undefined,
    RunOutputs: async () => outputs,
    ShowRunFolder: async () => undefined,
    // The run page's reads: the history's entry, and a
    // run In Agentry read live; Download output answers as Go does once the
    // output is down.
    RecordedRun: async () => recordedRun(),
    HostedRecord: async () => hostedRecord(hash === "runpage-hosted-kept"),
    DownloadHostedOutput: async () => { document.body.dataset.downloaded = "1"; return hostedRecord(true); },
    RemoveRecordedRun: async () => { document.body.dataset.removed = "1"; },
    // A review's report as Go reads it back from its folder.
    RecordedReview: async () => ({
      runId: RUN_ID, outDir: "/home/chris/Downloads/agentry-out/agentry-examiner-5e1f0004", ref: "me/release-notes@0.1.0", files: 4,
      outcome: "published", outcomeWords: "published", reason: "It has no finding the gate weighs, so the gate would be expected to publish it.",
      assumes: "a public version, as the manifest rules check it", policyFromRegistry: true, publishesUpTo: "low", aiCounted: true,
      examinerModel: "openai/qwen3.8-flash-next", notice: "The registry scans again at Submit, and its answer is the one that counts.",
      rules: { manifest: [], findings: [], bundleVersion: "0.3.2", bundleSource: "embedded", servers: [] },
      aiReview: { ran: true, model: "jev-1.13.0", rubricHash: "sha256:9c1e", findings: [], files: 4, notRead: 0 },
      reading: { summary: "Read the 4 files.", findings: [], notRead: [], withheld: [] },
      notRunHere: ["the registry's secrets stage (S2)"],
    }),
  },
  BuildAPI: {
    OpenRecordedDraft: async (id: string) => {
      if (id !== RUN_ID) throw new Error("wrong recorded draft selected");
      if (hash.endsWith("-error")) throw new Error("The draft's output is no longer available.");
      return selectedDraft();
    },
    OpenHostedDraft: async (registry: string, id: string) => {
      if (registry !== REG || id !== H_ID) throw new Error("wrong hosted draft selected");
      if (hash.endsWith("-error")) throw new Error("The draft's output is no longer available.");
      return selectedDraft();
    },
    ReviewStatus: async () => ({ key: { ready: false }, modelName: "harness" }),
    NewModel: async () => ({ name: "unrelated-blank-form", guide: {} }),
    Check: async () => ({ ok: true, problems: {}, fragments: {}, public: "" }),
    Summary: async (m: { name: string; summary: string; instructions: string }) => ({
      name: m.name, ref: "demo1/" + m.name + "@0.1.0", summary: m.summary, steps: m.instructions, tools: [], hosts: [], keys: [],
      servers: [], websites: [], grants: [], knowledge: [], overview: "", anyUnnamed: false,
    }),
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
import HostedRunFlow from "../src/pages/HostedRunFlow";
import ChatFlow from "../src/pages/ChatFlow";
import DraftFlow from "../src/pages/DraftFlow";
import { attachHostedRun, cancelHostedRun, followHostedChat, sendChat, startHostedRun, startLocalChat, startLocalRun, stopLocalRun } from "../src/runs";
import { startDraft } from "../src/drafts";
import { CornerHost } from "../src/progress";
import RunPage from "../src/pages/RunPage";
import Build from "../src/pages/Build";
import { stageRecordedDraft } from "../src/drafts";

// The run page under the shell, Home lit; where Back
// and a built-in's button lead is marked on the body (data-open).
function selectedDraft() {
  return {
    model: { schemaVersion: "2", namespace: "demo1", name: "csv-quality-check", version: "0.1.0", summary: "Checks the selected CSV draft.",
      instructions: "Read the selected draft's CSV and report its columns.", instructionFile: "AGENT.md", modes: ["autonomous"],
      license: "Apache-2.0", tags: [], dataAccess: ["files"], mcpServers: [], secrets: [], requiredSecrets: [], execCommands: [],
      egressMode: "none", egressHosts: [], fsRead: "none", fsReadPaths: [], fsWrite: "none", fsWritePaths: [],
      intake: [], guide: { overview: "", prepare: [], files: "", task: "", connect: [] } },
    notes: "Notes retained from the selected run.", decodeNotes: [], matches: [], sources: [], openQuestions: [], problems: [], unmentioned: [],
    check: { rubric: "flagged", draft: "pass", findings: [{ id: "C3_undeclared_capability", question: "Undeclared capability", score: 2, path: "AGENT.md" }], flagged: [], checks: 1 },
    aiReview: { scan: "registry", read: 5, passed: 5, withheld: 0, notScanned: 0, withheldSources: [] },
  } as unknown as import("../wailsjs/go/models").app.DraftView;
}
function RunPagePreview() {
  const [opened, setOpened] = useState(false);
  return (
    <div data-preview="runpage" className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page={opened ? "build" : "run"} />
        <div className="foot"><div>v0.6.75</div><div className="nowrap">{HOST}</div></div>
      </div>
      <div className="content">
        {opened ? <Build registry={REG} openDraftRev={1} /> : <RunPage source={hash.startsWith("runpage-hosted") ? "hosted" : "local"} id={hash.startsWith("runpage-hosted") ? H_ID : RUN_ID} registry={REG}
          backLabel="← back to Home" onBack={() => { document.body.dataset.open = "home"; }}
          onOpenPlace={(page) => { document.body.dataset.open = page; }}
          onOpenDraft={(draft) => { stageRecordedDraft(draft); document.body.dataset.open = "build"; setOpened(true); }} />}
      </div>
    </div>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="library" />
        <div className="foot"><div>v0.6.29</div><div className="nowrap">{HOST}</div></div>
      </div>
      <div className="content">
        <h1>Library</h1>
        <p className="muted">{children}</p>
      </div>
    </div>
  );
}

function Preview() {
  const [corner, setCorner] = useState<HTMLElement | null>(null);
  const card = hash === "run-card" || hash === "run-stopped" || hash === "run-card-stacked" || hash === "hosted-card-stacked";
  return (
    <CornerHost.Provider value={corner}>
      <div data-preview="runs">
        <Shell>The page stays usable while the run goes on in its card.</Shell>
        <RunFlow onBackToSetup={() => {}} initialMinimized={card} />
        <HostedRunFlow onBackToSetup={() => {}} initialMinimized={hash === "hosted-card-stacked"} />
        <ChatFlow onBackToSetup={() => {}} initialMinimized={hash.endsWith("-working") || hash.endsWith("-waiting") || hash === "chat-local-ended"} />
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
const setup = { panel: { registry: REG, ns: "demo1", name: "incident-summarizer", version: "1.2.0", attestation: null }, form: { ...form, mode: "hosted" } };
const chatSetup = { ...setup, form: { ...form, mode: hash.startsWith("chat-hosted") ? "hosted-session" : "interactive" } };
if (hash.startsWith("chat-local")) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  startLocalChat(chatSetup, REF, "", "", () => g.go.app.RunAPI.StartSession() as Promise<any>);
  if (hash === "chat-local-working") setTimeout(() => void sendChat("Does the pager export change the timeline?").catch(() => {}), 200);
}
if (hash.startsWith("chat-hosted")) followHostedChat(chatSetup, hosted.session, terms as never, "", "");
if (hash === "hosted-card" || hash === "hosted-card-finished") void attachHostedRun(REG);
else if (hash.startsWith("hosted-") && !hash.startsWith("chat-")) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  void startHostedRun(setup, () => g.go.app.RunAPI.StartHostedRun() as Promise<any>).then(() => {
    if (hash === "hosted-cancel") void cancelHostedRun();
  });
}
if (!hostedOnly) void startLocalRun(
  { panel: { registry: REG, ns: "demo1", name: "incident-summarizer", version: "1.2.0", attestation: null }, form },
  REF,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  () => g.go.app.RunAPI.RunAgent() as Promise<any>,
).then(() => {
  if (hash === "run-stopped") setTimeout(() => void stopLocalRun(), 120);
});
if (hash === "run-card-stacked") startDraft(REG, "agentry", "Check a CSV file I add for missing values, duplicate rows and mixed formats.", []);
createRoot(document.getElementById("root")!).render(runPage ? <RunPagePreview /> : <Preview />);
