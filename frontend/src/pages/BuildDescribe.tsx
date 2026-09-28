// Describe it (plan builder-rework.md §7.1, D5, D11, D22, D25): the Build
// tab's third way in. A description and three optional hints, the site's,
// then a draft made in one of two places:
//   - In Agentry (#281): the built-in Drafter on a sandbox host against the
//     platform's model, for credits, with AI Review reading every text
//     through the registry. It needs an account; the page states the hold,
//     the balance and the honest line first, and follows the draft the way
//     a run In Agentry is followed. BuildAPI.DescribeInAgentry starts it.
//   - On this computer (#282): the Drafter against the runner policy's
//     model with the person's own AI Review key, for no credits.
//     BuildAPI.DescribeOnThisComputer refuses without a key and asks in a
//     native dialog before anything leaves this computer.
// Either way the draft is decoded in Go into the form's Model as data and
// opens on Review; nothing is written into a BriefAgent folder until the
// person saves.
//
// DraftNotes is what the Drafter said, shown beside Review once a draft is
// in the form, with Describe again under it. All of it is agent-written:
// the notes render through the run-output reader (AgentMarkdown: no raw
// HTML, no image, links only to the system browser), a withheld address is
// plain text and never a link, and every other field is text.
import { useEffect, useRef, useState } from "react";
import * as BuildAPI from "../../wailsjs/go/main/BuildAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { build, main } from "../../wailsjs/go/models";
import { AgentMarkdown, ExternalLink, webHref } from "../AgentMarkdown";
import { RevealText } from "../reveal";
import { Button, Card, ErrorBox, Notice, Select, Textarea } from "../ui";

type Where = "agentry" | "here";

// The hints as the site asks them (agentry-web app/build/describe), and the
// lines the Drafter reads for them (app/build/draft hintsOf): each labelled
// so it knows which hint it is. A blank one is left out.
type Hints = { services: string; mode: "" | "autonomous" | "interactive"; example: string };
const NO_HINTS: Hints = { services: "", mode: "", example: "" };
function hintLines(h: Hints): string[] {
  const out: string[] = [];
  const one = (s: string) => s.replace(/\s+/g, " ").trim();
  if (one(h.services)) out.push(`The services or sites it works with: ${one(h.services)}`);
  if (h.mode === "autonomous") out.push("It runs on its own, from a task, with no conversation.");
  else if (h.mode === "interactive") out.push("It runs in a conversation with a person.");
  if (one(h.example)) out.push(`An example of the output wanted: ${one(h.example)}`);
  return out;
}

// The description the page keeps while the tab is away, as Build keeps its
// form (the tab unmounts on every change).
let kept: { text: string; hints: Hints; where: Where | null } = { text: "", hints: NO_HINTS, where: null };

// A draft outlives the page. It takes minutes, and Build unmounts on every
// tab change, so the draft in flight, what it has said and the draft or
// refusal it ends with are kept here, and whichever page is showing when it
// lands takes it (takeLandedDraft). One can be in flight at a time.
let localInflight: Promise<void> | null = null;
let inflightLog: string[] = [];
// hosted is the draft In Agentry this app follows, as its last event said;
// hostedStarting covers the create's round trip.
let hosted: main.HostedDraftView | null = null;
let hostedStarting = false;
let hostedListening = false;
let landed: main.DraftView | null = null;
let landedError: string | null = null;
let refusal: main.HostedDraftStartView | null = null;
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((f) => f());

/** Whether a draft is being made now, here or In Agentry. */
export function draftInFlight(): boolean {
  return localInflight !== null || hostedStarting || (hosted !== null && !hosted.final);
}

/** The draft that finished and no page has taken yet, taken. */
export function takeLandedDraft(): main.DraftView | null {
  const v = landed;
  landed = null;
  return v;
}

function land(v: main.DraftView | undefined | null, orError: string) {
  if (v?.model) {
    landed = v;
    kept = { text: "", hints: NO_HINTS, where: kept.where };
  } else {
    landedError = orError;
  }
}

function startLocal(registry: string, text: string, hints: string[]) {
  inflightLog = [];
  landedError = null;
  refusal = null;
  const off = EventsOn("build:draft-log", (line: string) => {
    inflightLog = [...inflightLog, line];
    notify();
  });
  localInflight = BuildAPI.DescribeOnThisComputer(registry, text, hints)
    .then((v) => land(v, "The Drafter returned no draft the form can open."))
    .catch((e) => { landedError = String(e); })
    .finally(() => {
      off();
      localInflight = null;
      notify();
    });
  notify();
}

