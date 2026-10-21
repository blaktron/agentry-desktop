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
  mainStatusLine,
  canAddServer,
  portFormOK,
  portOpenable,
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
  coderRunning,
  enableOutcome,
  termLines,
  startFailureLines,
  startReasonWords,
  serverState,
  serverShareSwitch,
  retryChanges,
  installingAny,
  shareSwitchNote,
  shareCheckWords,
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
  // Main's place is its Home card: no sidebar entry, running or not.
  assert.deepEqual(agentNavEntries([agent({ name: "main", state: "running", displayName: "Main" })]), []);
});

test("the Main card's status line counts turns and tokens over Main's life, and says a message starts it", () => {
  assert.equal(mainStatusLine(agent({ name: "main" })), "Starts when you send it a message");
  assert.equal(mainStatusLine(agent({ name: "main", state: "stopped", lifetime: { segments: 2, inputTokens: 0, outputTokens: 0, totalTokens: 1204, userTurns: 3, toolCalls: 1 } })),
    "Stopped · 3 turns · 1,204 tokens");
  assert.equal(mainStatusLine(agent({ name: "main", state: "running", lifetime: { segments: 1, inputTokens: 0, outputTokens: 0, totalTokens: 0, userTurns: 1, toolCalls: 0 } })),
    "Running · 1 turn");
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
  // The Coding agent's grant and its dev server have groups of their own.
  assert.deepEqual(rosterGroups([e("commands", "tools"), e("egress", "tools"), e("devhost", "host")]).map((g) => g.title), ["Tools", "Dev server"]);
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

test("the sidebar's Coding agent badge shows while its segment runs here, and not after Stop", () => {
  assert.equal(coderRunning([agent({ name: "coder", state: "running" })]), true);
  assert.equal(coderRunning([agent({ name: "coder", state: "stopped" })]), false);
  assert.equal(coderRunning([agent({ name: "assistant", state: "running" })]), false);
});

test("a No in the Coding agent's dialog leaves it off, and Settings says nothing was changed", () => {
  assert.equal(enableOutcome(true, { state: "disabled" }), "Nothing was changed.");
  assert.equal(enableOutcome(true, { state: "stopped" }), null);
  assert.equal(enableOutcome(false, { state: "disabled" }), null);
});

test("a port form takes a name the CLI takes and an unprivileged number", () => {
  assert.equal(portFormOK("web", "3000"), true);
  assert.equal(portFormOK(" web ", " 8080 "), true);
  for (const [name, port] of [["Web", "3000"], ["-web", "3000"], ["my web", "3000"], ["", "3000"], ["web", "80"], ["web", "65536"], ["web", "3e3"], ["web", ""]]) {
    assert.equal(portFormOK(name, port), false, `${name}=${port}`);
  }
});

test("a port opens only while the agent runs with it on and applied", () => {
  const port = { kind: "port", on: true, state: "on" };
  assert.equal(portOpenable({ state: "running" }, port), true);
  assert.equal(portOpenable({ state: "elsewhere" }, port), true);
  assert.equal(portOpenable({ state: "stopped" }, port), false);
  assert.equal(portOpenable({ state: "running" }, { ...port, state: "pending" }), false);
  assert.equal(portOpenable({ state: "running" }, { ...port, on: false, state: "off" }), false);
  assert.equal(portOpenable({ state: "running" }, { ...port, kind: "host" }), false);
  assert.deepEqual(rosterGroups([{ id: "port:web", kind: "port" } as never]).map((g) => g.title), ["Ports"]);
});

test("a starting agent shows the step its start is on, and Starting… before the first", () => {
  assert.equal(agentStateWords({ state: "starting" }), "Starting…");
  assert.equal(agentStateWords({ state: "starting", stage: "Installing pypi:office-word-mcp-server" }), "Installing pypi:office-word-mcp-server…");
  assert.equal(mainStatusLine(agent({ name: "main", state: "starting", stage: "Getting images ready" })), "Getting images ready… · 0 turns");
});

test("a failed start says where it stopped, its error, its reason in words and its log, until it starts again", () => {
  const failure = {
    error: "pip install exited 1",
    stage: "Installing pypi:office-word-mcp-server",
    reason: { code: "mcp-install-failed", names: ["pypi:office-word-mcp-server"], detail: "builder" },
    log: "/home/p/.agentry/runner/persistent/assistant/supervisor.log",
    at: "2026-10-21T12:00:00Z",
  } as app.PersistentFailureView;
  assert.deepEqual(startFailureLines({ state: "stopped", failure }), {
    title: "The last start failed at: Installing pypi:office-word-mcp-server",
    lines: [
      "pip install exited 1",
      "A server could not be installed: pypi:office-word-mcp-server (builder)",
      "Its log: /home/p/.agentry/runner/persistent/assistant/supervisor.log",
    ],
  });
  assert.equal(startFailureLines({ state: "starting", failure }), null);
  assert.equal(startFailureLines({ state: "stopped" }), null);
  assert.equal(startFailureLines({ state: "stopped", failure: { ...failure, stage: undefined, reason: undefined } as app.PersistentFailureView })?.title, "The last start failed");
  // A code this build does not name is shown as it is.
  assert.equal(startReasonWords({ code: "new-code", names: [] }), "new-code");
});

// A server the person added: installing, then ready, or failed with the
// runner's reason and a Retry; and its "Can see your share folder" switch.
const srv = (over: Partial<app.PersistentRosterEntry>) =>
  ({ id: "server:pypi-office-word-mcp-server", kind: "server", name: "pypi:office-word-mcp-server@1.1.11", grantedBy: "you", state: "on", on: true, line: "", ...over }) as app.PersistentRosterEntry;

test("a server's install state reads Installing…, Ready, or Failed with its reason and a Retry", () => {
  const running = agent({ state: "running" });
  assert.deepEqual(serverState(running, srv({ state: "installing" })), { words: "Installing…", retry: false });
  assert.deepEqual(serverState(running, srv({ state: "on" })), { words: "Ready", retry: false });
  assert.deepEqual(serverState(running, srv({ state: "pending" })), { words: "Ready, joins at your next message", retry: false });
  assert.deepEqual(serverState(running, srv({ state: "failed", reason: "resolve pypi:x@9: not found in the configured registry" })), {
    words: "Failed", reason: "resolve pypi:x@9: not found in the configured registry", retry: true,
  });
  // A failed install is shown, and retried, while the agent is stopped too.
  assert.equal(serverState(agent({ state: "stopped" }), srv({ state: "failed", reason: "x" })).retry, true);
  // Only the supervisor in this app knows a server is installed: elsewhere
  // and stopped, a server that is on is only on.
  assert.equal(serverState(agent({ state: "stopped" }), srv({ state: "on" })).words, "On");
  assert.equal(serverState(agent({ state: "elsewhere" }), srv({ state: "on" })).words, "On");
  assert.equal(serverState(running, srv({ state: "off", on: false })).words, "Off");
  assert.equal(serverState(running, srv({ state: "pending", on: false })).words, "Applies at the next turn");
  assert.equal(serverState(running, srv({ state: "needs-sign-in", on: false })).words, "Needs a sign-in");
});

test("Retry switches the failed server off and on, in that order", () => {
  assert.deepEqual(retryChanges("server:pypi-x"), [{ switch: { id: "server:pypi-x", on: false } }, { switch: { id: "server:pypi-x", on: true } }]);
});

test("the share switch is offered on a server that may see the share, and shown disabled with the reason on one that may not", () => {
  assert.deepEqual(serverShareSwitch(srv({ shareSwitch: "server:pypi-x:share" })), { id: "server:pypi-x:share", on: false, disabled: false });
  assert.deepEqual(serverShareSwitch(srv({ shareSwitch: "server:pypi-x:share", share: true })), { id: "server:pypi-x:share", on: true, disabled: false });
  for (const why of [
    "https://mcp.example.com/mcp (remote) is a remote server, another party's: it is never given your share folder",
    "npm:fetcher@1.0.0 reaches api.example.com, so it could send your files there: a server that reaches the network is never given your share folder",
    "npm:signs@1.0.0 signs in by OAuth, which a persistent agent cannot do in this version",
  ]) {
    assert.deepEqual(serverShareSwitch(srv({ shareRefusal: why })), { id: "", on: false, disabled: true, reason: why });
  }
  // Not a server, or a server the runner says nothing about: no switch.
  assert.equal(serverShareSwitch({ ...srv({}), kind: "knowledge" } as app.PersistentRosterEntry), null);
  assert.equal(serverShareSwitch(srv({})), null);
  // Under the switch: the refusal, or, while the agent's own share folder
  // is off, that the server sees it only while it is on.
  const on = { id: "server:pypi-x:share", on: true, disabled: false };
  const shareEntry = (state: string) => ({ id: "share", kind: "files", name: "Your share folder", grantedBy: "built-in", state, on: state !== "off", line: "" }) as app.PersistentRosterEntry;
  assert.equal(shareSwitchNote(agent({ roster: [shareEntry("on")] }), on), null);
  assert.equal(shareSwitchNote(agent({ roster: [shareEntry("off")] }), on), "Only while your share folder is on");
  assert.equal(shareSwitchNote(agent({}), { id: "", on: false, disabled: true, reason: "why" }), "why");
});

test("the list is read again while a server installs", () => {
  assert.equal(installingAny([agent({ state: "running", roster: [srv({ state: "installing" })] })]), true);
  assert.equal(installingAny([agent({ state: "running", roster: [srv({ state: "on" }), srv({ state: "failed" })] })]), false);
});

test("the share folder's line says whether a file added there is checked for malware", () => {
  assert.equal(shareCheckWords(false, true), ", and a file you add is checked for malware before it reads it");
  assert.equal(shareCheckWords(false, false), ", and reads a file you add unchecked: the malware check is off on this computer");
  assert.equal(shareCheckWords(true, true), ", and reads the files in it as they are");
});
