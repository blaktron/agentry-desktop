// A registry is shown signed in only while it accepts the sign-in this app
// holds: a token the registry answered 401 to shows as signed out, with
// the reason, whatever the keychain holds.
import { test } from "node:test";
import assert from "node:assert/strict";
import { rejectedLine, signInState } from "../src/signInState.ts";

const r = (url: string, hasToken: boolean, tokenRejected = false) => ({ url, hasToken, tokenRejected });

test("a token the registry no longer accepts reads as signed out, with the reason", () => {
  assert.equal(signInState([r("https://r.example", true)], "https://r.example"), "signed-in");
  assert.equal(signInState([r("https://r.example", false)], "https://r.example"), "signed-out");
  assert.equal(signInState([r("https://r.example", false, true)], "https://r.example"), "rejected");
  assert.equal(signInState([r("https://other.example", true)], "https://r.example"), "signed-out", "a registry not listed");
  assert.equal(signInState([], "https://r.example"), "signed-out");
  assert.match(rejectedLine("r.example"), /^r\.example no longer accepts this app's sign-in/);
  assert.match(rejectedLine("r.example"), /Sign in again/);
});
