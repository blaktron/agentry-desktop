// A run on this computer, as the app follows it above the pages
// (agentry-desktop#465; plan desktop-review-key-and-run-cards.md §5.2): what
// Go has reported of it, keyed by its id, and how it ended. Pure, so the
// store (runs.ts) and its tests share one set of rules: a report or a line of
// another run, or of this run after it ended, changes nothing.
import type { main } from "../wailsjs/go/models";
import type { JobProgress, Tone } from "./jobProgress";

/** How far a run on this computer has got, as Go reports it
 *  (runner:run-progress, RunAPI's RunProgress): the report every job carries,
 *  the run's id and the release. Its budgets are zero until the run is
 *  prepared. */
export type RunProgress = JobProgress & { runId: string; ref: string };

/** One line of a run's log, with its id (runner:run-line). */
export type RunLine = { runId: string; line: string };

export type LocalRun = {
  /** The release run, ns/name@version. */
  ref: string;
  /** Known once Go reports that the run has begun. */
  runId: string | null;
  progress: RunProgress | null;
  log: string[];
  result: main.RunResultView | null;
  /** Why it ended with no result, in Go's words. */
  error: string | null;
  over: boolean;
  /** Stop was asked for; stopped: the run ended after it. */
  stopping: boolean;
  stopped: boolean;
};

/** A run of ref about to start. */
export function starting(ref: string): LocalRun {
  return { ref, runId: null, progress: null, log: [], result: null, error: null, over: false, stopping: false, stopped: false };
}

/** A report of this run: the first names its id, the rest must carry it. */
export function withProgress(r: LocalRun, p: RunProgress): LocalRun {
  if (r.over || p.ref !== r.ref || (r.runId !== null && p.runId !== r.runId)) return r;
  return { ...r, runId: p.runId, progress: p };
}

/** A line of this run's log. */
export function withLine(r: LocalRun, l: RunLine): LocalRun {
  if (r.over || r.runId === null || l.runId !== r.runId) return r;
  return { ...r, log: [...r.log, l.line] };
}

/** The run's result: it is over. */
export function finished(r: LocalRun, result: main.RunResultView): LocalRun {
  if (r.over) return r;
  return { ...r, runId: r.runId ?? result.runId, result, over: true, stopped: r.stopping };
}

/** The run ended with no result. */
export function failed(r: LocalRun, error: string): LocalRun {
  if (r.over) return r;
  return { ...r, error, over: true, stopped: r.stopping };
}

/** Stop was asked for. */
export function stopAsked(r: LocalRun): LocalRun {
  return r.over ? r : { ...r, stopping: true };
}

/** How the run ended, in the card's line and tone; "" while it runs. */
export function outcome(r: LocalRun): { line: string; tone: Tone } {
  if (!r.over) return { line: "", tone: "" };
  if (r.stopped) return { line: "You stopped the run. Its receipts are written as far as it got.", tone: "warn" };
  if (r.result?.status === "ok") return { line: "Finished.", tone: "ok" };
  if (r.result) return { line: `It ended in an error (exit ${r.result.exitCode}).`, tone: "bad" };
  return { line: "The run did not finish.", tone: "bad" };
}