// listenHosted follows build:hosted-draft for the app's life: a draft In
// Agentry reports every state change, and its final word carries the draft.
function listenHosted() {
  if (hostedListening) return;
  hostedListening = true;
  EventsOn("build:hosted-draft", (v: main.HostedDraftView) => {
    hosted = v;
    if (v.final) {
      const why = [v.outcomeLine, v.error, v.draftError].filter(Boolean).join(" ");
      land(v.draft, why || "The draft ended with no draft the form can open.");
    }
    notify();
  });
}

function startHosted(registry: string, text: string, hints: string[]) {
  listenHosted();
  landedError = null;
  refusal = null;
  hosted = null;
  hostedStarting = true;
  notify();
  BuildAPI.DescribeInAgentry(registry, text, hints)
    .then((v) => {
      if (v.draft) {
        // An event may already have said more than the create's answer.
        if (!hosted || hosted.id !== v.draft.id) hosted = v.draft;
      } else {
        refusal = v;
      }
    })
    .catch((e) => { landedError = String(e); })
    .finally(() => {
      hostedStarting = false;
      notify();
    });
}

/** Starts a draft from the page, in the place chosen. */
export function startDraft(registry: string, where: Where, text: string, hints: string[]) {
  if (where === "agentry") startHosted(registry, text, hints);
  else startLocal(registry, text, hints);
}

/** Follows this account's draft In Agentry after the app was reopened. */
function attachHosted(registry: string) {
  if (hosted || hostedStarting) return;
  listenHosted();
  BuildAPI.AttachHostedDraft(registry)
    .then((v) => {
      if (v.draft && !hosted) {
        hosted = v.draft;
        notify();
      }
    })
    .catch(() => {});
}

function minutes(s: number): string {
  const m = Math.round(s / 60);
  return m === 1 ? "1 minute" : `${m} minutes`;
}

