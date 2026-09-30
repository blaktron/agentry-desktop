import type { main } from "../wailsjs/go/models";

// The subscription's words, shared by Settings → Profile and its test
// (agentry-desktop#415): pure functions of the billing account view, so
// node's own test runner can check them without a browser. The import
// above is type-only and erased, which is what lets node load this file.

// TIER_LABEL is the product's name for each tier: the two a person may
// hold, the two an organization may (plan organization-subscriptions.md §4),
// and the promo trial's (plan promo-codes.md §4.1).
const TIER_LABEL: Record<string, string> = {
  standard: "Standard",
  pro: "Pro",
  promo_pro: "Promo Pro",
  business: "Business",
  enterprise: "Enterprise",
};

export const tierLabel = (t: string) => TIER_LABEL[t] ?? t;

export function dayOf(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
}

export const creditCount = (n: number) => (n === 1 ? "1 credit" : `${n} credits`);

export const runningPromo = (s: main.SubscriptionView) => (s.promo?.state === "running" ? s.promo : null);

// creditsPhrase: what the account has to spend; during a promo trial the
// credits are the code's.
export function creditsPhrase(s: main.SubscriptionView): string {
  const promo = runningPromo(s);
  if (promo && promo.credits) return `${s.balance} of ${promo.credits} promo credits`;
  return `${s.balance} of ${s.allowance} credits`;
}

// changesPhrase: the tier the account moves to at the period's end
// ("Standard from 30 October 2026"), or null when nothing is pending.
export function changesPhrase(s: main.SubscriptionView): string | null {
  return s.pendingTier ? `${tierLabel(s.pendingTier)} from ${dayOf(s.periodEnd)}` : null;
}
