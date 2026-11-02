// The hints Describe it hands the Drafter, and the choice of an agent that
// stays running among them.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MAX_HINT_BYTES, NO_HINTS, PERSISTENT_HINT, RUN_WAYS, hintLines } from "../src/describeHints.ts";

test("no hint is no line", () => {
  assert.deepEqual(hintLines(NO_HINTS), []);
  assert.deepEqual(hintLines({ services: "  \n ", mode: "", example: "\t" }), []);
});

test("the earlier choices carry the lines they always did", () => {
  assert.deepEqual(hintLines({ services: "GitHub", mode: "autonomous", example: "a table" }), [
    "The services or sites it works with: GitHub",
    "It runs on its own, from a task, with no conversation.",
    "An example of the output wanted: a table",
  ]);
  assert.deepEqual(hintLines({ services: "", mode: "interactive", example: "" }), ["It runs in a conversation with a person."]);
});

test("an agent that stays running is carried as one hint that asks for a persistent agent", () => {
  const lines = hintLines({ services: "Todoist", mode: "persistent", example: "" });
  assert.equal(lines.length, 2);
  assert.equal(lines[1], PERSISTENT_HINT);
  // The words the Drafter drafts a persistent agent for.
  for (const word of ["stays running", "conversation", "memory", "schedule", "persistent agent"]) {
    assert.ok(PERSISTENT_HINT.includes(word), `the hint does not say "${word}"`);
  }
  // Not a conversation that ends, and not a one-off task.
  assert.ok(!lines.includes("It runs in a conversation with a person."));
  assert.ok(!lines.includes("It runs on its own, from a task, with no conversation."));
});

test("the persistent hint fits the Drafter's bound for one hint", () => {
  assert.ok(new TextEncoder().encode(PERSISTENT_HINT).length <= MAX_HINT_BYTES);
  assert.ok(!PERSISTENT_HINT.includes("\n"));
});

test("the run choices offer an agent that stays running last", () => {
  assert.deepEqual(RUN_WAYS.map((w) => w.value), ["", "autonomous", "interactive", "persistent"]);
  assert.equal(RUN_WAYS[3].label, "An agent that stays running");
});