export function DescribePanel({ registry, onDrafted, onOpenSettings }: {
  registry: string;
  onDrafted: (v: main.DraftView) => void;
  onOpenSettings?: () => void;
}) {
  const [text, setText] = useState(kept.text);
  const [hints, setHints] = useState<Hints>(kept.hints);
  const [where, setWhere] = useState<Where | null>(kept.where);
  const [status, setStatus] = useState<main.DescribeStatusView | null>(null);
  const [agentry, setAgentry] = useState<main.DescribeHostedView | null>(null);
  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [, redraw] = useState(0);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    BuildAPI.DescribeStatus().then(setStatus).catch((e) => setError(String(e)));
    BuildAPI.DescribeInAgentryStatus(registry).then(setAgentry).catch(() => {});
    RunAPI.SetupStatus().then(setSetup).catch(() => {});
    attachHosted(registry);
  }, [registry]);
  useEffect(() => {
    const f = () => redraw((n) => n + 1);
    listeners.add(f);
    return () => { listeners.delete(f); };
  }, []);
  // In Agentry is chosen first where it can be, as on the site.
  useEffect(() => {
    if (where === null && agentry) setWhere(agentry.available ? "agentry" : "here");
  }, [agentry, where]);
  useEffect(() => { kept = { text, hints, where }; }, [text, hints, where]);
  // A draft that landed, while this page showed or before it came back.
  useEffect(() => {
    const v = takeLandedDraft();
    if (v) onDrafted(v);
  });
  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }); }, [inflightLog.length]);

  const busy = draftInFlight();
  const shownError = error ?? (busy ? null : landedError);
  const key = status?.key;
  const dockerOk = setup?.dockerOk ?? false;
  const readyHere = !!key?.ready && dockerOk;
  const readyAgentry = !!agentry?.available;
  const ready = text.trim() !== "" && (where === "agentry" ? readyAgentry : where === "here" && readyHere);
  const following = hosted && !hosted.final ? hosted : null;

  function draft() {
    if (!where) return;
    setError(null);
    startDraft(registry, where, text, hintLines(hints));
  }

  return (
    <section aria-labelledby="describe-heading">
      <h2 id="describe-heading">Describe it</h2>
      <p className="muted small">
        Say what the BriefAgent should do, in plain words. The Drafter searches the registry and the web, reads what it
        finds, and drafts the BriefAgent: what it may reach, its instructions, the questions a run asks and what a person
        reads first. The draft opens on Review. Nothing is written into a BriefAgent folder or submitted until you save or
        publish it.
      </p>
      <Card>
        <label className="small" style={{ display: "block" }}>Describe what the BriefAgent should do
          <Textarea rows={6} style={{ width: "100%" }} value={text} disabled={busy}
            onChange={(e) => setText(e.target.value)}
            placeholder="e.g. Check a CSV file I add for missing values, duplicate rows and columns whose values are not all the same format, and report the problems per column." />
        </label>
        <p className="small" style={{ marginBottom: 4 }}>Hints, if you have them</p>
        <label className="small" style={{ display: "block", marginTop: 6 }}>The services or sites it works with
          <input type="text" style={{ width: "100%" }} value={hints.services} placeholder="e.g. GitHub" disabled={busy} maxLength={430}
            onChange={(e) => setHints({ ...hints, services: e.target.value })} />
        </label>
        <label className="small" style={{ display: "block", marginTop: 6 }}>How it runs
          <Select style={{ width: "100%" }} value={hints.mode} disabled={busy}
            onChange={(e) => setHints({ ...hints, mode: e.target.value as Hints["mode"] })}>
            <option value="">Either, or not sure</option>
            <option value="autonomous">On its own, from a task</option>
            <option value="interactive">In a conversation with a person</option>
          </Select>
        </label>
        <label className="small" style={{ display: "block", marginTop: 6 }}>An example of the output you want
          <input type="text" style={{ width: "100%" }} value={hints.example} placeholder="e.g. a table of problems per column" disabled={busy} maxLength={430}
            onChange={(e) => setHints({ ...hints, example: e.target.value })} />
        </label>
      </Card>

      <h3 className="small">Where it drafts</h3>
      {agentry?.offered && (
        <Card className={where === "agentry" ? "selected" : ""}>
          <label className="small" style={{ display: "block" }}>
            <input type="radio" name="describe-where" checked={where === "agentry"} disabled={busy}
              onChange={() => setWhere("agentry")} />{" "}
            <strong>In Agentry</strong>{" "}
            <span className="muted">— on Agentry&apos;s infrastructure against the platform&apos;s model, for credits. AI Review reads the description and every result through the registry.</span>
          </label>
          {agentry.available || agentry.holdSentence ? (
            <div className="small">
              {agentry.holdSentence && (
                <p style={{ marginBottom: 4 }}>
                  {agentry.holdSentence}
                  {agentry.balanceKnown && !agentry.unmetered && agentry.holdCredits > 0
                    ? agentry.pool ? ` ${agentry.pool}'s pool has ${agentry.balance} credits.` : ` This account has ${agentry.balance} credits.`
                    : ""}
                </p>
              )}
              {agentry.timeoutSeconds > 0 && (
                <p className="muted" style={{ marginTop: 0 }}>
                  A draft may run for up to {minutes(agentry.timeoutSeconds)} and {agentry.maxTurns} turns, and AI Review reads up to {agentry.maxPages} pages for it.
                </p>
              )}
              {agentry.short && (
                <Notice tone="warn">
                  {agentry.pool ? `${agentry.pool}'s pool has` : "This account has"} fewer credits than a draft holds, so the registry will refuse this one.{" "}
                  <Button variant="link" onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => setError(String(e)))}>Credits and upgrade</Button>
                </Notice>
              )}
            </div>
          ) : null}
          {!agentry.available && agentry.reason && (
            <Notice tone="warn">{agentry.reason}{agentry.remedy ? ` ${agentry.remedy}` : ""}</Notice>
          )}
        </Card>
      )}
      <Card className={where === "here" ? "selected mt" : "mt"}>
        <label className="small" style={{ display: "block" }}>
          <input type="radio" name="describe-where" checked={where === "here"} disabled={busy}
            onChange={() => setWhere("here")} />{" "}
          <strong>On this computer</strong>{" "}
          <span className="muted">
            — against the model your runner policy names ({status?.modelName ?? "…"}), for no credits. Your own AI Review
            key reads the description and every result first.
          </span>
        </label>
        {key && (key.ready
          ? <p className="small" style={{ marginBottom: 0 }}>AI Review uses your key {key.envSet ? <>from <span className="mono">${key.envName}</span></> : "from this computer's keychain"}.</p>
          : (
            <Notice tone="warn">
              Drafting on this computer needs your own TypeSafe Jev key, because AI Review reads your description and
              everything the draft reads with it before the model does.{" "}
              {onOpenSettings && <Button variant="link" onClick={onOpenSettings}>Add one in Settings</Button>}
            </Notice>
          ))}
        {status && <p className="muted small" style={{ marginBottom: 0 }}>{status.searchSentence}</p>}
        {setup && !dockerOk && (
          <Notice tone="warn">The Drafter runs in containers, and docker is not ready on this computer{setup.dockerErr ? `: ${setup.dockerErr}` : ""}. Settings → Agent runs sets it up.</Notice>
        )}
      </Card>

      {where === "agentry" ? (
        <p className="small">{agentry?.inputsSentence} The draft&apos;s output and its receipts are kept in your Downloads folder, under <span className="mono">agentry-out</span>.</p>
      ) : (
        <p className="small">
          Your description, the Drafter&apos;s searches and the pages it reads go to TypeSafe (AI Review), to your model&apos;s
          provider, to your search provider when you have one, and to the sites it reads, so the description should hold no
          secret. AI Review reads every result before the Drafter&apos;s model does, and holds back what it flags. The
          run&apos;s receipts and the draft are kept in your Downloads folder, under <span className="mono">agentry-out</span>.
        </p>
      )}
      <div className="row">
        <Button variant="primary" disabled={!ready || busy} onClick={draft}>{busy ? "Drafting…" : "Draft it"}</Button>
        {busy && <span className="muted small">A draft takes a few minutes. You can leave this page; the draft opens here when it is done.</span>}
      </div>
      {(hostedStarting || following) && <HostedProgress v={following} />}
      {localInflight && inflightLog.length > 0 && <pre ref={logRef} className="run-log" style={{ maxHeight: 200, overflow: "auto", marginTop: 12 }}>{inflightLog.join("\n")}</pre>}
      {!busy && refusal && <DraftRefusal v={refusal} registry={registry} onError={setError} />}
      {!busy && hosted?.final && !hosted.draft && <p className="small muted">{hosted.creditsLine}</p>}
      <ErrorBox error={shownError} />
    </section>
  );
}

