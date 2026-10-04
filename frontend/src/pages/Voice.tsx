// The Voice page (plan writing-voice.md §8, D6, D11, D12; agentry-desktop
// #510, D-054; reshaped by plan voice-page-redesign.md §4, §7, §8,
// agentry-desktop#610, D-063): two cards, as the Build page's (DEV-201).
// Local Voice opens this computer's voice: its editor, Download Cloud
// version and Upload to Cloud (each warning in a native dialog when it
// would put an older voice over a newer one, D2), Edit with AI, the switch
// Settings' Run defaults also shows (D4), and Delete. Cloud Voice opens the
// account's voice, AI Review's status and its editor.
//
// The editor is a monospace textarea the person writes in and saves from,
// with a Preview tab beside it, never instead of it (the operator's
// condition on D12; plan voice-page-redesign.md M7 replaces it). Saving this
// computer's voice is free and immediate; saving the account's makes a
// version AI Review reads before it is live. Edit with AI is one dialog:
// the Voice Writer on this computer for no credits, an AI Edit with a note
// and optional samples, or a Create where this computer keeps no voice.
//
// Every gate is in Go (services_voice.go): samples come only from the
// native pickers, a replace, an upload over a newer version and a delete
// are asked in a native dialog, and a Voice Writer run here takes the run
// lock.
import { useCallback, useEffect, useState } from "react";
import * as VoiceAPI from "../../wailsjs/go/main/VoiceAPI";
import { ChooseDirectory, ChooseFiles } from "../../wailsjs/go/main/App";
import { main, voice } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Card, EmptyState, ErrorBox, Modal, Notice, PageHeader, Spinner, Textarea } from "../ui";
import { AgentMarkdown } from "../AgentMarkdown";
import { FoldedLog, StopButton } from "../progress";
import { WayCard } from "./BuildWays";
import { CloudVoiceFigure, LocalVoiceFigure } from "./VoiceFigures";
import {
  CLOUD_CARD_BLURB, LOCAL_CARD_BLURB, LOCAL_SWITCH_LABEL, MAX_VOICE_BYTES, MAX_VOICE_NOTE_BYTES, VOICE_ON, VOICE_TAGLINE,
  accountSource, cloudStatus, downloadButton, editWithAIProblem, findingText, localStatus, reviewCostLine, reviewWord,
  uploadButton, voiceTextProblem,
} from "../voice";

export type VoiceView = "start" | "local" | "cloud";

export default function Voice({ registry, onOpenSettings, initialView = "start", initialDialog = false }: {
  registry: string;
  /** Opens Settings at the voice's switch: "here" (Run defaults),
   *  "account" (Profile), or "sign-in" (Registry settings). */
  onOpenSettings: (which: "here" | "account" | "sign-in") => void;
  /** The harness opens a page, or the Local page's dialog, directly. */
  initialView?: VoiceView;
  initialDialog?: boolean;
}) {
  const [page, setPage] = useState<VoiceView>(initialView);
  const [view, setView] = useState<main.VoiceView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const refresh = useCallback(() => {
    VoiceAPI.View(registry)
      .then((v) => {
        setView(v);
        setError(null);
      })
      .catch((e) => setError(String(e)));
  }, [registry]);
  useEffect(refresh, [refresh]);

  async function act(what: string, call: () => Promise<string | void>) {
    setBusy(what);
    setError(null);
    setNotice(null);
    try {
      const msg = await call();
      if (msg) setNotice(msg);
      refresh();
    } catch (e) {
      const s = String(e);
      // A No in the native dialog is not an error to show.
      if (!s.includes("not confirmed")) setError(s);
    } finally {
      setBusy(null);
    }
  }
  function open(next: VoiceView) {
    setNotice(null);
    setError(null);
    setPage(next);
  }

  if (!view) return error ? <ErrorBox error={error} /> : <Spinner label="Reading your voice…" />;
  const messages = (
    <>
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
    </>
  );

  if (page === "local") {
    return (
      <LocalVoice
        view={view}
        registry={registry}
        busy={busy}
        act={act}
        messages={messages}
        initialDialog={initialDialog}
        onBack={() => open("start")}
        onDone={(msg) => { setError(null); setNotice(msg); refresh(); }}
      />
    );
  }
  if (page === "cloud") {
    return (
      <div className="voice-page">
        <PageHeader title="Cloud Voice" sub={CLOUD_CARD_BLURB} actions={<Button onClick={() => open("start")}>Back to Voice</Button>} />
        {messages}
        {!view.signedIn ? (
          <Notice tone="info">
            Sign in to <span className="mono">{registry}</span> to keep a voice in your account, reviewed by AI Review, which runs In Agentry write in.{" "}
            <Button variant="link" onClick={() => onOpenSettings("sign-in")}>Registry settings</Button>
          </Notice>
        ) : !view.accountOffered ? (
          <Notice tone="info">This registry keeps no Writing Voices, so your voice stays on this computer.</Notice>
        ) : (
          <AccountVoice view={view} account={view.account} busy={busy} onOpenSettings={() => onOpenSettings("account")} act={act} registry={registry} />
        )}
      </div>
    );
  }
  return (
    <div className="voice-page">
      <PageHeader title="Voice" sub={VOICE_TAGLINE} />
      {messages}
      <div className="build-ways">
        <WayCard title="Local Voice" blurb={LOCAL_CARD_BLURB} figure={<LocalVoiceFigure />} onOpen={() => open("local")}>
          {localStatus(view.local)}
        </WayCard>
        <WayCard title="Cloud Voice" blurb={CLOUD_CARD_BLURB} figure={<CloudVoiceFigure />} onOpen={() => open("cloud")}>
          {cloudStatus(view)}
        </WayCard>
      </div>
    </div>
  );
}

