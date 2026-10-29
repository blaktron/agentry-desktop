import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../../wailsjs/go/app/RunAPI";
import { agentNamed, pickHomeAgent, sendToAgent, startAgent, stopTurn, useAgentChat, useAgents, useHomeAgent } from "../../agents";
import { CardFrame } from "../../cards";
import { agentCardHint, helperSuperseded, homeAgentDrawn } from "../../homeAgent";
import { AGENT_STATES, MAIN_AGENT, isLive, mainStatusLine, runningInBackground, startFailureLines, termBlocks } from "../../persistentAgents";
import { Button } from "../../ui";
import { AgentTermBlocks } from "../AgentTerm";

// Home's agent card: a chat with a persistent agent, in a terminal's manner,
// small. It runs the agent the person gave it, here in its switcher, in
// Settings → Home or in the Welcome, while that agent is on, else the
// Assistant or Agentry Helper, whichever is on; Home draws no card while
// none of them is (homeAgentDrawn). Opening Home reads
// the agent's state and its last lines and starts nothing: its containers
// start when the person sends the first message, which says "Starting…"
// meanwhile. Without a model set up it says so and links to Settings →
// Provider. The chat is the agents' store's, so it goes on while the person
// is on another page.
export function AgentCard({ onOpenSettings, onOpenAgent, onSetUpModel }: {
  // Agentry Helper's title: Settings → Persistent Agents, where it is listed.
  onOpenSettings: () => void;
  // Any other agent's title: its own page.
  onOpenAgent: (name: string) => void;
  onSetUpModel: () => void;
}) {
  const agents = useAgents();
  const chosen = useHomeAgent();
  const name = homeAgentDrawn(chosen ?? MAIN_AGENT, agents) ?? MAIN_AGENT;
  const agent = agents.find((a) => a.name === name) ?? null;
  const rec = useAgentChat(name);
  const [modelSet, setModelSet] = useState<boolean | null>(null);
  const [draft, setDraft] = useState("");
  const [starting, setStarting] = useState(false);
  const [picking, setPicking] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const blocks = termBlocks(rec.chat);
  const inBackground = runningInBackground(blocks);
  const called = agent?.displayName || "Agentry Helper";

  // Whether a model is set up: read, never written.
  useEffect(() => {
    let live = true;
    RunAPI.SetupStatus().then((s) => { if (live) setModelSet(s.providerSetUp); }).catch(() => { if (live) setModelSet(true); });
    return () => { live = false; };
  }, []);

  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [rec.chat.lines.length, rec.chat.streaming, starting]);

  // A new agent on the card starts with nothing said about the last one.
  useEffect(() => { setError(null); }, [name]);

  const running = !!agent && isLive(agent);
  const answering = starting || rec.sending || rec.chat.busy;
  const elsewhere = agent?.state === AGENT_STATES.elsewhere;

  const send = async () => {
    const text = draft.trim();
    if (!text || answering || !agent) return;
    setError(null);
    if (!running) {
      // The first message starts the agent; Go asks natively first where its
      // folders have not been agreed to on this computer.
      setStarting(true);
      const failed = await startAgent(name);
      setStarting(false);
      const now = agentNamed(name);
      if (failed !== null || now?.state !== AGENT_STATES.running) {
        // A start that failed says why; one stopped from Settings sends
        // nothing, and the message stays to send again.
        const why = now ? startFailureLines(now) : null;
        setError(failed ?? (why ? [why.title, ...why.lines].join("\n") : null));
        return;
      }
    }
    setDraft("");
    await sendToAgent(name, text);
  };

  // Stop ends the turn the agent is on; the agent keeps running.
  const stop = async () => {
    setStopping(true);
    setError(await stopTurn(name));
    setStopping(false);
  };

  const pick = async (next: string) => {
    if (next === name) return;
    setPicking(true);
    setError(await pickHomeAgent(next));
    setPicking(false);
  };

  const noModel = modelSet === false;
  // Agentry Helper is no choice while the Assistant, which supersedes it,
  // is on.
  const choices = helperSuperseded(agents) ? agents.filter((a) => a.name !== MAIN_AGENT) : agents;
  const switcher = choices.length > 1 && (
    <select aria-label="Agent on this card" data-home-agent-switch value={name} disabled={picking || answering}
      onChange={(e) => void pick(e.target.value)}>
      {choices.map((a) => (
        <option key={a.name} value={a.name}>
          {a.displayName}{a.state === AGENT_STATES.disabled ? " (turn on)" : ""}
        </option>
      ))}
    </select>
  );
  return (
    <CardFrame title={called} onOpen={name === MAIN_AGENT ? onOpenSettings : () => onOpenAgent(name)} aside={switcher}
      className="home-card main-card" status={agent ? mainStatusLine(agent) : "Reading the agent…"}>
      <div className="build-way-part main-card-chat" data-home-agent={name}>
        <div className="main-card-scroll" ref={scroller} role="log" aria-live="polite" aria-label={`Chat with ${called}`}>
          {blocks.length === 0 && !starting && !rec.chat.streaming && (
            <p className="agent-term-note">{noModel || !agent ? "" : agentCardHint(agent)}</p>
          )}
          <AgentTermBlocks blocks={blocks} />
          {starting && <div className="agent-term-note">{agent?.stage ? `${agent.stage}…` : `Starting ${called}…`}</div>}
          {rec.chat.streaming && !inBackground && <div className="agent-term-agent streaming">{rec.chat.streaming}</div>}
          {!starting && answering && !rec.chat.streaming && !inBackground && <div className="agent-term-note">thinking…</div>}
        </div>
        {noModel && (
          <p className="small" data-main-no-model>
            Set up a model to talk to {called}. <Button variant="link" onClick={onSetUpModel}>Settings → Provider</Button>
          </p>
        )}
        {elsewhere && <p className="small muted">{called} is running in {agent?.holder || "another client"}; you can talk to it here.</p>}
        {(error || rec.error) && <p className="small agent-term-warn" role="alert">{error || rec.error}</p>}
        <form
          className="agent-term-prompt main-card-prompt"
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          <span aria-hidden="true">&gt;</span>
          <input
            type="text"
            aria-label={`Message ${called}`}
            value={draft}
            disabled={noModel || elsewhere || !agent}
            placeholder={noModel ? "Set up a model first" : `Message ${called}`}
            onChange={(e) => setDraft(e.target.value)}
          />
          {answering && running && !starting ? (
            <Button type="button" data-stop-turn disabled={stopping} onClick={() => void stop()} title="Stop this turn. The agent keeps running.">
              {stopping ? "Stopping…" : "Stop"}
            </Button>
          ) : (
            <Button type="submit" variant="primary" disabled={noModel || elsewhere || answering || !draft.trim()}>Send</Button>
          )}
        </form>
      </div>
    </CardFrame>
  );
}
