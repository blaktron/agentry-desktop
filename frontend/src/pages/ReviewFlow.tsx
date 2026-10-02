// A review by the Examiner, followed above the pages (agentry-desktop#469;
// plan desktop-review-key-and-run-cards.md §6.3, D24), as a run, a draft and
// a knowledge build are: App mounts this, and the dialog opens once Go
// reports that the review has begun (after its native dialog, inside the run
// lock). While it runs, the dialog shows the stage (the registry's rules
// here, the Examiner, the report), the time against its limit, its turns and
// tokens over its log folded beneath; Run in the background, or closing the
// dialog, shrinks it to a card in the bottom-right corner with its own Stop
// (BuildAPI.StopReview, which stops only a review). Once it is over the
// dialog is its report, and Dismiss makes the app forget its folder.
//
// The report (ReviewReport) shows the outcome the pipeline would be expected
// to reach, every finding under the kind it belongs to with its file and
// line, what did not run here, and that the registry scans again at Submit.
// It never says what the registry will do, and never calls the QuickAgent
// anything but what was found. Every string the QuickAgent or the
// Examiner's model could have written is drawn as text, with its invisible
// characters shown.
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import type { main } from "../../wailsjs/go/models";
import { CornerCard, FoldedLog, LogPane, ProgressView, RunningView, StopButton } from "../progress";
import { RevealText } from "../reveal";
import { type ReviewProgress, outcome, outcomeTone } from "../review";
import { dismissReview, showReview, stopReview, useReview } from "../reviews";
import { Button, Card, ErrorBox, Excerpt, Modal, Notice, Sev } from "../ui";

const STAGE: Record<Exclude<ReviewProgress["stage"], "">, string> = {
  rules: "The registry's rules are running here",
  examining: "The Examiner is reading the QuickAgent, and AI Review is answering",
  reporting: "Writing the report",
};

function stageLine(p: ReviewProgress | null): string {
  return (p?.stage && STAGE[p.stage]) || "Starting the review";
}

export default function ReviewFlow({ initialMinimized }: {
  /** Harness-only: open as the corner card. */
  initialMinimized?: boolean;
}) {
  const s = useReview();
  const r = s.review;
  const [cardFor, setCardFor] = useState<number | null>(null);
  const minimized = cardFor === s.raised;
  const minimize = () => setCardFor(s.raised);
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
  const name = r.view?.ref ?? r.dir;
  const stop = () => void stopReview();

  if (minimized) {
    return (
      <CornerCard
        label="Review Your QuickAgents locally"
        running={running}
        title={running ? "Reviewing a QuickAgent here" : "The review is over"}
        line={running ? stageLine(r.progress) : line}
        tone={tone}
        actions={<>
          {running && <StopButton stopping={r.stopping} onStop={stop} />}
          {!running && <Button onClick={dismissReview}>Dismiss</Button>}
          <Button variant="primary" onClick={showReview}>Open</Button>
        </>}
      >
        {running && <ReviewProgressView progress={r.progress} compact />}
      </CornerCard>
    );
  }

  if (running) {
    return (
      <Modal title="Reviewing a QuickAgent on this computer" onClose={minimize} wide>
        <RunningView
          spinner={r.stopping ? "Stopping the review…" : `${stageLine(r.progress)}…`}
          about={<>
            The Examiner reviews <span className="mono"><RevealText text={name} /></span> on this computer: the registry&apos;s
            rules run here, then AI Review reads every file with your key before the Examiner&apos;s model does. Nothing of the
            QuickAgent runs, and nothing is submitted. A review takes a few minutes.
          </>}
          actions={<>
            <StopButton stopping={r.stopping} onStop={stop} label="Stop the review" />
            <Button variant="primary" onClick={minimize}>Run in the background</Button>
          </>}
          background={<>
            In the background, the review goes on in a card in the corner and the rest of the app stays usable. Stop ends it,
            and the report says what of it ran.
          </>}
        >
          <ReviewProgressView progress={r.progress} />
          <FoldedLog label="Review log" lines={r.log} reveal />
        </RunningView>
      </Modal>
    );
  }

  return (
    <Modal title="Review Your QuickAgents locally" onClose={minimize} wide>
      {r.view ? (
        <>
          {r.stopped && <Notice tone="warn">You stopped the review. The report says what of it ran.</Notice>}
          <ReviewReport v={r.view} log={r.log} />
        </>
      ) : (
        <>
          <h3 className={`run-outcome run-outcome-${tone || "bad"}`}>{line}</h3>
          {r.error && <div className="error-box selectable"><RevealText text={r.error} /></div>}
          {r.log.length > 0 && <LogPane label="Review log" lines={r.log} reveal />}
        </>
      )}
      <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
        <Button variant="primary" onClick={dismissReview}>Dismiss</Button>
      </div>
    </Modal>
  );
}

