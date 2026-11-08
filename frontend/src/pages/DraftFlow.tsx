// A draft by the Drafter, here or In Agentry, followed above the pages in
// its run window (RunWindow.tsx), as a QuickAgent's run is: App mounts this,
// and Draft it or Describe again opens it. On this computer the window shows
// the time used against the draft's limit, its turns, its tokens and the
// stage it is in (AI Review of the description, the Drafter's turn, AI
// Review's check, the repair turn), with the Run log; In Agentry it shows
// where the draft stands, its hold and Cancel. Stop stops only a draft
// (BuildAPI.StopDraft); Cancel asks the registry. Once the draft is over the
// card and the dialog say how it ended, and a draft that landed offers Open
// the draft, which opens it on the Build tab.
//
// The run lock is unchanged: a draft on this computer is the run in flight,
// so a run or a build started meanwhile is refused with the reason. Every
// string here that an agent or the registry wrote (the log, the refusal,
// the error) is drawn as text.
import { useEffect, useState } from "react";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { DraftProgress, cancelHostedDraft, dismissDraft, showDraft, stopDraft, useDraft } from "../drafts";
import type { Tone } from "../jobProgress";
import { ProgressView } from "../progress";
import { RevealText } from "../reveal";
import { Button, ErrorBox, Notice } from "../ui";
import { RunWindow, useFollowed } from "./RunWindow";

const STAGE: Record<Exclude<DraftProgress["stage"], "">, string> = {
  description: "AI Review is reading your description",
  knowledge: "AI Review is scanning your knowledge packs that were not scanned in Agentry",
  drafting: "The Drafter is drafting",
  checking: "AI Review is checking the draft",
  repair: "The Drafter is repairing the draft, in its one repair turn",
};

function stageLine(p: DraftProgress | null): string {
  return (p?.stage && STAGE[p.stage]) || "Starting the draft";
}

