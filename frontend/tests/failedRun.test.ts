// What a run that did not finish shows: the five things the failure view
// names, from a run on this computer, a run In Agentry, and a run read back
// from the history.
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { didFail, failureHeadline, failureOf, hostedFailureInput, localFailureInput, reportText, retryLine, rowFailureInput, sentence } from "../src/failedRun.ts";

const harness = {
  runId: "ab12cd34", status: "error", exitCode: 14, exitClass: "harness",
  exitClassMeaning: "the agent's harness failed or could not reach its model", retryLikelyHelps: true,
  error: "harness: the model answered 503", ref: "acme/agent@1.2.0", outDir: "/home/me/out", receiptsDir: "/home/me/out/receipts",
} as unknown as app.RunResultView;

test("a run on this computer that failed shows what, which class and why, the cause and the receipts", () => {
  const f = failureOf(localFailureInput(harness));
  assert.ok(f);
  assert.equal(f.what, "acme/agent");
  assert.equal(f.version, "1.2.0");
  assert.equal(f.where, "On this computer");
  assert.equal(f.className, "harness");
  assert.equal(f.meaning, "the agent's harness failed or could not reach its model");
  assert.equal(f.retryLikelyHelps, true);
  assert.equal(f.cause, "harness: the model answered 503");
  assert.deepEqual(f.evidence, ["The receipts are in /home/me/out/receipts."]);
  assert.equal(failureHeadline(f), "Failed (harness): the agent's harness failed or could not reach its model.");
  assert.match(retryLine(f), /^Running it again with the same answers may well succeed/);
});

test("a class whose retry hint is false says so, and a run that ended ok, stopped or cancelled is no failure", () => {
  const policy = { ...harness, exitCode: 11, exitClass: "policy", exitClassMeaning: "refused by the operator policy or a missing secret before the agent started", retryLikelyHelps: false } as unknown as app.RunResultView;
  const f = failureOf(localFailureInput(policy))!;
  assert.match(retryLine(f), /^Running it again with the same answers would most likely fail the same way/);
  assert.equal(failureOf(localFailureInput({ ...harness, status: "ok" } as unknown as app.RunResultView)), null);
  assert.equal(failureOf(localFailureInput(harness, true)), null, "a stopped run is not a failure");
  assert.equal(didFail({ where: "agentry", outcome: "succeeded" }), false);
  assert.equal(didFail({ where: "agentry", outcome: "", status: "cancelled" }), false);
  assert.equal(didFail({ where: "agentry", outcome: "failed" }), true);
  assert.equal(didFail({ where: "here", outcome: "blocked" }), true, "a blocked run shows the failure view in the class's words");
});

test("a run with no class known says what it knows, and a run that ended before it had a folder says it left no receipts", () => {
  const f = failureOf({ where: "here", status: "error", exitCode: 7 })!;
  assert.equal(f.className, "exit 7");
  assert.equal(failureHeadline(f), "Failed (exit 7).");
  assert.equal(retryLine(f), "");
  assert.deepEqual(f.evidence, ["The run ended before it had a folder, so it left no receipts."]);
  const none = failureOf({ where: "here", status: "error", cause: "docker is not running" })!;
  assert.equal(none.className, "");
  assert.equal(failureHeadline(none), "The run did not finish.");
  assert.equal(none.cause, "docker is not running");
});

test("a row read back from the history carries its class words, its cause and its kept log", () => {
  const row = {
    id: "ab12cd34", kind: "run", where: "here", ref: "acme/agent@1.2.0", outcome: "error", exitCode: 15, exitClass: "timeout",
    exitClassMeaning: "the run's time budget ran out", retryLikelyHelps: false, cause: "timeout: 900s elapsed", logKept: true, tone: "bad", kept: true, source: "local",
  } as unknown as app.RecentRunView;
  const f = failureOf(rowFailureInput(row, "/home/me/out"))!;
  assert.equal(f.className, "timeout");
  assert.equal(f.cause, "timeout: 900s elapsed");
  assert.deepEqual(f.evidence, ["The receipts are in the run folder, /home/me/out, under receipts/.", "The run's log is kept with it below."]);
  const stopped = { ...row, outcome: "stopped" } as unknown as app.RecentRunView;
  assert.equal(failureOf(rowFailureInput(stopped)), null);
  const draft = { ...row, kind: "draft", ref: "" } as unknown as app.RecentRunView;
  assert.equal(failureOf(rowFailureInput(draft))!.what, "A draft by the Drafter");
});

test("a run In Agentry points at its run page, and its outputs folder when they came down", () => {
  const run = {
    id: "1111", ref: "acme/agent@1.2.0", status: "finished", outcome: "failed", exitClass: "budget",
    exitClassMeaning: "a turn, cost or usage budget ran out before the agent finished", retryLikelyHelps: false,
    error: "budget: 60 turns", outDir: "/home/me/out", outputs: [], final: true,
  } as unknown as app.HostedRunView;
  const f = failureOf(hostedFailureInput(run))!;
  assert.equal(f.where, "In Agentry");
  assert.deepEqual(f.evidence, ["The run page In Agentry holds the run's record and its receipt.", "Its outputs came down into /home/me/out."]);
  assert.equal(failureOf(hostedFailureInput({ ...run, outcome: "succeeded" } as unknown as app.HostedRunView)), null);
});

test("the bug report starts from the facts, never the task or the result", () => {
  const f = failureOf(localFailureInput(harness))!;
  const text = reportText(f, "ab12cd34");
  assert.match(text, /^acme\/agent 1\.2\.0 failed on this computer with exit class harness\.\n/);
  assert.match(text, /What that means: The agent's harness failed or could not reach its model\.\n/);
  assert.match(text, /Cause: harness: the model answered 503\nRun: ab12cd34\n/);
  assert.match(text, /What I was doing:\n$/);
  assert.equal(sentence("  the run ended  "), "The run ended.");
  assert.equal(sentence("Already a sentence."), "Already a sentence.");
  assert.equal(sentence(""), "");
});
