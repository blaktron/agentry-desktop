// The model form's context-window field: what it sends, and for which model.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_CONTEXT_WINDOW, parseContextWindow, windowKey } from "../src/providerForm.ts";

test("an empty field sends nothing, so setup asks the endpoint", () => {
  assert.deepEqual(parseContextWindow(""), { tokens: 0, error: null });
  assert.deepEqual(parseContextWindow("   "), { tokens: 0, error: null });
});

test("a typed window is sent as typed, separators dropped", () => {
  assert.deepEqual(parseContextWindow("262144"), { tokens: 262144, error: null });
  assert.deepEqual(parseContextWindow("131_072"), { tokens: 131072, error: null });
  assert.deepEqual(parseContextWindow("32,768"), { tokens: 32768, error: null });
});

test("a window outside the CLI's bounds, or not a whole number, is refused", () => {
  for (const bad of ["512", "1023", String(MAX_CONTEXT_WINDOW + 1), "64k", "1.5", "-4096"]) {
    const got = parseContextWindow(bad);
    assert.equal(got.tokens, 0, bad);
    assert.ok(got.error, bad);
  }
  assert.equal(parseContextWindow("1024").tokens, 1024);
  assert.equal(parseContextWindow(String(MAX_CONTEXT_WINDOW)).tokens, MAX_CONTEXT_WINDOW);
});

test("a window belongs to one model at one endpoint", () => {
  const k = windowKey("custom", " http://127.0.0.1:1234/v1 ", "qwen3 ");
  assert.equal(k, windowKey("custom", "http://127.0.0.1:1234/v1", "qwen3"));
  assert.notEqual(k, windowKey("custom", "http://127.0.0.1:1234/v1", "llama"));
  assert.notEqual(k, windowKey("custom", "http://127.0.0.1:8080/v1", "qwen3"));
  // A known provider has no endpoint of its own to tell apart.
  assert.equal(windowKey("openai", "http://x/v1", "gpt"), windowKey("openai", "", "gpt"));
});
