// A run on this computer, followed above the pages (agentry-desktop#465;
// plan desktop-review-key-and-run-cards.md §5.2, §5.4; D5, D9, D10), as a
// draft and a knowledge build are: App mounts this, the run setup hands the
// run over at Run and closes once it has begun, and the dialog shows the time
// used against the run's limit, its turns and tokens over its log folded
// beneath. Run in the background, or closing the dialog while the run goes,
// shrinks it to a card in the bottom-right corner that keeps the progress,
// stops the run (RunAPI.StopRun, which stops only a run) and opens the dialog
// again. Once the run is over the dialog is its result (the result first,
// Run details beneath, D-035's rules) and the card says how it ended; Dismiss
// makes the app forget it, and Back to the setup reopens the setup on that
// QuickAgent with the answers as they were.
//
// The run lock is unchanged: this run is the one local job, and anything
// started meanwhile is refused with what is running (D16). Every string the
// run wrote (the log, the error, the result) is drawn as text.
import { useEffect, useRef, useState } from "react";
import { type LocalRun, headlineOf, outcome } from "../localRun";
import { CornerCard, FoldedLog, LogPane, ProgressView, RunningView, StopButton } from "../progress";
import { type RunSetup, backToSetup, dismissLocalRun, showLocalRun, stopLocalRun, useLocalRun } from "../runs";
import { Button, ErrorBox, Modal } from "../ui";
import { LocalRunDetails, RunResultView } from "./RunResult";

export default function RunFlow({ onBackToSetup, initialMinimized }: {
  /** Opens the run's setup again, with the answers as they were. */
  onBackToSetup: (s: RunSetup) => void;
  /** Harness-only: open as the corner card. */
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
          <FoldedLog label="Run log" lines={r.log} />
        </RunningView>
      </Modal>
    );
  }

  const res = r.result;
  const back = () => {
    const s = backToSetup();
    if (s) onBackToSetup(s);
  };
  // Closing the dialog of a run that is over keeps its card: only Dismiss
  // makes the app forget the run.
  return (
    <Modal title={`Run ${r.ref}`} onClose={minimize} wide>
      {res ? (
        <RunResultView
          runId={res.runId}
          headline={headlineOf(res, r.stopped).line}
          tone={headlineOf(res, r.stopped).tone}
          error={res.error || undefined}
          resultJson={res.resultJson}
          knowledge={res.knowledge}
          unsafeExec={res.unsafeExec}
        >
          <LocalRunDetails result={res} log={r.log} />
        </RunResultView>
      ) : (
        <>
          <h3 className={`run-outcome run-outcome-${tone || "bad"}`}>{line}</h3>
          <ErrorBox error={r.error} />
          {r.log.length > 0 && <LogPane label="Run log" lines={r.log} />}
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
