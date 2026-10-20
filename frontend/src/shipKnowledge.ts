// Shipping a knowledge pack with a QuickAgent, in the Build tab and in
// Describe it. A shipped pack goes into the release's archive, unencrypted,
// so everyone who can download the release can read it; Go asks the
// publisher's confirmation of each one in a native dialog at Submit, and
// this module holds the rules the page draws: which packs may be ticked in
// a QuickAgent's namespace (the same rules as internal/build's
// ShipOption.Reason), the five-pack limit, and the notes beside a pack.
// Pure and free of the Wails bindings, so it is tested on its own.

/** The most packs one release ships (the registry's rule). */
export const MAX_SHIPS = 5;
/** The most bytes one shipped pack holds. */
export const MAX_SHIP_BYTES = 10 << 20;

/** A pack the Knowledge section can offer: BuildAPI.ShipOptions. */
export interface ShipOption {
  name: string;
  /** "own" for the person's pack, "org" for an organization's shared one. */
  kind: string;
  org?: string;
  id?: string;
  agentryId?: string;
  bytes: number;
  pages: number;
  ready: boolean;
  orgAdmin: boolean;
}

/** The part of the form's model that says what it ships. */
export interface ShipState {
  knowledgeShips?: string[];
  ships?: ShipSourceLike[];
  /** The include list that came with the workspace, empty for the form's own. */
  publishInclude?: string[];
}

/** What a tick or an untick changes in the form. */
export type ShipChange = Required<ShipState>;

/** The glob that packs the shipped packs. */
export const SHIPS_GLOB = "knowledge/**";

/** A stored pack the form ships (build.ShipSource). */
export interface ShipSourceLike {
  name: string;
  kind: string;
  org?: string;
  id?: string;
  agentryId?: string;
  bytes: number;
  pages: number;
}

