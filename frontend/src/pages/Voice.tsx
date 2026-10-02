// The Voice panel (plan writing-voice.md §8, D6, D11, D12; agentry-desktop
// #510, D-054): the Writing Voice kept on this computer and, signed in, the
// one the account keeps in Agentry. Two places, never synced on their own:
// Submit to Agentry sends this computer's for review, and Use Agentry's
// voice keeps the account's live one here, asking first in a native dialog
// when it replaces a different one.
//
// The editor is a monospace textarea the person writes in and saves from,
// with a Preview tab beside it, never instead of it (the operator's
// condition on D12). Saving this computer's voice is free and immediate;
// saving the account's makes a version AI Review reads before it is live.
// The Voice Writer writes or rewrites a voice on this computer for no
// credits, or In Agentry at the registry's price.
//
// Every gate is in Go (services_voice.go): samples come only from the
// native pickers, a replace or a delete is asked in a native dialog, and a
// Voice Writer run here takes the run lock.
import { useCallback, useEffect, useState } from "react";
import * as VoiceAPI from "../../wailsjs/go/main/VoiceAPI";
import { ChooseDirectory, ChooseFiles, OpenExternal } from "../../wailsjs/go/main/App";
import { main, voice } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Card, EmptyState, ErrorBox, Notice, PageHeader, Spinner, Textarea, fmtDate } from "../ui";
import { AgentMarkdown } from "../AgentMarkdown";
import { MAX_VOICE_BYTES, VOICE_ON, accountSource, findingText, hostedVoiceCost, reviewCostLine, reviewWord, submitCostLine, voiceTextProblem } from "../voice";

type Job = "create" | "improve" | "edit";

export default function Voice({ registry, onOpenSettings }: {
  registry: string;
  /** Opens Settings at the voice's switch: "here" (Run defaults),
   *  "account" (Profile), or "sign-in" (Registry settings). */
  onOpenSettings: (which: "here" | "account" | "sign-in") => void;
}) {
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

  if (!view) return error ? <ErrorBox error={error} /> : <Spinner label="Reading your voice…" />;
  const saveHere = (text: string) =>
    act("save-here", async () => {
      const r = await VoiceAPI.SaveLocal(registry, text);
      return r.created ? `Saved on this computer. ${VOICE_ON}` : "Saved on this computer.";
    });
  const local = view.local;
  const account = view.account;

  return (
    <div className="voice-page">
      <PageHeader
        title="Voice"
        sub="Your Writing Voice is one VOICE.md: the instructions a run follows to write the way you do. A run that writes in it is told the voice changes how it writes, never what it does."
      />
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}

      <Card>
        <div className="row spread">
          <h2>On this computer</h2>
          {local?.exists && <span className="badge kind">{local.alwaysAttach ? "On for every run here" : "Off by default"}</span>}
        </div>
        <p className="muted small">Kept sealed in the runner home and never reviewed. Making and changing it here costs no credits.</p>
        {local?.problem && <Notice tone="warn">{local.problem}</Notice>}
        {local?.exists && !local.problem ? (
          <>
            <p className="muted small">
              {local.phrase[0].toUpperCase() + local.phrase.slice(1)}, {local.bytes.toLocaleString("en-GB")} bytes, updated {fmtDate(local.updated)}.{" "}
              <Button variant="link" onClick={() => onOpenSettings("here")}>Run defaults</Button>
            </p>
            <VoiceEditor
              key={`here:${local.digest}`}
              initial={local.text}
              note="Save keeps it on this computer at once, for no credits."
              busy={busy === "save-here"}
              onSave={saveHere}
            />
            {view.accountOffered && <p className="muted small">{submitCostLine(view.reviewCredits)}</p>}
            <div className="row gap mt">
              {view.accountOffered && (
                <Button busy={busy === "submit"} busyLabel="Submitting…" onClick={() => act("submit", async () => {
                  await VoiceAPI.Submit(registry);
                  return "Submitted to Agentry. AI Review reads it before it is live in your account; this computer's voice is unchanged.";
                })}>
                  Submit to Agentry
                </Button>
              )}
              <Button variant="danger" busy={busy === "delete-here"} onClick={() => act("delete-here", async () => {
                await VoiceAPI.DeleteLocal(registry);
                return "This computer's voice is deleted.";
              })}>
                Delete
              </Button>
            </div>
          </>
        ) : !local?.exists ? (
          <>
            <EmptyState title="No voice on this computer yet.">
              Have the Voice Writer write one from samples of your writing, below, or write one by hand here.
            </EmptyState>
            <VoiceEditor
              initial=""
              note="Save keeps it on this computer at once, for no credits."
              busy={busy === "save-here"}
              onSave={saveHere}
            />
          </>
        ) : null}
      </Card>

      {!view.signedIn ? (
        <Notice tone="info">
          Sign in to <span className="mono">{registry}</span> to keep a voice in your account, reviewed by AI Review, which runs In Agentry write in.{" "}
          <Button variant="link" onClick={() => onOpenSettings("sign-in")}>Registry settings</Button>
        </Notice>
      ) : !view.accountOffered ? (
        <Notice tone="info">This registry keeps no Writing Voices, so your voice stays on this computer.</Notice>
      ) : (
        <AccountVoice
          view={view}
          account={account}
          busy={busy}
          onOpenSettings={() => onOpenSettings("account")}
          act={act}
          registry={registry}
        />
      )}

      <VoiceWriter view={view} registry={registry} onDone={(msg) => { if (msg) setNotice(msg); refresh(); }} />
    </div>
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
                : "AI Review is reading this version. Reopen the panel to see its outcome."}
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
        <p className="muted">Your account keeps no Writing Voice yet. Submit this computer's, write one below, or have the Voice Writer write it In Agentry.</p>
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
      {live && (
        <div className="row gap mt">
          <Button busy={busy === "use"} busyLabel="Keeping it here…" onClick={() => act("use", async () => {
            const r = await VoiceAPI.UseAccount(registry);
            return r.created ? `Your account's live voice is kept on this computer. ${VOICE_ON}` : "Your account's live voice is kept on this computer.";
          })}>
            Use Agentry's voice here
          </Button>
        </div>
      )}
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

