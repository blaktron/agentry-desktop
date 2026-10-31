// What a run that did not finish shows, in the desktop's frame: the headline
// names the exit class and what it means; the facts say what failed and
// where, the cause (the runner's last error line, drawn as text) and where
// the evidence is; the actions are Run again where the class says a retry
// may help, Open folder or Open run, and Report, which opens Report bug
// with the facts filled in. The same view serves a run on this computer and
// a run In Agentry, live in its dialog and read back on its run page
// (failedRun.ts has the facts).
import type { ReactNode } from "react";
import { type Failure, failureHeadline, retryLine } from "../failedRun";
import { RevealText } from "../reveal";
import { Button } from "../ui";

export function FailedRunView({ failure, runAgain, openFolder, openRun, report, children }: {
  failure: Failure;
  /** Run again: with the same answers from a live dialog, or the run setup
   *  from a run page. Drawn only when the class says a retry may help. */
  runAgain?: { label: string; onClick: () => void };
  openFolder?: () => void;
  openRun?: () => void;
  report?: () => void;
  /** Drawn after the facts: the kept log, or the result's files. */
  children?: ReactNode;
}) {
  const retry = retryLine(failure);
  return (
    <section className="run-failure" aria-labelledby="run-failure-headline" data-run-failure={failure.className || "unknown"}>
      <h3 id="run-failure-headline" className="run-outcome run-outcome-bad">{failureHeadline(failure)}</h3>
      {retry && <p className="small" data-run-failure-retry={failure.retryLikelyHelps ? "likely" : "unlikely"}>{retry}</p>}
      <dl className="run-failure-facts">
        <dt>What failed</dt>
        <dd>
          <span className="mono">{failure.what}</span>{failure.version && <span className="muted"> {failure.version}</span>}, {failure.where.charAt(0).toLowerCase()}{failure.where.slice(1)}.
        </dd>
        {failure.className && (
          <>
            <dt>Why</dt>
            <dd><span className="badge bad">{failure.className}</span>{failure.meaning && <> {failure.meaning}.</>}</dd>
          </>
        )}
        <dt>Cause</dt>
        <dd>
          {failure.cause
            ? <pre className="run-failure-cause selectable" data-run-failure-cause><RevealText text={failure.cause} /></pre>
            : <span className="muted">The runner recorded no error line.</span>}
        </dd>
        <dt>Evidence</dt>
        <dd>{failure.evidence.map((line, i) => <div key={i} className="selectable">{line}</div>)}</dd>
      </dl>
      {(runAgain || openFolder || openRun || report) && (
        <div className="row run-failure-actions">
          {runAgain && failure.retryLikelyHelps && <Button variant="primary" data-run-again onClick={runAgain.onClick}>{runAgain.label}</Button>}
          {openFolder && <Button onClick={openFolder}>Open folder</Button>}
          {openRun && <Button onClick={openRun}>Open run In Agentry</Button>}
          {report && <Button onClick={report}>Report</Button>}
        </div>
      )}
      {children}
    </section>
  );
}
