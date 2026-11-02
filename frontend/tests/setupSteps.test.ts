// The one line a person waiting on the image preparation or a sign-in reads.
import { test } from "node:test";
import assert from "node:assert/strict";
import { setupStep, signInWaiting } from "../src/setupSteps.ts";

test("the step is the newest line that names one, by the image's own name", () => {
  assert.equal(setupStep([]), null);
  assert.equal(setupStep(["policy written: /x"]), null);
  const log = [
    "pulling ghcr.io/blaktron/agentry-mcp-gateway:v0.44.3@sha256:abc (first use)",
    "building agentry-runner:0.7.3 from embedded context with GO_IMAGE=x (first use)",
  ];
  assert.equal(setupStep(log.slice(0, 1)), "Downloading the tool gateway image…");
  assert.match(setupStep(log) ?? "", /^Building the runner 0\.7\.3 image on this computer/);
  assert.equal(setupStep([...log, "a docker build line", "#5 [2/3] RUN set -e"]), setupStep(log));
});

test("the newest release first, its build with its version, and the fallback are each said", () => {
  const newest = "claude-image: the vendor's newest release is 2.1.316, newer than the tested 2.1.296; building it first, with the tested release as the fallback";
  assert.equal(setupStep([newest]), "Getting Claude Code 2.1.316, the newest release, ready…");
  assert.equal(
    setupStep([newest, "building agentry-claude-code:2.1.316 from @anthropic-ai/claude-code@2.1.316 …"]),
    "Building the Claude Code 2.1.316 image on this computer — this can take a few minutes…",
  );
  assert.equal(
    setupStep([newest, "the newest claude-image release 2.1.316 could not be used (refused); using the tested release 2.1.296"]),
    "The newest release could not be used; preparing the tested one…",
  );
  assert.equal(setupStep(["pulling ghcr.io/blaktron/agentry-codex:0.166.0-2026.10.30.4@sha256:abc (first use)"]), "Downloading the Codex image…");
});

test("only the Claude sign-in prepares something before the browser", () => {
  assert.equal(signInWaiting("claude"), "Getting Claude Code ready…");
  assert.equal(signInWaiting("chatgpt"), "Waiting for you in your browser…");
});
