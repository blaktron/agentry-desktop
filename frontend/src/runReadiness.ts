// What this computer's setup means for the run setup: which setup, if any,
// the panel shows in place before a run on this computer can start, the
// sentence the list above Run carries for it, and the line beside the Run
// button that says why it is off. Pure, with a type-only import, so node's
// own test runner can hold it without a browser.
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";

/** The setup the panel embeds: the container engine's while none answers,
 *  then the model provider's while no way to pay for the model is set up.
 *  null when both are ready, while the status is still being read (the
 *  blocker says so) and for a run In Agentry, which needs nothing from this
 *  machine. */
export type SetupGap = "engine" | "model" | null;

export function setupGap(setup: app.SetupStatusView | null, inAgentry: boolean): SetupGap {
  if (inAgentry || !setup) return null;
  if (!setup.dockerOk) return "engine";
  if (!setup.modelReady) return "model";
  return null;
}

/** The second half of every gap's sentence: the setup is in this panel, so
 *  nothing entered is lost by taking it. */
export const KEPT = "Set it up in this panel; everything you entered stays.";

/** What the engine's gap says: that no engine answers, and the way through.
 *  The engine's own words are the card's, drawn under this line, not
 *  repeated here. */
export function engineGapLine(hostedOffered: boolean): string {
  return `No container engine answers on this computer, and a QuickAgent run here needs one. ${KEPT}${hostedOffered ? " Or run this release In Agentry." : ""}`;
}

/** What the model's gap says: Go's sentence for what is missing, else a
 *  plain one, and the way through. */
export function modelGapLine(setup: app.SetupStatusView, hostedOffered: boolean): string {
  const gap = setup.modelGap || "No model provider is set up for runs on this computer.";
  return `${gap} ${KEPT}${hostedOffered ? " Or run this release In Agentry." : ""}`;
}

/** The blocker a gap puts in the list above Run, null when there is none. */
export function gapBlocker(setup: app.SetupStatusView | null, inAgentry: boolean, hostedOffered: boolean): string | null {
  if (!setup) return null;
  switch (setupGap(setup, inAgentry)) {
    case "engine":
      return engineGapLine(hostedOffered);
    case "model":
      return modelGapLine(setup, hostedOffered);
    default:
      return null;
  }
}

/** The line under the Run button while it is off: the first blocker, and
 *  how many more the list above holds, so the reason is beside the button
 *  and not only in a tooltip or further up the page. Nothing while the run
 *  is starting, and nothing when the button is on. */
export function runButtonWhy(blockers: string[], busy: boolean): string | null {
  if (busy || blockers.length === 0) return null;
  const more = blockers.length - 1;
  if (more === 0) return blockers[0];
  return `${blockers[0]} And ${more === 1 ? "one more thing" : `${more} more things`}, listed above.`;
}
