// The interactive run surface: a chat over a session, one
// component with two transports.
//
// On this machine it drives the runner's session API: every message goes
// through RunAPI.SendTurn, or while the agent is answering into its turn
// through RunAPI.SteerTurn, Stop turn through RunAPI.InterruptTurn, every
// file through RunAPI.AddSessionInput, and
// the engine ends the session on the operator's caps whatever this view
// does. Finish asks the agent for its final result before teardown; End
// ends without one. Either way the receipts folder holds transcript.md and
// the session's events. Its strip has no caps and no meter.
//
// In Agentry it follows a hosted session the registry relays: the same events,
// from runner:hosted-session-event, with the run's state and session record
// from runner:hosted-session-state; messages, files and the end go through
// SendHostedTurn, AddHostedSessionFile and EndHostedSession. Its strip says the
// time it has run, the free minutes left, the credits its time has cost and the
// idle close, and the transport sentence sits beside the input. The result,
// once it ends, is the chat's flow's result view (ChatFlow) over the sealed
// output, which holds transcript.md.
import { useCallback, useEffect, useRef, useState } from "react";
import { ChooseFiles } from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/app";
import { Button, ErrorBox, Input, Spinner } from "../ui";
import { ChatAnnouncer } from "../chatAnnounce";
import { plainName } from "../reveal";
import { addChatFiles, endChat, sendChat, stopChatTurn } from "../runs";
import { DURING_WORDS, HELD_NOTE, SESSION_TRANSPORT_SENTENCE, endText, hostedStatusLine, localStatusLine, working, type ChatRecord, type Line } from "../sessionChat";

