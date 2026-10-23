// The persistent agents, as logic: the chat an agent's page draws from its
// transcript and its live events, the sidebar's entries, the roster grouped
// for its panel, and Settings' status line. Pure, so the pages and their
// tests share it. Every text here is data the agent or the person wrote, or
// the runner's sentence, drawn as text.
import type { app } from "../wailsjs/go/models";
import { applyEvent, newChat, type Chat, type SessionEvent } from "./sessionChat.ts";

/** The states an agent's view reads. */
export const AGENT_STATES = { disabled: "disabled", stopped: "stopped", starting: "starting", running: "running", elsewhere: "elsewhere" } as const;

/** An agent's chat: its conversation, and the tokens of its last turn. */
export type AgentChat = Chat & { lastTurnTokens: number };

export function newAgentChat(): AgentChat {
  return { ...newChat("", ""), lastTurnTokens: 0 };
}

/** applyAgentEvent is what one of an agent's events does to its chat. A
 *  segment's end is not the conversation's: a change ends one segment and
 *  starts the next, and the conversation goes on, so it only stops the
 *  "thinking" state. A turn's usage is kept as the last turn's tokens. */
export function applyAgentEvent(c: AgentChat, ev: SessionEvent, now: number): AgentChat {
  if (ev.kind === "session_end") return { ...c, busy: false, streaming: "" };
  const next = applyEvent(c, ev, now);
  let lastTurnTokens = c.lastTurnTokens;
  if (ev.kind === "user_turn") lastTurnTokens = 0;
  if (ev.kind === "usage") {
    const split = (ev.inputTokens ?? 0) + (ev.outputTokens ?? 0);
    lastTurnTokens += split > 0 ? split : (ev.totalTokens ?? 0);
  }
  return { ...next, lastTurnTokens };
}

/** chatFromTranscript is the chat as the transcript left it, before the
 *  live events: its turns, tool calls, errors and notices. */
export function chatFromTranscript(events: SessionEvent[], now: number): AgentChat {
  let c = newAgentChat();
  for (const ev of events) c = applyAgentEvent(c, ev, now);
  // A turn the transcript left unanswered was ended with its segment.
  return { ...c, busy: false, streaming: "", lastTurnTokens: 0 };
}

/** A terminal-style line of the chat: the person's after a ">", the
 *  agent's as markdown, a tool call and the runner's notes dim. */
export type TermLine =
  | { kind: "person"; text: string }
  | { kind: "agent"; text: string }
  | { kind: "tool"; text: string }
  | { kind: "note"; text: string }
  | { kind: "warn"; text: string };

export function termLines(c: Chat): TermLine[] {
  return c.lines.flatMap((l): TermLine[] => {
    switch (l.kind) {
      case "user":
        return l.synthetic ? [] : [{ kind: "person", text: l.text }];
      case "assistant":
        return [{ kind: "agent", text: l.text }];
      case "note":
        return l.text.startsWith("tool: ") ? [{ kind: "tool", text: l.text.slice(6) }] : [{ kind: "note", text: l.text }];
      case "warn":
        return [{ kind: "warn", text: l.text }];
      case "end":
        return [];
    }
  });
}

/** Main's name: its place is its Home card, with no sidebar entry or page. */
export const MAIN_AGENT = "main";

/** viewingAgent is the persistent agent whose page the person is looking at,
 *  for Go: its page shown while the window is visible and has the focus, or
 *  "" for anything else. A task of that agent raises no desktop notification
 *  then, since its answer is already in front of them. */
export function viewingAgent(route: { page: string; id?: string }, focused: boolean, visible: boolean): string {
  return route.page === "persistent" && focused && visible && route.id ? route.id : "";
}

/** The Coding agent's name. */
export const CODER_AGENT = "coder";

/** What Settings says before the Coding agent's switch, as the native
 *  dialog that turning it on raises says it. */
export const CODER_WARNING =
  "The Coding agent builds and runs software, reaches the web and any dev server you give it, and reads your codingshare folder as it is. That is riskier than other agents.";

