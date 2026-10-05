// Publisher Earnings in the desktop (plan creator-monetization.md D10, D13;
// agentry-desktop#613): the sentences are the site's (agentry-web#1087,
// lib/earnings-view.ts), and whether a finished run asks to be starred is
// decided here. Go says whether the publisher earns and whether the
// QuickAgent is someone else's and not yet starred (services_earnings.go).

/** The one line the In Agentry run setup carries where the release's
 *  publisher is in Publisher Earnings (D10). */
export const PUBLISHER_EARNS_SENTENCE = "Its publisher earns a share of the credits runs of it spend.";

/** What the run setup adds after it, as the site's does. */
export const PUBLISHER_EARNS_AFTER = "What you pay does not change.";

/** The ask on a finished hosted run of someone else's QuickAgent (D10). */
export function starAskSentence(name: string): string {
  return `If this run helped, star ${name}. Stars help other people find it.`;
}

/** What the result says once the ask was answered. */
export function starredSentence(name: string): string {
  return `You starred ${name}.`;
}

/** starAskWanted: whether a run In Agentry is one whose result may ask to
 *  be starred: it finished, and it succeeded. A closed session finishes as
 *  a run does, its outcome succeeded (the site's rule). A run that failed,
 *  was cancelled, or that this app lost touch with asks nothing. Whose
 *  QuickAgent it is, and whether it is starred, is Go's to read
 *  (RunAPI.StarAsk). */
export function starAskWanted(run: { status: string; outcome?: string; final: boolean; ref: string }): boolean {
  return run.final && run.status === "finished" && run.outcome === "succeeded" && run.ref.includes("/");
}
