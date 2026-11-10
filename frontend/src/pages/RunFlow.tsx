// A run on this computer, followed above the pages in its run window
// (RunWindow.tsx), as every built-in's job is: App mounts this, the run
// setup hands the run over at Run and closes once it has begun, and the
// window shows the time used against the run's limit, its turns and tokens,
// an image pull's or build's progress while it prepares, and its Run log.
// Stop stops only a run (RunAPI.StopRun). Once the run is over the dialog is
// its result (the result first, Run details with the log beneath); Back to
// the setup reopens the setup on that QuickAgent with the answers as they
// were.
//
// The window follows one run at a time. Several runs can be under way at
// once (Settings → Run defaults → Runs at once), and a run started while
// every slot is taken waits its turn. With more than one run, or any
// waiting, one card in the corner lists them all: each run under way with
// its progress, Open and Stop; each run over with how it ended, Open and
// Dismiss; and the first waiting runs with Cancel, then "and N more", which
// lists the rest in a scrolling list. The followed run's own card gives way
// to it; a run the window does not follow is always in it. Any other job
// started meanwhile is refused with what is running.
// Every string the run wrote (the log, the error, the result) is drawn as
// text.
import { useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { failureOf, localFailureInput, reportText } from "../failedRun";
import { type LocalRun, type QueuedRun, headlineOf, outcome, queuedLine, runsCardShown, runsCardTitle, shownLog, WAITING_SHOWN, waitingShown } from "../localRun";
import { BytesBar, CornerCard, ProgressView, StepBar, StopButton } from "../progress";
import { type RunSetup, backToSetup, cancelQueuedRun, dismissLocalRun, showLocalRun, stopLocalRun, useLocalRun } from "../runs";
import { Button } from "../ui";
import { bytesProgress, stepProgress } from "../bytesProgress";
import { FailedRunView } from "./FailedRun";
import { LocalRunDetails, RunResultView } from "./RunResult";
import { RunWindow, useFollowed } from "./RunWindow";

export type RunFlowActions = {
  /** Opens the run's setup again, with the answers as they were. */
  onBackToSetup: (s: RunSetup) => void;
  /** Run again: the setup opens with the answers as they were and starts the
   *  run at once. Offered on a failure whose class says a retry may help. */
  onRunAgain?: (s: RunSetup) => void;
  /** Report: opens Report bug with the failure's facts filled in. */
  onReport?: (text: string) => void;
};

/** How far the run's preparation has got, read from its log: an image
 *  pull's bytes and an image build's steps, while either is under way. */
function PrepareBars({ log }: { log: string[] }) {
  const pull = bytesProgress(log);
  const build = stepProgress(log);
  return (
    <>
      {pull && <BytesBar progress={pull} label="Pulled" />}
      {build && <StepBar step={build.step} steps={build.steps} label="Image build" />}
    </>
  );
}

export default function RunFlow(props: RunFlowActions & {
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useLocalRun();
  const many = runsCardShown(s.runs, s.queue, s.run?.runId ?? null);
  return (
    <>
      <FollowedRun {...props} cardElsewhere={many} />
      {many && <RunsCard runs={s.runs} queue={s.queue} />}
    </>
  );
}

// RunsCard lists every run followed and every run waiting to start: the runs
// under way first, with their progress, Open and Stop; then the runs over,
// with how each ended, Open and Dismiss; then the waiting runs, the next
// first, each with what it waits for and Cancel, the first few of them
// until "and N more" lists the rest.
function RunsCard({ runs, queue }: { runs: LocalRun[]; queue: QueuedRun[] }) {
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);
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
  const going = runs.filter((r) => !r.over);
  const over = runs.filter((r) => r.over).reverse();
  const waiting = waitingShown(queue, expanded);
  return (
    <CornerCard
      label="Runs on this computer"
      running={going.length > 0}
      title={runsCardTitle(runs, queue)}
      line={error}
      tone={error ? "bad" : undefined}
      actions={null}
    >
      {(going.length > 0 || over.length > 0) && (
        <ul className="run-queue runs-list" data-runs-list aria-label="Runs">
          {going.map((r) => (
            <li key={r.runId ?? r.ref} className="run-queue-item">
              <div className="grow">
                <strong>{r.ref}</strong>
                <RunProgressView r={r} compact />
              </div>
              <StopButton stopping={r.stopping} onStop={() => void stopLocalRun(r.runId ?? undefined)} />
              <Button variant="primary" onClick={() => showLocalRun(r.runId ?? undefined)}>Open</Button>
            </li>
          ))}
          {over.map((r) => {
            const o = outcome(r);
            return (
              <li key={r.runId ?? r.ref} className="run-queue-item">
                <div className="grow">
                  <strong>{r.ref}</strong>
                  <div className={`small run-outcome-${o.tone || "bad"}`}>{o.line}</div>
                </div>
                <Button onClick={() => dismissLocalRun(r.runId ?? undefined)}>Dismiss</Button>
                <Button variant="primary" onClick={() => showLocalRun(r.runId ?? undefined)}>Open</Button>
              </li>
            );
          })}
        </ul>
      )}
      {queue.length > 0 && (
        <ul className={`run-queue${expanded ? " run-queue-all" : ""}`} data-run-queue aria-label="Waiting to start">
          {waiting.shown.map((q) => (
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
      )}
      {waiting.more > 0 && (
        <Button variant="link" onClick={() => setExpanded(true)} data-runs-more>
          {`and ${waiting.more} more`}
        </Button>
      )}
      {expanded && queue.length > WAITING_SHOWN && (
        <Button variant="link" onClick={() => setExpanded(false)}>Show fewer</Button>
      )}
    </CornerCard>
  );
}

function FollowedRun({ onBackToSetup, onRunAgain, onReport, initialMinimized, cardElsewhere }: RunFlowActions & {
  initialMinimized?: boolean;
  /** The runs card lists this run, so its own card is not drawn. */
  cardElsewhere?: boolean;
}) {
  const s = useLocalRun();
  const followed = useFollowed({ raised: s.raised, initialMinimized });
  const r = s.run;
  if (!s.shown || !r) return null;
  if (cardElsewhere && followed.minimized) return null;

  const running = !r.over;
  const { line, tone } = outcome(r);
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
  const failure = running ? null : res ? failureOf(localFailureInput(res, r.stopped)) : r.stopped ? null : failureOf({ where: "here", status: "error", cause: r.error ?? "", ref: r.ref });
  const runId = res?.runId ?? r.runId ?? "";
  // Report shrinks the dialog to its card first, so the Report bug card it
  // opens is the one in front.
  const failed = failure && (
    <FailedRunView
      failure={failure}
      runAgain={again && { label: "Run again with the same answers", onClick: again }}
      openFolder={res?.outDir ? () => void RunAPI.ShowRunFolder(res.runId).catch(() => undefined) : undefined}
      report={onReport ? () => { followed.minimize(); onReport(reportText(failure, runId)); } : undefined}
    />
  );
  return (
    <RunWindow
      {...followed}
      onOpen={() => showLocalRun()}
      onDismiss={() => dismissLocalRun()}
      label={`Run of ${r.ref}`}
      title={running ? `Running ${r.ref}` : `Run ${r.ref}`}
      cardTitle={running ? `Running ${r.ref}` : `${r.ref}: the run is over`}
      running={running}
      spinner={r.stopping ? "Stopping the run…" : r.progress?.prepared ? "The QuickAgent is running…" : "Preparing the run…"}
      about={<>
        The QuickAgent runs on this computer in its containers, under the contract you accepted. Preparing takes longer the
        first time: the release is installed and the images it needs are pulled or built.
      </>}
      progress={(compact) => <RunProgressView r={r} compact={compact} />}
      prepare={!r.progress?.prepared && <PrepareBars log={r.log} />}
      background={<>
        In the background, the run goes on in a card in the corner and the rest of the app stays usable. Stop ends it, and
        its receipts are written as far as it got.
      </>}
      stop={{ label: "Stop the run", cardLabel: "Stop", stopping: r.stopping, onStop: () => void stopLocalRun(r.runId ?? undefined) }}
      log={shownLog(r)}
      line={line}
      tone={tone}
      error={failed ? null : r.error}
      outcome={res ? (
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
      ) : failed || undefined}
      logInOutcome={!!res}
      actions={<Button onClick={back}>Back to the setup</Button>}
    />
  );
}

// RunProgressView is how far the run has got: the shared view, whose compact
// form names the turns after the clock. Its budgets arrive once the run is
// prepared; until then the clock runs with no limit shown.
function RunProgressView({ r, compact }: { r: LocalRun; compact?: boolean }) {
  return <ProgressView progress={r.progress} job="run" compact={compact} fact={(p) => (p.turns === 1 ? "1 turn" : `${p.turns} turns`)} />;
}
