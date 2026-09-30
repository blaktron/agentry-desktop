// The one run on this computer this app is following (agentry-desktop#465;
// plan desktop-review-key-and-run-cards.md §5.2), kept outside every page as
// drafts.ts keeps a draft: the run setup hands the run over at Run, closes
// once Go reports the run has begun, and RunFlow (mounted by App above the
// pages) follows it in its dialog and corner card. The run is followed by
// its id (localRun.ts), so a setup open on another QuickAgent never shows its
// lines or its result. Dismiss makes the app forget it (RunAPI.ForgetRun);
// quitting forgets it too, since nothing of it is kept.
//
// A run In Agentry is followed the same way, by its own store beside this
// one (agentry-desktop#466), which HostedRunFlow draws.
//
// The stores' rules are localRun.ts's createRunStore and hostedRun.ts's
// createHostedRunStore, which node tests hold; this module wires them to
// Go's events and calls.
import { useEffect, useState } from "react";
import * as RunAPI from "../wailsjs/go/main/RunAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { main, trust } from "../wailsjs/go/models";
import { type RunStoreState, createRunStore } from "./localRun";
import { type HostedRunState, createHostedRunStore } from "./hostedRun";
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

// The run In Agentry the app follows (agentry-desktop#466), beside the run
// on this computer: Go reports it as runner:hosted-run, which this store
// listens to for the app's life, so a run that ends with its dialog closed
// lands in its card with its result. At most one of each: Go runs one local
// job at a time, and the registry one run per account.
const hostedStore = createHostedRunStore<RunSetup>({
  on: (event, cb) => EventsOn(event, cb as (v: unknown) => void),
  cancel: () => RunAPI.CancelHostedRun(),
  forget: (runId) => void RunAPI.ForgetRun(runId),
});

export type HostedState = HostedRunState<RunSetup>;

/** The run In Agentry the app follows, re-read whenever it changes. */
export function useHostedRun(): HostedState {
  const [, bump] = useState(0);
  useEffect(() => hostedStore.subscribe(() => bump((n) => n + 1)), []);
  return hostedStore.state();
}

/** Starts a run In Agentry by call (RunAPI.StartHostedRun) and follows it
 *  once the registry has created it; a refusal is handed back for the setup
 *  to show, and nothing is followed. */
export const startHostedRun = (s: RunSetup, call: () => Promise<main.HostedStartView>): Promise<main.HostedStartView> =>
  hostedStore.start(s, call);

/** Follows this account's run In Agentry already in flight, as its card:
 *  App asks as it starts. */
export const attachHostedRun = (registry: string): Promise<boolean> =>
  hostedStore.attach(() => RunAPI.AttachHostedRunAtStart(registry));

/** Follows, as its card, a run In Agentry a run setup found in flight as it
 *  opened (RunAPI.AttachHostedRun): one started elsewhere since the app
 *  started. A run already followed stays as it is. */
export const followHostedRunFound = (found: main.HostedStartView): Promise<boolean> =>
  hostedStore.attach(async () => found);

/** Follows again a run the app lost touch with, in its dialog. */
export const followHostedRunAgain = (registry: string): Promise<boolean> =>
  hostedStore.attach(() => RunAPI.AttachHostedRunAtStart(registry), false);

/** Asks Agentry to cancel the run. */
export const cancelHostedRun = (): Promise<void> => hostedStore.cancel();

/** Opens the run In Agentry's flow as the dialog. */
export const showHostedRun = (): void => hostedStore.show();

/** Dismiss, once the run is over: the app forgets it. */
export const dismissHostedRun = (): void => hostedStore.dismiss();

/** Back to the setup from a run In Agentry that is over: it is dismissed,
 *  and its setup, if it had one, is returned to open again as it was. */
export function backToHostedSetup(): RunSetup | null {
  const s = hostedStore.state().setup;
  hostedStore.dismiss();
  return s;
}
