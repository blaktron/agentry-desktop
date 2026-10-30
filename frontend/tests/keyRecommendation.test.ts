// The AI Review key's recommendation: its words, and the one address the
// link opens. The never-words are the copy check's to hold
// (scripts/copy-check.sh reads this module with the rest).
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { GET_A_KEY_LABEL, KEY_RECOMMENDATION, TYPESAFE_URL, keyStanding } from "../src/keyRecommendation.ts";

test("the recommendation is the agreed sentence, word for word", () => {
  assert.equal(
    KEY_RECOMMENDATION,
    "Agentry's AI Review is powered by TypeSafe AI's Jev System One. We highly recommend signing up with TypeSafe and getting an API key if you want to build shareable QuickAgents or submit them to Agentry.",
  );
  assert.equal(GET_A_KEY_LABEL, "Get a key from TypeSafe");
});

test("the link goes to TypeSafe over https, with nothing of the person's in it", () => {
  assert.equal(TYPESAFE_URL, "https://typesafe.ai");
});

test("where the key stands is decided once, for every surface that says it", () => {
  const view = (v: Record<string, unknown>) => v as unknown as app.JevKeyView;
  // The variable goes first, whatever the keychain holds.
  assert.equal(keyStanding(view({ envSet: true, stored: true, source: "keyring" })), "env");
  assert.equal(keyStanding(view({ envSet: false, stored: true, source: "keyring" })), "saved");
  // In the keychain, but the policy does not read it from there.
  assert.equal(keyStanding(view({ envSet: false, stored: true, source: "" })), "unread");
  assert.equal(keyStanding(view({ envSet: false, stored: false, source: "keyring" })), "none");
});
