// What the Scheduled page and the run setup's When step say and decide
// (plan quickagent-scheduling.md §4, §7, D2, D11, D12, D19; D-073;
// agentry-desktop#743). Pure and self-contained, so the node tests can
// import it: Go reads the CLI's store and runs the agentry command
// (ScheduleAPI), this says what it read and builds what the page sends.

/** The When step's choice. */
export type WhenChoice = "now" | "later" | "repeat";

/** A repeat's shape (D2). */
export type RepeatKind = "daily" | "weekdays" | "weekly" | "monthly";

/** The When step's answers, as the form holds them. */
export type WhenForm = {
  choice: WhenChoice;
  /** Later: the day (YYYY-MM-DD) and time (HH:MM). */
  date: string;
  time: string;
  repeat: RepeatKind;
  /** Every week: the days (mon … sun). */
  days: string[];
  /** Every month: the day, 1 to 31. */
  monthDay: number;
  end: "never" | "on" | "after";
  until: string;
  times: number;
  /** An IANA zone. */
  zone: string;
  /** Also run when I'm signed out (D19). */
  signedOut: boolean;
};

/** What Go takes (schedules.When). */
export type WhenSpec = {
  kind: string;
  date?: string;
  time: string;
  days?: string[];
  monthDay?: number;
  end?: string;
  until?: string;
  times?: number;
  zone?: string;
};

export const DAYS: { id: string; label: string }[] = [
  { id: "mon", label: "Mon" }, { id: "tue", label: "Tue" }, { id: "wed", label: "Wed" }, { id: "thu", label: "Thu" },
  { id: "fri", label: "Fri" }, { id: "sat", label: "Sat" }, { id: "sun", label: "Sun" },
];

const pad = (n: number) => String(n).padStart(2, "0");

/** A local date as YYYY-MM-DD. */
export const ymd = (d: Date): string => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

/** The next whole hour after now, local: Schedule a new run's default
 *  time (D12). */
export function nextWholeHour(now: Date): Date {
  const d = new Date(now.getTime());
  d.setMinutes(0, 0, 0);
  d.setHours(d.getHours() + 1);
  return d;
}

const dayIDs = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];

/** A When form's starting answers: Now, or (Schedule a new run) Repeat,
 *  every day, at the next whole hour, in this computer's zone. */
export function defaultWhen(now: Date, zone: string, choice: WhenChoice = "now"): WhenForm {
  const at = nextWholeHour(now);
  return {
    choice, date: ymd(at), time: `${pad(at.getHours())}:00`, repeat: "daily", days: [dayIDs[at.getDay()]],
    monthDay: at.getDate(), end: "never", until: "", times: 5, zone, signedOut: false,
  };
}

const HHMM = /^([01]\d|2[0-3]):[0-5]\d$/;
const YMD = /^\d{4}-\d{2}-\d{2}$/;

/** What is wrong with the When answers, as a sentence; null when nothing. */
export function whenProblem(w: WhenForm): string | null {
  if (w.choice === "now") return null;
  if (!HHMM.test(w.time)) return "Choose a time.";
  if (!w.zone) return "Choose a time zone.";
  if (w.choice === "later") return YMD.test(w.date) ? null : "Choose a day.";
  if (w.repeat === "weekly" && w.days.length === 0) return "Choose at least one day of the week.";
  if (w.repeat === "monthly" && !(w.monthDay >= 1 && w.monthDay <= 31)) return "Choose a day of the month, 1 to 31.";
  if (w.end === "on" && !YMD.test(w.until)) return "Choose the last day it runs.";
  if (w.end === "after" && !(w.times >= 1)) return "Choose how many runs, one or more.";
  return null;
}

/** The answers as Go takes them; null for Now. */
export function whenSpec(w: WhenForm): WhenSpec | null {
  if (w.choice === "now") return null;
  if (w.choice === "later") return { kind: "once", date: w.date, time: w.time, zone: w.zone };
  const spec: WhenSpec = { kind: w.repeat, time: w.time, zone: w.zone, end: w.end };
  if (w.repeat === "weekly") spec.days = DAYS.map((d) => d.id).filter((d) => w.days.includes(d));
  if (w.repeat === "monthly") spec.monthDay = w.monthDay;
  if (w.end === "on") spec.until = w.until;
  if (w.end === "after") spec.times = w.times;
  return spec;
}

