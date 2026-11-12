// Settings → Models and keys → Harness: the two container harnesses, each
// harness's name, and the models that run on their vendor's harness.
import { test } from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_HARNESS, HARNESS_OPTIONS, changedLine, harnessName, harnessPhrase, runsOnLine, signInLines, vendorHarness } from "../src/containerHarness.ts";

test("the choice offers Docker Agent first, as the default, then the Agentry harness", () => {
  assert.deepEqual(HARNESS_OPTIONS.map((h) => h.id), ["docker-agent", "agentry-harness"]);
  assert.equal(DEFAULT_HARNESS, "docker-agent");
});

test("a receipt's harness is named as the product names it", () => {
  assert.equal(harnessName("docker-agent"), "Docker Agent");
  assert.equal(harnessName("agentry-harness"), "Agentry harness");
  assert.equal(harnessName("claude-image"), "Claude Code");
  assert.equal(harnessName("claude"), "Claude Code");
  assert.equal(harnessName("codex-image"), "Codex");
  assert.equal(harnessName("copilot-image"), "Copilot CLI");
  assert.equal(harnessName("something-new"), "something-new");
  assert.equal(harnessName(undefined), "");
  assert.equal(harnessPhrase("agentry-harness"), "the Agentry harness");
  assert.equal(harnessPhrase("docker-agent"), "Docker Agent");
});

test("only a model sign-in runs on its vendor's harness", () => {
  assert.equal(vendorHarness("chatgpt"), "Codex");
  assert.equal(vendorHarness("claude"), "Claude Code");
  assert.equal(vendorHarness("copilot"), "Copilot CLI");
  for (const way of ["key", "connect", "credits"]) assert.equal(vendorHarness(way), null);
});

test("each sign-in among the providers gets one line, and keys and credits none", () => {
  assert.deepEqual(signInLines([{ way: "key" }, { way: "credits" }, { way: "connect" }]), []);
  assert.deepEqual(signInLines([{ way: "chatgpt" }, { way: "key" }, { way: "chatgpt" }, { way: "claude" }]), [
    "Models paid by your ChatGPT plan run on Codex whatever is chosen here.",
    "Models paid by your Claude subscription run on Claude Code whatever is chosen here.",
  ]);
});

test("the default model's harness is said only when it is not the choice", () => {
  assert.equal(runsOnLine({ harness: "agentry-harness", runsOn: "agentry-harness" }), null);
  assert.equal(runsOnLine({ harness: "docker-agent", runsOn: "" }), null);
  assert.equal(
    runsOnLine({ harness: "agentry-harness", runsOn: "codex-image" }),
    "Your default model is paid by a model sign-in, so runs on it use Codex whatever is chosen here.",
  );
});

test("choosing the Agentry harness warns about an older CLI, and going back does not", () => {
  assert.match(changedLine("agentry-harness"), /use the Agentry harness from their next start/);
  assert.match(changedLine("agentry-harness"), /older than v0\.7\.57/);
  assert.doesNotMatch(changedLine("docker-agent"), /older than/);
});
