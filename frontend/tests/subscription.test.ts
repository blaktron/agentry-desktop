// The subscription's words (agentry-desktop#415, plan promo-codes.md §7),
// run by node's own test runner: `npm test`, which CI runs beside the build.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import { changesPhrase, creditsPhrase, owedSentence, packBonus, packLabel, packPrice, tierLabel, topUpOffer } from "../src/subscription.ts";

const view = (v: Record<string, unknown>) => v as unknown as main.SubscriptionView;

test("every tier a person or an organization may hold has its product name", () => {
  assert.equal(tierLabel("standard"), "Standard");
  assert.equal(tierLabel("pro"), "Pro");
  assert.equal(tierLabel("promo_pro"), "Promo Pro");
  assert.equal(tierLabel("business"), "Business");
  assert.equal(tierLabel("enterprise"), "Enterprise");
});

test("a Promo Pro account spends the code's credits and becomes Standard at the trial's end", () => {
  const s = view({
    tier: "promo_pro", balance: 60, allowance: 0, submissionCost: 1,
    periodEnd: "2026-10-30T01:47:46.814803Z", pendingTier: "standard",
    promo: { state: "running", code: "LAUNCH", credits: 60, endsAt: "2026-10-30T01:47:46.814803Z" },
  });
  assert.equal(tierLabel(s.tier), "Promo Pro");
  assert.equal(creditsPhrase(s), "60 of 60 promo credits");
  assert.equal(changesPhrase(s), "Standard from 30 October 2026");
});

test("an account that may still redeem reads as Standard, with its allowance", () => {
  const s = view({
    tier: "standard", balance: 50, allowance: 50, submissionCost: 1,
    periodEnd: "2026-10-30T00:00:00Z", promo: { state: "eligible" },
  });
  assert.equal(creditsPhrase(s), "50 of 50 credits");
  assert.equal(changesPhrase(s), null);
});

// Credit packs (agentry-desktop#511): the card's condition, its numbers and
// labels, and the owed sentence.
const PACKS = [
  { id: "1250", credits: 1250, baseCredits: 1000, amount: 999, currency: "usd" },
  { id: "5000", credits: 5000, baseCredits: 3500, amount: 3499, currency: "usd" },
];
const pro = (extra: Record<string, unknown> = {}) =>
  view({ tier: "pro", balance: 80, allowance: 1000, submissionCost: 1, periodEnd: "2026-11-01T00:00:00Z",
    topUp: { available: true, url: "https://registry.test/settings/profile#top-up", processing: false, packs: PACKS }, ...extra });

test("the Top up credits card is drawn only where the registry offers this person packs", () => {
  assert.equal(topUpOffer(pro())?.packs.length, 2);
  assert.equal(topUpOffer(pro({ topUp: undefined })), null, "a registry from before packs");
  assert.equal(topUpOffer(pro({ topUp: { available: false, processing: false, packs: [] } })), null, "Standard, or no prices");
  assert.equal(topUpOffer(pro({ topUp: { available: true, processing: false, packs: [] } })), null, "no pack priced");
  assert.equal(topUpOffer(pro({ memberOf: "acme", tier: "business" })), null, "a member's view");
});

test("each pack reads its struck base, its bonus and one sentence, at the provider's price", () => {
  const [small, large] = PACKS as unknown as main.TopUpPack[];
  assert.equal(packBonus(small), "+250 bonus");
  assert.equal(packBonus(large), "+1,500 bonus");
  assert.equal(packLabel(small), "1,250 credits: 1,000 credits plus a 250 credit bonus");
  assert.equal(packLabel(large), "5,000 credits: 3,500 credits plus a 1,500 credit bonus");
  assert.equal(packLabel({ ...small, baseCredits: 1250 } as main.TopUpPack), "1,250 credits");
  assert.equal(packBonus({ ...small, baseCredits: 1250 } as main.TopUpPack), null);
  assert.equal(packPrice(small), "US$9.99");
  assert.equal(packPrice(large), "US$34.99");
  assert.equal(packPrice({ amount: 1450, currency: "eur" }), "€14.50");
  assert.equal(packPrice({ amount: 1500, currency: "jpy" }), "JP¥1,500");
});

test("a suspended account is told what it owes and how the suspension ends", () => {
  assert.match(owedSentence(1500), /owes 1,500 credits\./);
  assert.match(owedSentence(1), /owes 1 credit\./);
  assert.match(owedSentence(40), /The next credits to arrive pay what is owed first\./);
});
