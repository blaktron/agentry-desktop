// A built-in agent's job, on this computer or In Agentry, kept outside every
// page as runs.ts keeps a run, so it outlives the dialog that started it: the
// dialog hands the job over once it has begun and closes, and the job's run
// window (RunWindow.tsx, mounted by App above the pages) follows it to its
// end, open or as its card. One job of a kind at a time: Go runs one job on
// this computer under the run lock, and the registry one run of a built-in per
// account.
//
// The Voice Writer and the Report Designer are followed through this store
// (voiceRun.ts, designerRun.ts); a new built-in is too, by naming its events
// and its calls. Pure apart from the deps it is given, so node tests hold its
// rules.
import type { JobProgress } from "./jobProgress";
import { EMPTY_LOG, type RunLog, withLogLine } from "./runLog";

export type Where = "here" | "agentry";

/** What a run In Agentry's view must carry for the store to follow it. */
export type HostedLike = { id: string; final: boolean };

export type BuiltinJob<R, H extends HostedLike, M> = {
  where: Where;
  /** What the flow that started it needs to draw it (its registry, its job). */
  meta: M;
  progress: JobProgress | null;
  log: RunLog;
  /** What a job on this computer made. */
  result: R | null;
  /** Why it ended with nothing, in Go's or the registry's words. */
  error: string | null;
  /** A job In Agentry, as the registry last reported it. */
  hosted: H | null;
  over: boolean;
  /** Stop was asked for; stopped: the job ended after it. */
  stopping: boolean;
  stopped: boolean;
};

export type BuiltinJobState<R, H extends HostedLike, M> = {
  job: BuiltinJob<R, H, M> | null;
  /** The window is up (as the dialog or the card) until it is dismissed. */
  shown: boolean;
  /** The asks to open it; asCard says the last was for the card. */
  raised: number;
  asCard: boolean;
  /** Counts the jobs that ended with something made, so a page reads what
   *  it shows again. */
  landed: number;
};

export type BuiltinJobDeps<H> = {
  on: (event: string, cb: (v: never) => void) => () => void;
  /** The events a job on this computer reports on, and a job In Agentry's. */
  logEvent: string;
  progressEvent: string;
  hostedEvent: string;
  /** Stops the job on this computer, and nothing else. */
  stop: () => Promise<unknown>;
  /** Asks the registry to cancel the job In Agentry; answers its view. */
  cancel: () => Promise<H | null | undefined>;
  /** Says whether a job In Agentry that ended made what it was for. */
  applied: (h: H) => boolean;
};

