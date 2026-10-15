// A run In Agentry, as the app follows it above the pages: the
// registry's own view of it, which Go re-reads and reports as
// runner:hosted-run, kept by the run's id for the app's life, so a run that
// ends while its dialog is closed lands in its card with its result. The
// desktop follows the run and never drives it: Cancel asks the
// registry, and what the dialog and card say is what the registry said.
// Pure, so the store (runs.ts) and its tests share one set of rules.
import type { main } from "../wailsjs/go/models";
import type { JobProgress, Tone } from "./jobProgress";

type View = main.HostedRunView;

/** The run is still moving: queued, claimed by a host, or running. */
export function hostedActive(v: View): boolean {
  return v.status === "queued" || v.status === "claimed" || v.status === "running";
}

/** The app stopped hearing from the registry while the run was still
 *  moving: Go's last word on it is not how it ended. */
export function lostTouch(v: View): boolean {
  return v.final && hostedActive(v);
}

/** How a run In Agentry that is over ended, in a sentence: a pass says so,
 *  and anything else says what the registry's verdict line says. A run the
 *  app lost touch with has not ended as far as it knows. */
export function hostedHeadline(v: View): string {
  if (lostTouch(v)) return "This app lost touch with Agentry while the run was still going.";
  if (v.outcome === "succeeded") return "Finished.";
  if (v.status === "cancelled") return "Cancelled before it started.";
  const line = (v.outcomeLine || v.statusLine || "it did not finish").trim();
  return line.charAt(0).toUpperCase() + line.slice(1).replace(/\.?$/, ".");
}

/** The tone the registry's verdict is drawn in. */
export function hostedTone(v: View): Exclude<Tone, ""> {
  if (lostTouch(v)) return "warn";
  return v.outcomeTone === "ok" ? "ok" : v.outcomeTone === "bad" ? "bad" : "warn";
}

/** How far the run has got as the registry reports it: its clock from when
 *  a host started it, against the cap the run was created under. Before a
 *  host starts it, and for a registry that names no start, there is none.
 *  The registry reports turns only once the run is over, and a run In
 *  Agentry carries no tokens at all. */
export function hostedProgress(v: View): JobProgress | null {
  const started = v.startedAt ? Date.parse(v.startedAt) : NaN;
  if (!Number.isFinite(started)) return null;
  return {
    startedAt: started,
    timeoutSeconds: v.caps?.timeoutSeconds ?? 0,
    maxTurns: v.caps?.maxTurns ?? 0,
    turns: v.turns,
  };
}

/** The run is over and its last word is how it ended: not a run the app
 *  lost touch with, which may still be going. */
export function ended(v: View): boolean {
  return v.final && !lostTouch(v);
}

/** A report of the run followed: another run's, or one after the run's last
 *  word, changes nothing. */
export function withReport(cur: View | null, v: View): View | null {
  if (!cur || v.id !== cur.id || ended(cur)) return cur;
  return v;
}

/** What the store needs from the app: Go's events and the calls that cancel
 *  and forget a run. Injected, so its rules are tested without a webview. */
export type HostedRunDeps = {
  on: (event: string, cb: (v: never) => void) => () => void;
  cancel: () => Promise<View>;
  forget: (runId: string) => void;
};

export type HostedRunState<S> = {
  run: View | null;
  /** The registry the run is followed on, which Follow it again asks. */
  registry: string;
  /** A start is waiting for the registry's answer. */
  starting: boolean;
  /** What reopens the run's setup; none for a run attached at start. */
  setup: S | null;
  shown: boolean;
  raised: number;
  /** The last ask was for the card (an attach), not the dialog. */
  asCard: boolean;
  /** Why the last Cancel or Follow it again did not go through. */
  note: string | null;
};

/** The one run In Agentry the app follows. Its
 *  listener, once taken, lives as long as the store: a report of the run
 *  followed is taken whether or not the dialog, the card or any setup is
 *  open. */
