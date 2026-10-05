// A draft's progress dialog and its corner card (agentry-desktop#410), by
// location.hash, over Describe it under the shell chrome, with the bindings
// mocked (the pattern knowledge.tsx uses). Not shipped.
//
//   #draft-local          a draft on this computer under way: the stage, time,
//                         turns and tokens over its folded log
//   #draft-agentry        a draft In Agentry under way: where it stands, the
//                         hold, Cancel the draft
//   #draft-landed         a draft that landed: its name and Open the draft
//   #draft-failed         a draft that ended with no draft: the error, the log
//   #draft-hostile        the same, with an error and log lines carrying raw
//                         HTML, a script, an image and a right-to-left override
//   #draft-refused        Agentry refused it for credits: the refusal and
//                         Credits and upgrade
//   #card-local           a draft on this computer shrunk to its corner card
//   #card-agentry         a draft In Agentry in its card
//   #card-landed          the card once a draft landed: Open the draft, Dismiss
//   #card-stacked         a draft In Agentry's card over a knowledge build's,
//                         in App's corner, neither covering the other
import { useState } from "react";
import { createRoot } from "react-dom/client";
import "../src/style.css";
import { Wordmark } from "../src/ui";
import { SidebarNav } from "../src/nav";

const hash = window.location.hash.replace(/^#/, "") || "draft-local";
const REG = "https://secureagentry.net";
const HOST = "secureagentry.net";
const inAgentry = hash.endsWith("-agentry") || hash === "draft-refused" || hash === "card-stacked";
const hostile = hash === "draft-hostile";

const hostedRunning = {
  id: "01a0e424-0000-0000-0000-000000000000", status: "running", state: "running",
  statusLine: "The Drafter is running on a sandbox host.", queuePosition: 0, cancelRequested: false, holdCredits: 41,
  creditsLine: "41 credits are held for this draft.", final: false,
};
const landedDraft = {
  where: "here", description: "Check a CSV file I add for missing values, duplicate rows and mixed formats.", hints: [],
  model: { name: "csv-quality-check" }, aiReview: { read: 6, passed: 6, withheld: 0, notScanned: 0, withheldSources: [] },
  unmentioned: [], problems: [], moreProblems: 0, decodeNotes: [], matches: [], sources: [], openQuestions: [], notes: "", outDir: "/home/me/Downloads/agentry-out/agentry-drafter-1",
};
const failure = hostile
  ? "the Drafter's answer was not a draft: <img src=x onerror=alert(1)><script>alert(2)</script> ‮txt.exe"
  : "the Drafter's answer was not a draft, so none was made; its answer and the run's receipts are in /home/me/Downloads/agentry-out/agentry-drafter-20260929T101500Z-7c1e: exit 6: the final message is not valid JSON";
const draftLog = [
  "AI Review passed the description. The Drafter runs on this computer against the model your runner policy names (openai qwen3.8-flash-next), for no credits.",
  "run 20260929T101500Z-7c1e: agentry/drafter (builtin) against openai qwen3.8-flash-next",
  "tool: registry_search csv quality",
  "tool: web_read https://frictionlessdata.io/ — AI Review: pass",
  ...(hostile ? ["tool: web_read <b onclick=alert(3)>https://evil.example/</b> ‮gnp.exe"] : []),
];

const started = Date.now() - 134_000;
const progressReports = [
  { startedAt: started, timeoutSeconds: 1800, maxTurns: 60, turns: 0, inputTokens: 0, outputTokens: 0, stage: "description" },
  { startedAt: started, timeoutSeconds: 1800, maxTurns: 60, turns: 0, inputTokens: 0, outputTokens: 0, stage: "drafting" },
  { startedAt: started, timeoutSeconds: 1800, maxTurns: 60, turns: 7, inputTokens: 48_210, outputTokens: 3_904, stage: "drafting" },
];
const local = hash === "draft-local" || hash === "card-local";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
g.go = { main: {
  BuildAPI: {
    DescribeStatus: async () => ({ key: { stored: true, source: "keyring", envName: "TYPESAFE_API_KEY", envSet: false, ready: true }, search: "no-provider",
      modelName: "openai qwen3.8-flash-next", searchSentence: "It has no web search, since your runner policy names no search provider, so it reads the registry and the pages it knows." }),
    DescribeInAgentryStatus: async () => ({ offered: true, available: true, signedIn: true, holdCredits: 41,
      holdSentence: "This draft holds 41 credits and settles from what it uses.", inputsSentence: "Your description and the Drafter's searches go to a search provider, to AI Review and to the sites the Drafter reads.",
      timeoutSeconds: 1200, maxTurns: 60, maxPages: 40, balance: 496, balanceKnown: true, unmetered: false, short: false }),
    AttachHostedDraft: async () => ({}),
    DescribeOnThisComputer: async () => {
      if (hash === "draft-landed" || hash === "card-landed") return landedDraft;
      if (hash === "draft-failed" || hostile) {
        await new Promise((r) => setTimeout(r, 150));
        throw failure;
      }
      return new Promise(() => {});
    },
    DescribeInAgentry: async () => hash === "draft-refused"
      ? { refusal: "credits-exhausted", detail: "This account has 12 credits; a draft holds 41." }
      : { draft: hostedRunning },
    CancelHostedDraft: async () => ({ ...hostedRunning, cancelRequested: true }),
    StopDraft: async () => undefined,
  },
  KnowledgeAPI: {
    // Describe it lists the packs a draft may read as it mounts
    // (agentry-desktop#497, knowledge-and-drafter.md K4); none here, so the form is the one the shots show.
    List: async () => ({ packs: [], registry: REG, signedIn: true }),
    CreatorModel: async () => "openai qwen3.8-flash-next",
    CreatorBudget: async () => ({ timeoutSeconds: 3600, maxTurns: 60 }),
    HostedAvailability: async () => ({ offered: false, available: false }),
    AttachHostedBuild: async () => ({}),
    DocumentsSize: async () => ({ bytes: 2_411_724, localMaxBytes: 1 << 30, agentryMaxBytes: 10 << 20 }),
    StopBuild: async () => undefined,
    CreatorWeb: async () => ({ ready: true, provider: "Brave" }),
  },
  RunAPI: { SetupStatus: async () => ({ dockerOk: true, dockerErr: "", imagesReady: true, policyExists: true, provider: "openai", model: "qwen3.8-flash-next" }) },
  SettingsAPI: { OpenUpgrade: async () => undefined },
  App: { ChooseFiles: async () => [], ChooseDirectory: async () => "", OpenExternal: async () => undefined },
} };
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const subscribe = (name: string, cb: (v: any) => void) => {
  if (name === "build:draft-log" && (local || hash === "draft-failed" || hostile)) setTimeout(() => draftLog.forEach((l) => cb(l)), 30);
  if (name === "build:draft-progress" && local) progressReports.forEach((p, i) => setTimeout(() => cb(p), 40 + 40 * i));
  if (name === "knowledge:build-progress" && hash === "card-stacked") {
    setTimeout(() => cb({ startedAt: Date.now() - 754_000, timeoutSeconds: 3600, maxTurns: 60, turns: 14, inputTokens: 184_210, outputTokens: 12_044, pages: 5 }), 40);
  }
  return () => {};
};
g.runtime = {
  EventsOn: subscribe, EventsOnMultiple: subscribe, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import { DescribePanel } from "../src/pages/BuildDescribe";
import DraftFlow from "../src/pages/DraftFlow";
import KnowledgeCreate from "../src/pages/KnowledgeCreate";
import { startDraft } from "../src/drafts";
import { CornerHost } from "../src/progress";

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="build" />
        <div className="foot"><div>v0.6.22</div><div className="nowrap">{HOST}</div></div>
      </div>
      <div className="content">{children}</div>
    </div>
  );
}

function Preview() {
  const [corner, setCorner] = useState<HTMLElement | null>(null);
  return (
    <CornerHost.Provider value={corner}>
      <div data-preview="drafts">
        <Shell>
          <DescribePanel registry={REG} onDrafted={() => {}} />
        </Shell>
        <DraftFlow registry={REG} onOpenDraft={() => {}} initialMinimized={hash.startsWith("card-")} />
        {hash === "card-stacked" && (
          <KnowledgeCreate registry={REG} onClose={() => {}} onPreview={() => {}} minimized initialBuilding initialName="handbook"
            initialDocuments={["/home/me/Documents/handbook"]} onMinimize={() => {}} onRestore={() => {}} />
        )}
        <div className="corner-cards" ref={setCorner} />
      </div>
    </CornerHost.Provider>
  );
}

startDraft(REG, inAgentry ? "agentry" : "here", "Check a CSV file I add for missing values, duplicate rows and mixed formats.", []);
createRoot(document.getElementById("root")!).render(<Preview />);
