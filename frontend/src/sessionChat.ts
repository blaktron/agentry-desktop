// A session's chat, as logic: what each event does to the conversation, and the
// hosted status strip's words. SessionChat draws it for both transports — the
// engine's session on this machine (runner:session-event) and a hosted session
// In Agentry (runner:hosted-session-event), which Go hands over in the same
// shape. It matches the site's chat element by element: the You, Runner and
// Agent labels, streamed agent text replaced by the finished turn, the same
// notes, and "Session ended: out of credits" where a minute the balance could
// not cover stopped it. Every text here is data the agent or the person wrote,
// drawn as text.
import type { app } from "../wailsjs/go/models";

/** One session event (app.SessionEventView on the Go side; it travels
 * only over the event bus, so it is not in the bindings). */
export interface SessionEvent {
  /** A hosted session's frame sequence number; absent on this machine. */
  seq?: number;
  kind: string;
  time: string;
  text?: string;
  synthetic?: boolean;
  /** On a persistent agent's turn that its heartbeat sent, the task's id:
   *  the turn is the agent's own, not the person's. */
  task?: string;
  /** On a persistent agent's turn, the runner's mark of an upkeep turn (a
   *  compaction or a memory tidy): the turn is the runner's, not the
   *  person's. */
  upkeep?: boolean;
  stopReason?: string;
  path?: string;
  digest?: string;
  reason?: string;
  /** A usage event's tokens for its turn (this machine's harnesses; a
   *  session In Agentry carries none). totalTokens is
   *  a harness's that gives no input/output split. */
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  costUsd?: number;
}

export type Line =
  | { kind: "user"; text: string; synthetic: boolean; task?: string; upkeep?: boolean }
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
  /** The tokens its usage events have reported, summed as the CLI's
   *  session counts them (this machine's harnesses). */
  tokens: number;
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

/** SESSION_TRANSPORT_SENTENCE is the copy beside a hosted session's
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
  return { lines, streaming: "", turns: 0, tools: 0, cost: 0, tokens: 0, busy: false, ended: null, lastActivity: null, endedAt: null, lastSeq: 0 };
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
        turns: ev.synthetic || ev.task || ev.upkeep ? c.turns : c.turns + 1,
        lines: [...c.lines, {
          kind: "user", text, synthetic: !!ev.synthetic,
          ...(ev.task ? { task: ev.task } : {}),
          ...(ev.upkeep ? { upkeep: true } : {}),
        }],
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
    case "usage": {
      const split = (ev.inputTokens ?? 0) + (ev.outputTokens ?? 0);
      const tokens = c.tokens + (split > 0 ? split : (ev.totalTokens ?? 0));
      return { ...next, tokens, cost: Math.max(c.cost, ev.costUsd ?? 0) };
    }
    case "error":
      return { ...next, lines: [...c.lines, { kind: "warn", text: `harness error: ${text}` }] };
    case "session_end": {
      const reason = ev.reason || "ended";
      return { ...next, busy: false, streaming: "", ended: reason, endedAt: when, lines: [...c.lines, { kind: "end", reason }] };
    }
    case "stream_lost":
      return { ...next, lines: [...c.lines, { kind: "warn", text }] };
    // The runner's own word in the chat: a persistent agent applying a
    // change at its next turn.
    case "notice":
      return { ...next, lines: [...c.lines, { kind: "note", text }] };
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

/** localStatusLine is the strip under a chat on this computer: the
 * release, the turns and tool calls, the tokens its usage reported and the
 * provider's dollar cost, and whether the agent is answering or how it
 * ended. Never credits: the person's own provider is paid in its own
 * terms. */
export function localStatusLine(ref: string, c: Pick<Chat, "turns" | "tools" | "tokens" | "cost" | "ended">, busy: boolean): string {
  return `${ref} · interactive · ${c.turns} turn${c.turns === 1 ? "" : "s"} · ${c.tools} tool call${c.tools === 1 ? "" : "s"}` +
    `${c.tokens > 0 ? ` · ${c.tokens.toLocaleString("en-US")} tokens` : ""}` +
    `${c.cost > 0 ? ` · $${c.cost.toFixed(4)}` : ""}${busy ? " · thinking…" : ""}${c.ended ? ` · ended (${c.ended})` : ""}`;
}

