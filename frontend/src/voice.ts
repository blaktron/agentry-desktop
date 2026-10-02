// The Writing Voice's words and choices (plan writing-voice.md §5, §8, D6,
// D11, D12; agentry-desktop#510). Pure, so the Voice panel, Settings and the
// run setup share them and the tests read them without a webview.
//
// A person keeps a voice in two places: this computer's (free, never
// reviewed) and their account's (reviewed by AI Review before it is live).
// A run on this computer writes in this computer's; a run In Agentry in the
// account's live one.

/** The most a VOICE.md holds (plan D3). */
export const MAX_VOICE_BYTES = 32 * 1024;

/** The notice after a voice is first made or first goes live (plan §8). */
export const VOICE_ON = "Your voice is on for every run. You can turn it off in Settings.";

/** The run setup's checkbox and its line (plan §5, D11). */
export const WRITE_IN_MY_VOICE = "Write in my voice";
export const WRITE_IN_MY_VOICE_HINT =
  "The run is given your Writing Voice and told to write anything meant for a person in it. It is told the voice changes how it writes, not what it does.";

/** What the run setup needs to know of the two voices. */
export interface VoiceFacts {
  local?: { exists: boolean; alwaysAttach: boolean; problem?: string } | null;
  account?: { live?: unknown | null; alwaysAttach: boolean } | null;
}

/** Whether the run setup shows Write in my voice, and ticked or not: the
 *  voice the run would use must exist (this computer's, opening; the
 *  account's live one In Agentry), and its own switch ticks the box. */
export function voiceChoice(v: VoiceFacts | null | undefined, inAgentry: boolean): { offered: boolean; ticked: boolean } {
  if (!v) return { offered: false, ticked: false };
  if (inAgentry) {
    const a = v.account;
    return a?.live ? { offered: true, ticked: a.alwaysAttach } : { offered: false, ticked: false };
  }
  const l = v.local;
  return l?.exists && !l.problem ? { offered: true, ticked: l.alwaysAttach } : { offered: false, ticked: false };
}

/** How a voice on this computer came to be, as Go's voice.Source.Phrase
 *  says it; the account's versions use the registry's words. */
const ACCOUNT_SOURCE: Record<string, string> = {
  build: "written by the Voice Writer",
  "ai-edit": "an AI Edit",
  edit: "edited by hand",
  upload: "submitted from a computer",
  restore: "restored",
};
export function accountSource(s: string): string {
  return ACCOUNT_SOURCE[s] ?? s;
}

/** Where an account version stands in its review. */
export function reviewWord(review: string): string {
  switch (review) {
    case "reviewing":
      return "In review";
    case "flagged":
      return "Flagged";
    case "passed":
      return "Live";
  }
  return review;
}

/** The voice question set's questions, in words (the core's pkg/judge
 *  VoiceQuestions; the site's lib/voice-view says the same). */
const QUESTION: Record<string, string> = {
  beyond_style: "It asks for more than a way of writing: a tool, a file, the network, a secret or an action.",
  override: "It tries to override the run's own instructions or its QuickAgent's.",
  inserted_content: "It tells the model to add content of its own to what it writes.",
  task_change: "It changes what a run does, not only how it writes.",
};

/** One finding as the panel lists it: the question in words when AI Review
 *  scored one, else the rule's title, with its line. */
export function findingText(f: { title: string; question?: string; line?: number }): string {
  const what = (f.question && QUESTION[f.question]) || f.title;
  return f.line ? `${what} (line ${f.line})` : what;
}

/** The editor's check before Save, the format's own rule (plan D3). */
export function voiceTextProblem(text: string): string | null {
  if (text.trim() === "") return "Write the voice before you save it.";
  const bytes = new TextEncoder().encode(text).length;
  if (bytes > MAX_VOICE_BYTES) return `A voice is at most 32 KiB, and this text is ${Math.ceil(bytes / 1024)} KiB.`;
  return null;
}

/** What one Voice Writer run In Agentry costs this payer, from the
 *  registry's numbers. */
export function hostedVoiceCost(a: { credits: number; holdCredits: number; limited: boolean; left: number }): string {
  if (a.holdCredits === 0) return "It costs no credits: the organization that pays for your runs is not metered.";
  const cost = `It costs ${a.credits} credits, AI Review included. If no voice comes out, the credits are refunded.`;
  if (!a.limited) return cost;
  return `${cost} On Standard you can run it in Agentry once a billing cycle, and you have ${a.left} left this cycle.`;
}
