// The provider form's base URL travels for the custom provider only
// (agentry-desktop#724): a URL loaded from a custom policy, or typed and
// left behind on a switch, must not reach ApplySetup with a known
// provider, which refuses it (D-030).
import { test } from "node:test";
import assert from "node:assert/strict";
import { baseURLFor, takesBaseURL, tileAfterKey } from "../src/providerForm.ts";

test("the custom provider sends its URL, trimmed", () => {
  assert.equal(baseURLFor("custom", "  https://llm.example.net/v1 "), "https://llm.example.net/v1");
  assert.equal(baseURLFor("custom", ""), "");
});

test("a known provider sends no URL, whatever the field still holds", () => {
  for (const provider of ["anthropic", "openai", "google"]) {
    assert.equal(baseURLFor(provider, "https://llm.example.net/v1"), "", provider);
  }
});

test("only the custom provider shows the field", () => {
  assert.equal(takesBaseURL("custom"), true);
  for (const provider of ["anthropic", "openai", "google"]) assert.equal(takesBaseURL(provider), false, provider);
});

// The provider tiles are one radio group (agentry-desktop#830): the arrows
// walk them in order and wrap, Home and End go to the ends, and a provider
// no tile names starts the walk at an end.
test("the arrows move the provider choice as a radio group's do", () => {
  const ids = ["anthropic", "openai", "copilot", "openrouter", "poe", "huggingface", "google", "custom"];
  assert.equal(tileAfterKey(ids, "anthropic", "ArrowRight"), "openai");
  assert.equal(tileAfterKey(ids, "openrouter", "ArrowDown"), "poe", "the end of a row goes on to the next");
  assert.equal(tileAfterKey(ids, "custom", "ArrowRight"), "anthropic", "the last wraps to the first");
  assert.equal(tileAfterKey(ids, "anthropic", "ArrowLeft"), "custom", "the first wraps to the last");
  assert.equal(tileAfterKey(ids, "poe", "ArrowUp"), "openrouter");
  assert.equal(tileAfterKey(ids, "poe", "Home"), "anthropic");
  assert.equal(tileAfterKey(ids, "poe", "End"), "custom");
  assert.equal(tileAfterKey(ids, "mistral", "ArrowRight"), "anthropic");
  assert.equal(tileAfterKey(ids, "mistral", "ArrowLeft"), "custom");
  assert.equal(tileAfterKey(ids, "poe", "Enter"), null, "a key the group does not own is left alone");
  assert.equal(tileAfterKey(ids, "poe", "Tab"), null);
});