// LocalVoice is the Local Voice page (plan §4.2): this computer's voice,
// Download Cloud version, Upload to Cloud and Edit with AI above its
// editor, the switch, and Delete at the foot.
function LocalVoice({ view, registry, busy, act, messages, initialDialog, onBack, onDone }: {
  view: main.VoiceView;
  registry: string;
  busy: string | null;
  act: (what: string, call: () => Promise<string | void>) => void;
  messages: React.ReactNode;
  initialDialog: boolean;
  onBack: () => void;
  onDone: (msg: string) => void;
}) {
  const [dialog, setDialog] = useState(initialDialog);
  const local = view.local;
  const exists = !!local?.exists;
  const opens = exists && !local.problem;
  const download = downloadButton(view);
  const upload = uploadButton(view);
  const saveHere = (text: string) =>
    act("save-here", async () => {
      const r = await VoiceAPI.SaveLocal(registry, text);
      return r.created ? `Saved on this computer. ${VOICE_ON}` : "Saved on this computer.";
    });
  return (
    <div className="voice-page">
      <PageHeader title="Local Voice" sub={LOCAL_CARD_BLURB} actions={<Button onClick={onBack}>Back to Voice</Button>} />
      {messages}
      <p className="muted small">{localStatus(local)} Kept sealed in the runner home and never reviewed.</p>
      {local?.problem && <Notice tone="warn">{local.problem}</Notice>}

      <div className="voice-actions">
        <div className="voice-action">
          <Button
            disabled={download.disabled}
            busy={busy === "download"}
            busyLabel="Downloading…"
            onClick={() => act("download", async () => {
              const r = await VoiceAPI.UseAccount(registry);
              return r.created ? `Your account's live voice is kept on this computer. ${VOICE_ON}` : "Your account's live voice is kept on this computer.";
            })}
          >
            Download Cloud version
          </Button>
          <p className="muted small">{download.line}</p>
        </div>
        <div className="voice-action">
          <Button
            disabled={upload.disabled}
            busy={busy === "upload"}
            busyLabel="Uploading…"
            onClick={() => act("upload", async () => {
              await VoiceAPI.Submit(registry);
              return "Uploaded to your account. AI Review reads it before it is live there; this computer's voice is unchanged.";
            })}
          >
            Upload to Cloud
          </Button>
          <p className="muted small">{upload.line}</p>
        </div>
        <div className="voice-action">
          <Button variant="primary" disabled={exists && !opens} onClick={() => setDialog(true)}>
            {exists ? "Edit with AI" : "Write with AI"}
          </Button>
          <p className="muted small">
            {exists ? "Tell the Voice Writer what should change. Free, on this computer." : "The Voice Writer writes your voice from samples of your writing. Free, on this computer."}
          </p>
        </div>
      </div>

      {opens && (
        <label className="check-row mt">
          <input
            type="checkbox"
            checked={local.alwaysAttach}
            disabled={busy === "attach"}
            onChange={(e) => {
              const on = e.target.checked;
              act("attach", async () => {
                await VoiceAPI.SetLocalAttach(registry, on);
                return on ? "Every run on this computer writes in your voice unless its setup says otherwise." : "Runs on this computer write in your voice only when their setup says so.";
              });
            }}
          />
          <span>
            {LOCAL_SWITCH_LABEL}
            <span className="muted small"> — the same switch as Settings → Run defaults. Every run's setup shows Write in my voice, ticked from it.</span>
          </span>
        </label>
      )}

      <Card>
        {opens ? (
          <VoiceEditor key={`here:${local.digest}`} initial={local.text} note="Save keeps it on this computer at once, for no credits." busy={busy === "save-here"} onSave={saveHere} />
        ) : !exists ? (
          <>
            <EmptyState title="No voice on this computer yet.">
              Write with AI from samples of your writing, download your account's, or write one by hand here.
            </EmptyState>
            <VoiceEditor initial="" note="Save keeps it on this computer at once, for no credits." busy={busy === "save-here"} onSave={saveHere} />
          </>
        ) : null}
      </Card>

      {exists && (
        <div className="row gap mt">
          <Button variant="danger" busy={busy === "delete-here"} onClick={() => act("delete-here", async () => {
            await VoiceAPI.DeleteLocal(registry);
            return "This computer's voice is deleted.";
          })}>
            Delete this computer's voice
          </Button>
        </div>
      )}

      {dialog && (
        <EditWithAI
          registry={registry}
          hasVoice={exists}
          model={view.model}
          onClose={() => setDialog(false)}
          onDone={(msg) => { setDialog(false); onDone(msg); }}
        />
      )}
    </div>
  );
}