/** Whether the Coding agent's segment is live in this app: the sidebar's
 *  "Coding agent running" badge. */
export function coderRunning(agents: Pick<app.PersistentAgentView, "name" | "state">[]): boolean {
  return agents.some((a) => a.name === CODER_AGENT && a.state === AGENT_STATES.running);
}

/** What the Settings page says after a switch: a No in the Coding agent's
 *  dialog answers the agent as it was, and nothing was changed. */
export function enableOutcome(asked: boolean, after: Pick<app.PersistentAgentView, "state">): string | null {
  return asked && after.state === AGENT_STATES.disabled ? "Nothing was changed." : null;
}

/** The sidebar's entry for each enabled agent but Main: the Assistant by the
 *  name the person gave it, and a dot for running or stopped. */
export type AgentNavEntry = { id: string; label: string; running: boolean };

export function agentNavEntries(agents: Pick<app.PersistentAgentView, "name" | "displayName" | "state">[]): AgentNavEntry[] {
  return agents
    .filter((a) => a.state !== AGENT_STATES.disabled && a.name !== MAIN_AGENT)
    .map((a) => ({ id: a.name, label: a.displayName, running: a.state === AGENT_STATES.running || a.state === AGENT_STATES.starting || a.state === AGENT_STATES.elsewhere }));
}

/** The Main card's status line: "Main · 3 turns · 1,204 tokens" over its
 *  whole life, "Running" or "Stopped" after its name, and before its first
 *  start a line saying a message starts it. */
export function mainStatusLine(a: Pick<app.PersistentAgentView, "state" | "lifetime" | "status"> & Partial<Pick<app.PersistentAgentView, "holder" | "stage">>): string {
  if (a.state === AGENT_STATES.disabled) return "Starts when you send it a message";
  const turns = (a.lifetime?.userTurns ?? 0) + (a.status?.userTurns ?? 0);
  const tokens = (a.lifetime?.totalTokens ?? 0) + (a.status?.totalTokens ?? 0);
  const parts = [a.state === AGENT_STATES.running ? "Running" : agentStateWords(a)];
  parts.push(`${turns} turn${turns === 1 ? "" : "s"}`);
  if (tokens > 0) parts.push(`${tokens.toLocaleString("en-US")} tokens`);
  return parts.join(" · ");
}

/** The words for an agent's state: while it starts, the step its start is
 *  on, in the runner's words ("Getting images ready…"). */
export function agentStateWords(a: Pick<app.PersistentAgentView, "state" | "holder" | "stage">): string {
  switch (a.state) {
    case "running":
      return "Running";
    case "starting":
      return a.stage ? `${a.stage}…` : "Starting…";
    case "elsewhere":
      return a.holder ? `Running in ${a.holder}` : "Running elsewhere";
    case "stopped":
      return "Stopped";
    default:
      return "Off";
  }
}

/** Settings → Persistent Agents' status line on its card. */
export function persistentAgentsStatus(agents: Pick<app.PersistentAgentView, "state">[]): string {
  const enabled = agents.filter((a) => a.state !== AGENT_STATES.disabled).length;
  const running = agents.filter((a) => a.state === AGENT_STATES.running || a.state === AGENT_STATES.elsewhere).length;
  if (enabled === 0) return "None enabled";
  return `${enabled} enabled, ${running} running`;
}

/** The changes Retry makes, in order: the failed server switched off, which
 *  clears its failure, and on again, which installs it again. */
export function retryChanges(id: string): { switch: { id: string; on: boolean } }[] {
  return [{ switch: { id, on: false } }, { switch: { id, on: true } }];
}

/** A server's "Can see your share folder" switch: its id and whether it is
 *  on, or, for a server the share is never given to, disabled with the
 *  runner's reason. */
export type ShareSwitch = { id: string; on: boolean; disabled: boolean; reason?: string };

