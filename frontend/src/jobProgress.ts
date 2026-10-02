// What every long job of the app's own reports while it runs, and how its
// clock reads (agentry-desktop#464): shared by the progress module that draws
// it and by each flow that declares its own report beside these fields, so a
// draft's, a build's and, later, a run's and a chat's progress are one shape.

/** What every job's progress carries, as Go reports it (internal/progress's
 *  Report); a job's own report adds its facts beside these. */
export type JobProgress = {
  startedAt: number;
  timeoutSeconds: number;
  maxTurns: number;
  turns: number;
  /** The tokens a job on this computer has used so far. A job In Agentry
   *  carries none: it is stated in credits (agentry-desktop#457). */
  inputTokens?: number;
  outputTokens?: number;
};

/** The tone a card's line and an outcome's heading are drawn in. */
export type Tone = "" | "ok" | "warn" | "bad";

/** Seconds as m:ss, or h:mm:ss past an hour. */
export function fmtClock(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}
