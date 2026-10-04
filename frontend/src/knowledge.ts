// The Knowledge panel's pure parts (plan knowledge.md §4.8;
// agentry-desktop#273): the words for a pack's state, its size, and the
// name rule the Go side enforces again (agentry-cli's knowledge.ValidName).
// The copy names what holds a pack's key and what read it, and never calls
// a pack "safe", "secure" or "private" (plan §3).
import { knowledge } from "../wailsjs/go/models";
import type { JobProgress } from "./jobProgress";
import { fmtBytes } from "./ui";

/** A pack name: 1-64 lowercase letters, digits, '.', '-' or '_', starting
 *  with a letter or a digit — one directory under /knowledge in a run. */
export const PACK_NAME = /^[a-z0-9][a-z0-9._-]{0,63}$/;

export function nameProblem(name: string): string | null {
  if (!name) return "Give the pack a name.";
  if (!PACK_NAME.test(name)) return "Use 1 to 64 lowercase letters, digits, '.', '-' or '_', starting with a letter or a digit.";
  return null;
}

export function pagesOf(n: number): string {
  return n === 1 ? "1 page" : `${n} pages`;
}

/** The badge's tone for a pack's scan state; its words are Go's (p.scan),
 *  the receipt's words for a pack's scan record. */
export function scanTone(p: knowledge.Pack): string {
  switch (p.scanState) {
    case "scanned":
      return "ok";
    case "failed":
      return "bad";
    case "not-scanned":
    case "unconfirmed":
      return "warn";
    default:
      return "";
  }
}

/** Whether a run on this computer can attach the pack: it is kept here. A
 *  pack kept only in Agentry is downloaded first. */
export function attachable(p: knowledge.Pack): boolean {
  return p.local;
}

/** Whether a run In Agentry can attach the pack (agentry-desktop#275):
 *  Agentry keeps it and has scanned it. A pack kept only here is uploaded
 *  first; one being scanned there waits for the scan. */
export function attachableInAgentry(p: knowledge.Pack): boolean {
  return p.inAgentry && p.agentryReady;
}

/** Why a pack cannot be attached to a run In Agentry, or what attaching it
 *  means, in a sentence for the step's row. */
export function inAgentryLine(p: knowledge.Pack): string {
  if (p.shared && !p.inAgentry) return `${p.org} does not list this pack in Agentry now, so a run there cannot attach it`;
  if (!p.inAgentry) return "kept on this computer only: upload it to Agentry to attach it to a run there";
  if (!p.agentryReady) {
    return p.scanState === "scanning"
      ? "attach it once Agentry's scan of it ends"
      : p.scanState === "failed"
        ? `Agentry's scan could not read it: upload it again${p.local ? "" : " from a computer that keeps it"} to attach it`
        : `Agentry holds no pages of it yet: upload it${p.local ? "" : " from a computer that keeps it"} to attach it`;
  }
  if (p.local && p.changed) return "the run gets the copy Agentry keeps, which is older than this computer's: upload it again first to attach this one";
  return "kept in Agentry; the run reads Agentry's copy";
}

/** The Knowledge step's check before Next: at most the registry's number
 *  of packs for a run In Agentry. */
export function tooManyPacks(count: number, max: number | undefined): string | null {
  if (!max || count <= max) return null;
  const over = count - max;
  return `A run In Agentry attaches at most ${max} knowledge ${max === 1 ? "pack" : "packs"}. Untick ${over === 1 ? "one" : over} to go on.`;
}

/** The sentence a sign-in adds when it moved packs off this computer's key
 *  (plan D20); empty when it moved none. */
export function rewrapLine(res: { knowledgeRewrapped?: string[]; knowledgeNote?: string }): string {
  const moved = res.knowledgeRewrapped ?? [];
  let line = "";
  if (moved.length === 1) line = ` The knowledge pack ${moved[0]} is now encrypted to a key this registry keeps for your account, not to this computer's key.`;
  else if (moved.length > 1) line = ` The knowledge packs ${moved.join(", ")} are now encrypted to a key this registry keeps for your account, not to this computer's key.`;
  if (res.knowledgeNote) line += ` ${res.knowledgeNote}`;
  return line;
}

