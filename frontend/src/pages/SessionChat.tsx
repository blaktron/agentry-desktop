// The interactive run surface (plan §7, D-017): a chat over the runner's
// session API. Every message goes through RunAPI.SendTurn, every file
// through RunAPI.AddSessionInput, and the engine ends the session on the
// operator's caps whatever this view does. Finish asks the agent for its
// final result before teardown; Stop ends without one. Either way the
// receipts folder holds transcript.md and the session's events.
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { ChooseFiles } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { ErrorBox, Spinner } from "../ui";

/** One runner:session-event payload (main.SessionEventView on the Go side;
 * it travels only over the event bus, so it is not in the bindings). */
interface SessionEvent {
  kind: string;
  time: string;
  text?: string;
  synthetic?: boolean;
  stopReason?: string;
  path?: string;
  digest?: string;
  reason?: string;
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
}

type Line =
  | { kind: "user"; text: string; synthetic: boolean }
  | { kind: "assistant"; text: string }
  | { kind: "note"; text: string }
  | { kind: "warn"; text: string };

export default function SessionChat({
  registry, agentRef, task, inputs, outDir, keepProfile, timeoutSec, maxTurns, acceptUnsigned, allowVulnerableMCP, intakeAnswers, opening, onResult,
}: {
  registry: string;
  agentRef: string;
  task: string;
  inputs: string[];
  outDir: string;
  keepProfile: boolean | null;
  timeoutSec: number;
  maxTurns: number;
  acceptUnsigned: boolean;
  /** Run despite a known high/critical advisory on a declared MCP server (recorded in the receipt). */
  allowVulnerableMCP: boolean;
  intakeAnswers: Record<string, string>;
  opening: string;
  /** Called once the session has ended and its envelope is written. */
  onResult: (r: main.RunResultView | null, err: string | null) => void;
}) {
  const [lines, setLines] = useState<Line[]>([]);
  const [streaming, setStreaming] = useState("");
  const [input, setInput] = useState("");
  const [starting, setStarting] = useState(true);
  const [busy, setBusy] = useState(false);
  const [ending, setEnding] = useState(false);
  const [ended, setEnded] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [turns, setTurns] = useState(0);
  const [tools, setTools] = useState(0);
  const [cost, setCost] = useState(0);
  const paneRef = useRef<HTMLDivElement | null>(null);
  const endedRef = useRef(false);

  useEffect(() => {
    const off = EventsOn("runner:session-event", (ev: SessionEvent) => {
      switch (ev.kind) {
        case "user_turn":
          setStreaming("");
          setLines((p) => [...p, { kind: "user", text: ev.text ?? "", synthetic: !!ev.synthetic }]);
          if (!ev.synthetic) setTurns((n) => n + 1);
          break;
        case "assistant_text":
          setStreaming((s) => s + (ev.text ?? ""));
          break;
        case "assistant_turn":
          setStreaming("");
          setLines((p) => [...p, { kind: "assistant", text: (ev.text ?? "").trim() }]);
          break;
        case "tool_call":
          setTools((n) => n + 1);
          setLines((p) => [...p, { kind: "note", text: `tool: ${ev.text}` }]);
          break;
        case "permission_denied":
          setLines((p) => [...p, { kind: "note", text: `permission request denied: ${ev.text}` }]);
          break;
        case "input_added":
          setLines((p) => [...p, { kind: "note", text: `input added: ${ev.path}` }]);
          break;
        case "usage":
          if (ev.costUsd !== undefined && ev.costUsd > 0) { const c0 = ev.costUsd; setCost((c) => Math.max(c, c0)); }
          break;
        case "error":
          setLines((p) => [...p, { kind: "warn", text: `harness error: ${ev.text}` }]);
          break;
        case "session_end":
          endedRef.current = true;
          setEnded(ev.reason || "ended");
          setLines((p) => [...p, { kind: "warn", text: `Session ended: ${ev.reason}` }]);
          break;
        default:
          break;
      }
    });
    return off;
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await RunAPI.StartSession(registry, agentRef, task, inputs, outDir, keepProfile, timeoutSec, maxTurns, acceptUnsigned, intakeAnswers, allowVulnerableMCP);
        if (!cancelled) {
          if (!task.trim() && opening.trim()) setLines((p) => [...p, { kind: "assistant", text: opening }]);
          setStarting(false);
        }
      } catch (e) {
        if (!cancelled) onResult(null, String(e));
      }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    paneRef.current?.scrollTo({ top: paneRef.current.scrollHeight });
  }, [lines, streaming]);

  // The engine ended the session by itself (budget, idle, harness exit):
  // collect the envelope so the receipts show like any other run.
  useEffect(() => {
    if (ended && !ending) void end(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ended]);

  async function send() {
    const text = input.trim();
    if (!text || busy || ended) return;
    setInput("");
    setBusy(true);
    setError(null);
    try {
      await RunAPI.SendTurn(text);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function addFile() {
    try {
      const files = await ChooseFiles("Give the agent a file (read-only under /inputs)");
      for (const f of files) await RunAPI.AddSessionInput(f);
    } catch (e) {
      setError(String(e));
    }
  }

  async function end(finish: boolean) {
    if (ending) return;
    setEnding(true);
    try {
      const r = await RunAPI.EndSession(finish);
      onResult(r, null);
    } catch (e) {
      onResult(null, String(e));
    }
  }

  return (
    <div>
      <h3>Session</h3>
      <div ref={paneRef} className="run-log selectable" style={{ minHeight: 220, maxHeight: 360, overflowY: "auto", whiteSpace: "pre-wrap" }}>
        {starting && <Spinner label="Starting the sandbox and the agent…" />}
        {lines.map((l, i) => {
          if (l.kind === "user") return <div key={i}><strong>{l.synthetic ? "Runner" : "You"}:</strong> {l.text}</div>;
          if (l.kind === "assistant") return <div key={i} style={{ marginBottom: 6 }}><strong>Agent:</strong> {l.text}</div>;
          if (l.kind === "warn") return <div key={i} className="badge warn">{l.text}</div>;
          return <div key={i} className="muted small">· {l.text}</div>;
        })}
        {streaming && <div><strong>Agent:</strong> {streaming}</div>}
      </div>
      <p className="muted small" style={{ margin: "4px 0" }}>
        {agentRef} · interactive · {turns} turn{turns === 1 ? "" : "s"} · {tools} tool call{tools === 1 ? "" : "s"}
        {cost > 0 ? ` · $${cost.toFixed(4)}` : ""}{busy ? " · thinking…" : ""}{ended ? ` · ended (${ended})` : ""}
      </p>
      <ErrorBox error={error} />
      <div className="row">
        <input
          type="text"
          className="grow"
          placeholder={ended ? "the session has ended" : "message the agent"}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && send()}
          disabled={starting || busy || !!ended || ending}
        />
        <button onClick={send} disabled={starting || busy || !!ended || ending || !input.trim()}>Send</button>
        <button onClick={addFile} disabled={starting || !!ended || ending} title="Give the agent another file, read-only under /inputs">Add file…</button>
        <button className="primary" onClick={() => end(true)} disabled={starting || busy || !!ended || ending} title="Ask the agent for its final result, then end">
          {ending ? "Ending…" : "Finish"}
        </button>
        <button onClick={() => end(false)} disabled={starting || ending} title="End now without a final result">Stop</button>
      </div>
    </div>
  );
}
