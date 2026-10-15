// The one review by the Examiner this app is following, kept outside every page
// as runs.ts keeps a run: the Build tab's summary and Validate start it, Go
// asks in a native dialog, and ReviewFlow (mounted by App above the pages)
// follows it in its dialog and corner card from Go's first report. Dismiss
// makes the app forget its folder (RunAPI.ForgetRun); quitting forgets it too.
//
// The store's rules are review.ts's createReviewStore, which node tests
// hold; this module wires it to Go's events and calls.
import { useEffect, useState } from "react";
import * as BuildAPI from "../wailsjs/go/main/BuildAPI";
import * as RunAPI from "../wailsjs/go/main/RunAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { type ReviewStoreState, createReviewStore } from "./review";

const store = createReviewStore({
  on: (event, cb) => EventsOn(event, cb as (v: unknown) => void),
  stop: () => BuildAPI.StopReview(),
  forget: (runId) => void RunAPI.ForgetRun(runId),
});

export type ReviewState = ReviewStoreState;

/** The review the app follows, re-read whenever it changes. */
export function useReview(): ReviewState {
  const [, bump] = useState(0);
  useEffect(() => store.subscribe(() => bump((n) => n + 1)), []);
  return store.state();
}

/** Reviews the QuickAgent in dir (BuildAPI.ReviewLocally), asking the
 *  registry at registry for its public policy and server records. Resolves
 *  true once the review has begun and its dialog is up, false when the
 *  person said No in the native dialog; rejects with Go's words if it never
 *  began (no key, another job under way). */
export const startReview = (registry: string, dir: string): Promise<boolean> =>
  store.start(dir, () => BuildAPI.ReviewLocally(registry, dir));

export const stopReview = (): Promise<void> => store.stop();
export const showReview = (): void => store.show();
export const dismissReview = (): void => store.dismiss();
