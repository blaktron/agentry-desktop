// The persistent agents, as logic: the chat an agent's page draws from its
// transcript and its live events, the sidebar's entries, the roster grouped
// for its panel, and Settings' status line. Pure, so the pages and their
// tests share it. Every text here is data the agent or the person wrote, or
// the runner's sentence, drawn as text.
import type { app } from "../wailsjs/go/models";
import { applyEvent, newChat, type Chat, type SessionEvent } from "./sessionChat.ts";

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

/** The sidebar's entry for each enabled agent: the Assistant by the name
 *  the person gave it, and a dot for running or stopped. */
export type AgentNavEntry = { id: string; label: string; running: boolean };

export function agentNavEntries(agents: Pick<app.PersistentAgentView, "name" | "displayName" | "state">[]): AgentNavEntry[] {
  return agents
    .filter((a) => a.state !== "disabled")
    .map((a) => ({ id: a.name, label: a.displayName, running: a.state === "running" || a.state === "starting" || a.state === "elsewhere" }));
}

/** The words for an agent's state. */
export function agentStateWords(a: Pick<app.PersistentAgentView, "state" | "holder">): string {
  switch (a.state) {
    case "running":
      return "Running";
    case "starting":
      return "Starting…";
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
  const enabled = agents.filter((a) => a.state !== "disabled").length;
  const running = agents.filter((a) => a.state === "running" || a.state === "elsewhere").length;
  if (enabled === 0) return "None enabled";
  return `${enabled} enabled, ${running} running`;
}

/** The roster's panel: its entries grouped by kind, in the roster's order. */
export type RosterGroup = { title: string; entries: app.PersistentRosterEntry[] };

const GROUP_TITLES: Record<string, string> = { files: "Folders", knowledge: "Knowledge packs", server: "MCP servers" };

export function rosterGroups(roster: app.PersistentRosterEntry[]): RosterGroup[] {
  const out: RosterGroup[] = [];
  for (const e of roster) {
    const title = GROUP_TITLES[e.kind] ?? "Other";
    let g = out.find((x) => x.title === title);
    if (!g) {
      g = { title, entries: [] };
      out.push(g);
    }
    g.entries.push(e);
  }
  return out;
}

/** The words for an entry's state. */
export function rosterStateWords(e: Pick<app.PersistentRosterEntry, "state">): string {
  switch (e.state) {
    case "on":
      return "On";
    case "pending":
      return "Applies at the next turn";
    case "needs-sign-in":
      return "Needs a sign-in";
    default:
      return "Off";
  }
}

/** Whether a roster change is waiting for the agent's next turn. */
export function pendingChange(a: Pick<app.PersistentAgentView, "roster" | "status">): boolean {
  return !!a.status?.pending || a.roster.some((e) => e.state === "pending");
}
