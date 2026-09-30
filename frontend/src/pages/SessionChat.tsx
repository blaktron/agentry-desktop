// The interactive run surface (plan §7, D-017): a chat over a session, one
// component with two transports (plan hosted-sessions.md §5, §6;
// agentry-desktop#403).
//
// On this machine it drives the runner's session API: every message goes
// through RunAPI.SendTurn, every file through RunAPI.AddSessionInput, and
// the engine ends the session on the operator's caps whatever this view
// does. Finish asks the agent for its final result before teardown; Stop
// ends without one. Either way the receipts folder holds transcript.md and
// the session's events. Its strip has no caps and no meter (D9).
//
// In Agentry it follows a hosted session the registry relays: the same
// events, from runner:hosted-session-event, with the run's state and
// session record from runner:hosted-session-state; messages, files and the
// end go through SendHostedTurn, AddHostedSessionFile and EndHostedSession.
// Its strip says the time it has run, the free minutes left, the credits
// its time has cost and the idle close, and D2's sentence sits beside the
// input. The result, once it ends, is the panel's result view over the
// sealed output, which holds transcript.md.
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { ChooseFiles } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { Button, ErrorBox, Input, Spinner } from "../ui";
import { SESSION_TRANSPORT_SENTENCE, applyEvent, endText, hostedStatusLine, newChat, type Chat, type SessionEvent } from "../sessionChat";

/** A hosted session's state (main.HostedSessionStateView), from the event
 * bus. */
interface HostedState {
  status: string;
  startedAt?: string;
  finishedAt?: string;
  session?: main.HostedSessionView;
}

/** What SessionChat needs to follow a session In Agentry. */
export interface HostedSession {
  /** The run as the panel last heard of it. */
  run: main.HostedRunView;
  /** The registry's terms for a session and D2's sentence. */
  terms: main.HostedSessionOfferView | null;
}

type LocalProps = {
  registry: string;
  inputs: string[];
  outDir: string;
  keepProfile: boolean | null;
  timeoutSec: number;
  maxTurns: number;
  acceptUnsigned: boolean;
  acceptUnscanned: boolean;
  /** The knowledge packs kept on this computer to attach, by name. */
  knowledge: string[];
  /** Run despite a known high/critical advisory on a declared MCP server (recorded in the receipt). */
  allowVulnerableMCP: boolean;
  intakeAnswers: Record<string, string>;
  /** Called once the session has ended and its envelope is written. */
  onResult: (r: main.RunResultView | null, err: string | null) => void;
};