/** hostedStatusLine is the strip under a hosted session: the
 * release, the mode, the turns and tool calls; once it runs, the time it
 * has run, the free minutes left, the credits its time has cost and when
 * it closes if nothing happens; and whether the agent is answering, or how
 * it ended. Its model use settles at the end, in credits, and shows in the
 * result. */
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

// A chat with a QuickAgent, on this computer or In Agentry, as the app follows
// it above the pages: the store starts the session
// and owns its events, so the chat's dialog and its corner card only draw it,
// and a switch between them never starts a second session. When the engine
// ends a chat on this computer the store collects its result itself, whether
// or not the dialog is open, so the run lock and the containers are released.
// A chat In Agentry is followed as a hosted session is: its frames, its state
// and, once its output is down, its last word as a run. One chat per
// app. Pure, so the store (runs.ts) and its tests share one set of
// rules.

/** A hosted session's state (app.HostedSessionStateView). */
export type HostedChatState = {
  status: string;
  startedAt?: string;
  finishedAt?: string;
  session?: app.HostedSessionView;
};

export type ChatKind = "local" | "hosted";

export type ChatRecord = {
  kind: ChatKind;
  /** In Agentry: the registry the chat is followed on. */
  registry: string;
  /** The release, ns/name@version. */
  ref: string;
  task: string;
  opening: string;
  chat: Chat;
  /** On this computer: the session is starting (the native consent, the
   *  sandbox and the harness). */
  starting: boolean;
  /** A message is on its way (SendTurn returns with the agent's turn on
   *  this computer, once the host has it In Agentry). */
  sending: boolean;
  adding: boolean;
  /** Finish or Stop was asked for, or the engine ended it and its result is
   *  being collected. */
  ending: boolean;
  /** On this computer: the session's log (runner:run-log). */
  log: string[];
  /** In Agentry: the run as the registry reports it, its state and the
   *  terms the strip reads. */
  run: app.HostedRunView | null;
  hosted: HostedChatState | null;
  terms: app.HostedSessionOfferView | null;
  /** On this computer, once it has ended: its envelope. */
  result: app.RunResultView | null;
  /** The last thing that went wrong, in Go's words. */
  error: string | null;
  /** On this computer: it never started, or it ended with no envelope. */
  failed: boolean;
  over: boolean;
};

/** Whether the QuickAgent is working on a turn, rather than waiting for the
 *  person. */
export function working(c: ChatRecord): boolean {
  if (c.over) return false;
  // The engine sends a task's first turn by itself, so the events, not only
  // the person's messages, say when the agent is on a turn.
  return c.starting || c.sending || c.chat.busy;
}

function overNow(c: ChatRecord): boolean {
  if (c.kind === "local") return !!c.result || c.failed;
  return !!c.run?.final;
}

/** What the store needs from the app: Go's events and the chat's calls.
 *  Injected, so the store's rules are tested without a webview. */
export type ChatDeps = {
  on: (event: string, cb: (v: never) => void) => () => void;
  sendLocal: (text: string) => Promise<void>;
  addLocal: (path: string) => Promise<void>;
  endLocal: (finish: boolean) => Promise<app.RunResultView>;
  sendHosted: (text: string) => Promise<void>;
  addHosted: (path: string) => Promise<void>;
  endHosted: (finish: boolean) => Promise<void>;
  /** Asks the host for every frame it still holds, for a chat the app has
   *  just begun to follow; each is taken once (applyEvent's seq). */
  replay: () => Promise<void>;
  forget: (runId: string) => void;
};

export type ChatStoreState<S> = { chat: ChatRecord | null; setup: S | null; shown: boolean; raised: number; asCard: boolean };

/** What the store follows a chat In Agentry from. */
export type HostedChatStart = {
  registry: string;
  run: app.HostedRunView;
  terms: app.HostedSessionOfferView | null;
  opening: string;
  task: string;
};

