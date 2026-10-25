// Meeting notes' words: durations, the recording's clock, each meeting's
// state, and the No of a native dialog.
import { test } from "node:test";
import assert from "node:assert/strict";
import { canTranscribe, downloadWords, duration, elapsed, isNo, meetingState, recordingWhat } from "../src/meetingNotes.ts";

test("durations read in seconds, minutes, then hours", () => {
  assert.equal(duration(45), "45 s");
  assert.equal(duration(12 * 60 + 5), "12 min");
  assert.equal(duration(3600 + 5 * 60), "1 h 05 min");
  assert.equal(duration(-3), "0 s");
});

test("the recording's clock counts from its start", () => {
  const start = "2026-10-24T14:00:00Z";
  assert.equal(elapsed(start, Date.parse("2026-10-24T14:01:05Z")), "01:05");
  assert.equal(elapsed(start, Date.parse("2026-10-24T15:02:03Z")), "1:02:03");
});

test("each meeting says where it stands", () => {
  const base = { progress: -1, hasAudio: true, hasTranscript: false };
  assert.equal(meetingState({ ...base, state: "transcribing", progress: 40 }).words, "Transcribing, 40%");
  assert.equal(meetingState({ ...base, state: "done", hasTranscript: true }).tone, "ok");
  assert.match(meetingState({ ...base, state: "done" }).words, /moved or deleted/);
  assert.match(meetingState({ ...base, state: "failed", error: "no model" }).words, /no model/);
  assert.equal(canTranscribe({ ...base, state: "failed" }), true);
  assert.equal(canTranscribe({ ...base, state: "failed", hasAudio: false }), false);
  assert.equal(canTranscribe({ ...base, state: "done" }), false);
});

test("what is recorded is said plainly, the whole device included", () => {
  assert.equal(recordingWhat("Zoom", false), "your microphone and Zoom");
  assert.equal(recordingWhat("", true), "your microphone and everything your computer plays");
});

test("a download counts in megabytes, and a dialog's No is not an error", () => {
  assert.equal(downloadWords(120_000_000, 190_098_681), "120 of 190 MB");
  assert.equal(downloadWords(0, 0), "");
  assert.equal(isNo("Error: not confirmed: Record this meeting?"), true);
  assert.equal(isNo("Error: the audio system could not be reached"), false);
});
