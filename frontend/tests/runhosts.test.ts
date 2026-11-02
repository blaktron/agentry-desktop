import assert from "node:assert/strict";
import { test } from "node:test";
import { runhostNameProblem, runhostOf, signInWords, unsafeModeWords, updateWords } from "../src/runhosts.ts";

test("a runhost's name follows the CLI's rule", () => {
  for (const ok of ["lab", "a", "build-01", "x".repeat(32)]) assert.equal(runhostNameProblem(ok), "", ok);
  for (const bad of ["", "Lab", "-lab", "lab-", "lab_1", "x".repeat(33), "lab server"]) assert.notEqual(runhostNameProblem(bad), "", bad);
});

test("each plan sign-in says where it is kept", () => {
  assert.match(signInWords("claude").blurb, /on the runhost/);
  assert.match(signInWords("chatgpt").blurb, /store, encrypted/);
  assert.match(signInWords("copilot").title, /Copilot/);
});

test("Unsafe Mode's words say what each setting does", () => {
  assert.match(unsafeModeWords(true), /^on\. .*without the command catalogue/);
  assert.match(unsafeModeWords(false), /^off\./);
});

test("a runhost default names its runhost, and nothing else does", () => {
  assert.equal(runhostOf("runhost:lab"), "lab");
  assert.equal(runhostOf("here"), "");
  assert.equal(runhostOf("agentry"), "");
  assert.equal(runhostOf(undefined), "");
});

test("a runhost's last update says what happened and when the person must act", () => {
  assert.deepEqual(updateWords(undefined), { line: "", attention: false });
  assert.match(updateWords({ at: "t", from: "0.7.9", to: "0.7.10", updated: true }).line, /^Updated from CLI 0\.7\.9 to 0\.7\.10\.$/);
  assert.match(updateWords({ at: "t", to: "0.7.10", updated: true, waitsForJobs: 2 }).line, /once its 2 running jobs finish/);
  assert.equal(updateWords({ at: "t", to: "0.7.10", updated: false }).attention, false);
  const behind = updateWords({ at: "t", updated: false, deviceBehind: "0.7.11" });
  assert.ok(behind.attention && /update this app/.test(behind.line));
  const admin = updateWords({ at: "t", updated: false, needsAttention: ["its doctor reports a failed check: agentry runhost add lab … repairs it"] });
  assert.ok(admin.attention && admin.line.startsWith("Needs attention: its doctor"));
  assert.ok(updateWords({ at: "t", updated: false, problem: "no route to host" }).attention);
});
