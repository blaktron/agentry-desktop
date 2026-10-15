import { install, main } from "../../../wailsjs/go/models";
import { CardFrame } from "../../cards";
import { latestRunFor, pinRecord, type Pin } from "../../pins";
import { howItEnded, whenRan, whereRan } from "../../recentRuns";

// A Pinned card on Home: up to four pinned QuickAgents, one row each,
// in the order Go placed them. A row names the QuickAgent (its page, or the
// Library for a folder install), its version and update, and how its latest
// run ended, on this computer or In Agentry, or that it has not run yet.
// Open latest run opens the run page; Run opens the usual run setup. A pin
// whose QuickAgent is no longer installed here says so and offers Unpin.
// The card opens the Library. Nothing a run wrote is shown here.
// The launch check's answer for one record: its whole key, as the Library
// matches it.
const updateFor = (updates: install.UpdateInfo[], rec: install.Item) =>
  updates.find((u) => u.updateAvailable && u.key.registry === rec.registry && u.key.namespace === rec.namespace
    && u.key.name === rec.name && u.key.target === rec.target && u.key.dir === rec.dir);

export function PinnedCard({ pins, records, updates, runs, now, onOpenLibrary, onOpenAgent, onOpenRun, onRun, onUnpin }: {
  pins: Pin[];
  records: install.Item[] | null;
  updates: install.UpdateInfo[];
  // Recent runs, newest first, as RunAPI.RecentRuns merges them; null
  // while they are read.
  runs: main.RecentRunView[] | null;
  now: Date;
  onOpenLibrary: () => void;
  onOpenAgent: (registry: string, ns: string, name: string) => void;
  onOpenRun: (row: main.RecentRunView) => void;
  onRun: (rec: install.Item) => void;
  onUnpin: (pin: Pin) => void;
}) {
  return (
    <CardFrame title="Pinned" onOpen={onOpenLibrary} className="home-card" status={records ? null : "Reading the Library…"}>
      {records && (
        <ul className="home-card-list pinned-list" data-pinned>
          {pins.map((pin) => {
            const rec = pinRecord(records, pin);
            const run = runs ? latestRunFor(runs, pin) : undefined;
            const update = rec && updateFor(updates, rec);
            return (
              <li key={`${pin.registry ?? ""} ${pin.ref}`} data-pin={pin.ref}>
                <div>
                  {rec
                    ? <button type="button" className="link build-way-part mono" onClick={() => (rec.local ? onOpenLibrary() : onOpenAgent(rec.origin, rec.namespace, rec.name))}>{pin.ref}</button>
                    : <span className="mono">{pin.ref}</span>}
                  {rec && <span className="muted small"> v{rec.version}</span>}
                  {update && <span className="badge warn" style={{ marginLeft: 6 }}>update: {update.latest}</span>}
                </div>
                {rec
                  ? (
                    <div className="muted small" data-pin-run={run?.id ?? ""}>
                      {run
                        ? <>{[whereRan(run), whenRan(run, now)].filter(Boolean).join(" · ")} · <span className={`recent-run-end tone-${run.tone || "warn"}`}>{howItEnded(run)}</span></>
                        : runs ? "Not run yet" : "Reading its latest run…"}
                    </div>
                  )
                  : <div className="muted small">No longer installed on this computer.</div>}
                <div className="pinned-actions small">
                  {rec && run && <button type="button" className="link build-way-part" onClick={() => onOpenRun(run)}>Open latest run</button>}
                  {rec && <button type="button" className="link build-way-part" onClick={() => onRun(rec)}>Run</button>}
                  {!rec && <button type="button" className="link build-way-part" onClick={() => onUnpin(pin)}>Unpin</button>}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </CardFrame>
  );
}
