import { test } from "node:test";
import assert from "node:assert/strict";
import { BUG_REPORT_THANKS, bugReportLength, bugReportOutcome, bugTextProblem } from "../src/bugReport.ts";

// Report bug's words (agentry-desktop#841): the text counted as the
// registry counts it, and one sentence, with Sign in where it helps, for
// each answer the binding returns.

test("counts the text as the registry does: trimmed, CRLF as one break, in characters", () => {
  assert.equal(bugReportLength("  ab\r\ncd  "), 5);
  assert.equal(bugReportLength("é".repeat(10)), 10);
  assert.equal(bugReportLength("🐛".repeat(3)), 3);
});

test("holds Submit back below 10 and above 4,000 characters, and says nothing about an empty box", () => {
  assert.equal(bugTextProblem(""), null);
  assert.equal(bugTextProblem("   "), null);
  assert.equal(bugTextProblem("too short"), "A bug report is at least 10 characters.");
  assert.equal(bugTextProblem("long enough"), null);
  assert.equal(bugTextProblem("x".repeat(4000)), null);
  assert.equal(bugTextProblem("x".repeat(4001)), "A bug report is at most 4,000 characters; this one is 4,001.");
});

test("thanks the person when filed, and says each refusal in one sentence", () => {
  assert.deepEqual(bugReportOutcome({ filed: true }), { tone: "ok", sentence: BUG_REPORT_THANKS });
  const signin = bugReportOutcome({ filed: false, reason: "signin" });
  assert.equal(signin.signIn, true);
  assert.match(signin.sentence, /^Sign in again to report a bug/);
  assert.equal(bugReportOutcome({ filed: false, reason: "absent" }).sentence, "This registry does not take bug reports.");
  assert.equal(bugReportOutcome({ filed: false, reason: "absent" }).signIn, undefined);
  assert.equal(
    bugReportOutcome({ filed: false, reason: "limited", message: "You have sent 10 bug reports in the last hour. Try again later." }).sentence,
    "You have sent 10 bug reports in the last hour. Try again later.",
  );
  assert.equal(bugReportOutcome({ filed: false, reason: "refused", message: "Activate your account first." }).sentence, "Not sent: Activate your account first.");
  assert.match(bugReportOutcome({ filed: false, reason: "unreachable" }).sentence, /did not take the report just now/);
  assert.match(bugReportOutcome({ filed: false, reason: "text" }).sentence, /10 to 4,000 characters/);
});
