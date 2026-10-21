import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../../wailsjs/go/app/RunAPI";
import { AgentMarkdown } from "../../AgentMarkdown";
import { agentNamed, sendToAgent, startAgent, useAgentChat, useAgents } from "../../agents";
import { CardFrame } from "../../cards";
import { AGENT_STATES, MAIN_AGENT, mainStatusLine, startFailureLines, termLines } from "../../persistentAgents";
import { Button } from "../../ui";

// Home's Main card: a chat with Main, Agentry's own agent, in a terminal's
// manner, small. Opening Home reads Main's state and its last lines and
// starts nothing: Main's containers start when the person sends the first
// message, which says "Starting Main…" meanwhile. Without a model set up
// it says so and links to Settings → Provider. Main's chat is the agents'
// store's, so it goes on while the person is on another page.
export function MainCard({ onOpenSettings, onSetUpModel }: {
  // The card's title: Settings → Persistent Agents, where Main is listed.
  onOpenSettings: () => void;
  onSetUpModel: () => void;
}) {
  const agent = useAgents().find((a) => a.name === MAIN_AGENT) ?? null;
  const rec = useAgentChat(MAIN_AGENT);
  const [modelSet, setModelSet] = useState<boolean | null>(null);
  const [draft, setDraft] = useState("");
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const lines = termLines(rec.chat);

  // Whether a model is set up: read, never written.
  useEffect(() => {
    let live = true;
    RunAPI.SetupStatus().then((s) => { if (live) setModelSet(s.providerSetUp); }).catch(() => { if (live) setModelSet(true); });
    return () => { live = false; };
  }, []);

  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [lines.length, rec.chat.streaming, starting]);

  const running = agent?.state === AGENT_STATES.running;
  const answering = starting || rec.sending || rec.chat.busy;
  const elsewhere = agent?.state === AGENT_STATES.elsewhere;

  const send = async () => {
    const text = draft.trim();
    if (!text || answering || !agent || elsewhere) return;
    setError(null);
    if (!running) {
      // The first message starts Main; Go asks natively first where its
      // folders have not been agreed to on this computer.
      setStarting(true);
      const failed = await startAgent(MAIN_AGENT);
      setStarting(false);
      const now = agentNamed(MAIN_AGENT);
      if (failed !== null || now?.state !== AGENT_STATES.running) {
        // A start that failed says why; one stopped from Settings sends
        // nothing, and the message stays to send again.
        const why = now ? startFailureLines(now) : null;
        setError(failed ?? (why ? [why.title, ...why.lines].join("\n") : null));
        return;
      }
    }
    setDraft("");
    await sendToAgent(MAIN_AGENT, text);
  };

  const noModel = modelSet === false;
  return (
    <CardFrame title="Agentry Helper" onOpen={onOpenSettings} className="home-card main-card" status={agent ? mainStatusLine(agent) : "Reading Agentry Helper…"}>
      <div className="build-way-part main-card-chat">
        <div className="main-card-scroll" ref={scroller} role="log" aria-live="polite" aria-label="Chat with Agentry Helper">
          {lines.length === 0 && !starting && !rec.chat.streaming && (
            <p className="agent-term-note">
              {noModel ? "" : "Ask Agentry Helper to find a QuickAgent, run one on a file in your mainshare folder, or schedule one."}
            </p>
          )}
          {lines.map((l, i) => {
            switch (l.kind) {
              case "person":
                return <div key={i} className="agent-term-person"><span aria-hidden="true">&gt; </span>{l.text}</div>;
              case "agent":
                return <div key={i} className="agent-term-agent"><AgentMarkdown text={l.text} /></div>;
              case "tool":
                return <div key={i} className="agent-term-tool">· {l.text}</div>;
              case "warn":
                return <div key={i} className="agent-term-warn">{l.text}</div>;
              default:
                return <div key={i} className="agent-term-note">{l.text}</div>;
            }
          })}
          {starting && <div className="agent-term-note">{agent?.stage ? `${agent.stage}…` : "Starting Agentry Helper…"}</div>}
          {rec.chat.streaming && <div className="agent-term-agent streaming">{rec.chat.streaming}</div>}
          {!starting && answering && !rec.chat.streaming && <div className="agent-term-note">thinking…</div>}
        </div>
        {noModel && (
          <p className="small" data-main-no-model>
            Set up a model to talk to Agentry Helper. <Button variant="link" onClick={onSetUpModel}>Settings → Provider</Button>
          </p>
        )}
        {elsewhere && <p className="small muted">Agentry Helper is running in {agent?.holder || "another client"}.</p>}
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
            aria-label="Message Agentry Helper"
            value={draft}
            disabled={noModel || elsewhere || !agent}
            placeholder={noModel ? "Set up a model first" : "Message Agentry Helper"}
            onChange={(e) => setDraft(e.target.value)}
          />
          <Button type="submit" variant="primary" disabled={noModel || elsewhere || answering || !draft.trim()}>Send</Button>
        </form>
      </div>
    </CardFrame>
  );
}