export function createHostedRunStore<S>(deps: HostedRunDeps) {
  let run: View | null = null;
  let registry = "";
  let setup: S | null = null;
  let shown = false;
  let raised = 0;
  let asCard = false;
  let note: string | null = null;
  // starting: a start is waiting for the registry's answer. Reports that
  // arrive meanwhile are kept by id, so the run's first moves are not lost
  // to the answer that names it.
  let starting = false;
  let early = new Map<string, View>();
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((f) => f());

  // The listener is taken on first use (App's flow subscribes as it
  // mounts) and kept for the store's life.
  let listening = false;
  const listen = () => {
    if (listening) return;
    listening = true;
    deps.on("runner:hosted-run", (v: View) => {
      if (starting) early.set(v.id, v);
      const next = withReport(run, v);
      if (next === run) return;
      run = next;
      notify();
    });
  };

  // follow takes v as the run followed: a card that is over for another
  // run is forgotten, and the dialog (or, for an attach, the card) opens.
  const follow = (v: View, reg: string, s: S | null, card: boolean) => {
    if (run && run.id !== v.id && ended(run)) deps.forget(run.id);
    if (run && run.id === v.id) s = s ?? setup;
    run = early.get(v.id) ?? v;
    registry = reg;
    early = new Map();
    setup = s;
    note = null;
    shown = true;
    asCard = card;
    raised++;
  };

  return {
    state: (): HostedRunState<S> => ({ run, registry, starting, setup, shown, raised, asCard, note }),
    subscribe(f: () => void): () => void {
      listen();
      listeners.add(f);
      return () => {
        listeners.delete(f);
      };
    },

    /** Starts a run In Agentry by call (RunAPI.StartHostedRun) and follows
     *  it once the registry has created it. A refusal, or an answer with no
     *  run, is handed back for the setup to show, and nothing is followed. */
    async start(s: S, reg: string, call: () => Promise<main.HostedStartView>): Promise<main.HostedStartView> {
      if (starting) throw "A run In Agentry is being started; wait for the registry's answer.";
      listen();
      starting = true;
      early = new Map();
      notify();
      try {
        const v = await Promise.resolve().then(call);
        if (!v.refusal && v.run) follow(v.run, reg, s, false);
        return v;
      } finally {
        starting = false;
        early = new Map();
        notify();
      }
    },

    /** Follows the run an attach found (RunAPI.AttachHostedRunAtStart), as
     *  its card: at start, and when the person asks to follow a run the app
     *  lost touch with. It says whether there was one. */
    async attach(reg: string, call: () => Promise<main.HostedStartView>, card = true): Promise<boolean> {
      if (starting || (run && !run.final)) return false;
      listen();
      let v: main.HostedStartView;
      try {
        v = await Promise.resolve().then(call);
      } catch (e) {
        note = String(e);
        notify();
        return false;
      }
      if (!v.run) {
        if (run && lostTouch(run)) {
          note = "Agentry lists no run of this account in progress any more, so this app cannot follow it again. " +
            "Agentry's Runs page shows how it ended.";
          notify();
        }
        return false;
      }
      follow(v.run, reg, null, card);
      notify();
      return true;
    },

    /** Asks the registry to cancel the run: queued, it is cancelled and its
     *  hold released; running, its host interrupts it and settles what it
     *  used. */
    async cancel(): Promise<void> {
      if (!run || run.final) return;
      const id = run.id;
      try {
        const v = await deps.cancel();
        if (run && run.id === id && v.id === id && !run.final) run = v;
        note = null;
      } catch (e) {
        note = String(e);
      }
      notify();
    },

    /** Opens the flow as the dialog. */
    show(): void {
      shown = true;
      asCard = false;
      raised++;
      notify();
    },

    /** Dismiss, once the run is over: the flow goes, and the app forgets
     *  the run (its folder is left as it is). */
    dismiss(): void {
      if (run && !run.final) return;
      if (run) deps.forget(run.id);
      run = null;
      registry = "";
      setup = null;
      shown = false;
      asCard = false;
      note = null;
      notify();
    },
  };
}
