// The one run on this computer this app is following (agentry-desktop#465;
// plan desktop-review-key-and-run-cards.md §5.2), kept outside every page as
// drafts.ts keeps a draft: the run setup hands the run over at Run, closes
// once Go reports the run has begun, and RunFlow (mounted by App above the
// pages) follows it in its dialog and corner card. The run is followed by
// its id (localRun.ts), so a setup open on another QuickAgent never shows its
// lines or its result. Dismiss makes the app forget it (RunAPI.ForgetRun);
// quitting forgets it too, since nothing of it is kept.
//
// The store's rules are localRun.ts's createRunStore, which a node test holds;
// this module wires it to Go's events and calls.
import { useEffect, useState } from "react";
import * as RunAPI from "../wailsjs/go/main/RunAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { main, trust } from "../wailsjs/go/models";
import { type RunStoreState, createRunStore } from "./localRun";
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

const store = createRunStore<RunSetup>({
  on: (event, cb) => EventsOn(event, cb as (v: unknown) => void),
  stop: () => RunAPI.StopRun(),
  forget: (runId) => void RunAPI.ForgetRun(runId),
});

export type RunState = RunStoreState<RunSetup>;

/** The run the app follows, re-read whenever it changes. */
export function useLocalRun(): RunState {
  const [, bump] = useState(0);
  useEffect(() => store.subscribe(() => bump((n) => n + 1)), []);
  return store.state();
}

/** Starts a run of ref by call (RunAPI.RunAgent): the promise resolves once
 *  Go reports that the run has begun, which is when the setup closes, and
 *  rejects with Go's words if it never began (the consent declined, another
 *  job holding the lock, a gate refused). */
export function startLocalRun(s: RunSetup, ref: string, call: () => Promise<main.RunResultView>): Promise<void> {
  return store.start(s, ref, call);
}

/** Stop: the run ends and its receipts are written as far as it got. */
export const stopLocalRun = (): Promise<void> => store.stop();

/** Opens the flow as the dialog. */
export const showLocalRun = (): void => store.show();

/** Dismiss: the flow goes, and the app forgets the run. */
export const dismissLocalRun = (): void => store.dismiss();

/** Back to the setup: the run is dismissed, and its setup is returned for
 *  the caller to open again with the answers as they were. */
export function backToSetup(): RunSetup | null {
  const s = store.state().setup;
  store.dismiss();
  return s;
}
