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
// The run lock is unchanged: this run is the one local job. A run started
// meanwhile waits its turn in a card of its own, "Waiting to start", with
// Cancel, and takes this card over when it begins; any other job started
// meanwhile is refused with what is running. Every string the run wrote
// (the log, the error, the result) is drawn as text.
import { useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { failureOf, localFailureInput, reportText } from "../failedRun";
import { type LocalRun, type QueuedRun, headlineOf, outcome, queuedLine, shownLog } from "../localRun";
import { BytesBar, CornerCard, ProgressView, StepBar } from "../progress";
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
  const followed = useFollowed({ raised: s.raised, initialMinimized });
  const r = s.run;
  if (!s.shown || !r) return null;

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
      onOpen={showLocalRun}
      onDismiss={dismissLocalRun}
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
      stop={{ label: "Stop the run", cardLabel: "Stop", stopping: r.stopping, onStop: () => void stopLocalRun() }}
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
