// The Knowledge panel's states (Local and Cloud Knowledge, Create knowledge,
// Add documents, Preview), by location.hash, under the shell chrome, with
// the bindings mocked (the pattern pages.tsx and modals.tsx use). Not
// shipped.
//
//   #knowledge            signed in: the start page's two cards, Local
//                         Knowledge and Cloud Knowledge, with their lines
//   #knowledge-local      the Local Knowledge page: a pack kept here only
//                         (not scanned, opens offline), one in both places
//                         (scanned in Agentry, 2 findings), one changed since
//                         it was copied, one past Agentry's cap, and acme's
//                         shared pack kept here
//   #knowledge-cloud      the Cloud Knowledge page: the account's packs (one
//                         also here, one changed, one in Agentry only, one
//                         being scanned), the allocation, and acme's shared
//                         packs in their group
//   #knowledge-signed-out not signed in, on the Local page: a pack under this
//                         computer's key
//   #knowledge-signed-out-start  not signed in: the cards
//   #knowledge-empty      no pack yet: the cards say so
//   #knowledge-empty-cloud  no pack in the account: Create knowledge
//   #create-documents | #create-name | #create-review | #create-blocked
//                         Create knowledge's steps (blocked: docker not ready)
//   #create-documents-large  documents of 48 MB: the warning that the build
//                         can take a very long time and the pack stays here
//   #create-documents-huge   documents past 1 GB: Next refused
//   #hosted-where-large   the Where step with 48 MB: In Agentry not choosable
//   #create-building      the build under way: time, turns, pages written
//                         and tokens over its folded log
//   #create-done          what the build made
//   #create-stopped-brief a brief-only build that stopped at its turn budget:
//                         continued with the same brief, never documents
//   #hosted-stopped-brief the same In Agentry: Continue starts from this
//                         brief
//   #card-building        a build shrunk to its corner card over the page,
//                         its progress counting up
//   #card-done            the card once the build is over: how it ended
//   #card-hosted          a build In Agentry in its card
//   #add-review           Add documents' review
//   #hosted-where-pro     the Where step for a Standard account: In Agentry
//                         not choosable, the Pro sentence and See Pro
//   #hosted-where-pro-cloud  the same, opened from the Cloud page: On this
//                         computer selected
//   #create-done-cloud    a pack made here from the Cloud page: Copy to Cloud
//   #preview              a pack's index and pages
//   #preview-hostile      a page with raw HTML, a script, an image, a
//                         javascript: link and a right-to-left override
//   #preview-empty        a pack a build left short: its index and no page,
//                         the list answered as null
//   #render-fault         a page that throws while rendering, caught by
//                         Recover under a sidebar that still works
import { createRoot } from "react-dom/client";
import "../src/style.css";
import "../src/themes.css";
import { Wordmark } from "../src/ui";
import { SidebarNav } from "../src/nav";