/** Before a build (plan D19): what runs, where, against what, and that
 *  nothing it reads or writes is scanned. */
export function unscannedWarning(model: string): string {
  return (
    `The Knowledge Creator runs on this computer, against the model your runner policy names (${model}). ` +
    "It costs no credits. Nothing it reads or writes is scanned: not your documents, and not the pack it makes, " +
    "which is recorded as “not scanned” until you upload it to Agentry, where it is scanned."
  );
}


/** What a finished build cost, from the registry's settlement. */
export function buildCreditsLine(v: { holdCredits: number; settledCredits: number; settled: boolean; scanCredits: number }): string {
  if (!v.settled) return `${v.holdCredits} credits are held for this build.`;
  if (v.settledCredits === 0) return `${v.holdCredits} credits were held and all of them were released: this build cost nothing.`;
  const scan = v.scanCredits > 0 ? ` (${v.scanCredits === 1 ? "1 of them" : `${v.scanCredits} of them`} for the pack's scan)` : "";
  return `${v.holdCredits} credits were held; ${v.settledCredits} settled${scan} and the rest was released.`;
}

/** Before a build In Agentry: what goes where, what runs it, and what
 *  comes back. */
export function inAgentryNote(registry: string, documents = 1, brief = false): string {
  const what = documents === 0 ? "The brief goes" : brief ? "The documents and the brief go" : "The documents go";
  return (
    `${what} to ${registry}, where the Knowledge Creator runs against the platform's model, for credits. ` +
    "The pack is kept in Agentry, encrypted under a key of its own, and scanned there; once the scan ends, it is downloaded and kept on this computer too."
  );
}

/** How far a build on this computer has got, as Go's knowledge.Progress
 *  emits it on knowledge:build-progress (agentry-desktop#352): the report
 *  every job carries, and the pages written so far. */
export type BuildProgress = JobProgress & { pages: number };

/** The warning the review step gives before a build on this computer: it
 *  is long work, and how long it may run. A timeout of 0 is Run defaults'
 *  no-time-limit switch for the built-in agents (agentry-desktop#606). */
export function buildTimeWarning(timeoutSeconds: number, maxTurns: number): string {
  const minutes = Math.round(timeoutSeconds / 60);
  const limit =
    timeoutSeconds > 0
      ? `This build may run for up to ${minutes} minutes${maxTurns > 0 ? ` and ${maxTurns} turns` : ""}, `
      : `This build has no time limit (Settings → Run defaults)${maxTurns > 0 ? ` and may run for up to ${maxTurns} turns` : ""}, `;
  return (
    "Creating knowledge is a big job. A few short documents take a few minutes; a large set, a model running on this " +
    "computer, or a frontier model at high reasoning can take more than 20 minutes. " +
    limit +
    "and shows its progress as it goes. If it stops at a limit, the pack keeps the pages it wrote that pass its checks, and Add documents continues it."
  );
}

/** Agentry keeps packs of at most 10 MB (Go's knowledge.MaxBytes), for the
 *  pack list, which has no answer from Go that carries it; a pack made on
 *  this computer may hold up to 1 GB (agentry-cli#429, agentry-desktop#354). */
export const AGENTRY_MAX_BYTES = 10 * 1024 * 1024;

/** The Documents step's warning past Agentry's cap. */
export function largeDocumentsWarning(bytes: number, agentryMax: number): string {
  return (
    `The documents come to ${fmtBytes(bytes)}. A build that size can take a very long time, ` +
    `and the pack will stay on this computer: Agentry keeps packs of at most ${fmtCap(agentryMax)}, so it cannot be uploaded, shared or built In Agentry.`
  );
}

/** A cap as a person reads it: "10 MB", "1 GB" (Go's knowledge.FormatCap). */
export function fmtCap(n: number): string {
  return n >= 1024 * 1024 * 1024 && n % (1024 * 1024 * 1024) === 0 ? `${n / (1024 * 1024 * 1024)} GB` : `${Math.round(n / (1024 * 1024))} MB`;
}
