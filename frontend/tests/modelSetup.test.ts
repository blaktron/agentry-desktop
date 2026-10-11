// Model setup's rules (agentry-desktop#734, plan model-sign-ins.md §4.1):
// the ways each provider offers, a sign-in first; which one the saved
// policy uses; what is kept; and what a choice needs before Save.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  CLAUDE_SIGN_IN_LABEL, PROVIDERS, copilotModelAfterList, copilotModelLabel, copilotOffered, keptFor, keptLine, modelAsk, removeLabel, savedWay, signInSaveHint, startingModel, waysFor,
  type KeptSignIn,
} from "../src/modelSetup.ts";

const chatgpt: KeptSignIn = { vendor: "chatgpt", provider: "openai", kind: "chatgpt-plan", connected: false, account: "person@example.com", image: true };
const claudeImage: KeptSignIn = { vendor: "claude", provider: "anthropic", kind: "claude-signin", connected: false, account: "person@example.com (Example)", image: true };
const claudeInstalledOnly: KeptSignIn = { ...claudeImage, image: false };
const openrouter: KeptSignIn = { vendor: "openrouter", provider: "openrouter", kind: "api", connected: true, account: "person", image: true };
const poe: KeptSignIn = { vendor: "poe", provider: "poe", kind: "api", connected: true, keyExpiresAt: "2026-11-11T00:00:00Z", image: true };

test("each provider offers the plan's choices, a sign-in before a key", () => {
  const ways = (id: string) => waysFor(id).map((w) => w.id).join(" ");
  assert.equal(ways("openai"), "chatgpt key");
  assert.equal(ways("anthropic"), "claude key");
  assert.equal(ways("copilot"), "copilot");
  for (const id of ["openrouter", "poe", "huggingface"]) assert.equal(ways(id), "connect key", id);
  assert.equal(ways("google"), "key");
  assert.equal(ways("custom"), "key");
  assert.equal(ways("mistral"), "key", "a provider the list does not know is paid by a key");
  assert.equal(waysFor("anthropic")[0].label, CLAUDE_SIGN_IN_LABEL);
});

test("no choice asks anyone to paste a code or names OAuth, and no sign-in speaks of tokens", () => {
  for (const p of PROVIDERS) {
    for (const w of p.ways) {
      const words = `${w.label} ${w.hint}`;
      assert.doesNotMatch(words, /OAuth|paste (a|the) code/i, `${p.id}/${w.id}: ${words}`);
      // A key's own name stays the vendor's (Hugging Face calls it an
      // access token); a sign-in or a connect never says it.
      if (w.id !== "key") assert.doesNotMatch(words, /token|API key/i, `${p.id}/${w.id}: ${words}`);
    }
  }
});

test("the saved policy's way: its sign-in kind, a kept connect, else a key", () => {
  assert.equal(savedWay("openai", "openai", "chatgpt-plan", []), "chatgpt");
  assert.equal(savedWay("openai", "openai", "", []), "key");
  assert.equal(savedWay("anthropic", "anthropic", "claude-signin", []), "claude");
  assert.equal(savedWay("anthropic", "anthropic", "subscription", []), "key", "DR-26's pasted setup-token is a key here");
  assert.equal(savedWay("copilot", "copilot", "copilot-signin", []), "copilot");
  assert.equal(savedWay("openrouter", "openrouter", "", [openrouter]), "connect");
  assert.equal(savedWay("openrouter", "openrouter", "", []), "key", "a pasted key forgets the connect");
  assert.equal(savedWay("openai", "anthropic", "claude-signin", [chatgpt]), "chatgpt", "another provider's choice starts on its first way");
  assert.equal(savedWay("anthropic", "anthropic", "", [], false), "claude", "nobody chose a provider yet: the sign-in leads");
});

test("a Claude sign-in counts where the Claude Code image is signed in", () => {
  const claude = waysFor("anthropic")[0];
  assert.equal(keptFor(claude, [claudeImage]), claudeImage);
  assert.equal(keptFor(claude, [claudeInstalledOnly]), undefined);
  assert.equal(keptFor(waysFor("anthropic")[1], [claudeImage]), undefined, "a key keeps no sign-in");
  assert.equal(keptFor(waysFor("openrouter")[0], [openrouter, chatgpt]), openrouter);
});

