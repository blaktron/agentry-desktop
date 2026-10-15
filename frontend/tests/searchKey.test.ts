// The person's own web search key: the card's words
// for where the key stands, and for a save that replaces another
// provider's key, decided once.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import { initialProvider, replaceNote, searchKeySentence } from "../src/searchKey.ts";

const providers = [
  { name: "brave", label: "Brave", envName: "BRAVE_API_KEY" },
  { name: "exa", label: "Exa", envName: "EXA_API_KEY" },
  { name: "tavily", label: "Tavily", envName: "TAVILY_API_KEY" },
];
const view = (v: Record<string, unknown>) => ({ providers, stored: false, envSet: false, ready: false, ...v }) as unknown as main.SearchKeyView;

test("the picker starts on the policy's provider, else the first offered", () => {
  assert.equal(initialProvider(view({})), "brave");
  assert.equal(initialProvider(view({ provider: "exa" })), "exa");
});

test("where the key stands: none, the variable first, saved, unread, missing", () => {
  assert.match(searchKeySentence(view({})), /No search provider chosen/);
  assert.match(searchKeySentence(view({ provider: "tavily", providerLabel: "Tavily", envSet: true, envName: "TAVILY_API_KEY", stored: true, source: "keyring" })),
    /^\$TAVILY_API_KEY is set .* searches Tavily with it before the keychain/);
  assert.equal(searchKeySentence(view({ provider: "tavily", providerLabel: "Tavily", stored: true, source: "keyring" })),
    "Tavily is chosen, and its key is saved in this computer's keychain.");
  assert.match(searchKeySentence(view({ provider: "exa", providerLabel: "Exa", stored: true, source: "" })), /does not read it from there/);
  assert.match(searchKeySentence(view({ provider: "exa", providerLabel: "Exa" })), /no key for it is saved/);
});

test("a save for another provider says it replaces the saved key; the same provider, or nothing saved, says nothing", () => {
  const saved = view({ provider: "brave", providerLabel: "Brave", stored: true, source: "keyring" });
  assert.match(searchKeySentence(view({ provider: "brave", providerLabel: "Brave" })), /Brave is chosen, but no key for it is saved/);
  assert.equal(replaceNote(saved, "exa"), "Saving makes Exa your search provider and removes the Brave key from this computer's keychain.");
  assert.equal(replaceNote(saved, "brave"), "");
  assert.equal(replaceNote(view({ provider: "brave", providerLabel: "Brave" }), "exa"), "");
  assert.equal(replaceNote(view({}), "exa"), "");
  // A key the policy does not read from the keychain is not removed, so not promised.
  assert.equal(replaceNote(view({ provider: "brave", providerLabel: "Brave", stored: true, source: "" }), "exa"), "");
});
