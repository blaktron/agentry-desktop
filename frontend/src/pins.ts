// Pins on Home (plan desktop-home-and-library.md §4.3, D5, D11;
// agentry-desktop#660). Pure, so the node tests can import it.
//
// Go owns the pins (HomeAPI.Pin and Unpin; desktop.yaml's home key): it
// places each one, four to a Pinned card, and refuses the 17th. This
// module reads them back for the page: whether a QuickAgent is pinned,
// what the Pin button offers, which record and which run a pin shows.

export type Pin = { ref: string; registry?: string };
type CardWithPins = { kind: string; pins?: Pin[] };

// A pin's registry: the record's origin, "" for a folder install. Compared
// as Go's settings.Normalize does, and case aside, as edit.ts's
// sameRegistry does.
const norm = (u = "") => u.trim().replace(/\/+$/, "").toLowerCase();
const sameOrigin = (a = "", b = ""): boolean => norm(a) === norm(b);

/** Every pin, card by card, in the layout's order. */
export function pinsOf(cards: CardWithPins[]): Pin[] {
  return cards.filter((c) => c.kind === "pinned").flatMap((c) => c.pins ?? []);
}

export function isPinned(cards: CardWithPins[], ref: string, registry = ""): boolean {
  return pinsOf(cards).some((p) => p.ref === ref && sameOrigin(p.registry, registry));
}

/** What the Pin button offers for ref: Unpin when it is pinned, Pin while
 * Home has room, and Pin held with Go's limit in a sentence when it has
 * none (D5), rather than a refusal after the click. */
export function pinOffer(cards: CardWithPins[], maxPins: number, ref: string, registry = ""):
  { pinned: boolean; full: boolean; why?: string } {
  if (isPinned(cards, ref, registry)) return { pinned: true, full: false };
  if (maxPins > 0 && pinsOf(cards).length >= maxPins) {
    return { pinned: false, full: true, why: `Home holds ${maxPins} pinned QuickAgents, four to a Pinned card. Unpin one to pin this one.` };
  }
  return { pinned: false, full: false };
}

type Rec = { namespace: string; name: string; origin: string; local: boolean; present: boolean };

/** The record a pin shows: one on this computer from the pin's registry
 * (or from a folder), else any on this computer of the same QuickAgent (a
 * runner install's origin follows the app's default registry), else none:
 * the QuickAgent is no longer installed here. */
export function pinRecord<R extends Rec>(records: R[], pin: Pin): R | undefined {
  const here = records.filter((r) => r.present && `${r.namespace}/${r.name}` === pin.ref);
  return here.find((r) => (r.local ? !pin.registry : sameOrigin(r.origin, pin.registry))) ?? here.find((r) => !r.local);
}

type Row = { kind: string; ref?: string };

/** A pinned QuickAgent's latest run: the newest run or chat of its ref, on
 * this computer or In Agentry. Rows come newest first, as Go merges them
 * (RunAPI.RecentRuns). A built-in's job never counts. */
export function latestRunFor<R extends Row>(rows: R[], ref: string): R | undefined {
  return rows.find((r) => (r.kind === "run" || r.kind === "chat") && (r.ref ?? "").split("@")[0] === ref);
}