// EditWithAI is the one dialog of plan §7 (D3) on this computer: what
// should change and optional samples from the native pickers for an AI
// Edit, or samples and an optional brief for a Create where there is no
// voice here. It shows the run's progress and Stop, and the page reloads
// the voice into the editor when it ends.
function EditWithAI({ registry, hasVoice, model, onClose, onDone }: {
  registry: string;
  hasVoice: boolean;
  model: string;
  onClose: () => void;
  onDone: (msg: string) => void;
}) {
  const [text, setText] = useState("");
  const [samples, setSamples] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [log, setLog] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => EventsOn("voice:write-log", (line: string) => setLog((p) => [...p.slice(-199), line])), []);

  async function pickFiles() {
    const picked = await ChooseFiles("Samples of your writing");
    if (picked?.length) setSamples((d) => [...d, ...picked.filter((p) => !d.includes(p))]);
  }
  async function pickFolder() {
    const dir = await ChooseDirectory("A folder of your writing");
    if (dir) setSamples((d) => (d.includes(dir) ? d : [...d, dir]));
  }
  async function start() {
    setError(null);
    setLog([]);
    setStopping(false);
    setRunning(true);
    try {
      const res = await VoiceAPI.WriteWithAI(registry, text, samples);
      onDone(res.created ? `The Voice Writer kept your voice on this computer. ${VOICE_ON}` : "The Voice Writer kept your new voice on this computer.");
    } catch (e) {
      setRunning(false);
      const s = String(e);
      if (!s.includes("not confirmed")) setError(s);
    }
  }

  const problem = editWithAIProblem(hasVoice, text, samples.length);
  return (
    <Modal title={hasVoice ? "Edit with AI" : "Write with AI"} onClose={running ? () => {} : onClose}>
      {running ? (
        <>
          <Spinner label="The Voice Writer is running on this computer…" />
          <p className="muted small">It reads {hasVoice ? "your voice, what should change" : "your samples"}{hasVoice && samples.length > 0 ? " and your samples" : ""}, then writes VOICE.md. When it ends, the voice here is replaced with what it wrote.</p>
          {log.length > 0 && <FoldedLog label="The Voice Writer's log" lines={log} />}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <StopButton stopping={stopping} onStop={() => { setStopping(true); VoiceAPI.StopWrite().catch(() => setStopping(false)); }} />
          </div>
        </>
      ) : (
        <>
          <p className="muted small">
            {hasVoice
              ? "Tell the Voice Writer what should change in your voice. Add samples of your writing if they help it."
              : "The Voice Writer reads samples of your writing and writes your voice as instructions an LLM can follow."}
          </p>
          <div className="field">
            <label className="field-label" htmlFor="voice-ai-text">{hasVoice ? "What should change?" : "How do you want to sound? (optional)"}</label>
            <Textarea
              id="voice-ai-text"
              rows={4}
              value={text}
              maxLength={MAX_VOICE_NOTE_BYTES}
              placeholder={hasVoice ? "Shorter sentences, and no exclamation marks." : "Plain and direct; leave out the jokes."}
              onChange={(e) => setText(e.target.value)}
            />
          </div>
          <div className="field">
            <div className="field-label">{hasVoice ? "Add writing samples (optional)" : "Samples of your writing"}</div>
            <div className="row gap">
              <Button onClick={pickFiles}>Choose files…</Button>
              <Button onClick={pickFolder}>Choose a folder…</Button>
            </div>
            {samples.length > 0 && (
              <ul className="small mono selectable">
                {samples.map((s) => <li key={s}>{s} <Button variant="link" onClick={() => setSamples((d) => d.filter((x) => x !== s))}>Remove</Button></li>)}
              </ul>
            )}
          </div>
          <p className="small">Free, on this computer: it runs on {model} for no credits, and the voice it writes is kept here, not reviewed.</p>
          <ErrorBox error={error} />
          <div className="row gap" style={{ justifyContent: "flex-end" }}>
            <Button onClick={onClose}>Cancel</Button>
            <Button variant="primary" disabled={problem !== null} onClick={start}>Start</Button>
          </div>
          {problem && (text.trim() !== "" || samples.length > 0) && <p className="muted small">{problem}</p>}
        </>
      )}
    </Modal>
  );
}

