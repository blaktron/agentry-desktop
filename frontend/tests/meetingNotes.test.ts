// Meeting notes' words: durations, the recording's clock, each meeting's
// state, the No of a native dialog, the offer and the longest recording.
import { test } from "node:test";
import assert from "node:assert/strict";
import { canTranscribe, downloadWords, duration, elapsed, hoursLabel, isNo, meetingState, offerTitle, recordingWhat } from "../src/meetingNotes.ts";

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

test("an offer names its app, and the longest recording reads in hours", () => {
  assert.equal(offerTitle("Zoom Meeting"), "Record this meeting? (Zoom Meeting)");
  assert.equal(offerTitle(""), "Record this meeting? (a meeting app)");
  assert.equal(hoursLabel(1), "1 hour");
  assert.equal(hoursLabel(4), "4 hours");
});

test("a meeting says how it was handed to the Assistant, and offers Ask only while it may", async () => {
  const { deliveryWords, canAskAssistant, possessive, capitalised } = await import("../src/meetingNotes.ts");
  assert.equal(deliveryWords({ delivery: "" }, "Ada"), "");
  assert.equal(deliveryWords({ delivery: "waiting" }, "Ada"), "In Ada's share folder, waiting for you to ask Ada to summarise it");
  assert.equal(deliveryWords({ delivery: "sent" }, "the Assistant"), "Given to the Assistant to summarise");
  assert.equal(deliveryWords({ delivery: "failed", deliveryError: "no share" }, "Ada"), "Not given to Ada: no share");
  const done = { state: "done", hasTranscript: true };
  const on = { enabled: true, sendToAssistant: true };
  assert.equal(canAskAssistant({ ...done, delivery: "waiting" }, on), true);
  assert.equal(canAskAssistant({ ...done, delivery: "failed" }, on), true);
  assert.equal(canAskAssistant({ ...done, delivery: "" }, on), true);
  assert.equal(canAskAssistant({ ...done, delivery: "sent" }, on), false);
  assert.equal(canAskAssistant({ ...done, delivery: "sending" }, on), false);
  assert.equal(canAskAssistant({ ...done, delivery: "waiting" }, { enabled: true, sendToAssistant: false }), false);
  assert.equal(canAskAssistant({ state: "transcribing", hasTranscript: false }, on), false);
  assert.equal(possessive("James"), "James'");
  assert.equal(capitalised("the Assistant"), "The Assistant");
  assert.equal(capitalised("Ada"), "Ada");
});

test("an armed window says when it records and where it stands", async () => {
  const { windowWhen, windowStateWords } = await import("../src/meetingNotes.ts");
  // Times in this computer's own format, whatever its locale.
  const t = (d: Date) => d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const now = new Date(2026, 9, 25, 13, 0);
  const [o, c] = [new Date(2026, 9, 25, 13, 55), new Date(2026, 9, 25, 15, 15)];
  assert.equal(windowWhen(o.toISOString(), c.toISOString(), now), `${t(o)} to ${t(c)}`);
  const [o2, c2] = [new Date(2026, 9, 26, 8, 55), new Date(2026, 9, 26, 9, 45)];
  const tomorrow = windowWhen(o2.toISOString(), c2.toISOString(), now);
  assert.ok(tomorrow.endsWith(`, ${t(o2)} to ${t(c2)}`) && tomorrow.length > `, ${t(o2)} to ${t(c2)}`.length, tomorrow);
  assert.equal(windowStateWords({ state: "waiting", recorded: 0 }), "Waiting to open");
  assert.equal(windowStateWords({ state: "closed", recorded: 0 }), "Closed: no call was noticed");
  assert.equal(windowStateWords({ state: "closed", recorded: 1 }), "Recorded");
  assert.equal(windowStateWords({ state: "recording", recorded: 1 }), "Recording");
});

test("the graphics card's offer names the download, and Settings says how it measured", async () => {
  const { gpuOfferTitle, gpuHint } = await import("../src/meetingNotes.ts");
  assert.equal(gpuOfferTitle(574_041_195), "Use your graphics card with the better model (574 MB)");
  assert.match(gpuHint({ name: "Arc A770", fast: true, modelName: "Large v3 Turbo" }), /Arc A770 fast enough/);
  assert.match(gpuHint({ name: "HD 630", fast: false, modelName: "Large v3 Turbo" }), /too slow to keep up/);
});
