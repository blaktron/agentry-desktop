import { app } from "../../../wailsjs/go/models";
import { CardFrame } from "../../cards";
import { asSentence, byDay, mergeFires, scheduledCardStatus, whenOf, type FireRow, type GridDay } from "../../scheduled";
import { MonthGrid } from "../MonthGrid";

// Home's Scheduled card: this month's mini calendar, its dots in the
// colour of what happened, and the next three occurrences, from the CLI's
// store read in process (ScheduleAPI.HomeCard) and, signed in, the
// account's schedules In Agentry, marked so. Large, pinned from the
// Scheduled page's options menu. The card opens the Scheduled page, where
// every change is made; it changes and runs nothing.
export function ScheduledCard({ view, error, weeks, title, onOpen }: {
  view: app.ScheduledCardView | null;
  error: string | null;
  /** This month's grid (monthGrid of now), and its title. */
  weeks: GridDay[][];
  title: string;
  onOpen: () => void;
}) {
  const days = byDay(mergeFires((view?.month ?? []) as FireRow[], (view?.hostedMonth ?? []) as FireRow[]));
  const next = view?.next ?? [];
  const foot = [view?.reason ? asSentence(view.reason) : "", view?.hostedReason ?? ""].filter(Boolean).join(" ");
  return (
    <CardFrame title="Scheduled" onOpen={onOpen} className="home-card home-scheduled" status={scheduledCardStatus(view, error)}
      foot={foot || undefined}>
      {view && (
        <div className="home-scheduled-body">
          <div className="home-scheduled-month">
            <p className="small home-scheduled-title">{title}</p>
            <MonthGrid weeks={weeks} days={days} mini />
          </div>
          <div className="home-scheduled-next">
            <p className="muted small home-scheduled-title">Next</p>
            {next.length === 0 ? (
              <p className="muted small" data-scheduled-next-none>{view.schedules === 0 && !view.hostedSchedules ? "Schedule a run from the Scheduled page." : "Nothing is due."}</p>
            ) : (
              <ul className="home-card-list" data-scheduled-next>
                {next.map((f, k) => (
                  <li key={k}>
                    <span className="small">{whenOf(f.at)}</span>
                    <div className="mono small">{f.name}</div>
                    {f.hosted && <div className="muted small" data-in-agentry>In Agentry</div>}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </CardFrame>
  );
}
