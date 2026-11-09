// Home's card registry. Pure, so the node tests can import it.
//
// Go owns the layout (HomeAPI; desktop.yaml's home key): which cards, in
// what order, which hidden. This module says what each kind is, and which
// of the layout's cards the page draws. A new kind is one entry here and
// one renderer beside the page (pages/home/).

// The shortcut cards: one cell each, a figure and a title that opens the
// page where the thing is done.
export type ShortcutKind = "shortcut-run" | "shortcut-draft" | "shortcut-knowledge" | "shortcut-meetings" | "shortcut-voice" | "shortcut-agent" | "shortcut-setup";

export type HomeCardKind = "main" | "deliverables" | "recent-runs" | "installed" | "profile" | "run-defaults" | "pinned" | "scheduled" | "meetings"
  | "get-started" | "runhost" | "knowledge" | "learn" | ShortcutKind;

export type HomeCardDescriptor = {
  kind: HomeCardKind;
  // What Settings → Home calls it, and the line under its switch there.
  title: string;
  line: string;
  // One card of the kind, or several (a Pinned card holds four pins).
  multiple: boolean;
};

// The three starting Homes, one per place runs go: Go picks one from
// Settings → Runs → Where to run (HomeAPI's start).
export type HomeStart = "local" | "remote" | "cloud";

export const HOME_CARDS: Record<HomeCardKind, HomeCardDescriptor> = {
  // The agent card: a chat with a persistent agent, the one the person
  // picks while it is on, else the Assistant or Agentry Helper; Home draws
  // it while one of them is on. It starts on the first message sent to it,
  // never when Home opens.
  main: { kind: "main", title: "Agent", multiple: false,
    line: "A chat with a persistent agent: the one you pick below, else the Assistant or Agentry Helper, whichever is on. It shows while one of them is on." },
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
    line: "This month's calendar of runs scheduled on this computer, and the next three. Pin to Home on the Scheduled page adds it." },
  // Added when Meeting notes is turned on, where this copy records. Not a
  // default: Reset Home removes it.
  meetings: { kind: "meetings", title: "Meeting notes", multiple: false,
    line: "Record, the recording under way, the meetings your Assistant set up to record, and the last meetings, with Ask to summarise." },
  "get-started": { kind: "get-started", title: "Get started", multiple: false,
    line: "What your setup still needs for where your runs go, and the first things to try, each with the button that does it." },
  runhost: { kind: "runhost", title: "Runhost", multiple: false,
    line: "The runhost your runs go to: whether it answers, its model sign-ins, and its settings." },
  knowledge: { kind: "knowledge", title: "Knowledge packs", multiple: false,
    line: "Your knowledge packs on this computer and In Agentry, with Create." },
  learn: { kind: "learn", title: "Learn", multiple: false,
    line: "A few pages of the docs for someone new: QuickAgents, running one, drafting one, knowledge." },
  "shortcut-run": { kind: "shortcut-run", title: "Run a QuickAgent", multiple: false, line: "Opens your Library, or Browse while nothing is installed." },
  "shortcut-draft": { kind: "shortcut-draft", title: "Draft a QuickAgent", multiple: false, line: "Opens Build's Describe it, where the Drafter writes one from what you tell it." },
  "shortcut-knowledge": { kind: "shortcut-knowledge", title: "Create knowledge", multiple: false, line: "Starts a knowledge pack from your documents." },
  "shortcut-meetings": { kind: "shortcut-meetings", title: "Meeting notes", multiple: false, line: "Opens Meeting notes, to record a meeting and transcribe it here." },
  "shortcut-voice": { kind: "shortcut-voice", title: "Writing Voice", multiple: false, line: "Opens Writing Voice, so what your runs write sounds like you." },
  "shortcut-agent": { kind: "shortcut-agent", title: "Persistent agent", multiple: false, line: "Opens the Agents page, to turn on the Assistant." },
  "shortcut-setup": { kind: "shortcut-setup", title: "Setup", multiple: false, line: "Opens the setup you saw on the first start, to change where runs go or what they use." },
};

export const isShortcut = (k: string): k is ShortcutKind => k.startsWith("shortcut-") && Object.prototype.hasOwnProperty.call(HOME_CARDS, k);

/** The starting Homes' cards, in their order and at their sizes: Go's
 * homeStarts and homeKindSize in internal/settings/home.go, which the page
 * draws only when Home's layout cannot be read at all. The two change
 * together. */
export const HOME_STARTS: Record<HomeStart, HomeCardKind[]> = {
  local: ["get-started", "shortcut-run", "shortcut-draft", "shortcut-knowledge", "shortcut-setup",
    "main", "recent-runs", "deliverables", "learn", "knowledge", "shortcut-meetings", "shortcut-voice", "shortcut-agent"],
  remote: ["get-started", "shortcut-run", "shortcut-draft", "shortcut-knowledge", "shortcut-setup",
    "runhost", "main", "recent-runs", "deliverables", "learn", "knowledge", "shortcut-voice"],
  cloud: ["get-started", "shortcut-run", "shortcut-draft", "shortcut-knowledge", "shortcut-setup",
    "profile", "main", "recent-runs", "deliverables", "learn", "knowledge", "shortcut-voice"],
};

/** A kind's size on the grid until the person sizes it: a shortcut one
 * cell, Learn one column by two rows, every other card the grid's default. */
export function kindSize(kind: string): { cols?: number; rows?: number } {
  if (kind.startsWith("shortcut-")) return { cols: 1, rows: 1 };
  if (kind === "learn") return { cols: 1, rows: 2 };
  return {};
}

