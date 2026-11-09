// Add from the library: a dropdown of the library's applications for this
// computer, grouped by category and searchable, each with a check box. The
// applications already on the list are shown checked; checking others and
// pressing Add puts them all on the list at once, nothing approved. Their
// steps then show on each application's card.
import { useEffect, useId, useMemo, useRef, useState } from "react";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as library from "../../bindings/github.com/blaktron/agentry-desktop/internal/library/models";
import { Button } from "../ui";
import { addCheckedLabel, libraryGroups, libraryStatusLine } from "../yourComputer";

export function LibraryPicker({ presets, status, busy, disabled, onAdd }: {
  presets: app.LibraryPreset[];
  status?: library.Status;
  busy: boolean;
  disabled: boolean;
  /** Adds the checked ids; resolves true when they were added. */
  onAdd: (ids: string[]) => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [checked, setChecked] = useState<string[]>([]);
  const panelId = useId();
  const root = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const groups = useMemo(() => libraryGroups(presets, query), [presets, query]);
  const onList = presets.filter((p) => p.onList).length;

  // A click outside or Escape closes the list and keeps what was checked.
  useEffect(() => {
    if (!open) return;
    search.current?.focus();
    const away = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  // An id that went on the list, or left the library, is no longer checked.
  useEffect(() => {
    setChecked((c) => c.filter((id) => presets.some((p) => p.id === id && !p.onList)));
  }, [presets]);

  const toggle = (id: string, on: boolean) => setChecked((c) => (on ? [...c, id] : c.filter((x) => x !== id)));

  async function add() {
    if (await onAdd(checked)) {
      setChecked([]);
      setQuery("");
      setOpen(false);
    }
  }

  return (
    <div className="library-picker" ref={root} data-library-picker>
      <Button onClick={() => setOpen(!open)} disabled={disabled} aria-expanded={open} aria-controls={panelId} data-add-library>
        Add from the library{checked.length > 0 ? ` (${checked.length} checked)` : ""} <span aria-hidden="true">▾</span>
      </Button>
      {open && (
        <div className="library-picker-panel" id={panelId} role="group" aria-label="The library's applications">
          <input type="search" ref={search} value={query} onChange={(e) => setQuery(e.target.value)}
            placeholder="Find an application" aria-label="Find an application" data-library-search />
          <p className="muted small" data-library-status>
            {libraryStatusLine(status, presets.length)}{" "}
            {onList > 0 && `${onList} ${onList === 1 ? "is" : "are"} on your list already. `}
            Adding one puts it on your list with nothing approved.
          </p>
          <div className="library-picker-list">
            {groups.length === 0 && <p className="muted small">No application of the library matches “{query.trim()}”. Add another application sets up any other.</p>}
            {groups.map((g) => (
              <fieldset key={g.category} className="library-picker-group">
                <legend>{g.category}</legend>
                {g.presets.map((p) => {
                  const on = p.onList || checked.includes(p.id);
                  return (
                    <label key={p.id} className={`library-picker-row${p.onList ? " on-list" : ""}`} data-library-preset={p.id}
                      title={`id ${p.id}, server ${p.package}`}>
                      <input type="checkbox" checked={on} disabled={p.onList || busy}
                        onChange={(e) => toggle(p.id, e.target.checked)} />
                      <span className="library-picker-text">
                        <strong>{p.application}</strong>
                        {p.onList && <span className="muted"> · on your list</span>}
                        {p.summary && <span className="muted small library-picker-summary">{p.summary}</span>}
                        {!p.onList && (p.exec.length > 0 || p.asks.length > 0) && (
                          <span className="muted small library-picker-summary">
                            {p.exec.length > 0 && `${p.exec.join(", ")} ${p.exec.length === 1 ? "runs" : "run"} code, asked first. `}
                            {p.asks.length > 0 && `Asks you for ${p.asks.join(", ")}.`}
                          </span>
                        )}
                      </span>
                    </label>
                  );
                })}
              </fieldset>
            ))}
          </div>
          <div className="row library-picker-foot">
            <Button variant="primary" onClick={add} disabled={busy || checked.length === 0} data-library-add>
              {addCheckedLabel(checked.length)}
            </Button>
            {checked.length > 0 && <button type="button" className="link small" onClick={() => setChecked([])} disabled={busy}>Clear</button>}
            <button type="button" className="link small" onClick={() => setOpen(false)}>Close</button>
          </div>
        </div>
      )}
    </div>
  );
}
