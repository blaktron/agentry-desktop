// A run on this computer, followed by its id (agentry-desktop#465): what the
// store takes from Go's reports, and what it never takes.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import { failed, finished, outcome, starting, stopAsked, withLine, withProgress, type RunProgress } from "../src/localRun.ts";

const report = (runId: string, ref: string, turns = 0): RunProgress => ({
  runId, ref, startedAt: 1, timeoutSeconds: 900, maxTurns: 60, turns, inputTokens: 0, outputTokens: 0,
});
const result = (runId: string, status = "ok", exitCode = 0) => ({ runId, status, exitCode }) as unknown as main.RunResultView;

test("the first report names the run, and later ones must carry its id", () => {
  let r = starting("acme/a@1.0.0");
  r = withProgress(r, report("r1", "acme/a@1.0.0"));
  assert.equal(r.runId, "r1");
  const other = withProgress(r, report("r2", "acme/a@1.0.0", 9));
  assert.equal(other, r, "a report of another run of the same release changed this one");
  r = withProgress(r, report("r1", "acme/a@1.0.0", 3));
  assert.equal(r.progress?.turns, 3);
});

test("a report of another release is not this run's", () => {
  const r = starting("acme/a@1.0.0");
  assert.equal(withProgress(r, report("r9", "acme/b@2.0.0")), r);
});

test("only this run's lines join its log, and none before it has begun", () => {
  let r = starting("acme/a@1.0.0");
  assert.equal(withLine(r, { runId: "r1", line: "early" }), r, "a line before the run began was kept");
  r = withProgress(r, report("r1", "acme/a@1.0.0"));
  r = withLine(r, { runId: "r1", line: "one" });
  r = withLine(r, { runId: "r2", line: "a line of another run" });
  r = withLine(r, { runId: "r1", line: "two" });
  assert.deepEqual(r.log, ["one", "two"]);
});

test("once over, nothing more is taken", () => {
  let r = withProgress(starting("acme/a@1.0.0"), report("r1", "acme/a@1.0.0"));
  r = finished(r, result("r1"));
  assert.equal(withLine(r, { runId: "r1", line: "late" }), r);
  assert.equal(withProgress(r, report("r1", "acme/a@1.0.0", 5)), r);
  assert.equal(failed(r, "late error"), r);
});

test("how it ended, for the card", () => {
  const begun = withProgress(starting("acme/a@1.0.0"), report("r1", "acme/a@1.0.0"));
  assert.deepEqual(outcome(begun), { line: "", tone: "" });
  assert.deepEqual(outcome(finished(begun, result("r1"))), { line: "Finished.", tone: "ok" });
  assert.deepEqual(outcome(finished(begun, result("r1", "error", 7))), { line: "It ended in an error (exit 7).", tone: "bad" });
  assert.deepEqual(outcome(failed(begun, "docker is not running")), { line: "The run did not finish.", tone: "bad" });
  const stopped = finished(stopAsked(begun), result("r1", "error", 130));
  assert.equal(stopped.stopped, true);
  assert.deepEqual(outcome(stopped), { line: "You stopped the run. Its receipts are written as far as it got.", tone: "warn" });
});
