import React, { useRef } from "react";
import type { app } from "../../wailsjs/go/models";
import { ConfigIcon } from "../configGlyphs";
import { HELPER_SUPERSEDED_LINE, helperSuperseded, isOn, tilesFor, type AgentTile } from "../homeAgent";
import { tileAfterKey } from "../providerForm";
import { AGENT_STATES, MAIN_AGENT } from "../persistentAgents";
import { Mark } from "../ui";

const BUSY_STATES: ReadonlySet<string> = new Set([AGENT_STATES.running, AGENT_STATES.starting, AGENT_STATES.elsewhere]);

function TileGlyph({ glyph }: { glyph: AgentTile["glyph"] }) {
  return glyph === "mark" ? <Mark size={32} /> : <ConfigIcon icon={{ kind: "glyph", key: glyph }} />;
}

/** The persistent agents as tiles, drawn like the model provider tiles: a
 *  glyph, the agent's name and one line on what it does.
 *
 *  "toggle" (the Welcome's Agents step) makes each tile a switch
 *  (aria-pressed) that turns its agent on or off; a running agent's is held
 *  until it stops. A tile that is on shows "Start when Agentry starts" under
 *  it. In either mode Agentry Helper's tile is held off, saying why, while
 *  the Assistant is on, and the Assistant's carries a Recommended mark.
 *  "pick" (Settings → Home) makes the tiles one radio group, the agent Home's
 *  card runs: the arrows, Home and End move the choice as a radio group's
 *  do, and Tab reaches the chosen tile. */
export function AgentTiles(props: {
  agents: app.PersistentAgentView[];
  label: string;
  busy?: boolean;
} & (
  | { mode: "toggle"; onToggle: (name: string, on: boolean) => void; onRunOnStart: (name: string, on: boolean) => void }
  | { mode: "pick"; value: string; onPick: (name: string) => void }
)) {
  const { agents, label, busy } = props;
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});
  const tiles = tilesFor(agents);
  const superseded = helperSuperseded(agents);
  const ids = tiles.map((t) => t.name);
  const labelId = `agent-tiles-${props.mode}`;

  function onKeyDown(e: React.KeyboardEvent) {
    if (props.mode !== "pick") return;
    const next = tileAfterKey(ids, props.value, e.key);
    if (next === null) return;
    e.preventDefault();
    if (next !== props.value) props.onPick(next);
    refs.current[next]?.focus();
  }

  return (
    <div className="field">
      <span className="field-label" id={labelId}>{label}</span>
      <div className={`provider-tiles agent-tiles agent-tiles-${props.mode}`} role={props.mode === "pick" ? "radiogroup" : "group"}
        aria-labelledby={labelId} onKeyDown={busy ? undefined : onKeyDown}>
        {tiles.map((t) => {
          const a = agents.find((x) => x.name === t.name)!;
          const on = props.mode === "pick" ? props.value === t.name : isOn(a);
          // An agent's tile is held while it is running, starting or
          // running elsewhere, as Settings holds its switch: it is stopped
          // before it is turned off. Agentry Helper's is held off while the
          // Assistant is on, which Go would refuse.
          const blocked = t.name === MAIN_AGENT && superseded && !on;
          const held = blocked || (props.mode === "toggle" && BUSY_STATES.has(a.state));
          const tile = (
            <button
              key={t.name}
              ref={(el) => { refs.current[t.name] = el; }}
              type="button"
              {...(props.mode === "pick"
                ? { role: "radio", "aria-checked": on, tabIndex: on || (!ids.includes(props.value) && t.name === ids[0]) ? 0 : -1 }
                : { "aria-pressed": on })}
              aria-label={t.title}
              aria-describedby={`agent-tile-line-${props.mode}-${t.name}`}
              className={`provider-tile agent-tile${on ? " on" : ""}`}
              data-agent-tile={t.name}
              disabled={busy || held}
              onClick={() => {
                if (props.mode === "pick") {
                  if (!on) props.onPick(t.name);
                } else if (!held) {
                  props.onToggle(t.name, !on);
                }
              }}
            >
              <TileGlyph glyph={t.glyph} />
              <span className="provider-tile-name">{t.title}</span>
              {t.recommended && <span className="badge kind" data-agent-recommended>Recommended</span>}
              <span className="provider-tile-offer" id={`agent-tile-line-${props.mode}-${t.name}`}>{blocked ? HELPER_SUPERSEDED_LINE : t.line}</span>
            </button>
          );
          if (props.mode === "pick") return tile;
          return (
            <div key={t.name} className="agent-tile-cell">
              {tile}
              {on && t.name !== MAIN_AGENT && (
                <label className="check-row agent-tile-start">
                  <input type="checkbox" checked={a.runOnStart} disabled={busy} data-agent-run-on-start={t.name}
                    onChange={(e) => props.onRunOnStart(t.name, e.target.checked)} />
                  Start when Agentry starts
                </label>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
