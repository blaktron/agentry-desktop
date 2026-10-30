// Build: author a QuickAgent as a form, save it as a real
// workspace (the same files `agentry init` writes), validate it with the
// registry's own scanners, and publish it through the full pipeline. The
// form is an editor for agent.yaml — the manifest stays the contract, and
// nothing here bypasses a publication gate.
//
// The tab opens on a start page, and the form has two views of one Model:
// Step by step, one section at a time on the step chrome in primitives.tsx,
// and Advanced, every section at once. Switching keeps everything. A step
// is checked before Next by BuildAPI.Check, which runs the form's rules and
// the core's own V-rules over the model without touching disk, and shows
// the lines the step writes once it passes. The folder is chosen at Save,
// so a person can start without one.
//
// The start page is two cards, as the site's /build: Describe it to AI,
// which opens Describe it, and Build it by hand, which opens Step by step
// and offers Advanced (BuildWays.tsx). Describe it makes a draft with the
// built-in Drafter (BuildDescribe.tsx), In Agentry or on this computer; the
// draft is decoded into the Model and lands on the summary
// (BuildSummary.tsx): how it works and what it needs, then Save, Install
// and Submit. The folder is made at its first save under the default
// output folder, with no dialog. Both form views end on the same summary
// through Continue to submit, so it is the one publish page, and Review
// keeps the Drafter's notes folded under "How it was drafted", with
// Describe again under them.
import { useEffect, useRef, useState, type ReactNode } from "react";
import * as BuildAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/buildapi";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import { ChooseDirectory, OpenExternal } from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/app";
import { EventsOn } from "../host";
import * as build from "../../bindings/github.com/blaktron/agentry-desktop/internal/build/models";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import * as mcpvuln from "../../bindings/github.com/blaktron/agentry-desktop/internal/mcpvuln/models";
import * as validate from "../../bindings/github.com/blaktron/agentry-desktop/internal/validate/models";
import { DescribeAgain, DescribePanel, DraftNotes } from "./BuildDescribe";
import { draftInFlight, takeLandedDraft } from "../drafts";
import { editingLine, noteHeldForm, sameRegistry, stillEditing, takeEditRequest, type Editing } from "../edit";
import { startReview } from "../reviews";
import { SummaryView } from "./BuildSummary";
import { RevisionView } from "./BuildRevision";
import { BuildShips } from "./BuildShips";
import { BUILD_TAGLINE, BuildWays } from "./BuildWays";
import { Button, Card, ErrorBox, Notice, PageHeader, Recap, Sev, Spinner, StepFoot, StepHeading, StepList, Textarea } from "../ui";
import { KeyHelpList } from "../KeyHelp";
import { advisoryWords } from "../advisory";
import { declaredKeys, optionalKeys, questionRequired, requiredKeys } from "../requiredKeys";
import { isRemote, reachRow, REMOTE_CHECK, serverRef } from "../remoteServers";
import { RevealText } from "../reveal";

const DATA_ACCESS = ["email", "calendar", "files", "tickets", "code", "messages", "payments", "other"];
// The instruction-file quota, as internal/build's maxInstructionBytes.
const MAX_INSTRUCTION_BYTES = 1 << 20;

// What the tab was holding when the person last left it. Build unmounts on
// every tab change, and the form is not a file until the first save, so
// this module keeps the model, the folder and the view for the rest of the
// app's session, and the start page offers it back as Continue.
let kept: { model: build.Model; dir: string; view: View; drafted: app.DraftView | null; editing: Editing | null; revisionShown: boolean; shipNotes: string[] } | null = null;

type View = "start" | "steps" | "advanced" | "describe" | "summary";
type SectionKey = "basics" | "instructions" | "tools" | "access" | "questions" | "guide" | "publish";
type StepKey = SectionKey | "review";

// The eight steps, each with what it is for in two sentences.
const STEPS: { key: StepKey; label: string; intro: string }[] = [
  { key: "basics", label: "Basics", intro: "Name the QuickAgent and say in one line what it does. Then choose how it runs." },
  { key: "instructions", label: "Instructions", intro: "The instruction file tells the agent its one job. Say what it is given, the steps it takes and what it returns." },
  { key: "tools", label: "Tools and web access", intro: "Say which sites it may reach and which MCP servers it uses, with the tools each may call. Everything is off until you list it." },
  { key: "access", label: "Files, commands and data", intro: "Say what else it may touch. The least answer is already chosen." },
  { key: "questions", label: "Questions", intro: "Questions a run asks before it starts, in either mode. Ask only what the run cannot find out for itself." },
  { key: "guide", label: "Before a run", intro: "What a person reads before they start a run: what it does and what to gather first. It is shown to people only, never to the agent." },
  { key: "publish", label: "Publishing", intro: "Where it is published, under which version and licence. Submit makes it public." },
  { key: "review", label: "Review", intro: "Check what it says it is and what it may reach, then save it, validate it or publish it." },
];

/** Comma-separated text input bound to a string array. */
function ListInput({
  value, onChange, placeholder, mono = true, label,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
  mono?: boolean;
  label?: string;
}) {
  return (
    <input
      type="text"
      className={mono ? "mono" : ""}
      style={{ width: "100%" }}
      placeholder={placeholder}
      aria-label={label}
      value={value.join(", ")}
      onChange={(e) => onChange(e.target.value.split(",").map((s) => s.trim()).filter(Boolean))}
    />
  );
}

/** One entry per line, bound to a string array. */
function LinesInput({ value, onChange, placeholder, rows = 3 }: { value: string[]; onChange: (v: string[]) => void; placeholder?: string; rows?: number }) {
  return (
    <textarea
      rows={rows}
      style={{ width: "100%" }}
      placeholder={placeholder}
      value={value.join("\n")}
      onChange={(e) => onChange(e.target.value.split("\n"))}
    />
  );
}

/** A summary as one sentence: its own full stop, never two. */
function sentence(s: string): string {
  const t = s.trim().replace(/[.\s]+$/, "");
  return t ? `${t}.` : "";
}

const stepOf = (key: StepKey) => STEPS.findIndex((s) => s.key === key);

// The registry's check of one server version in plain words. The status
// "safe" is never shown as a word: it means no known advisories at a date.
function checkSentence(c: mcpvuln.Status): { tone: "muted" | "high"; text: string } {
  const at = c.checkedAt ? new Date(c.checkedAt) : null;
  const when = at && !Number.isNaN(at.getTime()) ? ` as of ${at.toISOString().slice(0, 16).replace("T", " ")} UTC` : "";
  switch (c.status) {
    case "vulnerable": {
      // Weighed at the gate's rating: a run refuses only an
      // advisory the gate rates high or critical, so only that is "high".
      const advs = c.advisories ?? [];
      const n = advs.length;
      const list = advs.map((a) => `${a.id} ${advisoryWords(a)}`).join(", ");
      return { tone: c.refuses ? "high" : "muted", text: `${n} known ${n === 1 ? "advisory" : "advisories"}: ${list}.${c.refuses ? " A run of a release that declares it is refused, except in Unsafe Mode with your consent." : ""}` };
    }
    case "safe":
      return { tone: "muted", text: `No known advisories${when}.` };
    case "error":
      return { tone: "muted", text: "The registry's last check of this version failed; it tries again on its next sweep." };
    case "unknown":
      return { tone: "muted", text: "No check on record yet. The registry checks this version once a submission declares it." };
    default:
      return { tone: "muted", text: `The registry did not answer${c.error ? ` (${c.error})` : ""}.` };
  }
}

type PatchRow = (row: Partial<build.MCPServerRow>) => void;

// A remote server's row: its
// address, how it signs in, and the scopes or the key that goes with that.
// The Go model writes only these (MCPServerRow.Declared), so a package's
// fields a source switch hides are kept for switching back, never written.
function RemoteServerRow({ srv, patch, source, remove }: { srv: build.MCPServerRow; patch: PatchRow; source: ReactNode; remove: ReactNode }) {
  return (
    <div className="card" style={{ marginTop: 8 }}>
      <div className="row">
        {source}
        <label className="small grow">address
          <input type="text" className="mono" style={{ width: "100%" }} placeholder="https://mcp.example.com/mcp"
            value={srv.url ?? ""} onChange={(e) => patch({ url: e.target.value })} />
        </label>
        <label className="small">signs in with
          <select value={srv.auth ?? ""} onChange={(e) => patch({ auth: e.target.value })}>
            <option value="">choose</option>
            <option value="oauth">OAuth</option>
            <option value="secret">a key</option>
            <option value="none">nothing</option>
          </select>
        </label>
        {remove}
      </div>
      <div className="row" style={{ marginTop: 6 }}>
        {srv.auth === "oauth" && (
          <label className="small grow">scopes it asks for (empty = the vendor&apos;s)
            <ListInput value={srv.scopes ?? []} onChange={(v) => patch({ scopes: v })} placeholder="read, write" />
          </label>
        )}
        <label className="small grow">tools it may call (empty = all it offers)
          <ListInput value={srv.tools ?? []} onChange={(v) => patch({ tools: v })} placeholder="list_issues, get_issue" />
        </label>
        {srv.auth === "secret" && (
          <label className="small grow">the key it sends (a NAME)
            <ListInput value={srv.secrets ?? []} onChange={(v) => patch({ secrets: v })} placeholder="GITHUB_TOKEN" />
          </label>
        )}
      </div>
      <p className="muted small" style={{ margin: "6px 0 0" }}>
        It runs at its vendor: Agentry checks its address and what it asks for, not the vendor&apos;s code.{" "}
        {srv.auth === "oauth" && "Before its first run, whoever runs the QuickAgent signs in at the vendor and allows these scopes; the run gets an access token and nothing else."}
        {srv.auth === "secret" && "The key goes to the vendor as a bearer token; whoever runs the QuickAgent supplies it. Under Files, commands and data you say whether a run can start without it."}
      </p>
    </div>
  );
}

