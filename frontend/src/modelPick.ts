// The Model dropdowns, as logic: what each one offers (the default it starts
// on, then every saved model), which are greyed and why, and what a pick
// blocks. Run setup, the schedule form, a persistent agent's settings and a
// runhost's agent cards share it. Pure, so the pages and node's test runner
// share it.

/** One saved model as a dropdown is given it: this computer's
 *  (run.ModelChoice) or a runhost's (remote.SavedModel). refusal is why a
 *  run on it would be refused now; empty when it can be used. */
export type PickModel = {
  id: string;
  name: string;
  provider: string;
  model: string;
  default?: boolean;
  refusal?: string;
};

/** One option of a Model dropdown. value "" is the default. */
export type PickOption = { value: string; label: string; disabled: boolean; reason: string };

/** The default's option label: "Default (Haiku)", or "Default" when the
 *  list names no default. */
export function defaultLabel(models: PickModel[]): string {
  const d = models.find((m) => m.default);
  return d ? `Default (${d.name})` : "Default";
}

/** A saved model as an option names it: its name, then provider/model. */
export function modelLabel(m: PickModel): string {
  return `${m.name} — ${m.provider}/${m.model}`;
}

/** The options of a Model dropdown: the default first, then every saved
 *  model in the list's order, each one that cannot be used greyed with its
 *  reason. The default is greyed with the default model's own reason. */
export function modelOptions(models: PickModel[]): PickOption[] {
  const d = models.find((m) => m.default);
  const out: PickOption[] = [{ value: "", label: defaultLabel(models), disabled: !!d?.refusal, reason: d?.refusal ?? "" }];
  for (const m of models) {
    const reason = m.refusal ?? "";
    out.push({ value: m.id, label: reason ? `${modelLabel(m)} (cannot be used now)` : modelLabel(m), disabled: !!reason, reason });
  }
  return out;
}

/** What a pick stops: a saved model that is gone, or one that cannot be
 *  used now, in a sentence; "" when the run may start on it. The default
 *  blocks nothing here: a run on it meets its own refusal where it runs. */
export function pickBlocker(models: PickModel[], pick: string): string {
  if (!pick) return "";
  const m = models.find((x) => x.id === pick);
  if (!m) return "The saved model this setup picked is no longer on this computer: pick another in the Model list.";
  if (m.refusal) return `${m.name} cannot be used now. ${m.refusal}`;
  return "";
}

/** The line under a dropdown: the picked model's reason when it is greyed,
 *  else which saved model the default is now. */
export function pickLine(models: PickModel[], pick: string): string {
  const opts = modelOptions(models);
  const o = opts.find((x) => x.value === pick);
  if (o?.reason) return o.reason;
  if (!pick) {
    const d = models.find((m) => m.default);
    return d ? `Runs on your default saved model, now ${d.name} (${d.provider}/${d.model}).` : "";
  }
  return "";
}

/** The model a record names, as a line says it: the saved model's name,
 *  "the default (Haiku)" for one that follows it, or "" when nothing is
 *  known. */
export function modelSaid(models: PickModel[], id: string): string {
  if (!id || id === "default") {
    const d = models.find((m) => m.default);
    return d ? `the default (${d.name})` : "the default";
  }
  return models.find((m) => m.id === id)?.name ?? "a saved model no longer kept";
}

/** The greyed saved models, each with its reason, as the lines under a
 *  dropdown list them. */
export function greyedLines(models: PickModel[]): { id: string; line: string }[] {
  return models.filter((m) => m.refusal).map((m) => ({ id: m.id, line: `${modelLabel(m)}: ${m.refusal}` }));
}
