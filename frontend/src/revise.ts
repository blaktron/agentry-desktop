// Edit with AI's form: two texts, Additional instructions and Problems with this
// draft, and where it drafts. In Agentry is for a payer above Standard,
// as the registry says (caller.revise); on this computer is for anyone the
// registry says may publish to the QuickAgent, with their own AI Review key
// and docker. Pure, so the rules are tested on their own; Go holds the
// texts to the same rule again (hostedrun.CheckRevision).

/** Each text's bound, the registry's and the CLI's. */
export const MAX_REVISION_TEXT_BYTES = 4000;

export type ReviseWhere = "agentry" | "here";

/** The bytes a text is sent as: line breaks as LF, trimmed. */
export function revisionBytes(text: string): number {
  return new TextEncoder().encode(text.replace(/\r\n/g, "\n").trim()).length;
}

/** Why the texts cannot be sent yet, or null when they can. */
export function textsProblem(instructions: string, problems: string): string | null {
  const a = revisionBytes(instructions), b = revisionBytes(problems);
  if (a === 0 && b === 0) return "Say what should change, or what is wrong with this version.";
  if (a > MAX_REVISION_TEXT_BYTES) return `Additional instructions are ${a} bytes; at most ${MAX_REVISION_TEXT_BYTES}.`;
  if (b > MAX_REVISION_TEXT_BYTES) return `Problems with this draft are ${b} bytes; at most ${MAX_REVISION_TEXT_BYTES}.`;
  return null;
}

/** What the dialog knows of each way. */
export interface ReviseWays {
  /** In Agentry: offered at all, and available to this payer now. */
  agentryOffered: boolean;
  agentryAvailable: boolean;
  /** On this computer: the person's AI Review key, and docker. */
  keyReady: boolean;
  dockerOk: boolean;
}

/** In Agentry is chosen first where it can be, as Describe it chooses. */
export function firstWhere(w: ReviseWays): ReviseWhere {
  return w.agentryOffered && w.agentryAvailable ? "agentry" : "here";
}

/** Whether the revision can start: the texts, and the way chosen ready. */
export function canStart(where: ReviseWhere, w: ReviseWays, instructions: string, problems: string): boolean {
  if (textsProblem(instructions, problems) !== null) return false;
  return where === "agentry" ? w.agentryOffered && w.agentryAvailable : w.keyReady && w.dockerOk;
}
