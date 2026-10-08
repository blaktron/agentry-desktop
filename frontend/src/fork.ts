// Fork in the desktop (plan forks-and-publisher.md §5, D6–D10, D12; M7,
// agentry-desktop#706): which QuickAgent the agent page offers Fork on,
// how a fork's origin and copy are said, and how a new fork's progress
// reads while the Publisher section follows it, in the site's words
// (agentry-web lib/fork.ts). Pure, so the node tests can import it. Go
// makes the fork and says what happened (services_fork.go); the core
// decides every rule, so what is drawn here only saves a refused click.

/** The fields of the agent page's model that decide what it says of forks. */
export interface ForkAgent {
  visibility: string;
  quarantine?: string;
  forkedFrom?: { namespace: string; name: string; version: string; available: boolean };
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

/** What a fork's page says of its copy while it is not ready. */
export function forkCopyLine(a: ForkAgent): string | null {
  if (!a.forkedFrom) return null;
  if (a.forkState === "copying") {
    return `Its history is being copied from ${a.forkedFrom.namespace}/${a.forkedFrom.name}, and its first version then goes through the pipeline.`;
  }
  if (a.forkState === "failed") return `The copy failed${a.forkDetail ? `: ${a.forkDetail}` : "."}`;
  return null;
}

/** A public QuickAgent's fork count, a number only (invariant 5: nothing
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
 * pipeline, then published (or stopped). */
export function forkProgressLine(ref: string, p: ForkProgress): { line: string; done: boolean } {
  if (p.forkState === "failed") return { line: `The copy of ${ref} failed${p.forkDetail ? `: ${p.forkDetail}` : "."}`, done: true };
  const first = p.versions[p.versions.length - 1];
  if (p.forkState === "copying" || !first) return { line: `Copying ${ref}'s history from its origin…`, done: false };
  switch (first.status) {
    case "published":
      return { line: `${ref}'s first version, ${first.version}, is published. It is private, and yours to change.`, done: true };
    case "submitted":
    case "scanning":
    case "approved":
      return { line: `${ref}'s first version, ${first.version}, is going through the pipeline (${first.status}).`, done: false };
    case "needs_review":
      return { line: `${ref}'s first version, ${first.version}, is waiting for a review.`, done: true };
    default:
      return { line: `${ref}'s first version, ${first.version}, stopped: ${first.status.replace(/_/g, " ")}.`, done: true };
  }
}
