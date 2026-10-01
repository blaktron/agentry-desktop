// The one draft this app is making, here or In Agentry (plan
// builder-rework.md §7.1; #281, #282), kept outside every page. A draft
// takes minutes and Build unmounts on every tab change, so the draft in
// flight, how far it has got, what it has said and the draft or refusal it
// ends with live here. Describe it starts one; the progress dialog and its
// corner card (DraftFlow, mounted by App above the pages, agentry-desktop#410)
// follow it; and whichever page is showing when it lands takes the draft
// (takeLandedDraft). One can be in flight at a time.
import { useEffect, useState } from "react";
import * as BuildAPI from "../wailsjs/go/main/BuildAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { main } from "../wailsjs/go/models";
import type { JobProgress } from "./jobProgress";

export type Where = "agentry" | "here";

/** How far a draft on this computer has got, as Go reports it
 *  (internal/draft's Progress, build:draft-progress): the report every job
 *  carries, and the stage the draft is in. */
export type DraftProgress = JobProgress & {
  stage: "description" | "knowledge" | "drafting" | "checking" | "repair" | "";
};

let where: Where | null = null;
let localInflight: Promise<void> | null = null;
let inflightLog: string[] = [];
let progress: DraftProgress | null = null;
// stopping: Stop was pressed on a draft on this computer; stopped: the
// draft ended after it, so the flow says the person stopped it.
let stopping = false;
let stopped = false;
// hosted is the draft In Agentry this app follows, as its last event said;
// hostedStarting covers the create's round trip.
let hosted: main.HostedDraftView | null = null;
let hostedStarting = false;
let hostedListening = false;
let landed: main.DraftView | null = null;
// drafted names the draft that landed, for the dialog and the card, after
// a page has taken it.
let drafted: string | null = null;
let landedError: string | null = null;
let refusal: main.HostedDraftStartView | null = null;
// shown: the flow is up (as the dialog or the card) until it is dismissed;
// raised counts the times it was asked to open, and asCard says the last
// ask was for the card (a draft In Agentry the app found running).
let shown = false;
let raised = 0;
let asCard = false;
const listeners = new Set<() => void>();
const landers = new Set<() => void>();
const notify = () => listeners.forEach((f) => f());

/** Whether a draft is being made now, here or In Agentry. */
export function draftInFlight(): boolean {
  return localInflight !== null || hostedStarting || (hosted !== null && !hosted.final);
}

/** The draft that finished and no page has taken yet, taken. */
export function takeLandedDraft(): main.DraftView | null {
  const v = landed;
  landed = null;
  return v;
}

/** Tells f whenever a draft lands, so a page can clear what it kept. */
export function whenLanded(f: () => void): void {
  landers.add(f);
}

function land(v: main.DraftView | undefined | null, orError: string) {
  if (v?.model) {
    landed = v;
    drafted = v.model.name;
    landers.forEach((f) => f());
  } else {
    landedError = orError;
  }
}

// follow puts the flow up for a draft made in w: as the dialog for one the
// person just started, as the card for one the app found running.
function follow(w: Where, card = false) {
  where = w;
  shown = true;
  asCard = card;
  raised++;
}

// clear forgets how the last draft ended.
function clear() {
  landedError = null;
  refusal = null;
  drafted = null;
  stopped = false;
  inflightLog = [];
  progress = null;
}

function begin(w: Where) {
  clear();
  landed = null;
  follow(w);
}

function startLocal(registry: string, text: string, hints: string[], packs: string[]) {
  begin("here");
  stopping = false;
  const offLog = EventsOn("build:draft-log", (line: string) => {
    inflightLog = [...inflightLog, line];
    notify();
  });
  const offProgress = EventsOn("build:draft-progress", (p: DraftProgress) => {
    progress = p;
    notify();
  });
  localInflight = BuildAPI.DescribeOnThisComputer(registry, text, hints, packs)
    .then((v) => land(v, "The Drafter returned no draft the form can open."))
    .catch((e) => {
      stopped = stopping;
      landedError = stopped ? "You stopped the draft, so none was made." : String(e);
    })
    .finally(() => {
      offLog();
      offProgress();
      localInflight = null;
      notify();
    });
  notify();
}

