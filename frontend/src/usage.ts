// How a job's use and terms read (agentry-desktop#457; plan
// scan-and-run-tweaks.md track B). Two halves that never meet:
//
// - In Agentry — a run, a session, a knowledge build, a draft — is stated in
//   credits, time and turns only: no token count and no tokens-per-credit
//   rate, before, during or after. The words are the site's (agentry-web
//   lib/runs.ts), and every number is the registry's.
// - On this computer, with the person's own provider, a job is stated in
//   tokens (and the provider's dollar cost when it reports one), never in
//   credits.
//
// Pure, so the pages and their tests share one set of sentences.

/** Seconds in the largest whole unit: hours, minutes or seconds. */
export function fmtDuration(sec: number): string {
  if (sec % 3600 === 0) return `${sec / 3600} hour${sec === 3600 ? "" : "s"}`;
  if (sec % 60 === 0) return `${sec / 60} minute${sec === 60 ? "" : "s"}`;
  return `${sec} second${sec === 1 ? "" : "s"}`;
}

/** "1 credit", "1,200 credits": subscription.ts's creditCount. Both modules
 *  keep their imports type-only so node's test runner can load them, which
 *  is why each holds its own copy. */
export function creditCount(n: number): string {
  return n === 1 ? "1 credit" : `${n.toLocaleString("en-US")} credits`;
}

const fmtNumber = (n: number) => n.toLocaleString("en-US");

/** The site's words for a payer whose organization is not metered. */
export const UNMETERED_RUNS_CLAUSE = "the organization that pays for your runs is not metered";

/** A tier's ceilings for a run In Agentry, as the person is told them. */
export type HostedCaps = { timeoutSeconds: number; maxTurns: number };

/** "up to 15 minutes and 60 turns" (agentry-web lib/runs.ts capsPhrase). */
export function capsPhrase(caps: HostedCaps): string {
  return `up to ${fmtDuration(caps.timeoutSeconds)} and ${fmtNumber(caps.maxTurns)} ${caps.maxTurns === 1 ? "turn" : "turns"}`;
}

/** Settings → Profile's reading of a hosted run's terms: the hold for its
 *  cap in credits, the time and the turns (agentry-web lib/runs.ts
 *  capsSentence). The rates a settlement is computed at are the registry's
 *  and are not stated (plan scan-and-run-tweaks.md D9). */
export function capsSentence(c: { holdCredits: number; caps: HostedCaps }): string {
  const caps = capsPhrase(c.caps);
  if (c.holdCredits === 0) return `A hosted run may take ${caps}, and holds no credits: ${UNMETERED_RUNS_CLAUSE}.`;
  return `A hosted run holds ${creditCount(c.holdCredits)} for its cap — ${caps} — and settles from what it uses, never more than it holds.`;
}

/** What a run In Agentry holds, before it is created. */
export function runHoldSentence(holdCredits: number): string {
  if (holdCredits === 0) return `This run holds no credits: ${UNMETERED_RUNS_CLAUSE}.`;
  return `This run holds ${creditCount(holdCredits)} while it is queued and running, and settles from what it uses, never more than it holds.`;
}

/** A run In Agentry's ceilings on the account's tier. */
export function runCapsLine(tier: string, caps: HostedCaps): string {
  return `A run on the ${tier} tier may take ${capsPhrase(caps)}.`;
}

/** A session In Agentry's terms (agentry-web lib/runs.ts sessionSentence):
 *  it holds nothing, its first minutes are free, each minute after is
 *  pulled as it starts, and its model use settles when it ends. */
export function sessionTermsSentence(t: { freeMinutes: number; creditsPerMinute: number; minCredits: number }, metered = true): string {
  if (!metered) return `A session costs no credits here: ${UNMETERED_RUNS_CLAUSE}.`;
  return `A session holds nothing. Its first ${t.freeMinutes} minutes are free, then each minute costs ${creditCount(t.creditsPerMinute)} as it starts, and its model use settles when it ends. It starts on a balance of at least ${creditCount(t.minCredits)}.`;
}

