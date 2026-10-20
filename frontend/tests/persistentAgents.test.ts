// A persistent agent's page and its places in the app: the chat it draws
// from its transcript and its events, the sidebar's entries, the roster's
// groups and Settings' status line.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import {
  MAX_EARLY,
  PERSISTENT_ONLY_SENTENCE,
  agentNavEntries,
  canAddServer,
  holdEarly,
  persistentOnly,
  refreshesView,
  serverRowOf,
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

test("an event before the transcript is read is held only if the reload would not draw it", () => {
  let held = holdEarly([], { kind: "user_turn", time: t, text: "in the transcript" });
  assert.deepEqual(held, []);
  held = holdEarly(held, { kind: "assistant_turn", time: t, text: "in the transcript too" });
  held = holdEarly(held, { kind: "notice", time: t, text: "kept" });
  assert.deepEqual(held, []);
  held = holdEarly(held, { kind: "assistant_text", time: t, text: "streamed" });
  held = holdEarly(held, { kind: "usage", time: t, totalTokens: 10 });
  assert.deepEqual(held.map((e) => e.kind), ["assistant_text", "usage"]);
  for (let i = 0; i < MAX_EARLY + 5; i++) held = holdEarly(held, { kind: "assistant_text", time: t, text: String(i) });
  assert.equal(held.length, MAX_EARLY);
  assert.equal(held[held.length - 1].text, String(MAX_EARLY + 4), "the newest are kept");
  assert.equal(refreshesView({ kind: "assistant_turn", time: t }), true);
  assert.equal(refreshesView({ kind: "assistant_text", time: t }), false);
});

test("the Configuration's server form is the Builder's row: a pinned package, or a remote address with no sign-in", () => {
  const pkg = { source: "npm", package: " @acme/notes ", version: " 1.2.3 ", url: "", tools: "read, write ,," };
  assert.deepEqual(serverRowOf(pkg), { source: "npm", package: "@acme/notes", version: "1.2.3", tools: ["read", "write"] });
  assert.equal(canAddServer(pkg), true);
  assert.equal(canAddServer({ ...pkg, version: " " }), false, "a package needs its exact version");
  const remote = { source: "url", package: "", version: "", url: " https://mcp.example.com/mcp ", tools: "" };
  assert.deepEqual(serverRowOf(remote), { source: "url", url: "https://mcp.example.com/mcp", auth: "none", tools: [] });
  assert.equal(canAddServer({ ...remote, url: "" }), false);
});

test("a release that runs only persistent offers no run, in the runner's own words", () => {
  assert.equal(persistentOnly(["persistent"]), true);
  assert.equal(persistentOnly(["persistent", "interactive"]), false);
  assert.equal(persistentOnly(["autonomous"]), false);
  assert.equal(persistentOnly(undefined), false);
  assert.equal(PERSISTENT_ONLY_SENTENCE, "A persistent release runs only as a built-in in this version.");
});
