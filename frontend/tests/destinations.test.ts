import assert from "node:assert/strict";
import test from "node:test";

import { problemSentence, cannotChooseWhy, chosenLine, credentialsLine, defaultLine, destinationLabel, keyPairLine, kindPhrase, nameProblem, newSecret, objectPath, reasonWords, sendLine, sendStatus } from "../src/destinations.ts";

const send = (over: Record<string, unknown> = {}) => ({ destination: "n8n", host: "hooks.example.com:443", onMyNetwork: false, delivered: 0, refused: 0, failed: 0, result: false, canRetry: false, ...over });

test("a closed word is said in a person's words and an unknown one never as text", () => {
  assert.match(reasonWords("status"), /error/);
  assert.match(reasonWords("address"), /not one a delivery may reach/);
  assert.equal(reasonWords("something the receiver wrote"), "it did not arrive");
  assert.equal(reasonWords(""), "");
  assert.equal(reasonWords(undefined), "");
  // A word that is a property of every object is not a reason.
  for (const w of ["constructor", "toString", "__proto__", "hasOwnProperty"]) assert.equal(reasonWords(w), "it did not arrive");
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

test("a bucket's words are its own: a key pair, a service, a 64 MiB cap", () => {
  assert.match(reasonWords("credential", "s3"), /access key id or secret access key/);
  assert.match(reasonWords("credential"), /signing secret/);
  assert.match(reasonWords("status", "s3"), /service refused it or answered with an error/);
  assert.match(reasonWords("too-large", "s3"), /64 MiB/);
  assert.match(reasonWords("too-large", "webhook"), /10 MiB/);
  // A word only a webhook says still reads plainly for a bucket, and an unknown one never as text.
  assert.match(reasonWords("changed", "s3"), /not the one the run recorded/);
  assert.equal(reasonWords("a body the service wrote", "s3"), "it did not arrive");
  for (const w of ["constructor", "toString", "__proto__"]) assert.equal(reasonWords(w, "s3"), "it did not arrive");
});

test("a bucket's send never says a file was listed without content", () => {
  assert.equal(sendLine(send({ kind: "s3", delivered: 2 })), "2 files sent.");
  assert.equal(sendLine(send({ kind: "s3", delivered: 1, refused: 1, reason: "too-large" })), "1 file sent. 1 file not sent: it is over the 64 MiB a bucket takes, so it was not sent.");
  assert.match(sendLine(send({ kind: "s3", failed: 3, error: "credential" })), /^3 files not sent: the access key id or secret access key is missing/);
  assert.match(sendLine(send({ delivered: 1, refused: 1, reason: "too-large" })), /listed without content/);
});

test("a saved bucket is described without its credentials and says whether the pair is kept", () => {
  assert.equal(kindPhrase({ kind: "s3", where: "s3://my-reports/agentry at https://my-reports.s3.eu-west-1.amazonaws.com:443" }), "bucket my-reports/agentry at https://my-reports.s3.eu-west-1.amazonaws.com:443");
  assert.equal(kindPhrase({ kind: "webhook", where: "https://hooks.example.com" }), "webhook to https://hooks.example.com");
  assert.equal(kindPhrase({ where: "https://hooks.example.com" }), "webhook to https://hooks.example.com");
  assert.match(credentialsLine({ kind: "s3", hasSecret: true, hasAccessKey: true, hasAuthorization: false }), /^An access key id and a secret access key are kept in your system keychain\.$/);
  assert.match(credentialsLine({ kind: "s3", hasSecret: false, hasAccessKey: false, hasAuthorization: false }), /No access key is kept/);
  assert.match(credentialsLine({ kind: "s3", hasSecret: true, hasAccessKey: false, hasAuthorization: false }), /Only the secret access key is kept/);
  assert.match(credentialsLine({ kind: "s3", hasSecret: false, hasAccessKey: true, hasAuthorization: false }), /Only the access key id is kept/);
  assert.match(cannotChooseWhy({ kind: "s3" }), /access key/);
  assert.match(cannotChooseWhy({ kind: "webhook" }), /signing secret/);
});

test("the form shows where an object goes and whether the saved pair stays", () => {
  assert.equal(objectPath({ bucket: "my-reports", prefix: "agentry" }), "my-reports/agentry/<run id>/<file>");
  assert.equal(objectPath({ bucket: "my-reports", prefix: "/a/b/" }), "my-reports/a/b/<run id>/<file>");
  assert.equal(objectPath({ bucket: "my-reports", prefix: "" }), "my-reports/<run id>/<file>");
  assert.equal(objectPath({ bucket: " ", prefix: "" }), "<bucket>/<run id>/<file>");
  assert.match(keyPairLine(true, true), /saved pair is kept/);
  assert.match(keyPairLine(false, true), /another service/);
  assert.match(keyPairLine(false, false), /type both/);
});
