// Pins on Home. Pure, so the node tests can import it.
//
// Go owns the pins (HomeAPI.Pin and Unpin; desktop.yaml's home key): it
// places each one, four to a Pinned card, and refuses the 17th. This
// module reads them back for the page: whether a QuickAgent is pinned,
// what the Pin button offers, which record and which run a pin shows.

export type Pin = { ref: string; registry?: string };
type CardWithPins = { kind: string; pins?: Pin[] };

// A pin's registry: the record's origin, "" for a folder install. Compared
// exactly as Go's settings.Normalize compares it (no case folding, unlike
// edit.ts's sameRegistry), so the page never shows as pinned what Go's
// Unpin would not find.
const norm = (u = "") => u.trim().replace(/\/+$/, "");
const sameOrigin = (a = "", b = ""): boolean => norm(a) === norm(b);

/** A pin's registry as Settings → Home names it: the host and port only, never
 * a userinfo part an origin may carry, and nothing for a value that is not a
 * URL. */
export function registryHost(url: string): string {
  try {
    return new URL(norm(url)).host;
  } catch {
    return "";
  }
}

/** Every pin, card by card, in the layout's order. */
export function pinsOf(cards: CardWithPins[]): Pin[] {
  return cards.filter((c) => c.kind === "pinned").flatMap((c) => c.pins ?? []);
}

export function isPinned(cards: CardWithPins[], ref: string, registry = ""): boolean {
  return pinsOf(cards).some((p) => p.ref === ref && sameOrigin(p.registry, registry));
}

type Limits = { maxPins: number; pinsPerCard: number };

/** The sentence for a Home that holds every pin it can, with Go's
 * limits; null while it has room. */
export function homeFullSentence(cards: CardWithPins[], { maxPins, pinsPerCard }: Limits): string | null {
  return maxPins > 0 && pinsOf(cards).length >= maxPins
    ? `Home holds ${maxPins} pinned QuickAgents, ${pinsPerCard} to a Pinned card. Unpin one to pin another.`
    : null;
}

/** What the Pin button offers for ref: Unpin when it is pinned, Pin while
 * Home has room, and Pin held with the sentence when it has none,
 * rather than a refusal after the click. */
export function pinOffer(cards: CardWithPins[], limits: Limits, ref: string, registry = ""):
  { pinned: boolean; full: boolean; why?: string } {
  if (isPinned(cards, ref, registry)) return { pinned: true, full: false };
  const why = homeFullSentence(cards, limits);
  return why ? { pinned: false, full: true, why } : { pinned: false, full: false };
}

type Rec = { namespace: string; name: string; origin: string; local: boolean; present: boolean };

/** The record a pin shows. A folder pin shows only a folder install of
 * its QuickAgent. A registry pin shows the install from its registry, else
 * another install from a registry (a runner install's origin follows the
 * app's default registry). None: the QuickAgent is no longer installed here
 * in that way. */
export function pinRecord<R extends Rec>(records: R[], pin: Pin): R | undefined {
  const here = records.filter((r) => r.present && `${r.namespace}/${r.name}` === pin.ref);
  if (!pin.registry) return here.find((r) => r.local);
  return here.find((r) => !r.local && sameOrigin(r.origin, pin.registry)) ?? here.find((r) => !r.local);
}

type Row = { kind: string; ref?: string; registry?: string };

/** A pinned QuickAgent's latest run: the newest run or chat of its ref, on
 * this computer or In Agentry, from the pin's registry when it names one. A
 * folder install's run records the registry the app had then, so a folder
 * pin matches by ref alone. Rows come newest first, as Go merges them
 * (RunAPI.RecentRuns). A built-in's job never counts. */
export function latestRunFor<R extends Row>(rows: R[], pin: Pin): R | undefined {
  return rows.find((r) => (r.kind === "run" || r.kind === "chat") && (r.ref ?? "").split("@")[0] === pin.ref
    && (!pin.registry || !r.registry || sameOrigin(r.registry, pin.registry)));
}
