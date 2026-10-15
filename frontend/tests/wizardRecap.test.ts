// The first-run wizard's recap rules: the AI Review key has a row on
// Done, and that row is never part of what Browse's "Setup isn't finished"
// notice lists, whose sentence ("cannot sign in, run or build anything until
// it is") is not true of the key. Saving the key writes the runner policy
// from defaults, so the provider's row and "Nothing is set up yet" go by
// whether a provider was set up, not by whether the policy exists.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import { keyRow, missingForNotice, nothingSetUp, providerRow } from "../src/wizardRecap.ts";

const setup = (v: Record<string, unknown>) => v as unknown as app.SetupStatusView;

test("a key in the keychain that the policy reads is a done row", () => {
  assert.deepEqual(
    keyRow({ standing: "saved", envName: "TYPESAFE_API_KEY" }),
    { ok: true, text: "AI Review key saved", aside: true },
  );
});

test("a key from the variable the app was started with names the variable, never a value", () => {
  assert.deepEqual(
    keyRow({ standing: "env", envName: "TYPESAFE_API_KEY" }),
    { ok: true, text: "AI Review key from $TYPESAFE_API_KEY", aside: true },
  );
});

test("no key is a pending row that says what is off, and where to add one", () => {
  assert.deepEqual(
    keyRow({ standing: "none", envName: "TYPESAFE_API_KEY" }),
    { ok: false, text: "No AI Review key — drafting on this computer is off until you add one in Settings", aside: true },
  );
});

test("a key the keychain holds but the policy does not read is pending, and says to save it again", () => {
  const row = keyRow({ standing: "unread", envName: "TYPESAFE_API_KEY" });
  assert.equal(row?.ok, false);
  assert.equal(row?.aside, true);
  assert.match(row?.text ?? "", /save it again/);
});

test("a status that could not be read gives no row", () => {
  assert.equal(keyRow(null), null);
});

test("the notice lists what is pending, and never the key", () => {
  const rows = [
    { ok: false, text: "Browsing anonymously" },
    { ok: true, text: "Docker is available" },
    { ok: false, text: "Provider defaults will be written on your first run" },
    { ok: false, text: "No AI Review key — drafting on this computer is off until you add one in Settings", aside: true },
  ];
  assert.deepEqual(missingForNotice(rows), ["Browsing anonymously", "Provider defaults will be written on your first run"]);
});

test("a provider somebody set up is a done row", () => {
  assert.deepEqual(
    providerRow(setup({ policyExists: true, providerSetUp: true, provider: "openai", model: "gpt-5" })),
    { ok: true, text: "Provider openai (gpt-5) written to the runner policy" },
  );
});

test("a policy written from defaults is not a provider set up, and the row claims nothing about a key", () => {
  assert.deepEqual(
    providerRow(setup({ policyExists: true, providerSetUp: false, provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY" })),
    { ok: false, text: "The runner policy has only the default provider, anthropic (claude-sonnet-4-5), with its key read from $ANTHROPIC_API_KEY — choose yours in Settings" },
  );
});

test("no policy, or a status that could not be read, is the first run's to write", () => {
  const want = { ok: false, text: "Provider defaults will be written on your first run" };
  assert.deepEqual(providerRow(setup({ policyExists: false, providerSetUp: false, provider: "anthropic", model: "claude-sonnet-4-5" })), want);
  assert.deepEqual(providerRow(null), want);
});

test("saving only the AI Review key leaves nothing set up", () => {
  // The key's save wrote the policy; no sign-in, no provider, no coding agent.
  const afterKeyOnly = setup({ policyExists: true, providerSetUp: false, provider: "anthropic", model: "claude-sonnet-4-5" });
  assert.equal(nothingSetUp({ signedIn: false, setup: afterKeyOnly, codingAgentChosen: false }), true);
  // Any one of the three things the wizard is for is something set up.
  assert.equal(nothingSetUp({ signedIn: true, setup: afterKeyOnly, codingAgentChosen: false }), false);
  assert.equal(nothingSetUp({ signedIn: false, setup: afterKeyOnly, codingAgentChosen: true }), false);
  assert.equal(nothingSetUp({ signedIn: false, setup: setup({ policyExists: true, providerSetUp: true }), codingAgentChosen: false }), false);
  assert.equal(nothingSetUp({ signedIn: false, setup: null, codingAgentChosen: false }), true);
});
