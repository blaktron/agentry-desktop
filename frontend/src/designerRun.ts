// The Report Designer's run, on this computer or In Agentry, followed in its
// run window (DesignerFlow, mounted by App above the pages) through the
// built-ins' job store (builtinJob.ts): Design with AI hands the run over
// once it has begun and closes, and the run goes on in the window or its card
// while the person uses the rest of the app. What a run on this computer made
// waits in the window for Add to my library; the Report design page reads the
// library again whenever a run lands or a design is added.
import { useEffect, useState } from "react";
import * as DesignerAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/designerapi";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { type BuiltinJobState, createBuiltinJobStore } from "./builtinJob";
import type { DesignerJob } from "./designer";
import { EventsOn } from "./host";

/** What the window says of the run: its registry, its job, and what it was
 *  given to read. */
export type DesignerMeta = { registry: string; job: DesignerJob; images: number; urls: number; document: boolean };

const store = createBuiltinJobStore<app.DesignerResultView, app.DesignerHostedView, DesignerMeta>({
  on: (event, cb) => EventsOn(event, cb as (v: unknown) => void),
  logEvent: "designer:log",
  progressEvent: "designer:progress",
  hostedEvent: "designer:hosted",
  stop: () => DesignerAPI.StopHere(),
  cancel: () => DesignerAPI.CancelHosted(),
  applied: (h) => h.applied,
});

export type DesignerRunState = BuiltinJobState<app.DesignerResultView, app.DesignerHostedView, DesignerMeta>;

/** The Report Designer's run the app follows, re-read whenever it changes. */
export function useDesignerRun(): DesignerRunState {
  const [, bump] = useState(0);
  useEffect(() => store.subscribe(() => bump((n) => n + 1)), []);
  return store.state();
}

/** Runs the Report Designer on this computer (DesignerAPI.RunHere):
 *  resolves once it has begun, rejects with Go's words if it never began. */
export const startDesignerHere = (meta: DesignerMeta, req: app.DesignerRequest): Promise<void> =>
  store.startHere(meta, () => DesignerAPI.RunHere(meta.registry, req));

/** Follows a Report Designer run In Agentry: in its window for one just
 *  started, as its card for one found under way. */
export const followDesignerHosted = (meta: DesignerMeta, h: app.DesignerHostedView, card = false): void => store.followHosted(meta, h, card);

/** Brings up the window of the run In Agentry h, following it first. */
export function showDesignerHosted(meta: DesignerMeta, h: app.DesignerHostedView): void {
  if (store.follows(h.id)) store.show();
  else store.followHosted(meta, h, false);
}

/** A design the run made was added to the library. */
export const designerSaved = (): void => store.markLanded();

export const stopDesignerRun = (): Promise<void> => store.stop();
export const cancelDesignerRun = (): Promise<void> => store.cancel();
export const showDesignerRun = (): void => store.show();
export const dismissDesignerRun = (): void => store.dismiss();
