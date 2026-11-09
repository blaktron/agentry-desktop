// Settings → Models' rules that are not React: what a provider is called and
// what pays for it, whether its credential is here and what fixes it, what
// names a saved model, why Add is held, and what a removal must ask before it
// can go ahead. Pure, so node's own test runner can hold them.
//
// The words follow the product's: a provider is how the models are paid for,
// a saved model is a provider plus a model under a name, and the page calls
// them Models.

import { providerChoice } from "./modelSetup";

/** A provider as Go lists it (run.Provider): never a key or a token. */
export interface ProviderRow {
  id: string;
  vendor: string;
  name?: string;
  baseUrl?: string;
  way: string;
  keyEnv?: string;
  keySource?: string;
  takesNoKey?: boolean;
  state: string;
  account?: string;
  broken?: string;
  signInVendor?: string;
  copilotPlan?: string;
  models: string[];
}

/** What names a saved model (savedmodels.User). */
export interface ModelUser { kind: string; id: string; name: string; model: string }

/** A saved model as Go lists it (run.SavedModel). */
export interface SavedModelRow {
  id: string;
  name: string;
  provider: string;
  model: string;
  contextWindow?: number;
  default: boolean;
  users: ModelUser[];
}

/** The most saved models a computer keeps (policy.MaxSavedModels). */
export const MAX_SAVED_MODELS = 10;

/** The states a provider's credential is in. */
export const READY = "ready";
export const KEY_MISSING = "key-missing";
export const NEEDS_SIGN_IN = "needs-sign-in";

/** What a provider is called on the page: an endpoint of the person's own
 *  by the name they gave it, a vendor by its product name. */
export function providerName(p: { id: string; vendor: string; name?: string }): string {
  if (p.vendor === "custom") return p.name || p.id;
  return providerChoice(p.vendor)?.short ?? p.vendor;
}

/** What pays for a provider's models, in one short phrase. */
export function paidWith(p: ProviderRow): string {
  switch (p.way) {
    case "chatgpt": return "Your ChatGPT plan";
    case "claude": return "Your Claude subscription, through Claude Code";
    case "copilot": return "Your Copilot plan";
    case "connect": return "Your connected account";
    case "credits": return "Your Agentry credits";
  }
  if (p.takesNoKey) return "Takes no key";
  if (p.keySource === "keyring") return "An API key in this computer's keychain";
  return p.keyEnv ? `An API key from $${p.keyEnv}` : "An API key";
}

/** The badge a provider's state reads, and what fixes it ("" when ready). */
export function stateBadge(p: ProviderRow): { label: string; tone: "ok" | "warn"; fix: string } {
  if (p.state === KEY_MISSING) {
    return {
      label: "Key missing", tone: "warn",
      fix: p.keyEnv && p.keySource !== "keyring"
        ? `$${p.keyEnv} is not set where Agentry Desktop starts. Keep the key in this computer's keychain instead.`
        : "The key is not in this computer's keychain.",
    };
  }
  if (p.state === NEEDS_SIGN_IN) {
    if (p.way === "credits") return { label: "Needs sign-in", tone: "warn", fix: "This computer is not signed in to your SecureAgentry account." };
    return {
      label: "Needs sign-in", tone: "warn",
      fix: p.broken ? `The last refresh of the sign-in was refused (${p.broken}).` : "No sign-in is kept on this computer.",
    };
  }
  const who = p.account && p.way !== "credits" && p.way !== "key" ? `Signed in as ${p.account}` : "Ready";
  return { label: who, tone: "ok", fix: "" };
}

/** Whether a fix for the provider is a key typed here (Key missing). */
export const fixIsKey = (p: ProviderRow) => p.state === KEY_MISSING;

/** Whether a fix for the provider is signing in again in this app. */
export const fixIsSignIn = (p: ProviderRow) => p.state === NEEDS_SIGN_IN && p.way !== "credits";

/** One thing that names a model, in words: "the schedule Morning brief". */
export function userLine(u: ModelUser): string {
  return u.kind === "agent" ? `the agent ${u.name}` : `the schedule ${u.name}`;
}

