import { CardFrame } from "../../cards";
import { getStartedStatus, type GetStartedAction, type GetStartedRow } from "../../getStarted";

// Home's Get started card: a checklist of what the setup for the place runs
// go still needs, then the first things to try, each open row with the
// button that does it. Its rows come from reads Home already makes; a press
// opens a page and starts nothing.
export function GetStartedCard({ rows, onAction, onOpen }: {
  rows: GetStartedRow[];
  onAction: (a: GetStartedAction) => void;
  onOpen: () => void;
}) {
  return (
    <CardFrame title="Get started" onOpen={onOpen} className="home-card" status={getStartedStatus(rows)}>
      {rows.length > 0 && (
        <ul className="home-checklist" data-get-started>
          {rows.map((r) => (
            <li key={r.id} className={r.done ? "is-done" : undefined} data-row={r.id} data-done={r.done ? "1" : "0"}>
              <span className="home-check" aria-hidden="true">{r.done ? "✓" : "○"}</span>
              <span className="home-check-text">
                <span className="sr-only">{r.done ? "Done: " : "To do: "}</span>
                {r.text}
              </span>
              {!r.done && r.button && (
                <button type="button" className="home-check-button build-way-part" onClick={() => onAction(r.action)}>{r.button}</button>
              )}
            </li>
          ))}
        </ul>
      )}
    </CardFrame>
  );
}
