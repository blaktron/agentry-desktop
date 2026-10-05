// What Home's cards say (plan desktop-home-and-library.md §4.3;
// agentry-desktop#658). Pure and self-contained, so the node tests can
// import it.

export function installedLine(onComputer: number, withUpdate: number): string {
  if (onComputer === 0) return "Nothing installed on this computer yet.";
  const head = `${onComputer} QuickAgent${onComputer === 1 ? "" : "s"} on this computer`;
  return withUpdate > 0 ? `${head}, ${withUpdate} with an update.` : `${head}.`;
}

/** The newest installs whose directory is still on disk, each QuickAgent
 * once (its newest record), newest first, at most n. */
export function recentInstalls<T extends { namespace: string; name: string; installedAt: string; present: boolean }>(records: T[], n: number): T[] {
  const newest = new Map<string, T>();
  for (const r of records) {
    if (!r.present) continue;
    const key = `${r.namespace}/${r.name}`;
    const cur = newest.get(key);
    if (!cur || r.installedAt > cur.installedAt) newest.set(key, r);
  }
  return [...newest.values()].sort((a, b) => (a.installedAt < b.installedAt ? 1 : -1)).slice(0, n);
}

/** What a person is called: the display name, else the handle. */
export const nameOf = (user: { displayName?: string; handle: string }) => user.displayName?.trim() || user.handle;

/** Home's lead line; null signed out. */
export function greeting(user: { displayName?: string; handle: string } | null): string | null {
  return user ? `Hello, ${nameOf(user)}.` : null;
}