/** What a finished session In Agentry cost (plan hosted-sessions.md §7;
 *  DEV-178), in the site's sentence: it held nothing; its time is the
 *  minutes after the free ones, each pulled as it started; its model use
 *  settled at the end within the balance, and any rest was the platform's.
 *  `tokenCredits` and `tokensUnpaid` are credits, named after what they
 *  pay for. */
export function sessionCreditsSentence(run: {
  settled: boolean;
  settledCredits: number;
  session: { freeMinutes: number; paidThrough: number; timeCredits: number; tokenCredits: number; tokensUnpaid: number; outOfCredits: boolean };
}): string {
  const s = run.session;
  const paid = Math.max(0, s.paidThrough - s.freeMinutes);
  const time = `its time cost ${creditCount(s.timeCredits)}${paid > 0 ? ` (${paid} ${paid === 1 ? "minute" : "minutes"} after the ${s.freeMinutes} free)` : ` (within the ${s.freeMinutes} free minutes)`}`;
  const total = run.settled ? run.settledCredits : s.timeCredits + s.tokenCredits;
  const unpaid = s.tokensUnpaid > 0 ? ` The rest of its model use, ${creditCount(s.tokensUnpaid)}, went past the balance and was not charged.` : "";
  const stopped = s.outOfCredits ? " A minute the balance could not cover stopped it." : "";
  return `This session held nothing: ${time}, and its model use ${creditCount(s.tokenCredits)}, ${creditCount(total)} in all.${unpaid}${stopped}`;
}

/** What a knowledge build In Agentry holds (agentry-desktop#274), in the
 *  site's words: the scan's credit is inside the hold, never on top of it
 *  (DEV-150). */
export function buildHoldLine(hold: number, scan: number): string {
  if (hold === 0) return "This build holds no credits: the organization that pays for your builds is not metered.";
  return `This build holds ${creditCount(hold)} and settles from what it uses, plus ${creditCount(scan)} to scan the pack when the build changes it, never more than it holds.`;
}

/** A finished run In Agentry's use: its turns and its time. */
export function hostedUseLine(v: { turns: number; wallMs: number }): string {
  return `${v.turns} ${v.turns === 1 ? "turn" : "turns"} · ${(v.wallMs / 1000).toFixed(1)}s`;
}

/** A finished run on this computer's use: its turns, its tokens, its time
 *  and the provider's dollar cost when it reported one. */
export function localUseLine(r: { turns: number; inputTokens: number; outputTokens: number; wallMs: number; costUsd: number }): string {
  const cost = r.costUsd > 0 ? ` · $${r.costUsd.toFixed(4)}` : "";
  return `${r.turns} ${r.turns === 1 ? "turn" : "turns"} · ${fmtNumber(r.inputTokens + r.outputTokens)} tokens · ${(r.wallMs / 1000).toFixed(1)}s${cost}`;
}

/** What the Knowledge Creator used on this computer, as its provider
 *  reported it; null when it reported nothing. */
export function creatorUseLine(r: { inputTokens: number; outputTokens: number; costUsd: number }): string | null {
  if (r.inputTokens <= 0 && r.outputTokens <= 0) return null;
  const cost = r.costUsd > 0 ? `, US$${r.costUsd.toFixed(4)} as the provider reported it` : "";
  return `${fmtNumber(r.inputTokens)} input and ${fmtNumber(r.outputTokens)} output tokens${cost}`;
}

/** The tokens a job on this computer has used so far; null for a job that
 *  reports none, which every job In Agentry is. */
export function progressTokens(p: { inputTokens?: number; outputTokens?: number }): string | null {
  const i = p.inputTokens ?? 0;
  const o = p.outputTokens ?? 0;
  if (i <= 0 && o <= 0) return null;
  return `${fmtNumber(i)} input, ${fmtNumber(o)} output`;
}
