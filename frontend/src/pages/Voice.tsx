// The Voice page (plan writing-voice.md §8, D6, D11, D12; agentry-desktop
// #510, D-054; reshaped by plan voice-page-redesign.md §4, §7, §8,
// agentry-desktop#610, D-063): two cards, as the Build page's (DEV-201).
// Local Voice opens this computer's voice: its editor, Download Cloud
// version and Upload to Cloud (each warning in a native dialog when it
// would put an older voice over a newer one, D2), Edit with AI, the switch
// Settings' Run defaults also shows (D4), and Delete. Cloud Voice opens the
// account's voice as the site's Voice settings page has it (plan §4.3,
// §5.2, agentry-desktop#611): the version waiting for AI Review with its
// findings and Discard, the editor, Edit with AI run In Agentry, the
// switch Settings' Profile also shows, and Restore previous and Delete.
//
// The editor is a monospace textarea the person writes in and saves from,
// with a Preview tab beside it, never instead of it (the operator's
// condition on D12; plan voice-page-redesign.md M7 replaces it). Saving this
// computer's voice is free and immediate; saving the account's makes a
// version AI Review reads before it is live. Edit with AI is one dialog on
// each page: an AI Edit with a note and optional samples, or a Create where
// that place keeps no voice; on the Local page the Voice Writer runs on
// this computer for no credits, on the Cloud page In Agentry at the
// registry's flat price, and what it writes waits for AI Review.
//
// Every gate is in Go (services_voice.go): samples come only from the
// native pickers, a replace, an upload over a newer version, a discard and
// a delete are asked in a native dialog, and a Voice Writer run here takes
// the run lock.
import { type ReactNode, useCallback, useEffect, useState } from "react";
import * as VoiceAPI from "../../wailsjs/go/main/VoiceAPI";
import { ChooseDirectory, ChooseFiles, OpenExternal } from "../../wailsjs/go/main/App";
import { main, voice } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { Button, Card, EmptyState, ErrorBox, Modal, Notice, PageHeader, Spinner, Textarea } from "../ui";
import { AgentMarkdown } from "../AgentMarkdown";
import { FoldedLog, StopButton } from "../progress";
import { WayCard } from "./BuildWays";
import { CloudVoiceFigure, LocalVoiceFigure } from "./VoiceFigures";
import {
  CLOUD_CARD_BLURB, CLOUD_SWITCH_LABEL, LOCAL_CARD_BLURB, LOCAL_SWITCH_LABEL, MAX_VOICE_BYTES, MAX_VOICE_NOTE_BYTES, NOT_LIVE,
  NO_HOSTED_WRITER, VOICE_ON, VOICE_TAGLINE, accountJob, accountSource, cloudStatus, downloadButton, editWithAIProblem, findingText, hostedCostLine, hostedFailure,
  localStatus, pendingLine, restoreLine, reviewCostLine, uploadButton, versionLine, voiceStamp, voiceTextProblem,
} from "../voice";

export type VoiceView = "start" | "local" | "cloud";