// VoiceWriter starts the Voice Writer: Create, Improve or AI Edit, on this
// computer (free) or In Agentry (the registry's price).
function VoiceWriter({ view, registry, onDone }: { view: main.VoiceView; registry: string; onDone: (msg?: string) => void }) {
  const hasHere = !!view.local?.exists && !view.local.problem;
  const hasThere = !!view.account?.exists;
  const [job, setJob] = useState<Job>(hasHere || hasThere ? "improve" : "create");
  const [where, setWhere] = useState<"here" | "agentry">("here");
  const [samples, setSamples] = useState<string[]>([]);
  const [brief, setBrief] = useState("");
  const [avail, setAvail] = useState<main.VoiceHostedAvailabilityView | null>(null);
  const [running, setRunning] = useState(false);
  const [log, setLog] = useState<string[]>([]);
  const [hosted, setHosted] = useState<main.VoiceHostedView | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => EventsOn("voice:write-log", (line: string) => setLog((p) => [...p.slice(-199), line])), []);
  useEffect(() => EventsOn("voice:hosted", (v: main.VoiceHostedView) => {
    setHosted(v);
    if (v.final) {
      setRunning(false);
      onDone(v.applied ? undefined : v.note || v.outcomeLine);
    }
  }), [onDone]);
  useEffect(() => {
    if (!view.signedIn) return;
    VoiceAPI.HostedAvailability(registry, job).then(setAvail).catch(() => setAvail(null));
  }, [registry, job, view.signedIn]);

  const needsVoice = job !== "create";
  const hereOk = !needsVoice || hasHere;
  const agentryOk = !!avail?.available;
  const offered = !!avail?.offered;

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
    setHosted(null);
    setRunning(true);
    try {
      if (where === "here") {
        const res = await VoiceAPI.Write(registry, job, job === "edit" ? [] : samples, brief);
        setRunning(false);
        onDone(res.created ? `The Voice Writer kept your voice on this computer. ${VOICE_ON}` : "The Voice Writer kept your voice on this computer.");
      } else {
        const r = await VoiceAPI.StartHosted(registry, job, job === "edit" ? [] : samples, brief);
        if (r.refusal) {
          setRunning(false);
          setError(r.detail || r.refusal);
        } else if (r.run) {
          setHosted(r.run);
        }
      }
    } catch (e) {
      setRunning(false);
      setError(String(e));
    }
  }

  return (
    <Card>
      <h2>The Voice Writer</h2>
      <p className="muted small">
        It reads samples of your writing and writes your voice as instructions an LLM can follow. On this computer it runs on{" "}
        {view.model} for no credits; In Agentry it runs on Agentry's model and its voice waits for AI Review.
      </p>
      <div className="tabs" role="tablist" aria-label="What the Voice Writer does">
        {(["create", "improve", "edit"] as Job[]).map((j) => (
          <button key={j} role="tab" aria-selected={job === j} className={`tab ${job === j ? "active" : ""}`} onClick={() => setJob(j)}>
            {j === "create" ? "Create" : j === "improve" ? "Improve" : "AI Edit"}
          </button>
        ))}
      </div>
      <div className="mt" role="radiogroup" aria-label="Where the Voice Writer runs">
        <div className="field-label">Where</div>
        <label className="check-row">
          <input type="radio" name="voice-where" checked={where === "here"} disabled={!hereOk} onChange={() => setWhere("here")} />
          <span>
            <strong>On this computer</strong>{" "}
            <span className="muted">— against your runner policy's model; no credits, and the voice is kept here, not reviewed</span>
            {!hereOk && <span className="muted"> (this computer keeps no voice to {job === "edit" ? "edit" : "improve"})</span>}
          </span>
        </label>
        {offered && (
          <label className="check-row">
            <input type="radio" name="voice-where" checked={where === "agentry"} disabled={!agentryOk} onChange={() => setWhere("agentry")} />
            <span>
              <strong>In Agentry</strong>{" "}
              <span className="muted">— on the platform, for credits; the voice goes to your account and waits for AI Review</span>
            </span>
          </label>
        )}
        {offered && avail && !avail.available && (
          <p className="muted small">
            {avail.reason} {avail.remedy}{" "}
            {avail.upgradeUrl && <Button variant="link" onClick={() => OpenExternal(avail.upgradeUrl!)}>Upgrade</Button>}
          </p>
        )}
        {where === "agentry" && avail?.available && <p className="small">{hostedVoiceCost(avail)}</p>}
      </div>
      {job !== "edit" ? (
        <div className="field">
          <div className="field-label">{job === "create" ? "Samples of your writing" : "More samples of your writing"}</div>
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
      ) : null}
      <div className="field">
        <label className="field-label" htmlFor="voice-brief">{job === "edit" ? "What should change?" : "A brief (optional)"}</label>
        <Textarea
          id="voice-brief"
          rows={3}
          value={brief}
          maxLength={4096}
          placeholder={job === "edit" ? "Shorter sentences, and no exclamation marks." : "How you want to sound, or what to leave out."}
          onChange={(e) => setBrief(e.target.value)}
        />
      </div>
      <ErrorBox error={error} />
      <div className="row gap">
        <Button
          variant="primary"
          busy={running}
          busyLabel={where === "here" ? "The Voice Writer is running…" : "Running In Agentry…"}
          disabled={(job !== "edit" && samples.length === 0) || (job === "edit" && !brief.trim()) || (where === "here" ? !hereOk : !agentryOk)}
          onClick={start}
        >
          {job === "create" ? "Start the Voice Writer" : job === "improve" ? "Start Improve" : "Start AI Edit"}
        </Button>
        {running && where === "here" && <Button onClick={() => VoiceAPI.StopWrite().catch(() => {})}>Stop</Button>}
        {running && where === "agentry" && hosted && !hosted.final && (
          <Button onClick={() => VoiceAPI.CancelHosted().then(setHosted).catch((e) => setError(String(e)))}>Cancel run</Button>
        )}
      </div>
      {hosted && (
        <p className="small mt">
          {hosted.final ? hosted.outcomeLine || hosted.note || hosted.statusLine : hosted.statusLine}
          {hosted.final && hosted.settled && ` ${hosted.settledCredits} credits were settled.`}
        </p>
      )}
      {log.length > 0 && (
        <details className="knowledge-log" open={running}>
          <summary className="small">The Voice Writer's log</summary>
          <pre className="run-log">{log.join("\n")}</pre>
        </details>
      )}
    </Card>
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
    return <p className="muted small">{here ? "This computer keeps no Writing Voice yet." : "Your account keeps no Writing Voice yet."} Make one in the Voice panel.</p>;
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
