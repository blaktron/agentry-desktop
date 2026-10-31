// The run setup's readiness rules: which setup the panel shows in place,
// what the list above Run says for it, and the line beside the button.
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { engineGapLine, gapBlocker, modelGapLine, runButtonWhy, setupGap } from "../src/runReadiness.ts";

const status = (v: Record<string, unknown>) => ({ dockerOk: true, modelReady: true, imagesReady: true, ...v }) as unknown as app.SetupStatusView;

test("the engine comes first, then the model, and a run In Agentry needs neither", () => {
  assert.equal(setupGap(null, false), null, "a status not read yet is no gap: the blocker says it is being checked");
  assert.equal(setupGap(status({ dockerOk: false, modelReady: false }), false), "engine");
  assert.equal(setupGap(status({ dockerOk: true, modelReady: false }), false), "model");
  assert.equal(setupGap(status({}), false), null);
  assert.equal(setupGap(status({ dockerOk: false, modelReady: false }), true), null);
});

test("the engine's blocker says no engine answers and that the answers stay; the engine's own words are the card's", () => {
  const line = engineGapLine(false);
  assert.match(line, /^No container engine answers on this computer, and a QuickAgent run here needs one\./);
  assert.match(line, /everything you entered stays/);
  assert.doesNotMatch(line, /In Agentry/);
  assert.match(engineGapLine(true), /Or run this release In Agentry\.$/);
  const withErr = gapBlocker(status({ dockerOk: false, dockerErr: "the docker socket is not there" }), false, false) ?? "";
  assert.doesNotMatch(withErr, /docker socket/, "the engine's words are drawn once, on the card");
});

test("the model's blocker is Go's sentence for what is missing, with a fallback", () => {
  const gap = "$ANTHROPIC_API_KEY is not set in this app's environment, and no anthropic key is saved on this computer.";
  assert.equal(modelGapLine(status({ modelReady: false, modelGap: gap }), false), `${gap} Set it up in this panel; everything you entered stays.`);
  assert.match(modelGapLine(status({ modelReady: false }), true), /^No model provider is set up for runs on this computer\. .* Or run this release In Agentry\.$/);
});

test("gapBlocker is the gap's line, and nothing when there is no gap", () => {
  assert.equal(gapBlocker(status({}), false, true), null);
  assert.equal(gapBlocker(null, false, true), null);
  assert.match(gapBlocker(status({ dockerOk: false }), false, false) ?? "", /^No container engine/);
  assert.match(gapBlocker(status({ modelReady: false, modelGap: "No key." }), false, false) ?? "", /^No key\./);
  assert.equal(gapBlocker(status({ dockerOk: false }), true, true), null);
});

test("the Run button's line is the first blocker, counting the rest, and nothing while starting or when Run is on", () => {
  assert.equal(runButtonWhy([], false), null);
  assert.equal(runButtonWhy(["Answer: Since when?"], true), null);
  assert.equal(runButtonWhy(["Answer: Since when?"], false), "Answer: Since when?");
  assert.equal(runButtonWhy(["Answer: Since when?", "Tick “Run an unsigned release”."], false), "Answer: Since when? And one more thing, listed above.");
  assert.equal(runButtonWhy(["A", "B", "C"], false), "A And 2 more things, listed above.");
});
