// The persistent agents' stores: one chat per agent, kept for the app's
// life so a conversation goes on while the person is on another page, fed by
// runner:agent-event keyed by the agent; and the list of agents the sidebar
// and Settings read, read again whenever runner:agent-state says one
// started or stopped. The logic is persistentAgents.ts's.
import { useEffect, useState } from "react";
import * as PersistentAPI from "../wailsjs/go/app/PersistentAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import type { app } from "../wailsjs/go/models";
import { applyAgentEvent, chatFromTranscript, holdEarly, newAgentChat, refreshesView, type AgentChat } from "./persistentAgents";
import type { SessionEvent } from "./sessionChat";

export type AgentRecord = {
  chat: AgentChat;
  /** The transcript has been read into the chat. */
  loaded: boolean;
  /** A message the person sent that has not been answered. */
  sending: boolean;
  error: string;
};

const records = new Map<string, AgentRecord>();
let list: app.PersistentAgentView[] = [];
let bump = 0;
const listeners = new Set<() => void>();
const changed = () => {
  bump++;
  listeners.forEach((l) => l());
};

const record = (name: string): AgentRecord => records.get(name) ?? { chat: newAgentChat(), loaded: false, sending: false, error: "" };
const put = (name: string, r: AgentRecord) => {
  records.set(name, r);
  changed();
};

// Events that arrive before the transcript is read are held, then applied
// after it, so the reload never draws a live turn above the history; one the
// transcript keeps is not held, since the reload draws it (holdEarly).
const early = new Map<string, SessionEvent[]>();

let wired = false;
function wire() {
  if (wired) return;
  wired = true;
  EventsOn("runner:agent-event", (p: { agent: string; event: SessionEvent }) => {
    const r = record(p.agent);
    if (refreshesView(p.event)) void refreshAgents();
    if (!r.loaded) {
      early.set(p.agent, holdEarly(early.get(p.agent) ?? [], p.event));
      return;
    }
    put(p.agent, { ...r, chat: applyAgentEvent(r.chat, p.event, Date.now()) });
  });
  EventsOn("runner:agent-state", () => {
    void refreshAgents();
  });
}

/** refreshAgents reads the list of agents again. */
export async function refreshAgents(): Promise<app.PersistentAgentView[]> {
  wire();
  try {
    list = await PersistentAPI.List();
  } catch {
    list = [];
  }
  changed();
  return list;
}

/** loadChat reads the agent's transcript into its chat, once. */
export async function loadChat(name: string): Promise<void> {
  wire();
  if (record(name).loaded) return;
  let history: SessionEvent[] = [];
  try {
    history = (await PersistentAPI.Transcript(name)) as SessionEvent[];
  } catch (e) {
    put(name, { ...record(name), error: String(e) });
  }
  let chat = chatFromTranscript(history, Date.now());
  for (const ev of early.get(name) ?? []) chat = applyAgentEvent(chat, ev, Date.now());
  early.delete(name);
  put(name, { ...record(name), chat, loaded: true });
}

/** sendToAgent sends the person's message as the agent's next turn. */
export async function sendToAgent(name: string, text: string): Promise<void> {
  put(name, { ...record(name), sending: true, error: "" });
  try {
    await PersistentAPI.Send(name, text);
    put(name, { ...record(name), sending: false });
  } catch (e) {
    put(name, { ...record(name), sending: false, error: String(e) });
  }
}

/** useAgents is the list of agents, read on first use and kept current. */
export function useAgents(): app.PersistentAgentView[] {
  const [, setBump] = useState(bump);
  useEffect(() => {
    const l = () => setBump(bump);
    listeners.add(l);
    if (list.length === 0) void refreshAgents();
    return () => {
      listeners.delete(l);
    };
  }, []);
  return list;
}

/** useAgentChat is one agent's record, its transcript read on first use. */
export function useAgentChat(name: string): AgentRecord {
  const [, setBump] = useState(bump);
  useEffect(() => {
    const l = () => setBump(bump);
    listeners.add(l);
    void loadChat(name);
    return () => {
      listeners.delete(l);
    };
  }, [name]);
  return record(name);
}

/** updateAgent puts a view a binding returned into the list. */
export function updateAgent(v: app.PersistentAgentView) {
  const i = list.findIndex((a) => a.name === v.name);
  list = i < 0 ? [...list, v] : list.map((a, j) => (j === i ? v : a));
  changed();
}

/** forgetChat drops an agent's chat after a reset, so its page reads the
 *  (now empty) transcript again. */
export function forgetChat(name: string) {
  records.delete(name);
  early.delete(name);
  changed();
}