export default function SessionChat(props: {
  agentRef: string;
  task: string;
  opening: string;
} & ({ hosted: HostedSession } | ({ hosted?: undefined } & LocalProps))) {
  const { agentRef, task, opening, hosted } = props;
  const inAgentry = !!hosted;
  const [chat, setChat] = useState<Chat>(() => newChat(inAgentry ? opening : "", task));
  // How many lines a hosted chat opened with (the opening, or none): it has
  // started talking once there are more.
  const [openingLines] = useState(() => chat.lines.length);
  const [input, setInput] = useState("");
  const [starting, setStarting] = useState(!inAgentry);
  const [sending, setSending] = useState(false);
  const [adding, setAdding] = useState(false);
  const [ending, setEnding] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [state, setState] = useState<HostedState | null>(() =>
    hosted ? { status: hosted.run.status, startedAt: hosted.run.startedAt, session: hosted.run.session } : null);
  const [now, setNow] = useState(() => Date.now());
  const [sentAt, setSentAt] = useState<number | null>(null);
  const paneRef = useRef<HTMLDivElement | null>(null);

  // The events, from the transport this chat is on.
  useEffect(() => {
    const offEvent = EventsOn(inAgentry ? "runner:hosted-session-event" : "runner:session-event", (ev: SessionEvent) => {
      setChat((c) => applyEvent(c, ev, Date.now()));
    });
    if (!inAgentry) return offEvent;
    const offState = EventsOn("runner:hosted-session-state", (s: HostedState) => setState(s));
    // A chat opening on a session already under way gets every frame the
    // host still holds; each is taken once.
    RunAPI.ReplayHostedSession().catch((e) => setError(String(e)));
    return () => { offEvent(); offState(); };
  }, [inAgentry]);

  // On this machine: start the engine's session.
  useEffect(() => {
    if (props.hosted) return;
    const p = props;
    let cancelled = false;
    (async () => {
      try {
        await RunAPI.StartSession(p.registry, agentRef, task, p.inputs, p.outDir, p.keepProfile, p.timeoutSec, p.maxTurns, p.acceptUnsigned, p.intakeAnswers, p.allowVulnerableMCP, p.acceptUnscanned, p.knowledge);
        if (!cancelled) {
          if (!task.trim() && opening.trim()) setChat((c) => ({ ...c, lines: [...c.lines, { kind: "assistant", text: opening }] }));
          setStarting(false);
        }
      } catch (e) {
        if (!cancelled) p.onResult(null, String(e));
      }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // In Agentry: the strip's clock.
  useEffect(() => {
    if (!inAgentry) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [inAgentry]);

  useEffect(() => {
    paneRef.current?.scrollTo({ top: paneRef.current.scrollHeight });
  }, [chat.lines, chat.streaming]);

  // On this machine, the engine ended the session by itself (budget, idle,
  // harness exit): collect the envelope so the receipts show like any other
  // run. In Agentry the registry delivers the output by itself.
  useEffect(() => {
    if (chat.ended && !ending && !inAgentry) void end(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chat.ended]);

  const running = !inAgentry || state?.status === "running";
  const over = !!chat.ended || state?.status === "finished" || state?.status === "cancelled";
  // On this machine a message is in flight until SendTurn returns (it
  // returns with the agent's turn); In Agentry the turn call returns once
  // the host has it, and the events say when the agent has answered.
  const busy = inAgentry ? chat.busy || sending : sending;
  const outOfCredits = !!state?.session?.outOfCredits;

  async function send() {
    const text = input.trim();
    if (!text || busy || over || !running) return;
    setInput("");
    setSending(true);
    setSentAt(Date.now());
    setError(null);
    try {
      await (inAgentry ? RunAPI.SendHostedTurn(text) : RunAPI.SendTurn(text));
    } catch (e) {
      setError(String(e));
      if (inAgentry) setInput(text);
    } finally {
      setSending(false);
    }
  }

  async function addFile() {
    setAdding(true);
    try {
      const files = await ChooseFiles("Give the agent a file (read-only under /inputs)");
      for (const f of files) await (inAgentry ? RunAPI.AddHostedSessionFile(f) : RunAPI.AddSessionInput(f));
    } catch (e) {
      setError(String(e));
    } finally {
      setAdding(false);
    }
  }

  async function end(finish: boolean) {
    if (ending) return;
    setEnding(true);
    if (props.hosted) {
      try {
        await RunAPI.EndHostedSession(finish);
      } catch (e) {
        setError(String(e));
        setEnding(false);
      }
      return;
    }
    try {
      const r = await RunAPI.EndSession(finish);
      props.onResult(r, null);
    } catch (e) {
      props.onResult(null, String(e));
    }
  }

  const startedAt = state?.startedAt ? Date.parse(state.startedAt) : undefined;
  const line = inAgentry
    ? hostedStatusLine({
        ref: agentRef,
        turns: chat.turns,
        tools: chat.tools,
        // The clock stops when the session ends.
        now: chat.endedAt ?? now,
        startedAt: state?.status === "running" || over ? startedAt : undefined,
        lastActivity: Math.max(chat.lastActivity ?? 0, sentAt ?? 0) || null,
        freeMinutes: state?.session?.freeMinutes ?? hosted?.terms?.freeMinutes ?? 0,
        timeCredits: state?.session?.timeCredits ?? 0,
        idleSeconds: state?.session?.idleSeconds ?? hosted?.terms?.idleSeconds ?? 300,
        busy,
        ended: chat.ended,
        outOfCredits,
      })
    : `${agentRef} · interactive · ${chat.turns} turn${chat.turns === 1 ? "" : "s"} · ${chat.tools} tool call${chat.tools === 1 ? "" : "s"}` +
      `${chat.cost > 0 ? ` · $${chat.cost.toFixed(4)}` : ""}${busy ? " · thinking…" : ""}${chat.ended ? ` · ended (${chat.ended})` : ""}`;
  // In Agentry, until the first event: where the session stands.
  const waiting = inAgentry && !over && chat.lines.length === openingLines && !chat.streaming;
  const locked = starting || !running || over || ending;

  return (
    <div>
      <h3>Session{inAgentry ? " In Agentry" : ""}</h3>
      <div ref={paneRef} className="chat-pane" aria-live="polite" aria-label="The conversation">
        {starting && <Spinner label="Starting the sandbox and the agent…" />}
        {waiting && (
          <Spinner label={state?.status === "running" ? "Starting the sandbox and the agent…" : "Queued In Agentry — waiting for a sandbox host…"} />
        )}
        {chat.lines.map((l, i) => {
          if (l.kind === "user") return (
            <div key={i} className="chat-msg user"><div className="chat-bubble"><div className="chat-who">{l.synthetic ? "Runner" : "You"}</div>{l.text}</div></div>
          );
          if (l.kind === "assistant") return (
            <div key={i} className="chat-msg assistant"><div className="chat-bubble"><div className="chat-who">Agent</div>{l.text}</div></div>
          );
          if (l.kind === "end") return <div key={i} className="chat-note warn">{endText(l.reason, outOfCredits)}</div>;
          if (l.kind === "warn") return <div key={i} className="chat-note warn">{l.text}</div>;
          return <div key={i} className="chat-note">· {l.text}</div>;
        })}
        {chat.streaming && (
          <div className="chat-msg assistant"><div className="chat-bubble"><div className="chat-who">Agent</div>{chat.streaming}</div></div>
        )}
      </div>
      <p className="muted small" style={{ margin: "6px 0" }} aria-label="Session status">{line}</p>
      <ErrorBox error={error} />
      <div className="row">
        <Input
          type="text"
          className="grow"
          placeholder={over ? "the session has ended" : "message the agent"}
          value={input}
          maxLength={inAgentry ? 16384 : undefined}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && !e.nativeEvent.isComposing && send()}
          disabled={locked || busy}
        />
        <Button onClick={send} disabled={locked || busy || !input.trim()}>Send</Button>
        <Button onClick={addFile} disabled={locked || adding} title="Give the agent another file, read-only under /inputs">{adding ? "Adding…" : "Add file…"}</Button>
        <Button variant="primary" onClick={() => end(true)} busy={ending} busyLabel="Ending…" disabled={locked || busy} title="Ask the agent for its final result, then end">Finish</Button>
        <Button onClick={() => end(false)} disabled={starting || ending || over} title="End now without a final result">Stop</Button>
      </div>
      {hosted && (
        <>
          <p className="muted small" style={{ margin: "8px 0 0" }}>{hosted.terms?.transport || SESSION_TRANSPORT_SENTENCE}</p>
          <p className="muted small" style={{ margin: "4px 0 0" }}>
            Closing this panel or quitting the app does not end the session: it ends when you finish or stop it, or
            after it has been idle. Open this run again before then to pick the conversation up.
          </p>
        </>
      )}
    </div>
  );
}