/** What a saved model is assigned to, for its row. */
export function assignedLine(m: SavedModelRow): string {
  const named = m.users.map(userLine);
  if (m.default) {
    const rest = named.length > 0 ? `, and ${joinAnd(named)} ${named.length === 1 ? "names" : "name"} it too` : "";
    return `Runs, schedules and agents that name no model use it${rest}.`;
  }
  if (named.length === 0) return "Nothing names it yet: pick it when you start a run.";
  return `${capitalise(joinAnd(named))} ${named.length === 1 ? "names" : "name"} it.`;
}

/** Why Add a model is held, "" when it is not: the cap. */
export function addHeld(count: number, max = MAX_SAVED_MODELS): string {
  return count >= max ? `This computer keeps ${max} saved models, the most it can. Remove one to add another.` : "";
}

/** The count line under the list. */
export const countLine = (count: number, max = MAX_SAVED_MODELS) => `${count} of ${max} saved models.`;

/** What a removal must ask before it goes ahead. A model that is the
 *  default needs the new default; one that something names needs where
 *  those go. Either way the answer is one saved model, which is both. */
export interface RemovalAsk {
  /** The models removed (one, or a provider's). */
  removed: SavedModelRow[];
  /** What names them. */
  users: ModelUser[];
  /** Whether the default is among them. */
  takesDefault: boolean;
  /** The saved models they may move to, the default first when it stays. */
  targets: SavedModelRow[];
  /** The default that stays, which "Move them to <default>" names. */
  stayingDefault?: SavedModelRow;
}

export function removalAsk(all: SavedModelRow[], removedIds: string[]): RemovalAsk {
  const removed = all.filter((m) => removedIds.includes(m.id));
  const users = removed.flatMap((m) => m.users);
  const takesDefault = removed.some((m) => m.default);
  const rest = all.filter((m) => !removedIds.includes(m.id));
  const stayingDefault = takesDefault ? undefined : rest.find((m) => m.default);
  const targets = stayingDefault ? [stayingDefault, ...rest.filter((m) => m !== stayingDefault)] : rest;
  return { removed, users, takesDefault, targets, stayingDefault };
}

/** Whether the removal needs the person to pick where things go. */
export const removalNeedsTarget = (a: RemovalAsk) => a.takesDefault || a.users.length > 0;

/** What the Remove button waits for, "" when it can go ahead: a target
 *  picked, when one is needed and there is one to pick. */
export function removalHeld(a: RemovalAsk, target: string): string {
  if (!removalNeedsTarget(a)) return "";
  if (a.targets.length === 0) return "There is no other saved model to move them to: add one first.";
  if (!target || !a.targets.some((m) => m.id === target)) return a.takesDefault ? "Choose the new default first." : "Choose where they go first.";
  return "";
}

/** A model the person may name for a saved model: its own name, else the
 *  model id, made unique among the names already here. */
export function suggestedName(model: string, taken: string[]): string {
  const base = model.trim();
  if (!base) return "";
  const lower = taken.map((n) => n.toLowerCase());
  if (!lower.includes(base.toLowerCase())) return base;
  for (let i = 2; ; i++) {
    const n = `${base} ${i}`;
    if (!lower.includes(n.toLowerCase())) return n;
  }
}

/** What a refusal of the check says, with its Retry where the same save may
 *  be tried again as it is. */
export function refusalLine(r: { kind: string; reason: string }): string {
  const reason = r.reason.charAt(0).toUpperCase() + r.reason.slice(1);
  switch (r.kind) {
    case "cannot-check": return `${reason}. Nothing was saved.`;
    case "not-offered": return `${reason}. Pick one it offers.`;
    case "needs-sign-in": return `${reason}. Sign in again, then save.`;
    case "key-missing": return `${reason}. Add the key, then save.`;
  }
  return `${reason}.`;
}

function joinAnd(items: string[]): string {
  if (items.length <= 1) return items.join("");
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

function capitalise(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
