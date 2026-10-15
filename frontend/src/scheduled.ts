// What the Scheduled page and the run setup's When step say and decide. Pure
// and self-contained, so the node tests can import it: Go reads the CLI's store
// and runs the agentry command (ScheduleAPI), this says what it read and builds
// what the page sends.

/** The When step's choice. */
export type WhenChoice = "now" | "later" | "repeat";

/** A repeat's shape. */
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
  /** Also run when I'm signed out. */
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
 *  time. */
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
 *  schedule runs autonomous, either on this computer, from a registry
 *  release with the agentry command installed, or In Agentry,
 *  where the registry schedules runs and the account's plan does. */
export function whenUnavailable(s: {
  modes: string[];
  mode: string;
  inAgentry: boolean;
  local: boolean;
  availability: { available: boolean; reason?: string } | null;
  /** Talk to it In Agentry: a hosted session, which needs a person. */
  session?: boolean;
  /** The registry's offer of Later and Repeat In Agentry; null where it
   *  makes none. */
  hostedOffer?: { available: boolean; reason?: string; remedy?: string } | null;
}): string | null {
  if (s.modes.length > 0 && !s.modes.includes("autonomous")) return "Needs you there to answer: this release runs interactive only, and a schedule runs with nobody there.";
  if (s.inAgentry) {
    if (s.session) return "A session needs you there to answer, and a schedule runs autonomous, with nobody there: choose a run In Agentry on Before you start.";
    if (!s.hostedOffer) return "This registry does not schedule runs In Agentry: choose to run it on this computer on Before you start to schedule it there.";
    if (!s.hostedOffer.available) return [s.hostedOffer.reason, s.hostedOffer.remedy].filter(Boolean).join(" ") || "A schedule In Agentry cannot be made for this account.";
    return null;
  }
  if (s.mode === "interactive") return "A schedule runs autonomous, with nobody there to answer: choose autonomous on Before you start.";
  if (s.local) return "A QuickAgent installed from a folder is scheduled with the agentry command: agentry schedule add --local.";
  if (!s.availability) return "Checking whether this computer can schedule a run.";
  if (!s.availability.available) return s.availability.reason || "This computer cannot schedule a run.";
  return null;
}

/** Why the signed-out box is disabled, or null when it can be ticked. */
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
  /** In Agentry: the account's schedule there, and
   *  the registry's reason for a fire that started no run. */
  hosted?: boolean;
  reason?: string;
};

/** Why a fire In Agentry started no run, in words (x-schedules.yaml
 *  ScheduleFire.reason): a code this app does not know is shown as it is,
 *  never guessed. */
export function hostedReasonWords(reason: string | undefined): string {
  switch (reason) {
    case "run-active": return "a run of yours was still going after an hour of tries";
    case "credits": return "there were not enough credits";
    case "paused": return "the schedule was paused";
    case "ended": return "the schedule had ended";
    case "missed": return "it was not tried within the hour";
    case "entitlement": return "your plan no longer schedules runs In Agentry, so the schedule paused";
    case "account": return "the account was not active";
    case "release": return "the version could not be run (gone, unscanned or changed)";
    case "connection": return "a connection it needs is missing";
    case "secret": return "a secret it needs is missing";
    case "refused": return "Agentry refused the run";
    case "unavailable": return "Agentry was not available";
    case "interrupted": return "it was interrupted";
  }
  return reason || "no reason was given";
}

/** What became of an occurrence, in a few words (plan invariant 8):
 *  never "done" when nothing ran. */
export function outcomeLabel(f: Pick<FireRow, "outcome" | "exitCode" | "exitClass" | "manual" | "hosted" | "reason">): string {
  if (f.hosted) {
    // In Agentry a fire that ran started a run, whose own page says how it
    // ended: never "done" (the site's words).
    switch (f.outcome) {
      case "due": return "Due";
      case "ran": return "Run started";
      case "pending": return f.reason ? `Not started yet: ${hostedReasonWords(f.reason)}` : "Starting";
      case "skipped": return `Skipped: ${hostedReasonWords(f.reason)}. Nothing ran.`;
      case "failed": return `Failed: ${hostedReasonWords(f.reason)}. Nothing ran.`;
    }
    return f.outcome;
  }
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
    case "missed":
    case "skipped": return "warn";
  }
  return "due";
}

/** One day of the month grid. */
export type GridDay = { date: string; day: number; inMonth: boolean; today: boolean };

/** The month's grid, Monday first: the weeks that hold its days,
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
export function scheduleState(s: { paused?: boolean; ended?: boolean; signedOut?: boolean; keysSealed?: boolean; hosted?: boolean; pausedReason?: string }): string {
  if (s.ended) return "Ended";
  if (s.hosted && s.paused) {
    switch (s.pausedReason) {
      case "refusals": return "Paused after three refused runs in a row: Resume checks it again";
      case "entitlement": return "Paused: your plan no longer schedules runs In Agentry";
      case "account": return "Paused: the account is not active";
    }
    return "Paused";
  }
  if (s.paused) return "Paused";
  if (s.hosted) return "Runs In Agentry, whether or not this computer is on";
  if (s.signedOut && !s.keysSealed) return "Keys not sealed: it does not run while you are signed out";
  if (s.signedOut) return "Runs while you are signed out too";
  return "Runs while you are signed in";
}

/** The rows of two calendars as one, by time: what is here and what is In
 *  Agentry. */
export function mergeFires<T extends { at: string }>(a: T[], b: T[], newestFirst = false): T[] {
  const out = [...a, ...b];
  out.sort((x, y) => (newestFirst ? (x.at < y.at ? 1 : x.at > y.at ? -1 : 0) : (x.at < y.at ? -1 : x.at > y.at ? 1 : 0)));
  return out;
}

/** What the When step says of a schedule In Agentry the registry refused
 *  to make, as a heading over its own sentence; the gaps it can fix in
 *  place (secrets, connections) have their own asks. */
export function hostedGapHeading(refusal: string): string {
  switch (refusal) {
    case "schedule-tier": return "Scheduling runs In Agentry is not part of this plan.";
    case "schedule-limit": return "This account keeps its most schedules.";
    case "when-passed": return "This repeat has nothing left to run.";
    case "release-changed": return "This version is no longer the one it was.";
    case "connection-missing": return "Agentry keeps no connection to a server this release signs in to.";
    case "secret-missing":
    case "secret-not-kept": return "A schedule's runs use secrets your account keeps.";
  }
  return "Agentry did not make the schedule.";
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

/** Home's Scheduled card's status line:
 *  how many schedules this computer keeps and the account keeps In
 *  Agentry, or what kept them from being read. */
export function scheduledCardStatus(v: { schedules: number; hostedSchedules?: number } | null, error: string | null): string {
  if (error) return "The schedules could not be read.";
  if (!v) return "Reading the schedules…";
  const here = v.schedules, there = v.hostedSchedules ?? 0;
  const n = (k: number) => `${k} schedule${k === 1 ? "" : "s"}`;
  if (here === 0 && there === 0) return "Nothing is scheduled on this computer yet.";
  if (there === 0) return `${n(here)} on this computer.`;
  if (here === 0) return `${n(there)} In Agentry.`;
  return `${n(here)} on this computer, ${there} In Agentry.`;
}

/** A reason from Go as a sentence: its first letter capital, one stop. */
export function asSentence(reason: string): string {
  const t = reason.trim();
  if (!t) return "";
  return t[0].toUpperCase() + t.slice(1) + (/[.!?]$/.test(t) ? "" : ".");
}
