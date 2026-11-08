// A job's log as every run window keeps it: the newest MAX_RUN_LOG_LINES
// lines, counting the ones let go, so a QuickAgent's run and each built-in's
// job (a draft, a review, a knowledge build, a Voice Writer or Report
// Designer run) hold the same amount and show the same note when they have
// let lines go. Pure, so each store and its tests share it.

/** How many lines of a job's log the app keeps: a job whose agent calls
 *  tools for an hour writes more than a page should hold. */
export const MAX_RUN_LOG_LINES = 5000;

/** The log as kept: the newest lines, and how many earlier ones went. */
export type RunLog = { lines: string[]; dropped: number };

export const EMPTY_LOG: RunLog = { lines: [], dropped: 0 };

/** The note that stands first in a log that has let earlier lines go. */
export function droppedNote(n: number): string {
  return `(${n} earlier ${n === 1 ? "line is" : "lines are"} not shown here.)`;
}

/** lines with line added; past MAX_RUN_LOG_LINES the oldest go, counted in
 *  dropped. */
export function addLine(lines: string[], dropped: number, line: string): RunLog {
  const next = [...lines, line];
  if (next.length <= MAX_RUN_LOG_LINES) return { lines: next, dropped };
  const over = next.length - MAX_RUN_LOG_LINES;
  return { lines: next.slice(over), dropped: dropped + over };
}

/** The log with one more line. */
export function withLogLine(log: RunLog, line: string): RunLog {
  return addLine(log.lines, log.dropped, line);
}

/** The log as a run window shows it: the note of the lines let go, then
 *  the lines kept. */
export function logLines(lines: string[], dropped = 0): string[] {
  return dropped ? [droppedNote(dropped), ...lines] : lines;
}
