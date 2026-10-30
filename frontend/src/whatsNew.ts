// What's new: the cards of the versions Agentry Desktop moved through since
// the person last looked, newest first, and what one step through them shows.
// The highlights are compiled into the app (Go's internal/highlights); this is
// only how the screen walks them.

export type WhatsNewCard = { version: string; highlights: string[] };
export type WhatsNewData = { current: string; cards: WhatsNewCard[]; earlier: number; releasesUrl: string };

/** Why the screen is open: on start after a newer version, or from Settings. */
export type WhatsNewMode = "start" | "settings";

export type WhatsNewStep = {
  card: WhatsNewCard;
  title: string;
  /** The line under the title: what version is running. */
  lead: string;
  /** "2 of 5", or "" when there is one card. */
  counter: string;
  backDisabled: boolean;
  /** The last card: Next reads Done and closes. */
  last: boolean;
  nextLabel: "Next" | "Done";
  /** On the last card only, the older versions not shown, counted. */
  earlier: string;
};

/** "And 1 earlier version." / "And 12 earlier versions.", "" for none. */
export function earlierLine(n: number): string {
  if (n <= 0) return "";
  return `And ${n} earlier version${n === 1 ? "" : "s"}.`;
}

/** The step at `at`, clamped into the cards; null when there are none. */
export function whatsNewStep(data: WhatsNewData, at: number, mode: WhatsNewMode): WhatsNewStep | null {
  const total = data.cards.length;
  if (total === 0) return null;
  const i = Math.min(Math.max(0, at), total - 1);
  const last = i === total - 1;
  return {
    card: data.cards[i],
    title: `What's new in ${data.cards[i].version}`,
    lead: mode === "start" ? `Agentry Desktop is now ${data.current}.` : `You are running Agentry Desktop ${data.current}.`,
    counter: total > 1 ? `${i + 1} of ${total}` : "",
    backDisabled: i === 0,
    last,
    nextLabel: last ? "Done" : "Next",
    earlier: last ? earlierLine(data.earlier) : "",
  };
}

/** What a key does on the screen: move to a card, close it, or nothing. */
export function whatsNewKey(key: string, at: number, total: number): { at: number } | "close" | null {
  switch (key) {
    case "Escape":
      return "close";
    case "ArrowRight":
      return at < total - 1 ? { at: at + 1 } : null;
    case "ArrowLeft":
      return at > 0 ? { at: at - 1 } : null;
  }
  return null;
}
