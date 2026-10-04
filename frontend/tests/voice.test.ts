// The Writing Voice's choices and words (plan writing-voice.md §5, §8, D11,
// D12; agentry-desktop#510): Write in my voice is drawn only where the voice
// the run would use exists, ticked from that voice's own switch; the editor
// holds a voice to the format's rule; the price is the registry's.
import { test } from "node:test";
import assert from "node:assert/strict";
import { ALREADY_THE_SAME, cloudStatus, downloadButton, editWithAIProblem, findingText, hostedVoiceCost, localStatus, reviewCostLine, uploadButton, voiceChoice, voiceTextProblem } from "../src/voice.ts";

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
  assert.deepEqual(voiceChoice({ local, account: { live: { digest: "sha256:a" }, alwaysAttach: false }, attachEnabled: true }, true), { offered: true, ticked: false });
  // A registry that attaches no voice to a run: no box, rather than a refusal at Start.
  assert.deepEqual(voiceChoice({ local, account: { live: { digest: "sha256:a" }, alwaysAttach: true }, attachEnabled: false }, true).offered, false);
});

test("the editor holds a voice to the format: not empty, at most 32 KiB", () => {
  assert.equal(voiceTextProblem("Write short."), null);
  assert.match(voiceTextProblem("  \n") ?? "", /Write the voice/);
  assert.match(voiceTextProblem("é".repeat(16 * 1024 + 1)) ?? "", /at most 32 KiB/);
});

test("a finding says the question in words, and the cost is the registry's", () => {
  assert.match(findingText({ title: "beyond_style", question: "beyond_style" }), /more than a way of writing/);
  assert.equal(findingText({ title: "A rule", line: 3 }), "A rule (line 3)");
  assert.match(hostedVoiceCost({ credits: 20, holdCredits: 20, limited: true, max: 1, left: 1 }), /20 credits.*once a billing cycle.*1 left this cycle/);
  assert.match(hostedVoiceCost({ credits: 20, holdCredits: 20, limited: true, max: 2, left: 2 }), /twice a billing cycle/);
  assert.match(hostedVoiceCost({ credits: 20, holdCredits: 0, limited: false, max: 0, left: 0 }), /no credits/);
  assert.match(reviewCostLine(1, true), /for 1 credit\. Your live voice stays live/);
  assert.equal(reviewCostLine(0, false), "Save sends it to AI Review. It goes live when it passes.");
});

// The Voice page's cards and the Local Voice page (plan
// voice-page-redesign.md §4, §7, §8, D2; agentry-desktop#610).
const NOON = "2026-10-04T12:00:00Z";
const live = { review: "passed", digest: "sha256:live", createdAt: "2026-10-03T12:00:00Z" };
const signedIn = { signedIn: true, accountOffered: true };
const same = { downloadSame: false, uploadSame: false, localAfterLive: false, accountAfterLocal: false, inReview: false };

test("the Local Voice card says whether a voice is kept here, when it was saved and whether runs use it", () => {
  assert.equal(localStatus(null), "No voice on this computer yet.");
  assert.equal(localStatus({ exists: false, alwaysAttach: false }), "No voice on this computer yet.");
  assert.equal(localStatus({ exists: true, alwaysAttach: true, updated: NOON }), "Voice set · updated 4 October 2026 · used by default.");
  assert.equal(localStatus({ exists: true, alwaysAttach: false, updated: NOON }), "Voice set · updated 4 October 2026 · not used by default.");
  assert.match(localStatus({ exists: true, alwaysAttach: true, updated: NOON, problem: "signed out" }), /· it did not open\.$/);
});

test("the Cloud Voice card says the account's voice in the site's words, and signed out asks for a sign-in", () => {
  assert.equal(cloudStatus({ signedIn: false, accountOffered: false }), "Sign in to keep a voice in your account.");
  assert.equal(cloudStatus({ signedIn: true, accountOffered: false }), "This registry keeps no Writing Voices.");
  assert.equal(cloudStatus({ ...signedIn, accountError: "boom" }), "Your account's voice could not be read.");
  assert.equal(cloudStatus({ ...signedIn, account: { exists: false, alwaysAttach: false } }), "No voice in your account yet.");
  assert.equal(cloudStatus({ ...signedIn, account: { exists: true, alwaysAttach: true, live: { ...live, reviewedAt: NOON } } }), "Live since 4 October 2026 · used by default.");
  assert.equal(
    cloudStatus({ ...signedIn, account: { exists: true, alwaysAttach: false, live, pending: { review: "reviewing", createdAt: NOON } } }),
    "Live since 3 October 2026 · a new version is in review.",
  );
  assert.equal(cloudStatus({ ...signedIn, account: { exists: true, alwaysAttach: true, pending: { review: "flagged", createdAt: NOON } } }), "Not live yet · a new version was flagged by AI Review.");
});

