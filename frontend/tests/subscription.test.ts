// The subscription's words (agentry-desktop#415, plan promo-codes.md §7),
// run by node's own test runner: `npm test`, which CI runs beside the build.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import { changesPhrase, creditsPhrase, tierLabel } from "../src/subscription.ts";

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
