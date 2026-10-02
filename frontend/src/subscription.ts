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

export const creditCount = (n: number) => (n === 1 ? "1 credit" : `${n.toLocaleString("en-US")} credits`);

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

// Credit packs (agentry-desktop#511, plan credit-top-ups.md D6, D7): the
// Top up credits card draws the registry's packs as the site's does, and
// Buy opens the site's card. topUpOffer is the card's condition: packs the
// registry offers this person, never on a member's view.
export function topUpOffer(s: main.SubscriptionView): main.TopUpView | null {
  const t = s.topUp;
  if (!t?.available || s.memberOf || !t.packs?.length) return null;
  return t;
}

// creditNumber: "1,250", the way the card writes a number of credits.
export const creditNumber = (n: number) => n.toLocaleString("en-US");

// bonusOf: the credits a pack brings above its struck base number; 0 for a
// pack sold at its base, which draws no struck number.
export const bonusOf = (p: main.TopUpPack) => Math.max(0, p.credits - p.baseCredits);

// packBonus: "+250 bonus", or null.
export function packBonus(p: main.TopUpPack): string | null {
  const b = bonusOf(p);
  return b > 0 ? `+${creditNumber(b)} bonus` : null;
}

// packLabel is what a screen reader hears for the two numbers: "1,250
// credits: 1,000 credits plus a 250 credit bonus", never two bare numbers.
export function packLabel(p: main.TopUpPack): string {
  const b = bonusOf(p);
  if (b <= 0) return `${creditNumber(p.credits)} credits`;
  return `${creditNumber(p.credits)} credits: ${creditNumber(p.baseCredits)} credits plus a ${creditNumber(b)} credit bonus`;
}

// packPrice: "US$9.99" from 999 USD, in the currency's own minor unit; an
// unknown currency code is printed beside the bare number.
export function packPrice(p: Pick<main.TopUpPack, "amount" | "currency">): string {
  const code = p.currency.toUpperCase();
  try {
    const fmt = new Intl.NumberFormat("en-GB", { style: "currency", currency: code });
    const minor = fmt.resolvedOptions().maximumFractionDigits ?? 2;
    return fmt.format(p.amount / 10 ** minor);
  } catch {
    return `${(p.amount / 100).toFixed(2)} ${code}`;
  }
}

// owedSentence: a suspension for non-payment (D11, D12), in the site's
// words: what is owed, what stops, what still works, how it ends.
export function owedSentence(owed: number): string {
  return `A credit pack's payment failed after some of its credits were spent, so this account owes ${creditCount(owed)}. Until the balance is back at zero, submissions, hosted runs, knowledge builds, drafts and hosted sessions are refused; sign-in, installs and your published QuickAgents work as before. The next credits to arrive pay what is owed first.`;
}
