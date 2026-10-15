import type { main } from "../wailsjs/go/models";

// The person's own web search key: where it stands,
// and what saving a key for another provider does, decided once for the
// Settings card. Pure, with a type-only import, so node's own test runner
// can hold it without a browser. The providers and their names come from
// the CLI's runner policy through SearchKeyStatus; nothing here lists them.

/** The provider the picker starts on: the policy's, else the first the
 *  policy offers (Brave, the registry's own). */
export function initialProvider(k: main.SearchKeyView): string {
  return k.provider || k.providers[0]?.name || "";
}

/** Where the key stands, in one sentence. */
export function searchKeySentence(k: main.SearchKeyView): string {
  if (!k.provider) {
    return "No search provider chosen. A draft on this computer reads the registry and the pages it knows, and does not search the web.";
  }
  const name = k.providerLabel || k.provider;
  if (k.envSet) {
    return `$${k.envName} is set where this app was started, and a draft searches ${name} with it before the keychain.`;
  }
  if (k.stored) {
    return k.source === "keyring"
      ? `${name} is chosen, and its key is saved in this computer's keychain.`
      : `A ${name} key is in the keychain, but the runner policy does not read it from there. Save it again to fix that.`;
  }
  return `${name} is chosen, but no key for it is saved. A draft on this computer does not search until you add one.`;
}

/** What Save will do to a key saved for another provider, or "" when it
 *  replaces nothing. */
export function replaceNote(k: main.SearchKeyView, chosen: string): string {
  // Go removes the replaced key only when the policy reads it from the
  // keychain; a key the policy does not read stays, and is not mentioned.
  if (!k.provider || k.provider === chosen || !k.stored || k.source !== "keyring") return "";
  const label = k.providers.find((p) => p.name === chosen)?.label ?? chosen;
  return `Saving makes ${label} your search provider and removes the ${k.providerLabel || k.provider} key from this computer's keychain.`;
}
