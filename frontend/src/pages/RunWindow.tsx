// The run window: how the app shows every job an agent runs for the person,
// a QuickAgent's run on this computer or In Agentry and each built-in's job
// alike (the Drafter's draft and revision, the Examiner's review, the
// Knowledge Creator's build, the Voice Writer's and the Report Designer's
// runs). Each flow describes its job (its words, its progress, its log, how
// it stops, how it ended and what it offers then) and draws it through
// RunWindow, so a change here reaches every job, and a new built-in is
// followed the same way by describing itself, never by copying a flow.
//
// While the job runs, the window is a dialog: what it is doing, what it is,
// how far it has got against its limit, the Run log folded beneath, its Stop
// (or Cancel run, for a job In Agentry) and Run in the background. Run in the
// background, or closing the dialog, shrinks it to a card in the corner that
// keeps the progress and the Stop, and Open brings the dialog back. Once the
// job is over, the dialog is its outcome with the Run log beneath (open when
// the job did not succeed), and the card says how it ended; Dismiss makes the
// app forget it.
//
// The Run log is the same for every job: one label, the newest lines with a
// note of those let go (runLog.ts), and its invisible characters shown, since
// a log carries what pages, files and a registry said. Everything a job wrote
// is drawn as text.
import { useEffect, useRef, useState } from "react";
import type { Tone } from "../jobProgress";
import { CornerCard, FoldedLog, StopButton } from "../progress";
import { RevealText } from "../reveal";
import { Button, ErrorBox, Modal, Spinner } from "../ui";

/** The Run log's label, the same in every window. */
export const RUN_LOG = "Run log";

/** How the window's job is shown: as the dialog, or shrunk to its card. */
export type Followed = { minimized: boolean; minimize: () => void };

/** The card or the dialog for a job a store follows. raised counts the asks
 *  to open it (a new job, or Open): the card is shown for the ask it was
 *  minimized under, so a new ask opens the dialog with no frame of the card
 *  first. asCard says the last ask was for the card (a job the app found
 *  running as it started). initialMinimized, for the design harness, shows
 *  the first ask as the card. */
export function useFollowed({ raised, asCard, initialMinimized }: { raised: number; asCard?: boolean; initialMinimized?: boolean }): Followed {
  const [cardFor, setCardFor] = useState<number | null>(initialMinimized && raised > 0 ? raised : null);
  const firstAsCard = useRef(!!initialMinimized && raised === 0);
  useEffect(() => {
    if (firstAsCard.current && raised > 0) {
      firstAsCard.current = false;
      setCardFor(raised);
    }
  }, [raised]);
  return { minimized: !!asCard || cardFor === raised, minimize: () => setCardFor(raised) };
}

/** How a running job is stopped: Stop for one on this computer, Cancel run
 *  for one In Agentry. The dialog says label; the card says cardLabel. */
export type RunStop = {
  label: string;
  cardLabel?: string;
  stopping: boolean;
  onStop: () => void;
};

/** An action the job offers once it is over, beside Dismiss; busy while
 *  what it started is under way. */
export type RunAction = { label: string; onClick: () => void; busy?: boolean; busyLabel?: string; disabled?: boolean };

