// What a knowledge build is made from: documents, git repositories and
// files at web addresses, a brief, or a mix, and the Search the web
// choice — the Documents step's check before Next, the warning, the
// choice's default and the sentences beside it. Pure,
// so the rules are tested apart from the page; Go checks the brief and the
// documents again (build.CheckBrief), and the registry once more.

/** A brief's most bytes (the CLI's build.MaxBriefBytes and the registry's
 *  hostapi.MaxBuildBriefBytes): 8 KiB. */
export const MAX_BRIEF_BYTES = 8192;

/** A brief's size as Go counts it: UTF-8 bytes, once trimmed. */
export function briefBytes(brief: string): number {
  return new TextEncoder().encode(brief.trim()).length;
}

/** The Documents step's check before Next: documents, a brief, or both,
 *  and a brief within its bound. A repository or an address counts as a
 *  document. */
export function sourcesProblem(documents: number, brief: string, maxBriefBytes = MAX_BRIEF_BYTES): string | null {
  const n = briefBytes(brief);
  if (documents === 0 && n === 0) return "Add a document or a folder, give a repository or a web address, write a brief that says what the pack should cover, or a mix.";
  if (n > maxBriefBytes) return `The brief is ${n.toLocaleString("en")} bytes; a brief is at most ${maxBriefBytes >> 10} KiB. Shorten it.`;
  return null;
}

/** The addresses in a field that holds one a line: each trimmed, blank
 *  lines dropped. */
export function addressLines(text: string): string[] {
  return text.split(/\r?\n/).map((l) => l.trim()).filter((l) => l !== "");
}

/** One line of Go's check of a build's sources (knowledge.SourceLine). */
export interface SourceLine {
  address: string;
  kind: string;
  problem?: string;
  host?: string;
  saved?: boolean;
}

/** The first thing wrong with a build's sources as the step says it, or
 *  null: the list's own problem (the count), then the first line's. */
export function sourceListProblem(check: { problem?: string; lines?: SourceLine[] } | null): string | null {
  if (!check) return null;
  if (check.problem) return `The repositories and addresses: ${check.problem}.`;
  const bad = (check.lines ?? []).find((l) => l.problem);
  return bad ? `${bad.address}: ${bad.problem}.` : null;
}

/** What a repository or a file that needs an access token uses for it, in
 *  each place, beside the fields: the key saved for its host, which goes to
 *  that host alone. */
export function sourceKeyLine(lines: SourceLine[], agentry: boolean): string {
  const saved = [...new Set(lines.filter((l) => l.saved && l.host).map((l) => l.host!))];
  const here = saved.length > 0
    ? `On this computer a build uses the key you saved for ${saved.join(", ")} in a run setup, for that host alone.`
    : "On this computer a repository or a file that needs an access token uses the key you saved for its host in a run setup.";
  return agentry ? `${here} In Agentry it uses the key saved in Agentry for that host, under Settings → Encrypted storage on the site.` : here;
}

/** The Review step's row for repositories or addresses. */
export function addressesRecap(list: string[], what: "repository" | "address"): string {
  if (list.length === 1) return list[0];
  return `${list.length} ${what === "repository" ? "repositories" : "web addresses"}`;
}

/** Search the web's default: on for a new pack with no documents,
 *  off with documents and when adding to a pack, since the searches would
 *  carry what the Creator read in them to the search provider. */
export function webDefault(adding: boolean, documents: number): boolean {
  return !adding && documents === 0;
}

/** The warning at the Documents step, louder (warn) when no
 *  document is chosen. Its words follow the web: without it the Creator
 *  writes from the brief and its own knowledge. */
export function sourcesWarning(documents: number, web: boolean): { tone: "warn" | "info"; text: string } {
  const without = web
    ? "Without them the Creator researches the web and its own knowledge, which can be wrong or out of date."
    : "Without them the Creator writes from the brief and its own knowledge, which can be wrong or out of date.";
  return { tone: documents === 0 ? "warn" : "info", text: `Specific documents make the best knowledge. ${without}` };
}

/** Whether a build on this computer can search: Go's CreatorWebView. */
export interface LocalWeb {
  ready: boolean;
  why?: string;
  provider?: string;
}

/** What Search the web means on this computer, beside the choice: the
 *  person's own keys, or why a build here runs without the web. */
export function localWebLine(w: LocalWeb | null): string {
  if (!w) return "On this computer the web takes your own AI Review key and a search key.";
  if (w.ready) {
    return `On this computer the Creator searches with ${w.provider || "your search provider"} and your key, and AI Review reads every result first, with your AI Review key.`;
  }
  return `On this computer a build runs without the web: ${w.why}.`;
}

/** What Search the web means In Agentry, beside the choice: the registry's
 *  note (searching can raise the credits a build uses), or that it does not
 *  search for a build. */
export function agentryWebLine(web: boolean, note: string | undefined): string {
  if (!web) return "In Agentry a build here does not search the web.";
  return `In Agentry the registry searches and AI Review reads every result first. ${note ?? ""}`.trim();
}

/** The Review step's Search the web row. */
export function webRecap(asked: boolean, where: "local" | "agentry", local: LocalWeb | null, agentryWeb: boolean): string {
  if (!asked) return "No";
  if (where === "agentry") return agentryWeb ? "Yes, through the registry" : "No: In Agentry does not search the web for a build here";
  if (local && !local.ready) return `No: ${local.why}`;
  return "Yes, with your own keys";
}

/** Whether the build will search, as it will start: the choice, where the
 *  place it runs can search. */
export function searches(asked: boolean, where: "local" | "agentry", local: LocalWeb | null, agentryWeb: boolean): boolean {
  if (!asked) return false;
  return where === "agentry" ? agentryWeb : !!local?.ready;
}

/** The Review step's Documents row; sources says the build is given
 *  repositories or addresses beside them. */
export function documentsRecap(documents: string[], sources = false): string {
  if (documents.length === 0) return sources ? "None" : "None: the Creator works from the brief";
  return documents.length === 1 ? documents[0] : `${documents.length} files and folders`;
}

/** What a build reads, in a sentence's words: "2 documents and your
 *  brief", "your brief", "3 documents". */
export function readsLine(documents: number, brief: boolean): string {
  const docs = documents === 1 ? "1 document" : `${documents} documents`;
  if (documents === 0) return brief ? "your brief" : "no documents";
  return brief ? `${docs} and your brief` : docs;
}

/** How many documents a finished build read: its result lists the brief
 *  among them, as brief.md. */
export function documentsRead(names: string[], brief: boolean): number {
  return Math.max(0, names.length - (brief ? 1 : 0));
}

/** What continues a build that stopped short, in a sentence's words: what
 *  it was given, never documents it did not have. */
export function continuesWith(documents: number, brief: boolean): string {
  if (!brief) return "the same documents";
  return documents > 0 ? "the same documents and brief" : "the same brief";
}

/** What a pack's size counts beside its pages, in a sentence's words: "its
 *  documents", "its brief", "its documents and brief". */
export function keptWith(documents: number, brief: boolean): string {
  if (!brief) return "its documents";
  return documents > 0 ? "its documents and brief" : "its brief";
}

/** What a build In Agentry's Continue starts from again, in a sentence's
 *  words: "this brief and these documents", "this brief", "these
 *  documents". */
export function continuedFrom(documents: number, brief: boolean): string {
  if (!brief) return "these documents";
  return documents > 0 ? "this brief and these documents" : "this brief";
}
