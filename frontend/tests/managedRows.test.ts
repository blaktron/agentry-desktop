// Managed rows as the Settings parts draw them: for each part, a locked row,
// an organization default and a row no document sets.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  forbids, lockedCap, lockedMembers, ownKeysUsed, partNotes, PART_ROWS, providerLocks, rowNote, rowOf, valueWords,
  type ManagedPart, type ManagedRow,
} from "../src/managed.ts";

const row = (id: string, value: string, state: ManagedRow["state"] = "locked", applied = true): ManagedRow =>
  ({ id, state, value, organization: "acme", applied });

// A restrictive locked value and a default value for the first row of each part.
const CASES: [ManagedPart, string, string, string, string][] = [
  // part, row id, locked value, default value, the lock's sentence
  ["budgets", "run.timeout", "900", "1800", "The longest a run may take: at most 900 seconds."],
  ["where", "run.where", '["local"]', '["local","hosted"]', "Where runs may go: this computer."],
  ["runhosts", "run.where", '["local","hosted"]', '["local","runhost"]', "Where runs may go: this computer and In Agentry."],
  ["deliverables", "deliverables.destinations", '["folder"]', '["folder","s3"]', "Where a run may send its result: a folder."],
  ["destinations", "deliverables.destinations", '["folder","webhook"]', '["s3"]', "Where a run may send its result: a folder and a webhook."],
  ["malware", "malwareCheck", '"required"', '"optional"', "The malware check: required."],
  ["unsafe", "run.unsafeMode", '"forbidden"', '"allowed"', "Unsafe Mode: not allowed."],
  ["applications", "desktop.applications", '"forbidden"', '"allowed"', "Runs using applications on this computer: not allowed."],
  ["screen", "desktop.screen", '"forbidden"', '"allowed"', "Runs seeing and using your screen: not allowed."],
  ["agents", "agents.assistant", '"forbidden"', '"allowed"', "The Assistant: not allowed."],
  ["installs", "install.acceptRisk", '"forbidden"', '"allowed"', "Accepting a release's risk at install: not allowed."],
  ["scheduled", "schedules", '"forbidden"', '"allowed"', "Scheduled runs: not allowed."],
  ["updates", "updates.minimumVersion", '"0.7.30"', '"0.7.30"', "The oldest Agentry client that may run: version 0.7.30 or later."],
  ["bug-report", "bugReports", '"forbidden"', '"allowed"', "Bug reports: not allowed."],
  ["provider", "model.source", '{"source":"organization-key"}', '{"source":"agentry-credits"}',
    "What pays for your models: acme's own model key, which stays with the registry."],
  ["review-key", "review.source", '{"source":"organization-key","key":{"id":"k","rev":2}}', '{"source":"agentry-credits"}',
    "What pays for AI Review: a key acme provides, kept in your keychain and never shown."],
  ["search-key", "webSearch.source", '{"source":"agentry-credits"}', '{"source":"organization-key","provider":"brave"}',
    "What pays for web search: acme's credits, through Agentry."],
  ["coding-tools", "codingTools.allowed", '["claude-code","codex"]', '["codex"]', "The coding tools runs may use: Claude Code and Codex."],
  ["assistant-config", "agents.assistant.name", '"Ada"', '"Ada"', "The Assistant's name: “Ada”."],
  ["coder-config", "agents.codingAgent.servers", '[{"name":"tickets","url":"https://mcp.acme.example/t","auth":"none"}]', '[]',
    "MCP servers on the Coding agent: tickets (mcp.acme.example)."],
];

for (const [part, id, lockedValue, defaultValue, sentence] of CASES) {
  test(`${part}: a lock names the organization and says what it sets`, () => {
    const [note] = partNotes([row(id, lockedValue)], part);
    assert.ok(note, "a locked row is drawn");
    assert.equal(note.badge, "Managed by acme");
    assert.equal(note.lines[0], sentence);
    assert.equal(note.use, "", "a lock offers no way to the organization's setting");
  });

  test(`${part}: a default is Set by the organization and offers Use its setting`, () => {
    const [note] = partNotes([row(id, defaultValue, "default")], part);
    assert.ok(note);
    assert.equal(note.badge, "Set by acme");
    assert.equal(note.use, "Use acme's setting");
    assert.equal(note.disabled, false, "a default never disables the control");
  });

  test(`${part}: a row no document sets draws nothing`, () => {
    assert.deepEqual(partNotes([], part).filter((n) => PART_ROWS[part].includes(n.id)), []);
    assert.deepEqual(partNotes(null, part), []);
    assert.deepEqual(partNotes([row("some.other", '"forbidden"')], part), []);
  });
}

test("a lock on a restrictive choice disables the control; one that leaves the choice open does not", () => {
  assert.equal(rowNote(row("malwareCheck", '"required"'))!.disabled, true);
  assert.equal(rowNote(row("schedules", '"forbidden"'))!.disabled, true);
  const open = rowNote(row("schedules", '"allowed"'))!;
  assert.equal(open.disabled, false);
  assert.deepEqual(open.lines, ["Scheduled runs: allowed.", "Acme leaves this to you."]);
  assert.equal(forbids(row("schedules", '"allowed"')), false);
});

