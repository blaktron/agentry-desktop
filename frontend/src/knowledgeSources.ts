// What a knowledge build is made from (agentry-desktop#498; plan
// knowledge-and-drafter.md §7, D15, D16): documents, a brief, or both, and
// the Search the web choice — the Documents step's check before Next, the
// plan's warning, the choice's default and the sentences beside it. Pure,
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
 *  and a brief within its bound. */
export function sourcesProblem(documents: number, brief: string, maxBriefBytes = MAX_BRIEF_BYTES): string | null {
  const n = briefBytes(brief);
  if (documents === 0 && n === 0) return "Add a document or a folder, write a brief that says what the pack should cover, or both.";
  if (n > maxBriefBytes) return `The brief is ${n.toLocaleString("en")} bytes; a brief is at most ${maxBriefBytes >> 10} KiB. Shorten it.`;
  return null;
}

/** Search the web's default (D16): on for a new pack with no documents,
 *  off with documents and when adding to a pack, since the searches would
 *  carry what the Creator read in them to the search provider. */
export function webDefault(adding: boolean, documents: number): boolean {
  return !adding && documents === 0;
}

/** The plan's warning at the Documents step, louder (warn) when no
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

/** The Review step's Documents row. */
export function documentsRecap(documents: string[]): string {
  if (documents.length === 0) return "None: the Creator works from the brief";
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
 *  among them, as brief.md (agentry-desktop#600). */
export function documentsOf(names: string[], brief: boolean): number {
  return Math.max(0, names.length - (brief ? 1 : 0));
}

/** What continues a build that stopped short, in a sentence's words: what
 *  it was given, never documents it did not have (agentry-desktop#600). */
export function continuesWith(documents: number, brief: boolean): string {
  if (documents === 0) return "the same brief";
  return brief ? "the same documents and brief" : "the same documents";
}
