// A run In Agentry, followed above the pages (agentry-desktop#466; plan
// desktop-review-key-and-run-cards.md §5.2; D10, D15), as a run on this
// computer is (RunFlow): App mounts this, the run setup hands the run over
// once the registry has created it and closes, and App attaches a run
// already in flight when it starts, as its card. The dialog says where the
// run stands in the registry's words, its place in the queue, the time it
// has run against its cap and the credits it holds; Run in the background,
// or closing, shrinks it to a card in the corner. Cancel asks Agentry to
// stop it. Once it is over the dialog is its result, downloaded into its
// folder (the result first, Run details beneath, D-035's rules), and the
// card says how it ended; Dismiss makes the app forget it.
//
// The desktop follows a run In Agentry and never drives it (D-022): every
// sentence here is the registry's, or says what the registry said.
import { useEffect, useRef, useState } from "react";
import type { main } from "../../wailsjs/go/models";
import { hostedActive, hostedHeadline, hostedProgress, hostedTone, lostTouch } from "../hostedRun";
import { CornerCard, ProgressView, RunningView } from "../progress";
import {
  type RunSetup, backToHostedSetup, cancelHostedRun, dismissHostedRun, followHostedRunAgain, showHostedRun, useHostedRun,
} from "../runs";
import { Button, ErrorBox, Modal } from "../ui";
import { HostedResult } from "./HostedRun";
import { RunResultView } from "./RunResult";

export default function HostedRunFlow({ onBackToSetup, initialMinimized }: {
  /** Opens the run's setup again, with the answers as they were. */
  onBackToSetup: (s: RunSetup) => void;
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useHostedRun();
  const r = s.run;
  // The card is shown for the ask it was minimized under, or when the ask
  // itself was for the card (an attach at start); Open, a new run, or
  // Follow it again is a new ask, so it opens as the dialog.
  const [cardFor, setCardFor] = useState<number | null>(null);
  const minimized = s.asCard || cardFor === s.raised;
  const minimize = () => setCardFor(s.raised);
  // Harness-only: the first ask to open, when the run is followed, is
  // shown as the card.
  const firstAsCard = useRef(!!initialMinimized);
  useEffect(() => {
    if (firstAsCard.current && s.raised > 0) {
      firstAsCard.current = false;
      setCardFor(s.raised);
    }
  }, [s.raised]);
  const [following, setFollowing] = useState(false);
  if (!s.shown || !r) return null;

  const moving = !r.final;
  const lost = lostTouch(r);
  const canCancel = moving && hostedActive(r) && !r.cancelRequested;
  const title = moving ? `Running ${r.ref} In Agentry` : `${r.ref} In Agentry: ${lost ? "lost touch" : "the run is over"}`;
  const cancel = () => void cancelHostedRun();
  const again = () => {
    setFollowing(true);
    void followHostedRunAgain().finally(() => setFollowing(false));
  };
  const followAgain = <Button onClick={again} disabled={following}>{following ? "Following…" : "Follow it again"}</Button>;

  if (minimized) {
    return (
      <CornerCard
        label={`Run of ${r.ref} In Agentry`}
        running={moving}
        title={title}
        line={moving ? (r.cancelRequested ? "Cancel asked for." : "") : hostedHeadline(r)}
        tone={moving ? "" : hostedTone(r)}
        actions={<>
          {canCancel && <Button onClick={cancel}>Cancel run</Button>}
          {lost && followAgain}
          {!moving && <Button onClick={dismissHostedRun}>Dismiss</Button>}
          <Button variant="primary" onClick={showHostedRun}>Open</Button>
        </>}
      >
        {moving && <HostedProgressView r={r} compact />}
      </CornerCard>
    );
  }

  if (moving) {
    return (
      <Modal title={title} onClose={minimize} wide>
        <RunningView
          spinner={r.cancelRequested ? "Cancel asked for: Agentry is stopping the run…" : (r.statusLine || r.state)}
          about={<>
            The QuickAgent runs In Agentry, on one of Agentry&apos;s hosts, under the contract you accepted. This app follows it
            and never drives it: what shows here is what Agentry reports. When it ends, its outputs come down into{" "}
            <span className="mono">{r.outDir}</span>.
          </>}
          actions={<>
            {canCancel && <Button onClick={cancel}>Cancel run</Button>}
            <Button variant="primary" onClick={minimize}>Run in the background</Button>
          </>}
          background={<>
            In the background, the run goes on In Agentry and this app follows it in a card in the corner. Quitting the app
            does not stop it; a run still going when the app starts again is followed again. Cancel asks Agentry to stop it and
            settle what it used.
          </>}
        >
          <HostedProgressView r={r} />
          <ErrorBox error={s.note} />
        </RunningView>
      </Modal>
    );
  }

  const back = () => {
    const setup = backToHostedSetup();
    if (setup) onBackToSetup(setup);
  };
  // Closing the dialog of a run that is over keeps its card: only Dismiss
  // makes the app forget the run.
  return (
    <Modal title={`Run ${r.ref} In Agentry`} onClose={minimize} wide>
      {lost ? (
        <>
          <h3 className="run-outcome run-outcome-warn">{hostedHeadline(r)}</h3>
          <p>
            The run may still be going In Agentry: nothing was downloaded, because this app does not know how it ended. Follow it
            again to see how it ends and bring its outputs down into <span className="mono">{r.outDir}</span>.
          </p>
          <ErrorBox error={r.downloadError ?? null} />
          <ErrorBox error={s.note} />
        </>
      ) : (
        <RunResultView
          runId={r.id}
          headline={hostedHeadline(r)}
          tone={hostedTone(r)}
          error={r.downloadError || undefined}
          resultJson={r.resultJson ?? ""}
          knowledge={r.knowledgeLine || undefined}
        >
          <HostedResult run={r} />
        </RunResultView>
      )}
      <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
        {lost && followAgain}
        {s.setup && <Button onClick={back}>Back to the setup</Button>}
        <Button variant="primary" onClick={dismissHostedRun}>Dismiss</Button>
      </div>
    </Modal>
  );
}

// HostedProgressView is where the run stands as the registry reports it:
// its place in the queue while it waits, then the time it has run against
// its cap, and the credits it holds. The registry reports turns only once
// the run is over, so there are none while it runs, and a run In Agentry
// is never stated in tokens (agentry-desktop#457).
function HostedProgressView({ r, compact }: { r: main.HostedRunView; compact?: boolean }) {
  const queued = r.status === "queued" && (r.queuePosition ?? 0) > 0;
  const held = r.holdCredits === 1 ? "1 credit held" : `${r.holdCredits} credits held`;
  const p = hostedProgress(r);
  if (!p) {
    return <p className="muted small">{queued ? `Waiting for a host: number ${r.queuePosition} in the queue. ` : ""}{held}.</p>;
  }
  return (
    <ProgressView
      progress={p}
      job="run"
      compact={compact}
      turns={false}
      fact={() => held}
      facts={() => <><dt>Credits held</dt><dd>{r.holdCredits}</dd></>}
    />
  );
}