/** The one chat the app follows. */
export function createChatStore<S>(deps: ChatDeps) {
  let c: ChatRecord | null = null;
  let setup: S | null = null;
  let shown = false;
  let raised = 0;
  let asCard = false;
  // gen names the chat followed: a call's answer that comes back after the
  // chat was dismissed, or another began, changes nothing.
  let gen = 0;
  // A chat In Agentry found while another chat is live: followed, as its
  // card, once that one is dismissed, rather than followed by Go with
  // nothing showing it.
  let pending: HostedChatStart | null = null;
  const listeners = new Set<() => void>();
  const notify = () => listeners.forEach((f) => f());
  const update = (g: number, f: (r: ChatRecord) => ChatRecord) => {
    if (g !== gen || !c) return;
    const next = f(c);
    c = { ...next, over: overNow(next) };
    notify();
  };

  // The engine ended a chat on this computer: its envelope is collected
  // whether or not anything shows the chat, so the lock and the containers
  // are released.
  const collect = (g: number) => {
    if (!c || c.kind !== "local" || c.ending || c.result || c.failed) return;
    update(g, (r) => ({ ...r, ending: true }));
    deps.endLocal(false)
      .then((res) => update(g, (r) => ({ ...r, ending: false, result: res })))
      .catch((e) => update(g, (r) => ({ ...r, ending: false, failed: true, error: String(e) })));
  };

  let listening = false;
  const listen = () => {
    if (listening) return;
    listening = true;
    deps.on("runner:session-event", (ev: SessionEvent) => {
      if (!c || c.kind !== "local" || c.over) return;
      const g = gen;
      // On this computer a turn that fails ends with an error rather than
      // the agent's turn (agentry-cli engine/session.go), so the agent is no
      // longer on one. In Agentry an error can come mid-turn (a file the
      // host refused), so there the turn ends with the agent's own.
      const ended = (chat: Chat) => (ev.kind === "error" ? { ...chat, busy: false, streaming: "" } : chat);
      update(g, (r) => ({ ...r, chat: ended(applyEvent(r.chat, ev, Date.now())) }));
      if (c?.chat.ended) collect(g);
    });
    deps.on("runner:run-log", (line: string) => {
      if (!c || c.kind !== "local" || c.over) return;
      update(gen, (r) => ({ ...r, log: [...r.log, line] }));
    });
    deps.on("runner:hosted-session-event", (ev: SessionEvent) => {
      if (!c || c.kind !== "hosted" || c.over) return;
      update(gen, (r) => ({ ...r, chat: applyEvent(r.chat, ev, Date.now()) }));
    });
    deps.on("runner:hosted-session-state", (s: HostedChatState) => {
      if (!c || c.kind !== "hosted" || c.over) return;
      update(gen, (r) => ({ ...r, hosted: s }));
    });
    deps.on("runner:hosted-run", (v: app.HostedRunView) => {
      // Go follows a waiting chat too: its last word is kept for when it is
      // shown.
      if (pending && v.id === pending.run.id) pending = { ...pending, run: v };
      if (!c || c.kind !== "hosted" || c.over || v.id !== c.run?.id) return;
      update(gen, (r) => ({ ...r, run: v, ending: v.final ? false : r.ending }));
    });
  };

  const begin = (next: ChatRecord, s: S | null, card: boolean): number => {
    // A chat that is over gives way to the next, and is forgotten.
    if (c?.run) deps.forget(c.run.id);
    if (c?.result) deps.forget(c.result.runId);
    gen++;
    c = { ...next, over: overNow(next) };
    setup = s;
    shown = true;
    asCard = card;
    raised++;
    notify();
    return gen;
  };

  const blank = (kind: ChatKind, ref: string, task: string, opening: string): ChatRecord => ({
    kind, ref, task, opening, registry: "",
    chat: newChat(kind === "hosted" ? opening : "", task),
    starting: false, sending: false, adding: false, ending: false, log: [],
    run: null, hosted: null, terms: null, result: null, error: null, failed: false, over: false,
  });

  const store = {
    state: (): ChatStoreState<S> => ({ chat: c, setup, shown, raised, asCard }),
    subscribe(f: () => void): () => void {
      listen();
      listeners.add(f);
      return () => {
        listeners.delete(f);
      };
    },

    /** A chat is live: starting, going, or ending. */
    live: (): boolean => !!c && !c.over,

    /** Starts a chat on this computer by call (RunAPI.StartSession) and
     *  opens its dialog at once, the start under way inside it: the native
     *  consent, the sandbox and the harness. Go's refusal is the chat's
     *  error, with its setup to go back to. One chat at a time. */
    startLocal(s: S, ref: string, task: string, opening: string, call: () => Promise<unknown>): void {
      if (c && !c.over) throw "A chat is already open in this app; end it, then start another.";
      listen();
      const g = begin({ ...blank("local", ref, task, opening), starting: true }, s, false);
      Promise.resolve()
        .then(call)
        .then(() => update(g, (r) => ({
          ...r, starting: false,
          // The release's opening is its first bubble once the session is
          // up and the person gave no first message.
          chat: !task.trim() && opening.trim() ? { ...r.chat, lines: [...r.chat.lines, { kind: "assistant", text: opening.trim() }] } : r.chat,
        })))
        .catch((e) => update(g, (r) => ({ ...r, starting: false, failed: true, error: String(e) })));
    },

    /** Follows a chat In Agentry the registry has created, or one this
     *  account already had (at start, or as a run setup opened). The host
     *  is asked for every frame it still holds. */
    followHosted(v: HostedChatStart, s: S | null, card: boolean): boolean {
      listen();
      if (c && c.kind === "hosted" && c.run?.id === v.run.id && !c.over) {
        if (!card) store.show();
        return true;
      }
      if (c && !c.over) {
        // A waiting chat already over keeps its last word.
        if (!(pending?.run.id === v.run.id && pending.run.final)) pending = v;
        return false;
      }
      // The chat followed now is the one Go follows: one waiting from
      // before is no longer, and its last word, if it has one, stands.
      if (pending?.run.id === v.run.id && pending.run.final) v = { ...v, run: pending.run };
      pending = null;
      const g = begin({
        ...blank("hosted", v.run.ref, v.task || v.run.task || "", v.opening),
        registry: v.registry, run: v.run, terms: v.terms, hosted: { status: v.run.status, startedAt: v.run.startedAt, session: v.run.session },
      }, s, card);
      // A chat already over has nothing for the host to replay.
      if (!v.run.final) deps.replay().catch((e) => update(g, (r) => ({ ...r, error: String(e) })));
      return true;
    },

    /** Sends the agent one message. */
    async send(text: string): Promise<void> {
      if (!c || c.over || c.starting || working(c) || !text.trim()) return;
      const g = gen;
      const hosted = c.kind === "hosted";
      update(g, (r) => ({ ...r, sending: true, error: null }));
      try {
        await (hosted ? deps.sendHosted(text) : deps.sendLocal(text));
        update(g, (r) => ({ ...r, sending: false }));
      } catch (e) {
        update(g, (r) => ({ ...r, sending: false, error: String(e) }));
        throw e;
      }
    },

    /** Gives the agent files a native picker returned. */
    async addFiles(paths: string[]): Promise<void> {
      if (!c || c.over || paths.length === 0) return;
      const g = gen;
      const hosted = c.kind === "hosted";
      update(g, (r) => ({ ...r, adding: true }));
      try {
        for (const p of paths) await (hosted ? deps.addHosted(p) : deps.addLocal(p));
        update(g, (r) => ({ ...r, adding: false }));
      } catch (e) {
        update(g, (r) => ({ ...r, adding: false, error: String(e) }));
      }
    },

    /** Finish asks the agent for its final result, then ends; Stop ends
     *  now without one. On this computer the envelope comes back; In
     *  Agentry the output comes down and the run's last word follows. */
    async end(finish: boolean): Promise<void> {
      if (!c || c.over || c.ending || c.starting) return;
      const g = gen;
      const hosted = c.kind === "hosted";
      update(g, (r) => ({ ...r, ending: true, error: null }));
      if (hosted) {
        try {
          await deps.endHosted(finish);
        } catch (e) {
          update(g, (r) => ({ ...r, ending: false, error: String(e) }));
        }
        return;
      }
      try {
        const res = await deps.endLocal(finish);
        update(g, (r) => ({ ...r, ending: false, result: res }));
      } catch (e) {
        // Go has let the session go whether or not its End succeeded: the
        // chat is over, with Go's words, and can be dismissed.
        update(g, (r) => ({ ...r, ending: false, failed: true, error: String(e) }));
      }
    },

    /** Opens the flow as the dialog. */
    show(): void {
      shown = true;
      asCard = false;
      raised++;
      notify();
    },

    /** Dismiss, once the chat is over: the flow goes, and the app forgets
     *  it. */
    dismiss(): void {
      if (c && !c.over) return;
      if (c?.run) deps.forget(c.run.id);
      if (c?.result) deps.forget(c.result.runId);
      gen++;
      c = null;
      setup = null;
      shown = false;
      asCard = false;
      notify();
      if (pending) {
        const next = pending;
        pending = null;
        store.followHosted(next, null, true);
      }
    },
  };
  return store;
}
