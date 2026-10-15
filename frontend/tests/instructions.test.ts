// The Agent tab's instruction file card.
import { test } from "node:test";
import assert from "node:assert/strict";
import { instructionsNote } from "../src/instructions.ts";

const file = { path: "AGENT.md", content: "# Do the task\n", truncated: false, binary: false };

test("a whole file needs no note; anything less says what is missing", () => {
  assert.equal(instructionsNote(null, null), null, "still reading");
  assert.equal(instructionsNote(file, null), null);
  assert.equal(instructionsNote({ ...file, truncated: true }, null), "AGENT.md is longer than the registry shows; this is its beginning.");
  assert.equal(instructionsNote({ ...file, path: "SKILL.md", binary: true, content: "" }, null), "SKILL.md could not be read as text.");
  assert.equal(instructionsNote({ ...file, content: " \n" }, null), "AGENT.md is empty.");
  assert.equal(instructionsNote(null, "registry said 404"), "The instruction file could not be read: registry said 404");
});
