// The Knowledge panel's states (agentry-desktop#273, plan knowledge.md
// §4.8), by location.hash, under the shell chrome, with the bindings
// mocked (the pattern pages.tsx and modals.tsx use). Not shipped.
//
//   #knowledge            signed in: a pack kept here only (not scanned, opens
//                         offline), one in both places (scanned in Agentry,
//                         2 findings), one changed since its upload, one kept
//                         in Agentry only, and one Agentry is still scanning
//   #knowledge-signed-out not signed in: a pack under this computer's key
//   #knowledge-empty      no pack yet
//   #create-documents | #create-name | #create-review | #create-blocked
//                         Create knowledge's steps (blocked: docker not ready)
//   #create-building      the build under way, its log streaming
//   #create-done          what the build made
//   #add-review           Add documents' review
//   #preview              a pack's index and pages
//   #preview-hostile      a page with raw HTML, a script, an image, a
//                         javascript: link and a right-to-left override
import { createRoot } from "react-dom/client";
import "../src/style.css";
import { Wordmark } from "../src/ui";

const hash = window.location.hash.replace(/^#/, "") || "knowledge";
const signedOut = hash === "knowledge-signed-out";
const REG = "https://secureagentry.net";
const HOST = "secureagentry.net";

const packs = signedOut
  ? [
    { name: "field-notes", id: "kp_1f2e3d4c5b6a79881f2e3d4c5b6a7988", pages: 12, bytes: 184_320, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, changed: false, where: "this computer",
      key: "this computer's key, in its keychain (not portable; rewrapped to your account at your next sign-in)", wrapper: "machine", opensOffline: true },
  ]
  : hash === "knowledge-empty" ? [] : [
    { name: "handbook", id: "kp_0a1b2c3d4e5f60718293a4b5c6d7e8f9", pages: 48, bytes: 2_411_724, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: false, changed: false, where: "this computer",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: true },
    { name: "incident-runbooks", id: "kp_9f8e7d6c5b4a39281f0e9d8c7b6a5948", agentryId: "3c7e1a52-5f7b-4f0e-9d0c-2b8a1f6e4d11", pages: 131, bytes: 6_902_112, scan: "scanned in Agentry, 2 findings", scanState: "scanned", local: true, inAgentry: true, changed: false, where: "this computer and Agentry",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: false },
    { name: "pricing-2026", id: "kp_5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d", agentryId: "8d2f6a90-1b3c-4e5d-8f7a-6b5c4d3e2f10", pages: 9, bytes: 96_400, scan: "not scanned", scanState: "not-scanned", local: true, inAgentry: true, changed: true, where: "this computer and Agentry (changed here since its upload)",
      key: `your account's key at ${HOST}, which Agentry holds`, wrapper: "registry", registry: HOST, opensOffline: true },
    { name: "supplier-contracts", agentryId: "b1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f", pages: 27, bytes: 1_203_000, scan: "scanned in Agentry, 0 findings", scanState: "scanned", local: false, inAgentry: true, changed: false, where: "Agentry", opensOffline: false },
    { name: "vendor-faq", agentryId: "c2e3f4a5-b6c7-4d8e-9f0a-1b2c3d4e5f60", pages: 14, bytes: 402_000, scan: "being scanned in Agentry", scanState: "scanning", local: false, inAgentry: true, changed: false, where: "Agentry", opensOffline: false },
  ];
const list = {
  packs, registry: REG, signedIn: !signedOut,
  agentry: signedOut ? `Not signed in to ${HOST}, so its copies in Agentry are not listed.` : "",
};

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
const preview = {
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
const buildLog = [
  "The Knowledge Creator runs on this computer against the model your runner policy names (anthropic claude-sonnet-4-5). It costs no credits, and nothing it reads or writes is scanned.",
  "decode: storage-guide.pdf — pdftotext, 38 pages of text",
  "decode: handbook/retention.docx — pandoc",
  "decode: handbook/scan-0042.heic — no decoder reads it",
  "run 20260925T221004Z-3f9a: agentry/knowledge-creator (builtin) against anthropic claude-sonnet-4-5",
  "tool: knowledge_list handbook",
  "tool: write_report wiki/topics/buckets.md",
];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = window as any;
g.go = { main: {
  KnowledgeAPI: {
    List: async () => list,
    CreatorModel: async () => "anthropic claude-sonnet-4-5",
    Preview: async () => preview,
    ReadPage: async () => page,
    Create: async () => new Promise(() => {}),
    Add: async () => new Promise(() => {}),
    Upload: async () => "Uploaded handbook to secureagentry.net. Agentry is scanning it; the scan's findings show here once it ends.",
    Download: async () => "Downloaded supplier-contracts.",
    Export: async () => ({ dir: "/home/me/Documents/handbook" }),
    Delete: async () => undefined,
  },
  RunAPI: {
    SetupStatus: async () => ({ dockerOk: hash !== "create-blocked", dockerErr: hash === "create-blocked" ? "Cannot connect to the Docker daemon at unix:///var/run/docker.sock" : "", imagesReady: true, policyExists: true, provider: "anthropic", model: "claude-sonnet-4-5" }),
  },
  App: { ChooseFiles: async () => [], ChooseDirectory: async () => "", OpenExternal: async () => undefined },
} };
// The build's log, streamed as the Go side streams it. The app's EventsOn
// goes through EventsOnMultiple (the wails runtime shim).
const subscribe = (name: string, cb: (line: string) => void) => {
  if (name === "knowledge:build-log" && hash === "create-building") setTimeout(() => buildLog.forEach((l) => cb(l)), 50);
  return () => {};
};
g.runtime = {
  EventsOn: subscribe, EventsOnMultiple: subscribe, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {},
};

import Knowledge from "../src/pages/Knowledge";
import KnowledgeCreate from "../src/pages/KnowledgeCreate";

function Shell({ children }: { children: React.ReactNode }) {
  const nav = ["Browse", "Installed", "Knowledge", "Build", "Validate", "Settings"];
  return (
    <div className="shell" style={{ height: "auto", minHeight: "100vh" }}>
      <div className="sidebar">
        <div className="brand"><Wordmark sub="Desktop" /></div>
        {nav.map((n) => <button key={n} className={`nav-item ${n === "Knowledge" ? "active" : ""}`}>{n}</button>)}
        <div className="foot"><div>v0.6.1</div><div className="nowrap">{HOST}</div></div>
      </div>
      <div className="content">{children}</div>
    </div>
  );
}

const docs = ["/home/me/Documents/storage-guide.pdf", "/home/me/Documents/handbook"];
const create = hash.startsWith("create-") || hash === "add-review";
const step = hash === "create-name" ? "name" : hash === "create-review" || hash === "create-blocked" || hash === "add-review" ? "review" : "documents";

createRoot(document.getElementById("root")!).render(
  <div data-preview="knowledge">
    <Shell>
      <Knowledge registry={REG} onOpenSettings={() => {}} initialPreview={hash.startsWith("preview") ? "handbook" : undefined} />
    </Shell>
    {create && (
      <KnowledgeCreate
        registry={REG}
        addTo={hash === "add-review" ? "handbook" : undefined}
        onClose={() => {}}
        onPreview={() => {}}
        initialStep={step}
        initialDocuments={hash === "create-documents" || step !== "documents" ? docs : []}
        initialName={hash === "add-review" ? undefined : "handbook"}
        initialBuilding={hash === "create-building"}
        initialResult={hash === "create-done" ? (result as never) : undefined}
      />
    )}
  </div>,
);
