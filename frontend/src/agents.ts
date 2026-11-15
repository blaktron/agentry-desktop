// The persistent agents' stores: one chat per agent, kept for the app's
// life so a conversation goes on while the person is on another page, fed by
// runner:agent-event keyed by the agent; and the list of agents the sidebar
// and Settings read, read again whenever runner:agent-state says one
// started or stopped. The logic is persistentAgents.ts's.
import { useEffect, useState } from "react";
import * as HomeAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/homeapi";
import * as PersistentAPI from "../bindings/github.com/blaktron/agentry-desktop/internal/app/persistentapi";
import { EventsOn } from "./host";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { AGENT_STATES, ASSISTANT_AGENT, MAIN_AGENT, applyAgentEvent, chatFromTranscript, holdEarly, installingAny, isLive, newAgentChat, refreshesView, type AgentChat } from "./persistentAgents";
import type { SessionEvent } from "./sessionChat";
import { QUEUE_NOT_SENT_NOTE, QUEUE_NO_STEER_NOTE, personLines, queueJoin } from "./queued";

export type AgentRecord = {
  chat: AgentChat;
  /** The transcript has been read into the chat. */
  loaded: boolean;
  /** A message the person sent that has not been answered. */
  sending: boolean;
  error: string;
  /** A message sent while the agent was on a turn, waiting for its next
   *  one (queued.ts). */
  queued: string | null;
  /** Sending the queued message when the turn ended failed: it waits for
   *  the person to send it or edit it. */
  queuePaused: boolean;
  /** Why the queued message still waits, when the person asked for more. */
  queueNote: string | null;
  /** The queued message is being handed to the turn in flight. */
  steering: boolean;
};

const records = new Map<string, AgentRecord>();
let list: app.PersistentAgentView[] = [];
let bump = 0;
const listeners = new Set<() => void>();
const changed = () => {
  bump++;
  listeners.forEach((l) => l());
};

const record = (name: string): AgentRecord =>
  records.get(name) ?? { chat: newAgentChat(), loaded: false, sending: false, error: "", queued: null, queuePaused: false, queueNote: null, steering: false };
const put = (name: string, r: AgentRecord) => {
  records.set(name, r);
  changed();
  if (r.queued) queueMicrotask(() => flushQueued(name));
};

/** answering says the agent is on a turn: the person's, or one of its own. */
export const answering = (r: Pick<AgentRecord, "sending" | "chat">): boolean => r.sending || r.chat.busy;

// The queued message goes as the next turn once the agent is running and
// between turns.
function flushQueued(name: string) {
  const r = record(name);
  const agent = agentNamed(name);
  if (!r.queued || r.queuePaused || r.steering || !r.loaded || answering(r) || !agent || !isLive(agent)) return;
  const text = r.queued;
  put(name, { ...r, queued: null, queueNote: null });
  void deliver(name, text, true);
}

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

/** sendToAgent sends the person's message as the agent's next turn. While
 *  the agent is on a turn it is queued for the next one instead, joining any
 *  message already waiting, and goes by itself when the turn ends; a
 *  message waiting from before goes with this one. */
export async function sendToAgent(name: string, text: string): Promise<"sent" | "queued"> {
  const r = record(name);
  if (answering(r) || r.steering) {
    put(name, { ...r, queued: queueJoin(r.queued, text), queuePaused: false, queueNote: null, error: "" });
    return "queued";
  }
  const waiting = r.queued;
  if (waiting) put(name, { ...r, queued: null, queuePaused: false, queueNote: null });
  await deliver(name, queueJoin(waiting, text) ?? text, false);
  return "sent";
}

// Sends one message as a turn. One from the queue that failed before its
// turn was drawn goes back to the queue, first, and waits for the person.
async function deliver(name: string, text: string, fromQueue: boolean): Promise<void> {
  put(name, { ...record(name), sending: true, error: "" });
  const lines = record(name).chat.lines;
  const before = lines.length;
  const people = personLines(lines);
  try {
    await PersistentAPI.Send(name, text);
    put(name, { ...record(name), sending: false });
  } catch (e) {
    // A turn that failed says why in the chat; its events can land just
    // after the call answers, so they are given a moment first.
    await new Promise((r) => setTimeout(r, TURN_ERROR_WAIT_MS));
    const now = record(name);
    const said = now.chat.lines.slice(before).some((l) => l.kind === "warn");
    const back = fromQueue && personLines(now.chat.lines) === people;
    put(name, {
      ...now, sending: false, error: said ? "" : String(e),
      ...(back ? { queued: queueJoin(text, now.queued), queuePaused: true, queueNote: QUEUE_NOT_SENT_NOTE } : {}),
    });
  }
}

/** sendQueuedNow is Send now: the queued message goes at once. While the
 *  agent is on a turn the turn is stopped, and the message is the next turn.
 *  It answers the error to show, or null. */
export async function sendQueuedNow(name: string): Promise<string | null> {
  const r = record(name);
  if (!r.queued) return null;
  put(name, { ...r, queuePaused: false, queueNote: null });
  if (!answering(r)) {
    flushQueued(name);
    return null;
  }
  return stopTurn(name);
}

/** takeQueued is Edit: the queued message, out of the queue, for the
 *  composer. Nothing is sent. */
export function takeQueued(name: string): string | null {
  const r = record(name);
  if (!r.queued) return null;
  put(name, { ...r, queued: null, queuePaused: false, queueNote: null });
  return r.queued;
}

/** addQueuedToTurn is Add to this turn: the queued message handed to the
 *  chat turn the agent is answering. The turn's steer event draws it, as
 *  "sent during the turn" and then "joined the turn". A turn that takes no
 *  message (one the agent runs on its own, or any on a harness that takes a
 *  message only between turns) keeps it queued, and says so; a turn that
 *  ended as it went sends it as the next. */
export async function addQueuedToTurn(name: string): Promise<void> {
  const r = record(name);
  if (!r.queued || r.steering || !answering(r)) return;
  const text = r.queued;
  put(name, { ...r, queued: null, queueNote: null, steering: true, error: "" });
  let answer: string;
  try {
    answer = await PersistentAPI.Steer(name, text);
  } catch (e) {
    const now = record(name);
    put(name, { ...now, steering: false, queued: queueJoin(text, now.queued), error: String(e) });
    return;
  }
  const now = record(name);
  if (answer === "sent") {
    put(name, { ...now, steering: false });
    return;
  }
  put(name, {
    ...now, steering: false, queued: queueJoin(text, now.queued),
    queueNote: answer === "between-turns" ? QUEUE_NO_STEER_NOTE : now.queueNote,
  });
}

/** agentRecord is one agent's record as the store holds it now. */
export const agentRecord = (name: string): AgentRecord => record(name);

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
  if (records.get(v.name)?.queued) queueMicrotask(() => flushQueued(v.name));
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