// HostedProgress is a draft In Agentry while it moves: where it stands, its
// hold, and Cancel. A pushed draft keeps no log, so there are no lines.
function HostedProgress({ v }: { v: main.HostedDraftView | null }) {
  const [err, setErr] = useState<string | null>(null);
  return (
    <Card className="mt">
      <p className="small" style={{ marginTop: 0 }}>
        <strong>{v ? v.state : "starting"}</strong>{" "}
        {v ? v.statusLine : "Asking Agentry to start the draft."}
      </p>
      {v && <p className="muted small">{v.creditsLine}</p>}
      {v && !v.cancelRequested && (
        <Button onClick={() => BuildAPI.CancelHostedDraft().then((c) => { hosted = c; notify(); }).catch((e) => setErr(String(e)))}>Cancel this draft</Button>
      )}
      {v?.cancelRequested && <p className="muted small">Cancel asked for. The draft stops, and what it used is settled.</p>}
      <ErrorBox error={err} />
    </Card>
  );
}

function DraftRefusal({ v, registry, onError }: { v: main.HostedDraftStartView; registry: string; onError: (e: string) => void }) {
  return (
    <Notice tone="warn">
      The draft was not started: <RevealText text={v.detail ?? v.refusal ?? ""} />
      {v.refusal === "credits-exhausted" && (
        <>
          {" "}
          <Button variant="link" onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)))}>Credits and upgrade</Button>
        </>
      )}
    </Notice>
  );
}

/** A registry match's reference split into its page's namespace and name. */
function refParts(ref: string): { ns: string; name: string } | null {
  const m = /^([a-z0-9][a-z0-9-]*)\/([a-z0-9][a-z0-9-]*)(@[^\s]+)?$/.exec(ref.trim());
  return m ? { ns: m[1], name: m[2] } : null;
}

