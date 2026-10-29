// A persistent agent's own page: three panes over the whole content area.
// Left, full height, the chat in a terminal's manner: the person's lines
// after a ">", the agent's as markdown in mono, a turn's tool calls folded
// to one dim line, the runner's notes dim, what the agent did on its own
// folded (AgentTermBlocks), a prompt line with Send and Add file, Send
// giving way to Stop while the agent is on a turn. Top right, the usage
// panel; bottom right, the roster, a switch per entry the person may
// change. The chat is the agent's store's, so it goes on while the person is
// on another page; its last lines come back from the transcript.
import { useEffect, useRef, useState } from "react";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import type { app } from "../../wailsjs/go/models";
import { sendToAgent, startAgent, stopStart, stopTurn, updateAgent, useAgentChat, useAgents } from "../agents";
import { AGENT_STATES, agentStateWords, isLive, pendingChange, runningInBackground, showsStop, termBlocks } from "../persistentAgents";
import { agentUsageRows } from "../usage";
import { Button, ErrorBox } from "../ui";
import { StartFailure } from "./StartFailure";
import { AgentTermBlocks } from "./AgentTerm";
import { AgentConfigModal } from "./SettingsPersistentAgents";
import { ConfigCards } from "./ConfigCards";

export default function PersistentAgentPage({ name, registry }: { name: string; registry: string }) {
  const agents = useAgents();
  const agent = agents.find((a) => a.name === name) ?? null;
  const rec = useAgentChat(name);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [configuring, setConfiguring] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [now, setNow] = useState(Date.now());
  const scroller = useRef<HTMLDivElement>(null);
  const blocks = termBlocks(rec.chat);
  const inBackground = runningInBackground(blocks);

  // The figures follow the agent: read again while it runs.
  useEffect(() => {
    const t = setInterval(() => {
      setNow(Date.now());
      if (agent && isLive(agent)) PersistentAPI.Get(name).then(updateAgent).catch(() => {});
    }, 5000);
    return () => clearInterval(t);
  }, [name, agent?.state]);

  // The newest line in view as the conversation grows.
  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [rec.chat.lines.length, rec.chat.streaming]);

  const act = async (f: () => Promise<app.PersistentAgentView>) => {
    setBusy(true);
    setError(null);
    try {
      updateAgent(await f());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  // Start waits for the whole start, which can take minutes; Stop, while it
  // starts, cancels it, and is not held back by the busy start.
  const start = async () => {
    setBusy(true);
    setError(null);
    setError(await startAgent(name));
    setBusy(false);
  };
  const cancelStart = async () => {
    setError(null);
    setError(await stopStart(name));
  };

  if (!agent) return <p className="muted">Reading the agent…</p>;
  const running = isLive(agent);
  const answering = rec.sending || rec.chat.busy;
  const send = async () => {
    const text = draft.trim();
    if (!text || answering || !running) return;
    setDraft("");
    await sendToAgent(name, text);
  };
  // Stop ends the turn the agent is on, the person's or one it runs on its
  // own; the agent keeps running and takes the next message.
  const stop = async () => {
    setStopping(true);
    setError(await stopTurn(name));
    setStopping(false);
  };
  const addFiles = async () => {
    setError(null);
    setNote(null);
    try {
      const written = await PersistentAPI.AddFiles(name);
      if (written?.length) setNote(`Added to the share folder: ${written.join(", ")}`);
    } catch (e) {
      setError(String(e));
    }
  };
  const status = agent.status;

  return (
    <div className="agent-screen">
      <section className="agent-term" aria-label={`Chat with ${agent.displayName}`}>
        <header className="agent-term-head">
          <div>
            <h1>{agent.displayName}</h1>
            <span className="small muted">
              <span className={`agent-dot ${running ? "on" : ""}`} aria-hidden="true" /> {agentStateWords(agent)}
              {status && ` · segment ${status.segment} · ${status.adapter} with ${status.model}`}
            </span>
          </div>
          <div className="row gap">
            {agent.state === AGENT_STATES.stopped && <Button variant="primary" disabled={busy} onClick={() => void start()}>Start</Button>}
            {agent.state === AGENT_STATES.starting && <Button data-stop-start onClick={() => void cancelStart()}>Stop</Button>}
            {running && <Button disabled={busy} onClick={() => act(() => PersistentAPI.Stop(name))}>Stop</Button>}
            <Button variant="ghost" onClick={() => setConfiguring(true)}>Configure</Button>
          </div>
        </header>
        <StartFailure agent={agent} />
        <div className="agent-term-scroll" ref={scroller} role="log" aria-live="polite">
          {blocks.length === 0 && !rec.chat.streaming && (
            <p className="agent-term-note">{running ? "Say hello to start the conversation." : "Start the agent to talk to it. The conversation picks up where it left off."}</p>
          )}
          <AgentTermBlocks blocks={blocks} />
          {rec.chat.streaming && !inBackground && <div className="agent-term-agent streaming">{rec.chat.streaming}</div>}
          {answering && !rec.chat.streaming && !inBackground && <div className="agent-term-note">thinking…</div>}
        </div>
        {pendingChange(agent) && running && <p className="agent-term-pending">Applying your change at the next turn.</p>}
        {(rec.error || error) && <ErrorBox error={rec.error || error} />}
        {note && <p className="small muted">{note}</p>}
        <form
          className="agent-term-prompt"
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          <span aria-hidden="true">&gt;</span>
          <textarea
            aria-label="Message"
            rows={2}
            value={draft}
            disabled={!running}
            placeholder={running ? "Message — Enter sends, Shift+Enter is a new line" : "Start the agent to send a message"}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send();
              }
            }}
          />
          {showsStop(agent, answering) ? (
            <Button type="button" busy={stopping} busyLabel="Stopping…" onClick={() => void stop()} title="Stop this turn. The agent keeps running.">
              Stop
            </Button>
          ) : (
            <Button type="submit" variant="primary" disabled={!running || !draft.trim()}>Send</Button>
          )}
          <Button type="button" onClick={addFiles} title="Copy files into the agent's share folder">Add file</Button>
        </form>
      </section>
      <aside className="agent-usage" aria-label="Usage">
        <h2>Usage</h2>
        <dl>
          {agentUsageRows({
            segment: status ? { inputTokens: status.inputTokens, outputTokens: status.outputTokens, totalTokens: status.totalTokens, costUsd: status.costUsd, userTurns: status.userTurns, toolCalls: status.toolCalls, since: status.since } : undefined,
            lastTurnTokens: rec.chat.lastTurnTokens,
            contextWindow: 0,
            contextUsed: 0,
            lifetime: agent.lifetime,
          }, now).map((r) => (
            <div key={r.label} className="agent-usage-row"><dt>{r.label}</dt><dd>{r.value}</dd></div>
          ))}
          <div className="agent-usage-row"><dt>Volumes</dt><dd title={`${agent.volumes.session}\n${agent.volumes.data}`}>its conversation and its files, kept between segments</dd></div>
        </dl>
      </aside>
      <aside className="agent-roster" aria-label="What it has">
        <h2>What it has</h2>
        <ConfigCards agent={agent} onChanged={updateAgent} onError={setError} withAdds={false} />
      </aside>
      {configuring && <AgentConfigModal name={name} registry={registry} onClose={() => setConfiguring(false)} />}
    </div>
  );
}
