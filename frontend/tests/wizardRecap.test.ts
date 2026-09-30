// The first-run wizard's recap rules (agentry-desktop#462; plan
// desktop-review-key-and-run-cards.md D13): the AI Review key has a row on
// Done, and that row is never part of what Browse's "Setup isn't finished"
// notice lists, whose sentence ("cannot sign in, run or build anything until
// it is") is not true of the key.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import { keyRow, missingForNotice } from "../src/wizardRecap.ts";

const key = (v: Record<string, unknown>) => v as unknown as main.JevKeyView;

test("a key in the keychain that the policy reads is a done row", () => {
  assert.deepEqual(
    keyRow(key({ stored: true, source: "keyring", envName: "TYPESAFE_API_KEY", envSet: false, ready: true })),
    { ok: true, text: "AI Review key saved", aside: true },
  );
});

test("a key from the variable the app was started with names the variable, never a value", () => {
  assert.deepEqual(
    keyRow(key({ stored: false, envName: "TYPESAFE_API_KEY", envSet: true, ready: true })),
    { ok: true, text: "AI Review key from $TYPESAFE_API_KEY", aside: true },
  );
});

test("no key is a pending row that says what is off, and where to add one", () => {
  const row = keyRow(key({ stored: false, envName: "TYPESAFE_API_KEY", envSet: false, ready: false }));
  assert.equal(row?.ok, false);
  assert.equal(row?.aside, true);
  assert.equal(row?.text, "No AI Review key — drafting on this computer is off until you add one in Settings");
});

test("a key the keychain holds but the policy does not read is pending, and says to save it again", () => {
  const row = keyRow(key({ stored: true, source: "", envName: "TYPESAFE_API_KEY", envSet: false, ready: false }));
  assert.equal(row?.ok, false);
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