test("a cap or a set lock keeps the control and names what it allows", () => {
  const cap = row("run.maxTurns", "40");
  assert.equal(rowNote(cap)!.disabled, false);
  assert.equal(lockedCap(cap), 40);
  assert.equal(lockedCap(row("run.maxTurns", "40", "default")), null);
  assert.equal(rowNote(cap)!.lines[0], "The most turns a run may take: at most 40 turns.");
  const set = row("run.where", '["local","target-host"]');
  assert.deepEqual(lockedMembers(set), ["local", "target-host"]);
  assert.equal(valueWords(set), "this computer and a target host");
  assert.equal(lockedMembers(row("run.where", '["local"]', "default")), null);
});

test("an install source list is worded, and a row this client does not apply says so", () => {
  const any = row("install.sources", '{"scope":"any"}');
  assert.equal(valueWords(any), "anywhere");
  assert.equal(valueWords(row("install.sources", '{"scope":"organization-and-built-ins","namespaces":["acme","tools"]}')),
    "the organization, the built-ins and acme and tools");
  assert.equal(valueWords(row("install.sources", '{"scope":"organization-and-built-ins","namespaces":[]}')), "the organization and the built-ins");
  const unapplied = rowNote(row("bugReports", '"forbidden"', "locked", false))!;
  assert.equal(unapplied.lines.at(-1), "This version of the agentry command cannot hold you to it, so no run starts on this computer until Agentry is updated.");
  const unappliedDefault = rowNote(row("bugReports", '"forbidden"', "default", false))!;
  assert.equal(unappliedDefault.lines.at(-1), "This version of the agentry command does not apply it yet.");
  assert.equal(rowNote(row("bugReports", '"forbidden"'))!.lines.length, 1);
});

test("the models-and-keys rows are worded, and a source lock other than the member's own disables the key", () => {
  assert.equal(valueWords(row("model.providers", '["anthropic","openai","custom"]')), "Anthropic, OpenAI and an endpoint of your own");
  assert.equal(valueWords(row("model.signIns", "[]")), "none");
  assert.equal(valueWords(row("model.signIns", '["claude-signin","chatgpt-plan"]')), "a Claude sign-in and a ChatGPT plan");
  assert.equal(valueWords(row("model.default", '{"provider":"anthropic","model":"claude-sonnet-5-5"}')), "claude-sonnet-5-5 on Anthropic");
  assert.equal(valueWords(row("webSearch.source", '{"source":"organization-key","provider":"brave","key":{"id":"k","rev":1}}')),
    "a key acme provides (Brave), kept in your keychain and never shown");
  assert.equal(valueWords(row("review.source", '{"source":"off"}')), "off");
  for (const src of ["organization-key", "agentry-credits", "off"]) {
    assert.equal(rowNote(row("review.source", `{"source":"${src}"}`))!.disabled, true, src);
  }
  assert.equal(rowNote(row("review.source", '{"source":"member"}'))!.disabled, false, "the member's own key stays usable");
  assert.equal(valueWords(row("review.source", '{"source":"member"}')), "your own keys");
  const httpsOnly = rowNote(row("model.customEndpoints", '"https-only"'))!;
  assert.deepEqual(httpsOnly.lines, ["A model endpoint of your own: allowed over https only."], "https only does not leave the choice to the member");
  const order = partNotes([row("model.customEndpoints", '"forbidden"'), row("model.providers", '["openai"]'), row("model.source", '{"source":"member"}', "default")], "provider")
    .map((n) => n.id);
  assert.deepEqual(order, ["model.source", "model.providers", "model.customEndpoints"]);
});

test("the provider tiles a lock leaves out say who does not allow them", () => {
  const locks = providerLocks([row("model.providers", '["anthropic","custom"]'), row("model.customEndpoints", '"forbidden"')]);
  assert.equal(locks.openai, "Not allowed by acme");
  assert.equal(locks.anthropic, undefined);
  assert.equal(locks.custom, "Not allowed by acme", "a forbidden endpoint locks the custom tile even when the provider list has it");
  assert.deepEqual(providerLocks([row("model.providers", '["openai"]', "default")]), {}, "a default locks no tile");
  assert.deepEqual(providerLocks(null), {});
  assert.equal(ownKeysUsed(row("review.source", '{"source":"member"}', "default")), true);
  assert.equal(ownKeysUsed(row("review.source", '{"source":"agentry-credits"}', "default")), false);
  assert.equal(ownKeysUsed(null), true);
});

test("a row is found by its id", () => {
  const rows = [row("malwareCheck", '"required"'), row("schedules", '"allowed"')];
  assert.equal(rowOf(rows, "schedules")!.id, "schedules");
  assert.equal(rowOf(rows, "messengers"), null);
  assert.equal(rowOf(undefined, "messengers"), null);
});

test("the agent configuration rows say what they set", () => {
  assert.equal(valueWords(row("agents.assistant.runOnStart", "true")), "on");
  assert.equal(valueWords(row("agents.assistant.runOnStart", "false", "default")), "off");
  assert.equal(valueWords(row("agents.assistant.personality", '"Brief."')), "6 characters of its own");
  assert.equal(valueWords(row("agents.assistant.knowledge", '["handbook","sales"]')), "handbook and sales");
  assert.equal(valueWords(row("codingTools.models", '{"codex":"gpt-5.5","claude-code":"claude-sonnet-5-5"}')),
    "gpt-5.5 for Codex and claude-sonnet-5-5 for Claude Code");
  assert.equal(rowNote(row("agents.unreviewed", '"forbidden"'))!.disabled, true);
  assert.ok(PART_ROWS.agents.includes("agents.unreviewed"));
});
