// The Writing Voice's words and choices. Pure, so the Voice panel, Settings and
// the run setup share them and the tests read them without a webview.
//
// A person keeps a voice in two places: this computer's (free, never
// reviewed) and their account's (reviewed by AI Review before it is live).
// A run on this computer writes in this computer's; a run In Agentry in the
// account's live one.

/** "1 credit", "20 credits". voice.ts imports no values, so node's test
 *  runner loads it alone (subscription.ts has the same phrase). */
function credits(n: number): string {
  return n === 1 ? "1 credit" : `${n} credits`;
}

/** A day as the Voice cards say it: "4 October 2026", the UTC day the
 *  site's Voice page says too. */
export function voiceDay(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
}

/** The most a VOICE.md holds. */
export const MAX_VOICE_BYTES = 32 * 1024;

/** The notice after a voice is first made or first goes live. */
export const VOICE_ON = "Your voice is on for every run. You can turn it off in Settings.";

/** The run setup's checkbox and its line. */
export const WRITE_IN_MY_VOICE = "Write in my voice";
export const WRITE_IN_MY_VOICE_HINT =
  "The run is given your Writing Voice and told to write anything meant for a person in it. It is told the voice changes how it writes, not what it does.";

/** What the run setup needs to know of the two voices. */
export interface VoiceFacts {
  local?: { exists: boolean; alwaysAttach: boolean; problem?: string } | null;
  account?: { live?: unknown | null; alwaysAttach: boolean } | null;
  /** The registry lets a run In Agentry write in the account's voice. */
  attachEnabled?: boolean;
}

/** Whether the run setup shows Write in my voice, and ticked or not: the
 *  voice the run would use must exist (this computer's, opening; the
 *  account's live one In Agentry), and its own switch ticks the box. */
