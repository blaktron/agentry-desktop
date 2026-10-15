// An advisory on a declared MCP server, in words: its own severity and
// CVSS score, and the registry gate's rating where it differs, which is
// what a run weighs. A High advisory the gate rates medium reads as both,
// never as "high" alone on a server that runs.

export type AdvisoryLike = { severity: string; rating?: string; cvssScore?: number };

export function advisoryWords(a: AdvisoryLike): string {
  let w = a.severity || "unrated";
  if (typeof a.cvssScore === "number") w += ` (CVSS ${a.cvssScore.toFixed(1)})`;
  if (a.rating && a.rating !== a.severity) w += `, which the gate rates ${a.rating}`;
  return w;
}
