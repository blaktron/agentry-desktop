// The Assistant's Tasks view, as logic: a task's cron in words (always shown
// beside the expression as written), its times in the person's own clock,
// and what each task's row says. Pure, so the view and its tests share it.
// Every name, instruction and answer is the agent's or the person's, drawn
// as text.
import type { app } from "../wailsjs/go/models";

const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

const num = (s: string, lo: number, hi: number): number | null => {
  if (!/^\d+$/.test(s)) return null;
  const n = Number(s);
  return n >= lo && n <= hi ? n : null;
};

const clock = (h: number, m: number) => `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;

/** dayWords is a day-of-week field in words: one day, a range (1-5 is
 *  weekdays), or a list; null for what it does not read. Sunday is 0 or 7. */
function dayWords(f: string): string | null {
  if (f === "1-5") return "on weekdays";
  if (f === "0,6" || f === "6,0" || f === "6,7") return "on weekends";
  const day = (s: string) => {
    const n = num(s, 0, 7);
    return n === null ? null : DAYS[n % 7];
  };
  if (/^\d+-\d+$/.test(f)) {
    const [a, b] = f.split("-").map(day);
    return a && b ? `${a} to ${b}` : null;
  }
  const days = f.split(",").map(day);
  if (days.some((d) => d === null)) return null;
  return days.length === 1 ? `every ${days[0]}` : `on ${days.slice(0, -1).join(", ")} and ${days[days.length - 1]}`;
}

/** cronWords reads the common shapes of a five-field cron expression in
 *  words: every n minutes, every hour at a minute, every day or chosen
 *  days at a time, a day of the month at a time. Anything else answers
 *  null, and the view shows the expression alone. */
export function cronWords(expr: string): string | null {
  const f = expr.trim().split(/\s+/);
  if (f.length !== 5) return null;
  const [mi, ho, dom, mon, dow] = f;
  if (mon !== "*") return null;
  const step = /^\*\/(\d+)$/.exec(mi);
  if (step && ho === "*" && dom === "*" && dow === "*") {
    const n = Number(step[1]);
    return n === 1 ? "every minute" : `every ${n} minutes`;
  }
  if (mi === "*" && ho === "*" && dom === "*" && dow === "*") return "every minute";
  const m = num(mi, 0, 59);
  if (m === null) return null;
  if (ho === "*" && dom === "*" && dow === "*") return m === 0 ? "every hour, on the hour" : `every hour at ${m} past`;
  const h = num(ho, 0, 23);
  if (h === null) return null;
  const at = `at ${clock(h, m)}`;
  if (dom === "*" && dow === "*") return `every day ${at}`;
  if (dom === "*") {
    const d = dayWords(dow);
    return d ? `${d} ${at}` : null;
  }
  if (dow === "*") {
    const d = num(dom, 1, 31);
    return d === null ? null : `on day ${d} of every month ${at}`;
  }
  return null;
}

/** when is an RFC 3339 time in the person's own clock, "" for none. */
export function when(stamp: string | undefined, locale?: string): string {
  if (!stamp) return "";
  const t = new Date(stamp);
  if (Number.isNaN(t.getTime())) return "";
  return t.toLocaleString(locale, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

/** A task's row in the view. */
export type TaskRow = {
  id: string;
  name: string;
  schedule: string;
  cron: string;
  zone: string;
  paused: boolean;
  next: string;
  last: string;
  answer: string;
  madeBy: string;
  instruction: string;
};

/** taskRow is what a task's row says. A paused task has no next fire. */
export function taskRow(t: app.PersistentTaskView, locale?: string): TaskRow {
  const paused = t.state === "paused";
  return {
    id: t.id,
    name: t.name,
    schedule: cronWords(t.cron) ?? "on its own schedule",
    cron: t.cron,
    zone: t.zone,
    paused,
    next: paused ? "Paused" : when(t.next, locale) || "Not scheduled",
    last: when(t.lastFire, locale) || "Not yet",
    answer: t.lastResult ?? "",
    madeBy: t.madeBy === "agent" ? "Made by the agent" : t.madeBy,
    instruction: t.instruction,
  };
}

/** The operations the view offers on a task: pause or resume, and remove. */
export function taskOps(t: Pick<app.PersistentTaskView, "state">): { label: string; op: string }[] {
  return [t.state === "paused" ? { label: "Resume", op: "resume" } : { label: "Pause", op: "pause" }, { label: "Remove", op: "remove" }];
}
