// What a long run of the app's own shows while it goes (agentry-desktop#352,
// #353, #410): the time it has used against its limit as a bar, the facts of
// how far it has got, its log folded beneath, its Stop, and the corner card
// a running flow shrinks to so the rest of the app stays usable. Create
// knowledge and Describe it both draw through these (agentry-desktop#464,
// plan desktop-review-key-and-run-cards.md §5.1), so the long runs look the
// same, and a run and a chat will too.
//
// The cards stack: App provides the corner they go in (CornerHost), and a
// card rendered with no corner, as the design harness renders one, places
// itself in the bottom-right corner on its own.
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { JobProgress, Tone, fmtClock } from "./jobProgress";
import { progressTokens } from "./usage";
import { Button, Spinner } from "./primitives";
import { RevealText } from "./reveal";

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
 *  reports none, agentry-desktop#457). Compact, as the card shows it: the
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
export function StopButton({ stopping, onStop, label = "Stop" }: { stopping: boolean; onStop: () => void; label?: string }) {
  return <Button onClick={onStop} disabled={stopping}>{stopping ? "Stopping…" : label}</Button>;
}

/** The progress dialog while a job runs: what it is doing, what it is,
 *  how far it has got with its log (children), its actions, and what
 *  running it in the background means. */
export function RunningView({ spinner, about, children, actions, background }: {
  spinner: string;
  about: React.ReactNode;
  children: React.ReactNode;
  actions: React.ReactNode;
  background: React.ReactNode;
}) {
  return (
    <>
      <Spinner label={spinner} />
      <p className="muted small">{about}</p>
      {children}
      <div className="row" style={{ justifyContent: "flex-end" }}>{actions}</div>
      <p className="muted small">{background}</p>
    </>
  );
}