function ReviewProgressView({ progress, compact }: { progress: ReviewProgress | null; compact?: boolean }) {
  return <ProgressView progress={progress} job="review" compact={compact} fact={(p) => (p.turns === 1 ? "1 turn" : `${p.turns} turns`)} />;
}

// at is a finding's place: its file and, when it has one, its line.
function At({ file, line }: { file?: string; line?: number }) {
  if (!file) return null;
  return <div className="loc mono"><RevealText text={line ? `${file}:${line}` : file} /></div>;
}

function Finding({ f, kind }: { f: main.ReviewFindingView; kind?: string }) {
  return (
    <div className="finding">
      <div className="row">
        {f.severity && <Sev severity={f.severity} />}
        {kind ? <span className="badge">{kind}</span> : <span className="mono"><RevealText text={f.scanner} /></span>}
        <strong><RevealText text={f.title} /></strong>
        {f.confidence && <span className="badge">{f.confidence}</span>}
      </div>
      <At file={f.file} line={f.line} />
      <Excerpt text={f.excerpt} />
      {f.detail && <div className="muted small selectable"><RevealText text={f.detail} /></div>}
    </div>
  );
}

const READ_KIND: Record<string, string> = { secret: "a secret", script: "a script", instruction: "an instruction", other: "other" };

/** The report of a finished review: the expected outcome, the three kinds of
 *  finding, what did not run here, and the notice. */
