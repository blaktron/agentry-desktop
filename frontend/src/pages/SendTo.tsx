// Send deliverables to: the saved destinations a run on this computer (or each
// fire of a schedule on it) sends its result and deliverables to once it
// succeeds, chosen in the run setup. The list is Settings → Destinations',
// kept by the CLI; a destination with no signing secret is shown and cannot be
// chosen. The QuickAgent's default set preselects its destinations, and the
// choice can be kept as that default. Nothing here is a credential or an
// address beyond its host.
import { Button } from "../ui";
import { chosenLine, defaultLine, destinationLabel } from "../destinations";
import type { destinations } from "../../wailsjs/go/models";

export function SendTo({ saved, chosen, onChange, makeDefault, onMakeDefault, disabled, onOpenSettings }: {
  saved: destinations.View[];
  chosen: string[];
  onChange: (names: string[]) => void;
  makeDefault: boolean;
  onMakeDefault: (on: boolean) => void;
  disabled?: boolean;
  onOpenSettings?: () => void;
}) {
  const toggle = (name: string, on: boolean) => onChange(on ? [...chosen.filter((n) => n !== name), name] : chosen.filter((n) => n !== name));
  return (
    <section aria-labelledby="send-to-heading" data-send-to>
      <h4 id="send-to-heading">Send deliverables to</h4>
      {saved.length === 0 ? (
        <p className="muted small">
          No destinations saved. A destination is a webhook that receives the run&apos;s result and files once it succeeds.
          {onOpenSettings && <> <Button variant="link" onClick={onOpenSettings}>Add one in Settings</Button></>}
        </p>
      ) : (
        <>
          {saved.map((d) => (
            <label key={d.name} className="check-row" data-send-to-option={d.name}>
              <input
                type="checkbox" checked={chosen.includes(d.name)} disabled={disabled || !d.hasSecret}
                onChange={(e) => toggle(d.name, e.target.checked)}
              />
              <span>
                {destinationLabel(d)}
                {!d.hasSecret && <span className="muted small"> — no signing secret kept, so it cannot be chosen</span>}
              </span>
            </label>
          ))}
          <p className="muted small" style={{ margin: "2px 0 6px" }} data-send-to-line>{chosenLine(chosen)}</p>
          {saved.some((d) => defaultLine(d)) && null}
          <label className="check-row">
            <input type="checkbox" checked={makeDefault} disabled={disabled} onChange={(e) => onMakeDefault(e.target.checked)} />
            <span>Choose these every time I run this QuickAgent</span>
          </label>
        </>
      )}
    </section>
  );
}
