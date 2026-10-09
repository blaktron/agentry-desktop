// The Scheduled page and the run setup's When step (plan
// quickagent-scheduling.md §7, D2, D11, D12, D19; D-073;
// agentry-desktop#743).
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  byDay, defaultWhen, gridRange, monthGrid, nextWholeHour, outcomeLabel, outcomeTone, sameWhen, scheduleState, signedOutUnavailable,
  whenForm, whenProblem, whenSpec, whenUnavailable, zoneChoices,
} from "../src/scheduled.ts";
import { whereRan } from "../src/recentRuns.ts";

const now = new Date(2026, 9, 9, 14, 37); // Friday 9 October 2026, 14:37 local
const ready = { available: true, zone: "Europe/London", signedOut: { available: true }, lingerOn: false };

test("When starts on Now; Schedule a new run starts on Repeat, every day at the next whole hour", () => {
  assert.equal(defaultWhen(now, "Europe/London").choice, "now");
  const w = defaultWhen(now, "Europe/London", "repeat");
  assert.deepEqual([w.choice, w.repeat, w.time, w.zone, w.end, w.signedOut], ["repeat", "daily", "15:00", "Europe/London", "never", false]);
  assert.equal(nextWholeHour(new Date(2026, 9, 9, 23, 5)).getDate(), 10);
  assert.equal(whenSpec(defaultWhen(now, "UTC")), null);
});

test("When's answers become what Go takes", () => {
  const base = defaultWhen(now, "Europe/London", "repeat");
  assert.deepEqual(whenSpec({ ...base, choice: "later", date: "2026-10-20", time: "08:00" }), { kind: "once", date: "2026-10-20", time: "08:00", zone: "Europe/London" });
  assert.deepEqual(whenSpec(base), { kind: "daily", time: "15:00", zone: "Europe/London", end: "never" });
  assert.deepEqual(whenSpec({ ...base, repeat: "weekly", days: ["thu", "tue"], end: "after", times: 4 }),
    { kind: "weekly", time: "15:00", zone: "Europe/London", end: "after", days: ["tue", "thu"], times: 4 });
  assert.deepEqual(whenSpec({ ...base, repeat: "monthly", monthDay: 31, end: "on", until: "2027-01-31" }),
    { kind: "monthly", time: "15:00", zone: "Europe/London", end: "on", monthDay: 31, until: "2027-01-31" });
});

test("a When that cannot be scheduled says what is missing", () => {
  const base = defaultWhen(now, "UTC", "repeat");
  assert.equal(whenProblem(defaultWhen(now, "UTC")), null);
  assert.equal(whenProblem(base), null);
  assert.equal(whenProblem({ ...base, time: "" }), "Choose a time.");
  assert.equal(whenProblem({ ...base, choice: "later", date: "" }), "Choose a day.");
  assert.equal(whenProblem({ ...base, repeat: "weekly", days: [] }), "Choose at least one day of the week.");
  assert.equal(whenProblem({ ...base, repeat: "monthly", monthDay: 0 }), "Choose a day of the month, 1 to 31.");
  assert.equal(whenProblem({ ...base, end: "on", until: "" }), "Choose the last day it runs.");
  assert.equal(whenProblem({ ...base, end: "after", times: 0 }), "Choose how many runs, one or more.");
  assert.equal(whenProblem({ ...base, zone: "" }), "Choose a time zone.");
});

test("When is offered only for an autonomous run on this computer of a registry release, with the command installed", () => {
  const s = { modes: ["autonomous"], mode: "autonomous", inAgentry: false, local: false, availability: ready };
  assert.equal(whenUnavailable(s), null);
  assert.match(whenUnavailable({ ...s, modes: ["interactive"] })!, /^Needs you there to answer/);
  assert.match(whenUnavailable({ ...s, modes: ["autonomous", "interactive"], mode: "interactive" })!, /runs autonomous/);
  assert.match(whenUnavailable({ ...s, inAgentry: true })!, /runs on this computer/);
  assert.match(whenUnavailable({ ...s, local: true })!, /agentry schedule add --local/);
  assert.match(whenUnavailable({ ...s, availability: null })!, /^Checking/);
  assert.equal(whenUnavailable({ ...s, availability: { ...ready, available: false, reason: "the agentry command was not found" } }), "the agentry command was not found");
});

test("the signed-out box says why it is off", () => {
  assert.equal(signedOutUnavailable(ready), null);
  assert.equal(signedOutUnavailable({ signedOut: { available: false, reason: "--signed-out is not available on windows: Docker Desktop stops when you sign out" } }),
    "Docker Desktop stops when you sign out.");
  assert.equal(signedOutUnavailable({ signedOut: { available: false, reason: "running while signed out needs Docker Engine: Docker Desktop stops when you sign out" } }),
    "Needs Docker Engine: Docker Desktop stops when you sign out.");
  assert.match(signedOutUnavailable({ signedOut: { available: false, reason: "running while signed out needs systemd 256 or later" } })!, /systemd 256/);
  assert.match(signedOutUnavailable(null)!, /^Checking/);
});

