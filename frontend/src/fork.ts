// Fork in the desktop: which QuickAgent the agent page offers Fork on,
// how a fork's origin and copy are said, and how a new fork's progress
// reads while the Publisher section follows it, in the site's words.
// Pure, so the node tests can import it. Go makes the fork and says what
// happened (services_fork.go); the core decides every rule, so what is
// drawn here only saves a refused click.

/** The fields of the agent page's model that decide what it says of forks. */
export interface ForkAgent {
  visibility: string;
  quarantine?: string;
  forkedFrom?: { namespace: string; name: string; version: string; available: boolean } | null;
  forkState?: string;
  forkDetail?: string;
  forkCount?: number;
}

/** Whether the agent page offers Fork: a public QuickAgent with a
 * published version, neither quarantined nor blocked, to a signed-in
 * person. A fork is private and a retired or deleted QuickAgent is too, so
 * this also keeps Fork off them. */
export function forkable(a: ForkAgent, published: boolean, signedIn: boolean): boolean {
  return signedIn && a.visibility === "public" && !a.quarantine && published && !a.forkedFrom;
}

/** "Fork of ns/name@version", a fork's line on its page. */
export function forkOfLabel(a: ForkAgent): string | null {
  const f = a.forkedFrom;
  return f ? `Fork of ${f.namespace}/${f.name}@${f.version}` : null;
}

/** Said after the label when the origin can no longer be read, since a
 * link would only reach a missing name (the site's ForkOf). */
export const ORIGIN_GONE = "(the original is no longer available)";

/** What a fork's page says of its copy while it is not ready, in the
 * site's words: a title and its sentence. A failed copy says what to do. */
export function forkCopyNotice(a: ForkAgent): { title: string; text: string; failed: boolean } | null {
  if (!a.forkedFrom) return null;
  if (a.forkState === "copying") {
    return {
      title: "This fork is being made.",
      text: `Its history is being copied from ${a.forkedFrom.namespace}/${a.forkedFrom.name} and its first version submitted to the pipeline. This page updates by itself.`,
      failed: false,
    };
  }
  if (a.forkState === "failed") {
    return {
      title: "This fork could not be made.",
      text: `${sentenceOf(a.forkDetail || "The copy did not finish.")} Delete this fork in the Publisher section, then fork the original again.`,
      failed: true,
    };
  }
  return null;
}

/** A detail as a sentence: a capital first and a full stop last. */
function sentenceOf(s: string): string {
  const t = s.trim();
  if (!t) return t;
  const head = t[0].toUpperCase() + t.slice(1);
  return /[.!?]$/.test(head) ? head : `${head}.`;
}

/** A public QuickAgent's fork count, a number only (nothing
 * names a fork to the origin's publisher). */
export function forkCountLine(n: number | undefined): string | null {
  if (n === undefined || n <= 0) return null;
  return n === 1 ? "1 fork" : `${n.toLocaleString("en-US")} forks`;
}

/** What a fork is, said in the Fork dialog before anything is made: the
 * site's FORK_TERMS, which Go's native question repeats. */
export const FORK_TERMS: readonly string[] = [
  "A fork stays private for good. It can be changed and given new versions, but it can never be made public.",
  "Its first version is a submission through the pipeline, and it costs credits as any submission does.",
  "Hosted runs of the fork pay the original's publisher their Publisher Earnings share, unless the fork is in their own namespace or their organization's. Runs on your own computer pay nobody.",
];

/** The fork the Publisher section follows after Fork, and what the
 * registry last said of it: its copy and its versions, newest first. */
export interface ForkProgress {
  forkState?: string;
  forkDetail?: string;
  versions: { version: string; status: string }[];
}

/** The line the Publisher section shows for a new fork, and whether it is
 * still worth asking again: copying, then the first version through the
 * pipeline and any review, then published (or stopped). */
export function forkProgressLine(ref: string, p: ForkProgress): { line: string; done: boolean } {
  if (p.forkState === "failed") return { line: `The copy of ${ref} did not finish. ${sentenceOf(p.forkDetail || "")}`.trim(), done: true };
  const first = p.versions[p.versions.length - 1];
  if (p.forkState === "copying" || !first) return { line: `Copying ${ref}'s history from its origin…`, done: false };
  switch (first.status) {
    case "published":
      return { line: `${ref}'s first version, v${first.version}, is published. It is private, and yours to change.`, done: true };
    case "submitted":
    case "scanning":
    case "approved":
      return { line: `${ref}'s first version, v${first.version}, is going through the pipeline (${first.status}).`, done: false };
    case "needs_review":
      // A review can still publish or reject it, so the follow goes on.
      return { line: `${ref}'s first version, v${first.version}, is waiting for a review.`, done: false };
    default:
      return { line: `${ref}'s first version, v${first.version}, stopped: ${first.status.replace(/_/g, " ")}.`, done: true };
  }
}
