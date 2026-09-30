// A session's chat, as logic (plan hosted-sessions.md §6; agentry-desktop
// #403): what each event does to the conversation, and the hosted status
// strip's words. SessionChat draws it for both transports — the engine's
// session on this machine (runner:session-event) and a hosted session In
// Agentry (runner:hosted-session-event), which Go hands over in the same
// shape. It matches the site's lib/session-chat.ts (agentry-web#748)
// element by element: the You, Runner and Agent labels, streamed agent
// text replaced by the finished turn, the same notes, and "Session ended:
// out of credits" where a minute the balance could not cover stopped it.
// Every text here is data the agent or the person wrote, drawn as text.

/** One session event (main.SessionEventView on the Go side; it travels
 * only over the event bus, so it is not in the bindings). */
export interface SessionEvent {
  /** A hosted session's frame sequence number; absent on this machine. */
  seq?: number;
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

export type Line =
  | { kind: "user"; text: string; synthetic: boolean }
  | { kind: "assistant"; text: string }
  | { kind: "note"; text: string }
  | { kind: "warn"; text: string }
  | { kind: "end"; reason: string };

export interface Chat {
  lines: Line[];
  /** The agent's reply as it streams in, replaced by its finished turn. */
  streaming: string;
  /** The person's turns (the runner's own notices are not counted). */
  turns: number;
  tools: number;
  /** The session's reported cost in dollars (this machine's harnesses). */
  cost: number;
  /** A message the agent has not answered yet (hosted: the turn call
   * returns before the reply, so the events say when it is done). */
  busy: boolean;
  /** The end reason, once the session has ended. */
  ended: string | null;
  /** When the session last did something that resets its idle close. */
  lastActivity: number | null;
  /** When it ended, so its clock stops there. */
  endedAt: number | null;
  /** The last hosted frame taken: a replay gives each one once. */
  lastSeq: number;
}

/** SESSION_TRANSPORT_SENTENCE is plan D2's copy, beside a hosted session's
 * input only. Go sends the same words with the offer
 * (SessionTransportSentence); this one stands in where the offer was not
 * read, as for a session the panel reattached to. */
export const SESSION_TRANSPORT_SENTENCE =
  "Transport encrypted by the user key, messages not stored; the transcript comes back only in your sealed output.";

/** DROPPED_NOTE is what the host's memory guard letting go of earlier
 * events says: they are only in the sealed transcript. */
export const DROPPED_NOTE = "Earlier turns are not shown here. They are in the transcript in your sealed output.";

/** newChat is the conversation before any event: the release's opening as
 * the first Agent bubble when the session starts with no first message. */
export function newChat(opening: string, task: string): Chat {
  const lines: Line[] = !task.trim() && opening.trim() ? [{ kind: "assistant", text: opening.trim() }] : [];
  return { lines, streaming: "", turns: 0, tools: 0, cost: 0, busy: false, ended: null, lastActivity: null, endedAt: null, lastSeq: 0 };
}

// The events that reset a session's idle close, as the CLI counts them
// (agentry-cli pkg/runner/engine/session.go).
const ACTIVITY = new Set(["user_turn", "assistant_text", "tool_call", "assistant_turn"]);

/** applyEvent is what one event does to the conversation. `now` is the
 * page's clock; an event's own time is taken where it is not later, so a
 * replay keeps the idle close honest. */
export function applyEvent(c: Chat, ev: SessionEvent, now: number): Chat {
  if (ev.kind === "dropped") {
    // Its seq is the first frame after the gap: a chat that already has it
    // (a replay) has said this once.
    return ev.seq && ev.seq <= c.lastSeq ? c : { ...c, lines: [...c.lines, { kind: "note", text: DROPPED_NOTE }] };
  }
  if (ev.seq && ev.seq <= c.lastSeq) return c;
  const at = ev.time ? Date.parse(ev.time) : NaN;
  const when = Number.isFinite(at) && at <= now ? at : now;
  const next: Chat = { ...c, lastSeq: ev.seq || c.lastSeq, lastActivity: ACTIVITY.has(ev.kind) ? when : c.lastActivity };
  const text = ev.text ?? "";
  switch (ev.kind) {
    case "user_turn":
      return {
        ...next, streaming: "", busy: true,
        turns: ev.synthetic ? c.turns : c.turns + 1,
        lines: [...c.lines, { kind: "user", text, synthetic: !!ev.synthetic }],
      };
    case "assistant_text":
      return { ...next, streaming: c.streaming + text };
    case "assistant_turn":
      return { ...next, streaming: "", busy: false, lines: [...c.lines, { kind: "assistant", text: text.trim() }] };
    case "tool_call":
      return { ...next, tools: c.tools + 1, lines: [...c.lines, { kind: "note", text: `tool: ${text}` }] };
    case "permission_denied":
      return { ...next, lines: [...c.lines, { kind: "note", text: `permission request denied: ${text}` }] };
    case "input_added":
      return { ...next, lines: [...c.lines, { kind: "note", text: `input added: ${ev.path}` }] };
    case "usage":
      return ev.costUsd !== undefined && ev.costUsd > 0 ? { ...next, cost: Math.max(c.cost, ev.costUsd) } : next;
    case "error":
      return { ...next, lines: [...c.lines, { kind: "warn", text: `harness error: ${text}` }] };
    case "session_end": {
      const reason = ev.reason || "ended";
      return { ...next, busy: false, streaming: "", ended: reason, endedAt: when, lines: [...c.lines, { kind: "end", reason }] };
    }
    case "stream_lost":
      return { ...next, lines: [...c.lines, { kind: "warn", text }] };
    default:
      return next;
  }
}

/** endText is the end note's words: the session's own reason, or "out of
 * credits" where a minute the balance could not cover stopped it. */
export function endText(reason: string, outOfCredits?: boolean): string {
  return `Session ended: ${outOfCredits ? "out of credits" : reason}`;
}

/** clock reads an elapsed time as m:ss, or h:mm:ss past an hour. */
export function clock(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

/** hostedStatusLine is the strip under a hosted session (plan §6): the
 * release, the mode, the turns and tool calls; once it runs, the time it
 * has run, the free minutes left, the credits its time has cost and when
 * it closes if nothing happens; and whether the agent is answering, or how
 * it ended. The tokens settle at the end and show in the result. */
export function hostedStatusLine(a: {
  ref: string;
  turns: number;
  tools: number;
  now: number;
  startedAt?: number;
  lastActivity?: number | null;
  freeMinutes: number;
  timeCredits: number;
  idleSeconds: number;
  busy: boolean;
  ended: string | null;
  outOfCredits?: boolean;
}): string {
  const parts = [
    a.ref,
    "interactive in Agentry",
    `${a.turns} turn${a.turns === 1 ? "" : "s"}`,
    `${a.tools} tool call${a.tools === 1 ? "" : "s"}`,
  ];
  if (a.startedAt !== undefined) {
    const elapsed = Math.max(0, a.now - a.startedAt);
    parts.push(clock(elapsed));
    parts.push(`free minutes left ${Math.max(0, a.freeMinutes - Math.floor(elapsed / 60_000))}`);
    parts.push(`${a.timeCredits} credit${a.timeCredits === 1 ? "" : "s"} for time`);
    if (!a.ended) {
      const since = Math.max(a.startedAt, a.lastActivity ?? a.startedAt);
      const left = Math.max(0, Math.ceil((a.idleSeconds * 1000 - (a.now - since)) / 1000));
      parts.push(`idle closes in ${left >= 60 ? `${Math.ceil(left / 60)}m` : `${left}s`}`);
    }
  }
  if (a.busy && !a.ended) parts.push("thinking…");
  if (a.ended) parts.push(`ended (${a.outOfCredits ? "out of credits" : a.ended})`);
  return parts.join(" · ");
}