export default function Voice({ registry, onOpenSettings }: {
  registry: string;
  /** Opens Settings at the voice's switch: "here" (Run defaults),
   *  "account" (Profile), or "sign-in" (Registry settings). */
  onOpenSettings: (which: "here" | "account" | "sign-in") => void;
}) {
  const [page, setPage] = useState<VoiceView>("start");
  const [view, setView] = useState<main.VoiceView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  // The Voice Writer run In Agentry this page started, followed here so it
  // outlives its dialog: its voice lands in the account, which is read
  // again when it ends.
  const [hosted, setHosted] = useState<main.VoiceHostedView | null>(null);

  const refresh = useCallback(() => {
    VoiceAPI.View(registry)
      .then((v) => {
        setView(v);
        setError(null);
      })
      .catch((e) => setError(String(e)));
  }, [registry]);
  useEffect(refresh, [refresh]);
  useEffect(() => EventsOn("voice:hosted", (v: main.VoiceHostedView) => {
    setHosted(v);
    if (!v.final) return;
    if (v.applied) {
      setError(null);
      setNotice(v.outcomeLine || "The Voice Writer wrote a new version of your voice. AI Review reads it before it is live.");
    } else {
      setError(hostedFailure(v));
    }
    refresh();
  }), [refresh]);
  // The Cloud page follows a version in review as the site's does: the
  // account is read again every 5 seconds until AI Review decides.
  const reviewing = page === "cloud" && view?.account?.pending?.review === "reviewing";
  useEffect(() => {
    if (!reviewing) return;
    const t = window.setInterval(refresh, 5000);
    return () => window.clearInterval(t);
  }, [reviewing, refresh]);

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
  // Each page reads both voices again: the account's can change elsewhere
  // (the site, the CLI, a review that passed), and the buttons' lines are
  // only as true as the last read.
  function open(next: VoiceView) {
    setNotice(null);
    setError(null);
    setPage(next);
    refresh();
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
        ) : view.accountError ? (
          <ErrorBox error={view.accountError} />
        ) : (
          <CloudVoice view={view} registry={registry} busy={busy} act={act} hosted={hosted} onHosted={setHosted} />
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
function LocalVoice({ view, registry, busy, act, messages, onBack, onDone }: {
  view: main.VoiceView;
  registry: string;
  busy: string | null;
  act: (what: string, call: () => Promise<string | void>) => void;
  messages: ReactNode;
  onBack: () => void;
  onDone: (msg: string) => void;
}) {
  const [dialog, setDialog] = useState(false);
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
          <SamplesPicker label={hasVoice ? "Add writing samples (optional)" : "Samples of your writing"} samples={samples} onChange={setSamples} />
          <p className="small">
            Free, on this computer: it runs on {model} for no credits, and the voice it writes is kept here, not reviewed.
            {hasVoice && " When it ends, it replaces the voice on this computer."}
          </p>
          <ErrorBox error={error} />
          {/* A run that failed keeps its log, so the person can see why. */}
          {log.length > 0 && <FoldedLog label="The last run's log" lines={log} />}
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

// CloudVoice is the Cloud Voice page (plan §4.3): the site's Voice
// settings page (§5.2) in the desktop. In order: the version waiting for
// AI Review, where there is one, with its findings and Discard; the editor
// on the newest version, with Edit with AI In Agentry under it; the switch
// Settings' Profile also shows (D4); Restore previous and Delete at the
// foot (D5). No Download or Upload: those are this computer's (D7).
function CloudVoice({ view, registry, busy, act, hosted, onHosted }: {
  view: main.VoiceView;
  registry: string;
  busy: string | null;
  act: (what: string, call: () => Promise<string | void>) => void;
  hosted: main.VoiceHostedView | null;
  onHosted: (v: main.VoiceHostedView | null) => void;
}) {
  const account = view.account;
  const live = account?.live;
  const pending = account?.pending;
  const has = !!(live || pending);
  const job = accountJob(account);
  const [avail, setAvail] = useState<main.VoiceHostedAvailabilityView | null>(null);
  const [availError, setAvailError] = useState(false);
  const [dialog, setDialog] = useState(false);
  // A Voice Writer run In Agentry already in flight (started before the
  // page was left, or before a restart) is followed again, not offered a
  // second time.
  useEffect(() => {
    VoiceAPI.AttachHosted(registry).then((r) => { if (r.run) onHosted(r.run); }).catch(() => {});
  }, [registry, onHosted]);
  useEffect(() => {
    setAvailError(false);
    VoiceAPI.HostedAvailability(registry, job).then(setAvail).catch(() => { setAvail(null); setAvailError(true); });
  }, [registry, job]);
  const running = !!hosted && !hosted.final;
  // The editor opens on the person's newest text: a pending version to
  // repair or finish, else the live one.
  const draft = pending?.text ?? live?.text ?? "";
  return (
    <>
      {running && !dialog && (
        <Notice tone="info">
          The Voice Writer is running In Agentry: {hosted.statusLine}{" "}
          <Button variant="link" onClick={() => setDialog(true)}>Show it</Button>
        </Notice>
      )}
      {pending && (
        <PendingVersion
          version={pending}
          live={!!live}
          busy={busy === "discard"}
          onDiscard={() => act("discard", async () => {
            await VoiceAPI.DiscardAccount(registry);
            return live ? "Discarded. Your live voice stays live." : "Discarded. Your account keeps no voice that is live.";
          })}
        />
      )}

      <Card>
        <div className="row spread">
          <h2>Your voice</h2>
          {live && <span className="badge ok">Live</span>}
        </div>
        {live ? (
          <p className="muted small">{versionLine(live)}</p>
        ) : (
          <p className="muted small">
            {pending
              ? NOT_LIVE
              : "Your account keeps no Writing Voice yet. Have the Voice Writer write one from your writing, upload this computer's from the Local Voice page, or write it here: how you build sentences and paragraphs, the words you use and avoid, your tone, and a few short examples."}
          </p>
        )}
        {has && <p className="muted small">{pending ? "The editor holds your newest version, the one waiting above." : "The editor holds your live voice."}</p>}
        <VoiceEditor
          key={`account:${pending?.digest ?? live?.digest ?? "none"}`}
          initial={draft}
          note={reviewCostLine(view.reviewCredits, !!live)}
          busy={busy === "save-account"}
          onSave={(text) => act("save-account", async () => {
            await VoiceAPI.SaveAccount(registry, text);
            return "Sent to AI Review. It goes live in your account when it passes.";
          })}
        />
        <div className="voice-cloud-ai">
          {avail && !avail.offered ? (
            <p className="muted small">{NO_HOSTED_WRITER}</p>
          ) : (
            <>
              <Button disabled={!avail?.available || running} onClick={() => setDialog(true)}>
                {has ? "Edit with AI" : "Write with AI"}
              </Button>
              <span className="muted small">
                {avail && !avail.available ? (
                  <>
                    {avail.reason} {avail.remedy}{" "}
                    {avail.upgradeUrl && <Button variant="link" onClick={() => OpenExternal(avail.upgradeUrl!)}>Upgrade</Button>}
                  </>
                ) : availError ? (
                  "Whether the Voice Writer can run In Agentry could not be read. Open this page again to retry."
                ) : running ? (
                  "The Voice Writer is running In Agentry."
                ) : has ? (
                  "Tell the Voice Writer what should change, with samples of your writing if they show it. It runs In Agentry."
                ) : (
                  "The Voice Writer writes your voice from samples of your writing. It runs In Agentry."
                )}
              </span>
            </>
          )}
        </div>
      </Card>

      {has && (
        <Card>
          <h2>Use by default</h2>
          <label className="check-row">
            <input
              type="checkbox"
              checked={!!account?.alwaysAttach}
              disabled={busy === "attach-account"}
              onChange={(e) => {
                const on = e.target.checked;
                act("attach-account", async () => {
                  await VoiceAPI.SetAccountAttach(registry, on);
                  return on ? "Every run In Agentry writes in your voice unless its setup says otherwise." : "Runs In Agentry write in your voice only when their setup says so.";
                });
              }}
            />
            <span>
              {CLOUD_SWITCH_LABEL}
              <span className="muted small"> — the same switch as Settings → Profile. Every run's setup shows Write in my voice, ticked from it, and you can untick it for one run.</span>
            </span>
          </label>
          {!live && <p className="muted small">{NOT_LIVE}</p>}
        </Card>
      )}

      {has && (
        <Card>
          {account?.previous && (
            <div className="voice-foot-item">
              <p className="muted small">
                The previous version, {accountSource(account.previous.source)}, is from {voiceStamp(account.previous.createdAt)}.{" "}
                {restoreLine(account, view.reviewCredits)}
              </p>
              <Button busy={busy === "restore"} busyLabel="Restoring…" onClick={() => act("restore", async () => {
                const r = await VoiceAPI.RestoreAccount(registry);
                return r.inReview ? "The previous version is in review. It goes live when AI Review passes it." : "The previous version is live again.";
              })}>
                Restore previous
              </Button>
            </div>
          )}
          <div className="voice-foot-item">
            <p className="muted small">Delete removes your voice in Agentry, every version and its key. The voice kept on this computer is not touched.</p>
            <Button variant="danger" busy={busy === "delete-account"} onClick={() => act("delete-account", async () => {
              await VoiceAPI.DeleteAccount(registry);
              return "Your account's voice is deleted.";
            })}>
              Delete voice
            </Button>
          </div>
        </Card>
      )}

      {dialog && avail && (
        <CloudEditWithAI
          registry={registry}
          hasVoice={has}
          avail={avail}
          hosted={hosted}
          onHosted={onHosted}
          onClose={() => setDialog(false)}
        />
      )}
    </>
  );
}

// PendingVersion is the version waiting in the account (D5): in review,
// which the page follows, or flagged with what AI Review found; Discard on
// it either way.
function PendingVersion({ version, live, busy, onDiscard }: {
  version: voice.Version;
  live: boolean;
  busy: boolean;
  onDiscard: () => void;
}) {
  const flagged = version.review === "flagged";
  const findings = [...(version.findings ?? [])].sort((a, b) => Number(b.flags) - Number(a.flags));
  return (
    <Card className="voice-pending">
      <div className="row spread">
        <h2>{live ? "Your new version" : "Your first version"}</h2>
        <span className={`badge ${flagged ? "bad" : "warn"}`}>{flagged ? "Flagged" : "In review"}</span>
      </div>
      <p role="status">{pendingLine(version, live)}</p>
      <p className="muted small">{versionLine(version)}</p>
      {flagged && findings.length > 0 && (
        <>
          <h3>What AI Review found</h3>
          <ul className="small">
            {findings.map((f, i) => (
              <li key={i}>{findingText(f)}{!f.flags && <span className="muted"> (noted, does not keep it from going live)</span>}</li>
            ))}
          </ul>
        </>
      )}
      <Button variant="danger" busy={busy} busyLabel="Discarding…" onClick={onDiscard}>Discard this version</Button>
    </Card>
  );
}

// SamplesPicker is the dialogs' writing samples: files and folders from
// the native pickers only (the gate is in Go), each removable.
function SamplesPicker({ label, samples, onChange, hint }: {
  label: string;
  samples: string[];
  onChange: (next: string[]) => void;
  hint?: ReactNode;
}) {
  async function pickFiles() {
    const picked = await ChooseFiles("Samples of your writing");
    if (picked?.length) onChange([...samples, ...picked.filter((p) => !samples.includes(p))]);
  }
  async function pickFolder() {
    const dir = await ChooseDirectory("A folder of your writing");
    if (dir && !samples.includes(dir)) onChange([...samples, dir]);
  }
  return (
    <div className="field">
      <div className="field-label">{label}</div>
      <div className="row gap">
        <Button onClick={pickFiles}>Choose files…</Button>
        <Button onClick={pickFolder}>Choose a folder…</Button>
      </div>
      {hint && <div className="field-hint muted small">{hint}</div>}
      {samples.length > 0 && (
        <ul className="small mono selectable">
          {samples.map((s) => <li key={s}>{s} <Button variant="link" onClick={() => onChange(samples.filter((x) => x !== s))}>Remove</Button></li>)}
        </ul>
      )}
    </div>
  );
}

// CloudEditWithAI is the one dialog of plan §7 (D3) on the Cloud page: the
// Voice Writer In Agentry at the registry's flat price, an AI Edit of the
// account's newest version with a note and optional samples, or a Create
// from samples where the account keeps no voice (Go decides which,
// HostedWithAI). While it runs the dialog shows its status and Cancel run,
// and it can be closed: the page keeps following it, and what it writes
// waits for AI Review.
function CloudEditWithAI({ registry, hasVoice, avail, hosted, onHosted, onClose }: {
  registry: string;
  hasVoice: boolean;
  avail: main.VoiceHostedAvailabilityView;
  hosted: main.VoiceHostedView | null;
  onHosted: (v: main.VoiceHostedView | null) => void;
  onClose: () => void;
}) {
  const [text, setText] = useState("");
  const [samples, setSamples] = useState<string[]>([]);
  const [starting, setStarting] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const running = !!hosted && !hosted.final;

  async function start() {
    setError(null);
    setStarting(true);
    try {
      const r = await VoiceAPI.HostedWithAI(registry, text, samples);
      if (r.refusal) setError(r.detail || r.refusal);
      else if (r.run) onHosted(r.run);
    } catch (e) {
      setError(String(e));
    } finally {
      setStarting(false);
    }
  }

  const title = hasVoice ? "Edit with AI" : "Write with AI";
  if (running) {
    return (
      <Modal title={title} onClose={onClose}>
        <Spinner label="The Voice Writer is running In Agentry…" />
        <p className="small" role="status">{hosted.statusLine}</p>
        <p className="muted small">
          When it ends, what it wrote becomes your account's new version and waits for AI Review; your live voice stays as it is until it
          passes. You can close this: the page keeps following the run.
        </p>
        <ErrorBox error={error} />
        <div className="row gap" style={{ justifyContent: "flex-end" }}>
          <Button
            busy={cancelling || hosted.cancelRequested}
            busyLabel="Cancelling…"
            onClick={() => {
              setCancelling(true);
              VoiceAPI.CancelHosted().then(onHosted).catch((e) => setError(String(e))).finally(() => setCancelling(false));
            }}
          >
            Cancel run
          </Button>
          <Button onClick={onClose}>Close</Button>
        </div>
      </Modal>
    );
  }
  const problem = editWithAIProblem(hasVoice, text, samples.length);
  const mb = Math.round(avail.maxInputBytes / (1024 * 1024));
  return (
    <Modal title={title} onClose={onClose}>
      <p className="muted small">
        {hasVoice
          ? "Tell the Voice Writer what should change in your voice. Add samples of your writing if they show it."
          : "The Voice Writer reads samples of your writing and writes your voice as instructions an LLM can follow."}
      </p>
      <div className="field">
        <label className="field-label" htmlFor="voice-cloud-ai-text">{hasVoice ? "What should change?" : "How do you want to sound? (optional)"}</label>
        <Textarea
          id="voice-cloud-ai-text"
          rows={4}
          value={text}
          maxLength={MAX_VOICE_NOTE_BYTES}
          placeholder={hasVoice ? "Warmer, more like my newer posts, and no exclamation marks." : "Warmer than my emails, and never formal."}
          onChange={(e) => setText(e.target.value)}
        />
      </div>
      <SamplesPicker
        label={hasVoice ? "Add writing samples (optional)" : "Samples of your writing"}
        samples={samples}
        onChange={setSamples}
        hint={`Text, Markdown, PDF or Word documents you wrote. They are uploaded when it starts: at most ${avail.maxInputFiles} files, ${mb} MB together. The Voice Writer reads up to 1 MB of their text.`}
      />
      <p className="small">{hostedCostLine(avail)}</p>
      <p className="muted small">
        {hasVoice ? "It rewrites your newest version, the one in the editor. " : ""}What it writes waits for AI Review like any version you save. The
        samples are deleted with the run.
      </p>
      <ErrorBox error={error} />
      {hosted?.final && !hosted.applied && <p className="muted small">The last run: {hostedFailure(hosted)}</p>}
      <div className="row gap" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" disabled={problem !== null} busy={starting} busyLabel="Starting…" onClick={start}>Start</Button>
      </div>
      {problem && (text.trim() !== "" || samples.length > 0) && <p className="muted small">{problem}</p>}
    </Modal>
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
