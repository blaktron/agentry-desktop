import assert from "node:assert/strict";
import test from "node:test";

import { problemSentence, chosenLine, credentialsLine, defaultLine, destinationLabel, nameProblem, newSecret, reasonWords, sendLine, sendStatus } from "../src/destinations.ts";

const send = (over: Record<string, unknown> = {}) => ({ destination: "n8n", host: "hooks.example.com:443", onMyNetwork: false, delivered: 0, refused: 0, failed: 0, result: false, canRetry: false, ...over });

test("a closed word is said in a person's words and an unknown one never as text", () => {
  assert.match(reasonWords("status"), /error/);
  assert.match(reasonWords("address"), /not one a delivery may reach/);
  assert.equal(reasonWords("something the receiver wrote"), "it did not arrive");
  assert.equal(reasonWords(""), "");
  assert.equal(reasonWords(undefined), "");
});

test("a send says what arrived and what did not", () => {
  assert.equal(sendLine(send({ delivered: 2 })), "2 files sent.");
  assert.equal(sendLine(send({ delivered: 1, result: true })), "The result was sent.");
  assert.match(sendLine(send({ failed: 1, error: "status" })), /^1 file not sent: it answered with an error\.$/);
  assert.match(sendLine(send({ delivered: 1, refused: 1, reason: "too-large" })), /1 file sent\. 1 file listed without content: it is over the 10 MiB/);
  assert.equal(sendLine(send()), "Nothing was sent.");
});

test("a send is failed, partly sent or sent", () => {
  assert.equal(sendStatus(send({ failed: 2 })), "failed");
  assert.equal(sendStatus(send({ failed: 1, delivered: 1 })), "partly");
  assert.equal(sendStatus(send({ delivered: 3 })), "sent");
  assert.equal(sendStatus(send({ refused: 1, delivered: 1 })), "sent");
});

test("a name is held to the CLI's rule", () => {
  assert.equal(nameProblem("n8n"), "");
  assert.equal(nameProblem("my.hook_2-x"), "");
  assert.notEqual(nameProblem(""), "");
  assert.notEqual(nameProblem("Upper"), "");
  assert.notEqual(nameProblem("has space"), "");
  assert.notEqual(nameProblem("-lead"), "");
  assert.notEqual(nameProblem("a".repeat(41)), "");
  assert.equal(nameProblem("a".repeat(40)), "");
});

test("a generated secret is whsec_ and 32 random bytes in base64url", () => {
  const s = newSecret((b) => b.fill(255));
  assert.equal(s, "whsec_" + "_".repeat(42) + "8");
  assert.match(newSecret(), /^whsec_[A-Za-z0-9_-]{43}$/);
  assert.notEqual(newSecret(), newSecret());
});

test("the credentials line says whether each is kept and never what it is", () => {
  assert.match(credentialsLine({ hasSecret: true, hasAuthorization: true }), /signing secret is kept in your system keychain; an Authorization value is kept too/);
  assert.match(credentialsLine({ hasSecret: true, hasAuthorization: false }), /keychain\.$/);
  assert.match(credentialsLine({ hasSecret: false, hasAuthorization: false }), /No signing secret is kept/);
});

test("a destination is labelled with where it goes", () => {
  assert.equal(destinationLabel({ name: "lan", where: "http://192.168.1.20:5678", onMyNetwork: true }), "lan (http://192.168.1.20:5678, on my network)");
  assert.equal(destinationLabel({ name: "n8n", where: "https://hooks.example.com", onMyNetwork: false }), "n8n (https://hooks.example.com)");
});

test("the setup says where a run will send, and the default line where a destination starts selected", () => {
  assert.match(chosenLine([]), /Nowhere beyond the run folder/);
  assert.equal(chosenLine(["n8n"]), "The result and each deliverable go to n8n once the run succeeds.");
  assert.equal(chosenLine(["a", "b"]), "The result and each deliverable go to a, b once the run succeeds.");
  assert.equal(defaultLine({ defaultFor: [] }), "");
  assert.equal(defaultLine({}), "");
  assert.equal(defaultLine({ defaultFor: ["acme/weekly"] }), "Selected when you run acme/weekly.");
});

test("a problem from the CLI's rule is shown as a sentence", () => {
  assert.equal(problemSentence("the address is http: a destination must be https, unless it is on your own network"), "The address is http: a destination must be https, unless it is on your own network.");
  assert.equal(problemSentence("Already a sentence."), "Already a sentence.");
  assert.equal(problemSentence("  "), "");
});
