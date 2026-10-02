// The Writing Voice's choices and words (plan writing-voice.md §5, §8, D11,
// D12; agentry-desktop#510): Write in my voice is drawn only where the voice
// the run would use exists, ticked from that voice's own switch; the editor
// holds a voice to the format's rule; the price is the registry's.
import { test } from "node:test";
import assert from "node:assert/strict";
import { findingText, hostedVoiceCost, voiceChoice, voiceTextProblem } from "../src/voice.ts";

test("a run on this computer is offered this computer's voice, ticked from its switch", () => {
  assert.deepEqual(voiceChoice(null, false), { offered: false, ticked: false });
  assert.deepEqual(voiceChoice({ local: { exists: false, alwaysAttach: false } }, false), { offered: false, ticked: false });
  assert.deepEqual(voiceChoice({ local: { exists: true, alwaysAttach: true } }, false), { offered: true, ticked: true });
  assert.deepEqual(voiceChoice({ local: { exists: true, alwaysAttach: false } }, false), { offered: true, ticked: false });
  // A voice that will not open (signed out of its key's registry) is not
  // offered: the run would refuse it.
  assert.deepEqual(voiceChoice({ local: { exists: true, alwaysAttach: true, problem: "did not open" } }, false).offered, false);
});

test("a run In Agentry is offered the account's live voice only, ticked from the account's switch", () => {
  const local = { exists: true, alwaysAttach: true };
  assert.deepEqual(voiceChoice({ local, account: null }, true), { offered: false, ticked: false });
  assert.deepEqual(voiceChoice({ local, account: { live: null, alwaysAttach: true } }, true), { offered: false, ticked: false });
  assert.deepEqual(voiceChoice({ local, account: { live: { digest: "sha256:a" }, alwaysAttach: false } }, true), { offered: true, ticked: false });
});

test("the editor holds a voice to the format: not empty, at most 32 KiB", () => {
  assert.equal(voiceTextProblem("Write short."), null);
  assert.match(voiceTextProblem("  \n") ?? "", /Write the voice/);
  assert.match(voiceTextProblem("é".repeat(16 * 1024 + 1)) ?? "", /at most 32 KiB/);
});

test("a finding says the question in words, and the cost is the registry's", () => {
  assert.match(findingText({ title: "beyond_style", question: "beyond_style" }), /more than a way of writing/);
  assert.equal(findingText({ title: "A rule", line: 3 }), "A rule (line 3)");
  assert.match(hostedVoiceCost({ credits: 20, holdCredits: 20, limited: true, left: 1 }), /20 credits.*1 left this cycle/);
  assert.match(hostedVoiceCost({ credits: 20, holdCredits: 0, limited: false, left: 0 }), /no credits/);
});
