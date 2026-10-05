// Home's card registry (plan desktop-home-and-library.md §4.4, D-067;
// agentry-desktop#658). Pure, so the node tests can import it.
//
// Go owns the layout (HomeAPI; desktop.yaml's home key): which cards, in
// what order, which hidden. This module says what each kind is, and which
// of the layout's cards the page draws. A new kind is one entry here and
// one renderer beside the page (pages/home/).

export type HomeCardKind = "recent-runs" | "installed" | "profile" | "run-defaults" | "pinned";

export type HomeCardDescriptor = {
  kind: HomeCardKind;
  // What Settings → Home calls it.
  title: string;
  // One card of the kind, or several (a Pinned card holds four pins, D5).
  multiple: boolean;
};

export const HOME_CARDS: Record<HomeCardKind, HomeCardDescriptor> = {
  "recent-runs": { kind: "recent-runs", title: "Recent runs", multiple: false },
  installed: { kind: "installed", title: "Installed QuickAgents", multiple: false },
  profile: { kind: "profile", title: "Profile", multiple: false },
  "run-defaults": { kind: "run-defaults", title: "Run defaults", multiple: false },
  pinned: { kind: "pinned", title: "Pinned", multiple: true },
};

import type { settings } from "../wailsjs/go/models";

// The one size today, the Build, Voice and Knowledge cards' (D12).
export type HomeCardSize = "large";

export type LayoutCard = Pick<settings.HomeCard, "kind" | "hidden" | "size">;
export type DrawnCard = { kind: HomeCardKind; key: string; size: HomeCardSize };

const isKind = (k: string): k is HomeCardKind => Object.prototype.hasOwnProperty.call(HOME_CARDS, k);

/** The layout's cards the page draws, in the layout's order: not hidden,
 * of a kind this app knows and has a renderer for (drawable), a single
 * kind once. Every card is Large until smaller sizes exist. */
export function cardsToDraw(cards: LayoutCard[], drawable: ReadonlySet<string>): DrawnCard[] {
  const out: DrawnCard[] = [];
  const seen = new Set<string>();
  cards.forEach((c, i) => {
    if (c.hidden || !isKind(c.kind) || !drawable.has(c.kind)) return;
    if (!HOME_CARDS[c.kind].multiple) {
      if (seen.has(c.kind)) return;
      seen.add(c.kind);
    }
    out.push({ kind: c.kind, key: HOME_CARDS[c.kind].multiple ? `${c.kind}-${i}` : c.kind, size: "large" });
  });
  return out;
}