/** Whether two repeats are the same: Edit sends When only when it
 *  changed, because the agentry command starts a changed repeat afresh
 *  (its runs counted from naught again). */
export function sameWhen(a: WhenSpec | null, b: WhenSpec): boolean {
  if (!a) return false;
  const norm = (w: WhenSpec) => JSON.stringify({
    kind: w.kind, date: w.kind === "once" ? w.date ?? "" : "", time: w.time, zone: w.zone ?? "",
    days: w.kind === "weekly" ? [...(w.days ?? [])].sort() : [], monthDay: w.kind === "monthly" ? w.monthDay ?? 0 : 0,
    end: w.kind === "once" ? "" : w.end || "never", until: w.end === "on" ? w.until ?? "" : "", times: w.end === "after" ? w.times ?? 0 : 0,
  });
  return norm(a) === norm(b);
}

/** A schedule's stored repeat as the form's answers, for Edit. */
export function whenForm(spec: WhenSpec, signedOut: boolean, now: Date, zone: string): WhenForm {
  const base = defaultWhen(now, zone, spec.kind === "once" ? "later" : "repeat");
  return {
    ...base,
    date: spec.date || base.date,
    time: spec.time,
    repeat: spec.kind === "once" ? "daily" : (spec.kind as RepeatKind),
    days: spec.days?.length ? spec.days : base.days,
    monthDay: spec.monthDay || base.monthDay,
    end: spec.end === "on" || spec.end === "after" ? spec.end : "never",
    until: spec.until || "",
    times: spec.times || base.times,
    zone: spec.zone || zone,
    signedOut,
  };
}

/** Why When is disabled for this run, or null when it is offered: a
 *  schedule runs autonomous on this computer, from a registry release, with
 *  the agentry command installed (D12, §5). */
export function whenUnavailable(s: {
  modes: string[];
  mode: string;
  inAgentry: boolean;
  local: boolean;
  availability: { available: boolean; reason?: string } | null;
}): string | null {
  if (s.modes.length > 0 && !s.modes.includes("autonomous")) return "Needs you there to answer: this release runs interactive only, and a schedule runs with nobody there.";
  if (s.inAgentry) return "A schedule runs on this computer: choose to run it here on Before you start. Schedules In Agentry come later.";
  if (s.mode === "interactive") return "A schedule runs autonomous, with nobody there to answer: choose autonomous on Before you start.";
  if (s.local) return "A QuickAgent installed from a folder is scheduled with the agentry command: agentry schedule add --local.";
  if (!s.availability) return "Checking whether this computer can schedule a run.";
  if (!s.availability.available) return s.availability.reason || "This computer cannot schedule a run.";
  return null;
}

/** Why the signed-out box is disabled, or null when it can be ticked
 *  (D17, D19). */
export function signedOutUnavailable(a: { signedOut: { available: boolean; reason?: string } } | null): string | null {
  if (!a) return "Checking whether this computer can.";
  if (a.signedOut.available) return null;
  const why = a.signedOut.reason || "";
  if (/Docker Desktop stops when you sign out/.test(why) && !/Docker Engine/.test(why)) return "Docker Desktop stops when you sign out.";
  if (/needs Docker Engine/.test(why)) return "Needs Docker Engine: Docker Desktop stops when you sign out.";
  return why || "This computer cannot run a schedule while you are signed out.";
}

/** One fire or occurrence on the calendar, as Go reads it. */
export type FireRow = {
  scheduleId: string;
  name: string;
  ref: string;
  at: string;
  outcome: string;
  manual?: boolean;
  runId?: string;
  exitCode?: number;
  exitClass?: string;
  message?: string;
};

/** What became of an occurrence, in a few words (plan invariant 8):
 *  never "done" when nothing ran. */
