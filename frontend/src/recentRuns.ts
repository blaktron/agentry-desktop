// What Home's Recent runs card and the run page say of a run (plan
// desktop-home-and-library.md §4.3, §4.6, §4.7, D8, D13; D-068;
// agentry-desktop#659). Pure and self-contained, so the node tests can
// import it: Go gives the rows (RunAPI.RecentRuns), this says them.

/** A built-in's own place, which its run page links to (D13). */
export type BuiltinPage = "build" | "knowledge" | "voice";

export type RunRow = {
  id: string;
  kind: string;
  where: string;
  ref?: string;
  startedAt?: string;
  endedAt?: string;
  outcome: string;
  exitCode?: number;
  state?: string;
  tone: string;
  /** A schedule on this computer fired it (agentry-desktop#743). */
  scheduled?: boolean;
  schedule?: string;
};

const BUILTIN: Record<string, string> = {
  draft: "A draft by the Drafter",
  review: "A review by the Examiner",
  knowledge: "A knowledge build",
  voice: "A Writing Voice build",
};

/** ns/name of a ref, without its version; "" for none. */
export function nameOf(ref?: string): string {
  if (!ref) return "";
  const at = ref.indexOf("@");
  return at < 0 ? ref : ref.slice(0, at);
}

/** The version of a ref, "" for none. */
export function versionOf(ref?: string): string {
  if (!ref) return "";
  const at = ref.indexOf("@");
  return at < 0 ? "" : ref.slice(at + 1);
}

/** What ran: the QuickAgent, a chat with it, or the built-in's job. */
export function whatRan(row: Pick<RunRow, "kind" | "ref">): string {
  if (BUILTIN[row.kind]) return BUILTIN[row.kind];
  const name = nameOf(row.ref) || "A QuickAgent";
  return row.kind === "chat" ? `A chat with ${name}` : name;
}

export const isBuiltin = (kind: string): boolean => kind in BUILTIN;

/** Where it ran, in the glossary's words; a run a schedule fired is
 *  marked Scheduled (plan quickagent-scheduling.md D5). */
export const whereRan = (row: Pick<RunRow, "where" | "scheduled">): string =>
  row.where === "agentry" ? "In Agentry" : row.scheduled ? "Scheduled, on this computer" : "On this computer";

/** How it ended, in a few words; a run In Agentry in the site's words. */
export function howItEnded(row: Pick<RunRow, "where" | "outcome" | "exitCode" | "state">): string {
  if (row.where === "agentry") return row.state || row.outcome;
  switch (row.outcome) {
    case "ok": return "Finished";
    case "stopped": return "Stopped";
    case "blocked": return "Blocked";
    default: return row.exitCode ? `Ended in an error (exit ${row.exitCode})` : "Did not finish";
  }
}

/** The run page's headline: how the run ended, in a sentence. A run In
 *  Agentry's own line is the registry's (HostedRunView.outcomeLine). */
export function headline(row: Pick<RunRow, "kind" | "outcome" | "exitCode">): string {
  const subject = row.kind === "run" || row.kind === "chat" ? "It" : "The job";
  switch (row.outcome) {
    case "ok": return "Finished.";
    case "stopped": return `${subject} was stopped.`;
    case "blocked": return `${subject} was blocked: the QuickAgent could not do the task with what it had.`;
    default: return row.exitCode ? `${subject} ended in an error (exit ${row.exitCode}).` : `${subject} did not finish.`;
  }
}

/** When it ended (or started, while it runs), relative to now. */
export function whenRan(row: Pick<RunRow, "startedAt" | "endedAt">, now: Date): string {
  const iso = row.endedAt || row.startedAt;
  if (!iso) return "";
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return "";
  const mins = Math.round((now.getTime() - t.getTime()) / 60_000);
  const lead = row.endedAt ? "" : "started ";
  if (mins < 1) return `${lead}just now`;
  if (mins < 60) return `${lead}${mins} minute${mins === 1 ? "" : "s"} ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${lead}${hours} hour${hours === 1 ? "" : "s"} ago`;
  const days = Math.round(hours / 24);
  if (days === 1) return `${lead}yesterday`;
  if (days < 7) return `${lead}${days} days ago`;
  return `${lead}${t.toLocaleDateString(undefined, { day: "numeric", month: "short" })}`;
}

/** A row's line under its name: where, when, how it ended. */
export function rowLine(row: RunRow, now: Date): string {
  return [whereRan(row), whenRan(row, now), howItEnded(row)].filter(Boolean).join(" · ");
}

/** A built-in's own place, which its run page links to (D13); none for a
 *  review, whose report is on the run page itself, and for a QuickAgent. */
export function builtinPlace(kind: string): { label: string; page: BuiltinPage } | null {
  switch (kind) {
    case "draft": return { label: "Open in Build", page: "build" };
    case "knowledge": return { label: "Open in Knowledge", page: "knowledge" };
    case "voice": return { label: "Open in Voice", page: "voice" };
  }
  return null;
}

/** The card's line when it has no row, or a note under its rows. */
export function emptyLine(view: { remember: boolean; signedIn: boolean }): string {
  if (!view.remember) {
    return view.signedIn
      ? "No run In Agentry yet, and runs on this computer are not remembered: Remember recent runs is off in Run defaults."
      : "Runs on this computer are not remembered: Remember recent runs is off in Run defaults.";
  }
  return "No runs yet. A QuickAgent you run, and a draft, review or build you make, will show here.";
}

/** Signed out: what signing in would add (plan §4.3). */
export const signedOutNote = "Sign in to see your runs In Agentry here too.";
