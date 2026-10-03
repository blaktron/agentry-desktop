// Edit with AI's form (agentry-desktop#501): the texts' bound, and each
// way's gate: In Agentry on the registry's word for this payer, on this
// computer on the person's own key and docker (D10).
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_REVISION_TEXT_BYTES, canStart, firstWhere, revisionBytes, textsProblem, type ReviseWays } from "../src/revise.ts";

const pro: ReviseWays = { agentryOffered: true, agentryAvailable: true, keyReady: false, dockerOk: true };
const standard: ReviseWays = { agentryOffered: true, agentryAvailable: false, keyReady: true, dockerOk: true };

test("the texts are counted as sent, at least one, each within the bound", () => {
  assert.equal(revisionBytes("  a\r\nb  "), 3);
  assert.equal(revisionBytes("é"), 2);
  assert.match(textsProblem(" ", "\n") ?? "", /Say what should change/);
  assert.equal(textsProblem("Add dates.", ""), null);
  assert.equal(textsProblem("", "It misses dates."), null);
  assert.match(textsProblem("x".repeat(MAX_REVISION_TEXT_BYTES + 1), "") ?? "", /at most 4000/);
  assert.match(textsProblem("", "é".repeat(2001)) ?? "", /Problems with this draft/);
});

test("In Agentry goes first for a payer who may; anyone else starts on this computer", () => {
  assert.equal(firstWhere(pro), "agentry");
  assert.equal(firstWhere(standard), "here");
  assert.equal(firstWhere({ ...pro, agentryOffered: false }), "here");
});

test("each way starts only when it is ready", () => {
  assert.equal(canStart("agentry", pro, "Add dates.", ""), true);
  assert.equal(canStart("agentry", standard, "Add dates.", ""), false);
  // On this computer: anyone, with their own key and docker.
  assert.equal(canStart("here", standard, "Add dates.", ""), true);
  assert.equal(canStart("here", pro, "Add dates.", ""), false);
  assert.equal(canStart("here", { ...standard, dockerOk: false }, "Add dates.", ""), false);
  assert.equal(canStart("agentry", pro, "", ""), false);
});