export type RunWindowProps = Followed & {
  /** Opens the dialog from the card (the store's show). */
  onOpen: () => void;
  /** Dismiss: the app forgets the job. */
  onDismiss: () => void;
  /** The card's accessible name. */
  label: string;
  /** The dialog's title. */
  title: string;
  /** The card's title; the dialog's when not given. */
  cardTitle?: React.ReactNode;
  running: boolean;

  // While it runs.
  /** What the job is doing now, beside the spinner. */
  spinner: string;
  /** What the job is and where it runs. */
  about?: React.ReactNode;
  /** Lines of where it stands, above its progress: a status, its credits. */
  status?: React.ReactNode;
  /** How far it has got, in full or compact (the card's). */
  progress?: (compact: boolean) => React.ReactNode;
  /** More of how far it has got, beneath the progress in the dialog: an
   *  image pull's bytes, an image build's steps. */
  prepare?: React.ReactNode;
  /** What running in the background means for this job. */
  background: React.ReactNode;
  stop?: RunStop | null;

  /** The job's Run log as shown (runLog.ts's logLines); null for a job
   *  that keeps none here (one In Agentry). */
  log?: string[] | null;
  /** The card's line in tone: a stage while it runs, how it ended after. */
  line?: string;
  tone?: Tone;
  /** An error beside the progress or the outcome, as Go or the registry
   *  said it. */
  error?: string | null;

  // Once it is over.
  /** The outcome: the result or the report, or why it failed. Without one
   *  the dialog says line, in tone, and error. */
  outcome?: React.ReactNode;
  /** The outcome already holds the Run log (a run's details). */
  logInOutcome?: boolean;
  /** What the job offers once it is over, as the primary action of the
   *  dialog and of the card ("Open the draft"). */
  primary?: RunAction | null;
  /** More actions of the dialog once it is over ("Back to the setup"). */
  actions?: React.ReactNode;
  /** More actions of the card ("Preview", "Follow it again"). */
  cardActions?: React.ReactNode;
  /** "Dismiss" by default. */
  dismissLabel?: string;
  /** Closing the dialog once the job is over keeps its card ("minimize",
   *  the default) or dismisses it, for a job whose outcome has been
   *  taken elsewhere. */
  closeOver?: "minimize" | "dismiss";
};

export function RunWindow(p: RunWindowProps) {
  const { running, stop, log } = p;
  const stopButton = (card: boolean) => stop && (
    <StopButton
      stopping={stop.stopping}
      onStop={stop.onStop}
      label={card ? stop.cardLabel ?? stop.label : stop.label}
    />
  );

  if (p.minimized) {
    return (
      <CornerCard
        label={p.label}
        running={running}
        title={p.cardTitle ?? p.title}
        line={p.error && running ? p.error : p.line}
        tone={p.error && running ? "bad" : p.tone}
        actions={<>
          {running && stopButton(true)}
          {p.cardActions}
          {!running && <Button onClick={p.onDismiss}>{p.dismissLabel ?? "Dismiss"}</Button>}
          {!running && p.primary
            ? <PrimaryButton action={p.primary} />
            : <Button variant="primary" onClick={p.onOpen}>Open</Button>}
        </>}
      >
        {running && p.progress?.(true)}
      </CornerCard>
    );
  }

  if (running) {
    return (
      <Modal title={p.title} onClose={p.minimize} wide>
        <Spinner label={p.spinner} />
        {p.about && <p className="muted small">{p.about}</p>}
        {p.status}
        {p.progress?.(false)}
        {p.prepare}
        <ErrorBox error={p.error ?? null} />
        {log && <FoldedLog label={RUN_LOG} lines={log} reveal />}
        <div className="row" style={{ justifyContent: "flex-end" }}>
          {stopButton(false)}
          <Button variant="primary" onClick={p.minimize}>Run in the background</Button>
        </div>
        <p className="muted small">{p.background}</p>
      </Modal>
    );
  }

  // A job that did not succeed shows its log open: it is where the why is.
  const logOpen = p.tone === "bad" || p.tone === "warn" || (!p.outcome && !p.tone);
  return (
    <Modal title={p.title} onClose={p.closeOver === "dismiss" ? p.onDismiss : p.minimize} wide>
      {p.outcome ?? (
        <>
          <h3 className={`run-outcome run-outcome-${p.tone || "bad"}`}><RevealText text={p.line ?? ""} /></h3>
          <ErrorBox error={p.error ?? null} />
        </>
      )}
      {log && log.length > 0 && !p.logInOutcome && <FoldedLog label={RUN_LOG} lines={log} reveal open={logOpen} />}
      <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
        {p.actions}
        <Button variant={p.primary ? undefined : "primary"} onClick={p.onDismiss}>{p.dismissLabel ?? "Dismiss"}</Button>
        {p.primary && <PrimaryButton action={p.primary} />}
      </div>
    </Modal>
  );
}

function PrimaryButton({ action }: { action: RunAction }) {
  return (
    <Button variant="primary" onClick={action.onClick} busy={action.busy} busyLabel={action.busyLabel} disabled={action.disabled}>
      {action.label}
    </Button>
  );
}
