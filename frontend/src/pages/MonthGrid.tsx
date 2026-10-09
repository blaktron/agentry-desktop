// The month grid (plan quickagent-scheduling.md D11; D-073): the Scheduled
// page's calendar and, mini, Home's Scheduled card (D13;
// agentry-desktop#744). Hand-built, Monday first, each day's occurrences
// as dots in the colour of what happened (hollow while due). On the page a
// day is a button that chooses it; on the card the whole card is the
// button, so the days are plain cells.
import { outcomeLabel, outcomeTone, type FireRow, type GridDay } from "../scheduled";

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const MINI_WEEKDAYS = ["M", "T", "W", "T", "F", "S", "S"];

export function MonthGrid({ weeks, days, chosen, onChoose, mini }: {
  weeks: GridDay[][];
  /** The fires and occurrences by day (byDay). */
  days: Map<string, FireRow[]>;
  chosen?: string;
  /** Absent on the mini grid, whose days are not buttons. */
  onChoose?: (date: string) => void;
  mini?: boolean;
}) {
  const dots = mini ? 3 : 4;
  return (
    <table className={`cal${mini ? " cal-mini" : ""}`} role="grid" data-month-grid={mini ? "mini" : "full"}>
      <thead>
        <tr>{(mini ? MINI_WEEKDAYS : WEEKDAYS).map((d, i) => <th key={i} scope="col" abbr={WEEKDAYS[i]}>{d}</th>)}</tr>
      </thead>
      <tbody>
        {weeks.map((w, i) => (
          <tr key={i}>
            {w.map((g) => {
              const rows = days.get(g.date) ?? [];
              const label = `${g.date}: ${rows.length === 0 ? "nothing scheduled" : rows.map((r) => `${r.name} ${outcomeLabel(r)}`).join(", ")}`;
              const inner = (
                <>
                  <span className="cal-num">{g.day}</span>
                  <span className="cal-dots" aria-hidden>
                    {rows.slice(0, dots).map((r, k) => <span key={k} className={`cal-dot tone-${outcomeTone(r.outcome)}`} />)}
                    {rows.length > dots && <span className="cal-more">+{rows.length - dots}</span>}
                  </span>
                </>
              );
              return (
                <td key={g.date} className={`${g.inMonth ? "" : "cal-out"}${g.today ? " cal-today" : ""}${g.date === chosen ? " cal-chosen" : ""}`}
                  data-cal-day={g.date} data-cal-count={rows.length || undefined}>
                  {onChoose
                    ? <button type="button" className="cal-day" onClick={() => onChoose(g.date)} aria-pressed={g.date === chosen} aria-label={label}>{inner}</button>
                    : <div className="cal-day" title={label}>{inner}</div>}
                </td>
              );
            })}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** What the dots mean. */
export function CalendarKey() {
  return (
    <p className="muted small cal-key">
      <span className="cal-dot tone-due" /> due <span className="cal-dot tone-ok" /> ran <span className="cal-dot tone-bad" /> failed{" "}
      <span className="cal-dot tone-warn" /> missed
    </p>
  );
}
