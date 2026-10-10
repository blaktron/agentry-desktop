// The Builder's Required/Optional choice per key (plan optional-secrets.md
// D2, D7; agentry-desktop#655).
import { test } from "node:test";
import assert from "node:assert/strict";
import { declaredKeys, optionalKeys, questionRequired, requiredKeys, secretsRow } from "../src/requiredKeys.ts";
import { goesWithoutSentence, needList, offerSetKeys, requiredOf } from "../src/runSetup.ts";

const m = { secrets: ["API_TOKEN"], mcpServers: [{ secrets: ["SEARCH_KEY", "API_TOKEN"] }], requiredSecrets: [] as string[] };

test("every declared key, its own then its servers', once; a key nobody marked is optional", () => {
  assert.deepEqual(declaredKeys(m), ["API_TOKEN", "SEARCH_KEY"]);
  assert.deepEqual(optionalKeys(m), ["API_TOKEN", "SEARCH_KEY"]);
});

test("one key's choice is written beside the others, in the form's order", () => {
  const marked = requiredKeys(m, { name: "SEARCH_KEY", required: true });
  assert.deepEqual(marked, ["SEARCH_KEY"]);
  assert.deepEqual(requiredKeys({ ...m, requiredSecrets: marked }, { name: "API_TOKEN", required: true }), ["API_TOKEN", "SEARCH_KEY"]);
  assert.deepEqual(requiredKeys({ ...m, requiredSecrets: ["API_TOKEN", "SEARCH_KEY"] }, { name: "API_TOKEN", required: false }), ["SEARCH_KEY"]);
});

test("a mark on a removed key goes with it", () => {
  assert.deepEqual(requiredKeys({ secrets: ["API_TOKEN"], mcpServers: [], requiredSecrets: ["SEARCH_KEY", "API_TOKEN"] }), ["API_TOKEN"]);
});

test("a secret question of the QuickAgent's own key follows the key; any other follows its box", () => {
  const q = { id: "api_token", type: "secret", required: true };
  assert.deepEqual(questionRequired(m, q), { required: false, follows: true });
  assert.deepEqual(questionRequired({ ...m, requiredSecrets: ["API_TOKEN"] }, { ...q, required: false }), { required: true, follows: true });
  assert.deepEqual(questionRequired(m, { id: "search_key", type: "secret", required: true }), { required: true, follows: false });
  assert.deepEqual(questionRequired(m, { id: "notes", type: "text", required: true }), { required: true, follows: false });
});


test("RunPanel holds a run only for a required key, and says which optional ones it goes without", () => {
  const p = { declaredSecrets: ["OPT_KEY", "REQ_KEY"], optionalSecrets: ["OPT_KEY"], storedSecretNames: ["REQ_KEY"] };
  assert.deepEqual(requiredOf(p), ["REQ_KEY"]);
  assert.deepEqual(requiredOf({ declaredSecrets: ["A"] }), ["A"]);
  assert.equal(goesWithoutSentence(p, false), "; the run goes without the optional key OPT_KEY unless you set it in Connect tools or this computer's environment holds it");
  assert.equal(goesWithoutSentence({ ...p, storedSecretNames: ["REQ_KEY", "OPT_KEY"] }, false), "");
  assert.equal(goesWithoutSentence(p, true), "; the run goes without the optional key OPT_KEY unless your account keeps it");
  assert.equal(goesWithoutSentence({ optionalSecrets: ["A", "B"] }, true), "; the run goes without the optional keys A, B unless your account keeps them");
  assert.equal(goesWithoutSentence({ declaredSecrets: ["A"] }, true), "");
});

test("an optional key can be set at run time on this computer, not only from the environment (agentry-desktop#784)", () => {
  // All optional, nothing saved: the form never opens by itself, so it is offered.
  assert.equal(offerSetKeys({ declaredSecrets: ["OPT_KEY"], optionalSecrets: ["OPT_KEY"], exists: false }), true);
  // A required key opens the form by itself; a saved profile offers Reconfigure…
  assert.equal(offerSetKeys({ declaredSecrets: ["OPT_KEY", "REQ_KEY"], optionalSecrets: ["OPT_KEY"], exists: false }), false);
  assert.equal(offerSetKeys({ declaredSecrets: ["OPT_KEY"], optionalSecrets: ["OPT_KEY"], exists: true }), false);
  assert.equal(offerSetKeys({ declaredSecrets: [], optionalSecrets: [], exists: false }), false);
  assert.equal(goesWithoutSentence({ optionalSecrets: ["A", "B"] }, false), "; the run goes without the optional keys A, B unless you set them in Connect tools or this computer's environment holds them");
});

test("the ready list says a run can start without an optional key", () => {
  const need = needList({ intake: [], secrets: ["OPT_KEY", "REQ_KEY"], optional: ["OPT_KEY"] });
  assert.ok(need.some((l) => l.endsWith("A run can start without OPT_KEY.")), need.join("\n"));
  assert.ok(!needList({ intake: [], secrets: ["A"] }).some((l) => l.includes("start without")));
});


test("the consent names every declared secret, an optional one marked, as before for a release without them", () => {
  assert.equal(secretsRow(["REQ_KEY", "OPT_KEY", "SRV_KEY"], ["OPT_KEY", "SRV_KEY"]), "REQ_KEY, OPT_KEY (optional), SRV_KEY (optional)");
  assert.equal(secretsRow(["A", "B"], []), "A, B");
  assert.equal(secretsRow(undefined, undefined), "");
});