// listenHosted follows build:hosted-draft for the app's life: a draft In
// Agentry reports every state change, and its final word carries the draft.
function listenHosted() {
  if (hostedListening) return;
  hostedListening = true;
  EventsOn("build:hosted-draft", (v: main.HostedDraftView) => {
    hosted = v;
    if (v.final) {
      const why = [v.outcomeLine, v.error, v.draftError].filter(Boolean).join(" ");
      land(v.draft, why || "The draft ended with no draft the form can open.");
    }
    notify();
  });
}

function startHosted(registry: string, text: string, hints: string[], packs: string[]) {
  listenHosted();
  begin("agentry");
  hosted = null;
  hostedStarting = true;
  notify();
  BuildAPI.DescribeInAgentry(registry, text, hints, packs)
    .then((v) => {
      if (v.draft) {
        // An event may already have said more than the create's answer.
        if (!hosted || hosted.id !== v.draft.id) hosted = v.draft;
      } else {
        refusal = v;
      }
    })
    .catch((e) => { landedError = String(e); })
    .finally(() => {
      hostedStarting = false;
      notify();
    });
}

/** Starts a draft in the place chosen, and opens its progress dialog.
 *  packs are the knowledge packs it reads first, by name (#497). */
export function startDraft(registry: string, w: Where, text: string, hints: string[], packs: string[] = []) {
  if (w === "agentry") startHosted(registry, text, hints, packs);
  else startLocal(registry, text, hints, packs);
}

/** Follows this account's draft In Agentry after the app was reopened, in
 *  the corner card: App asks as it starts, and Describe it as it opens. */
export function attachHosted(registry: string) {
  if (hosted || hostedStarting) return;
  listenHosted();
  BuildAPI.AttachHostedDraft(registry)
    .then((v) => {
      if (v.draft && !hosted) {
        hosted = v.draft;
        follow("agentry", true);
        notify();
      }
    })
    .catch(() => {});
}

/** Stops the draft on this computer (BuildAPI.StopDraft). */
export function stopDraft(): Promise<void> {
  stopping = true;
  notify();
  return BuildAPI.StopDraft().catch(() => {
    // The draft ended as Stop was pressed: its own draft or error is what
    // the flow shows, not this.
    stopping = false;
    notify();
  });
}

/** Asks Agentry to cancel the draft it is making. */
export function cancelHostedDraft(): Promise<void> {
  return BuildAPI.CancelHostedDraft().then((c) => {
    hosted = c;
    notify();
  });
}

/** Opens the flow's dialog again, from the card or the Describe it page. */
export function showDraft(): void {
  if (!shown) return;
  asCard = false;
  raised++;
  notify();
}

/** Puts the flow away once the draft is over. A draft that landed and no
 *  page took stays for the Build tab, which opens it when it next shows. */
export function dismissDraft(): void {
  if (draftInFlight()) return;
  shown = false;
  clear();
  hosted = null;
  notify();
}

/** The draft as the flow shows it. */
export type DraftState = {
  shown: boolean;
  raised: number;
  asCard: boolean;
  where: Where | null;
  inFlight: boolean;
  local: boolean;
  log: string[];
  progress: DraftProgress | null;
  stopping: boolean;
  /** The draft ended because the person stopped it. */
  stopped: boolean;
  hosted: main.HostedDraftView | null;
  hostedStarting: boolean;
  drafted: string | null;
  error: string | null;
  refusal: main.HostedDraftStartView | null;
};

function snapshot(): DraftState {
  const inFlight = draftInFlight();
  return {
    shown, raised, asCard, where, inFlight, local: localInflight !== null, log: inflightLog, progress, stopping, stopped: !inFlight && stopped, hosted, hostedStarting,
    drafted, error: inFlight ? null : landedError, refusal: inFlight ? null : refusal,
  };
}

/** The draft, redrawn whenever it changes. */
export function useDraft(): DraftState {
  const [, redraw] = useState(0);
  useEffect(() => {
    const f = () => redraw((n) => n + 1);
    listeners.add(f);
    return () => { listeners.delete(f); };
  }, []);
  return snapshot();
}
