// A run on this computer, followed by its id: what the
// store takes from Go's reports, and what it never takes.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import { MAX_RUN_LOG_LINES, droppedNote, failed, shownLog, finished, outcome, starting, stopAsked, withLine, withProgress, type RunProgress } from "../src/localRun.ts";

const report = (runId: string, ref: string, turns = 0): RunProgress => ({
  runId, ref, startedAt: 1, timeoutSeconds: 900, maxTurns: 60, turns, inputTokens: 0, outputTokens: 0, prepared: true,
});
const result = (runId: string, status = "ok", exitCode = 0) => ({ runId, status, exitCode }) as unknown as app.RunResultView;

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

test("a long log keeps its newest lines, shown after a note of how many went", () => {
  let r = withProgress(starting("acme/a@1.0.0"), report("r1", "acme/a@1.0.0"));
  for (let i = 1; i <= MAX_RUN_LOG_LINES; i++) r = withLine(r, { runId: "r1", line: `line ${i}` });
  assert.equal(r.log.length, MAX_RUN_LOG_LINES);
  assert.equal(r.log[0], "line 1", "a log at its limit lost a line");
  assert.equal(shownLog(r), r.log, "a log that lost nothing carries a note");
  r = withLine(r, { runId: "r1", line: "one more" });
  assert.equal(r.log.length, MAX_RUN_LOG_LINES);
  assert.deepEqual(shownLog(r).slice(0, 2), [droppedNote(1), "line 2"]);
  r = withLine(r, { runId: "r1", line: "and another" });
  assert.deepEqual(shownLog(r).slice(0, 2), [droppedNote(2), "line 3"]);
  assert.equal(r.log[r.log.length - 1], "and another");
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
  // Stopped while it was still being prepared: Go returns no result, and
  // nothing claims receipts.
  const early = failed(stopAsked(begun), "context canceled");
  assert.deepEqual(outcome(early), { line: "You stopped the run before the QuickAgent started.", tone: "warn" });
});

// A run that ended blocked (exit 18)
// is a warning that says who stopped it, never an error and never a pass.
test("a blocked run says who stopped it, as a warning", () => {
  const blocked = (reason: unknown, error = "") =>
    ({ runId: "r1", status: "error", exitCode: 18, exitClass: "blocked", reason, error }) as unknown as app.RunResultView;
  const begun = withProgress(starting("acme/a@1.0.0"), report("r1", "acme/a@1.0.0"));
  const agent = outcome(finished(begun, blocked({ code: "agent-blocked" }, "blocked: the agent could not do the task: the archive is encrypted.")));
  assert.deepEqual(agent, { line: "Blocked: the QuickAgent could not do the task with what this run had. It said: the archive is encrypted.", tone: "warn" });
  assert.equal(
    outcome(finished(begun, blocked({ code: "agent-blocked" }, "something else"))).line,
    "Blocked: the QuickAgent could not do the task with what this run had. Its output says why.",
  );
  for (const [detail, words] of [
    ["tools-unrecorded", "could not write this run's output folder"],
    ["inputs-unread", "could not read this run's inputs"],
    ["report-unwritten", "could not write this run's report"],
    ["a later word", "failed this run"],
  ]) {
    const o = outcome(finished(begun, blocked({ code: "runner-blocked", detail })));
    assert.equal(o.tone, "warn");
    assert.match(o.line, new RegExp(words.replace(/'/g, "'")));
  }
  assert.match(outcome(finished(begun, blocked(undefined))).line, /failed this run/);
  // Any other failure is still the error it was.
  assert.deepEqual(outcome(finished(begun, result("r1", "error", 14))), { line: "It ended in an error (exit 14).", tone: "bad" });
});