test("what a kept sign-in says, and how it is removed", () => {
  assert.equal(keptLine(chatgpt), "Signed in as person@example.com");
  assert.equal(keptLine(openrouter), "Connected as person");
  assert.equal(keptLine(poe), "Connected — the key works until 2026-11-11");
  assert.match(keptLine({ ...chatgpt, broken: "invalid_grant" }), /^Sign in again/);
  assert.equal(removeLabel(chatgpt), "Remove");
  assert.equal(removeLabel(openrouter), "Disconnect");
});

test("how each way asks for the model", () => {
  assert.equal(modelAsk("openai", "chatgpt", ["gpt-5.2", "gpt-5-mini"]).mode, "plan");
  assert.equal(modelAsk("openai", "chatgpt", ["gpt-5.2"]).defaultModel, "gpt-5.2");
  assert.deepEqual(modelAsk("anthropic", "claude", []).options, ["sonnet", "opus", "haiku"]);
  assert.equal(modelAsk("copilot", "copilot", []).defaultModel, "auto");
  const hf = modelAsk("huggingface", "connect", []);
  assert.equal(hf.mode, "typed");
  assert.equal(hf.defaultModel, "", "the person types the model id (D33)");
  assert.match(hf.placeholder, /:groq/);
  assert.equal(modelAsk("anthropic", "key", []).defaultModel, "claude-sonnet-4-5");
});

test("a sign-in way saves once something is signed in and a model is named", () => {
  const way = waysFor("openai")[0];
  assert.equal(signInSaveHint(way, undefined, "gpt-5.2"), "Sign in with ChatGPT first");
  assert.equal(signInSaveHint(way, { ...chatgpt, broken: "x" }, "gpt-5.2"), "sign in again first");
  assert.equal(signInSaveHint(way, chatgpt, " "), "pick a model");
  assert.equal(signInSaveHint(waysFor("poe")[0], poe, ""), "type the model id");
  assert.equal(signInSaveHint(way, chatgpt, "gpt-5.2"), "");
});

test("a way starts on the saved model only where the policy already pays that way", () => {
  const saved = { provider: "anthropic", model: "opus", way: "claude" as const };
  assert.equal(startingModel("anthropic", "claude", saved, []), "opus");
  assert.equal(startingModel("anthropic", "key", saved, []), "claude-sonnet-4-5");
  assert.equal(startingModel("openai", "chatgpt", saved, ["gpt-5.2"]), "gpt-5.2");
});

test("Settings' summary says what pays where a sign-in does", async () => {
  const { paidBy } = await import("../src/modelSetup.ts");
  assert.equal(paidBy("claude-signin"), "your Claude subscription");
  assert.equal(paidBy("chatgpt-plan"), "your ChatGPT plan");
  assert.equal(paidBy("copilot-signin"), "your Copilot plan");
  assert.equal(paidBy("api"), "");
  assert.equal(paidBy(""), "");
});

test("a Copilot plan's listed models are offered, auto first, each once (agentry-desktop#811)", () => {
  assert.equal(modelAsk("copilot", "copilot", []).mode, "typed", "before the list, the id is typed");
  const ask = modelAsk("copilot", "copilot", ["claude-sonnet-5.5", "gpt-5-mini", "claude-sonnet-5.5"]);
  assert.equal(ask.mode, "plan");
  assert.deepEqual(ask.options, ["auto", "claude-sonnet-5.5", "gpt-5-mini"]);
  assert.equal(ask.defaultModel, "auto");
  assert.deepEqual(copilotOffered(["auto", "gpt-5-mini"]), ["auto", "gpt-5-mini"], "a listed auto is not offered twice");
  const models = [
    { id: "claude-opus-5.5", name: "Claude Opus 5.5", multiplier: 10 },
    { id: "gpt-5-mini", name: "gpt-5-mini", multiplier: 0 },
  ];
  assert.equal(copilotModelLabel("auto", models), "auto — Copilot chooses");
  assert.equal(copilotModelLabel("claude-opus-5.5", models), "claude-opus-5.5 — Claude Opus 5.5, 10× premium requests");
  assert.equal(copilotModelLabel("gpt-5-mini", models), "gpt-5-mini", "a multiplier GitHub did not give is not shown as free");
  assert.equal(copilotModelAfterList("gpt-5-mini", ["gpt-5-mini"]), "gpt-5-mini");
  assert.equal(copilotModelAfterList("claude-opus-5.5", ["gpt-5-mini"]), "auto", "a model the plan no longer offers falls back to auto");
});
