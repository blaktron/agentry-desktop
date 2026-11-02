import { test } from "node:test";
import assert from "node:assert/strict";
import { persistentKeys } from "../src/ui.tsx";

test("a persistent release's declared keys are its switches that are on and its lists that name something", () => {
  assert.deepEqual(persistentKeys({ clock: true, memory: true, coder: false, secrets: ["API_KEY"], mail: [] }), ["clock", "memory", "secrets"]);
  assert.deepEqual(persistentKeys(undefined), []);
  assert.deepEqual(persistentKeys("clock"), []);
});
