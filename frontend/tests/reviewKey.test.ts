// The AI Review key's recommendation (agentry-desktop#461; plan
// desktop-review-key-and-run-cards.md D1, D12): the operator's words as
// decided, and the one address the link opens.
import { test } from "node:test";
import assert from "node:assert/strict";
import { GET_A_KEY, KEY_RECOMMENDATION, TYPESAFE_URL } from "../src/reviewKey.ts";

test("the recommendation is the operator's, word for word", () => {
  assert.equal(
    KEY_RECOMMENDATION,
    "Agentry's AI Review is powered by TypeSafe AI's Jev System One. We highly recommend signing up with TypeSafe and getting an API key if you want to build shareable QuickAgents or submit them to Agentry.",
  );
  assert.equal(GET_A_KEY, "Get a key from TypeSafe");
});

test("the link goes to TypeSafe over https, with nothing of the person's in it", () => {
  const u = new URL(TYPESAFE_URL);
  assert.equal(u.protocol, "https:");
  assert.equal(u.host, "typesafe.ai");
  assert.equal(u.username + u.password + u.search + u.hash, "");
  assert.equal(TYPESAFE_URL, "https://typesafe.ai");
});

test("the recommendation makes no promise the product does not make", () => {
  // "TypeSafe" is the vendor's name; the never-words are whole words.
  assert.doesNotMatch(KEY_RECOMMENDATION, /\b(safe|safely|secure|securely|secured|guarantee[sd]?|trusted)\b/i);
});
