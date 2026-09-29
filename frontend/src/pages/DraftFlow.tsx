// A draft's progress (agentry-desktop#410), as Create knowledge shows a
// build's (#352, #353): App mounts this above the pages, and Draft it or
// Describe again opens it as a dialog. On this computer the dialog shows the
// time used against the draft's limit, its turns, its tokens and the stage
// it is in (AI Review of the description, the Drafter's turn, AI Review's
// check, the repair turn), over the Drafter's log folded beneath; In Agentry
// it shows where the draft stands, its hold and Cancel. Run in the
// background, or closing the dialog while the draft runs, shrinks it to a
// card in the bottom-right corner that keeps the progress, stops the draft
// (on this computer: BuildAPI.StopDraft, which stops only a draft; In
// Agentry: the registry's cancel) and opens the dialog again. Once the draft
// is over the card and the dialog say how it ended, and a draft that landed
// offers Open the draft, which opens it on the Build tab.
//
// The run lock is unchanged: a draft on this computer is the run in flight,
// so a run or a build started meanwhile is refused with the reason. Every
// string here that an agent or the registry wrote (the log, the refusal,
// the error) is drawn as text.
import { useEffect, useRef, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { main } from "../../wailsjs/go/models";
import { DraftProgress, DraftState, STOPPED, cancelHostedDraft, dismissDraft, showDraft, stopDraft, useDraft } from "../drafts";
import { fmtClock } from "../knowledge";
import { CornerCard, TimeBar, useNow } from "../progress";
import { RevealText } from "../reveal";
import { Button, ErrorBox, Modal, Notice, Spinner } from "../ui";

const STAGE: Record<string, string> = {
  description: "AI Review is reading your description",
  drafting: "The Drafter is drafting",
  checking: "AI Review is checking the draft",
  repair: "The Drafter is repairing the draft, in its one repair turn",
};

function stageLine(p: DraftProgress | null): string {
  return (p && STAGE[p.stage]) || "Starting the draft";
}

export default function DraftFlow({ registry, onOpenDraft, initialMinimized }: {
  registry: string;
  /** Opens the draft that landed on the Build tab. */
  onOpenDraft: () => void;
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const d = useDraft();
  const [minimized, setMinimized] = useState(initialMinimized ?? false);
  const [err, setErr] = useState<string | null>(null);
  // A new draft, or Show its progress, opens the dialog.
  const raised = useRef(d.raised);
  useEffect(() => {
    if (d.raised !== raised.current) {
      raised.current = d.raised;
      setMinimized(false);
      setErr(null);
    }
  }, [d.raised]);
  if (!d.shown) return null;

  const inAgentry = d.where === "agentry";
  const hosted = d.hosted;
  const cancel = () => cancelHostedDraft().catch((e) => setErr(String(e)));
  const canCancel = !!hosted && !hosted.final && !hosted.cancelRequested;
  const open = () => {
    dismissDraft();
    onOpenDraft();
  };

  if (minimized) {
    let line = "";
    let tone: "" | "ok" | "warn" | "bad" = "";
    if (d.local) line = stageLine(d.progress);
    else if (d.inFlight) line = hosted ? hosted.statusLine || hosted.state : "Asking Agentry to start the draft";
    else if (d.drafted) {
      line = `Drafted ${d.drafted}`;
      tone = "ok";
    } else if (d.refusal) {
      line = "Agentry did not start the draft";
      tone = "bad";
    } else if (d.error) {
      line = "No draft was made";
      tone = "bad";
    }
    return (
      <CornerCard
        label="Describe it: the draft"
        running={d.inFlight}
        title={d.inFlight ? `Drafting a QuickAgent${inAgentry ? " In Agentry" : ""}` : "Describe it: the draft is over"}
        line={line}
        tone={tone}
        actions={<>
          {d.local && <Button onClick={() => void stopDraft()} disabled={d.stopping}>{d.stopping ? "Stopping…" : "Stop"}</Button>}
          {canCancel && <Button onClick={cancel}>Cancel the draft</Button>}
          {!d.inFlight && <Button onClick={dismissDraft}>Dismiss</Button>}
          {!d.inFlight && d.drafted
            ? <Button variant="primary" onClick={open}>Open the draft</Button>
            : <Button variant="primary" onClick={() => { setMinimized(false); showDraft(); }}>Open</Button>}
        </>}
      >
        {d.local && <DraftProgressView progress={d.progress} compact />}
      </CornerCard>
    );
  }

  const title = d.inFlight ? `Drafting a QuickAgent${inAgentry ? " In Agentry" : ""}` : "Describe it";
  return (
    <Modal title={title} onClose={d.inFlight ? () => setMinimized(true) : dismissDraft} wide>
      {d.local && (
        <>
          <Spinner label={`${stageLine(d.progress)}…`} />
          <p className="muted small">
            AI Review reads your description first, then every search result and page before the Drafter&apos;s model does,
            and checks the finished draft. A draft takes a few minutes.
          </p>
          <DraftProgressView progress={d.progress} />
          <details className="knowledge-log">
            <summary>Draft log</summary>
            <DraftLog lines={d.log} />
          </details>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => void stopDraft()} disabled={d.stopping}>{d.stopping ? "Stopping…" : "Stop the draft"}</Button>
            <Button variant="primary" onClick={() => setMinimized(true)}>Run in the background</Button>
          </div>
          <p className="muted small">
            In the background, the draft goes on in a card in the corner and the rest of the app stays usable. Stop ends it,
            and no draft is made.
          </p>
        </>
      )}

      {!d.local && d.inFlight && (
        <>
          <Spinner label={hosted ? hosted.statusLine || hosted.state : "Asking Agentry to start the draft…"} />
          <p className="muted small">
            The Drafter runs In Agentry, and AI Review reads every result through the registry. Run in the background keeps it
            in a card in the corner while you use the rest of the app; the draft goes on in Agentry either way.
          </p>
          {hosted && <p className="small">{hosted.creditsLine}</p>}
          {hosted?.cancelRequested && <p className="muted small">Cancel asked for. The draft stops, and what it used is settled.</p>}
          <ErrorBox error={err} />
          <div className="row" style={{ justifyContent: "flex-end" }}>
            {canCancel && <Button onClick={cancel}>Cancel the draft</Button>}
            <Button variant="primary" onClick={() => setMinimized(true)}>Run in the background</Button>
          </div>
        </>
      )}

      {!d.inFlight && d.drafted && (
        <>
          <h3 className="run-outcome run-outcome-ok">Drafted {d.drafted}</h3>
          <p>
            The draft opens on the Build tab. It is a starting point: check every part, and what it may reach, before you save
            or publish it.
          </p>
          {hosted?.final && <p className="small">{hosted.creditsLine}</p>}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={dismissDraft}>Close</Button>
            <Button variant="primary" onClick={open}>Open the draft</Button>
          </div>
        </>
      )}

      {!d.inFlight && !d.drafted && d.refusal && (
        <>
          <h3 className="run-outcome run-outcome-bad">Agentry did not start the draft</h3>
          <DraftRefusal v={d.refusal} registry={registry} onError={setErr} />
          <ErrorBox error={err} />
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button variant="primary" onClick={dismissDraft}>Close</Button>
          </div>
        </>
      )}

      {!d.inFlight && !d.drafted && !d.refusal && (
        <DraftFailed d={d} />
      )}
    </Modal>
  );
}

// DraftFailed is a draft that ended with no draft: why, in the words Go or
// the registry gave, then what it used and its log.
function DraftFailed({ d }: { d: DraftState }) {
  const stopped = d.error === STOPPED;
  return (
    <>
      <h3 className={`run-outcome run-outcome-${stopped ? "warn" : "bad"}`}>No draft was made</h3>
      {d.error && <div className="error-box selectable"><RevealText text={d.error} /></div>}
      {d.hosted?.final && <p className="small">{d.hosted.creditsLine}</p>}
      {d.log.length > 0 && <DraftLog lines={d.log} />}
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button variant="primary" onClick={dismissDraft}>Close</Button>
      </div>
    </>
  );
}

function DraftLog({ lines }: { lines: string[] }) {
  const ref = useRef<HTMLPreElement | null>(null);
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [lines.length]);
  // The log carries what pages and the registry said, so an invisible
  // character in it is shown, as in the error above it.
  return <pre ref={ref} className="run-log selectable" aria-label="Draft log"><RevealText text={lines.join("\n")} /></pre>;
}

