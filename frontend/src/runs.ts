// The one run on this computer this app is following (agentry-desktop#465;
// plan desktop-review-key-and-run-cards.md §5.2), kept outside every page as
// drafts.ts keeps a draft: the run setup hands the run over at Run, closes
// once Go reports the run has begun, and RunFlow (mounted by App above the
// pages) follows it in its dialog and corner card. The run is followed by
// its id (localRun.ts), so a setup open on another QuickAgent never shows its
// lines or its result. Dismiss makes the app forget it (RunAPI.ForgetRun);
// quitting forgets it too, since nothing of it is kept.
import { useEffect, useState } from "react";
import * as RunAPI from "../wailsjs/go/main/RunAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { main, trust } from "../wailsjs/go/models";
import { type LocalRun, type RunLine, type RunProgress, failed, finished, starting, stopAsked, withLine, withProgress } from "./localRun";
import type { RunForm } from "./runSetup";

/** What reopens the run setup as the person left it. */
export type RunSetup = {
  panel: {
    registry: string;
    ns: string;
    name: string;
    version: string;
    summary?: string;
    attestation: trust.AttestationResult | null;
    local?: boolean;
  };
  form: RunForm;
};

let run: LocalRun | null = null;
let setup: RunSetup | null = null;
// shown: the flow is up, as the dialog or the card, until it is dismissed;
// raised counts the asks to open it as the dialog.
let shown = false;
let raised = 0;
// reopen: the setup Back to the setup asked App to open again.
let reopen: RunSetup | null = null;
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((f) => f());

export type RunState = {
  run: LocalRun | null;
  setup: RunSetup | null;
  shown: boolean;
  raised: number;
  reopen: RunSetup | null;
};

/** The run the app follows, re-read whenever it changes. */
export function useLocalRun(): RunState {
  const [, bump] = useState(0);
  useEffect(() => {
    const f = () => bump((n) => n + 1);
    listeners.add(f);
    return () => {
      listeners.delete(f);
    };
  }, []);
  return { run, setup, shown, raised, reopen };
}

/** Whether a run on this computer is under way. */
export function localRunInFlight(): boolean {
  return run !== null && !run.over;
}

/** Starts a run of ref by call (RunAPI.RunAgent): the promise resolves once
 *  Go reports that the run has begun, which is when the setup closes, and
 *  rejects with Go's words if it never began (the consent declined, the lock
 *  held, a gate refused), leaving any earlier run's card as it was. */
export function startLocalRun(s: RunSetup, ref: string, call: () => Promise<main.RunResultView>): Promise<void> {
  const before = run;
  const beforeSetup = setup;
  return new Promise((resolve, reject) => {
    let begun = false;
    const begin = () => {
      if (begun) return;
      begun = true;
      // An earlier run whose card was still up is forgotten now.
      if (before?.runId && before.runId !== run?.runId) void RunAPI.ForgetRun(before.runId);
      setup = s;
      shown = true;
      raised++;
      resolve();
    };
    run = starting(ref);
    const offProgress = EventsOn("runner:run-progress", (p: RunProgress) => {
      if (!run) return;
      const next = withProgress(run, p);
      if (next === run) return;
      run = next;
      begin();
      notify();
    });
    const offLine = EventsOn("runner:run-line", (l: RunLine) => {
      if (!run) return;
      const next = withLine(run, l);
      if (next === run) return;
      run = next;
      notify();
    });
    call()
      .then((r) => {
        if (!run) return;
        run = finished(run, r);
        begin();
      })
      .catch((e) => {
        if (!begun) {
          run = before;
          setup = beforeSetup;
          reject(e);
          return;
        }
        if (run) run = failed(run, String(e));
      })
      .finally(() => {
        offProgress();
        offLine();
        notify();
      });
  });
}

/** Stop: the run ends and its receipts are written as far as it got. */
export async function stopLocalRun(): Promise<void> {
  if (!run || run.over) return;
  run = stopAsked(run);
  notify();
  try {
    await RunAPI.StopRun();
  } catch {
    // The run ended as Stop was pressed: its own result is what shows.
  }
}

/** Opens the flow as the dialog. */
export function showLocalRun(): void {
  shown = true;
  raised++;
  notify();
}

/** Dismiss: the flow goes, and the app forgets the run. */
export function dismissLocalRun(): void {
  if (run?.runId) void RunAPI.ForgetRun(run.runId);
  run = null;
  setup = null;
  shown = false;
  notify();
}

/** Back to the setup: the run is dismissed, and App opens its setup again
 *  with the answers as they were. */
export function backToSetup(): void {
  const s = setup;
  dismissLocalRun();
  reopen = s;
  notify();
}

/** The setup to open again, taken. */
export function takeReopen(): RunSetup | null {
  const s = reopen;
  reopen = null;
  if (s) notify();
  return s;
}
