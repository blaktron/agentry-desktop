// The Voice Writer's run, on this computer or In Agentry, followed in its
// run window (VoiceFlow, mounted by App above the pages) through the
// built-ins' job store (builtinJob.ts): Edit with AI hands the run over once
// it has begun and closes, and the run goes on in the window or its card
// while the person uses the rest of the app. The Voice page reads the voice
// again whenever a run lands.
import { useEffect, useState } from "react";
import * as VoiceAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/voiceapi";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as voice from "../bindings/github.com/blaktron/agentry-desktop/internal/voice/models";
import { type BuiltinJobState, createBuiltinJobStore } from "./builtinJob";
import { EventsOn } from "./host";

/** What the window says of the run: its registry, whether it edits a voice
 *  or writes the first, and what it was given to read. */
export type VoiceMeta = { registry: string; hasVoice: boolean; samples: number; urls: number };

const store = createBuiltinJobStore<voice.BuildResult, app.VoiceHostedView, VoiceMeta>({
  on: (event, cb) => EventsOn(event, cb as (v: unknown) => void),
  logEvent: "voice:write-log",
  progressEvent: "voice:write-progress",
  hostedEvent: "voice:hosted",
  stop: () => VoiceAPI.StopWrite(),
  cancel: () => VoiceAPI.CancelHosted(),
  applied: (h) => h.applied,
});

export type VoiceRunState = BuiltinJobState<voice.BuildResult, app.VoiceHostedView, VoiceMeta>;

/** The Voice Writer's run the app follows, re-read whenever it changes. */
export function useVoiceRun(): VoiceRunState {
  const [, bump] = useState(0);
  useEffect(() => store.subscribe(() => bump((n) => n + 1)), []);
  return store.state();
}

/** Runs the Voice Writer on this computer (VoiceAPI.WriteWithAI): resolves
 *  once it has begun, rejects with Go's words if it never began (a No in
 *  the native dialog, another job under way). */
export const startVoiceHere = (meta: VoiceMeta, text: string, samples: string[], urls: string[]): Promise<void> =>
  store.startHere(meta, () => VoiceAPI.WriteWithAI(meta.registry, text, samples, urls));

/** Follows a Voice Writer run In Agentry: in its window for one just
 *  started or asked to be shown, as its card for one found under way. */
export const followVoiceHosted = (meta: VoiceMeta, h: app.VoiceHostedView, card = false): void => store.followHosted(meta, h, card);

/** Brings up the window of the run In Agentry h, following it first. */
export function showVoiceHosted(meta: VoiceMeta, h: app.VoiceHostedView): void {
  if (store.follows(h.id)) store.show();
  else store.followHosted(meta, h, false);
}

export const stopVoiceRun = (): Promise<void> => store.stop();
export const cancelVoiceRun = (): Promise<void> => store.cancel();
export const showVoiceRun = (): void => store.show();
export const dismissVoiceRun = (): void => store.dismiss();
