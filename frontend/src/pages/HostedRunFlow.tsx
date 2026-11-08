// A run In Agentry, followed above the pages in its run window
// (RunWindow.tsx), as a run on this computer is (RunFlow): App mounts this, the run setup hands the run over
// once the registry has created it and closes, and App attaches a run
// already in flight when it starts, as its card. The dialog says where the
// run stands in the registry's words, its place in the queue, the time it
// has run against its cap and the credits it holds; Run in the background,
// or closing, shrinks it to a card in the corner. Cancel asks Agentry to
// stop it. Once it is over the dialog is its result, downloaded into its
// folder (the result first, Run details beneath), and the
// card says how it ended; Dismiss makes the app forget it.
//
// The desktop follows a run In Agentry and never drives it: every
// sentence here is the registry's, or says what the registry said.
import { useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { failureOf, hostedFailureInput, reportText } from "../failedRun";
import { hostedActive, hostedHeadline, hostedProgress, hostedTone, lostTouch } from "../hostedRun";
import { ProgressView } from "../progress";
import {
  type RunSetup, backToHostedSetup, cancelHostedRun, dismissHostedRun, followHostedRunAgain, showHostedRun, useHostedRun,
} from "../runs";
import { Button, ErrorBox } from "../ui";
import { FailedRunView } from "./FailedRun";
import { HostedResult, StarAsk } from "./HostedRun";
import type { RunFlowActions } from "./RunFlow";
import { RunResultView } from "./RunResult";
import { RunWindow, useFollowed } from "./RunWindow";

export default function HostedRunFlow({ onBackToSetup, onRunAgain, onReport, initialMinimized }: RunFlowActions & {
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useHostedRun();
  // An attach at start asks for the card; Open, a new run, or Follow it
  // again is a new ask, so it opens as the dialog.
  const followed = useFollowed({ raised: s.raised, asCard: s.asCard, initialMinimized });
  const [following, setFollowing] = useState(false);
  const r = s.run;
  if (!s.shown || !r) return null;

  const moving = !r.final;
  const lost = lostTouch(r);
  const canCancel = moving && hostedActive(r) && !r.cancelRequested;
  const again = () => {
    setFollowing(true);
    void followHostedRunAgain().finally(() => setFollowing(false));
  };
  const followAgain = lost && <Button onClick={again} disabled={following}>{following ? "Following…" : "Follow it again"}</Button>;
  const back = () => {
    const setup = backToHostedSetup();
    if (setup) onBackToSetup(setup);
  };
  const runAgainNow = onRunAgain && s.setup ? () => {
    const setup = backToHostedSetup();
    if (setup) onRunAgain(setup);
  } : undefined;
  // A run that failed In Agentry: the registry's class and error line, the
  // run page as the evidence, and the outputs that came down.
  const failure = moving || lost ? null : failureOf(hostedFailureInput(r));
  const failed = failure && (
    <FailedRunView
      failure={failure}
      runAgain={runAgainNow && { label: "Run again with the same answers", onClick: runAgainNow }}
      openRun={() => void RunAPI.OpenHostedRunPage(s.registry, r.id).catch(() => undefined)}
      openFolder={r.outputs.length > 0 && r.outDir ? () => void RunAPI.ShowRunFolder(r.id).catch(() => undefined) : undefined}
      report={onReport ? () => { followed.minimize(); onReport(reportText(failure, r.id)); } : undefined}
    />
  );
  return (
    <RunWindow
      {...followed}
      onOpen={showHostedRun}
      onDismiss={dismissHostedRun}
      label={`Run of ${r.ref} In Agentry`}
      title={moving ? `Running ${r.ref} In Agentry` : `Run ${r.ref} In Agentry`}
      cardTitle={moving ? `Running ${r.ref} In Agentry` : `${r.ref} In Agentry: ${lost ? "lost touch" : "the run is over"}`}
      running={moving}
      spinner={r.cancelRequested ? "Cancel asked for: Agentry is stopping the run…" : (r.statusLine || r.state)}
      about={<>
        The QuickAgent runs In Agentry, on one of Agentry&apos;s hosts, under the contract you accepted. This app follows it
        and never drives it: what shows here is what Agentry reports. When it ends, its outputs come down into{" "}
        <span className="mono">{r.outDir}</span>.
      </>}
      progress={(compact) => <HostedProgressView r={r} compact={compact} />}
      background={<>
        In the background, the run goes on In Agentry and this app follows it in a card in the corner. Quitting the app
        does not stop it; a run still going when the app starts again is followed again. Cancel asks Agentry to stop it and
        settle what it used.
      </>}
      stop={canCancel ? { label: "Cancel run", stopping: false, onStop: () => void cancelHostedRun() } : null}
      log={null}
      line={moving ? (r.cancelRequested ? "Cancel asked for." : "") : hostedHeadline(r)}
      tone={moving ? "" : hostedTone(r)}
      error={moving ? s.note : null}
      outcome={lost ? (
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
          lead={failed || undefined}
          resultJson={r.resultJson ?? ""}
          knowledge={r.knowledgeLine || undefined}
          ask={<StarAsk registry={s.registry} run={r} />}
        >
          <HostedResult run={r} />
        </RunResultView>
      )}
      actions={<>
        {followAgain}
        {s.setup && <Button onClick={back}>Back to the setup</Button>}
      </>}
      cardActions={!moving && followAgain}
    />
  );
}

// HostedProgressView is where the run stands as the registry reports it:
// its place in the queue while it waits, then the time it has run against
// its cap, and the credits it holds. The registry reports turns only once
// the run is over, so there are none while it runs, and a run In Agentry
// is never stated in tokens.
function HostedProgressView({ r, compact }: { r: app.HostedRunView; compact?: boolean }) {
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
