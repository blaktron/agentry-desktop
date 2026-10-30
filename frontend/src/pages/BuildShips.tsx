// The Build tab's Knowledge section for packs a QuickAgent ships: the
// person's own packs kept here or in Agentry, and the shared packs of an
// organization they administer when the QuickAgent is in that
// organization's namespace. Up to five can be ticked. A ticked pack goes into
// the release, unencrypted; each one is read again from where it is kept
// every time the form is saved, and Submit asks the publisher's confirmation
// of each in a native dialog the page cannot answer. The rules the rows draw
// are shipKnowledge.ts's.
import { useEffect, useState } from "react";
import * as BuildAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/buildapi";
import * as build from "../../bindings/github.com/blaktron/agentry-desktop/internal/build/models";
import { RevealText } from "../reveal";
import { Notice } from "../ui";
import {
  SHIP_EXPLAINER, afterDraftNote, folderOwn, isTicked, optionKey, shipSize, shipTotalLine, tick, tickProblem, untick, type ShipChange, type ShipOption,
} from "../shipKnowledge";

export function BuildShips({ registry, m, onChange, draftShipped, notes }: {
  registry: string;
  m: build.Model;
  /** The form takes the new ships. */
  onChange: (change: ShipChange) => void;
  /** The packs the draft in the form shipped, or null when the form holds no draft. */
  draftShipped: string[] | null;
  /** What Edit could not carry over: a pack the previous version shipped that is gone. */
  notes: string[];
}) {
  const [options, setOptions] = useState<ShipOption[] | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    BuildAPI.ShipOptions(registry)
      .then((o) => { if (live) setOptions((o ?? []) as ShipOption[]); })
      .catch((e) => { if (live) { setOptions([]); setProblem(`Your packs could not be listed: ${String(e)}`); } });
    return () => { live = false; };
  }, [registry]);

  const names = m.knowledgeShips ?? [];
  const own = folderOwn(m);
  const listed = options ?? [];
  // A ticked pack no pack of the person's stands behind any more.
  const orphans = (m.ships ?? []).filter((s) => !listed.some((o) => optionKey(o) === optionKey(s)));

  function toggle(o: ShipOption, on: boolean) {
    if (!on) {
      onChange(untick(m, o.name));
      return;
    }
    const why = tickProblem(m, o, m.namespace);
    if (why) {
      setProblem(`${o.name} cannot be ticked: ${why}.`);
      return;
    }
    setProblem(null);
    onChange(tick(m, o, m.namespace));
  }

  return (
    <div className="card" style={{ marginTop: 12 }} role="group" aria-label="Knowledge it ships">
      <strong className="small">Knowledge it ships</strong>
      <p className="muted small" style={{ margin: "4px 0" }}>{SHIP_EXPLAINER}</p>
      {notes.map((n) => <Notice key={n} tone="warn">{n}</Notice>)}
      {options === null && <p className="muted small">Listing your packs…</p>}
      {options !== null && listed.length === 0 && orphans.length === 0 && own.length === 0 && (
        <p className="muted small">You keep no knowledge packs to ship. Create one under Knowledge, then tick it here.</p>
      )}
      <fieldset className="describe-knowledge">
        <legend className="sr-only">Knowledge packs to ship with the QuickAgent</legend>
        {listed.map((o) => {
          const ticked = isTicked(m, o);
          const why = ticked ? null : tickProblem(m, o, m.namespace);
          const note = ticked && draftShipped !== null ? afterDraftNote(m, o.name, draftShipped) : null;
          return (
            <label key={optionKey(o)} className="small" style={{ display: "block", marginTop: 4 }}>
              <input type="checkbox" checked={ticked} disabled={!ticked && why !== null} onChange={(e) => toggle(o, e.target.checked)} />{" "}
              <span className="mono"><RevealText text={o.name} /></span>
              {o.kind === "org" && o.org && <span className="muted"> shared by <RevealText text={o.org} /></span>}
              <span className="muted"> — {o.pages} {o.pages === 1 ? "page" : "pages"}, {shipSize(o.bytes)}</span>
              {why && <span className="muted"> ({why})</span>}
              {note && <span className="muted"> ({note})</span>}
            </label>
          );
        })}
        {orphans.map((s) => (
          <label key={optionKey(s)} className="small" style={{ display: "block", marginTop: 4 }}>
            <input type="checkbox" checked onChange={() => onChange(untick(m, s.name))} />{" "}
            <span className="mono"><RevealText text={s.name} /></span>
            <span className="muted"> (not among your packs now: untick it, or save will fail)</span>
          </label>
        ))}
        {own.map((n) => (
          <label key={`own:${n}`} className="small" style={{ display: "block", marginTop: 4 }}>
            <input type="checkbox" checked onChange={() => onChange(untick(m, n))} />{" "}
            <span className="mono"><RevealText text={n} /></span>
            <span className="muted"> (the files in this QuickAgent&apos;s folder, kept as they are)</span>
          </label>
        ))}
      </fieldset>
      <p className="small muted" style={{ marginBottom: 0 }}>{shipTotalLine(m)}</p>
      {problem && <Notice tone="warn">{problem}</Notice>}
      {names.length > 0 && <p className="small muted">Submit asks you to confirm {names.length === 1 ? "this pack" : "each pack"}: it holds no private data.</p>}
    </div>
  );
}
