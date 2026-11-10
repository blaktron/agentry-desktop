// A runhost schedule's Model list, as logic: which option it starts on and
// what a pick asks the runhost. A schedule names one of the runhost's saved
// models by its id, "default" for the runhost's default at each run, or
// nothing when it carries the model of the device that made it. Pure, so the
// card and node's test runner share it.

import { modelSaid, type PickModel, type PickOption } from "./modelPick";

/** The option a schedule made with a device's model starts on. */
export const MADE_WITH = "made-with";

/** The option a schedule's Model list starts on: "" for the runhost's
 *  default, the saved model's id, or MADE_WITH. */
export function scheduleValue(model: string | undefined): string {
  if (!model) return MADE_WITH;
  return model === "default" ? "" : model;
}

/** The option the list offers before the default when the schedule's model
 *  is none of the runhost's: the model it was made with, or a saved model
 *  the runhost no longer holds. Greyed: it can be left, never chosen again. */
export function scheduleExtraOption(models: PickModel[], model: string | undefined): PickOption | null {
  if (!model) return { value: MADE_WITH, label: "The model it was made with", disabled: true, reason: "It runs on the model this computer gave it when it was made; choose one the runhost holds to move it." };
  if (model !== "default" && !models.some((m) => m.id === model)) {
    return { value: model, label: "A saved model the runhost no longer holds", disabled: true, reason: "Its next run is refused until you choose one the runhost holds." };
  }
  return null;
}

/** What a pick asks the runhost: the saved model's id, "" for its default,
 *  or null when the pick is the option the schedule already has. */
export function scheduleMove(current: string | undefined, pick: string): string | null {
  if (pick === MADE_WITH || pick === scheduleValue(current)) return null;
  return pick;
}

/** A schedule's model in a sentence, where no list is drawn: the saved
 *  model's name or the default's once the runhost's list is read. */
export function scheduleModelSaid(models: PickModel[] | null, model: string | undefined): string {
  if (!model) return "the model it was made with";
  if (models) return modelSaid(models, model);
  return model === "default" ? "the default" : "a saved model";
}
