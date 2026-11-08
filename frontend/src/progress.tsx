// The parts a job's run window (pages/RunWindow.tsx) and a chat are drawn
// from: the time a job has used against its limit as a bar, the facts of how
// far it has got, its log, its Stop, and the corner card a running flow
// shrinks to so the rest of the app stays usable.
//
// The cards stack: App provides the corner they go in (CornerHost), and a
// card rendered with no corner, as the design harness renders one, places
// itself in the bottom-right corner on its own.
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { JobProgress, Tone, fmtClock } from "./jobProgress";
import { progressTokens } from "./usage";
import { Button } from "./primitives";
import { RevealText } from "./reveal";
import { bytesShare, bytesWords, type BytesProgress } from "./bytesProgress";

/** The corner the running flows' cards stack in; App provides it. */
export const CornerHost = createContext<HTMLElement | null>(null);

/** The time now, ticking once a second while it is in use. */
function useNow(): number {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  return now;
}

/** The time a run has used against its limit, as a bar. With no limit the
 *  bar stays empty. */
function TimeBar({ elapsed, timeoutSeconds, label }: { elapsed: number; timeoutSeconds: number; label: string }) {
  const share = timeoutSeconds > 0 ? Math.min(1, elapsed / timeoutSeconds) : 0;
  return (
    <div
      className="knowledge-progress-bar"
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={timeoutSeconds}
      aria-valuenow={Math.floor(elapsed)}
    >
      <span style={{ width: `${share * 100}%` }} />
    </div>
  );
}

/** A running flow shrunk to a card: its title, with the pulsing dot while it
 *  runs, what it shows of its progress, a line in the outcome's tone, and
 *  its actions. */
export function CornerCard({ label, title, running, line, tone, children, actions }: {
  /** The card's accessible name. */
  label: string;
  title: React.ReactNode;
  running: boolean;
  line?: string;
  tone?: Tone;
  children?: React.ReactNode;
  actions: React.ReactNode;
}) {
  const host = useContext(CornerHost);
  const card = (
    <aside className="build-card" role="status" aria-live="polite" aria-label={label}>
      <div className="build-card-head">
        {running && <span className="build-card-dot" aria-hidden />}
        <strong className="grow">{title}</strong>
      </div>
      {children}
      {line && <p className={`small build-card-line${tone ? ` run-outcome-${tone}` : ""}`}>{line}</p>}
      <div className="row build-card-actions">{actions}</div>
    </aside>
  );
  return host ? createPortal(card, host) : card;
}

/** How far a job has got: the time it has run against its limit, as a bar
 *  and a clock, the turns against the turn budget, the job's own facts and,
 *  for a job on this computer, the tokens it has used (a job In Agentry
 *  reports none). Compact, as the card shows it: the
 *  bar, and the clock beside the one fact the job names. Before the first
 *  report it says the job is starting. `job` is its name in the sentences ("draft",
 *  "build"). */
export function ProgressView<P extends JobProgress>({ progress, job, compact, fact, facts, turns = true }: {
  progress: P | null;
  job: string;
  compact?: boolean;
  /** Whether the full view has a Turns row: a job whose turns are not
   *  reported while it runs (a run In Agentry) has none. */
  turns?: boolean;
  /** The compact view's fact, after the clock. */
  fact: (p: P) => React.ReactNode;
  /** The full view's own rows, after the turns. */
  facts?: (p: P) => React.ReactNode;
}) {
  const now = useNow();
  if (!progress) return <p className="muted small">Starting the {job}…</p>;
  const elapsed = Math.max(0, (now - progress.startedAt) / 1000);
  const bar = <TimeBar elapsed={elapsed} timeoutSeconds={progress.timeoutSeconds} label={`Time used of the ${job}'s limit`} />;
  const time = <>{fmtClock(elapsed)}{progress.timeoutSeconds > 0 && <> of up to {fmtClock(progress.timeoutSeconds)}</>}</>;
  if (compact) {
    return (
      <div className="knowledge-progress">
        {bar}
        <p className="small muted">{time} · {fact(progress)}</p>
      </div>
    );
  }
  return (
    <div className="knowledge-progress">
      {bar}
      <dl className="knowledge-facts">
        <dt>Time</dt>
        <dd>{time}</dd>
        {turns && (
          <>
            <dt>Turns</dt>
            <dd>
              {progress.turns}
              {progress.maxTurns > 0 && <> of up to {progress.maxTurns}</>}
            </dd>
          </>
        )}
        {facts?.(progress)}
        {progressTokens(progress) && (
          <>
            <dt>Tokens</dt>
            <dd>{progressTokens(progress)}</dd>
          </>
        )}
      </dl>
    </div>
  );
}

/** How far a download or an image pull has got, as a bar and the bytes:
 *  "Downloaded: 12 of 40 MB". With no total known the bar sweeps and the
 *  line says the bytes so far. */
export function BytesBar({ progress, label }: { progress: BytesProgress; label: string }) {
  const known = progress.total > 0;
  return (
    <div className="knowledge-progress" data-bytes-bar>
      <div
        className={`knowledge-progress-bar${known ? "" : " indeterminate"}`}
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={known ? progress.total : undefined}
        aria-valuenow={known ? Math.min(progress.done, progress.total) : undefined}
      >
        <span style={known ? { width: `${bytesShare(progress) * 100}%` } : undefined} />
      </div>
      <p className="small muted">{label}: {bytesWords(progress)}</p>
    </div>
  );
}

/** How far a job of counted steps has got: "Image build: step 3 of 6". */
export function StepBar({ step, steps, label }: { step: number; steps: number; label: string }) {
  return (
    <div className="knowledge-progress" data-step-bar>
      <div className="knowledge-progress-bar" role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={steps} aria-valuenow={step}>
        <span style={{ width: `${steps > 0 ? (step / steps) * 100 : 0}%` }} />
      </div>
      <p className="small muted">{label}: step {step} of {steps}</p>
    </div>
  );
}

/** A job's log as it streams, kept scrolled to its last line. `reveal`
 *  shows the invisible characters in it, for a log that carries what pages
 *  and a registry said. */
export function LogPane({ lines, label, reveal }: { lines: string[]; label: string; reveal?: boolean }) {
  const ref = useRef<HTMLPreElement | null>(null);
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [lines.length]);
  const text = lines.join("\n");
  return <pre ref={ref} className="run-log selectable" aria-label={label}>{reveal ? <RevealText text={text} /> : text}</pre>;
}

/** The log folded beneath a running job's progress. */
export function FoldedLog({ label, ...pane }: { lines: string[]; label: string; reveal?: boolean }) {
  return (
    <details className="knowledge-log">
      <summary>{label}</summary>
      <LogPane label={label} {...pane} />
    </details>
  );
}

/** Stop, and "Stopping…" once it is asked for. */
export function StopButton({ stopping, onStop, label = "Stop", stoppingLabel = "Stopping…" }: {
  stopping: boolean;
  onStop: () => void;
  label?: string;
  stoppingLabel?: string;
}) {
  return <Button onClick={onStop} disabled={stopping}>{stopping ? stoppingLabel : label}</Button>;
}
