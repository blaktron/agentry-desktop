// Managed rows as the Settings parts draw them: for each part, a locked row,
// an organization default and a row no document sets.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  forbids, lockedCap, lockedMembers, partNotes, PART_ROWS, rowNote, rowOf, valueWords,
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
  assert.equal(unapplied.lines.at(-1), "This version of the agentry command does not apply it yet, so runs are not held to it.");
  assert.equal(rowNote(row("bugReports", '"forbidden"'))!.lines.length, 1);
});

test("a row is found by its id", () => {
  const rows = [row("malwareCheck", '"required"'), row("schedules", '"allowed"')];
  assert.equal(rowOf(rows, "schedules")!.id, "schedules");
  assert.equal(rowOf(rows, "messengers"), null);
  assert.equal(rowOf(undefined, "messengers"), null);
});
