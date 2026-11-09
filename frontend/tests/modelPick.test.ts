// The Model dropdowns: what each offers, greys and blocks; the Home card's
// model line; where a saved model is available; Recent runs' model.
import { test } from "node:test";
import assert from "node:assert/strict";
import { defaultLabel, greyedLines, modelOptions, modelSaid, pickBlocker, pickLine, type PickModel } from "../src/modelPick.ts";
import { agentModelLine } from "../src/homeAgent.ts";
import { availableLine, availableOn, runhostMoveTargets } from "../src/savedModels.ts";
import { modelRan, rowLine } from "../src/recentRuns.ts";

const models: PickModel[] = [
  { id: "m1", name: "Haiku", provider: "anthropic", model: "claude-haiku-5", default: true },
  { id: "m2", name: "Qwen", provider: "custom-lab", model: "qwen3" },
  { id: "m3", name: "GPT", provider: "openai", model: "gpt-5.5", refusal: "Key missing: add its key in Settings → Models." },
];

test("a dropdown starts on the default and offers every saved model, greying one that cannot be used", () => {
  const opts = modelOptions(models);
  assert.deepEqual(opts.map((o) => o.value), ["", "m1", "m2", "m3"]);
  assert.equal(opts[0].label, "Default (Haiku)");
  assert.equal(opts[0].disabled, false);
  assert.equal(opts[2].label, "Qwen — custom-lab/qwen3");
  assert.equal(opts[3].disabled, true);
  assert.match(opts[3].label, /cannot be used now/);
  assert.equal(opts[3].reason, "Key missing: add its key in Settings → Models.");
  assert.deepEqual(greyedLines(models), [{ id: "m3", line: "GPT — openai/gpt-5.5: Key missing: add its key in Settings → Models." }]);
  assert.equal(defaultLabel([]), "Default");
});

test("the default is greyed with the default model's own reason", () => {
  const broken = models.map((m) => (m.default ? { ...m, refusal: "Needs sign-in: sign in again in Settings → Models." } : m));
  const opts = modelOptions(broken);
  assert.equal(opts[0].disabled, true);
  assert.equal(pickLine(broken, ""), "Needs sign-in: sign in again in Settings → Models.");
});

test("a pick that is gone or cannot be used blocks the run, by name; the default and a usable pick block nothing", () => {
  assert.equal(pickBlocker(models, ""), "");
  assert.equal(pickBlocker(models, "m2"), "");
  assert.equal(pickBlocker(models, "m3"), "GPT cannot be used now. Key missing: add its key in Settings → Models.");
  assert.match(pickBlocker(models, "m9"), /no longer on this computer/);
  assert.equal(pickLine(models, ""), "Runs on your default saved model, now Haiku (anthropic/claude-haiku-5).");
  assert.equal(pickLine(models, "m2"), "");
});

test("a record's model is said by name, the default by the one it is now", () => {
  assert.equal(modelSaid(models, "m2"), "Qwen");
  assert.equal(modelSaid(models, ""), "the default (Haiku)");
  assert.equal(modelSaid(models, "default"), "the default (Haiku)");
  assert.equal(modelSaid(models, "m9"), "a saved model no longer kept");
});

test("Home's agent card names the agent's model, the default it follows, or why it cannot run", () => {
  assert.equal(agentModelLine(undefined), "");
  assert.equal(agentModelLine({ id: "", name: "Haiku", provider: "anthropic", model: "claude-haiku-5", followsDefault: true }), "Model: the default, Haiku (anthropic/claude-haiku-5)");
  assert.equal(agentModelLine({ id: "m2", name: "Qwen", provider: "custom-lab", model: "qwen3", followsDefault: false }), "Model: Qwen (custom-lab/qwen3)");
  assert.equal(agentModelLine({ id: "m3", name: "GPT", provider: "openai", model: "gpt-5.5", followsDefault: false, code: "key-missing", refusal: "Key missing: add its key in Settings → Models." }),
    "Model: GPT (openai/gpt-5.5). Key missing: add its key in Settings → Models.");
  assert.match(agentModelLine({ id: "gone", name: "", provider: "", model: "", followsDefault: false, code: "missing", refusal: "x" }), /gone/);
});

test("a saved model is available on the runhosts that hold it, by its id", () => {
  const runhosts = [
    { name: "lab", models: [{ id: "m2", name: "Qwen", default: true }, { id: "m1", name: "Haiku" }] },
    { name: "edge", models: [] },
    { name: "new", models: null },
  ];
  assert.deepEqual(availableOn("m2", runhosts), [
    { runhost: "lab", holds: true, isDefault: true, unknown: false },
    { runhost: "edge", holds: false, isDefault: false, unknown: false },
    { runhost: "new", holds: false, isDefault: false, unknown: true },
  ]);
  assert.equal(availableLine("m2", runhosts), "Available on lab (its default).");
  assert.equal(availableLine("m3", runhosts), "Available on this computer only.");
  assert.deepEqual(runhostMoveTargets("m2", runhosts[0]), [{ id: "m1", name: "Haiku" }]);
});

test("Recent runs name the saved model a run was on", () => {
  assert.equal(modelRan({ model: "Qwen" }), "on Qwen");
  assert.equal(modelRan({}), "");
  const row = { id: "r1", kind: "run", where: "here", outcome: "ok", tone: "ok", model: "Qwen" };
  assert.match(rowLine(row, new Date()), /^On this computer on Qwen · /);
});