// A package server's OAuth sign-in (the manifest's oauth block):
// the issuer, the scopes, and how the access token reaches the server.
function SignInFields({ srv, patch }: { srv: build.MCPServerRow; patch: PatchRow }) {
  const o = srv.oauth;
  const set = (next: Partial<build.OAuthRow>) => patch({ oauth: new build.OAuthRow({ ...o, ...next }) });
  const files = o?.files ?? [];
  const env = o?.env ?? [];
  return (
    <div style={{ marginTop: 6 }}>
      <label className="small">
        <input type="checkbox" checked={!!o}
          onChange={(e) => patch({ oauth: e.target.checked ? new build.OAuthRow({ issuer: "", scopes: [], files: [], env: [] }) : undefined })} />{" "}
        it signs in to the person&apos;s own account (OAuth)
      </label>
      {o && (
        <>
          <div className="row" style={{ marginTop: 4 }}>
            <label className="small grow">issuer
              <input type="text" className="mono" style={{ width: "100%" }} placeholder="https://accounts.google.com"
                value={o.issuer} onChange={(e) => set({ issuer: e.target.value })} />
            </label>
            <label className="small grow">scopes it asks for
              <ListInput value={o.scopes ?? []} onChange={(v) => set({ scopes: v })} placeholder="https://www.googleapis.com/auth/gmail.readonly" />
            </label>
          </div>
          <div style={{ marginTop: 6 }}>
            <span className="small">token files, written into the server&apos;s own folder ({"{access_token}"}, {"{expiry}"}, {"{expiry_ms}"})</span>
            {files.map((f, k) => (
              <div className="row" key={k} style={{ marginTop: 4 }}>
                <input type="text" className="mono" aria-label="token file name" placeholder="credentials.json" style={{ width: 180 }} value={f.path}
                  onChange={(ev) => set({ files: files.map((x, j) => (j === k ? new build.TokenFileRow({ ...x, path: ev.target.value }) : x)) })} />
                <input type="text" className="mono grow" aria-label="token file template" placeholder='{"token": "{access_token}"}' value={f.template}
                  onChange={(ev) => set({ files: files.map((x, j) => (j === k ? new build.TokenFileRow({ ...x, template: ev.target.value }) : x)) })} />
                <Button type="button" onClick={() => set({ files: files.filter((_, j) => j !== k) })}>remove</Button>
              </div>
            ))}
            <Button type="button" className="small" onClick={() => set({ files: [...files, new build.TokenFileRow({ path: "", template: "" })] })}>+ add token file</Button>
          </div>
          <div style={{ marginTop: 6 }}>
            <span className="small">token settings, environment variables ({"{access_token}"}, {"{dir}"} for the folder)</span>
            {env.map((e, k) => (
              <div className="row" key={k} style={{ marginTop: 4 }}>
                <input type="text" className="mono" aria-label="token setting name" placeholder="NAME" style={{ width: 180 }} value={e.name}
                  onChange={(ev) => set({ env: env.map((x, j) => (j === k ? new build.EnvRow({ ...x, name: ev.target.value }) : x)) })} />
                <input type="text" className="mono grow" aria-label="token setting value" placeholder="{dir}" value={e.value}
                  onChange={(ev) => set({ env: env.map((x, j) => (j === k ? new build.EnvRow({ ...x, value: ev.target.value }) : x)) })} />
                <Button type="button" onClick={() => set({ env: env.filter((_, j) => j !== k) })}>remove</Button>
              </div>
            ))}
            <Button type="button" className="small" onClick={() => set({ env: [...env, new build.EnvRow({ name: "", value: "" })] })}>+ add token setting</Button>
          </div>
          <p className="muted small" style={{ margin: "6px 0 0" }}>
            Before its first run, whoever runs the QuickAgent signs in and allows these scopes; the server gets an access token,
            never a refresh token. A token file is packed with the QuickAgent and anyone can read it: where it needs a client id
            or secret, write a dummy such as &ldquo;agentry&rdquo;, never a real token or secret.
          </p>
        </>
      )}
    </div>
  );
}

function ServerChecks({ m, checks }: { m: build.Model; checks: Record<string, mcpvuln.Status> }) {
  const rows = (m.mcpServers ?? []).filter((s) => (isRemote(s) ? (s.url ?? "").trim() : s.package.trim() && s.version.trim()));
  if (rows.length === 0) return null;
  return (
    <div className="card" style={{ marginTop: 12 }}>
      <strong className="small">The declared servers, as the registry last checked them</strong>
      {rows.map((s, i) => {
        const c = checks[serverRef(s)];
        // A remote server has no package for the registry to look up.
        const said = isRemote(s) ? { tone: "muted" as const, text: REMOTE_CHECK } : c ? checkSentence(c) : { tone: "muted" as const, text: "Asking the registry…" };
        return (
          <div key={i} className="small" style={{ marginTop: 6 }}>
            <span className="mono"><RevealText text={serverRef(s)} /></span>
            <div className={said.tone === "high" ? "sev-high" : "muted"} style={said.tone === "high" ? { color: "var(--color-sev-high)" } : undefined}>{said.text}</div>
          </div>
        );
      })}
    </div>
  );
}

/** Where the QuickAgent and each of its servers can reach, in plain words. */
function networkReach(m: build.Model): { what: string; value: string }[] {
  const rows = [{
    what: "The internet",
    value: m.egressMode === "all" ? "any host (*): routed to review, refused by runners whose policy does not allow it"
      : m.egressMode === "hosts" ? (m.egressHosts.join(", ") || "no hosts listed yet") : "none",
  }];
  for (const s of m.mcpServers ?? []) rows.push(reachRow(s));
  return rows;
}

function localReach(m: build.Model): { what: string; value: string }[] {
  const fs = (mode: string, paths: string[]) => (mode === "workspace" ? "the workspace" : mode === "paths" ? paths.join(", ") : "nothing");
  return [
    { what: "Reads", value: fs(m.fsRead, m.fsReadPaths) },
    { what: "Writes", value: fs(m.fsWrite, m.fsWritePaths) },
    { what: "Commands", value: m.execCommands.join(", ") || "none" },
    { what: "Its own keys", value: m.secrets.join(", ") || "none" },
    { what: "Optional keys", value: optionalKeys(m).join(", ") || "none" },
    { what: "Data it touches", value: m.dataAccess.join(", ") || "none declared" },
  ];
}

// RequiredBox is an intake question's required box. A secret question of
// the QuickAgent's own key follows the key's Required/Optional choice:
// drawn from it, and not a box of its own.
function RequiredBox({ m, q, onChange }: { m: build.Model; q: build.IntakeRow; onChange: (required: boolean) => void }) {
  const r = questionRequired(m, q);
  return (
    <label className="check-row" style={{ margin: 0 }} title={r.follows ? "Follows its key's choice under Files, commands and data" : undefined}>
      <input type="checkbox" checked={r.required} disabled={r.follows} onChange={(e) => onChange(e.target.checked)} />
      <span>{r.follows ? "required (follows its key)" : "required"}</span>
    </label>
  );
}

// KeyChoices is a Required/Optional choice for each key the QuickAgent and
// its servers declare:
// a key starts as Optional, and the model holds the Required ones.
function KeyChoices({ m, onChange }: { m: build.Model; onChange: (required: string[]) => void }) {
  const keys = declaredKeys(m);
  if (keys.length === 0) return null;
  const required = new Set(m.requiredSecrets ?? []);
  return (
    <fieldset className="small" style={{ marginTop: 8 }}>
      <legend>does a run need each key?</legend>
      <p className="muted small" style={{ margin: "0 0 6px" }}>
        A run starts without an optional key, and the QuickAgent is told which keys it was not given. Mark a key required when the
        QuickAgent cannot do its job without it.
      </p>
      {keys.map((k) => (
        <div key={k} className="row small" style={{ gap: 6 }}>
          <span className="mono">{k}</span>
          <select aria-label={`Does a run need ${k}?`} value={required.has(k) ? "required" : "optional"}
            onChange={(e) => onChange(requiredKeys(m, { name: k, required: e.target.value === "required" }))}>
            <option value="optional">optional: a run starts without it</option>
            <option value="required">required: a run cannot start without it</option>
          </select>
        </div>
      ))}
    </fieldset>
  );
}