export default function DraftFlow({ registry, onOpenDraft, initialMinimized }: {
  registry: string;
  /** Opens the draft that landed on the Build tab. */
  onOpenDraft: () => void;
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const d = useDraft();
  // A draft In Agentry the app found running asks for the card; a new
  // draft, or Open, is a new ask, so it opens as the dialog.
  const followed = useFollowed({ raised: d.raised, asCard: d.asCard, initialMinimized });
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => setErr(null), [d.raised]);
  if (!d.shown) return null;

  const inAgentry = d.where === "agentry";
  const hosted = d.hosted;
  const cancel = () => cancelHostedDraft().catch((e) => setErr(String(e)));
  const canCancel = !!hosted && !hosted.final && !hosted.cancelRequested;
  // Stop reaches a draft once Go has begun it: its first report comes after
  // the native dialog and inside the run lock, and before then there is
  // nothing to stop.
  const canStop = d.local && d.progress !== null;
  const open = () => {
    dismissDraft();
    onOpenDraft();
  };
  const what = `${d.revising ? `Revising ${d.revising}` : "Drafting a QuickAgent"}${inAgentry ? " In Agentry" : ""}`;

  let line = "";
  let tone: Tone = "";
  if (d.local) line = stageLine(d.progress);
  else if (d.inFlight) line = hosted ? hosted.statusLine || hosted.state : "Asking Agentry to start the draft";
  else if (d.drafted) {
    line = d.revising ? `Drafted the next version of ${d.revising}` : `Drafted ${d.drafted}`;
    tone = "ok";
  } else if (d.refusal) {
    line = "Agentry did not start the draft";
    tone = "bad";
  } else if (d.error) {
    line = d.stopped ? "You stopped the draft, so none was made" : "No draft was made";
    tone = d.stopped ? "warn" : "bad";
  }

  let outcome: React.ReactNode = null;
  if (!d.inFlight && d.drafted) {
    outcome = (
      <>
        <h3 className="run-outcome run-outcome-ok">{d.revising ? "Drafted the next version of " : "Drafted "}<RevealText text={d.revising ?? d.drafted} /></h3>
        <p>
          {d.revising
            ? "It opens on the Build tab with what changed. Publish it as drafted, or open it in the Builder to change it first."
            : "The draft opens on the Build tab. It is a starting point: check every part, and what it may reach, before you save or publish it."}
        </p>
        {hosted?.final && <p className="small">{hosted.creditsLine}</p>}
      </>
    );
  } else if (!d.inFlight && d.refusal) {
    outcome = (
      <>
        <h3 className="run-outcome run-outcome-bad">Agentry did not start the draft</h3>
        <DraftRefusal v={d.refusal} registry={registry} onError={setErr} />
        <ErrorBox error={err} />
      </>
    );
  } else if (!d.inFlight) {
    // A draft that ended with no draft: why, in the words Go or the
    // registry gave, then what it used; its log follows, open.
    outcome = (
      <>
        <h3 className={`run-outcome run-outcome-${d.stopped ? "warn" : "bad"}`}>No draft was made</h3>
        {d.error && <div className="error-box selectable"><RevealText text={d.error} /></div>}
        {hosted?.final && <p className="small">{hosted.creditsLine}</p>}
      </>
    );
  }

  return (
    <RunWindow
      {...followed}
      onOpen={showDraft}
      onDismiss={dismissDraft}
      label={d.revising ? "Edit with AI: the revision" : "Describe it: the draft"}
      title={d.inFlight ? what : d.revising ? "Edit with AI" : "Describe it"}
      cardTitle={d.inFlight ? what : d.revising ? "Edit with AI: the revision is over" : "Describe it: the draft is over"}
      running={d.inFlight}
      spinner={d.local ? `${stageLine(d.progress)}…` : hosted ? hosted.statusLine || hosted.state : "Asking Agentry to start the draft…"}
      about={d.local ? <>
        AI Review reads your description first, then every search result and page before the Drafter&apos;s model does,
        and checks the finished draft. A draft takes a few minutes.
      </> : <>
        The Drafter runs In Agentry, and AI Review reads every result through the registry.
      </>}
      status={!d.local && <>
        {hosted && <p className="small">{hosted.creditsLine}</p>}
        {hosted?.cancelRequested && <p className="muted small">Cancel asked for. The draft stops, and what it used is settled.</p>}
      </>}
      progress={d.local ? (compact) => <DraftProgressView progress={d.progress} compact={compact} /> : undefined}
      background={d.local ? <>
        In the background, the draft goes on in a card in the corner and the rest of the app stays usable. Stop ends it,
        and no draft is made.
      </> : <>
        In the background, the draft goes on In Agentry and this app follows it in a card in the corner. Cancel asks
        Agentry to stop it and settle what it used.
      </>}
      stop={canStop
        ? { label: "Stop the draft", cardLabel: "Stop", stopping: d.stopping, onStop: () => void stopDraft() }
        : canCancel ? { label: "Cancel the draft", stopping: false, onStop: () => void cancel() } : null}
      log={inAgentry ? null : d.log}
      line={line}
      tone={tone}
      error={d.inFlight ? err : null}
      outcome={outcome}
      primary={!d.inFlight && d.drafted ? { label: "Open the draft", onClick: open } : null}
      closeOver="dismiss"
    />
  );
}

// DraftProgressView is how far a draft on this computer has got: the shared
// view, whose compact form names the turns after the clock.
function DraftProgressView({ progress, compact }: { progress: DraftProgress | null; compact?: boolean }) {
  return <ProgressView progress={progress} job="draft" compact={compact} fact={(p) => (p.turns === 1 ? "1 turn" : `${p.turns} turns`)} />;
}

function DraftRefusal({ v, registry, onError }: { v: app.HostedDraftStartView; registry: string; onError: (e: string) => void }) {
  return (
    <Notice tone="warn">
      The draft was not started: <RevealText text={v.detail ?? v.refusal ?? ""} />
      {v.refusal === "revise-tier" && (
        <>
          {" "}Edit with AI In Agentry is part of Pro and above; on this computer it runs with your own keys.{" "}
          <Button variant="link" onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)))}>See Pro</Button>
        </>
      )}
      {v.refusal === "credits-exhausted" && (
        <>
          {" "}
          <Button variant="link" onClick={() => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)))}>Credits and upgrade</Button>
        </>
      )}
    </Notice>
  );
}
