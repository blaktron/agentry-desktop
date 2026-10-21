// A persistent agent's roster, as its page and its Configuration draw it:
// the entries grouped by kind, each with who granted it, its state and its
// consent sentence, a switch on each the person may change, and Open in
// browser on a port the agent publishes now. A server the person added
// shows its install state (a failed one with the reason and Retry) and its
// "Can see your share folder" switch, disabled with the reason on a server
// the share is never given to.
import { useState } from "react";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import type { app } from "../../wailsjs/go/models";
import { SHARE_SWITCH_LABEL, portOpenable, retryChanges, rosterGroups, rosterStateWords, serverShareSwitch, serverState } from "../persistentAgents";

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
  // Retry switches the failed server off, then on: its install starts
  // again, at once while the agent runs, else at its next start.
  const retry = async (id: string) => {
    setBusy(id);
    onError(null);
    try {
      let v: app.PersistentAgentView | null = null;
      for (const c of retryChanges(id)) v = await PersistentAPI.Configure(agent.name, c as app.PersistentChange);
      if (v) onChanged(v);
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
          {g.entries.map((e) => {
            const server = e.kind === "server" ? serverState(agent, e) : null;
            const share = serverShareSwitch(e);
            return (
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
                    <strong>{e.name}</strong> <span className="small muted">· {e.grantedBy === "you" ? "added by you" : "built-in"} · {server ? server.words : rosterStateWords(e)}</span>
                    <br />
                    <span className="small">{e.line}</span>
                  </span>
                </label>
                {server?.retry && (
                  <div className="agent-roster-failed">
                    <span className="small" role="alert">Could not be installed: {server.reason}</span>
                    <button type="button" className="btn ghost small" disabled={busy !== null} onClick={() => void retry(e.id)}>Retry</button>
                  </div>
                )}
                {share && (
                  <label className="check-row agent-roster-share">
                    <input
                      type="checkbox"
                      role="switch"
                      aria-label={`${e.name}: ${SHARE_SWITCH_LABEL}: ${share.on ? "on" : "off"}`}
                      checked={share.on}
                      disabled={busy !== null || share.disabled}
                      onChange={(ev) => void flip(share.id, ev.target.checked)}
                    />
                    <span>
                      {SHARE_SWITCH_LABEL}
                      {share.reason && (
                        <>
                          <br />
                          <span className="small muted">{share.reason}</span>
                        </>
                      )}
                    </span>
                  </label>
                )}
                {portOpenable(agent, e) && (
                  <button type="button" className="btn ghost small" onClick={() => open(e.id)}>Open in browser</button>
                )}
              </div>
            );
          })}
        </section>
      ))}
    </div>
  );
}