test("a stored repeat opens in Edit as it was", () => {
  const f = whenForm({ kind: "weekly", time: "22:00", days: ["tue"], end: "after", times: 3, zone: "America/New_York" }, true, now, "UTC");
  assert.deepEqual([f.choice, f.repeat, f.days, f.time, f.end, f.times, f.zone, f.signedOut], ["repeat", "weekly", ["tue"], "22:00", "after", 3, "America/New_York", true]);
  const once = whenForm({ kind: "once", date: "2026-10-20", time: "08:00", zone: "UTC" }, false, now, "UTC");
  assert.deepEqual([once.choice, once.date, once.time], ["later", "2026-10-20", "08:00"]);
  assert.deepEqual(whenSpec(f), { kind: "weekly", time: "22:00", zone: "America/New_York", end: "after", days: ["tue"], times: 3 });
});

test("a past occurrence says what happened, and nothing reads as done when nothing ran", () => {
  assert.equal(outcomeLabel({ outcome: "ran" }), "Ran");
  assert.equal(outcomeLabel({ outcome: "ran", manual: true }), "Ran (Run now)");
  assert.equal(outcomeLabel({ outcome: "failed", exitCode: 13, exitClass: "docker" }), "Failed (docker, exit 13)");
  assert.equal(outcomeLabel({ outcome: "failed" }), "Failed");
  assert.match(outcomeLabel({ outcome: "missed" }), /^Missed: the computer was off/);
  assert.equal(outcomeLabel({ outcome: "due" }), "Due");
  assert.deepEqual(["ran", "failed", "missed", "due"].map(outcomeTone), ["ok", "bad", "warn", "due"]);
});

test("the month grid starts on Monday and covers whole weeks", () => {
  const weeks = monthGrid(2026, 9, now); // October 2026: the 1st is a Thursday
  assert.equal(weeks[0][0].date, "2026-09-28");
  assert.equal(weeks[0][3].date, "2026-10-01");
  assert.equal(weeks[0][3].inMonth, true);
  assert.equal(weeks[0][0].inMonth, false);
  assert.equal(weeks.length, 5);
  assert.equal(weeks[weeks.length - 1][6].date, "2026-11-01");
  assert.ok(weeks.flat().some((d) => d.today && d.date === "2026-10-09"));
  const r = gridRange(weeks);
  assert.ok(new Date(r.from) <= new Date(2026, 8, 28) && new Date(r.to) > new Date(2026, 10, 1));
});

test("fires fall on their local day", () => {
  const at = new Date(2026, 9, 9, 9, 0).toISOString();
  const m = byDay([{ scheduleId: "a", name: "digest", ref: "acme/digest@1.0.0", at, outcome: "ran" }, { scheduleId: "a", name: "digest", ref: "", at: "nonsense", outcome: "ran" }]);
  assert.equal(m.get("2026-10-09")?.length, 1);
  assert.equal(m.size, 1);
});

test("a schedule's state, and a run it fired marked Scheduled", () => {
  assert.equal(scheduleState({ paused: true }), "Paused");
  assert.equal(scheduleState({ ended: true, paused: true }), "Ended");
  assert.match(scheduleState({ signedOut: true }), /^Keys not sealed/);
  assert.equal(scheduleState({ signedOut: true, keysSealed: true }), "Runs while you are signed out too");
  assert.equal(scheduleState({}), "Runs while you are signed in");
  assert.equal(whereRan({ where: "here", scheduled: true }), "Scheduled, on this computer");
  assert.equal(whereRan({ where: "here" }), "On this computer");
});

test("this computer's zone is offered first", () => {
  assert.deepEqual(zoneChoices("Europe/London", ["America/New_York", "Europe/London"]), ["Europe/London", "America/New_York"]);
  assert.deepEqual(zoneChoices("", ["UTC"]), ["UTC"]);
});

test("Edit sends When only when it changed, since a changed repeat starts afresh", () => {
  const stored = { kind: "weekly", time: "22:00", days: ["tue", "thu"], end: "after", times: 3, zone: "Europe/London" };
  const f = whenForm(stored, false, now, "UTC");
  assert.equal(sameWhen(whenSpec(f), stored), true);
  assert.equal(sameWhen(whenSpec({ ...f, days: ["thu", "tue"] }), stored), true);
  assert.equal(sameWhen(whenSpec({ ...f, time: "21:00" }), stored), false);
  assert.equal(sameWhen(whenSpec({ ...f, times: 4 }), stored), false);
  const daily = { kind: "daily", time: "09:00", zone: "UTC", end: "never" };
  assert.equal(sameWhen(whenSpec(whenForm(daily, false, now, "UTC")), daily), true);
  assert.equal(sameWhen(null, daily), false);
});
