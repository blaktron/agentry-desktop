// The Publisher section's model (plan forks-and-publisher.md §4.2, D12; M6,
// agentry-desktop#705): what a row of the list says about a QuickAgent and
// which owner actions it offers, in the site's words (agentry-web
// lib/publisher.ts). Pure, so the node tests can import it. Go makes the
// calls and says what they did or why the registry refused them
// (services_publisher.go).

/** A row as the page reads it: main.PublisherAgent's fields that decide
 * what the row says. */
export interface PublisherRow {
  namespace: string;
  name: string;
  visibility: string;
  retiredAt?: string;
  quarantineState: string;
  forkedFrom?: { namespace: string; name: string; version: string; available: boolean };
}

export type VisibilityFilter = "" | "public" | "private";
export type StateFilter = "" | "active" | "retired";
export type ForksFilter = "" | "only" | "none";
export type ListSort = "newest" | "installs" | "submitted" | "name";

/** The sorts, as Browse offers them, read for a namespace. */
export const LIST_SORTS: { id: ListSort; label: string }[] = [
  { id: "newest", label: "Newest" },
  { id: "installs", label: "Most installed (30 days)" },
  { id: "submitted", label: "Last submission" },
  { id: "name", label: "Name" },
];

/** Visibility is not one of the core's filters: it is applied to the list
 * the registry returned. */
export function visibleIn<T extends { visibility: string }>(rows: readonly T[], v: VisibilityFilter): T[] {
  return v === "" ? [...rows] : rows.filter((r) => r.visibility === v);
}

export interface RowBadge {
  label: string;
  tone: "kind" | "warn" | "bad"; // the stylesheet's .badge classes
}

/** The badges a row carries beside its name, in the order the plan names
 * them (§4.2): Private, Fork, Retired, then a quarantine by its state. */
export function rowBadges(a: PublisherRow): RowBadge[] {
  const out: RowBadge[] = [];
  if (a.visibility === "private") out.push({ label: "Private", tone: "kind" });
  if (a.forkedFrom) out.push({ label: "Fork", tone: "kind" });
  if (a.retiredAt) out.push({ label: "Retired", tone: "warn" });
  if (a.quarantineState === "quarantined") out.push({ label: "Quarantined", tone: "bad" });
  if (a.quarantineState === "blocked") out.push({ label: "Blocked", tone: "bad" });
  return out;
}

/** "Fork of ns/name@version", the line a fork's row carries. */
export function forkOfLabel(a: PublisherRow): string | null {
  const f = a.forkedFrom;
  return f ? `Fork of ${f.namespace}/${f.name}@${f.version}` : null;
}

/** Why a fork offers no Make public (D9), said where the button would be. */
export const FORK_STAYS_PRIVATE = "A fork stays private for good, so it is never made public.";

/** The owner actions a row offers (D5). A retired QuickAgent offers Restore
 * in place of Make public and Retire, and no Edit, since it takes no new
 * version. A fork never offers Make public (D9). Delete is every row's. */
export interface RowActions {
  edit: boolean;
  visibility: "private" | "public" | null;
  retire: "retire" | "restore";
  forkStaysPrivate: boolean;
}

export function rowActions(a: PublisherRow): RowActions {
  const fork = !!a.forkedFrom;
  if (a.retiredAt) return { edit: false, visibility: null, retire: "restore", forkStaysPrivate: fork };
  if (fork) return { edit: true, visibility: a.visibility === "public" ? "private" : null, retire: "retire", forkStaysPrivate: true };
  return { edit: true, visibility: a.visibility === "public" ? "private" : "public", retire: "retire", forkStaysPrivate: false };
}

/** The label of the visibility button. */
export function visibilityLabel(to: "private" | "public"): string {
  return to === "private" ? "Make private" : "Make public";
}

/** The counts under the namespace's heading. */
export function listCounts(rows: readonly PublisherRow[]): { total: number; private: number; retired: number } {
  return {
    total: rows.length,
    private: rows.filter((a) => a.visibility === "private" && !a.retiredAt).length,
    retired: rows.filter((a) => !!a.retiredAt).length,
  };
}

/** The counts in words: "3 QuickAgents · 1 private · 1 retired". */
export function countsLine(c: { total: number; private: number; retired: number }): string {
  const parts = [`${c.total} QuickAgent${c.total === 1 ? "" : "s"}`];
  if (c.private) parts.push(`${c.private} private`);
  if (c.retired) parts.push(`${c.retired} retired`);
  return parts.join(" · ");
}

/** What a namespace whose owner actions the person may not take says in
 * place of them: an organization's members may publish to it (Edit), and
 * only its admins change who sees a QuickAgent, retire or delete it. */
export function notAdminLine(ns: string): string {
  return `Only admins of ${ns} can make its QuickAgents public or private, retire, restore or delete them.`;
}

/** What the section says to a sign-in that cannot take the owner actions:
 * one made before the grant carried agent:manage (agentry#993). */
export const SIGN_IN_AGAIN =
  "This sign-in was made before Agentry Desktop could retire, restore, delete or change who sees a QuickAgent. Sign out and sign in again under Settings to use those actions here.";

/** Whether a Delete's typed name matches, as Go checks it again. */
export function deleteNameMatches(typed: string, name: string): boolean {
  return typed.trim() === name;
}
