// The Assistant's Tasks view: a task's cron in words, its row, its
// operations.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import { cronWords, taskOps, taskRow, when } from "../src/tasksView.ts";

test("a cron's common shapes read in words, and anything else reads none", () => {
  const cases: [string, string | null][] = [
    ["0 8 * * 1-5", "on weekdays at 08:00"],
    ["30 7 * * *", "every day at 07:30"],
    ["0 18 * * 0", "every Sunday at 18:00"],
    ["0 18 * * 7", "every Sunday at 18:00"],
    ["15 9 * * 1,3,5", "on Monday, Wednesday and Friday at 09:15"],
    ["0 10 * * 6,0", "on weekends at 10:00"],
    ["0 9 1 * *", "on day 1 of every month at 09:00"],
    ["*/15 * * * *", "every 15 minutes"],
    ["*/40 * * * *", null],
    ["*/90 * * * *", null],
    ["0 0 * * *", "every day at 00:00"],
    ["0 12 * * 1,2", "on Monday and Tuesday at 12:00"],
    ["0 * * * *", "every hour, on the hour"],
    ["5 * * * *", "every hour at 5 past"],
    ["0 9 * 1 *", null],
    ["0 9 1 * 1", null],
    ["61 9 * * *", null],
    ["0 9 * *", null],
    ["@daily", null],
  ];
  for (const [expr, want] of cases) assert.equal(cronWords(expr), want, expr);
});

const task = (t: Partial<app.PersistentTaskView>): app.PersistentTaskView =>
  ({ id: "t1", name: "Morning summary", cron: "0 8 * * 1-5", zone: "Europe/Oslo", instruction: "Summarise my day.", state: "on", madeBy: "agent", ...t }) as app.PersistentTaskView;

test("a task's row: its schedule in words and as written, its zone, its next and last fire and that answer's first line", () => {
  const r = taskRow(task({ next: "2026-10-24T06:00:00Z", lastFire: "2026-10-23T06:00:00Z", lastResult: "Three meetings today." }), "en-GB");
  assert.equal(r.schedule, "on weekdays at 08:00");
  assert.equal(r.cron, "0 8 * * 1-5");
  assert.equal(r.zone, "Europe/Oslo");
  assert.ok(r.next.length > 0 && r.next !== "Paused");
  assert.ok(r.last.length > 0 && r.last !== "Not yet");
  assert.equal(r.answer, "Three meetings today.");
  assert.equal(r.madeBy, "Made by the agent");
  const p = taskRow(task({ state: "paused", next: "2026-10-24T06:00:00Z" }));
  assert.equal(p.next, "Paused");
  assert.equal(p.last, "Not yet");
  assert.equal(taskRow(task({ cron: "0 9 * 1 *" })).schedule, "on its own schedule");
});

test("a task offers Pause or Resume, and Remove, and never Add", () => {
  assert.deepEqual(taskOps({ state: "on" }).map((o) => o.op), ["pause", "remove"]);
  assert.deepEqual(taskOps({ state: "paused" }).map((o) => o.op), ["resume", "remove"]);
  assert.equal(when("not a time"), "");
  assert.equal(when(undefined), "");
});
