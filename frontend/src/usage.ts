// How a job's use and terms read. Two halves that never meet:
//
// - In Agentry — a run, a session, a knowledge build, a draft — is stated in
//   credits, time and turns only: no token count and no tokens-per-credit
//   rate, before, during or after. The words are the site's, and every number is the registry's.
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

/** "up to 15 minutes and 60 turns". */
export function capsPhrase(caps: HostedCaps): string {
  return `up to ${fmtDuration(caps.timeoutSeconds)} and ${fmtNumber(caps.maxTurns)} ${caps.maxTurns === 1 ? "turn" : "turns"}`;
}

/** Settings → Profile's reading of a hosted run's terms: the hold for its cap
 *  in credits, the time and the turns. The rates a settlement is computed at
 *  are the registry's and are not stated. */
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

/** A session In Agentry's terms (the site's sessionSentence):
 *  it holds nothing, its first minutes are free, each minute after is
 *  pulled as it starts, and its model use settles when it ends. */
export function sessionTermsSentence(t: { freeMinutes: number; creditsPerMinute: number; minCredits: number }, metered = true): string {
  if (!metered) return `A session costs no credits here: ${UNMETERED_RUNS_CLAUSE}.`;
  return `A session holds nothing. Its first ${t.freeMinutes} minutes are free, then each minute costs ${creditCount(t.creditsPerMinute)} as it starts, and its model use settles when it ends. It starts on a balance of at least ${creditCount(t.minCredits)}.`;
}

/** What a finished session In Agentry cost, in the site's sentence: it held
 *  nothing; its time is the minutes after the free ones, each pulled as it
 *  started; its model use settled at the end within the balance, and any rest
 *  was the platform's. `tokenCredits` and `tokensUnpaid` are credits, named
 *  after what they pay for. */
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

/** What a knowledge build In Agentry holds, in the
 *  site's words: the scan's credit is inside the hold, never on top of it. */
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

/** A persistent agent's figures, as its usage panel shows them: what the
 *  segment running now has used, its last turn, its context, what it cost,
 *  its turns and tool calls, since when it runs, and every segment's tokens
 *  together. A figure the harness did not report reads "not reported",
 *  never 0, and a model whose context window is not known reads "unknown". */
export type AgentUsage = {
  /** The running segment's usage; absent when the agent is not running. */
  segment?: { inputTokens: number; outputTokens: number; totalTokens: number; costUsd: number; userTurns: number; toolCalls: number; since: string };
  /** The last turn's tokens, from its usage events; 0 when none came. */
  lastTurnTokens: number;
  /** The model's context window in tokens; 0 when it is not known. */
  contextWindow: number;
  /** The context the harness says the conversation holds; 0 when it does not say. */
  contextUsed: number;
  /** Every segment's tokens, summed. */
  lifetime: { segments: number; inputTokens: number; outputTokens: number; totalTokens: number };
};

export const NOT_REPORTED = "not reported";
export const UNKNOWN = "unknown";

const tokenLine = (input: number, output: number, total: number): string => {
  if (input > 0 || output > 0) return `${fmtNumber(input + output)} (${fmtNumber(input)} in, ${fmtNumber(output)} out)`;
  return total > 0 ? fmtNumber(total) : NOT_REPORTED;
};

/** The usage panel's rows, label and value. `now` is the page's clock. */
export function agentUsageRows(u: AgentUsage, now: number): { label: string; value: string }[] {
  const s = u.segment;
  const since = s ? Date.parse(s.since) : NaN;
  const context = u.contextWindow > 0
    ? u.contextUsed > 0 ? `${fmtNumber(u.contextUsed)} of ${fmtNumber(u.contextWindow)} tokens` : `${NOT_REPORTED} of ${fmtNumber(u.contextWindow)} tokens`
    : UNKNOWN;
  return [
    { label: "This segment", value: s ? tokenLine(s.inputTokens, s.outputTokens, s.totalTokens) : "not running" },
    { label: "Last turn", value: u.lastTurnTokens > 0 ? `${fmtNumber(u.lastTurnTokens)} tokens` : NOT_REPORTED },
    { label: "Context", value: context },
    { label: "Cost", value: s && s.costUsd > 0 ? `$${s.costUsd.toFixed(4)}` : NOT_REPORTED },
    { label: "Turns", value: s ? fmtNumber(s.userTurns) : "—" },
    { label: "Tool calls", value: s ? fmtNumber(s.toolCalls) : "—" },
    { label: "Running for", value: s && Number.isFinite(since) ? fmtElapsed(Math.max(0, now - since)) : "—" },
    {
      label: "All segments",
      value: u.lifetime.segments > 0
        ? `${tokenLine(u.lifetime.inputTokens, u.lifetime.outputTokens, u.lifetime.totalTokens)} over ${u.lifetime.segments} segment${u.lifetime.segments === 1 ? "" : "s"}`
        : "none yet",
    },
  ];
}

/** An elapsed time in its two largest units: "3 min", "2 h 5 min". */
function fmtElapsed(ms: number): string {
  const m = Math.floor(ms / 60_000);
  if (m < 1) return "under a minute";
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return `${h} h ${m % 60} min`;
}
