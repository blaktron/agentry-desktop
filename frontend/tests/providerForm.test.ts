// The provider form's base URL travels for the custom provider only
// (agentry-desktop#724): a URL loaded from a custom policy, or typed and
// left behind on a switch, must not reach ApplySetup with a known
// provider, which refuses it (D-030).
import { test } from "node:test";
import assert from "node:assert/strict";
import { baseURLFor } from "../src/providerForm.ts";

test("the custom provider sends its URL, trimmed", () => {
  assert.equal(baseURLFor("custom", "  https://llm.example.net/v1 "), "https://llm.example.net/v1");
  assert.equal(baseURLFor("custom", ""), "");
});

test("a known provider sends no URL, whatever the field still holds", () => {
  for (const provider of ["anthropic", "openai", "google"]) {
    assert.equal(baseURLFor(provider, "https://llm.example.net/v1"), "", provider);
  }
});
