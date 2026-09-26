// The Knowledge panel's pure parts (plan knowledge.md §4.8;
// agentry-desktop#273): the words for a pack's state, its size, and the
// name rule the Go side enforces again (agentry-cli's knowledge.ValidName).
// The copy names what holds a pack's key and what read it, and never calls
// a pack "safe", "secure" or "private" (plan §3).
import { knowledge } from "../wailsjs/go/models";

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
