// A persistent agent's roster, as its page and its Configuration draw it:
// the entries grouped by kind, each with who granted it, its state and its
// consent sentence, a switch on each the person may change, and Open in
// browser on a port the agent publishes now.
import { useState } from "react";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import type { app } from "../../wailsjs/go/models";
import { portOpenable, rosterGroups, rosterStateWords } from "../persistentAgents";

/** The roster's entries grouped by kind, a switch on each the person may
 *  change. A switch on what they have not agreed to on this computer asks
 *  first, in Go's native dialog. */
export function RosterList({ agent, onChanged, onError }: {
  agent: app.PersistentAgentView;
  onChanged: (v: app.PersistentAgentView) => void;
  onError: (e: string | null) => void;
}) {
  const [busy, setBusy] = useState<string | null>(null);
  const flip = async (id: string, on: boolean) => {
    setBusy(id);
    onError(null);
    try {
      onChanged(await PersistentAPI.Configure(agent.name, { switch: { id, on } } as app.PersistentChange));
    } catch (e) {
      onError(String(e));
    } finally {
      setBusy(null);
    }
  };
  const open = (id: string) => {
    onError(null);
    PersistentAPI.OpenPort(agent.name, id.replace(/^port:/, "")).catch((e) => onError(String(e)));
  };
  return (
    <div className="agent-roster-list">
      {rosterGroups(agent.roster).map((g) => (
        <section key={g.title}>
          <h3>{g.title}</h3>
          {g.entries.map((e) => (
            <div key={e.id} className={`agent-roster-entry state-${e.state}`}>
              <label className="check-row">
                {!e.fixed && (
                  <input
                    type="checkbox"
                    role="switch"
                    aria-label={`${e.name}: ${e.on ? "on" : "off"}`}
                    checked={e.on}
                    disabled={busy !== null || e.state === "needs-sign-in"}
                    onChange={(ev) => void flip(e.id, ev.target.checked)}
                  />
                )}
                <span>
                  <strong>{e.name}</strong> <span className="small muted">· {e.grantedBy === "you" ? "added by you" : "built-in"} · {rosterStateWords(e)}</span>
                  <br />
                  <span className="small">{e.line}</span>
                </span>
              </label>
              {portOpenable(agent, e) && (
                <button type="button" className="btn ghost small" onClick={() => open(e.id)}>Open in browser</button>
              )}
            </div>
          ))}
        </section>
      ))}
    </div>
  );
}