export const SHARE_SWITCH_LABEL = "Can see your share folder";

/** What the switch says while the agent's own share folder is off: the
 *  server sees the folder only while the agent has it. */
export const SHARE_OFF_NOTE = "Only while your share folder is on";

/** shareSwitchNote is the line under a server's share switch: why it is
 *  disabled, or that it waits for the agent's share folder to be on. */
export function shareSwitchNote(agent: Pick<app.PersistentAgentView, "roster">, s: ShareSwitch): string | null {
  if (s.reason) return s.reason;
  const share = agent.roster.find((e) => e.id === "share");
  return share && !share.on ? SHARE_OFF_NOTE : null;
}

/** serverShareSwitch is the switch a server entry shows, or null for an
 *  entry that is not a server, or a server the runner says nothing about.
 *  A disabled switch has no id: nothing can be switched through it. */
export function serverShareSwitch(e: app.PersistentRosterEntry): ShareSwitch | null {
  if (e.kind !== "server") return null;
  if (e.shareSwitch) return { id: e.shareSwitch, on: !!e.share, disabled: false };
  if (e.shareRefusal) return { id: "", on: false, disabled: true, reason: e.shareRefusal };
  return null;
}

/** Whether any agent has a server installing: the list is read again
 *  until none has, besides the notices that say an install ended. */
export function installingAny(agents: Pick<app.PersistentAgentView, "roster">[]): boolean {
  return agents.some((a) => a.roster.some((e) => e.kind === "server" && e.state === "installing"));
}

/** What the Configuration's share folder line says the agent does with a
 *  file added there: the Coding agent reads it as it is; the others check
 *  it with ClamAV first when the malware check is on, and read it
 *  unchecked when it is off. */
export function shareCheckWords(coder: boolean, malwareCheck: boolean): string {
  if (coder) return ", and reads the files in it as they are";
  return malwareCheck
    ? ", and a file you add is checked for malware before it reads it"
    : ", and reads a file you add unchecked: the malware check is off on this computer";
}

/** Whether a roster change is waiting for the agent's next turn. */
export function pendingChange(a: Pick<app.PersistentAgentView, "roster" | "status">): boolean {
  return !!a.status?.pending || a.roster.some((e) => e.state === "pending");
}

/** What the app says where a release runs only persistent: the runner's own
 *  refusal, word for word. */
export const PERSISTENT_ONLY_SENTENCE = "A persistent release runs only as a built-in in this version.";

/** Whether a release's modes are persistent alone: no run of it is offered. */
export function persistentOnly(modes?: string[]): boolean {
  const m = modes ?? [];
  return m.includes("persistent") && !m.includes("autonomous") && !m.includes("interactive");
}

// The kinds the transcript keeps, which a reload already draws.
const KEPT = new Set(["user_turn", "assistant_turn", "tool_call", "error", "session_end", "input_added", "permission_denied", "notice"]);

/** The most events held for an agent whose transcript is not read yet. */
export const MAX_EARLY = 200;

/** holdEarly is what an event that arrives before the agent's transcript is
 *  read leaves held: an event the transcript keeps is dropped, since the
 *  reload draws it, and what it does not keep (the streamed reply, its
 *  usage) is held, at most MAX_EARLY, the newest kept. */
export function holdEarly(held: SessionEvent[], ev: SessionEvent): SessionEvent[] {
  if (KEPT.has(ev.kind)) return held;
  const next = [...held, ev];
  return next.length > MAX_EARLY ? next.slice(next.length - MAX_EARLY) : next;
}

/** Whether an event may change what the agent's view says: a turn
 *  answered (a change waiting for it has been applied) or the runner's
 *  notice. */
export function refreshesView(ev: SessionEvent): boolean {
  return ev.kind === "assistant_turn" || ev.kind === "notice" || ev.kind === "session_end";
}

/** The Configuration's server form. */
export type ServerForm = { source: string; package: string; version: string; url: string; tools: string };

const list = (s: string) => s.split(",").map((x) => x.trim()).filter(Boolean);

