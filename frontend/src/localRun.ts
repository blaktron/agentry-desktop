// A run on this computer, as the app follows it above the pages
// (agentry-desktop#465; plan desktop-review-key-and-run-cards.md §5.2): what
// Go has reported of it, keyed by its id, and how it ended. Pure, so the
// store (runs.ts) and its tests share one set of rules: a report or a line of
// another run, or of this run after it ended, changes nothing.
import type { main } from "../wailsjs/go/models";
import type { JobProgress, Tone } from "./jobProgress";

/** How far a run on this computer has got, as Go reports it
 *  (runner:run-progress, RunAPI's RunProgress): the report every job carries,
 *  the run's id and the release. Until it is prepared its budgets are zero
 *  and its clock counts the preparation; once prepared, the clock starts
 *  again and measures the run against its own timeout. */
export type RunProgress = JobProgress & { runId: string; ref: string; prepared: boolean };

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

/** How a run that produced a result ended, as its result view's headline
 *  says it: stopped, finished, or in an error. A chat's result says it the
 *  same way. */
export function headlineOf(result: main.RunResultView, stopped = false): { line: string; tone: Exclude<Tone, ""> } {
  if (stopped) return { line: "You stopped the run. Its receipts are written as far as it got.", tone: "warn" };
  if (result.status === "ok") return { line: "Finished.", tone: "ok" };
  return { line: `It ended in an error (exit ${result.exitCode}).`, tone: "bad" };
}

/** How the run ended, in the card's line and tone; "" while it runs. A run
 *  stopped before the engine started has no receipts to speak of. */
export function outcome(r: LocalRun): { line: string; tone: Tone } {
  if (!r.over) return { line: "", tone: "" };
  if (r.result) return headlineOf(r.result, r.stopped);
  if (r.stopped) return { line: "You stopped the run before the QuickAgent started.", tone: "warn" };
  return { line: "The run did not finish.", tone: "bad" };
}

/** What the store needs from the app: Go's events, and the calls that stop
 *  and forget a run. Injected, so the store's rules are tested without a
 *  webview. */
export type RunStoreDeps = {
  on: (event: string, cb: (v: never) => void) => () => void;
  stop: () => Promise<void>;
  forget: (runId: string) => void;
};

export type RunStoreState<S> = { run: LocalRun | null; setup: S | null; shown: boolean; raised: number };

/** The one run on this computer the app follows (agentry-desktop#465). A
 *  start owns nothing shared until its own run has begun: the run being
 *  followed (in the background, or a card that is over) is left as it is
 *  while a second Run waits for its consent and for Go's answer, and a start
 *  Go refuses changes nothing. setup is whatever reopens the run's setup. */
export function createRunStore<S>(deps: RunStoreDeps) {
  let run: LocalRun | null = null;
  let setup: S | null = null;
  // owner is the start whose run is the one followed: only it writes run,
  // and markStopping is how Stop reaches that start's own record.
  let owner: object | null = null;
  let markStopping: (() => void) | null = null;
  // starting: a start is waiting for its run to begin. Go runs one local
  // job at a time and a report does not say which start it answers, so a
  // second start waits for the first to begin or be refused.
  let starting_ = false;
  let shown = false;
  let raised = 0;
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((f) => f());

  return {
    state: (): RunStoreState<S> => ({ run, setup, shown, raised }),
    subscribe(f: () => void): () => void {
      listeners.add(f);
      return () => {
        listeners.delete(f);
      };
    },

    /** Starts a run of ref by call: resolves once Go reports that the run
     *  has begun, which is when the setup closes, and rejects with Go's
     *  words if it never began. */
    start(s: S, ref: string, call: () => Promise<main.RunResultView>): Promise<void> {
      if (starting_) return Promise.reject("A run is being started on this computer; wait for it to begin, then start another.");
      starting_ = true;
      const token = {};
      // The run followed when this start began is not this start's,
      // whatever its reports say.
      const followed = run && !run.over ? run.runId : null;
      let mine = starting(ref);
      let begun = false;
      const publish = () => {
        if (owner !== token) return;
        run = mine;
        notify();
      };
      return new Promise((resolve, reject) => {
        const begin = () => {
          if (begun) return;
          begun = true;
          starting_ = false;
          // An earlier run whose card was still up is forgotten now.
          if (run?.runId && run.runId !== mine.runId) deps.forget(run.runId);
          owner = token;
          markStopping = () => {
            mine = stopAsked(mine);
            publish();
          };
          setup = s;
          shown = true;
          raised++;
          resolve();
        };
        const offProgress = deps.on("runner:run-progress", (p: RunProgress) => {
          if (p.runId === followed) return;
          const next = withProgress(mine, p);
          if (next === mine) return;
          mine = next;
          begin();
          publish();
        });
        const offLine = deps.on("runner:run-line", (l: RunLine) => {
          const next = withLine(mine, l);
          if (next === mine) return;
          mine = next;
          publish();
        });
        // A call that throws before it returns a promise is refused like
        // one that rejects, so it never leaves the store starting. It runs
        // a microtask later, after the listeners above are up.
        Promise.resolve()
          .then(call)
          .then((r) => {
            mine = finished(mine, r);
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

    /** Stop: the run ends and its receipts are written as far as it got. */
    async stop(): Promise<void> {
      if (!run || run.over) return;
      markStopping?.();
      try {
        await deps.stop();
      } catch {
        // The run ended as Stop was pressed: its own result is what shows.
      }
    },

    /** Opens the flow as the dialog. */
    show(): void {
      shown = true;
      raised++;
      notify();
    },

    /** Dismiss: the flow goes, and the app forgets the run. */
    dismiss(): void {
      if (run?.runId) deps.forget(run.runId);
      run = null;
      setup = null;
      owner = null;
      markStopping = null;
      shown = false;
      notify();
    },
  };
}
