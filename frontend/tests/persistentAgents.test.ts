// A persistent agent's page and its places in the app: the chat it draws
// from its transcript and its events, the sidebar's entries, the roster's
// groups and Settings' status line.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import {
  agentNavEntries,
  agentStateWords,
  applyAgentEvent,
  chatFromTranscript,
  newAgentChat,
  pendingChange,
  persistentAgentsStatus,
  rosterGroups,
  rosterStateWords,
  termLines,
} from "../src/persistentAgents.ts";

const t = "2026-10-20T19:00:00Z";
const now = Date.parse("2026-10-20T19:10:00Z");

test("the transcript reloads as the chat: the person's turns, the agent's, tool calls and notices", () => {
  const c = chatFromTranscript([
    { kind: "user_turn", time: t, text: "Hi" },
    { kind: "tool_call", time: t, text: "fs_read" },
    { kind: "assistant_turn", time: t, text: "Hello, I'm Sandra." },
    { kind: "notice", time: t, text: "Applying your change: the agent restarts with it, and the conversation continues" },
    { kind: "user_turn", time: t, text: "left unanswered" },
  ], now);
  assert.equal(c.busy, false, "a turn the transcript left unanswered is not thinking");
  assert.deepEqual(termLines(c), [
    { kind: "person", text: "Hi" },
    { kind: "tool", text: "fs_read" },
    { kind: "agent", text: "Hello, I'm Sandra." },
    { kind: "note", text: "Applying your change: the agent restarts with it, and the conversation continues" },
    { kind: "person", text: "left unanswered" },
  ]);
});

test("a segment's end is not the conversation's: the chat goes on, and the last turn's tokens are kept", () => {
  let c = newAgentChat();
  c = applyAgentEvent(c, { kind: "user_turn", time: t, text: "Switch the share off" }, now);
  c = applyAgentEvent(c, { kind: "usage", time: t, inputTokens: 100, outputTokens: 20 }, now);
  c = applyAgentEvent(c, { kind: "usage", time: t, totalTokens: 30 }, now);
  assert.equal(c.lastTurnTokens, 150);
  c = applyAgentEvent(c, { kind: "session_end", time: t, reason: "stopped" }, now);
  assert.equal(c.ended, null);
  assert.equal(c.busy, false);
  c = applyAgentEvent(c, { kind: "user_turn", time: t, text: "next" }, now);
  assert.equal(c.lastTurnTokens, 0, "a new turn starts its count again");
  assert.equal(c.busy, true);
});

test("a synthetic turn the runner sent is not drawn as the person's", () => {
  const c = applyAgentEvent(newAgentChat(), { kind: "user_turn", time: t, text: "[frame]", synthetic: true }, now);
  assert.deepEqual(termLines(c), []);
});

const agent = (over: Partial<app.PersistentAgentView>): app.PersistentAgentView => ({
  name: "assistant", title: "Assistant", displayName: "Assistant", state: "disabled", share: "/home/me/agentry/assistantshare",
  defaultShare: true, knowledge: [], servers: [], roster: [], lifetime: { segments: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0, userTurns: 0, toolCalls: 0 },
  volumes: { session: "agentry-persistent-assistant-session", data: "agentry-persistent-assistant-data" },
  ...over,
} as app.PersistentAgentView);

test("the sidebar lists each enabled agent by the name the person gave it, with its dot", () => {
  assert.deepEqual(agentNavEntries([agent({ state: "disabled" })]), []);
  assert.deepEqual(agentNavEntries([agent({ state: "stopped", displayName: "Sandra" })]), [{ id: "assistant", label: "Sandra", running: false }]);
  assert.deepEqual(agentNavEntries([agent({ state: "running", displayName: "Sandra" })]), [{ id: "assistant", label: "Sandra", running: true }]);
  assert.equal(agentNavEntries([agent({ state: "elsewhere" })])[0].running, true);
});

test("Settings' card counts the enabled and the running agents", () => {
  assert.equal(persistentAgentsStatus([agent({})]), "None enabled");
  assert.equal(persistentAgentsStatus([agent({ state: "stopped" })]), "1 enabled, 0 running");
  assert.equal(persistentAgentsStatus([agent({ state: "running" })]), "1 enabled, 1 running");
  assert.equal(agentStateWords({ state: "elsewhere", holder: "the CLI (pid 42)" }), "Running in the CLI (pid 42)");
  assert.equal(agentStateWords({ state: "disabled" }), "Off");
});

test("the roster's panel groups its entries by kind, in the roster's order, and says what waits for the next turn", () => {
  const e = (id: string, kind: string, state = "on") => ({ id, kind, name: id, grantedBy: "built-in", state, on: state !== "off", line: "" }) as app.PersistentRosterEntry;
  const groups = rosterGroups([e("share", "files"), e("data", "files"), e("knowledge:wiki", "knowledge", "pending"), e("server:x", "server", "needs-sign-in")]);
  assert.deepEqual(groups.map((g) => [g.title, g.entries.map((x) => x.id)]), [
    ["Folders", ["share", "data"]],
    ["Knowledge packs", ["knowledge:wiki"]],
    ["MCP servers", ["server:x"]],
  ]);
  assert.equal(rosterStateWords({ state: "pending" }), "Applies at the next turn");
  assert.equal(rosterStateWords({ state: "needs-sign-in" }), "Needs a sign-in");
  assert.equal(pendingChange(agent({ roster: [e("knowledge:wiki", "knowledge", "pending")] })), true);
  assert.equal(pendingChange(agent({ roster: [e("share", "files")] })), false);
});
