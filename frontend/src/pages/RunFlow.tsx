// A run on this computer, followed above the pages, as a draft and a
// knowledge build are: App mounts this, the run setup hands the
// run over at Run and closes once it has begun, and the dialog shows the time
// used against the run's limit, its turns and tokens over its log folded
// beneath. Run in the background, or closing the dialog while the run goes,
// shrinks it to a card in the bottom-right corner that keeps the progress,
// stops the run (RunAPI.StopRun, which stops only a run) and opens the dialog
// again. Once the run is over the dialog is its result (the result first,
// Run details beneath) and the card says how it ended; Dismiss
// makes the app forget it, and Back to the setup reopens the setup on that
// QuickAgent with the answers as they were.
//
// The run lock is unchanged: this run is the one local job. A run started
// meanwhile waits its turn in a card of its own, "Waiting to start", with
// Cancel, and takes this card over when it begins; any other job started
// meanwhile is refused with what is running. Every string the run wrote
// (the log, the error, the result) is drawn as text.
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { failureOf, localFailureInput, reportText } from "../failedRun";
import { type LocalRun, type QueuedRun, headlineOf, outcome, queuedLine, shownLog } from "../localRun";
import { CornerCard, FoldedLog, LogPane, ProgressView, RunningView, StopButton } from "../progress";
import { type RunSetup, backToSetup, cancelQueuedRun, dismissLocalRun, showLocalRun, stopLocalRun, useLocalRun } from "../runs";
import { Button, ErrorBox, Modal } from "../ui";
import { FailedRunView } from "./FailedRun";
import { LocalRunDetails, RunResultView } from "./RunResult";

export type RunFlowActions = {
  /** Opens the run's setup again, with the answers as they were. */
  onBackToSetup: (s: RunSetup) => void;
  /** Run again: the setup opens with the answers as they were and starts the
   *  run at once. Offered on a failure whose class says a retry may help. */
  onRunAgain?: (s: RunSetup) => void;
  /** Report: opens Report bug with the failure's facts filled in. */
  onReport?: (text: string) => void;
};

