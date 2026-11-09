// Settings → Models' rules: what a provider is called and what pays for it,
// its credential's state and fix, what a saved model is assigned to, the cap,
// and what a removal asks before it goes ahead.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addHeld, assignedLine, countLine, fixIsKey, fixIsSignIn, paidWith, providerName, refusalLine, removalAsk, removalHeld, removalNeedsTarget, stateBadge, suggestedName,
  type ProviderRow, type SavedModelRow,
} from "../src/savedModels.ts";

const lab: ProviderRow = { id: "custom-lab", vendor: "custom", name: "Lab", baseUrl: "http://gpu-box.lan:8000/v1", way: "key", takesNoKey: true, state: "ready", models: ["q", "l"] };
const openai: ProviderRow = { id: "openai", vendor: "openai", way: "key", keyEnv: "OPENAI_API_KEY", keySource: "keyring", state: "key-missing", models: ["g"] };
const chatgpt: ProviderRow = { id: "openai", vendor: "openai", way: "chatgpt", state: "needs-sign-in", broken: "invalid_grant", signInVendor: "chatgpt", models: [] };
const claude: ProviderRow = { id: "anthropic", vendor: "anthropic", way: "claude", state: "ready", account: "you@example.com", models: [] };
const credits: ProviderRow = { id: "agentry", vendor: "agentry", way: "credits", state: "needs-sign-in", models: [] };

const qwen: SavedModelRow = { id: "q", name: "Qwen", provider: "custom-lab", model: "qwen3", default: true, users: [] };
const llama: SavedModelRow = { id: "l", name: "Llama", provider: "custom-lab", model: "llama4", default: false, users: [{ kind: "schedule", id: "abc", name: "Morning brief", model: "l" }] };
const gpt: SavedModelRow = { id: "g", name: "GPT", provider: "openai", model: "gpt-5.5", default: false, users: [] };

test("a provider is called by the person's name for an endpoint of their own, else by the product's", () => {
  assert.equal(providerName(lab), "Lab");
  assert.equal(providerName(openai), "OpenAI");
  assert.equal(providerName({ id: "custom", vendor: "custom" }), "custom");
});

test("what pays for a provider is said in words, never a key", () => {
  assert.equal(paidWith(lab), "Takes no key");
  assert.equal(paidWith(openai), "An API key in this computer's keychain");
  assert.equal(paidWith({ ...openai, keySource: "" }), "An API key from $OPENAI_API_KEY");
  assert.equal(paidWith(chatgpt), "Your ChatGPT plan");
  assert.equal(paidWith(claude), "Your Claude subscription, through Claude Code");
  assert.equal(paidWith(credits), "Your Agentry credits");
});

test("a missing key reads Key missing and a lost sign-in Needs sign-in, each with its fix", () => {
  const k = stateBadge(openai);
  assert.equal(k.label, "Key missing");
  assert.equal(k.tone, "warn");
  assert.equal(k.fix, "The key is not in this computer's keychain.");
  assert.match(stateBadge({ ...openai, keySource: "" }).fix, /^\$OPENAI_API_KEY is not set where Agentry Desktop starts\. Keep the key/);
  assert.ok(fixIsKey(openai) && !fixIsSignIn(openai));

  const s = stateBadge(chatgpt);
  assert.equal(s.label, "Needs sign-in");
  assert.equal(s.fix, "The last refresh of the sign-in was refused (invalid_grant).");
  assert.ok(fixIsSignIn(chatgpt) && !fixIsKey(chatgpt));

  assert.match(stateBadge(credits).fix, /not signed in to your SecureAgentry account/);
  assert.ok(!fixIsSignIn(credits), "credits are signed in from Account, not here");

  assert.deepEqual(stateBadge(claude), { label: "Signed in as you@example.com", tone: "ok", fix: "" });
  assert.deepEqual(stateBadge(lab), { label: "Ready", tone: "ok", fix: "" });
});

test("each saved model says what it is assigned to", () => {
  assert.equal(assignedLine(qwen), "Runs, schedules and agents that name no model use it.");
  assert.equal(assignedLine(llama), "The schedule Morning brief names it.");
  assert.equal(assignedLine(gpt), "Nothing names it yet: pick it when you start a run.");
  const both = { ...qwen, users: [llama.users[0], { kind: "agent", id: "assistant", name: "Assistant", model: "q" }] };
  assert.equal(assignedLine(both), "Runs, schedules and agents that name no model use it, and the schedule Morning brief and the agent Assistant name it too.");
});

test("at ten saved models Add is held, with the reason", () => {
  assert.equal(addHeld(9), "");
  assert.match(addHeld(10), /keeps 10 saved models, the most it can\. Remove one to add another\./);
  assert.equal(countLine(3), "3 of 10 saved models.");
});

test("removing a model nothing names and that is not the default asks nothing more", () => {
  const a = removalAsk([qwen, llama, gpt], ["g"]);
  assert.ok(!removalNeedsTarget(a));
  assert.equal(removalHeld(a, ""), "");
});

test("removing a model something names offers the default first, and nothing moves until one is picked", () => {
  const a = removalAsk([qwen, llama, gpt], ["l"]);
  assert.ok(removalNeedsTarget(a));
  assert.equal(a.stayingDefault?.id, "q");
  assert.deepEqual(a.targets.map((m) => m.id), ["q", "g"]);
  assert.equal(a.users.length, 1);
  assert.equal(removalHeld(a, ""), "Choose where they go first.");
  assert.equal(removalHeld(a, "l"), "Choose where they go first.", "never the model removed");
  assert.equal(removalHeld(a, "q"), "");
});

test("removing the default asks for the new default, which is never one removed", () => {
  const a = removalAsk([qwen, llama, gpt], ["q"]);
  assert.ok(a.takesDefault && removalNeedsTarget(a));
  assert.equal(a.stayingDefault, undefined);
  assert.equal(removalHeld(a, ""), "Choose the new default first.");
  assert.equal(removalHeld(a, "g"), "");

  // A provider's removal takes every model on it.
  const p = removalAsk([qwen, llama, gpt], ["q", "l"]);
  assert.deepEqual(p.targets.map((m) => m.id), ["g"]);
  assert.equal(removalHeld(p, "l"), "Choose the new default first.");
  assert.equal(removalHeld(removalAsk([qwen], ["q"]), ""), "There is no other saved model to move them to: add one first.");
});

test("a new model's name starts as its id, made unique", () => {
  assert.equal(suggestedName("qwen3", ["Qwen"]), "qwen3");
  assert.equal(suggestedName("qwen3", ["QWEN3", "qwen3 2"]), "qwen3 3");
  assert.equal(suggestedName("  ", []), "");
});

test("a refusal of the check says what to do next", () => {
  assert.equal(refusalLine({ kind: "cannot-check", reason: "could not check the model: dial tcp: connection refused" }),
    "Could not check the model: dial tcp: connection refused. Nothing was saved.");
  assert.match(refusalLine({ kind: "not-offered", reason: "custom-lab does not offer \"gemma\"" }), /Pick one it offers\.$/);
  assert.match(refusalLine({ kind: "key-missing", reason: "could not check the model: no key for anthropic" }), /\. Add the key first\.$/);
  assert.match(refusalLine({ kind: "needs-sign-in", reason: "could not check the model: Claude Code is not signed in" }), /\. Sign in again first\.$/);
});
