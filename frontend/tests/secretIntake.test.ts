// A secret intake question In Agentry: no field, no answer sent, a line
// that says where the value comes from, and a refusal framed in words.
import { test } from "node:test";
import assert from "node:assert/strict";
import { askedHere, intakeToSend, secretAnswerRefusal, secretName, secretQuestionLine } from "../src/secretIntake.ts";

const intake = [
  { id: "topic", type: "text", prompt: "What about?" },
  { id: "api_token", type: "secret", prompt: "Your API token" },
];
const values = { topic: "llamas", api_token: "s3cr3t-value", stale: "kept", empty: " " };

test("In Agentry a secret question is not asked and its answer is not sent", () => {
  assert.equal(askedHere(intake[1], true), false);
  assert.equal(askedHere(intake[0], true), true);
  assert.deepEqual(intakeToSend(values, intake, true), { topic: "llamas", stale: "kept" });
});

test("on this computer every question is asked and every answer sent", () => {
  assert.equal(askedHere(intake[1], false), true);
  assert.deepEqual(intakeToSend(values, intake, false), { topic: "llamas", api_token: "s3cr3t-value", stale: "kept" });
});

test("the line names the account's secret where the release declares it", () => {
  assert.equal(secretName("api_token"), "API_TOKEN");
  assert.match(secretQuestionLine(intake[1], ["API_TOKEN"]), /takes it from your account's secrets there \(API_TOKEN\)/);
  assert.match(secretQuestionLine(intake[1], ["OTHER"]), /answered only when the run is on this computer/);
});

test("a refusal naming a question is framed; any other is not", () => {
  const r = secretAnswerRefusal(
    { refusal: "validation", question: "api_token", detail: "Validation failed: The answer to api_token is a secret. Leave it unanswered." },
    intake,
  );
  assert.ok(r);
  assert.match(r.heading, /Your API token/);
  assert.equal(r.detail, "The answer to api_token is a secret. Leave it unanswered.");
  assert.equal(secretAnswerRefusal({ refusal: "validation", detail: "x" }), null);
  assert.equal(secretAnswerRefusal({ refusal: "run-active", question: "api_token" }), null);
  assert.match(secretAnswerRefusal({ refusal: "validation", question: "api_token" })!.heading, /api_token/);
});
