// The Library's search. Pure, so the node tests can import it.
//
// Typing filters the QuickAgents installed on this computer; the button
// beside the field always leads to Browse, carrying what was typed.

/** The records whose namespace/name holds the search, ignoring case and the
 * spaces at its edges; every record when the search is empty. */
export function filterInstalled<T extends { namespace: string; name: string }>(records: T[], search: string): T[] {
  const q = search.trim().toLowerCase();
  if (!q) return records;
  return records.filter((r) => `${r.namespace}/${r.name}`.toLowerCase().includes(q));
}

/** The query Browse is opened with: what was typed, trimmed, or none. */
export function registryQuery(search: string): string | undefined {
  return search.trim() || undefined;
}

/** The button that leads to Browse. */
export function registrySearchLabel(search: string): string {
  const q = registryQuery(search);
  return q ? `Search the registry for “${q}”` : "Find more QuickAgents";
}