function AccountVoice({ view, account, busy, onOpenSettings, act, registry }: {
  view: main.VoiceView;
  account?: voice.Account;
  busy: string | null;
  onOpenSettings: () => void;
  act: (what: string, call: () => Promise<string | void>) => void;
  registry: string;
}) {
  if (view.accountError) return <Card><h2>In Agentry</h2><ErrorBox error={view.accountError} /></Card>;
  const live = account?.live;
  const pending = account?.pending;
  // The editor holds the newest version: a pending one to repair or finish,
  // else the live one.
  const draft = pending?.text ?? live?.text ?? "";
  const saveNote = reviewCostLine(view.reviewCredits, !!live);
  return (
    <Card>
      <div className="row spread">
        <h2>In Agentry</h2>
        {live && <span className="badge kind">{account?.alwaysAttach ? "On for every run In Agentry" : "Off by default"}</span>}
      </div>
      <p className="muted small">Your account's voice. AI Review reads every version before it is live, and runs In Agentry write in the live one.</p>
      {live && (
        <Notice tone={account?.alwaysAttach ? "ok" : "info"}>
          {account?.alwaysAttach
            ? "Your voice is on for every run In Agentry. You can turn it off in Settings."
            : "Your voice is off by default for runs In Agentry. A run's setup can still tick Write in my voice."}
        </Notice>
      )}
      {pending && (
        <div className="voice-pending">
          <p>
            <span className={`badge ${pending.review === "flagged" ? "bad" : "warn"}`}>{reviewWord(pending.review)}</span>{" "}
            {pending.review === "flagged"
              ? live ? "AI Review flagged your new version, so your live voice stays live. Edit it and save it again." : "AI Review flagged this version, so it is not live. Edit it and save it again."
              : pending.verdict === "incomplete"
                ? "AI Review did not finish reading this version; Agentry tries again on its own."
                : "AI Review is reading this version. Reopen this page to see its outcome."}
          </p>
          {pending.review === "flagged" && (pending.findings?.length ?? 0) > 0 && (
            <ul className="small">
              {[...(pending.findings ?? [])].sort((a, b) => Number(b.flags) - Number(a.flags)).map((f, i) => (
                <li key={i}>{findingText(f)}{!f.flags && <span className="muted"> (noted, does not keep it from going live)</span>}</li>
              ))}
            </ul>
          )}
        </div>
      )}
      {live ? (
        <>
          <p className="muted small">
            Live: {accountSource(live.source)}, {live.bytes.toLocaleString("en-GB")} bytes.{" "}
            <Button variant="link" onClick={onOpenSettings}>Profile</Button>
          </p>
          <details>
            <summary className="small">The live voice</summary>
            <div className="readme selectable"><AgentMarkdown text={live.text} /></div>
          </details>
        </>
      ) : !pending ? (
        <p className="muted">Your account keeps no Writing Voice yet. Upload this computer's from the Local Voice page, or write one below.</p>
      ) : null}
      <VoiceEditor
        key={`account:${pending?.digest ?? live?.digest ?? "none"}`}
        initial={draft}
        note={saveNote}
        busy={busy === "save-account"}
        onSave={(text) => act("save-account", async () => {
          await VoiceAPI.SaveAccount(registry, text);
          return "Sent to AI Review. It goes live in your account when it passes.";
        })}
      />
    </Card>
  );
}

