import * as app from "../../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { CardFrame, CouldNotRead } from "../../cards";
import { emptyLine, howItEnded, signedOutNote, whatRan, whenRan, whereRan } from "../../recentRuns";

// Home's Recent runs card: the last five runs, chats and built-in jobs,
// newest first, on this computer (the history Go keeps) and In Agentry (the
// registry's list, read live while signed in). A row opens the run page;
// the card opens Run defaults' Recent runs, where the history is turned off
// or cleared. Nothing a run wrote is shown here.
export function RecentRunsCard({ view, error, onRetry, now, onOpenRun, onOpenSettings }: {
  view: app.RecentRunsView | null;
  error: string | null;
  onRetry?: () => void;
  now: Date;
  onOpenRun: (row: app.RecentRunView) => void;
  onOpenSettings: () => void;
}) {
  const rows = view?.runs ?? [];
  const status = error
    ? <CouldNotRead what="Recent runs" error={error} onRetry={onRetry} />
    : !view
      ? "Reading recent runs…"
      : rows.length === 0
        ? emptyLine(view)
        : null;
  return (
    <CardFrame title="Recent runs" onOpen={onOpenSettings} className="home-card" status={status}
      foot={view && !view.signedIn ? signedOutNote : view?.hostedError ? <CouldNotRead what="Your runs In Agentry" error={view.hostedError} onRetry={onRetry} /> : undefined}>
      {rows.length > 0 && (
        <ul className="home-card-list recent-runs" data-recent-runs>
          {rows.map((r) => (
            <li key={`${r.source}-${r.id}`} data-run-row={r.id} data-run-where={r.where} data-run-kind={r.kind} data-run-scheduled={r.scheduled ? "" : undefined}>
              <button type="button" className="link build-way-part" onClick={() => onOpenRun(r)}>
                <span className={r.kind === "run" || r.kind === "chat" ? "mono" : ""}>{whatRan(r)}</span>
              </button>
              <div className="muted small">
                {[whereRan(r), whenRan(r, now)].filter(Boolean).join(" · ")} ·{" "}
                <span className={`recent-run-end tone-${r.tone || "warn"}`}>{howItEnded(r)}</span>
              </div>
            </li>
          ))}
        </ul>
      )}
      {view?.historyError && <p className="small">The run history cannot be read: {view.historyError}</p>}
    </CardFrame>
  );
}
