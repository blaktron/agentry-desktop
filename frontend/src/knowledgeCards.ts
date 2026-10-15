// The Knowledge page's two places: which packs each page lists, and the
// cards' and rows' words. Pure, so the page, the harness and the tests
// share it.
//
// Local Knowledge lists every pack with a copy on this computer; Cloud
// Knowledge every pack the account keeps in Agentry, and each
// organization's shared packs in a group of their own. A pack kept in
// both places is on both pages, marked so. The words name what holds
// a key and what read a pack, and never call a pack "safe", "secure" or
// "private" (the copy check holds the line). Sizes are written by the
// caller's formatter (ui's fmtStorage), so this module imports nothing at
// run time and node's test runner reads it as it is.
import type { knowledge } from "../wailsjs/go/models";

/** Writes a size in bytes, as ui.tsx's fmtStorage does. */
export type SizeWords = (bytes: number) => string;

export const KNOWLEDGE_TAGLINE =
  "Knowledge packs a run can search: kept on this computer, in your Agentry account, or both. The agentry knowledge CLI keeps the same packs here.";
export const LOCAL_CARD_BLURB = "The knowledge kept on this computer. Create it free with your own model, and your runs here can search it.";
export const CLOUD_CARD_BLURB =
  "The knowledge kept in your Agentry account. AI Review scans every pack it keeps, and your runs In Agentry can search it.";

// The allocation as the registry states it (AccountAPI's
// KnowledgeAllocation): enabled false means nothing may be added or grown;
// max 0 is no cap.
export interface Allocation {
  enabled: boolean;
  bytes: number;
  max: number;
}

type Pack = Pick<knowledge.Pack, "name" | "local" | "inAgentry" | "shared" | "org" | "bytes" | "updated" | "agentryUpdated" | "changed">;

/** A day as the cards say it: "4 October 2026", the UTC day, as the Voice
 *  cards do. */
export function knowledgeDay(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
}

/** The packs the Local Knowledge page lists: every one with a copy here,
 *  an organization's shared copies included. */
export function localPacks<P extends Pack>(packs: P[]): P[] {
  return packs.filter((p) => p.local);
}

/** The account's own packs in Agentry, for the Cloud Knowledge page. */
export function cloudPacks<P extends Pack>(packs: P[]): P[] {
  return packs.filter((p) => p.inAgentry && !p.shared);
}

/** Each organization's shared packs in Agentry, by organization, in the
 *  order the list has them. */
export function sharedGroups<P extends Pack>(packs: P[]): { org: string; packs: P[] }[] {
  const out: { org: string; packs: P[] }[] = [];
  for (const p of packs) {
    if (!p.shared || !p.inAgentry || !p.org) continue;
    let g = out.find((x) => x.org === p.org);
    if (!g) out.push((g = { org: p.org, packs: [] }));
    g.packs.push(p);
  }
  return out;
}

const count = (n: number) => (n === 1 ? "1 pack" : `${n} packs`);

// newest is the latest of the dates, "" with none.
function newest(dates: (string | undefined)[]): string {
  return dates.filter((d): d is string => !!d).sort().pop() ?? "";
}

/** The Local Knowledge card's line: how many packs are kept here, what they
 *  come to, and when one last changed. */
export function localStatus(packs: Pack[], size: SizeWords): string {
  const here = localPacks(packs);
  if (here.length === 0) return "No knowledge on this computer yet.";
  const parts = [count(here.length), size(here.reduce((n, p) => n + p.bytes, 0))];
  const day = knowledgeDay(newest(here.map((p) => p.updated)));
  if (day) parts.push(`updated ${day}`);
  return parts.join(" · ") + ".";
}

/** allocationPhrase is the account's storage of its allocation, as Settings
 *  › Profile says it. */
export function allocationPhrase(k: Allocation, size: SizeWords): string {
  if (!k.enabled) return k.bytes > 0 ? `none may be added; ${size(k.bytes)} held` : "none may be added";
  return `${size(k.bytes)} of ${k.max === 0 ? "no cap" : size(k.max)}`;
}

/** The Cloud Knowledge card's line: signed out, the account's packs could
 *  not be listed, or how many it keeps, their storage of the allocation
 *  (where the registry states one) and how many its organizations share. */
export function cloudStatus(list: { signedIn: boolean; agentry?: string } | null, packs: Pack[], size: SizeWords, alloc?: Allocation | null): string {
  if (!list) return "";
  if (!list.signedIn) return "Sign in to keep knowledge in your account.";
  if (list.agentry) return list.agentry;
  const own = cloudPacks(packs);
  const shared = sharedGroups(packs);
  if (own.length === 0 && shared.length === 0) return "No knowledge in your account yet.";
  const parts = [own.length === 0 ? "No packs of your own" : count(own.length)];
  if (alloc) parts.push(allocationPhrase(alloc, size));
  for (const g of shared) parts.push(`${g.packs.length} shared by ${g.org}`);
  return parts.join(" · ") + ".";
}

/** The badges a row carries on one place's page about the other place:
 *  kept there too, and changed since it was copied. */
export function otherPlace(p: Pack, page: "local" | "cloud"): { label: string; tone: string; title: string }[] {
  if (p.shared) return [];
  const out: { label: string; tone: string; title: string }[] = [];
  if (page === "local" && p.inAgentry) out.push({ label: "also in your account", tone: "", title: "Agentry keeps a copy too: it is on the Cloud Knowledge page" });
  if (page === "cloud" && p.local) out.push({ label: "also on this computer", tone: "", title: "a copy is kept here too: it is on the Local Knowledge page" });
  if (p.local && p.inAgentry && p.changed) {
    out.push({ label: "changed since it was copied", tone: "warn", title: "the copy here changed after the copy in Agentry was made: Copy to Cloud sends this one" });
  }
  return out;
}
