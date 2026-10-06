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

export type LayoutCard = Pick<settings.HomeCard, "kind" | "hidden" | "size"> & { pins?: Pick<settings.HomePin, "ref" | "registry">[] };
// pins: a Pinned card's, in their rows' order (D11); empty for every other kind.
export type DrawnCard = { kind: HomeCardKind; key: string; size: HomeCardSize; pins: Pick<settings.HomePin, "ref" | "registry">[] };

const isKind = (k: string): k is HomeCardKind => Object.prototype.hasOwnProperty.call(HOME_CARDS, k);

/** The layout's cards the page draws, in the layout's order: not hidden,
 * of a kind this app knows and has a renderer for (drawable), a single
 * kind once, a Pinned card only while it holds a pin. Every card is Large
 * until smaller sizes exist. */
export function cardsToDraw(cards: LayoutCard[], drawable: ReadonlySet<string>): DrawnCard[] {
  const out: DrawnCard[] = [];
  const seen = new Set<string>();
  cards.forEach((c, i) => {
    if (c.hidden || !isKind(c.kind) || !drawable.has(c.kind)) return;
    if (c.kind === "pinned" && !c.pins?.length) return;
    if (!HOME_CARDS[c.kind].multiple) {
      if (seen.has(c.kind)) return;
      seen.add(c.kind);
    }
    out.push({ kind: c.kind, key: HOME_CARDS[c.kind].multiple ? `${c.kind}-${i}` : c.kind, size: "large", pins: c.pins ?? [] });
  });
  return out;
}

// Settings → Home (plan desktop-home-and-library.md §4.5, D6;
// agentry-desktop#661): a switch per default card, the pins, Reset Home.
// Reordering and sizes come later; the stored list already carries both.

/** The default cards, in their default order: Settings → Home's switches.
 * Go shows or hides these by kind (HomeAPI.SetShown); a Pinned card goes
 * by its pins. */
export const HOME_DEFAULTS: HomeCardKind[] = ["recent-runs", "installed", "profile", "run-defaults"];

export type CardSwitch = { kind: HomeCardKind; title: string; shown: boolean };

/** Each default card and whether Home draws it, in the default order. A
 * default the stored layout lacks is not drawn, so it reads as off; Go
 * adds it at the end when it is turned on. */
export function cardSwitches(cards: Pick<LayoutCard, "kind" | "hidden">[]): CardSwitch[] {
  return HOME_DEFAULTS.map((kind) => ({
    kind, title: HOME_CARDS[kind].title, shown: cards.some((c) => c.kind === kind && !c.hidden),
  }));
}

type StatusView = { cards: Pick<LayoutCard, "kind" | "hidden" | "pins">[]; newer: boolean; unreadable: boolean };

/** The Settings landing's line for the Home card: the default cards shown
 * and the QuickAgents pinned, or why the stored layout is not the one
 * drawn. It reads desktop.yaml only, through HomeAPI.Layout. */
export function homeStatus(v: StatusView): string {
  if (v.newer) return "Arranged by a newer Agentry Desktop; the default cards are shown.";
  if (v.unreadable) return "The arrangement cannot be read; the default cards are shown.";
  const total = HOME_DEFAULTS.length;
  const shown = cardSwitches(v.cards).filter((s) => s.shown).length;
  const pins = v.cards.filter((c) => c.kind === "pinned").reduce((n, c) => n + (c.pins?.length ?? 0), 0);
  const cards = shown === total ? `All ${total} cards shown` : shown === 0 ? "No card shown" : `${shown} of ${total} cards shown`;
  const pinned = pins === 0 ? "nothing pinned" : `${pins} QuickAgent${pins === 1 ? "" : "s"} pinned`;
  return `${cards}, ${pinned}.`;
}
