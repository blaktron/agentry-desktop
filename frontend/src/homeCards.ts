// Home's card registry. Pure, so the node tests can import it.
//
// Go owns the layout (HomeAPI; desktop.yaml's home key): which cards, in
// what order, which hidden. This module says what each kind is, and which
// of the layout's cards the page draws. A new kind is one entry here and
// one renderer beside the page (pages/home/).

export type HomeCardKind = "main" | "deliverables" | "recent-runs" | "installed" | "profile" | "run-defaults" | "pinned" | "scheduled";

export type HomeCardDescriptor = {
  kind: HomeCardKind;
  // What Settings → Home calls it, and the line under its switch there.
  title: string;
  line: string;
  // One card of the kind, or several (a Pinned card holds four pins).
  multiple: boolean;
};

export const HOME_CARDS: Record<HomeCardKind, HomeCardDescriptor> = {
  // Main: a chat with Agentry's own agent. It starts on the first message
  // sent to it, never when Home opens.
  main: { kind: "main", title: "Agentry Helper", multiple: false,
    line: "A chat with Agentry Helper, Agentry's own agent: it finds, installs, runs and schedules QuickAgents for you, asking you first in Agentry." },
  deliverables: { kind: "deliverables", title: "Deliverables", multiple: false,
    line: "The newest files your runs were for, with Open, View and Show in folder, and a button for your Deliverables folder." },
  "recent-runs": { kind: "recent-runs", title: "Recent runs", multiple: false,
    line: "The last five runs, chats and built-in jobs, on this computer and In Agentry." },
  installed: { kind: "installed", title: "Installed QuickAgents", multiple: false,
    line: "How many QuickAgents are on this computer, how many have an update, and the newest five." },
  profile: { kind: "profile", title: "Profile", multiple: false, line: "Your name, plan and credits on the registry in use, or Sign in." },
  "run-defaults": { kind: "run-defaults", title: "Run defaults", multiple: false,
    line: "Where runs go, their limits, the output folder, and Unsafe Mode while it is on." },
  pinned: { kind: "pinned", title: "Pinned", multiple: true, line: "Up to four pinned QuickAgents, each with its latest run and Run." },
  // Pinned from the Scheduled page's options menu. Not a default:
  // Reset Home removes it.
  scheduled: { kind: "scheduled", title: "Scheduled", multiple: false,
    line: "This month's calendar of runs scheduled on this computer, and the next three. Pin to Home on the Schedule page adds it." },
};

import type { settings } from "../wailsjs/go/models";

// The one size today, the Build, Voice and Knowledge cards'.
export type HomeCardSize = "large";

export type LayoutCard = Pick<settings.HomeCard, "kind" | "hidden" | "size"> & { pins?: Pick<settings.HomePin, "ref" | "registry">[] };
// pins: a Pinned card's, in their rows' order; empty for every other kind.
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

// Settings → Home: a switch per default card, the pins, Reset Home.
// The stored list carries an order and sizes; Settings offers neither yet.

/** The default cards, in their default order: Settings → Home's switches.
 * Go shows or hides these by kind (HomeAPI.SetShown); a Pinned card goes
 * by its pins. Go's list is settings.homeDefaults: the two change
 * together. */
export const HOME_DEFAULTS: HomeCardKind[] = ["main", "deliverables", "recent-runs", "installed", "profile", "run-defaults"];

/** Settings → Home's switches: the default cards, then the Scheduled card,
 * which Go shows or hides by kind like them. */
export const HOME_SWITCHES: HomeCardKind[] = [...HOME_DEFAULTS, "scheduled"];

export type CardSwitch = { kind: HomeCardKind; title: string; line: string; shown: boolean };

/** Whether Home draws the Scheduled card: one is in the layout and not
 * hidden. The Scheduled page's options menu offers Unpin then, else Pin. */
export function scheduledOnHome(cards: Pick<LayoutCard, "kind" | "hidden">[]): boolean {
  return cards.some((c) => c.kind === "scheduled" && !c.hidden);
}

/** Each switch's card and whether Home draws it, in the default order,
 * the Scheduled card last. A card the stored layout lacks is not drawn, so
 * it reads as off; Go adds it at the end when it is turned on. */
export function cardSwitches(cards: Pick<LayoutCard, "kind" | "hidden">[]): CardSwitch[] {
  return HOME_SWITCHES.map((kind) => ({
    kind, title: HOME_CARDS[kind].title, line: HOME_CARDS[kind].line, shown: cards.some((c) => c.kind === kind && !c.hidden),
  }));
}

type StatusView = { cards: Pick<LayoutCard, "kind" | "hidden" | "pins">[]; newer: boolean; unreadable: boolean };

/** The Settings landing's line for the Home card: the cards shown (the
 * defaults, and the Scheduled card while the layout holds it) and the
 * QuickAgents pinned, or why the stored layout is not the one drawn. It
 * reads desktop.yaml only, through HomeAPI.Layout. */
export function homeStatus(v: StatusView): string {
  if (v.newer) return "Arranged by a newer Agentry Desktop; the default cards are shown.";
  if (v.unreadable) return "The arrangement cannot be read; the default cards are shown.";
  const total = HOME_DEFAULTS.length + (v.cards.some((c) => c.kind === "scheduled") ? 1 : 0);
  const shown = cardSwitches(v.cards).filter((s) => s.shown).length;
  // pins.ts's pinsOf, counted here: node's tests load these modules
  // without a bundler, which resolves no extensionless import.
  const pins = v.cards.filter((c) => c.kind === "pinned").reduce((n, c) => n + (c.pins?.length ?? 0), 0);
  const cards = shown === total ? `All ${total} cards shown` : shown === 0 ? "No card shown" : `${shown} of ${total} cards shown`;
  const pinned = pins === 0 ? "nothing pinned" : `${pins} QuickAgent${pins === 1 ? "" : "s"} pinned`;
  return `${cards}, ${pinned}.`;
}