test("Download and Upload say Already the same and cannot be pressed when the digests match", () => {
  const local = { exists: true, alwaysAttach: true, updated: NOON };
  const account = { exists: true, alwaysAttach: true, live };
  const d = downloadButton({ ...signedIn, local, account, sync: { ...same, downloadSame: true, uploadSame: true } });
  const u = uploadButton({ ...signedIn, local, account, sync: { ...same, downloadSame: true, uploadSame: true }, reviewCredits: 1 });
  assert.deepEqual(d, { disabled: true, line: `${ALREADY_THE_SAME}.` });
  assert.deepEqual(u, { disabled: true, line: `${ALREADY_THE_SAME}.` });
});

test("Download says it asks first over a newer voice here, and that a newer version is in review", () => {
  const local = { exists: true, alwaysAttach: true, updated: NOON };
  const account = { exists: true, alwaysAttach: true, live };
  const d = downloadButton({ ...signedIn, local, account, sync: { ...same, localAfterLive: true, inReview: true } });
  assert.equal(d.disabled, false);
  assert.match(d.line, /^Replaces the voice on this computer with your account's live one\. The voice here was saved after it, so you are asked first\. A newer version is in review and is not live yet\.$/);
  assert.equal(downloadButton({ ...signedIn, local: { exists: false, alwaysAttach: false }, account, sync: same }).line, "Keeps your account's live voice on this computer.");
  assert.deepEqual(downloadButton({ ...signedIn, local, account: { exists: true, alwaysAttach: false }, sync: same }), { disabled: true, line: "Your account has no live voice yet." });
  assert.equal(downloadButton({ signedIn: false, accountOffered: false, local, sync: same }).disabled, true);
});

test("Upload says what AI Review costs, and that it asks first after a newer version in the account", () => {
  const local = { exists: true, alwaysAttach: true, updated: NOON };
  const account = { exists: true, alwaysAttach: true, live };
  const u = uploadButton({ ...signedIn, local, account, sync: { ...same, accountAfterLocal: true }, reviewCredits: 1 });
  assert.equal(u.disabled, false);
  assert.equal(u.line, "Sends this computer's voice to AI Review for 1 credit. It goes live in your account when it passes. Your account's version was set after this one, so you are asked first.");
  assert.equal(uploadButton({ ...signedIn, local, account, sync: same, reviewCredits: 0 }).line, "Sends this computer's voice to AI Review. It goes live in your account when it passes.");
  assert.deepEqual(uploadButton({ ...signedIn, local: { exists: false, alwaysAttach: false }, account, sync: same, reviewCredits: 1 }), { disabled: true, line: "This computer keeps no voice to upload yet." });
  assert.equal(uploadButton({ ...signedIn, local: { ...local, problem: "x" }, account, sync: same, reviewCredits: 1 }).disabled, true);
  assert.equal(uploadButton({ signedIn: false, accountOffered: false, local, sync: same, reviewCredits: 1 }).line, "Sign in to move your voice to or from your account.");
});

test("Edit with AI asks what should change over a voice, and samples where there is none", () => {
  assert.match(editWithAIProblem(true, " ", 3) ?? "", /what should change/);
  assert.equal(editWithAIProblem(true, "Shorter sentences.", 0), null);
  assert.equal(editWithAIProblem(true, "Shorter sentences.", 2), null);
  assert.match(editWithAIProblem(false, "plain", 0) ?? "", /samples of your writing/);
  assert.equal(editWithAIProblem(false, "", 1), null);
  assert.match(editWithAIProblem(true, "é".repeat(2049), 0) ?? "", /at most 4 KiB/);
});
