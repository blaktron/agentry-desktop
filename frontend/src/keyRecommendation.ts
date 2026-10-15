import type { main } from "../wailsjs/go/models";

// The AI Review key's recommendation: the words, the link's label and
// where the link goes, in one place, so every surface that recommends a key
// says the same thing and sends the person to the same address. The address
// is a constant: no registry answer and no setting can change it. And where
// the key stands, decided once for the card, Settings' status line
// and the wizard's recap, which each word it their own way. Pure, with a
// type-only import, so node's own test runner can hold it without a browser.

/** Where a person gets a TypeSafe key. Opened in the system browser. */
export const TYPESAFE_URL = "https://typesafe.ai";

/** Why a person who builds QuickAgents should have their own key. */
export const KEY_RECOMMENDATION =
  "Agentry's AI Review is powered by TypeSafe AI's Jev System One. We highly recommend signing up with TypeSafe and " +
  "getting an API key if you want to build shareable QuickAgents or submit them to Agentry.";

export const GET_A_KEY_LABEL = "Get a key from TypeSafe";

/** Where the person's key stands: "env" the variable the app was started
 *  with holds one, and goes first; "saved" the keychain holds one and the
 *  runner policy reads it there; "unread" the keychain holds one the policy
 *  does not read; "none". Go's Ready (JevKeyStatus's and HybridKeyStatus's,
 *  which keyStanding reads the same way) is "env" or "saved". */
export type KeyStanding = "env" | "saved" | "unread" | "none";

export function keyStanding(k: Pick<main.JevKeyView, "envSet" | "stored" | "source">): KeyStanding {
  if (k.envSet) return "env";
  if (k.stored) return k.source === "keyring" ? "saved" : "unread";
  return "none";
}
