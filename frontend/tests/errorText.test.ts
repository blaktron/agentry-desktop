// What an error box says of what it was given.
import { test } from "node:test";
import assert from "node:assert/strict";
import { causeOf } from "../src/errorText.ts";

test("the cause is the error's text as one sentence, with a thrown error's class name taken off", () => {
  assert.equal(causeOf("the registry could not be reached"), "The registry could not be reached.");
  assert.equal(causeOf(new Error("sign in to r.example to see its runs")), "Sign in to r.example to see its runs.");
  assert.equal(causeOf("Error: docker is not running"), "Docker is not running.");
  assert.equal(causeOf("RuntimeError: read the run: HTTP 503"), "Read the run: HTTP 503.");
  assert.equal(causeOf("Already a sentence."), "Already a sentence.");
  assert.equal(causeOf("A refusal (HTTP 401)"), "A refusal (HTTP 401)");
  assert.equal(causeOf(""), "");
  assert.equal(causeOf(null), "");
});
