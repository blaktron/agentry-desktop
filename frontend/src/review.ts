// A review by the Examiner on this computer, as the app follows it above the
// pages: what Go has reported of it and how it ended. Pure, so the store
// (reviews.ts) and its tests share one set of rules: a report or a line that
// comes when no review of this start is under way changes nothing, and a start
// Go refuses, or the person declines in the native dialog, leaves the review
// being followed as it is.
import type { main } from "../wailsjs/go/models";
import type { JobProgress, Tone } from "./jobProgress";

/** How far a review has got, as Go reports it (build:review-progress,
 *  ReviewProgressView): the report every job carries, the stage and the
 *  run's id. */
export type ReviewProgress = JobProgress & { runId: string; stage: "rules" | "examining" | "reporting" | "" };

export type Review = {
  /** The folder reviewed, as the page names it until the report does. */
  dir: string;
  runId: string | null;
  progress: ReviewProgress | null;
  log: string[];
  view: main.ReviewView | null;
  /** Why it ended with no report, in Go's words. */
  error: string | null;
  over: boolean;
  stopping: boolean;
  stopped: boolean;
};

export function starting(dir: string): Review {
  return { dir, runId: null, progress: null, log: [], view: null, error: null, over: false, stopping: false, stopped: false };
}

/** A report of this review: the first names its id, the rest must carry it. */
export function withProgress(r: Review, p: ReviewProgress): Review {
  if (r.over || (r.runId !== null && p.runId !== r.runId)) return r;
  return { ...r, runId: p.runId, progress: p };
}

/** A line of the review's log, once it has begun. */
export function withLine(r: Review, line: string): Review {
  if (r.over || r.runId === null) return r;
  return { ...r, log: [...r.log, line] };
}

export function finished(r: Review, view: main.ReviewView): Review {
  if (r.over) return r;
  return { ...r, runId: r.runId ?? view.runId, view, over: true, stopped: r.stopping };
}

export function failed(r: Review, error: string): Review {
  if (r.over) return r;
  return { ...r, error, over: true, stopped: r.stopping };
}

export function stopAsked(r: Review): Review {
  return r.over ? r : { ...r, stopping: true };
}

/** The tone an expected outcome is drawn in: what the pipeline would be
 *  expected to do, never a verdict on the QuickAgent. */
export function outcomeTone(outcome: string): Exclude<Tone, ""> {
  if (outcome === "published") return "ok";
  if (outcome === "rejected" || outcome === "refused_at_submit") return "bad";
  return "warn";
}

/** How the review ended, in the card's line and tone; "" while it runs. */
export function outcome(r: Review): { line: string; tone: Tone } {
  if (!r.over) return { line: "", tone: "" };
  if (r.view) {
    if (r.stopped) return { line: "You stopped the review. The report says what of it ran.", tone: "warn" };
    return { line: `Expected at Submit: ${r.view.outcomeWords}`, tone: outcomeTone(r.view.outcome) };
  }
  if (r.stopped) return { line: "You stopped the review before the Examiner started.", tone: "warn" };
  return { line: "The review did not finish.", tone: "bad" };
}

/** What the store needs from the app: Go's events, and the calls that stop
 *  and forget a review. Injected, so the store's rules are tested without
 *  a webview. */
export type ReviewStoreDeps = {
  on: (event: string, cb: (v: never) => void) => () => void;
  stop: () => Promise<void>;
  forget: (runId: string) => void;
};

export type ReviewStoreState = { review: Review | null; shown: boolean; raised: number };

/** The one review the app follows. A start owns nothing shared until its own
 *  review has begun (Go's first report, after the native dialog and inside
 *  the run lock): the review followed until then stays as it is, and a
 *  start that is declined or refused changes nothing. */
export function createReviewStore(deps: ReviewStoreDeps) {
  let review: Review | null = null;
  // owner is the start whose review is the one followed: only it writes
  // review, and markStopping is how Stop reaches that start's own record.
  let owner: object | null = null;
  let markStopping: (() => void) | null = null;
  // starting: a start is waiting for its native dialog and for Go to begin
  // it. Go runs one local job at a time and a report does not say which
  // start it answers, so a second start waits for the first.
  let starting_ = false;
  let shown = false;
  let raised = 0;
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((f) => f());

  return {
    state: (): ReviewStoreState => ({ review, shown, raised }),
    subscribe(f: () => void): () => void {
      listeners.add(f);
      return () => {
        listeners.delete(f);
      };
    },
    /** Starts a review of dir by call (BuildAPI.ReviewLocally). Resolves
     *  true once Go reports that it has begun, which is when its dialog
     *  opens, and false when the person said No in the native dialog;
     *  rejects with Go's words if it never began. */
    start(dir: string, call: () => Promise<main.ReviewView | null>): Promise<boolean> {
      if (starting_) return Promise.reject("A review is being started on this computer; wait for it to begin.");
      starting_ = true;
      const token = {};
      // The review followed when this start began is not this start's,
      // whatever its reports say.
      const followed = review && !review.over ? review.runId : null;
      let mine = starting(dir);
      let begun = false;
      const publish = () => {
        if (owner !== token) return;
        review = mine;
        notify();
      };
      return new Promise((resolve, reject) => {
        const begin = () => {
          if (begun) return;
          begun = true;
          starting_ = false;
          if (review?.runId && review.runId !== mine.runId) deps.forget(review.runId);
          owner = token;
          markStopping = () => {
            mine = stopAsked(mine);
            publish();
          };
          shown = true;
          raised++;
          resolve(true);
        };
        const offProgress = deps.on("build:review-progress", (p: ReviewProgress) => {
          if (p.runId === followed) return;
          const next = withProgress(mine, p);
          if (next === mine) return;
          mine = next;
          begin();
          publish();
        });
        const offLine = deps.on("build:review-log", (line: string) => {
          const next = withLine(mine, line);
          if (next === mine) return;
          mine = next;
          publish();
        });
        // A call that throws before it returns a promise is refused like one
        // that rejects, so it never leaves the store starting. It runs a
        // microtask later, after the listeners above are up.
        Promise.resolve()
          .then(call)
          .then((v) => {
            if (!v) {
              // The person said No: nothing was reviewed.
              if (!begun) {
                starting_ = false;
                resolve(false);
              }
              return;
            }
            mine = finished(mine, v);
            begin();
            publish();
          })
          .catch((e) => {
            if (!begun) {
              starting_ = false;
              reject(e);
              return;
            }
            mine = failed(mine, String(e));
            publish();
          })
          .finally(() => {
            offProgress();
            offLine();
          });
      });
    },

    /** Stop: the review ends, and its report says what of it ran. */
    async stop(): Promise<void> {
      if (!review || review.over) return;
      markStopping?.();
      try {
        await deps.stop();
      } catch {
        // The review ended as Stop was pressed: its own report is what shows.
      }
    },

    show(): void {
      if (!review) return;
      shown = true;
      raised++;
      notify();
    },

    /** Dismiss, once the review is over: the flow goes, and the app forgets
     *  the review's folder (the folder and its report stay on disk). */
    dismiss(): void {
      if (review && !review.over) return;
      if (review?.runId) deps.forget(review.runId);
      review = null;
      owner = null;
      markStopping = null;
      shown = false;
      notify();
    },
  };
}
