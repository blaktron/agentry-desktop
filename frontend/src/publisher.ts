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
  forkedFrom?: { namespace: string; name: string; version: string };
  forkState?: string;
  lastSubmittedAt?: string;
}

export type VisibilityFilter = "" | "public" | "private";
export type StateFilter = "" | "active" | "retired";
export type ForksFilter = "" | "only" | "none";
export type ListSort = "newest" | "installs" | "submitted" | "name";

/** The sorts, as Browse offers them, read for a namespace. */
export const LIST_SORTS: { id: ListSort; label: string }[] = [
  { id: "newest", label: "newest" },
  { id: "installs", label: "most installed (30 days)" },
  { id: "submitted", label: "last submission" },
  { id: "name", label: "name" },
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

/** "Fork of ns/name@version", the line a fork's row carries, with its
 * copy's state while it is not ready, as the site's row says it. */
export function forkOfLabel(a: PublisherRow): string | null {
  const f = a.forkedFrom;
  if (!f) return null;
  const state = a.forkState === "copying" ? " · being copied" : a.forkState === "failed" ? " · the copy failed" : "";
  return `Fork of ${f.namespace}/${f.name}@${f.version}${state}`;
}

/** Why a fork offers no Make public (D9), in the site's words
 * (lib/fork.ts). */
export const FORK_STAYS_PRIVATE = "A fork stays private for good, so it cannot be made public.";

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

/** What deleting does, said before the name is typed: the site's
 * DeleteWords. A QuickAgent with no version is deleted outright and frees
 * its name; one with versions leaves a tombstone and holds it (plan §4.1).
 * The list knows which from its last submission, as the site's does. */
export function deleteWords(ns: string, name: string, hasVersions: boolean): string[] {
  const ref = `${ns}/${name}`;
  if (!hasVersions) return [`${ref} has no version, so it is deleted outright with its repository, and the name is free to use again. This cannot be undone.`];
  return [
    `Every version of ${ref} is yanked, and it is gone for everyone, you included. Anyone who installed it keeps their copy, but can no longer update it.`,
    `This cannot be undone, and the name stays held: no QuickAgent in ${ns} can be called ${name} again. To put it away and keep the choice, Retire it instead.`,
  ];
}

/** Whether a Delete's typed name matches, as Go checks it again. */
export function deleteNameMatches(typed: string, name: string): boolean {
  return typed.trim() === name;
}