// The chat's store (runs.ts over sessionChat.ts's
// createChatStore) starts the session and owns its events, messages, files
// and end; this component only draws the record it is given, so the chat's
// dialog can close into its corner card and open again with nothing lost
// and no second session started.
export default function SessionChat({ c }: { c: ChatRecord }) {
  const inAgentry = c.kind === "hosted";
  const { chat, hosted: state } = c;
  const [input, setInput] = useState("");
  const [now, setNow] = useState(() => Date.now());
  const [sentAt, setSentAt] = useState<number | null>(null);
  // The harness took no message during the turn: the text stays, and the
  // composer says why until the answer arrives.
  const [held, setHeld] = useState(false);
  const paneRef = useRef<HTMLDivElement | null>(null);
  // How many lines a hosted chat opened with (the opening, or none): it has
  // started talking once there are more.
  const [openingLines] = useState(() => (c.kind === "hosted" && !c.task.trim() && c.opening.trim() ? 1 : 0));

  // In Agentry: the strip's clock.
  useEffect(() => {
    if (!inAgentry) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [inAgentry]);

  useEffect(() => {
    paneRef.current?.scrollTo({ top: paneRef.current.scrollHeight });
  }, [chat.lines, chat.streaming]);

  const { starting, ending, over } = c;
  const running = !inAgentry || state?.status === "running";
  const busy = working(c);
  // On this computer a message can go into the turn in flight; In Agentry it
  // waits for the answer.
  const intoTurn = !inAgentry && busy;
  useEffect(() => {
    if (!busy) setHeld(false);
  }, [busy]);
  const outOfCredits = !!state?.session?.outOfCredits;
  // What a screen reader hears of each new line: the agent's replies, the
  // warnings and the end.
  const sayChatLine = useCallback((l: Line) => {
    const said = l.kind === "assistant" || l.kind === "told" ? `Agent: ${l.text}`
      : l.kind === "warn" ? l.text
      : l.kind === "end" ? endText(l.reason, outOfCredits)
      : null;
    return said === null ? null : plainName(said);
  }, [outOfCredits]);

  function send() {
    const text = input.trim();
    if (!text || (busy && !intoTurn) || c.steering || over || !running) return;
    setInput("");
    setHeld(false);
    setSentAt(Date.now());
    // A message the host or the turn did not take stays in the field to
    // send again; a harness that takes one only between turns keeps it with
    // a word why.
    sendChat(text)
      .then((r) => {
        if (r === "held") {
          setInput(text);
          setHeld(true);
        }
      })
      .catch(() => (inAgentry || intoTurn) && setInput(text));
  }

  async function addFile() {
    const files = await ChooseFiles("Give the agent a file (read-only under /inputs)").catch(() => [] as string[]);
    await addChatFiles(files ?? []);
  }

  const startedAt = state?.startedAt ? Date.parse(state.startedAt) : undefined;
  const line = inAgentry
    ? hostedStatusLine({
        ref: c.ref,
        turns: chat.turns,
        tools: chat.tools,
        // The clock stops when the session ends.
        now: chat.endedAt ?? now,
        startedAt: state?.status === "running" || over ? startedAt : undefined,
        lastActivity: Math.max(chat.lastActivity ?? 0, sentAt ?? 0) || null,
        freeMinutes: state?.session?.freeMinutes ?? c.terms?.freeMinutes ?? 0,
        timeCredits: state?.session?.timeCredits ?? 0,
        idleSeconds: state?.session?.idleSeconds ?? c.terms?.idleSeconds ?? 300,
        busy,
        ended: chat.ended,
        outOfCredits,
      })
    : localStatusLine(c.ref, chat, busy);
  // In Agentry, until the first event: where the session stands.
  const waiting = inAgentry && !over && chat.lines.length === openingLines && !chat.streaming;
  const locked = starting || !running || over || ending;

  return (
    <div>
      <h3>Session{inAgentry ? " In Agentry" : ""}</h3>
      <div ref={paneRef} className="chat-pane" role="log" aria-live="off" aria-busy={!!chat.streaming} aria-label="The conversation">
        {starting && <Spinner label="Starting the sandbox and the agent…" />}
        {waiting && (
          <Spinner label={state?.status === "running" ? "Starting the sandbox and the agent…" : "Queued In Agentry — waiting for a sandbox host…"} />
        )}
        {chat.lines.map((l, i) => {
          if (l.kind === "user") return (
            <div key={i} className="chat-msg user" data-during={l.during}>
              <div className="chat-bubble">
                <div className="chat-who">{l.synthetic ? "Runner" : "You"}{l.during ? <span className="muted small"> · {DURING_WORDS[l.during]}</span> : null}</div>
                {l.text}
              </div>
            </div>
          );
          if (l.kind === "assistant" || l.kind === "told") return (
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
      <ChatAnnouncer items={chat.lines} say={sayChatLine} />
      <p className="muted small" style={{ margin: "6px 0" }} aria-label="Session status">{line}</p>
      <ErrorBox error={c.error} />
      {held && <p className="muted small" role="status" style={{ margin: "0 0 6px" }}>{HELD_NOTE}</p>}
      <div className="row">
        <Input
          type="text"
          className="grow"
          aria-label="Message the agent"
          placeholder={over ? "the session has ended" : intoTurn ? "message the agent: it joins the turn in progress" : "message the agent"}
          value={input}
          maxLength={inAgentry ? 16384 : undefined}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && !e.nativeEvent.isComposing && send()}
          disabled={locked || (busy && !intoTurn)}
        />
        <Button onClick={send} busy={c.steering} busyLabel="Sending…" disabled={locked || (busy && !intoTurn) || c.steering || !input.trim()}>Send</Button>
        {intoTurn && !locked && (
          <Button onClick={() => void stopChatTurn()} busy={c.stopping} busyLabel="Stopping…" disabled={c.stopping} title="Stop the agent's answer here; the chat goes on with your next message">Stop turn</Button>
        )}
        <Button onClick={() => void addFile()} disabled={locked || c.adding} title="Give the agent another file, read-only under /inputs">{c.adding ? "Adding…" : "Add file…"}</Button>
        <Button variant="primary" onClick={() => void endChat(true)} busy={ending} busyLabel="Ending…" disabled={locked || busy} title="Ask the agent for its final result, then end">Finish</Button>
        <Button onClick={() => void endChat(false)} disabled={starting || ending || over} title="End the chat now without a final result">End</Button>
      </div>
      {inAgentry && (
        <>
          <p className="muted small" style={{ margin: "8px 0 0" }}>{c.terms?.transport || SESSION_TRANSPORT_SENTENCE}</p>
          <p className="muted small" style={{ margin: "4px 0 0" }}>
            Closing this dialog or quitting the app does not end the session: it ends when you finish or stop it, or
            after it has been idle. A session still going when the app starts again is followed again.
          </p>
        </>
      )}
    </div>
  );
}
