// Publisher Earnings in the desktop: the run setup's
// line and the star ask say the site's words, and only a
// run that finished and succeeded asks to be starred.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  PUBLISHER_EARNS_AFTER,
  PUBLISHER_EARNS_SENTENCE,
  starAskSentence,
  starAskWanted,
  starredSentence,
} from "../src/earnings.ts";

test("the run setup's line is the site's, with what you pay unchanged", () => {
  assert.equal(PUBLISHER_EARNS_SENTENCE, "Its publisher earns a share of the credits runs of it spend.");
  assert.equal(PUBLISHER_EARNS_AFTER, "What you pay does not change.");
  // The program's name is Publisher Earnings, never Creator.
  assert.doesNotMatch(PUBLISHER_EARNS_SENTENCE + PUBLISHER_EARNS_AFTER, /creator/i);
});

test("the star ask names the QuickAgent in the site's words", () => {
  assert.equal(starAskSentence("acme/reporter"), "If this run helped, star acme/reporter. Stars help other people find it.");
  assert.equal(starredSentence("acme/reporter"), "Starred acme/reporter.");
});

test("only a run that finished and succeeded asks to be starred", () => {
  const done = { status: "finished", outcome: "succeeded", final: true, ref: "acme/reporter@1.0.0" };
  assert.equal(starAskWanted(done), true);
  // A closed session finishes as a run does, with its session's facts.
  const closed = { ...done, ref: "acme/chatter@2.0.0", mode: "interactive", session: { idleSeconds: 300, transport: "" } };
  assert.equal(starAskWanted(closed), true);
  assert.equal(starAskWanted({ ...closed, outcome: "failed" }), false);
  for (const [why, run] of [
    ["failed", { ...done, outcome: "failed" }],
    ["errored", { ...done, outcome: "error" }],
    ["cancelled", { ...done, status: "cancelled", outcome: undefined }],
    ["still running", { ...done, status: "running", outcome: undefined, final: false }],
    ["queued", { ...done, status: "queued", outcome: undefined, final: false }],
    ["lost touch", { ...done, status: "running", outcome: undefined }],
    ["finished, its last word not in", { ...done, final: false }],
    ["no QuickAgent named", { ...done, ref: "" }],
  ] as const) {
    assert.equal(starAskWanted(run), false, why);
  }
});