export function ReviewReport({ v, log }: { v: main.ReviewView; log: string[] }) {
  const [err, setErr] = useState<string | null>(null);
  const showFolder = () => RunAPI.ShowRunFolder(v.runId).catch((e) => setErr(String(e)));
  const policy = v.policyFromRegistry
    ? <>the registry&apos;s baseline policy, which publishes up to <strong>{v.publishesUpTo}</strong></>
    : <>the baseline policy compiled into this app (the registry&apos;s could not be read: <RevealText text={v.policyError ?? ""} />), which publishes up to <strong>{v.publishesUpTo}</strong></>;
  const rd = v.reading;
  return (
    <div className="review-report">
      <h3 className={`run-outcome run-outcome-${outcomeTone(v.outcome)}`}>
        Expected at Submit: {v.outcomeWords}
      </h3>
      <p className="selectable"><RevealText text={v.reason} /></p>
      <p className="muted small">
        For <span className="mono selectable"><RevealText text={v.ref} /></span> ({v.files} file{v.files === 1 ? "" : "s"}), under {policy},
        from {v.aiCounted ? "the registry's rules and AI Review" : "the registry's rules alone"}; assuming <RevealText text={v.assumes} />.
        The Examiner&apos;s own reading is not counted.
      </p>
      {v.runError && (
        <Notice tone="warn">
          The Examiner&apos;s run ended in an error, so part of the report is missing: <RevealText text={v.runError} />
        </Notice>
      )}

      <Card className="mt">
        <h3 style={{ marginTop: 0 }}>The registry&apos;s rules, run here</h3>
        <p className="muted small">
          The manifest rules, the inventory checks and the hidden-content and injection rules, under rule bundle{" "}
          <RevealText text={v.rules.bundleVersion} /> (<RevealText text={v.rules.bundleSource} />).
        </p>
        {v.rules.manifest.map((is, i) => (
          <div key={`m${i}`} className="finding">
            <div className="row">
              <span className="badge">{is.error ? "error: refused at Submit" : "warning"}</span>
              <span className="mono"><RevealText text={is.rule + (is.path ? ` (${is.path})` : "")} /></span>
            </div>
            <div className="selectable"><RevealText text={is.message} /></div>
          </div>
        ))}
        {v.rules.findings.map((f, i) => <Finding key={`f${i}`} f={f} />)}
        {v.rules.manifest.length === 0 && v.rules.findings.length === 0 && <p className="muted small">These rules found nothing.</p>}
        {v.rules.servers.length > 0 && (
          <>
            <h4 className="small">The registry&apos;s record of each declared MCP server</h4>
            <ul className="small">
              {v.rules.servers.map((sv, i) => (
                <li key={i}>
                  <span className="mono selectable"><RevealText text={sv.ref} /></span>: <RevealText text={sv.status} />
                  {sv.error && <span className="muted"> (<RevealText text={sv.error} />)</span>}
                </li>
              ))}
            </ul>
          </>
        )}
      </Card>

      <Card className="mt">
        <h3 style={{ marginTop: 0 }}>AI Review, asked as the pipeline asks it</h3>
        {v.aiReview.ran ? (
          <>
            <p className="muted small">
              TypeSafe&apos;s Jev (<RevealText text={v.aiReview.model ?? ""} />) answered the pipeline&apos;s six questions over{" "}
              {v.aiReview.files} file{v.aiReview.files === 1 ? "" : "s"}
              {v.aiReview.notRead > 0 && <>, leaving {v.aiReview.notRead} out past one review&apos;s budget</>}, with your key.
            </p>
            {v.aiReview.findings.map((f, i) => <Finding key={i} f={f} />)}
            {v.aiReview.findings.length === 0 && <p className="muted small">No question scored a finding.</p>}
          </>
        ) : (
          <p className="small">It did not answer here: <RevealText text={v.aiReview.failure ?? ""} /></p>
        )}
      </Card>

      <Card className="mt">
        <h3 style={{ marginTop: 0 }}>The Examiner&apos;s own reading</h3>
        <p className="muted small">
          What the model your runner policy names (<RevealText text={v.examinerModel || "none recorded"} />) wrote after reading the
          files. It is not counted in the expected outcome{rd.recovered && <>, and was recovered from a last message the runner refused at its output schema</>}.
        </p>
        {rd.error ? (
          <p className="small">There is no reading: <RevealText text={rd.error} /></p>
        ) : (
          <>
            {rd.summary && <p className="selectable"><RevealText text={rd.summary} /></p>}
            {rd.findings.map((f, i) => <Finding key={i} f={f} kind={READ_KIND[f.scanner] ?? "other"} />)}
          </>
        )}
        {rd.withheld.length > 0 && (
          <>
            <h4 className="small">Withheld from its model by AI Review</h4>
            <ul className="small">
              {rd.withheld.map((n, i) => (
                <li key={i}><span className="mono selectable"><RevealText text={n.file} /></span>: <RevealText text={n.why} /></li>
              ))}
            </ul>
          </>
        )}
        {rd.notRead.length > 0 && (
          <>
            <h4 className="small">Not read, as the Examiner says</h4>
            <ul className="small">
              {rd.notRead.map((n, i) => (
                <li key={i}><span className="mono selectable"><RevealText text={n.file} /></span>: <RevealText text={n.why} /></li>
              ))}
            </ul>
          </>
        )}
      </Card>

      <Card className="mt">
        <h3 style={{ marginTop: 0 }}>What did not run here</h3>
        <ul className="small">
          {v.notRunHere.map((n, i) => <li key={i}><RevealText text={n} /></li>)}
        </ul>
      </Card>

      <p className="small"><strong>{v.notice}</strong> Submitting is never held by this review.</p>
      <p className="muted small">
        The report (report.json) and the run&apos;s receipts are in <span className="mono selectable"><RevealText text={v.outDir} /></span>.{" "}
        <Button variant="link" onClick={showFolder}>Show the folder</Button>
      </p>
      <ErrorBox error={err} />
      {log.length > 0 && <FoldedLog label="Review log" lines={log} reveal />}
    </div>
  );
}