/** The starting Home's layout, as Go would answer it. */
export function startLayout(start: HomeStart): LayoutCard[] {
  return HOME_STARTS[start].map((kind) => ({ kind, ...kindSize(kind) }));
}

/** The start's name as Settings → Home says it. */
export const START_NAME: Record<HomeStart, string> = {
  local: placeInSentence("local"),
  remote: placeInSentence("remote"),
  cloud: placeInSentence("cloud"),
};

export const asStart = (s: string | undefined): HomeStart => (s === "remote" || s === "cloud" ? s : "local");

import type * as settings from "../bindings/github.com/blaktron/agentry-desktop/internal/settings/models";
import { placeInSentence } from "./firstRun";

// The one size today, the Build, Voice and Knowledge cards'.
export type HomeCardSize = "large";

export type LayoutCard = Pick<settings.HomeCard, "kind" | "hidden" | "size" | "cols" | "rows"> & { pins?: Pick<settings.HomePin, "ref" | "registry">[] };
// pins: a Pinned card's, in their rows' order; empty for every other kind.
// index: the card's place in the layout, hidden cards counted, which a move
// or a resize names. cols and rows: its stored size on the grid, absent
// until the person sizes it.
export type DrawnCard = {
  kind: HomeCardKind; key: string; size: HomeCardSize; pins: Pick<settings.HomePin, "ref" | "registry">[];
  index: number; cols?: number; rows?: number;
};

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
    // A Pinned card's key is its first pin, so it keeps its key, and its
    // cell, when a move changes its place; its place too, in a layout that
    // names that pin twice.
    let key: string = c.kind;
    if (HOME_CARDS[c.kind].multiple) {
      key = `${c.kind}-${c.pins?.[0] ? `${c.pins[0].registry ?? ""} ${c.pins[0].ref}` : i}`;
      if (seen.has(key)) key += `-${i}`;
      seen.add(key);
    }
    out.push({ kind: c.kind, key, size: "large", pins: c.pins ?? [], index: i, cols: c.cols || undefined, rows: c.rows || undefined });
  });
  return out;
}

// Settings → Home: a switch per card, the pins, Reset Home. The order and
// the sizes are set on Home itself, by dragging.

/** Settings → Home's switches: every card but a Pinned one, which goes by
 * its pins; the cards first, then the shortcuts. Go shows or hides these by
 * kind (HomeAPI.SetShown). Go's list is settings.homeSwitches: the two
 * change together. */
export const HOME_SWITCHES: HomeCardKind[] = [
  "get-started", "main", "deliverables", "recent-runs", "installed", "profile", "run-defaults",
  "runhost", "knowledge", "learn", "scheduled", "meetings",
  "shortcut-run", "shortcut-draft", "shortcut-knowledge", "shortcut-meetings", "shortcut-voice", "shortcut-agent", "shortcut-setup",
];

export type CardSwitch = { kind: HomeCardKind; title: string; line: string; shown: boolean; shortcut: boolean };

/** Whether Home draws the Scheduled card: one is in the layout and not
 * hidden. The Scheduled page's options menu offers Unpin then, else Pin. */
export function scheduledOnHome(cards: Pick<LayoutCard, "kind" | "hidden">[]): boolean {
  return cards.some((c) => c.kind === "scheduled" && !c.hidden);
}

/** Each switch's card and whether Home draws it, the cards first and the
 * shortcuts after; the Meeting notes card and its shortcut only where this
 * copy records meetings. A card the stored layout lacks is not drawn, so it
 * reads as off; Go adds it at the end when it is turned on. */
export function cardSwitches(cards: Pick<LayoutCard, "kind" | "hidden">[], meetings = false): CardSwitch[] {
  return HOME_SWITCHES.filter((kind) => (kind !== "meetings" && kind !== "shortcut-meetings") || meetings).map((kind) => ({
    kind, title: HOME_CARDS[kind].title, line: HOME_CARDS[kind].line, shown: cards.some((c) => c.kind === kind && !c.hidden), shortcut: isShortcut(kind),
  }));
}

type StatusView = { cards: Pick<LayoutCard, "kind" | "hidden" | "pins">[]; newer: boolean; unreadable: boolean };

/** The Settings landing's line for the Home card: the cards the layout
 * holds and how many of them are shown, and the QuickAgents pinned, or why
 * the stored layout is not the one drawn. It reads desktop.yaml only,
 * through HomeAPI.Layout. */
export function homeStatus(v: StatusView): string {
  if (v.newer) return "Arranged by a newer Agentry Desktop; the starting cards are shown.";
  if (v.unreadable) return "The arrangement cannot be read; the starting cards are shown.";
  const held = v.cards.filter((c) => c.kind !== "pinned" && Object.prototype.hasOwnProperty.call(HOME_CARDS, c.kind));
  const total = new Set(held.map((c) => c.kind)).size;
  const shown = new Set(held.filter((c) => !c.hidden).map((c) => c.kind)).size;
  // pins.ts's pinsOf, counted here: node's tests load these modules
  // without a bundler, which resolves no extensionless import.
  const pins = v.cards.filter((c) => c.kind === "pinned").reduce((n, c) => n + (c.pins?.length ?? 0), 0);
  const cards = shown === 0 ? "No card shown" : shown === total ? `All ${total} cards shown` : `${shown} of ${total} cards shown`;
  const pinned = pins === 0 ? "nothing pinned" : `${pins} QuickAgent${pins === 1 ? "" : "s"} pinned`;
  return `${cards}, ${pinned}.`;
}