export function outcomeLabel(f: Pick<FireRow, "outcome" | "exitCode" | "exitClass" | "manual">): string {
  switch (f.outcome) {
    case "due": return "Due";
    case "ran": return f.manual ? "Ran (Run now)" : "Ran";
    case "missed": return "Missed: the computer was off, asleep or signed out";
    case "failed": {
      const how = [f.exitClass, f.exitCode ? `exit ${f.exitCode}` : ""].filter(Boolean).join(", ");
      return how ? `Failed (${how})` : "Failed";
    }
  }
  return f.outcome;
}

/** The tone an outcome is drawn in. */
export function outcomeTone(outcome: string): "ok" | "bad" | "warn" | "due" {
  switch (outcome) {
    case "ran": return "ok";
    case "failed": return "bad";
    case "missed": return "warn";
  }
  return "due";
}

/** One day of the month grid. */
export type GridDay = { date: string; day: number; inMonth: boolean; today: boolean };

/** The month's grid (D11), Monday first: the weeks that hold its days,
 *  with the days of the months before and after that fill them. */
export function monthGrid(year: number, month: number, today: Date): GridDay[][] {
  const first = new Date(year, month, 1);
  const lead = (first.getDay() + 6) % 7; // Monday = 0
  const start = new Date(year, month, 1 - lead);
  const weeks: GridDay[][] = [];
  const todayKey = ymd(today);
  for (let w = 0; w < 6; w++) {
    const week: GridDay[] = [];
    for (let d = 0; d < 7; d++) {
      const day = new Date(start.getFullYear(), start.getMonth(), start.getDate() + w * 7 + d);
      week.push({ date: ymd(day), day: day.getDate(), inMonth: day.getMonth() === month, today: ymd(day) === todayKey });
    }
    if (w >= 4 && !week.some((g) => g.inMonth)) break;
    weeks.push(week);
  }
  return weeks;
}

/** The instants a month grid covers, [from, to) in UTC, for Calendar. */
export function gridRange(weeks: GridDay[][]): { from: string; to: string } {
  const first = weeks[0][0].date.split("-").map(Number);
  const lastDay = weeks[weeks.length - 1][6].date.split("-").map(Number);
  const from = new Date(first[0], first[1] - 1, first[2]);
  const to = new Date(lastDay[0], lastDay[1] - 1, lastDay[2] + 1);
  return { from: from.toISOString().replace(/\.\d{3}Z$/, "Z"), to: to.toISOString().replace(/\.\d{3}Z$/, "Z") };
}

/** The fires and occurrences by local day (YYYY-MM-DD). */
export function byDay(rows: FireRow[]): Map<string, FireRow[]> {
  const m = new Map<string, FireRow[]>();
  for (const r of rows) {
    const t = new Date(r.at);
    if (Number.isNaN(t.getTime())) continue;
    const k = ymd(t);
    const list = m.get(k) ?? [];
    list.push(r);
    m.set(k, list);
  }
  return m;
}

/** A time of day as the page shows it, in this computer's zone. */
export function clockOf(iso: string): string {
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return "";
  return `${pad(t.getHours())}:${pad(t.getMinutes())}`;
}

/** A date and time as the lists show them. */
export function whenOf(iso: string): string {
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return "";
  return `${t.toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" })}, ${clockOf(iso)}`;
}

/** A schedule's state, in a word, for its row. */
export function scheduleState(s: { paused?: boolean; ended?: boolean; signedOut?: boolean; keysSealed?: boolean }): string {
  if (s.ended) return "Ended";
  if (s.paused) return "Paused";
  if (s.signedOut && !s.keysSealed) return "Keys not sealed: it does not run while you are signed out";
  if (s.signedOut) return "Runs while you are signed out too";
  return "Runs while you are signed in";
}

/** The month title: "October 2026". */
export const monthTitle = (year: number, month: number): string =>
  new Date(year, month, 1).toLocaleDateString(undefined, { month: "long", year: "numeric" });

/** The time zones the When step offers: this computer's first, then
 *  the runtime's list. */
export function zoneChoices(here: string, supported: string[]): string[] {
  const rest = supported.filter((z) => z !== here);
  return here ? [here, ...rest] : rest;
}