/** A pack's size in the binary units the Knowledge pages count in. */
export function shipSize(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB"];
  let v = n / 1024;
  let i = 0;
  while (Number(v.toFixed(1)) >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const s = v.toFixed(1);
  return `${s.endsWith(".0") ? s.slice(0, -2) : s} ${units[i]}`;
}

/** Why the pack cannot ship with a QuickAgent in namespace, in a sentence
 *  for its row, or null when it can. An organization's pack ships only from
 *  its administrator into a QuickAgent in its own namespace; any pack only
 *  once AI Review has read it, and within one pack's cap. */
export function shipReason(o: ShipOption, namespace: string): string | null {
  const ns = namespace.trim();
  if (o.kind === "org" && !o.orgAdmin) return `only an administrator of ${o.org ?? "its organization"} can ship its pack`;
  if (o.kind === "org" && ns !== o.org) return `ships only into a QuickAgent in the ${o.org} namespace, and this one is in ${ns || "no namespace yet"}`;
  if (!o.ready) return "wait until AI Review has read it, then it can ship";
  if (o.bytes > MAX_SHIP_BYTES) return `it holds ${shipSize(o.bytes)}, and a release ships at most ${shipSize(MAX_SHIP_BYTES)} of one pack`;
  return null;
}

/** The key an option is ticked by: an organization's pack of the same name
 *  as a person's own is another pack. */
export function optionKey(o: { name: string; kind: string; org?: string }): string {
  return o.kind === "org" ? `org:${o.org ?? ""}/${o.name}` : `own:${o.name}`;
}

/** Whether the option is what the form ships under its name. */
export function isTicked(m: ShipState, o: ShipOption): boolean {
  const names = m.knowledgeShips ?? [];
  if (!names.includes(o.name)) return false;
  const src = (m.ships ?? []).find((s) => s.name === o.name);
  return !src || optionKey(src) === optionKey(o);
}

/** The first thing that stops a tick, or null: five at most, and no second
 *  pack under a name the form already ships (a pack's name is its folder). */
export function tickProblem(m: ShipState, o: ShipOption, namespace: string): string | null {
  const names = m.knowledgeShips ?? [];
  const why = shipReason(o, namespace);
  if (why) return why;
  if (names.includes(o.name) && !isTicked(m, o)) return `the form already ships a pack named ${o.name}`;
  if (!names.includes(o.name) && names.length >= MAX_SHIPS) return `a release ships at most ${MAX_SHIPS} knowledge packs`;
  return null;
}

/** The form's ships with the option ticked. A refused tick changes nothing. */
export function tick(m: ShipState, o: ShipOption, namespace: string): ShipChange {
  const names = m.knowledgeShips ?? [];
  const ships = m.ships ?? [];
  const include = (m.publishInclude ?? []).filter((g) => g.trim() !== "");
  if (tickProblem(m, o, namespace) !== null) return { knowledgeShips: names, ships, publishInclude: m.publishInclude ?? [] };
  const src: ShipSourceLike = { name: o.name, kind: o.kind, org: o.org, id: o.id, agentryId: o.agentryId, bytes: o.bytes, pages: o.pages };
  return {
    knowledgeShips: names.includes(o.name) ? names : [...names, o.name],
    ships: [...ships.filter((s) => s.name !== o.name), src],
    // An include list that came with the workspace has to pack the pack too;
    // the form's own list does already.
    publishInclude: include.length > 0 && !include.includes(SHIPS_GLOB) ? [...include, SHIPS_GLOB] : (m.publishInclude ?? []),
  };
}

/** The form's ships without the pack. */
export function untick(m: ShipState, name: string): ShipChange {
  const names = (m.knowledgeShips ?? []).filter((n) => n !== name);
  return {
    knowledgeShips: names,
    ships: (m.ships ?? []).filter((s) => s.name !== name),
    // With no pack left, the glob that packed them goes: files left under
    // knowledge/ belong to no shipped pack.
    publishInclude: names.length === 0 ? (m.publishInclude ?? []).filter((g) => g !== SHIPS_GLOB) : (m.publishInclude ?? []),
  };
}

/** The packs the form ships that no stored pack stands behind: the files
 *  were already in the folder it was loaded from. */
export function folderOwn(m: ShipState): string[] {
  const sources = new Set((m.ships ?? []).map((s) => s.name));
  return (m.knowledgeShips ?? []).filter((n) => !sources.has(n));
}

/** The line under the ticks: how many of the five, and their size. */
export function shipTotalLine(m: ShipState): string {
  const names = m.knowledgeShips ?? [];
  if (names.length === 0) return `None ticked. A release ships up to ${MAX_SHIPS} packs, ${shipSize(MAX_SHIP_BYTES)} each.`;
  const bytes = (m.ships ?? []).reduce((n, s) => n + s.bytes, 0);
  const size = (m.ships ?? []).length > 0 ? `, ${shipSize(bytes)} in all` : "";
  return `Ticked ${names.length} of up to ${MAX_SHIPS}${size}.`;
}

/** What the section says of shipping: the same facts the confirmation will
 *  ask, so nothing in Submit's dialog is a surprise. */
export const SHIP_EXPLAINER =
  "A pack you ship is put in the release, unencrypted. Everyone who can download the release can read it, and it stays in that version for good. " +
  "Each pack is read again from where you keep it every time you save, and Submit asks you to confirm each one.";

/** The note beside a pack ticked after a draft was made: the Drafter wrote
 *  the instructions without knowing it would ship. */
export function afterDraftNote(m: ShipState, name: string, draftShipped: string[]): string | null {
  if (!(m.knowledgeShips ?? []).includes(name) || draftShipped.includes(name)) return null;
  return "The draft was written without this pack, so its instructions may not mention it: read them before you submit.";
}

/** Describe it's choice for a pack the draft reads: read it, or read it and
 *  ship it with the QuickAgent. */
export type ReadOrShip = "read" | "ship";

/** The picks of a draft: the ships are among the packs it reads. */
export function shipsOf(picked: string[], shipped: string[]): string[] {
  return picked.filter((n) => shipped.includes(n));
}

/** Why a pack the draft reads cannot also ship with it, or null. A draft is
 *  made for the person's own namespace, so only their own packs ship from
 *  here; an organization's pack is ticked in the Knowledge section once the
 *  draft is in the form. A draft made In Agentry ships only where the
 *  registry takes the choice (shipEnabled). */
export function readAndShipReason(o: ShipOption | undefined, where: "here" | "agentry", shipEnabled = false): string | null {
  if (where === "agentry" && !shipEnabled) return "this registry does not ship a pack with a draft: tick it in the Knowledge section after the draft";
  if (!o) return "only a pack you keep can ship";
  if (o.kind === "org") return `${o.org ?? "its organization"}'s pack ships into a QuickAgent in its namespace: tick it in the Knowledge section after the draft`;
  return shipReason(o, "");
}

/** The line under Describe it's picks when some are shipped. */
export function shipPickLine(shipped: string[]): string | null {
  if (shipped.length === 0) return null;
  return `${shipped.join(", ")} will ship with the QuickAgent: ${shipped.length === 1 ? "it goes" : "they go"} into the release, readable by everyone who can download it, and you confirm ${shipped.length === 1 ? "it" : "each"} again when you submit.`;
}

/** How many ships a draft may have: the registry's cap, counted with the
 *  packs it already reads. */
export function canAddShip(shipped: string[]): boolean {
  return shipped.length < MAX_SHIPS;
}