export function createBuiltinJobStore<R, H extends HostedLike, M>(deps: BuiltinJobDeps<H>) {
  let job: BuiltinJob<R, H, M> | null = null;
  let shown = false;
  let raised = 0;
  let asCard = false;
  let landed = 0;
  // starting: a start on this computer waits for its job to begin; a second
  // waits for it to begin or be refused, as Go's reports do not say which
  // start they answer.
  let starting = false;
  let listening = false;
  // patchHere changes the record of the job on this computer that is
  // followed, which its start owns: Stop marks it there, so the end the
  // start publishes says it was stopped.
  let patchHere: ((f: (j: BuiltinJob<R, H, M>) => BuiltinJob<R, H, M>) => void) | null = null;
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((f) => f());
  const running = () => !!job && !job.over;

  function fresh(where: Where, meta: M, hosted: H | null): BuiltinJob<R, H, M> {
    return { where, meta, progress: null, log: EMPTY_LOG, result: null, error: null, hosted, over: false, stopping: false, stopped: false };
  }

  function raise(card: boolean) {
    shown = true;
    asCard = card;
    raised++;
  }

  // A job In Agentry reports every state change and its final word on one
  // event, for the app's life once one is followed.
  function listen() {
    if (listening) return;
    listening = true;
    deps.on(deps.hostedEvent, (h: H) => takeHosted(h));
  }

  function takeHosted(h: H) {
    if (!job || job.where !== "agentry" || job.hosted?.id !== h.id || job.over) return;
    job = { ...job, hosted: h, over: h.final };
    if (h.final && deps.applied(h)) landed++;
    notify();
  }

  return {
    state: (): BuiltinJobState<R, H, M> => ({ job, shown, raised, asCard, landed }),
    subscribe(f: () => void): () => void {
      listeners.add(f);
      return () => {
        listeners.delete(f);
      };
    },
    running,

    /** Starts a job on this computer by call: resolves once Go reports that
     *  it has begun (its first progress or log line, after its native
     *  dialog and inside the run lock), which is when the dialog that
     *  started it closes, and rejects with Go's words if it never began. A
     *  job that is under way is left as it is. */
    startHere(meta: M, call: () => Promise<R>): Promise<void> {
      if (starting || running()) return Promise.reject(new Error("A built-in agent is already running; wait for it to finish, or stop it."));
      starting = true;
      let mine = fresh("here", meta, null);
      let begun = false;
      const publish = () => {
        if (!begun) return;
        job = mine;
        notify();
      };
      return new Promise((resolve, reject) => {
        const begin = () => {
          if (begun) return;
          begun = true;
          starting = false;
          patchHere = (f) => {
            mine = f(mine);
            publish();
          };
          raise(false);
          resolve();
        };
        const offLog = deps.on(deps.logEvent, (line: string) => {
          if (mine.over) return;
          mine = { ...mine, log: withLogLine(mine.log, line) };
          begin();
          publish();
        });
        const offProgress = deps.on(deps.progressEvent, (p: JobProgress) => {
          if (mine.over) return;
          mine = { ...mine, progress: p };
          begin();
          publish();
        });
        Promise.resolve()
          .then(call)
          .then((r) => {
            mine = { ...mine, result: r, over: true, stopped: mine.stopping };
            landed++;
            begin();
            publish();
          })
          .catch((e) => {
            if (!begun) {
              starting = false;
              reject(e);
              return;
            }
            mine = { ...mine, error: String(e).replace(/^Error: /, ""), over: true, stopped: mine.stopping };
            publish();
          })
          .finally(() => {
            offLog();
            offProgress();
          });
      });
    },

    /** Follows a job In Agentry the registry has just created, in its
     *  dialog, or one already in flight, as its card. One already followed
     *  is brought up again rather than replaced. */
    followHosted(meta: M, h: H, card: boolean): void {
      listen();
      if (job?.where === "agentry" && job.hosted?.id === h.id) {
        if (!job.over && h.final) takeHosted(h);
        raise(card);
        notify();
        return;
      }
      if (running()) return;
      job = { ...fresh("agentry", meta, h), over: h.final };
      patchHere = null;
      raise(card);
      notify();
    },

    /** Whether the job In Agentry with this id is the one followed. */
    follows(id: string): boolean {
      return job?.where === "agentry" && job.hosted?.id === id;
    },

    /** Stop on a job on this computer: it ends as far as it got. */
    async stop(): Promise<void> {
      if (!job || job.over || job.where !== "here" || !patchHere) return;
      const patch = patchHere;
      patch((j) => (j.over ? j : { ...j, stopping: true }));
      try {
        await deps.stop();
      } catch {
        // It ended as Stop was pressed: its own result is what shows.
        patch((j) => (j.over ? j : { ...j, stopping: false }));
      }
    },

    /** Cancel run on a job In Agentry: the registry's answer is shown. */
    async cancel(): Promise<void> {
      if (!job || job.over || job.where !== "agentry") return;
      const h = await deps.cancel();
      if (h) takeHosted(h);
    },

    /** Opens the window as the dialog. */
    show(): void {
      if (!job) return;
      raise(false);
      notify();
    },

    /** Dismiss, once the job is over: the window goes and the app forgets
     *  the job. */
    dismiss(): void {
      if (running()) return;
      job = null;
      patchHere = null;
      shown = false;
      asCard = false;
      notify();
    },

    /** Marks that something the job made was kept elsewhere (a design
     *  added to the library), so the pages read it again. */
    markLanded(): void {
      landed++;
      notify();
    },
  };
}
