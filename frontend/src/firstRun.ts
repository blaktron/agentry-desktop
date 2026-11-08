// The words the first-run wizard shares with the rest of the app, and what
// the Home banner lists after the wizard is skipped. Pure, so node's own
// test runner can load it.

import type { WizardStep } from "./wizardPath";

/** The three places a run can happen, named as the whole app names them:
 *  the wizard's cards, Settings → Run defaults, Home and the run panel. */
export const PLACES = {
  local: "This computer",
  remote: "A runhost",
  cloud: "In Agentry",
} as const;

/** A place's name inside a sentence ("on this computer", "on a runhost",
 *  "In Agentry", which keeps its capitals as a name). */
export function placeInSentence(p: keyof typeof PLACES): string {
  return p === "cloud" ? PLACES.cloud : PLACES[p].charAt(0).toLowerCase() + PLACES[p].slice(1);
}

/** The Node.js release the agentry-mcp server needs (its engines floor):
 *  the coding agent step says it, and so does anything that installs it. */
export const MCP_NODE_MAJOR = 24;

/** What signing in is for, said where the wizard offers it and where the
 *  app says it is not done. */
export const SIGN_IN_OPTIONAL =
  "Signing in is optional: browsing public QuickAgents and running them on this computer need no account. " +
  "An account is for runs In Agentry, publishing and your library.";

/** One thing the Home banner names after the wizard was skipped, and the
 *  wizard step that does it. */
export type SkippedItem = { text: string; step: WizardStep; action: string };

/** What a skipped wizard left undone, from what the app can read now:
 *  the sign-in, and whether a run on this computer has a provider. Empty
 *  when both are done, and the banner is not shown. */
export function skippedItems(v: { signedIn: boolean; providerSetUp: boolean }): SkippedItem[] {
  const items: SkippedItem[] = [];
  if (!v.providerSetUp) {
    items.push({
      text: `No AI provider is set up for runs on ${placeInSentence("local")}, and no other place is chosen.`,
      step: "where",
      action: "Choose where runs happen",
    });
  }
  if (!v.signedIn) {
    items.push({ text: "You're not signed in. " + SIGN_IN_OPTIONAL, step: "signin", action: "Sign in" });
  }
  return items;
}
