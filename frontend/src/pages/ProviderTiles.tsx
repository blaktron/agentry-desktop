import React, { useRef } from "react";
import { PROVIDERS, providerChoice, tileOffer } from "../modelSetup";
import { tileAfterKey } from "../providerForm";
import { ProviderLogo } from "../providerLogos";

/** Model setup's provider choice (agentry-desktop#830): one tile per
 *  provider, two rows of four, each with its logo, its name and what it can
 *  be paid with, so every option shows before one is picked. It replaced a
 *  drop-down list and does what picking from it did: onPick resets the form
 *  below for the new provider.
 *
 *  The tiles are one radio group (role="radio", aria-checked): Tab reaches
 *  the chosen tile, or the first when none is, and the arrows, Home and End
 *  move the choice as a radio group's do. A provider only a hand-written
 *  policy names lights no tile, and the line under the tiles says so. */
export function ProviderTiles({ value, onPick, disabled }: {
  value: string;
  onPick: (id: string) => void;
  disabled?: boolean;
}) {
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});
  const ids = PROVIDERS.map((p) => p.id);
  const known = !!providerChoice(value);
  const tabStop = known ? value : ids[0];

  function onKeyDown(e: React.KeyboardEvent) {
    const next = tileAfterKey(ids, value, e.key);
    if (next === null) return;
    e.preventDefault();
    if (next !== value) onPick(next);
    refs.current[next]?.focus();
  }

  return (
    <div className="field">
      <span className="field-label" id="provider-tiles-label">Provider</span>
      <div className="provider-tiles" role="radiogroup" aria-labelledby="provider-tiles-label" onKeyDown={disabled ? undefined : onKeyDown}>
        {PROVIDERS.map((p) => {
          const on = p.id === value;
          return (
            <button
              key={p.id}
              ref={(el) => { refs.current[p.id] = el; }}
              type="button"
              role="radio"
              aria-checked={on}
              aria-label={p.label}
              title={p.label}
              tabIndex={p.id === tabStop ? 0 : -1}
              className={`provider-tile${on ? " on" : ""}`}
              data-provider={p.id}
              disabled={disabled}
              onClick={() => { if (!on) onPick(p.id); }}
            >
              <ProviderLogo provider={p.id} size={32} />
              <span className="provider-tile-name">{p.short}</span>
              <span className="provider-tile-offer">
                {/* Each choice kept on one line: "Your account or" / "API key". */}
                {tileOffer(p).split(" or ").map((part, i) => (
                  <React.Fragment key={i}>{i > 0 && " or "}<span className="nowrap">{part}</span></React.Fragment>
                ))}
              </span>
            </button>
          );
        })}
      </div>
      {!known && value && (
        <span className="muted small field-hint">
          Your runner policy names <span className="mono">{value}</span>, which is not one of these: it is paid by a key, below.
          Pick a tile to use another provider.
        </span>
      )}
    </div>
  );
}