// DraftProgressView is how far a draft on this computer has got: the time it
// has run against its limit, as a bar and a clock, the turns against the
// turn budget and the tokens it has used. Before the first report it says
// the draft is starting.
function DraftProgressView({ progress, compact }: { progress: DraftProgress | null; compact?: boolean }) {
  const now = useNow();
  if (!progress) return <p className="muted small">Starting the draft…</p>;
  const elapsed = Math.max(0, (now - progress.startedAt) / 1000);
  const bar = <TimeBar elapsed={elapsed} timeoutSeconds={progress.timeoutSeconds} label="Time used of the draft's limit" />;
  const time = <>{fmtClock(elapsed)}{progress.timeoutSeconds > 0 && <> of up to {fmtClock(progress.timeoutSeconds)}</>}</>;
  if (compact) {
    return (
      <div className="knowledge-progress">
        {bar}
        <p className="small muted">{time} · {progress.turns === 1 ? "1 turn" : `${progress.turns} turns`}</p>
      </div>
    );
  }
  return (
    <div className="knowledge-progress">
      {bar}
      <dl className="knowledge-facts">
        <dt>Time</dt>
        <dd>{time}</dd>
        <dt>Turns</dt>
        <dd>
          {progress.turns}
          {progress.maxTurns > 0 && <> of up to {progress.maxTurns}</>}
        </dd>
        {(progress.inputTokens > 0 || progress.outputTokens > 0) && (
          <>
            <dt>Tokens</dt>
            <dd>
              {progress.inputTokens.toLocaleString()} input, {progress.outputTokens.toLocaleString()} output
            </dd>
          </>
        )}
      </dl>
    </div>
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
