import assert from "node:assert/strict";
import { test } from "node:test";
import { runhostNameProblem, runhostOf, signInWords, unsafeModeWords } from "../src/runhosts.ts";

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