export default function RunFlow(props: RunFlowActions & {
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useLocalRun();
  return (
    <>
      <FollowedRun {...props} />
      {s.queue.length > 0 && <QueueCard queue={s.queue} />}
    </>
  );
}

// QueueCard lists the runs waiting to start, the next first, each with
// what it waits for and Cancel, which takes it out of the queue unstarted.
function QueueCard({ queue }: { queue: QueuedRun[] }) {
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState<string | null>(null);
  const cancel = async (id: string) => {
    setCancelling(id);
    setError("");
    try {
      await cancelQueuedRun(id);
    } catch (e) {
      setError(String(e));
    } finally {
      setCancelling(null);
    }
  };
  return (
    <CornerCard
      label="Runs waiting to start"
      running={false}
      title={queue.length === 1 ? "1 run waiting to start" : `${queue.length} runs waiting to start`}
      line={error}
      tone={error ? "bad" : undefined}
      actions={null}
    >
      <ul className="run-queue" data-run-queue>
        {queue.map((q) => (
          <li key={q.runId} className="run-queue-item">
            <div className="grow">
              <strong>{q.ref}</strong>
              <div className="small muted">{queuedLine(q)}</div>
            </div>
            <Button onClick={() => void cancel(q.runId)} disabled={cancelling === q.runId} busy={cancelling === q.runId} busyLabel="Cancelling…">
              Cancel
            </Button>
          </li>
        ))}
      </ul>
    </CornerCard>
  );
}

function FollowedRun({ onBackToSetup, onRunAgain, onReport, initialMinimized }: RunFlowActions & {
  initialMinimized?: boolean;
}) {
  const s = useLocalRun();
  const r = s.run;
  // The card is shown for the ask it was minimized under; a new ask (Open,
  // or a new run) opens the dialog again, with no frame of the card first.
  const [cardFor, setCardFor] = useState<number | null>(null);
  const minimized = cardFor === s.raised;
  const minimize = () => setCardFor(s.raised);
  // Harness-only: the first ask to open, when the run begins, is shown as
  // the card.
  const firstAsCard = useRef(!!initialMinimized);
  useEffect(() => {
    if (firstAsCard.current && s.raised > 0) {
      firstAsCard.current = false;
      setCardFor(s.raised);
    }
  }, [s.raised]);
  if (!s.shown || !r) return null;

  const running = !r.over;
  const { line, tone } = outcome(r);
  const title = running ? `Running ${r.ref}` : `${r.ref}: the run is over`;
  const stop = () => void stopLocalRun();

  if (minimized) {
    return (
      <CornerCard
        label={`Run of ${r.ref}`}
        running={running}
        title={title}
        line={running ? "" : line}
        tone={tone}
        actions={<>
          {running && <StopButton stopping={r.stopping} onStop={stop} />}
          {!running && <Button onClick={dismissLocalRun}>Dismiss</Button>}
          <Button variant="primary" onClick={showLocalRun}>Open</Button>
        </>}
      >
        {running && <RunProgressView r={r} compact />}
      </CornerCard>
    );
  }

  if (running) {
    return (
      <Modal title={title} onClose={minimize} wide>
        <RunningView
          spinner={r.stopping ? "Stopping the run…" : r.progress?.prepared ? "The QuickAgent is running…" : "Preparing the run…"}
          about={<>
            The QuickAgent runs on this computer in its containers, under the contract you accepted. Preparing takes longer the
            first time: the release is installed and the images it needs are pulled or built.
          </>}
          actions={<>
            <StopButton stopping={r.stopping} onStop={stop} label="Stop the run" />
            <Button variant="primary" onClick={minimize}>Run in the background</Button>
          </>}
          background={<>
            In the background, the run goes on in a card in the corner and the rest of the app stays usable. Stop ends it, and
            its receipts are written as far as it got.
          </>}
        >
          <RunProgressView r={r} />
          <FoldedLog label="Run log" lines={shownLog(r)} reveal />
        </RunningView>
      </Modal>
    );
  }

  const res = r.result;
  const back = () => {
    const s = backToSetup();
    if (s) onBackToSetup(s);
  };
  // Run again: the same answers, started at once.
  const again = onRunAgain && s.setup ? () => {
    const setup = backToSetup();
    if (setup) onRunAgain(setup);
  } : undefined;
  // A run that failed: its facts and actions stand where the headline was,
  // for a run with a result and one that ended before it had one alike.
  const failure = res ? failureOf(localFailureInput(res, r.stopped)) : r.stopped ? null : failureOf({ where: "here", status: "error", cause: r.error ?? "", ref: r.ref });
  const runId = res?.runId ?? r.runId ?? "";
  const failed = failure && (
    <FailedRunView
      failure={failure}
      runAgain={again && { label: "Run again with the same answers", onClick: again }}
      openFolder={res?.outDir ? () => void RunAPI.ShowRunFolder(res.runId).catch(() => undefined) : undefined}
      report={onReport ? () => onReport(reportText(failure, runId)) : undefined}
    />
  );
  // Closing the dialog of a run that is over keeps its card: only Dismiss
  // makes the app forget the run.
  return (
    <Modal title={`Run ${r.ref}`} onClose={minimize} wide>
      {res ? (
        <RunResultView
          runId={res.runId}
          headline={headlineOf(res, r.stopped).line}
          tone={headlineOf(res, r.stopped).tone}
          error={failure ? undefined : res.error || undefined}
          lead={failed || undefined}
          resultJson={res.resultJson}
          knowledge={res.knowledge}
          decode={res.decode}
          unsafeExec={res.unsafeExec}
        >
          <LocalRunDetails result={res} log={shownLog(r)} />
        </RunResultView>
      ) : (
        <>
          {failed ?? <h3 className={`run-outcome run-outcome-${tone || "bad"}`}>{line}</h3>}
          {!failed && <ErrorBox error={r.error} />}
          {r.log.length > 0 && <LogPane label="Run log" lines={shownLog(r)} reveal />}
        </>
      )}
      <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
        <Button onClick={back}>Back to the setup</Button>
        <Button variant="primary" onClick={dismissLocalRun}>Dismiss</Button>
      </div>
    </Modal>
  );
}

// RunProgressView is how far the run has got: the shared view, whose compact
// form names the turns after the clock. Its budgets arrive once the run is
// prepared; until then the clock runs with no limit shown.
function RunProgressView({ r, compact }: { r: LocalRun; compact?: boolean }) {
  return <ProgressView progress={r.progress} job="run" compact={compact} fact={(p) => (p.turns === 1 ? "1 turn" : `${p.turns} turns`)} />;
}