/** serverRowOf is the form as the Builder's server row: a package of npm or
 *  PyPI at an exact version, or a remote server's address, which signs in
 *  with nothing. */
export function serverRowOf(f: ServerForm): { source: string; package?: string; version?: string; url?: string; auth?: string; tools: string[] } {
  if (f.source === "url") return { source: "url", url: f.url.trim(), auth: "none", tools: list(f.tools) };
  return { source: f.source, package: f.package.trim(), version: f.version.trim(), tools: list(f.tools) };
}

/** Whether the form names a server to add. */
export function canAddServer(f: ServerForm): boolean {
  return f.source === "url" ? f.url.trim() !== "" : f.package.trim() !== "" && f.version.trim() !== "";
}

/** A port name the CLI takes: 1 to 32 lower-case letters, digits and
 *  hyphens, not starting with a hyphen. */
export const PORT_NAME = /^[a-z0-9][a-z0-9-]{0,31}$/;

/** Whether a port form's name and number make a port the CLI takes: a
 *  number from 1024 to 65535, the ports a program serves without privilege. */
export function portFormOK(name: string, port: string): boolean {
  const n = Number(port);
  return PORT_NAME.test(name.trim()) && /^\d+$/.test(port.trim()) && n >= 1024 && n <= 65535;
}

/** Whether an entry is a published port the agent serves now: on, not
 *  waiting for the next turn, while the agent runs. */
export function portOpenable(agent: Pick<app.PersistentAgentView, "state">, e: Pick<app.PersistentRosterEntry, "kind" | "on" | "state">): boolean {
  return e.kind === "port" && e.on && e.state === "on" && (agent.state === AGENT_STATES.running || agent.state === AGENT_STATES.elsewhere);
}

/** What each reason code of the runner's closed list says about a start
 *  that failed; a code this build does not know is shown as it is. */
const START_REASONS: Record<string, string> = {
  "mcp-install-failed": "A server could not be installed",
  "mcp-not-resolved": "A server's version was not found in the registry",
  "mcp-vulnerable": "The registry's vulnerability scan refused a server",
  "mcp-runtime-excluded": "A server does not run on the runtime the agent uses",
  "mcp-start-failed": "The gateway could not start a server",
  "secret-missing": "A secret has no value",
  "signin-missing": "Your model's sign-in is missing or cannot be used",
  "exec-not-provided": "This computer's policy does not provide a command the agent needs",
  "host-refused": "The dev server was not reached",
  "out-of-credits": "Your model's provider says the account has no credits left",
  "plan-limit": "Your plan's usage limit was reached",
  "org-policy": "A policy of your organization at the model's vendor refused it",
  "inputs-not-decoded": "The share folder's check could not run",
};

/** startReasonWords is a failed start's reason in words: what the code
 *  says, the names it concerns, and its detail. */
export function startReasonWords(r: Pick<app.PersistentReasonView, "code" | "detail" | "names">): string {
  let s = Object.hasOwn(START_REASONS, r.code) ? START_REASONS[r.code] : r.code;
  if (r.names?.length) s += `: ${r.names.join(", ")}`;
  if (r.detail) s += ` (${r.detail})`;
  return s;
}

/** startFailureLines are what an agent's page and its Settings card say of
 *  its last start, when it failed: the step it stopped in, the error, the
 *  reason, and the log that has the rest. Empty while it starts or runs. */
export function startFailureLines(a: Pick<app.PersistentAgentView, "state" | "failure">): { title: string; lines: string[] } | null {
  const f = a.failure;
  if (!f || (a.state !== AGENT_STATES.stopped && a.state !== AGENT_STATES.disabled)) return null;
  const lines = [f.error];
  if (f.reason) lines.push(startReasonWords(f.reason));
  if (f.log) lines.push(`Its log: ${f.log}`);
  return { title: f.stage ? `The last start failed at: ${f.stage}` : "The last start failed", lines };
}
