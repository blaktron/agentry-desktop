// The persistent agents' stores: one chat per agent, kept for the app's
// life so a conversation goes on while the person is on another page, fed by
// runner:agent-event keyed by the agent; and the list of agents the sidebar
// and Settings read, read again whenever runner:agent-state says one
// started or stopped. The logic is persistentAgents.ts's.
import { useEffect, useState } from "react";
import * as HomeAPI from "../wailsjs/go/app/HomeAPI";
import * as PersistentAPI from "../wailsjs/go/app/PersistentAPI";
import { EventsOn } from "../wailsjs/runtime/runtime";
import type { app } from "../wailsjs/go/models";
import { AGENT_STATES, ASSISTANT_AGENT, MAIN_AGENT, applyAgentEvent, chatFromTranscript, holdEarly, installingAny, newAgentChat, refreshesView, type AgentChat } from "./persistentAgents";
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

// While a server installs, the list is read again every few seconds: the
// supervisor's notice that an install ended refreshes it as well, and this
// keeps the state true should the notice come before the state moves.
const INSTALLING_POLL_MS = 3000;
let installPoll: ReturnType<typeof setTimeout> | null = null;

/** refreshAgents reads the list of agents again. */
export async function refreshAgents(): Promise<app.PersistentAgentView[]> {
  wire();
  try {
    list = await PersistentAPI.List();
  } catch {
    list = [];
  }
  changed();
  pollWhileInstalling();
  return list;
}

function pollWhileInstalling() {
  if (!installingAny(list) || installPoll !== null) return;
  installPoll = setTimeout(() => {
    installPoll = null;
    void refreshAgents();
  }, INSTALLING_POLL_MS);
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

// How long a failed send waits for its turn's error to reach the chat.
const TURN_ERROR_WAIT_MS = 500;

/** sendToAgent sends the person's message as the agent's next turn. */
export async function sendToAgent(name: string, text: string): Promise<void> {
  put(name, { ...record(name), sending: true, error: "" });
  const before = record(name).chat.lines.length;
  try {
    await PersistentAPI.Send(name, text);
    put(name, { ...record(name), sending: false });
  } catch (e) {
    // A turn that failed says why in the chat; its events can land just
    // after the call answers, so they are given a moment first.
    await new Promise((r) => setTimeout(r, TURN_ERROR_WAIT_MS));
    const said = record(name).chat.lines.slice(before).some((l) => l.kind === "warn");
    put(name, { ...record(name), sending: false, error: said ? "" : String(e) });
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

/** agentNamed is the agent's view as the list last read it. */
export function agentNamed(name: string): app.PersistentAgentView | undefined {
  return list.find((a) => a.name === name);
}

/** updateAgent puts a view a binding returned into the list. */
/** setAgentEnabled turns an agent on or off in Go and answers its view.
 *  Turning the Assistant on turns Agentry Helper off as well, so the list is
 *  read again rather than waiting for Go's event. */
export async function setAgentEnabled(name: string, on: boolean): Promise<app.PersistentAgentView> {
  const v = await PersistentAPI.SetEnabled(name, on);
  updateAgent(v);
  if (on && name === ASSISTANT_AGENT) await refreshAgents();
  return v;
}

export function updateAgent(v: app.PersistentAgentView) {
  const i = list.findIndex((a) => a.name === v.name);
  list = i < 0 ? [...list, v] : list.map((a, j) => (j === i ? v : a));
  changed();
  pollWhileInstalling();
}

/** forgetChat drops an agent's chat after a reset, so its page reads the
 *  (now empty) transcript again. */
export function forgetChat(name: string) {
  records.delete(name);
  early.delete(name);
  changed();
}

/** stopStart cancels an agent's start under way, and answers the error to
 *  show, or null. */
export async function stopStart(name: string): Promise<string | null> {
  try {
    updateAgent(await PersistentAPI.Stop(name));
    return null;
  } catch (e) {
    return String(e);
  }
}

/** stopTurn stops the turn the agent is on; the agent keeps running. It
 *  answers the error to show, or null. */
export async function stopTurn(name: string): Promise<string | null> {
  try {
    await PersistentAPI.Interrupt(name);
    return null;
  } catch (e) {
    return String(e);
  }
}

/** startAgent starts an agent and puts its view into the list. It answers
 *  the error to show, or null: a start that failed is shown by the agent's
 *  own view (its failure), so its error is not shown twice. */
export async function startAgent(name: string): Promise<string | null> {
  try {
    updateAgent(await PersistentAPI.Start(name));
    return null;
  } catch (e) {
    const now = await refreshAgents();
    return now.find((a) => a.name === name)?.failure ? null : String(e);
  }
}

// The agent Home's card runs, as Go keeps it: read on first use, and set
// through pickHomeAgent by the card's switcher, Settings → Home and the
// Welcome's Agents step alike.
let homeAgent: string | null = null;
let homeAgentRead: Promise<void> | null = null;

function readHomeAgent() {
  if (homeAgentRead) return;
  homeAgentRead = HomeAPI.Agent()
    .then((name) => { homeAgent = name; })
    .catch(() => { homeAgent = MAIN_AGENT; })
    .finally(changed);
}

/** rereadHomeAgent reads the card's agent again, after Reset Home. */
export function rereadHomeAgent() {
  homeAgentRead = null;
  readHomeAgent();
}

/** useHomeAgent is the agent Home's card was given, null until read. Who
 *  the card draws is homeAgentDrawn's: the Assistant or Agentry Helper
 *  while that agent is off, or no card. */
export function useHomeAgent(): string | null {
  const [, setBump] = useState(bump);
  useEffect(() => {
    const l = () => setBump(bump);
    listeners.add(l);
    readHomeAgent();
    return () => {
      listeners.delete(l);
    };
  }, []);
  return homeAgent;
}

/** pickHomeAgent gives Home's card an agent, turning it on first when it is
 *  off, as Settings does: the Coding agent's own dialog comes first, and a
 *  No changes nothing. It answers what to say: an error, "Nothing was
 *  changed." after a No, or null when the card has the agent. */
export async function pickHomeAgent(name: string): Promise<string | null> {
  try {
    const a = agentNamed(name);
    if (a && a.state === AGENT_STATES.disabled) {
      const v = await setAgentEnabled(name, true);
      if (v.state === AGENT_STATES.disabled) return "Nothing was changed.";
    }
    homeAgent = await HomeAPI.SetAgent(name);
    changed();
    return null;
  } catch (e) {
    return String(e);
  }
}