// VoiceEditor is a monospace textarea with Save on it, and Preview beside
// it, rendered with the hostile-markdown renderer (D12).
function VoiceEditor({ initial, note, busy, onSave }: {
  initial: string;
  note: string;
  busy: boolean;
  onSave: (text: string) => void;
}) {
  const [tab, setTab] = useState<"write" | "preview">("write");
  const [text, setText] = useState(initial);
  const [problem, setProblem] = useState<string | null>(null);
  const bytes = new TextEncoder().encode(text).length;
  return (
    <div className="voice-editor">
      <div className="tabs" role="tablist" aria-label="The voice editor">
        <button role="tab" aria-selected={tab === "write"} className={`tab ${tab === "write" ? "active" : ""}`} onClick={() => setTab("write")}>Write</button>
        <button role="tab" aria-selected={tab === "preview"} className={`tab ${tab === "preview" ? "active" : ""}`} onClick={() => setTab("preview")}>Preview</button>
      </div>
      {tab === "write" ? (
        <>
          <Textarea
            className="mono"
            rows={14}
            spellCheck={false}
            aria-label="VOICE.md"
            value={text}
            onChange={(e) => { setText(e.target.value); setProblem(null); }}
          />
          <div className="muted small">{Math.ceil(bytes / 1024)} KiB of {MAX_VOICE_BYTES / 1024} KiB</div>
        </>
      ) : (
        <div className="readme selectable">
          {text.trim() ? <AgentMarkdown text={text} /> : <p className="muted">Nothing to preview yet.</p>}
        </div>
      )}
      {problem && <p className="run-step-problem" role="alert">{problem}</p>}
      <p className="muted small">{note}</p>
      <Button
        variant="primary"
        busy={busy}
        busyLabel="Saving…"
        onClick={() => {
          const p = voiceTextProblem(text);
          if (p) {
            setTab("write");
            setProblem(p);
            return;
          }
          onSave(text);
        }}
      >
        Save
      </Button>
    </div>
  );
}

// VoiceSwitch is a voice's always-attach switch, for Settings: this
// computer's under Run defaults, the account's under Profile (D11).
export function VoiceSwitch({ registry, place }: { registry: string; place: "here" | "account" }) {
  const [view, setView] = useState<main.VoiceView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const read = useCallback(() => {
    Promise.resolve()
      .then(() => VoiceAPI.View(registry))
      .then(setView)
      .catch((e) => setError(String(e)));
  }, [registry]);
  useEffect(read, [read]);
  if (error) return <ErrorBox error={error} />;
  if (!view) return <Spinner />;
  const here = place === "here";
  const exists = here ? !!view.local?.exists : !!view.account?.exists;
  const on = here ? !!view.local?.alwaysAttach : !!view.account?.alwaysAttach;
  if (!here && !view.accountOffered) return <p className="muted small">This registry keeps no Writing Voices.</p>;
  if (!exists) {
    return <p className="muted small">{here ? "This computer keeps no Writing Voice yet." : "Your account keeps no Writing Voice yet."} Make one on the Voice page.</p>;
  }
  async function set(next: boolean) {
    setError(null);
    setSaved(null);
    try {
      if (here) await VoiceAPI.SetLocalAttach(registry, next);
      else await VoiceAPI.SetAccountAttach(registry, next);
      setSaved(next ? "Every run writes in your voice unless its setup says otherwise." : "Runs write in your voice only when their setup says so.");
      read();
    } catch (e) {
      setError(String(e));
    }
  }
  return (
    <Card>
      <label className="check-row">
        <input type="checkbox" checked={on} onChange={(e) => set(e.target.checked)} />
        <span>{here ? "Write every run on this computer in this computer's voice" : "Write every run In Agentry in my account's voice"}</span>
      </label>
      <div className="muted small">Every run's setup shows Write in my voice, ticked from this switch, and you can untick it for one run.</div>
      {saved && <div className="small">{saved}</div>}
    </Card>
  );
}
