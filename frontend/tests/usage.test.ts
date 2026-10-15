// How a job's use and terms read: In Agentry in
// credits, time and turns, with no token count and no rate; on this
// computer in tokens, with no credit wording.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import {
  buildHoldLine,
  capsSentence,
  creatorUseLine,
  hostedUseLine,
  localUseLine,
  progressTokens,
  runCapsLine,
  runHoldSentence,
  sessionCreditsSentence,
  sessionTermsSentence,
} from "../src/usage.ts";
import { hostedProgress } from "../src/hostedRun.ts";
import { applyEvent, hostedStatusLine, localStatusLine, newChat } from "../src/sessionChat.ts";

const noTokens = (s: string) => {
  assert.doesNotMatch(s, /token/i, s);
  assert.doesNotMatch(s, /per credit|for every/i, s);
};
const noCredits = (s: string) => assert.doesNotMatch(s, /credit/i, s);

const caps = { timeoutSeconds: 900, maxTurns: 60 };

test("the terms before a run In Agentry give the hold in credits, the time and the turns", () => {
  const hold = runHoldSentence(20);
  assert.equal(hold, "This run holds 20 credits while it is queued and running, and settles from what it uses, never more than it holds.");
  const line = runCapsLine("standard", caps);
  assert.equal(line, "A run on the standard tier may take up to 15 minutes and 60 turns.");
  assert.equal(runHoldSentence(0), "This run holds no credits: the organization that pays for your runs is not metered.");
  [hold, line, runHoldSentence(0)].forEach(noTokens);
});

test("Settings → Profile states a hosted run's cap in credits, time and turns", () => {
  const s = capsSentence({ holdCredits: 20, caps });
  assert.equal(s, "A hosted run holds 20 credits for its cap — up to 15 minutes and 60 turns — and settles from what it uses, never more than it holds.");
  const free = capsSentence({ holdCredits: 0, caps });
  assert.equal(free, "A hosted run may take up to 15 minutes and 60 turns, and holds no credits: the organization that pays for your runs is not metered.");
  [s, free].forEach(noTokens);
});

test("a session's terms and its finished credits name its model use, never tokens", () => {
  const terms = sessionTermsSentence({ freeMinutes: 10, creditsPerMinute: 50, minCredits: 50 });
  assert.equal(terms, "A session holds nothing. Its first 10 minutes are free, then each minute costs 50 credits as it starts, and its model use settles when it ends. It starts on a balance of at least 50 credits.");
  noTokens(terms);
  noTokens(sessionTermsSentence({ freeMinutes: 10, creditsPerMinute: 50, minCredits: 50 }, false));
  const done = sessionCreditsSentence({
    settled: true, settledCredits: 51,
    session: { freeMinutes: 10, paidThrough: 11, timeCredits: 50, tokenCredits: 1, tokensUnpaid: 3, outOfCredits: true },
  });
  assert.equal(done, "This session held nothing: its time cost 50 credits (1 minute after the 10 free), and its model use 1 credit, 51 credits in all. The rest of its model use, 3 credits, went past the balance and was not charged. A minute the balance could not cover stopped it.");
  noTokens(done);
});

test("a knowledge build In Agentry holds credits and settles from what it uses", () => {
  const s = buildHoldLine(201, 1);
  assert.equal(s, "This build holds 201 credits and settles from what it uses, plus 1 credit to scan the pack when the build changes it, never more than it holds.");
  noTokens(s);
  noTokens(buildHoldLine(0, 1));
});

test("a finished run In Agentry reads its turns and its time; one on this computer its tokens", () => {
  const hosted = hostedUseLine({ turns: 4, wallMs: 41200 });
  assert.equal(hosted, "4 turns · 41.2s");
  noTokens(hosted);
  const local = localUseLine({ turns: 4, inputTokens: 5120, outputTokens: 1430, wallMs: 41200, costUsd: 0.0182 });
  assert.equal(local, "4 turns · 6,550 tokens · 41.2s · $0.0182");
  noCredits(local);
});

test("the Knowledge Creator on this computer reads its tokens and the provider's cost", () => {
  const s = creatorUseLine({ inputTokens: 184210, outputTokens: 41088, costUsd: 0.1893 });
  assert.equal(s, "184,210 input and 41,088 output tokens, US$0.1893 as the provider reported it");
  noCredits(s!);
  assert.equal(creatorUseLine({ inputTokens: 0, outputTokens: 0, costUsd: 0 }), null);
});

test("progress on this computer shows its tokens; a run In Agentry's progress carries none", () => {
  const local = progressTokens({ inputTokens: 48210, outputTokens: 3904 });
  assert.equal(local, "48,210 input, 3,904 output");
  noCredits(local!);
  const v = { startedAt: "2026-10-02T10:00:00Z", caps, turns: 0 } as unknown as main.HostedRunView;
  const p = hostedProgress(v)!;
  assert.ok(p);
  assert.equal("inputTokens" in p, false);
  assert.equal("outputTokens" in p, false);
  assert.equal(progressTokens(p), null);
});

test("a chat on this computer counts its tokens and names no credit; a chat In Agentry names no token", () => {
  let c = newChat("", "hello");
  const at = "2026-10-02T10:00:00Z";
  c = applyEvent(c, { kind: "user_turn", time: at, text: "hello" }, 1);
  c = applyEvent(c, { kind: "usage", time: at, inputTokens: 5120, outputTokens: 1430, costUsd: 0.0182 }, 2);
  c = applyEvent(c, { kind: "usage", time: at, totalTokens: 900 }, 3);
  assert.equal(c.tokens, 7450);
  const local = localStatusLine("acme/helper@1.0.0", c, false);
  assert.equal(local, "acme/helper@1.0.0 · interactive · 1 turn · 0 tool calls · 7,450 tokens · $0.0182");
  noCredits(local);

  // A session In Agentry's frames carry no tokens, so its strip, the
  // credits its time has cost, has none to show.
  let h = newChat("", "hello");
  h = applyEvent(h, { seq: 1, kind: "user_turn", time: at, text: "hello" }, 1);
  h = applyEvent(h, { seq: 2, kind: "usage", time: at }, 2);
  const hosted = hostedStatusLine({
    ref: "acme/helper@1.0.0", turns: h.turns, tools: h.tools, now: 120_000, startedAt: 0, lastActivity: 60_000,
    freeMinutes: 10, timeCredits: 0, idleSeconds: 300, busy: false, ended: null,
  });
  noTokens(hosted);
  assert.equal(h.tokens, 0);
});
