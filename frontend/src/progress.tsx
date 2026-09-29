// What a long run of the app's own shows while it goes (agentry-desktop#352,
// #353, #410): the time it has used against its limit as a bar, and the
// corner card a running flow shrinks to so the rest of the app stays usable.
// Create knowledge and Describe it both use them, so the two long runs look
// the same.
//
// The cards stack: App provides the corner they go in (CornerHost), and a
// card rendered with no corner, as the design harness renders one, places
// itself in the bottom-right corner on its own.
import { createContext, useContext, useEffect, useState } from "react";
import { createPortal } from "react-dom";

/** The corner the running flows' cards stack in; App provides it. */
export const CornerHost = createContext<HTMLElement | null>(null);

/** The time now, ticking once a second while it is in use. */
export function useNow(): number {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  return now;
}

/** The time a run has used against its limit, as a bar. With no limit the
 *  bar stays empty. */
export function TimeBar({ elapsed, timeoutSeconds, label }: { elapsed: number; timeoutSeconds: number; label: string }) {
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
  tone?: "" | "ok" | "warn" | "bad";
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