function ReachList({ title, rows }: { title: string; rows: { what: string; value: string }[] }) {
  return (
    <div className="card" style={{ marginTop: 12 }}>
      <strong className="small">{title}</strong>
      <dl className="build-reach">
        {rows.map((r, i) => (
          <div key={i} style={{ display: "contents" }}>
            {/* Through RevealText: a scope, an address or a tool name can
                hold a character a reader cannot see. */}
            <dt className="mono small"><RevealText text={r.what} /></dt>
            <dd><RevealText text={r.value} /></dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export default function Build({ registry, initialView = "start", initialStep = 0, initialDraft = null, openDraftRev = 0, openEditRev = 0, onOpenAgent, onOpenSettings, onOpenReviewKey, onOpenLibrary }: {
  registry: string;
  /** Harness only: open on a view, and on a step of Step by step. */
  initialView?: View;
  initialStep?: number;
  /** Harness only: open with a draft in the form. */
  initialDraft?: app.DraftView | null;
  /** Raised by the draft's dialog or card's "Open the draft": the draft
   *  that landed opens now, whatever view the tab is on. */
  openDraftRev?: number;
  /** Raised by the QuickAgent page's Edit: the
   *  release it asked for (edit.ts) opens in the form now. */
  openEditRev?: number;
  /** A registry match's page, for Describe it's "install it instead". */
  onOpenAgent?: (ns: string, name: string) => void;
  /** Settings, for Describe it's missing AI Review key. */
  onOpenSettings?: () => void;
  /** Settings at the AI Review key, for Review Your QuickAgents locally. */
  onOpenReviewKey?: () => void;
  /** The Library, where Install puts the QuickAgent. */
  onOpenLibrary?: () => void;
}) {
  const [dir, setDir] = useState(kept?.dir ?? "");
  const [model, setModel] = useState<build.Model | null>(kept?.model ?? null);
  const [touched, setTouched] = useState(!!kept || !!initialDraft);
  // The Drafter's notes, while its draft is in the form.
  const [drafted, setDrafted] = useState<app.DraftView | null>(kept?.drafted ?? initialDraft);
  // The release the form edits, while the form still
  // names its QuickAgent: Submit then adds a version to it.
  const [editing, setEditing] = useState<Editing | null>(kept?.editing ?? null);
  // A landed revision shows its diff and Publish this version until the
  // person opens it in the form.
  const [revisionShown, setRevisionShown] = useState(kept?.revisionShown ?? false);
  // What Edit could not carry over of the previous version's shipped packs,
  // shown in the Knowledge section until the form is replaced.
  const [shipNotes, setShipNotes] = useState<string[]>(kept?.shipNotes ?? []);
  // A draft still being made is followed on Describe it.
  const [view, setView] = useState<View>(draftInFlight() ? "describe" : initialView);
  const [serverChecks, setServerChecks] = useState<Record<string, mcpvuln.Status>>({});
  const asked = useRef(new Set<string>());
  const [at, setAt] = useState(initialStep);
  const [stepProblem, setStepProblem] = useState<string | null>(null);
  const [check, setCheck] = useState<build.CheckResult | null>(null);
  const [summary, setSummary] = useState<build.Summary | null>(null);
  const summaryHeading = useRef<HTMLHeadingElement | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [result, setResult] = useState<validate.Result | null>(null);
  const [pubResult, setPubResult] = useState<build.PublishResult | null>(null);
  // What Install laid down on this computer.
  const [localInstall, setLocalInstall] = useState<app.LocalInstallView | null>(null);
  const [log, setLog] = useState<string[]>([]);
  const logRef = useRef<HTMLPreElement | null>(null);
  const heading = useRef<HTMLHeadingElement | null>(null);
  const moved = useRef(false);

  // A draft taken into the form while the fresh form was still being made:
  // the draft wins, whichever answers last.
  const tookDraft = useRef(false);

  useEffect(() => {
    if (kept) return;
    if (initialDraft?.model) {
      setModel(new build.Model(initialDraft.model));
      return;
    }
    BuildAPI.NewModel(registry)
      .then((m) => { if (!tookDraft.current) setModel(m); })
      .catch((e) => setError(String(e)));
  }, [registry, initialDraft]);

  // A draft that finished while the tab was away opens now, and so does one
  // the person asked to open from the draft's dialog or card.
  useEffect(() => {
    const v = takeLandedDraft();
    if (v) takeDraft(v);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openDraftRev]);

  // The QuickAgent page's Edit: its release, read through the registry,
  // opens in the form.
  useEffect(() => {
    const r = takeEditRequest();
    if (!r) return;
    void wrap("edit", async () => {
      // Submit sends to this tab's registry, so the release must be one of
      // its QuickAgents.
      if (!sameRegistry(r.registry, registry)) {
        throw new Error(`${r.namespace}/${r.name} is on ${r.registry}, and the Build tab submits to ${registry}. Choose that registry in Settings → Registry settings, then press Edit again.`);
      }
      takeEdit(await BuildAPI.EditRelease(r.registry, r.namespace, r.name));
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openEditRev]);

  // The edit lasts while the form names its QuickAgent.
  const editingNow = stillEditing(editing, model);
  useEffect(() => {
    if (editing && !editingNow) setEditing(null);
  }, [editing, editingNow]);

  // Keep the work for the start page's Continue (see `kept`).
  useEffect(() => {
    if (model && touched) kept = { model, dir, view: view === "start" || view === "describe" ? kept?.view ?? "steps" : view, drafted, editing, revisionShown, shipNotes };
    // What the QuickAgent page's Edit says it replaces.
    noteHeldForm(kept ? kept.model.name || "a QuickAgent with no name yet" : null);
  }, [model, dir, view, touched, drafted, editing, revisionShown, shipNotes]);

  useEffect(() => EventsOn("build:publish-log", (line: string) => setLog((p) => [...p, line])), []);
  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }); }, [log]);

  // The model's check, a moment after the last change: the lines each step
  // writes and the first problem in each. It reads and writes nothing.
  useEffect(() => {
    if (!model) return;
    let live = true;
    const t = setTimeout(() => {
      BuildAPI.Check(model)
        .then((c) => { if (live) setCheck(c); })
        .catch((e) => { if (live) { setCheck(null); setError(`The checks could not run: ${String(e)}`); } });
    }, 200);
    return () => { live = false; clearTimeout(t); };
  }, [model]);

  // The summary of the model as it stands, while the summary is showing.
  useEffect(() => {
    if (!model || view !== "summary") return;
    let live = true;
    BuildAPI.Summary(model, drafted?.unmentioned ?? [])
      .then((s) => { if (live) setSummary(s); })
      .catch((e) => { if (live) setError(`The summary could not be drawn: ${String(e)}`); });
    return () => { live = false; };
  }, [model, view, drafted]);

  // The heading takes focus once the summary is drawn, as a step's does, so
  // a keyboard or a screen reader starts on it.
  const summaryShown = summary !== null;
  useEffect(() => {
    if (view === "summary" && summaryShown && moved.current) summaryHeading.current?.focus();
  }, [view, summaryShown]);

  // What the registry last knew of each declared server version, asked once
  // per version while Tools and web access or Review is showing.
  useEffect(() => {
    if (!model || view !== "steps" || (STEPS[at].key !== "tools" && STEPS[at].key !== "review")) return;
    const fresh = (model.mcpServers ?? []).filter((s) => !isRemote(s) && s.package.trim() && s.version.trim() && !asked.current.has(serverRef(s)));
    if (fresh.length === 0) return;
    fresh.forEach((s) => asked.current.add(serverRef(s)));
    BuildAPI.ServerChecks(registry, new build.Model({ ...model, mcpServers: fresh }))
      .then((statuses) => setServerChecks((prev) => {
        const next = { ...prev };
        for (const st of statuses ?? []) next[st.ref] = st;
        return next;
      }))
      .catch(() => {});
  }, [model, view, at, registry]);

  // Focus follows the step, as in the run setup; not on the first render.
  useEffect(() => {
    if (moved.current && view === "steps") heading.current?.focus();
  }, [at, view]);

  const m = model;
  const set = (patch: Partial<build.Model>) => {
    if (!m) return;
    setTouched(true);
    const next = { ...m, ...patch };
    // A Required mark goes with its key (src/requiredKeys.ts).
    setModel(new build.Model({ ...next, requiredSecrets: requiredKeys(next) }));
  };

  async function wrap(label: string, fn: () => Promise<void>) {
    setBusy(label);
    setError(null);
    setNotice(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  const openFolder = () =>
    wrap("pick", async () => {
      const d = await ChooseDirectory("Choose a QuickAgent workspace folder");
      if (!d) return;
      if (!(await BuildAPI.Exists(d))) {
        setDir(d);
        setNotice(`${d} holds no agent.yaml yet: the form saves into it.`);
        return;
      }
      const loaded = await BuildAPI.Load(d);
      setDir(d);
      setModel(loaded);
      setDrafted(null);
      setEditing(null);
      setShipNotes([]);
      setTouched(true);
      setResult(null);
      setPubResult(null);
      setNotice(`Loaded the QuickAgent in ${d}.`);
      if (view === "start" || view === "describe") setView("advanced");
    });

  // The folder is made at the first save, with no file dialog: Go makes a
  // new folder named for the QuickAgent under the default output folder
  // (Settings → Run defaults, else ~/Downloads/agentry-out), so the page
  // names no path and nothing already there is written over. Later saves
  // reuse it.
  async function ensureDir(): Promise<string> {
    if (dir) return dir;
    const d = await BuildAPI.NewSaveFolder(m?.name ?? "");
    setDir(d);
    return d;
  }

  const save = () =>
    wrap("save", async () => {
      const d = await ensureDir();
      if (!m || !d) return;
      await BuildAPI.Save(registry, d, m);
      if (m.hadComments) setModel(new build.Model({ ...m, hadComments: false }));
      setNotice(`Saved to ${d}: agent.yaml, the instructions file, and on the first save README, LICENSE and .gitignore.`);
    });

  const runValidate = () =>
    wrap("validate", async () => {
      const d = await ensureDir();
      if (!m || !d) return;
      await BuildAPI.Save(registry, d, m);
      if (m.hadComments) setModel(new build.Model({ ...m, hadComments: false }));
      setResult(await BuildAPI.Validate(d));
    });

  // Publish this version: a new folder Go makes under
  // the default output folder, and Go saves and submits the revision
  // as drafted.
  const publishRevision = () =>
    wrap("submit", async () => {
      const rev = drafted?.revision;
      const d = await BuildAPI.NewSaveFolder(rev ? `${rev.name}-${rev.version}` : "");
      setLog([]);
      setPubResult(null);
      setLocalInstall(null);
      const r = await BuildAPI.PublishThisVersion(registry, d, drafted?.revision?.key ?? "");
      setDir(d);
      if (r) setPubResult(r);
    });

  // Submit is public: the Go side first asks, in a
  // native dialog, whether to spend the submission's credits, and says
  // whether it becomes public or, existing private, stays so; a No returns
  // nothing and sends nothing.
  const publish = () =>
    wrap("submit", async () => {
      const d = await ensureDir();
      if (!m || !d) return;
      await BuildAPI.Save(registry, d, m);
      if (m.hadComments) setModel(new build.Model({ ...m, hadComments: false }));
      setLog([]);
      setPubResult(null);
      setLocalInstall(null);
      const r = await BuildAPI.Submit(registry, d);
      if (r) setPubResult(r);
    });

  // Review Your QuickAgents locally reviews the saved
  // folder with the built-in Examiner on this computer: the Go side asks in
  // a native dialog first, and a No reviews nothing. The review is followed
  // in its own dialog and card (ReviewFlow); nothing here waits for it.
  const reviewHere = () =>
    wrap("review", async () => {
      const d = await ensureDir();
      if (!m || !d) return;
      await BuildAPI.Save(registry, d, m);
      if (m.hadComments) setModel(new build.Model({ ...m, hadComments: false }));
      await startReview(registry, d);
    });

  // Install lays the saved folder down on this computer, local and
  // unsigned; its consent is the Go side's native
  // dialog, and a No returns nothing.
  const installHere = () =>
    wrap("install", async () => {
      const d = await ensureDir();
      if (!m || !d) return;
      await BuildAPI.Save(registry, d, m);
      if (m.hadComments) setModel(new build.Model({ ...m, hadComments: false }));
      setPubResult(null);
      setLocalInstall(null);
      const v = await BuildAPI.Install(d);
      if (v) setLocalInstall(v);
    });

  if (!m) return error ? <ErrorBox error={error} /> : <Spinner />;

  const stepKey = STEPS[at].key;

  function goTo(i: number) {
    moved.current = true;
    setStepProblem(null);
    setAt(Math.max(0, Math.min(STEPS.length - 1, i)));
  }

  async function next() {
    if (!m) return;
    let c: build.CheckResult;
    try {
      c = await BuildAPI.Check(m);
    } catch (e) {
      setError(`The checks could not run: ${String(e)}`);
      return;
    }
    setCheck(c);
    const p = stepKey === "review" ? null : c.problems?.[stepKey];
    if (p) {
      moved.current = true;
      setStepProblem(p);
      heading.current?.focus();
      return;
    }
    goTo(at + 1);
  }

  // What an earlier save, validate, publish or step check said, cleared
  // whenever the form is replaced.
  function resetOutcome() {
    setResult(null);
    setPubResult(null);
    setLocalInstall(null);
    setStepProblem(null);
    setNotice(null);
    setError(null);
  }

  function startOver() {
    kept = null;
    noteHeldForm(null);
    setDrafted(null);
    setEditing(null);
    setShipNotes([]);
    resetOutcome();
    asked.current.clear();
    setServerChecks({});
    setTouched(false);
    setDir("");
    setModel(null);
    BuildAPI.NewModel(registry).then(setModel).catch((e) => setError(String(e)));
  }

  // A draft made by Describe it: into the form, with no folder yet (the
  // first save makes it), and open on the summary.
  function takeDraft(v: app.DraftView) {
    if (!v.model) return;
    tookDraft.current = true;
    moved.current = true;
    setModel(new build.Model(v.model));
    setDrafted(v);
    // Edit with AI's revision is an edit of its QuickAgent: the name fixed,
    // the next minor set.
    const r = v.revision;
    setEditing(r ? { namespace: r.namespace, name: r.name, from: r.from, version: r.version } : null);
    setRevisionShown(!!r);
    // A pack the draft or the previous version shipped that cannot ship now
    // is named in the Knowledge section; the draft's own notes name the rest.
    setShipNotes(r ? (v.decodeNotes ?? []) : []);
    setTouched(true);
    setDir("");
    resetOutcome();
    setSummary(null);
    setView("summary");
    setAt(stepOf("review"));
  }

  // A release opened by Edit: into the form under its own name at the next
  // minor, with no folder yet (the first save makes it), on the summary,
  // as a draft opens. What the Builder cannot hold was dropped in Go,
  // without a word.
  function takeEdit(v: app.EditView) {
    if (!v.model) return;
    tookDraft.current = true;
    moved.current = true;
    setModel(new build.Model(v.model));
    setDrafted(null);
    setEditing({ namespace: v.namespace, name: v.name, from: v.from, version: v.version });
    setShipNotes(v.notes ?? []);
    setTouched(true);
    setDir("");
    resetOutcome();
    setSummary(null);
    setView("summary");
    setAt(stepOf("review"));
  }

  function switchTo(v: View) {
    moved.current = true;
    // The form is the Builder: a revision opened there is the person's own.
    if (v === "steps" || v === "advanced") setRevisionShown(false);
    setStepProblem(null);
    // The summary is drawn again from the model as it now stands.
    if (v === "summary") setSummary(null);
    setView(v);
    if (v === "steps") setAt(0);
  }

  // The summary's See it in the form: Step by step on Review, where the
  // notes are, or Advanced.
  function toForm(v: "steps" | "advanced") {
    switchTo(v);
    if (v === "steps") setAt(stepOf("review"));
  }

  const stepped = view === "steps";
  const inputStyle = { width: "100%" };

  // ---- The sections: the same fields in both views. ----
  const sections: Record<SectionKey, ReactNode> = {
    basics: (
      <>
        <label className="small" style={{ display: "block" }}>name
          <input style={inputStyle} className="mono" value={m.name} onChange={(e) => set({ name: e.target.value })} placeholder="my-quickagent" readOnly={!!editingNow} title={editingNow ? "Edit adds a version to this QuickAgent, so its name stays" : undefined} />
        </label>
        <label className="small" style={{ display: "block", marginTop: 8 }}>summary (one line, ≤140 chars)
          <input style={inputStyle} maxLength={140} value={m.summary} onChange={(e) => set({ summary: e.target.value })} />
        </label>
        <div className="small" style={{ marginTop: 8 }}>run modes
          <span className="row" style={{ gap: 10, marginTop: 4 }}>
            {(["autonomous", "interactive"] as const).map((mode) => (
              <label key={mode} className="check-row" style={{ margin: 0 }}>
                <input
                  type="checkbox"
                  checked={(m.modes ?? []).includes(mode)}
                  onChange={(e) => set({ modes: e.target.checked ? [...(m.modes ?? []).filter((x) => x !== mode), mode] : (m.modes ?? []).filter((x) => x !== mode) })}
                />
                <span>{mode} <span className="muted">— {mode === "autonomous" ? "runs from its files and ends" : "a chat session under the same controls"}</span></span>
              </label>
            ))}
          </span>
        </div>
        {(m.modes ?? []).includes("interactive") && (
          <div className="row" style={{ marginTop: 8 }}>
            <label className="small grow">opening line (the agent&apos;s first message when a session starts without a task; ≤ 500 chars)
              <input style={inputStyle} maxLength={500} value={m.opening ?? ""} onChange={(e) => set({ opening: e.target.value })} placeholder="What should I look at?" />
            </label>
            <label className="small">idle timeout (seconds; 0 = the runner&apos;s)
              <input type="number" min={0} style={{ width: 110 }} value={m.idleTimeoutSeconds ?? 0} onChange={(e) => set({ idleTimeoutSeconds: Math.max(0, Number(e.target.value) || 0) })} />
            </label>
          </div>
        )}
      </>
    ),
    instructions: (
      <>
        <label className="small" style={{ display: "block" }}>{m.instructionFile || "AGENT.md"}
          <Textarea className="mono" style={{ minHeight: 260 }} value={m.instructions} onChange={(e) => set({ instructions: e.target.value })} />
        </label>
        <p className="muted small" style={{ margin: "4px 0 0" }}>
          {new TextEncoder().encode(m.instructions).length.toLocaleString("en")} of {MAX_INSTRUCTION_BYTES.toLocaleString("en")} bytes.
        </p>
      </>
    ),
    tools: (
      <>
        <strong className="small">Web access</strong>
        <div className="row" style={{ marginTop: 4 }}>
          <label className="small">does it need the internet?
            <select value={m.egressMode} onChange={(e) => set({ egressMode: e.target.value })}>
              <option value="none">no</option>
              <option value="hosts">only these sites</option>
              {/* Any host is Advanced's alone: a declared server's "*" is
                  always refused, and the QuickAgent's routes it to review. */}
              {(!stepped || m.egressMode === "all") && <option value="all">any host (*)</option>}
            </select>
          </label>
          {m.egressMode === "hosts" && (
            <label className="small grow">sites it may reach (host names)
              <ListInput value={m.egressHosts} onChange={(v) => set({ egressHosts: v })} placeholder="api.example.com, github.com" />
            </label>
          )}
          {m.egressMode === "all" && <span className="badge bad">refused at run time unless the operator allows wildcard egress</span>}
        </div>
        <div style={{ marginTop: 12 }}>
          <div className="row" style={{ justifyContent: "space-between" }}>
            <strong className="small">MCP servers (exact version pins; each installed per release)</strong>
            <div className="row">
              <Button
                type="button"
                onClick={() => set({ mcpServers: [...(m.mcpServers ?? []), new build.MCPServerRow({ source: "npm", package: "", version: "", args: [], tools: [], secrets: [], egress: [], env: [] })] })}
              >
                + add server
              </Button>
              <Button
                type="button"
                onClick={() => set({ mcpServers: [...(m.mcpServers ?? []), new build.MCPServerRow({ source: "url", package: "", version: "", args: [], url: "", auth: "", scopes: [], tools: [], secrets: [], egress: [], env: [] })] })}
              >
                + add remote server
              </Button>
            </div>
          </div>
          {(m.mcpServers ?? []).length === 0 && (
            <p className="muted small" style={{ margin: "4px 0 0" }}>
              None. A server declared here is resolved from npm or PyPI at the pinned version and installed into the
              release&apos;s own store; its tools, keys and hosts are scoped to that server. A remote server runs at its
              vendor and is reached at its https:// address.
            </p>
          )}
          {(m.mcpServers ?? []).map((srv, i) => {
            const patch = (row: Partial<build.MCPServerRow>) =>
              set({ mcpServers: m.mcpServers.map((r, j) => (j === i ? new build.MCPServerRow({ ...r, ...row }) : r)) });
            const env = srv.env ?? [];
            const args = srv.args ?? [];
            const sourceSelect = (
              <label className="small">source
                <select value={srv.source} onChange={(e) => patch({ source: e.target.value })}>
                  <option value="npm">npm</option>
                  <option value="pypi">PyPI</option>
                  <option value="url">remote</option>
                </select>
              </label>
            );
            const remove = <Button type="button" onClick={() => set({ mcpServers: m.mcpServers.filter((_, j) => j !== i) })}>remove</Button>;
            if (isRemote(srv)) return <RemoteServerRow key={i} srv={srv} patch={patch} source={sourceSelect} remove={remove} />;
            return (
              <div className="card" key={i} style={{ marginTop: 8 }}>
                <div className="row">
                  {sourceSelect}
                  <label className="small grow">package
                    <input type="text" className="mono" style={{ width: "100%" }} placeholder={srv.source === "npm" ? "@scope/server-name" : "mcp-server-name"}
                      value={srv.package} onChange={(e) => patch({ package: e.target.value })} />
                  </label>
                  <label className="small">exact version
                    <input type="text" className="mono" style={{ width: 120 }} placeholder="1.2.3" value={srv.version} onChange={(e) => patch({ version: e.target.value })} />
                  </label>
                  {remove}
                </div>
                {/* One field per argument, not a comma list: an argument may hold a comma. */}
                <div style={{ marginTop: 6 }}>
                  <span className="small">arguments its command starts with, in order (for a server that is a subcommand, such as <span className="mono">mcp</span>)</span>
                  <div className="row" style={{ marginTop: 4, flexWrap: "wrap" }}>
                    {args.map((a, k) => (
                      <span className="row" key={k} style={{ gap: 2 }}>
                        <input type="text" className="mono" aria-label={`argument ${k + 1}`} placeholder="word" style={{ width: 140 }} value={a}
                          onChange={(ev) => patch({ args: args.map((x, j) => (j === k ? ev.target.value : x)) })} />
                        <Button type="button" aria-label={`remove argument ${k + 1}`} onClick={() => patch({ args: args.filter((_, j) => j !== k) })}>remove</Button>
                      </span>
                    ))}
                    <Button type="button" className="small" onClick={() => patch({ args: [...args, ""] })}>+ add argument</Button>
                  </div>
                </div>
                <div className="row" style={{ marginTop: 6 }}>
                  <label className="small grow">tools it may call (empty = all it offers)
                    <ListInput value={srv.tools ?? []} onChange={(v) => patch({ tools: v })} placeholder="list_issues, get_issue" />
                  </label>
                  <label className="small grow">keys it needs (NAMES)
                    <ListInput value={srv.secrets ?? []} onChange={(v) => patch({ secrets: v })} placeholder="GITHUB_PAT" />
                  </label>
                  <label className="small grow">hosts it reaches (empty = none)
                    <ListInput value={srv.egress ?? []} onChange={(v) => patch({ egress: v })} placeholder="api.github.com" />
                  </label>
                </div>
                <div style={{ marginTop: 6 }}>
                  <span className="small">configuration (plain env values, never keys)</span>
                  {env.map((e, k) => (
                    <div className="row" key={k} style={{ marginTop: 4 }}>
                      <input type="text" className="mono" aria-label="env name" placeholder="NAME" style={{ width: 180 }} value={e.name}
                        onChange={(ev) => patch({ env: env.map((x, j) => (j === k ? new build.EnvRow({ ...x, name: ev.target.value }) : x)) })} />
                      <input type="text" className="mono grow" aria-label="env value" placeholder="value" value={e.value}
                        onChange={(ev) => patch({ env: env.map((x, j) => (j === k ? new build.EnvRow({ ...x, value: ev.target.value }) : x)) })} />
                      <Button type="button" onClick={() => patch({ env: env.filter((_, j) => j !== k) })}>remove</Button>
                    </div>
                  ))}
                  <Button type="button" className="small" onClick={() => patch({ env: [...env, new build.EnvRow({ name: "", value: "" })] })}>+ add configuration</Button>
                </div>
                <SignInFields srv={srv} patch={patch} />
                {(srv.secrets ?? []).length > 0 && (
                  <p className="muted small" style={{ margin: "6px 0 0" }}>
                    It needs {srv.secrets.join(", ")}: whoever runs the QuickAgent supplies {srv.secrets.length === 1 ? "it" : "them"}, on their computer or when they start it in Agentry. Under Files, commands and data you say whether a run can start without each.
                  </p>
                )}
              </div>
            );
          })}
        </div>
        {stepped && <ReachList title="Everywhere it and its servers can reach" rows={networkReach(m)} />}
        {stepped && <ServerChecks m={m} checks={serverChecks} />}
      </>
    ),
    access: (
      <>
        <div className="row">
          <label className="small">does it read files?
            <select value={m.fsRead} onChange={(e) => set({ fsRead: e.target.value })}>
              <option value="none">no</option>
              <option value="workspace">the workspace</option>
              <option value="paths">only these paths</option>
            </select>
          </label>
          {m.fsRead === "paths" && (
            <label className="small grow">paths it reads
              <ListInput value={m.fsReadPaths} onChange={(v) => set({ fsReadPaths: v })} />
            </label>
          )}
          <label className="small">does it write files?
            <select value={m.fsWrite} onChange={(e) => set({ fsWrite: e.target.value })}>
              <option value="none">no</option>
              <option value="workspace">the workspace</option>
              <option value="paths">only these paths</option>
            </select>
          </label>
          {m.fsWrite === "paths" && (
            <label className="small grow">paths it writes
              <ListInput value={m.fsWritePaths} onChange={(v) => set({ fsWritePaths: v })} />
            </label>
          )}
        </div>
        <label className="small" style={{ display: "block", marginTop: 8 }}>commands it runs (single words; empty = none)
          <ListInput value={m.execCommands} onChange={(v) => set({ execCommands: v })} placeholder="git, jq" />
        </label>
        <label className="small" style={{ display: "block", marginTop: 8 }}>keys or tokens of its own (NAMES of environment variables, never values)
          <ListInput value={m.secrets} onChange={(v) => set({ secrets: v })} placeholder="MY_API_TOKEN" />
        </label>
        <KeyChoices m={m} onChange={(requiredSecrets) => set({ requiredSecrets })} />
        <div className="small" style={{ marginTop: 8 }}>which kinds of data does it touch?
          <div className="row" style={{ flexWrap: "wrap" }}>
            {DATA_ACCESS.map((d) => (
              <label key={d} className="row small" style={{ gap: 4 }}>
                <input type="checkbox" checked={m.dataAccess.includes(d)}
                  onChange={(e) => set({ dataAccess: e.target.checked ? [...m.dataAccess, d] : m.dataAccess.filter((x) => x !== d) })} />
                {d}
              </label>
            ))}
          </div>
        </div>
        {/* capabilities.browser and targetHosts: no editor yet, carried
            as they were drafted or loaded. */}
        {m.browser && (
          <p className="small muted" style={{ marginTop: 8 }}>
            {m.browser.scope === "public"
              ? "It has a browser that may open any public site"
              : <>It has a browser that may open the {(m.browser.sites ?? []).length === 1 ? "site" : "sites"} the person gives for{" "}
                  <span className="mono">{(m.browser.sites ?? []).join(", ") || "(no question)"}</span>
                  {(m.browser.hosts ?? []).length > 0 && <>, and <span className="mono">{(m.browser.hosts ?? []).join(", ")}</span></>}</>}
            {(m.browser.secrets ?? []).length > 0 && <>; it may type <span className="mono">{(m.browser.secrets ?? []).join(", ")}</span> into the site&apos;s forms</>}
            . Kept as it was drafted or loaded.
          </p>
        )}
        {(m.targetHosts ?? []).length > 0 && (
          <p className="small muted" style={{ marginTop: 8 }}>
            It administers the {(m.targetHosts ?? []).length === 1 ? "machine" : "machines"} the person gives for{" "}
            <span className="mono">{(m.targetHosts ?? []).join(", ")}</span>. Kept as it was drafted or loaded.
          </p>
        )}
      </>
    ),
    questions: (
      <>
        <p className="muted small" style={{ marginTop: 0 }}>
          The answers are staged read-only for the agent at <span className="mono">/inputs/intake.json</span>. A <em>secret</em>{" "}
          answer is carried by a declared secret of the same upper-cased name and never reaches the agent or a receipt.
        </p>
        <div className="row">
          <Button type="button" onClick={() => set({ intake: [...(m.intake ?? []), new build.IntakeRow({ id: "", prompt: "", type: "text", required: false, choices: [] })] })}>
            Add question
          </Button>
          {(m.intake ?? []).length === 0 && <span className="muted small">No questions: the agent starts from its instructions and the task.</span>}
        </div>
        {(m.intake ?? []).map((q, i) => {
          const patch = (row: Partial<build.IntakeRow>) => set({ intake: m.intake.map((r, j) => (j === i ? new build.IntakeRow({ ...r, ...row }) : r)) });
          return (
            <div className="card" key={i} style={{ marginTop: 8 }}>
              <div className="row">
                <label className="small">id
                  <input type="text" className="mono" style={{ width: 140 }} placeholder="dataset" value={q.id} onChange={(e) => patch({ id: e.target.value })} />
                </label>
                <label className="small grow">prompt
                  <input type="text" style={{ width: "100%" }} placeholder="Path to the dataset to analyse" value={q.prompt} onChange={(e) => patch({ prompt: e.target.value })} />
                </label>
                <label className="small">type
                  <select value={q.type || "text"} onChange={(e) => patch({ type: e.target.value })}>
                    <option value="text">text</option>
                    <option value="choice">choice</option>
                    <option value="file">file</option>
                    <option value="secret">secret</option>
                    <option value="repository">repository</option>
                    <option value="url">url</option>
                    <option value="host">host</option>
                  </select>
                </label>
                <RequiredBox m={m} q={q} onChange={(required) => patch({ required })} />
                <Button type="button" onClick={() => set({ intake: m.intake.filter((_, j) => j !== i) })}>remove</Button>
              </div>
              {q.type === "repository" && (
                <p className="muted small" style={{ margin: "6px 0 0" }}>
                  The person gives a git repository&apos;s https address, and a branch, tag or commit if they want one. The run fetches it
                  before the agent starts, into <span className="mono">/inputs/{q.id || "<id>"}/</span>, with the key they saved for its host, or
                  none for a public one.
                </p>
              )}
              {q.type === "url" && !(m.browser?.sites ?? []).includes(q.id) && (
                <p className="muted small" style={{ margin: "6px 0 0" }}>
                  The person gives the https address of one file. The run fetches it before the agent starts, and the agent reads the file as it
                  would one the person added.
                </p>
              )}
              {q.type === "url" && (m.browser?.sites ?? []).includes(q.id) && (
                <p className="muted small" style={{ margin: "6px 0 0" }}>
                  The person gives a website&apos;s address. The run&apos;s browser may open it and its subdomains; nothing is fetched before the
                  agent starts.
                </p>
              )}
              {q.type === "host" && (
                <p className="muted small" style={{ margin: "6px 0 0" }}>
                  The person picks one of their target hosts when the run starts. The agent runs commands on it through the host tools.
                </p>
              )}
              {q.type === "choice" && (
                <label className="small" style={{ display: "block", marginTop: 6 }}>choices
                  <ListInput value={q.choices ?? []} onChange={(v) => patch({ choices: v })} placeholder="emea, apac, amer" />
                </label>
              )}
            </div>
          );
        })}
      </>
    ),
    guide: (() => {
      const g = m.guide ?? new build.GuideModel({ overview: "", prepare: [], files: "", task: "", connect: [] });
      const setGuide = (patch: Partial<build.GuideModel>) => set({ guide: new build.GuideModel({ ...g, ...patch }) });
      const connect = g.connect ?? [];
      const need = [...(g.prepare ?? []).filter((s) => s.trim()), ...(g.files?.trim() ? [g.files.trim()] : [])];
      return (
        <>
          <label className="small" style={{ display: "block" }}>what it does and returns (markdown, up to 4 KB; empty: the run setup shows the summary)
            <textarea rows={4} style={inputStyle} value={g.overview} onChange={(e) => setGuide({ overview: e.target.value })} />
          </label>
          <label className="small" style={{ display: "block", marginTop: 8 }}>you will need (one item per line, up to ten)
            <LinesInput value={g.prepare ?? []} onChange={(v) => setGuide({ prepare: v })} placeholder={"The incident's ticket number\nAccess to the service's logs"} />
          </label>
          <div className="row" style={{ marginTop: 8 }}>
            <label className="small grow">files to bring (one line)
              <input style={inputStyle} maxLength={500} value={g.files} onChange={(e) => setGuide({ files: e.target.value })} placeholder="The log excerpt, as a text file" />
            </label>
            <label className="small grow">task hint (the task field&apos;s placeholder)
              <input style={inputStyle} maxLength={200} value={g.task} onChange={(e) => setGuide({ task: e.target.value })} placeholder="Summarise the incident in INC-1234" />
            </label>
          </div>
          <div style={{ marginTop: 8 }}>
            <span className="small">help for each key it needs: where to create it, which scopes (markdown, up to 2,000 characters each)</span>
            {connect.map((c, k) => (
              <div className="row" key={k} style={{ marginTop: 4, alignItems: "flex-start" }}>
                <input type="text" className="mono" aria-label="key name" placeholder="GITHUB_TOKEN" style={{ width: 200 }} value={c.secret}
                  onChange={(e) => setGuide({ connect: connect.map((x, j) => (j === k ? new build.ConnectRow({ ...x, secret: e.target.value }) : x)) })} />
                <textarea className="grow" aria-label="help" rows={3} maxLength={2000} style={inputStyle}
                  placeholder={"1. Open GitHub → Settings → Developer settings → Fine-grained tokens.\n2. Give it read access to pull requests."} value={c.help}
                  onChange={(e) => setGuide({ connect: connect.map((x, j) => (j === k ? new build.ConnectRow({ ...x, help: e.target.value }) : x)) })} />
                <Button type="button" onClick={() => setGuide({ connect: connect.filter((_, j) => j !== k) })}>remove</Button>
              </div>
            ))}
            <Button type="button" className="small" onClick={() => setGuide({ connect: [...connect, new build.ConnectRow({ secret: "", help: "" })] })}>+ add help for a key</Button>
          </div>
          {/* knowledge.accepts: a pack of the person's
              own the run setup's Knowledge step asks for. */}
          <label className="small" style={{ display: "block", marginTop: 12 }}>knowledge it asks for (one line the run setup shows, ≤ 300 chars; empty for none)
            <input style={inputStyle} maxLength={300} value={m.knowledgePrompt ?? ""} onChange={(e) => set({ knowledgePrompt: e.target.value })}
              placeholder="Attach your team's runbook: the steps it quotes come from it." />
          </label>
          <label className="small">
            <input type="checkbox" checked={!!m.knowledgeRequired} onChange={(e) => set({ knowledgeRequired: e.target.checked })} />{" "}
            required: a run cannot start without a pack
          </label>
          <BuildShips registry={registry} m={m} onChange={(ships) => set(ships)} draftShipped={drafted ? (drafted.model?.knowledgeShips ?? []) : null} notes={shipNotes} />
          {stepped && (
            <div className="card" style={{ marginTop: 12 }}>
              <strong className="small">What a person sees before a run</strong>
              <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{g.overview.trim() || m.summary || "The summary goes here."}</p>
              {need.length > 0 && (
                <>
                  <strong className="small">What to have ready</strong>
                  <ul className="run-need">{need.map((n, k) => <li key={k}>{n}</li>)}</ul>
                </>
              )}
              {connect.some((c) => c.secret.trim() && c.help.trim()) && (
                <>
                  <strong className="small">Where to get each key</strong>
                  <KeyHelpList rows={connect} />
                </>
              )}
            </div>
          )}
        </>
      );
    })(),
    publish: (
      <>
        <div className="row">
          <label className="small grow">namespace
            <input style={inputStyle} className="mono" value={m.namespace} onChange={(e) => set({ namespace: e.target.value })} readOnly={!!editingNow} title={editingNow ? "Edit adds a version to this QuickAgent, so its namespace stays" : undefined} />
          </label>
          <label className="small">version
            <input className="mono" value={m.version} onChange={(e) => set({ version: e.target.value })} />
          </label>
          <label className="small">license (SPDX)
            <input className="mono" value={m.license} onChange={(e) => set({ license: e.target.value })} />
          </label>
        </div>
        <label className="small" style={{ display: "block", marginTop: 8 }}>homepage (optional)
          <input style={inputStyle} value={m.homepage ?? ""} onChange={(e) => set({ homepage: e.target.value })} placeholder="https://github.com/you/your-quickagent" />
        </label>
        <label className="small" style={{ display: "block", marginTop: 8 }}>tags (up to ten)
          <ListInput value={m.tags} onChange={(v) => set({ tags: v })} placeholder="comma, separated" />
        </label>
        {((m.authors ?? []).length > 0 || (m.runtimeModels ?? []).length > 0 || (m.dependencies ?? []).length > 0 || m.publishChannel) && (
          <p className="muted small" style={{ marginBottom: 0 }}>
            Carried through from agent.yaml as it is:{" "}
            {[
              (m.authors ?? []).length > 0 && `authors (${m.authors.map((a) => a.name).join(", ")})`,
              (m.runtimeModels ?? []).length > 0 && `runtime models (${m.runtimeModels.join(", ")})`,
              (m.dependencies ?? []).length > 0 && `dependencies (${m.dependencies.map((d) => d.ref).join(", ")})`,
              m.publishChannel && `the ${m.publishChannel} channel`,
            ].filter(Boolean).join("; ")}.
          </p>
        )}
      </>
    ),
  };

  // What a section writes to agent.yaml, once its fields pass.
  const writes = (key: SectionKey) => {
    const f = check?.fragments?.[key];
    if (!f || check?.problems?.[key]) return null;
    return (
      <details className="build-writes small" style={{ marginTop: 12 }}>
        <summary>What this step writes to agent.yaml</summary>
        <pre className="run-log">{f}</pre>
      </details>
    );
  };

  const actions = (
    <>
      <div className="row" style={{ marginTop: 12 }}>
        <Button disabled={!!busy} onClick={save}>{busy === "save" ? "Saving…" : "Save"}</Button>
        <Button disabled={!!busy} onClick={runValidate}>{busy === "validate" ? "Validating…" : "Save + Validate"}</Button>
        {/* One publish page: the form
            views end on the summary, where Submit is. */}
        <Button variant="primary" disabled={!!busy} onClick={() => switchTo("summary")}>Continue to submit</Button>
      </div>
      <p className="muted small">
        {dir ? <>Saves into <span className="mono selectable">{dir}</span>.</> : <>The first save writes it to a new folder named for it in your output folder (Settings → Run defaults, or <span className="mono">agentry-out</span> in your Downloads folder). To edit one you already have, open its folder.</>}
      </p>
      {result && <ValidateCard result={result} />}
    </>
  );

  const outcome = (log.length > 0 || pubResult || localInstall) ? (
    <Card className="mt">
      {log.length > 0 && <pre ref={logRef} className="run-log" style={{ maxHeight: 180, overflow: "auto" }}>{log.join("\n")}</pre>}
      {pubResult && <PublishOutcome pub={pubResult} m={m} registry={registry} onError={setError} />}
      {localInstall && <LocalInstallOutcome v={localInstall} onOpenLibrary={onOpenLibrary} />}
    </Card>
  ) : null;

  const header = (
    <PageHeader
      title="Build"
      sub={view === "start"
        ? BUILD_TAGLINE
        : <>Author a QuickAgent, run the registry&apos;s own checks on it, and publish it. The form edits a real workspace (<span className="mono">agent.yaml</span> + instructions) that the <span className="mono">agentry</span> CLI and git work on too.</>}
      actions={
        <>
          {view === "steps" && <Button onClick={() => switchTo("advanced")}>Switch to Advanced</Button>}
          {view === "advanced" && <Button onClick={() => switchTo("steps")}>Switch to step by step</Button>}
          <Button onClick={openFolder} disabled={!!busy}>Open folder…</Button>
        </>
      }
    />
  );

  const editingNotice = editingNow && (view === "steps" || view === "advanced" || view === "summary") ? (
    <Notice tone="info">
      {editingLine(editingNow, m.version)}{" "}
      {onOpenAgent && <Button variant="link" onClick={() => onOpenAgent(editingNow.namespace, editingNow.name)}>Open its page</Button>}
    </Notice>
  ) : null;

  const notices = (
    <>
      <ErrorBox error={error} />
      {notice && <Notice tone="info">{notice}</Notice>}
      {editingNotice}
      {busy === "edit" && <Spinner label="Reading the release into the form…" />}
      {drafted && (view === "steps" || view === "advanced") && (
        <Notice tone="info">The Drafter made this draft from your description. It is a starting point: check every step, and what it may reach on Review, before you save or publish it.</Notice>
      )}
      {m.hadComments && view !== "start" && (
        <Notice tone="warn">This agent.yaml has comments. Saving writes the file from the form, and the comments are not kept.</Notice>
      )}
    </>
  );

  if (view === "summary") {
    const first = check?.first ?? null;
    const label = first ? STEPS[stepOf(first.section as StepKey)]?.label ?? first.section : "";
    return (
      <div>
        {header}
        <ErrorBox error={error} />
        {notice && <Notice tone="info">{notice}</Notice>}
        {editingNotice}
        {drafted?.revision && revisionShown ? (
          <RevisionView v={drafted} busy={busy} outcome={outcome} onPublish={publishRevision}
            onOpenInBuilder={() => switchTo("advanced")} onOpenAgent={onOpenAgent} />
        ) : summary ? (
          <SummaryView
            s={summary}
            check={drafted?.check?.line ?? null}
            problem={first ? { label, text: first.problem, fix: () => { switchTo("steps"); setAt(stepOf(first.section as StepKey)); } } : null}
            publicProblem={check?.public ? { label: STEPS[stepOf("publish")].label, text: check.public, fix: () => { switchTo("steps"); setAt(stepOf("publish")); } } : null}
            onForm={toForm}
            onSave={save}
            onInstall={installHere}
            onSubmit={publish}
            onReview={reviewHere}
            onOpenReviewKey={onOpenReviewKey}
            busy={busy}
            registry={registry}
            ready={!!check && !busy}
            dir={dir}
            headingRef={summaryHeading}
            outcome={outcome}
            shipNotes={shipNotes}
          />
        ) : !error && <Spinner />}
      </div>
    );
  }

  if (view === "describe") {
    return (
      <div>
        {header}
        {notices}
        <DescribePanel registry={registry} onDrafted={takeDraft} onOpenSettings={onOpenSettings} />
        <p className="small"><Button variant="link" onClick={() => switchTo("start")}>Back to the ways in</Button></p>
      </div>
    );
  }

  if (view === "start") {
    return (
      <div>
        {header}
        {notices}
        {/* Two cards, the site's /build's: Describe it to AI first, then
            Build it by hand, which opens Step by step and offers Advanced.
            Their sentences are the same on both clients. */}
        <BuildWays onDescribe={() => switchTo("describe")} onSteps={() => switchTo("steps")} onAdvanced={() => switchTo("advanced")} />
        {touched && (
          <p className="small">
            {m.name ? <><span className="mono">{m.name}</span> is still in the form{dir ? <> (saving into <span className="mono">{dir}</span>)</> : ""}.</> : "The form still holds your work."}{" "}
            {drafted?.revision && revisionShown && (
              <>
                It is Edit with AI&apos;s revision of {drafted.revision.namespace}/{drafted.revision.name} {drafted.revision.from}:{" "}
                <Button variant="link" onClick={() => switchTo("summary")}>see what changed and publish it</Button>, or continue it{" "}
              </>
            )}
            {!(drafted?.revision && revisionShown) && "Continue it "}
            <Button variant="link" onClick={() => switchTo("steps")}>step by step</Button> or in{" "}
            <Button variant="link" onClick={() => switchTo("advanced")}>Advanced</Button>, or{" "}
            <Button variant="link" onClick={startOver}>start a new one</Button>.
          </p>
        )}
        <p className="muted small">To change a QuickAgent you already have, open its folder: it loads into the form.</p>
      </div>
    );
  }

  if (view === "advanced") {
    return (
      <div>
        {header}
        {notices}
        {STEPS.filter((s) => s.key !== "review").map((s) => (
          <section key={s.key}>
            <h2>{s.label}</h2>
            <p className="muted small">{s.intro}</p>
            <div className="card">{sections[s.key as SectionKey]}</div>
            {check?.problems?.[s.key] && <p className="step-problem">{check.problems[s.key]}</p>}
          </section>
        ))}
        {actions}
      </div>
    );
  }

  // Step by step.
  const step = STEPS[at];
  const net = networkReach(m);
  const local = localReach(m);
  const g = m.guide;
  const lines = m.instructions.replace(/\n+$/, "").split("\n").length;
  return (
    <div>
      {header}
      {notices}
      <StepList labels={STEPS.map((s) => s.label)} at={at} />
      <section aria-labelledby="build-step-heading">
        <StepHeading ref={heading} id="build-step-heading" at={at} total={STEPS.length} label={step.label} />
        <p className="muted small">{step.intro}</p>
        {step.key !== "review" ? (
          <>
            <div className="card">{sections[step.key]}</div>
            {writes(step.key)}
          </>
        ) : (
          <>
            {drafted && (
              // Folded: the summary
              // leaves the notes out, and Review keeps them one click away.
              <details className="card mt build-drafted">
                <summary><strong>How it was drafted</strong></summary>
                <DraftNotes v={drafted} onOpenAgent={onOpenAgent} />
              </details>
            )}
            {drafted && <DescribeAgain registry={registry} v={drafted} model={m} onStarted={() => switchTo("describe")} />}
            <Recap
              rows={[
                { label: "Basics", value: <><span className="mono">{m.name || "no name yet"}</span>: {sentence(m.summary) || "no summary yet."} Runs {(m.modes ?? []).join(" and ") || "in no mode yet"}.</>, onEdit: () => goTo(stepOf("basics")) },
                { label: "Instructions", value: m.instructions.trim() ? `${lines} ${lines === 1 ? "line" : "lines"} in ${m.instructionFile || "AGENT.md"}` : "Not written yet", onEdit: () => goTo(stepOf("instructions")) },
                { label: "Tools and web access", value: `The internet: ${net[0].value}. ${(m.mcpServers ?? []).length === 0 ? "No MCP servers." : `${m.mcpServers.length} MCP ${m.mcpServers.length === 1 ? "server" : "servers"}.`}`, onEdit: () => goTo(stepOf("tools")) },
                { label: "Files, commands and data", value: local.map((r) => `${r.what}: ${r.value}`).join(". ") + ".", onEdit: () => goTo(stepOf("access")) },
                { label: "Questions", value: (m.intake ?? []).length === 0 ? "None" : m.intake.map((q) => q.id).join(", "), onEdit: () => goTo(stepOf("questions")) },
                {
                  label: "Before a run",
                  value: g && (g.overview || (g.prepare ?? []).some((s) => s.trim()) || g.files || g.task || (g.connect ?? []).length) ? (
                    <>
                      Written
                      <KeyHelpList rows={g.connect ?? []} />
                    </>
                  ) : "Not written: the run setup uses the summary and the contract",
                  onEdit: () => goTo(stepOf("guide")),
                },
                { label: "Publishing", value: <><span className="mono">{m.namespace || "?"}/{m.name || "?"}@{m.version || "?"}</span>, public once submitted, licence {m.license || "none"}</>, onEdit: () => goTo(stepOf("publish")) },
              ]}
            />
            <ReachList title="What it may reach" rows={[...net, ...local]} />
            <ServerChecks m={m} checks={serverChecks} />
            <details className="build-writes small" style={{ marginTop: 12 }}>
              <summary>The agent.yaml this will write</summary>
              {check?.first
                ? <p className="muted">It appears here once every step passes. First: {check.first.problem}.</p>
                : <pre className="run-log">{check?.yaml ?? ""}</pre>}
            </details>
            {(check?.warnings ?? []).length > 0 && (
              <Notice tone="warn">{check!.warnings.join(" · ")}</Notice>
            )}
            {actions}
          </>
        )}
        <StepFoot problem={stepProblem} onBack={at > 0 ? () => goTo(at - 1) : undefined}>
          {step.key !== "review" && <Button variant="primary" onClick={next}>Next: {STEPS[at + 1].label}</Button>}
        </StepFoot>
      </section>
    </div>
  );
}

// ValidateCard shows a validate run the way the Validate page does: the
// manifest's V-rule problems, the inventory's and the content scan's
// findings with their rule and place.
function ValidateCard({ result }: { result: validate.Result }) {
  const issues = result.issues ?? [];
  const inventory = result.inventory ?? [];
  const findings = result.findings ?? [];
  return (
    <Card className="mt">
      <strong>{result.passed ? "validate: PASSED" : "validate: FAILED"}</strong>
      {result.manifestError && <div className="error-box selectable">{result.manifestError}</div>}
      {issues.map((iss, i) => (
        <div key={`i${i}`} className="finding">
          <div className="row">
            <Sev severity={iss.warning ? "medium" : "high"} />
            <span className="mono">{iss.rule}{iss.path ? ` (${iss.path})` : ""}</span>
            <span className="badge">{iss.warning ? "warning" : "error"}</span>
          </div>
          <div className="selectable">{iss.message}</div>
        </div>
      ))}
      {inventory.map((f, i) => (
        <div key={`v${i}`} className="finding">
          <div className="row">
            <Sev severity={f.severity} />
            <span className="mono">{f.scanner}</span>
            <strong>{f.title}</strong>
          </div>
          <div className="loc mono">{f.filePath}</div>
        </div>
      ))}
      {findings.map((f, i) => (
        <div key={`f${i}`} className="finding">
          <div className="row">
            <Sev severity={f.severity} />
            <span className="mono">{f.ruleId}</span>
            <strong>{f.title}</strong>
          </div>
          {f.filePath && <div className="loc mono">{f.filePath}{f.lineStart ? `:${f.lineStart}` : ""}</div>}
        </div>
      ))}
      {result.passed && <p className="muted small">The same manifest rules and content scanners the registry runs at publish time.</p>}
    </Card>
  );
}

function LocalInstallOutcome({ v, onOpenLibrary }: { v: app.LocalInstallView; onOpenLibrary?: () => void }) {
  return (
    <div className="selectable">
      <strong>{v.repaired ? "laid down again" : "installed"} {v.ref} on this computer — local and unsigned</strong>
      <p className="muted small">
        No registry holds it and nothing was submitted. It is in your Library, where Run starts it; every run asks you to
        accept that it is unsigned. Its archive digest is <span className="mono">{v.digest}</span>.
      </p>
      {(v.warnings ?? []).map((w, i) => <Notice key={i} tone="warn">{w}</Notice>)}
      {onOpenLibrary && <p className="small"><Button onClick={onOpenLibrary}>Open Library</Button></p>}
    </div>
  );
}

function PublishOutcome({ pub, m, registry, onError }: { pub: build.PublishResult; m: build.Model; registry: string; onError: (e: string) => void }) {
  return (
    <div className="selectable">
      <strong>
        {pub.status === "published" && `published ${m.namespace}/${m.name}@${pub.version}`}
        {pub.status === "needs_review" && `submitted — ${m.namespace}/${m.name}@${pub.version} needs human review`}
        {pub.status === "rejected" && `rejected — ${m.namespace}/${m.name}@${pub.version}`}
        {pub.status === "refused" && "refused — the registry stored nothing"}
      </strong>
      {pub.detail && <p className="muted small">{pub.detail}</p>}
      {pub.status !== "refused" && pub.visibility === "public" && <p className="muted small">It is public.</p>}
      {pub.visibilityNote && <Notice tone="warn">{pub.visibilityNote}</Notice>}
      {pub.settingsUrl && (
        // A private QuickAgent is made public on its settings page on the
        // site, or by Make public in the Publisher section, which the note
        // names.
        <p className="small"><Button onClick={() => OpenExternal(pub.settingsUrl!).catch((e) => onError(String(e)))}>Its settings on the site</Button></p>
      )}
      {pub.status === "refused" && pub.account && (
        // Whose subscription the refusal is about: a member publishes to
        // their own namespace but spends from the organization's pool.
        <p className="muted small">the account is <span className="selectable">{pub.account}</span></p>
      )}
      {pub.status === "refused" && (pub.refusal === "credits-exhausted" || pub.refusal === "subscription-inactive") && (
        <p className="small">
          <Button onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)))}>
            {pub.refusal === "subscription-inactive" ? "The subscription" : "Credits and upgrade"}
          </Button>
        </p>
      )}
      {pub.findingCounts && Object.keys(pub.findingCounts).length > 0 && (
        <p className="muted small">findings: {Object.entries(pub.findingCounts).map(([s, n]) => `${s} ${n}`).join(" · ")}</p>
      )}
    </div>
  );
}
