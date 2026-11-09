// A runhost schedule's Model list: the option it starts on, the greyed one
// for a model none of the runhost's, and what a pick asks the runhost.
import { test } from "node:test";
import assert from "node:assert/strict";
import { MADE_WITH, scheduleExtraOption, scheduleModelSaid, scheduleMove, scheduleValue } from "../src/runhostSchedules.ts";

const models = [
  { id: "bea51c0a", name: "Qwen", provider: "custom-lab", model: "qwen3", default: true },
  { id: "a7002bb0", name: "Llama", provider: "custom-lab", model: "llama3" },
];

test("the list starts on the default, the saved model, or the model it was made with", () => {
  assert.equal(scheduleValue("default"), "");
  assert.equal(scheduleValue("a7002bb0"), "a7002bb0");
  assert.equal(scheduleValue(""), MADE_WITH);
  assert.equal(scheduleValue(undefined), MADE_WITH);
});

test("a model none of the runhost's is a greyed option, and one it holds needs none", () => {
  assert.equal(scheduleExtraOption(models, "default"), null);
  assert.equal(scheduleExtraOption(models, "a7002bb0"), null);
  const made = scheduleExtraOption(models, "");
  assert.ok(made && made.value === MADE_WITH && made.disabled);
  const gone = scheduleExtraOption(models, "0badc0de");
  assert.ok(gone && gone.value === "0badc0de" && gone.disabled && /no longer holds/.test(gone.label));
});

test("a pick asks the runhost for the saved model or its default, and the current option asks nothing", () => {
  assert.equal(scheduleMove("bea51c0a", "a7002bb0"), "a7002bb0");
  assert.equal(scheduleMove("a7002bb0", ""), "");
  assert.equal(scheduleMove("default", ""), null);
  assert.equal(scheduleMove("a7002bb0", "a7002bb0"), null);
  assert.equal(scheduleMove("", "a7002bb0"), "a7002bb0");
  assert.equal(scheduleMove("", ""), "");
  assert.equal(scheduleMove("", MADE_WITH), null);
});

test("where no list is drawn, the model is said by name, as the default, or as the one it was made with", () => {
  assert.equal(scheduleModelSaid(models, "a7002bb0"), "Llama");
  assert.equal(scheduleModelSaid(models, "default"), "the default (Qwen)");
  assert.equal(scheduleModelSaid(models, ""), "the model it was made with");
  assert.equal(scheduleModelSaid(null, "default"), "the default");
  assert.equal(scheduleModelSaid(null, "a7002bb0"), "a saved model");
});
