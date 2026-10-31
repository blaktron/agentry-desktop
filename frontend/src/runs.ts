// The one run on this computer this app is following, kept outside every page as
// drafts.ts keeps a draft: the run setup hands the run over at Run, closes
// once Go reports the run has begun, and RunFlow (mounted by App above the
// pages) follows it in its dialog and corner card. The run is followed by
// its id (localRun.ts), so a setup open on another QuickAgent never shows its
// lines or its result. Dismiss makes the app forget it (RunAPI.ForgetRun);
// quitting forgets it too, since nothing of it is kept.
//
// A run In Agentry is followed the same way, by its own store beside this
// one, which HostedRunFlow draws.
//
// The stores' rules are localRun.ts's createRunStore and hostedRun.ts's
// createHostedRunStore, which node tests hold; this module wires them to
// Go's events and calls.
import { useEffect, useState } from "react";
import * as RunAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { EventsOn } from "./host";
import * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import * as trust from "../bindings/github.com/blaktron/agentry-cli/pkg/trust/models";
import { type QueuedRun, type RunStoreState, createRunStore } from "./localRun";
import { type HostedRunState, createHostedRunStore } from "./hostedRun";
import { type ChatStoreState, createChatStore } from "./sessionChat";
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
  cancelQueued: (runId) => RunAPI.CancelQueuedRun(runId),
});

// The runs waiting to start, for the app's life from the first use of the
// run store (RunFlow is mounted with the app): read once, then as Go reports
// each change. Not at module load, before the runtime is there.
let watchingQueue = false;
function watchQueue(): void {
  if (watchingQueue) return;
  watchingQueue = true;
  EventsOn("runner:run-queue", (q: QueuedRun[]) => store.setQueue(q));
  void Promise.resolve()
    .then(() => RunAPI.QueuedRuns())
    .then((q) => store.setQueue(q as QueuedRun[]), () => {});
}

export type RunState = RunStoreState<RunSetup>;

/** The run the app follows, re-read whenever it changes. */
export function useLocalRun(): RunState {
  const [, bump] = useState(0);
  useEffect(() => {
    watchQueue();
    return store.subscribe(() => bump((n) => n + 1));
  }, []);
  return store.state();
}

/** Starts a run of ref by call (RunAPI.RunAgent): the promise resolves once
 *  Go reports that the run has begun, which is when the setup closes, and
 *  rejects with Go's words if it never began (the consent declined, a gate
 *  refused). A run Go queues behind another job resolves once it is
 *  queued, and its card takes over when it begins. */
export function startLocalRun(s: RunSetup, ref: string, call: () => Promise<app.RunResultView>): Promise<void> {
  watchQueue();
  return store.start(s, ref, call);
}

/** Stop: the run ends and its receipts are written as far as it got. */
export const stopLocalRun = (): Promise<void> => store.stop();

/** Cancel on a queued run: it leaves the queue without starting. */
export const cancelQueuedRun = (runId: string): Promise<void> => store.cancelQueued(runId);

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

// The run In Agentry the app follows, beside the run
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
export const startHostedRun = (s: RunSetup, call: () => Promise<app.HostedStartView>): Promise<app.HostedStartView> =>
  hostedStore.start(s, s.panel.registry, call);

/** Follows what an attach found In Agentry: a chat by the chat's store, a
 *  run by the run's. */
async function followFound(registry: string, found: app.HostedStartView, card: boolean): Promise<boolean> {
  if (found.run?.mode === "interactive") {
    return chatStore.followHosted({ registry, run: found.run, terms: null, opening: "", task: found.run.task ?? "" }, null, card);
  }
  return hostedStore.attach(registry, async () => found, card);
}

/** Follows this account's run or chat In Agentry already in flight, as its
 *  card: App asks as it starts. */
export async function attachHostedRun(registry: string): Promise<boolean> {
  const found = await RunAPI.AttachHostedRunAtStart(registry).catch(() => null);
  return found ? followFound(registry, found, true) : false;
}

/** Follows, as its card, a run or chat In Agentry a run setup found in
 *  flight as it opened (RunAPI.AttachHostedRun): one started elsewhere since
 *  the app started. One already followed stays as it is. */
export const followHostedRunFound = (registry: string, found: app.HostedStartView): Promise<boolean> =>
  followFound(registry, found, true);

/** Follows again, in its dialog, a run or chat the app lost touch with, on
 *  the registry it was followed on. */
export async function followHostedRunAgain(registry = hostedStore.state().registry): Promise<boolean> {
  return followFound(registry, await RunAPI.AttachHostedRunAtStart(registry), false);
}

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

// The chat the app follows, on this computer or In
// Agentry: its store starts the session and owns its events, so the chat's
// dialog and card only draw it, and collects a chat the engine ended whether
// or not anything shows it. One chat per app.
const chatStore = createChatStore<RunSetup>({
  on: (event, cb) => EventsOn(event, cb as (v: unknown) => void),
  sendLocal: (text) => RunAPI.SendTurn(text),
  addLocal: (path) => RunAPI.AddSessionInput(path),
  endLocal: (finish) => RunAPI.EndSession(finish),
  sendHosted: (text) => RunAPI.SendHostedTurn(text),
  addHosted: (path) => RunAPI.AddHostedSessionFile(path),
  endHosted: (finish) => RunAPI.EndHostedSession(finish),
  replay: () => RunAPI.ReplayHostedSession(),
  forget: (runId) => void RunAPI.ForgetRun(runId),
});

export type ChatState = ChatStoreState<RunSetup>;

/** The chat the app follows, re-read whenever it changes. */
export function useChat(): ChatState {
  const [, bump] = useState(0);
  useEffect(() => chatStore.subscribe(() => bump((n) => n + 1)), []);
  return chatStore.state();
}

/** Starts a chat on this computer by call (RunAPI.StartSession): its dialog
 *  opens at once with the start under way inside it. */
export function startLocalChat(s: RunSetup, ref: string, task: string, opening: string, call: () => Promise<unknown>): void {
  chatStore.startLocal(s, ref, task, opening, call);
}

/** Follows, in its dialog, a chat In Agentry the registry has just created. */
export function followHostedChat(s: RunSetup, run: app.HostedRunView, terms: app.HostedSessionOfferView | null, opening: string, task: string): void {
  chatStore.followHosted({ registry: s.panel.registry, run, terms, opening, task }, s, false);
}

/** Whether a chat is starting, going or ending. */
export const chatLive = (): boolean => chatStore.live();

export const sendChat = (text: string): Promise<void> => chatStore.send(text);
export const addChatFiles = (paths: string[]): Promise<void> => chatStore.addFiles(paths);
export const endChat = (finish: boolean): Promise<void> => chatStore.end(finish);
export const showChat = (): void => chatStore.show();
export const dismissChat = (): void => chatStore.dismiss();

/** Back to the setup from a chat that is over: it is dismissed, and its
 *  setup, if it had one, is returned to open again as it was. */
export function backToChatSetup(): RunSetup | null {
  const s = chatStore.state().setup;
  chatStore.dismiss();
  return s;
}