export function voiceChoice(v: VoiceFacts | null | undefined, inAgentry: boolean): { offered: boolean; ticked: boolean } {
  if (!v) return { offered: false, ticked: false };
  if (inAgentry) {
    const a = v.account;
    return a?.live && v.attachEnabled ? { offered: true, ticked: a.alwaysAttach } : { offered: false, ticked: false };
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
  upload: "submitted from your computer",
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

/** A text's size in bytes as UTF-8, what the 32 KiB cap counts. */
export function voiceBytes(text: string): number {
  return new TextEncoder().encode(text).length;
}

/** The editor's check before Save, the format's own rule. */
export function voiceTextProblem(text: string): string | null {
  if (text.trim() === "") return "Write the voice before you save it.";
  const bytes = voiceBytes(text);
  if (bytes > MAX_VOICE_BYTES) return `A voice is at most 32 KiB, and this text is ${Math.ceil(bytes / 1024)} KiB.`;
  return null;
}

/** Save's decision: nothing
 *  when the voice is as it was opened, the format's rule, else the text. */
export type VoiceSave = { kind: "unchanged" } | { kind: "problem"; message: string } | { kind: "send"; text: string };

/** Save's decision for the plain textarea, the editor's stand-in while it
 *  loads and for a voice it cannot hold; the WYSIWYG editor's is
 *  voiceMarkdown.ts's voiceSave, which also judges by what it would write. */
export function plainVoiceSave(opened: string, text: string): VoiceSave {
  if (text === opened) return { kind: "unchanged" };
  const problem = voiceTextProblem(text);
  return problem ? { kind: "problem", message: problem } : { kind: "send", text };
}

/** What one Voice Writer run In Agentry costs this payer, from the
 *  registry's numbers: the Cloud Voice page's Edit with AI says it. */
export function hostedVoiceCost(a: { credits: number; holdCredits: number; limited: boolean; max: number; left: number }): string {
  if (a.holdCredits === 0) return "It costs no credits: the organization that pays for your runs is not metered.";
  const cost = `It costs ${a.credits} credits, AI Review included. If no voice comes out, the credits are refunded.`;
  if (!a.limited) return cost;
  return `${cost} On Standard you can run it in Agentry ${timesACycle(a.max)}, and you have ${a.left} left this cycle.`;
}

/** How often Standard may run the Voice Writer in a cycle, in the
 *  registry's number (caller.voice.max), as the site says it. */
export function timesACycle(max: number): string {
  return max === 1 ? "once a billing cycle" : max === 2 ? "twice a billing cycle" : `${max} times a billing cycle`;
}

/** The line beside the account's Save and Submit: AI Review reads it
 *  first, for the registry's credit where it charges one. */
export function reviewCostLine(n: number, live: boolean): string {
  const cost = n > 0 ? `, for ${credits(n)}` : "";
  return live
    ? `Save sends it to AI Review${cost}. Your live voice stays live until the new version passes.`
    : `Save sends it to AI Review${cost}. It goes live when it passes.`;
}

// The Voice page's two cards and the Local Voice page.

/** The start page's line under its title, in the style of BUILD_TAGLINE. */
export const VOICE_TAGLINE =
  "Your Writing Voice is the instructions a run follows to write the way you do. Keep one on this computer, one in your Agentry account, or both.";

/** The cards' words. */
export const LOCAL_CARD_BLURB =
  "The voice kept on this computer. Edit it by hand or with AI for free, and move it to or from your Agentry account.";
export const CLOUD_CARD_BLURB =
  "The voice kept in your Agentry account. AI Review checks every version before it goes live, and your runs In Agentry use it.";

/** The Local Voice page's switch: the same setting as Settings → Run
 *  defaults. */
export const LOCAL_SWITCH_LABEL = "Use this voice for every run on this computer";

interface LocalFacts {
  exists: boolean;
  alwaysAttach: boolean;
  updated?: string;
  problem?: string;
}

/** The Local Voice card's status. */
export function localStatus(l: LocalFacts | null | undefined): string {
  if (!l?.exists) return "No voice on this computer yet.";
  const parts = ["Voice set"];
  if (l.updated) parts.push(`updated ${voiceDay(l.updated)}`);
  parts.push(l.alwaysAttach ? "used by default" : "not used by default");
  if (l.problem) parts.push("it did not open");
  return parts.join(" · ") + ".";
}

interface VersionFacts {
  review: string;
  digest?: string;
  createdAt: string;
  reviewedAt?: string;
}

interface AccountFacts {
  exists: boolean;
  alwaysAttach: boolean;
  live?: VersionFacts | null;
  pending?: VersionFacts | null;
}

interface ViewFacts {
  signedIn: boolean;
  accountOffered: boolean;
  accountError?: string;
  account?: AccountFacts | null;
}

/** The Cloud Voice card's status, in the site's words. */
export function cloudStatus(v: ViewFacts): string {
  if (!v.signedIn) return "Sign in to keep a voice in your account.";
  if (!v.accountOffered) return "This registry keeps no Writing Voices.";
  if (v.accountError) return "Your account's voice could not be read.";
  const a = v.account;
  if (!a?.live && !a?.pending) return "No voice in your account yet.";
  const parts: string[] = [];
  if (a.live) parts.push(`Live since ${voiceDay(a.live.reviewedAt ?? a.live.createdAt)}`);
  else parts.push("Not live yet");
  if (a.pending?.review === "flagged") parts.push("a new version was flagged by AI Review");
  else if (a.pending) parts.push("a new version is in review");
  if (a.live && a.alwaysAttach) parts.push("used by default");
  return parts.join(" · ") + ".";
}

interface SyncFacts {
  downloadSame: boolean;
  uploadSame: boolean;
  localAfterLive: boolean;
  accountAfterLocal: boolean;
  inReview: boolean;
}

/** A Local Voice page button: whether it can be pressed, and the line
 *  under it. */
export interface SyncButton {
  disabled: boolean;
  line: string;
}

export const ALREADY_THE_SAME = "Already the same";

function accountUnreachable(v: ViewFacts): string | null {
  if (!v.signedIn) return "Sign in to move your voice to or from your account.";
  if (!v.accountOffered) return "This registry keeps no Writing Voices.";
  if (v.accountError) return "Your account's voice could not be read.";
  return null;
}

/** Download Cloud version: the account's live version kept
 *  here; it asks first in a native dialog when it would replace a voice
 *  saved here after the live version, or a newer version is in review. */
export function downloadButton(v: ViewFacts & { local?: LocalFacts | null; sync?: SyncFacts | null }): SyncButton {
  const off = accountUnreachable(v);
  if (off) return { disabled: true, line: off };
  if (!v.account?.live) return { disabled: true, line: "Your account has no live voice yet." };
  if (v.sync?.downloadSame) return { disabled: true, line: `${ALREADY_THE_SAME}.` };
  const parts = [v.local?.exists ? "Replaces the voice on this computer with your account's live one." : "Keeps your account's live voice on this computer."];
  if (v.sync?.localAfterLive) parts.push("The voice here was saved after it, so you are asked first.");
  if (v.sync?.inReview) parts.push("A newer version is in review and is not live yet.");
  return { disabled: false, line: parts.join(" ") };
}

/** Upload to Cloud: this computer's voice sent to AI Review
 *  for the registry's credit; it asks first when the account's newest
 *  version was set after this computer's voice was saved. */
export function uploadButton(v: ViewFacts & { local?: LocalFacts | null; sync?: SyncFacts | null; reviewCredits: number }): SyncButton {
  if (!v.local?.exists) return { disabled: true, line: "This computer keeps no voice to upload yet." };
  if (v.local.problem) return { disabled: true, line: "This computer's voice does not open, so it cannot be uploaded." };
  const off = accountUnreachable(v);
  if (off) return { disabled: true, line: off };
  if (v.sync?.uploadSame) return { disabled: true, line: `${ALREADY_THE_SAME}.` };
  const cost = v.reviewCredits > 0 ? ` for ${credits(v.reviewCredits)}` : "";
  const parts = [`Sends this computer's voice to AI Review${cost}. It goes live in your account when it passes.`];
  if (v.sync?.accountAfterLocal) parts.push("Your account's version was set after this one, so you are asked first.");
  return { disabled: false, line: parts.join(" ") };
}

/** The most an Edit with AI note or a brief holds (the registry's 4 KiB). */
export const MAX_VOICE_NOTE_BYTES = 4 * 1024;

/** The Edit with AI dialog's check before Start: with a voice
 *  here it is an AI Edit and says what should change, samples optional;
 *  with none it is a Create, samples required, the text its brief. */
export function editWithAIProblem(hasVoice: boolean, text: string, samples: number): string | null {
  if (hasVoice && text.trim() === "") return "Say what should change in your voice.";
  if (!hasVoice && samples === 0) return "Add samples of your writing: the Voice Writer writes your voice from them.";
  if (new TextEncoder().encode(text.trim()).length > MAX_VOICE_NOTE_BYTES) return "What you write here is at most 4 KiB.";
  return null;
}

// The Cloud Voice page: the site's Voice settings page in the desktop, in
// its words.

/** The Cloud Voice page's switch: the same setting as Settings → Profile. */
export const CLOUD_SWITCH_LABEL = "Use this voice for every run In Agentry";

/** A voice not live yet (the site's NOT_LIVE_SENTENCE). */
export const NOT_LIVE = "Your voice is not live yet, so no run writes in it until AI Review passes it.";

/** Where the registry runs no Voice Writer In Agentry (the site's
 *  NO_WRITER_SENTENCE, from the desktop). */
export const NO_HOSTED_WRITER =
  "This registry does not run the Voice Writer In Agentry. The Local Voice page runs it on this computer for no credits, and Upload to Cloud sends what it writes here.";

/** The line on a version waiting in the account (the site's
 *  pendingSentence): the page follows a version in review on its own. */
export function pendingLine(p: { review: string; verdict?: string }, live: boolean): string {
  if (p.review === "flagged") {
    return live
      ? "AI Review flagged this version, so your live voice stays live. Edit it and save it again, or discard it."
      : "AI Review flagged this version, so it is not live. Edit it and save it again, or discard it.";
  }
  if (p.verdict === "incomplete") {
    return "AI Review did not finish reading this version, so it is still in review. Agentry tries again on its own.";
  }
  return live
    ? "AI Review is reading this version. It replaces your live voice when it passes, and this page updates itself."
    : "AI Review is reading this version. It goes live when it passes, and this page updates itself.";
}

/** An instant as the site's version lines say it: "2026-10-01 22:20 UTC". */
export function voiceStamp(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())} UTC`;
}

/** One account version's line: how it was made, when, when AI Review read
 *  it, and its size (the site's VersionLine). */
export function versionLine(v: { source: string; createdAt: string; reviewedAt?: string | null; bytes: number }): string {
  const how = accountSource(v.source);
  const reviewed = v.reviewedAt ? `, reviewed ${voiceStamp(v.reviewedAt)}` : "";
  return `${how[0].toUpperCase()}${how.slice(1)}, ${voiceStamp(v.createdAt)}${reviewed}. ${v.bytes.toLocaleString("en-GB")} bytes.`;
}

/** What Restore previous does (the site's restoreSentence): a previous
 *  version reviewed under the question set AI Review asks now trades
 *  places with the live one at once; one reviewed under another goes to
 *  AI Review again, for the registry's credit. */
export function restoreLine(
  a: { questionSet?: string; previous?: { questionSet?: string } | null; pending?: unknown | null },
  reviewCredits: number,
): string {
  const set = a.previous?.questionSet ?? "";
  if (set !== "" && set === a.questionSet) {
    return "Restore makes the previous version live again at once, and your live one becomes the previous version.";
  }
  const line = reviewCredits > 0
    ? `AI Review has changed since the previous version passed, so Restore sends it to AI Review again, for ${credits(reviewCredits)}.`
    : "AI Review has changed since the previous version passed, so Restore sends it to AI Review again.";
  // Sent to review, it becomes the pending version, in place of one
  // already waiting (the core's restore queues it as any new version).
  return a.pending ? `${line} It takes the place of the version waiting above.` : line;
}

/** Why a Voice Writer run In Agentry that ended wrote no voice, in the
 *  registry's words: its error, the run's note, or its outcome. */
export function hostedFailure(v: { error?: string; note?: string; outcomeLine?: string; statusLine: string }): string {
  return v.error || v.note || v.outcomeLine || v.statusLine;
}

/** The Voice Writer's job behind Edit with AI In Agentry, as Go's
 *  accountJob decides it: an Edit of the account's newest version, live or
 *  pending, and a Create where it keeps neither. */
export function accountJob(a: { live?: unknown | null; pending?: unknown | null } | null | undefined): "edit" | "create" {
  return a?.live || a?.pending ? "edit" : "create";
}

/** The hosted dialog's cost: the price, Standard's cycle, and a balance
 *  short of what the run holds, which the registry would refuse (the site
 *  says the same above its button). */
export function hostedCostLine(a: {
  credits: number; holdCredits: number; limited: boolean; max: number; left: number; balance: number; balanceKnown: boolean;
}): string {
  const cost = hostedVoiceCost(a);
  if (!a.balanceKnown || a.holdCredits === 0 || a.balance >= a.holdCredits) return cost;
  return `${cost} This account has ${a.balance.toLocaleString("en-GB")} credits, fewer than the Voice Writer costs, so the registry will refuse it.`;
}