const hash = window.location.hash.replace(/^#/, "") || "knowledge";
const signedOut = hash.startsWith("knowledge-signed-out");
const view = hash === "knowledge-local" || hash === "knowledge-signed-out" ? "local" : hash === "knowledge-cloud" || hash === "knowledge-empty-cloud" ? "cloud" : "start";
const REG = "https://secureagentry.net";
const HOST = "secureagentry.net";

const packs = signedOut
  ? [
    { name: "field-notes", id: "kp_1f2e3d4c5b6a79881f2e3d4c5b6a7988", pages: 12, bytes: 184_320, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, changed: false, where: "this computer", updated: "2026-10-02T08:30:00Z",
      key: "this computer's key, in its keychain (not portable; rewrapped to your account at your next sign-in)", wrapper: "machine", opensOffline: true },
  ]
  : hash.startsWith("knowledge-empty") ? [] : [
    { updated: "2026-10-04T14:02:00Z", name: "handbook", id: "kp_0a1b2c3d4e5f60718293a4b5c6d7e8f9", pages: 48, bytes: 2_411_724, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, changed: false, where: "this computer",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: true },
    { updated: "2026-09-30T10:00:00Z", agentryUpdated: "2026-09-30T10:05:00Z", agentryScan: "scanned in Agentry, 2 findings", agentryScanState: "scanned", agentryPages: 131, agentryBytes: 6_902_112, name: "incident-runbooks", id: "kp_9f8e7d6c5b4a39281f0e9d8c7b6a5948", agentryId: "3c7e1a52-5f7b-4f0e-9d0c-2b8a1f6e4d11", pages: 131, bytes: 6_902_112, scan: "scanned in Agentry, 2 findings", scanState: "scanned", local: true, inAgentry: true, changed: false, where: "this computer and Agentry",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: false },
    { updated: "2026-10-03T16:40:00Z", agentryUpdated: "2026-09-28T09:00:00Z", agentryScan: "scanned in Agentry, 0 findings", agentryScanState: "scanned", agentryPages: 7, agentryBytes: 81_200, name: "pricing-2026", id: "kp_5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d", agentryId: "8d2f6a90-1b3c-4e5d-8f7a-6b5c4d3e2f10", pages: 9, bytes: 96_400, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: true, changed: true, where: "this computer and Agentry",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: true },
    { agentryUpdated: "2026-09-27T12:00:00Z", agentryScan: "scanned in Agentry, 0 findings", agentryScanState: "scanned", agentryPages: 27, agentryBytes: 1_203_000, name: "supplier-contracts", agentryId: "b1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f", pages: 27, bytes: 1_203_000, scan: "scanned in Agentry, 0 findings", scanState: "scanned", local: false, inAgentry: true, changed: false, where: "Agentry", opensOffline: false },
    { updated: "2026-09-21T07:00:00Z", name: "scanned-archive", id: "kp_7e6d5c4b3a2918f7e6d5c4b3a2918f7e", pages: 212, bytes: 231_000_000, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, changed: false, where: "this computer",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: true },
    { agentryUpdated: "2026-10-04T15:10:00Z", agentryScan: "being scanned in Agentry", agentryScanState: "scanning", agentryPages: 14, agentryBytes: 402_000, name: "vendor-faq", agentryId: "c2e3f4a5-b6c7-4d8e-9f0a-1b2c3d4e5f60", pages: 14, bytes: 402_000, scan: "being scanned in Agentry", scanState: "scanning", local: false, inAgentry: true, changed: false, where: "Agentry", opensOffline: false },
    // acme's shared knowledge one kept here too, one in Agentry only.
    { name: "acme-runbook", id: "kp_2b3c4d5e6f708192a3b4c5d6e7f80912", agentryId: "d3e4f5a6-b7c8-4d9e-8f0a-1b2c3d4e5f61", pages: 22, bytes: 512_000, scan: "scanned in Agentry, 0 findings", scanState: "scanned", local: true, inAgentry: true, changed: false, where: "this computer and Agentry",
      key: `acme's organization key at ${HOST}, which Agentry holds; it opens the pack's key only for the organization's current members`, wrapper: "registry", registry: HOST, opensOffline: false,
      shared: true, org: "acme", exportable: false, canExport: false, updated: "2026-10-01T11:00:00Z", agentryUpdated: "2026-09-29T11:00:00Z",
      agentryScan: "scanned in Agentry, 0 findings", agentryScanState: "scanned", agentryPages: 22, agentryBytes: 512_000 },
    { name: "acme-policies", agentryId: "e4f5a6b7-c8d9-4e0f-9a1b-2c3d4e5f6a72", pages: 40, bytes: 1_048_576, scan: "scanned in Agentry, 1 finding", scanState: "scanned", local: false, inAgentry: true, changed: false, where: "Agentry",
      opensOffline: false, shared: true, org: "acme", exportable: true, canExport: true, agentryUpdated: "2026-10-02T15:00:00Z",
      agentryScan: "scanned in Agentry, 1 finding", agentryScanState: "scanned", agentryPages: 40, agentryBytes: 1_048_576 },
  ];
const list = {
  packs, registry: REG, signedIn: !signedOut,
  agentry: signedOut ? `Not signed in to ${HOST}, so its copies in Agentry are not listed.` : "",
};

// The account's allocation, as Settings › Profile reads it.
const allocation = { enabled: true, bytes: 8_701_000, max: 100 << 20 };

const hostile = hash === "preview-hostile";
const index = {
  path: "wiki/index.md", title: "handbook",
  markdown: "# handbook\n\n48 pages.\n\n## Topics\n\n- [Buckets](topics/buckets.md) — naming and regions\n- [Retention](topics/retention.md) — how long each kind of record is kept\n\n## Procedures\n\n- [Rotating a key](procedures/rotate-a-key.md) — the four steps, with who approves each\n",
};
const page = hostile
  ? {
    path: "wiki/topics/buckets.md", title: "Buckets ‮exe.txt",
    markdown: "Bucket names are unique <b onclick=\"alert(1)\">everywhere</b>.\n\n<script>alert('x')</script>\n\n![a tracking pixel](https://evil.example/pixel.png)\n\n[click me](javascript:alert(1)) and [the docs](https://example.com/docs)\n",
  }
  : {
    path: "wiki/topics/buckets.md", title: "Buckets", summary: "Naming and regions",
    markdown: "A bucket name is globally unique across every account [src: doc:storage-guide.pdf].\n\n## Naming\n\nLowercase letters, digits and hyphens, 3 to 63 characters. See the [provider's page](https://example.com/storage/naming).\n",
  };
const preview = hash === "preview-empty" ? { name: "handbook", index: { ...index, markdown: "# handbook\n\n0 pages.\n" }, pages: null } : {
  name: "handbook", index,
  pages: [
    { path: "wiki/procedures/rotate-a-key.md", title: "Rotating a key" },
    { path: "wiki/topics/buckets.md", title: hostile ? "Buckets ‮exe.txt" : "Buckets" },
    { path: "wiki/topics/retention.md", title: "Retention" },
  ],
};
const result = {
  name: "handbook", pages: 48, bytes: 2_411_724, documents: ["storage-guide.pdf", "handbook/retention.docx", "handbook/keys.md"],
  skipped: ["handbook/scan-0042.heic"], dropped: [{ path: "wiki/topics/untitled.md", reason: "the page has no title in its frontmatter" }],
  key: `your account's key at ${HOST}, which Agentry holds`, inputTokens: 184_210, outputTokens: 41_088, costUsd: 0.1893,
};
// A build given only a brief, stopped at its turn budget with what it wrote
// kept.
const stoppedBrief = {
  name: "espresso", pages: 17, bytes: 96_400, documents: ["brief.md"], brief: true,
  stopped: "turn budget exceeded (60 tool calls)",
  key: `your account's key at ${HOST}, which Agentry holds`, inputTokens: 96_120, outputTokens: 22_410, costUsd: 0.0912,
};
const buildLog = [
  "The Knowledge Creator runs on this computer against the model your runner policy names (anthropic claude-sonnet-4-5). It costs no credits, and nothing it reads or writes is scanned.",
  "decode: storage-guide.pdf — pdftotext, 38 pages of text",
  "decode: handbook/retention.docx — pandoc",
  "decode: handbook/scan-0042.heic — no decoder reads it",
  "run 20260925T221004Z-3f9a: agentry/knowledge-creator (builtin) against anthropic claude-sonnet-4-5",
  "tool: knowledge_list handbook",
  "tool: write_report wiki/topics/buckets.md",
];

// In Agentry: the registry's answer and a build's
// states, as the Go side reports them. Offered only on the #hosted-* states,
// so the local flow's shots keep their three steps.
const hostedState = hash.startsWith("hosted-");
const availability = {
  offered: hostedState, available: hostedState, holdCredits: 100, scanCredits: 1, timeoutSeconds: 3600, maxTurns: 300,
  balance: 494, balanceKnown: true, unmetered: false, maxInputFiles: 20, maxInputBytes: 10 << 20,
  // A registry that takes a brief alone, and the web.
  takesBrief: true, maxBriefBytes: 8192, web: true,
  webNote: "Searching the web can raise the credits a build uses: each search costs 1 credit, inside the build's hold. It is on by default for a new pack with no documents, and off with them.",
  ...(hash.startsWith("hosted-where-pro")
    ? {
        available: false,
        reason: "Creating knowledge in Agentry is part of Pro, Business and Enterprise.",
        remedy: "Create it on this computer with your own model provider, for no credits, then Copy to Cloud, where its scan is 1 credit; or upgrade to Pro.",
        upgradeUrl: REG + "/settings/subscription?upgrade=pro",
      }
    : {}),
};
const hostedRunning = {
  id: "01a0de6e-c1fa-70fe-80ec-8b2cbba957df", name: "handbook", status: "running", state: "running",
  statusLine: "The build is running on a sandbox host.", cancelRequested: false, holdCredits: 100, settledCredits: 0,
  settled: false, scanCredits: 0, applied: false, pages: 0, bytes: 0, turns: 0,
  documents: [{ name: "storage-guide.pdf", bytes: 412000 }, { name: "buckets.md", bytes: 3100 }], canContinue: false,
  downloaded: false, final: false,
};
// A brief alone: the #*-brief states, and #*-noweb
// where this computer has no search key.
const briefState = hash.includes("-brief") || hash.endsWith("-noweb");
const brief = "How to look after the café's espresso machine, for new staff: daily cleaning, descaling, grind and dose, and what to do when a shot runs fast.";
const localWeb = hash.endsWith("-noweb")
  ? { ready: false, provider: "Brave", why: "no Brave key was found: add one under Settings → Provider settings → Web search" }
  : { ready: true, provider: "Brave" };
const hostedDone = {
  ...hostedRunning, status: "finished", state: "done — pass", statusLine: "", outcome: "succeeded",
  outcomeLine: "The Knowledge Creator finished.", outcomeTone: "ok", settledCredits: 2, settled: true, scanCredits: 1,
  applied: true, pages: 4, bytes: 5300, turns: 10, downloaded: true,
  kept: "Downloaded handbook: 4 pages, kept on this computer, encrypted to your account's key, which Agentry holds.",
  scanState: "scanned", scan: "scanned, 1 finding (medium)", final: true,
  ...(briefState
    ? {
        documents: [], brief, name: "espresso-care", pages: 20, bytes: 41200, holdCredits: 201, settledCredits: 126,
        kept: "Downloaded espresso-care: 20 pages, kept on this computer, encrypted to your account's key, which Agentry holds.",
        scan: "scanned, no findings",
        webLine: "This build searched the web 17 times, and the registry let through 16 page reads. AI Review read 16 texts from the web before the Creator did, and withheld none.",
      }
    : {}),
};

// A brief-only build In Agentry stopped at its turn limit: Continue starts
// again from this brief, never documents it did not have.
const hostedStoppedBrief = {
  ...hostedDone, state: "done — budget", outcome: "budget", outcomeLine: "Stopped at its turn or cost limit.", outcomeTone: "warn",
  canContinue: true, pages: 11, bytes: 22_600, settledCredits: 201,
  kept: "Downloaded espresso-care: 11 pages, kept on this computer, encrypted to your account's key, which Agentry holds.",
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
g.go = { app: {
  KnowledgeAPI: {
    List: async () => list,
    CreatorModel: async () => "anthropic claude-sonnet-4-5",
    CreatorBudget: async () => ({ timeoutSeconds: 3600, maxTurns: 60 }),
    DocumentsSize: async () => ({
      bytes: hash === "create-documents-huge" ? 1_610_612_736 : hash.endsWith("-large") ? 50_331_648 : 2_411_724,
      localMaxBytes: 1 << 30, agentryMaxBytes: 10 << 20,
    }),
    Preview: async () => preview,
    ReadPage: async () => page,
    Create: async () => new Promise(() => {}),
    Add: async () => new Promise(() => {}),
    Upload: async () => "Uploaded handbook to secureagentry.net. Agentry is scanning it; the scan's findings show here once it ends.",
    Download: async () => "Downloaded supplier-contracts.",
    DownloadShared: async () => "Downloaded acme-policies.",
    DeleteFromAgentry: async () => undefined,
    Export: async () => ({ dir: "/home/me/Documents/handbook" }),
    Delete: async () => undefined,
    HostedAvailability: async () => availability,
    AttachHostedBuild: async () => ({}),
    StartHostedBuild: async () => new Promise(() => {}),
    ContinueHostedBuild: async () => new Promise(() => {}),
    CancelHostedBuild: async () => hostedRunning,
    StopBuild: async () => undefined,
    CreatorWeb: async () => localWeb,
    // Go's check of the repositories and addresses, as it answers: an
    // address that is not https is refused, and a key is saved here for
    // github.com.
    CheckSources: async (s: { repos: string[]; urls: string[] }) => ({
      maxRepos: 10, maxUrls: 20,
      lines: [
        ...s.repos.map((a) => ({ address: a, kind: "repository", ...(a.startsWith("https://") ? { host: new URL(a).hostname, saved: a.includes("github.com") } : { problem: "a repository's address is https" }) })),
        ...s.urls.map((a) => ({ address: a, kind: "url", ...(a.startsWith("https://") ? { host: new URL(a).hostname } : { problem: "an address is https" }) })),
      ],
    }),
  },
  RunAPI: {
    SetupStatus: async () => ({ dockerOk: hash !== "create-blocked", dockerErr: hash === "create-blocked" ? "Cannot connect to the Docker daemon at unix:///var/run/docker.sock" : "", imagesReady: true, policyExists: true, provider: "anthropic", model: "claude-sonnet-4-5" }),
  },
  App: { ChooseFiles: async () => [], ChooseDirectory: async () => "", OpenExternal: async () => undefined },
  SettingsAPI: { Profile: async () => ({ user: {}, subscription: { knowledge: allocation } }) },
} };
// The build's log, streamed as the Go side streams it. The app's EventsOn
// goes through EventsOnMultiple (the wails runtime shim).
// The progress counts up as the Go side reports it: the start, then usage
// and pages.
const started = Date.now() - 754_000;
const progressReports = [
  { startedAt: started, timeoutSeconds: 3600, maxTurns: 60, turns: 0, inputTokens: 0, outputTokens: 0, pages: 0 },
  { startedAt: started, timeoutSeconds: 3600, maxTurns: 60, turns: 9, inputTokens: 101_400, outputTokens: 6_120, pages: 3 },
  { startedAt: started, timeoutSeconds: 3600, maxTurns: 60, turns: 14, inputTokens: 184_210, outputTokens: 12_044, pages: 5 },
];
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const subscribe = (name: string, cb: (v: any) => void) => {
  const live = hash === "create-building" || hash === "card-building";
  if (name === "knowledge:build-log" && live) setTimeout(() => buildLog.forEach((l) => cb(l)), 50);
  if (name === "knowledge:build-progress" && live) progressReports.forEach((p, i) => setTimeout(() => cb(p), 60 + 60 * i));
  return () => {};
};
g.runtime = {
  EventsOn: subscribe, EventsOnMultiple: subscribe, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import Knowledge from "../src/pages/Knowledge";
import { Recover } from "../src/Recover";
import KnowledgeCreate from "../src/pages/KnowledgeCreate";

// #render-fault: a page that throws while rendering, as the preview did on a
// null list, until Try again clears the fault.
let faulty = hash.startsWith("render-fault");
function MaybeBoom({ children }: { children: React.ReactNode }) {
  if (faulty) throw new TypeError("Cannot read properties of null (reading 'map')");
  return <>{children}</>;
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="knowledge" />
        <div className="foot"><div>v0.6.1</div><div className="nowrap">{HOST}</div></div>
      </div>
      <div className="content">{children}</div>
    </div>
  );
}

const docs = ["/home/me/Documents/storage-guide.pdf", "/home/me/Documents/handbook"];
const card = hash.startsWith("card-");
const create = hash.startsWith("create-") || hash === "add-review" || hostedState || card;
const step =
  hash === "create-name" ? "name"
  : hash === "hosted-where" || hash.startsWith("hosted-where-pro") || hash === "hosted-where-large" ? "where"
  : hash === "create-review" || hash === "create-blocked" || hash === "add-review" || hash === "hosted-review" || hash === "create-review-brief" || hash === "hosted-review-brief" || hash === "create-review-sources" ? "review"
  : "documents";

createRoot(document.getElementById("root")!).render(
  <div data-preview="knowledge">
    <Shell>
      <Recover subject="This page" onRecover={() => { faulty = false; }}>
        <MaybeBoom>
          <Knowledge
            registry={REG}
            onOpenSettings={() => {}}
            onCreate={() => {}}
            initialView={view}
            initialAllocation={signedOut ? null : allocation}
            initialPreview={hash.startsWith("preview") ? "handbook" : undefined}
          />
        </MaybeBoom>
      </Recover>
    </Shell>
    {create && (
      <KnowledgeCreate
        registry={REG}
        addTo={hash === "add-review" ? "handbook" : undefined}
        place={hash.endsWith("-cloud") ? "agentry" : undefined}
        onClose={() => {}}
        onPreview={() => {}}
        initialStep={step}
        initialDocuments={briefState ? [] : hash.startsWith("create-documents") || step !== "documents" ? docs : []}
        initialBrief={briefState ? brief : undefined}
        initialRepos={hash.endsWith("-sources") ? "https://github.com/google/uuid@v1.6.0" : undefined}
        initialUrls={hash === "create-documents-sources" ? "https://arxiv.org/pdf/1706.03762\nhttp://example.com/notes.txt" : hash.endsWith("-sources") ? "https://arxiv.org/pdf/1706.03762" : undefined}
        initialWeb={localWeb}
        initialName={hash === "add-review" ? undefined : "handbook"}
        minimized={card}
        onMinimize={() => {}}
        onRestore={() => {}}
        initialBuilding={hash === "create-building" || hash === "card-building"}
        initialResult={hash === "create-done" || hash === "create-done-cloud" || hash === "card-done" ? (result as never) : hash === "create-stopped-brief" ? (stoppedBrief as never) : undefined}
        initialAvailability={hostedState ? (availability as never) : undefined}
        initialWhere={hash === "hosted-review" || hash === "hosted-where" || hash === "hosted-review-brief" ? "agentry" : undefined}
        initialHosted={hash === "hosted-running" || hash === "card-hosted" ? (hostedRunning as never) : hash === "hosted-done" || hash === "hosted-done-brief" ? (hostedDone as never) : hash === "hosted-stopped-brief" ? (hostedStoppedBrief as never) : undefined}
      />
    )}
  </div>,
);
