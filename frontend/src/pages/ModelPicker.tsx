// A Model dropdown: the default it starts on, then every saved model, the
// ones that cannot be used greyed with the reason. Run setup, the schedule
// form, a persistent agent's settings and a runhost's agent cards draw it.
import { useEffect, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { Select } from "../primitives";
import { greyedLines, modelOptions, pickLine, type PickModel, type PickOption } from "../modelPick";

export function ModelPicker({ models, value, onChange, disabled, label = "Model", hint, testId, extra }: {
  models: PickModel[];
  value: string;
  onChange: (id: string) => void;
  disabled?: boolean;
  label?: string;
  /** A sentence after the picked model's line: what the pick applies to. */
  hint?: string;
  testId?: string;
  /** An option before the default, for a value that is none of the list's
   *  (a runhost schedule's model it was made with); its reason is the line. */
  extra?: PickOption | null;
}) {
  const options = extra ? [extra, ...modelOptions(models)] : modelOptions(models);
  const line = extra && value === extra.value ? extra.reason : pickLine(models, value);
  return (
    <label className="field" data-model-picker={testId ?? ""}>
      <span className="field-label">{label}</span>
      <Select value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled} aria-label={label}>
        {options.map((o) => (
          <option key={o.value || "default"} value={o.value} disabled={o.disabled && o.value !== value} title={o.reason || undefined}>
            {o.label}
          </option>
        ))}
      </Select>
      {(line || hint) && <span className="muted small" data-model-line>{[line, hint].filter(Boolean).join(" ")}</span>}
      {greyedLines(models).length > 0 && (
        <ul className="muted small model-greyed" data-model-greyed>
          {greyedLines(models).map((g) => <li key={g.id}>{g.line}</li>)}
        </ul>
      )}
    </label>
  );
}

/** This computer's saved models as its Model dropdowns offer them, read
 *  again whenever key changes; null until read, [] on a computer that keeps
 *  none a person chose (or when they could not be read). */
export function useModelChoices(key: unknown = 0): PickModel[] | null {
  const [models, setModels] = useState<PickModel[] | null>(null);
  useEffect(() => {
    let live = true;
    Promise.resolve().then(() => RunAPI.ModelChoices()).then((c) => live && setModels(c?.models ?? [])).catch(() => live && setModels([]));
    return () => { live = false; };
  }, [key]);
  return models;
}