export function DraftNotes({ v, onOpenAgent }: { v: main.DraftView; onOpenAgent?: (ns: string, name: string) => void }) {
  const ai = v.aiReview;
  const heldBack = ai.withheld + ai.notScanned;
  const how = v.where === "agentry" ? "through the registry" : "with your key";
  return (
    <Card className="mt">
      <strong>What the Drafter found</strong>
      <p className="small">
        {v.where === "agentry" ? "The draft was made In Agentry. " : ""}
        AI Review read {ai.read} {ai.read === 1 ? "result" : "results"} {how} and held back {heldBack}
        {ai.notScanned > 0 ? ` (${ai.notScanned} could not be scanned)` : ""}. The Drafter was given the address and the
        reason for each one held back, never its text.
      </p>
      {(ai.withheldSources ?? []).length > 0 && (
        <ul className="small">
          {ai.withheldSources.map((w, i) => (
            <li key={i}><span className="mono selectable"><RevealText text={w.url} /></span> ({w.tool}): {w.reason}</li>
          ))}
        </ul>
      )}
      {v.manifestError && <Notice tone="warn">The draft is not finished: <RevealText text={v.manifestError} /></Notice>}
      {(v.decodeNotes ?? []).map((n, i) => <Notice key={i} tone="info"><RevealText text={n} /></Notice>)}
      {(v.matches ?? []).length > 0 && (
        <>
          <h4 className="small">Already in the registry</h4>
          {v.matches.map((m, i) => {
            const at = refParts(m.ref);
            return (
              <div key={i} className="small" style={{ marginTop: 6 }}>
                <span className="mono selectable"><RevealText text={m.ref} /></span>: <RevealText text={m.why} />{" "}
                {at && onOpenAgent && <Button variant="link" onClick={() => onOpenAgent(at.ns, at.name)}>Open its page to install it instead</Button>}
              </div>
            );
          })}
        </>
      )}
      {(v.openQuestions ?? []).length > 0 && (
        <>
          <h4 className="small">What the description leaves open</h4>
          <ul className="small">{v.openQuestions.map((q, i) => <li key={i}><RevealText text={q} /></li>)}</ul>
        </>
      )}
      {v.notes.trim() && (
        <>
          <h4 className="small">The Drafter&apos;s notes</h4>
          <AgentMarkdown text={v.notes} />
        </>
      )}
      {(v.sources ?? []).length > 0 && (
        <details className="small">
          <summary>The sources it read ({v.sources.length})</summary>
          <ul>
            {v.sources.map((s, i) => {
              const href = s.withheld ? null : webHref(s.url);
              return (
                <li key={i}>
                  {href ? <ExternalLink href={href}>{href}</ExternalLink> : <span className="mono selectable"><RevealText text={s.url} /></span>}
                  {s.withheld ? " (held back by AI Review)" : ""}{s.used ? <>: <RevealText text={s.used} /></> : null}
                </li>
              );
            })}
          </ul>
        </details>
      )}
      <p className="muted small selectable" style={{ marginBottom: 0 }}>
        The {v.where === "agentry" ? "draft's output" : "run's receipts"} and <span className="mono">draft.json</span> are in <span className="mono">{v.outDir}</span>.
      </p>
    </Card>
  );
}

// DescribeAgain (plan D11) sits on Review under the notes: a note on what
// should change, and a new draft in the same place as this one, from what
// the person first asked and the form's draft as it stands, written by the
// form's own renderer (BuildAPI.DescribeAgainText). The new draft replaces
// this one when it lands, which is what Describe again asks for.
export function DescribeAgain({ registry, v, model, onStarted }: {
  registry: string;
  v: main.DraftView;
  model: build.Model;
  onStarted: () => void;
}) {
  const [note, setNote] = useState("");
  const [agentry, setAgentry] = useState<main.DescribeHostedView | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const where: Where = v.where === "agentry" ? "agentry" : "here";
  useEffect(() => {
    if (where === "agentry") BuildAPI.DescribeInAgentryStatus(registry).then(setAgentry).catch(() => {});
  }, [registry, where]);
  const blocked = draftInFlight() || (where === "agentry" && agentry !== null && !agentry.available);

  function again() {
    setErr(null);
    BuildAPI.DescribeAgainText(model, note, v.description)
      .then((text) => {
        startDraft(registry, where, text, v.hints ?? []);
        onStarted();
      })
      .catch((e) => setErr(String(e)));
  }

  return (
    <Card className="mt">
      <strong>Describe again</strong>
      <p className="small">
        Say what should change, and the Drafter makes a new draft {where === "agentry" ? "In Agentry" : "on this computer"} from
        what you first asked and this draft as it stands in the form. The new draft replaces this one.
      </p>
      <Textarea rows={3} style={{ width: "100%" }} value={note} maxLength={4000} onChange={(e) => setNote(e.target.value)}
        placeholder="e.g. Post the summary to Slack as well, and ask which channel." />
      {where === "agentry" && agentry?.holdSentence && <p className="small">{agentry.holdSentence}</p>}
      {where === "agentry" && agentry && !agentry.available && agentry.reason && <Notice tone="warn">{agentry.reason}{agentry.remedy ? ` ${agentry.remedy}` : ""}</Notice>}
      <p className="small muted">{where === "agentry" ? agentry?.inputsSentence : "The note and the draft go where a draft on this computer sends its description."}</p>
      <Button disabled={note.trim() === "" || blocked} onClick={again}>Describe again</Button>
      <ErrorBox error={err} />
    </Card>
  );
}
