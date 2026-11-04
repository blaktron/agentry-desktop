// Home's Get started card: what this person's setup still needs for the
// place their runs go, then the first things to try. Pure, with type-only
// imports, so node's own test runner can load it. Every input is a read
// Home already makes or one that only reads this computer; a read that is
// still under way, or failed, is null, and its row is left out rather than
// guessed.

import type { HomeStart } from "./homeCards";
import type { KeyStanding } from "./keyRecommendation";

// What a row's button opens. The page maps each to a route; none starts
// anything.
export type GetStartedAction = "provider" | "reviewKey" | "runhosts" | "signIn" | "topUp" | "run" | "knowledge" | "draft";

export type GetStartedRow = {
  id: string;
  done: boolean;
  // The row's sentence: what is so, or what to do.
  text: string;
  // The button on a row not yet done.
  button: string;
  action: GetStartedAction;
};

export type GetStartedInput = {
  start: HomeStart;
  // Signed in to the registry in use.
  signedIn: boolean | null;
  // This computer's runtime and model (Local).
  setup: { dockerOk: boolean; providerSetUp: boolean } | null;
  // Where the AI Review key comes from (Local).
  reviewKey: KeyStanding | null;
  // The runhost runs go to (Remote): none named, or its name, whether it
  // answers (null until the check comes back) and its model sign-ins.
  runhost: { name: string; known: boolean; reachable: boolean | null; signIns: string[] } | "none" | null;
  // The account's credits (Cloud).
  credits: number | null;
  // Whether any run is in the history, on this computer or In Agentry.
  ranOnce: boolean | null;
  // How many knowledge packs there are.
  packs: number | null;
};

const row = (id: string, done: boolean, text: string, button: string, action: GetStartedAction): GetStartedRow => ({ id, done, text, button, action });

/** The rows for one start, in the order to do them: what the place runs go
 * needs first, then a first run, a knowledge pack and, where it is not
 * needed, signing in. */
export function getStartedRows(v: GetStartedInput): GetStartedRow[] {
  const out: GetStartedRow[] = [];
  if (v.start === "local") {
    if (v.setup) {
      out.push(v.setup.providerSetUp
        ? row("model", true, "A model is set up for your runs", "", "provider")
        : row("model", false, "Set up a model for your runs: a plan you sign in to, or a key", "Set up a model", "provider"));
      out.push(v.setup.dockerOk
        ? row("runtime", true, "Docker is ready to run QuickAgents in their sandbox", "", "provider")
        : row("runtime", false, "Get Docker ready: QuickAgents run in a sandbox on it", "Set up Docker", "provider"));
    }
    if (v.reviewKey) {
      const has = v.reviewKey === "saved" || v.reviewKey === "env";
      out.push(has
        ? row("review-key", true, "An AI Review key is set, so you can draft QuickAgents here", "", "reviewKey")
        : v.reviewKey === "unread"
          ? row("review-key", false, "Your AI Review key is in the keychain, but runs do not read it yet: save it again", "Save it again", "reviewKey")
          : row("review-key", false, "Add an AI Review key to draft and check QuickAgents here", "Add a key", "reviewKey"));
    }
  }
  if (v.start === "remote" && v.runhost) {
    const r = v.runhost;
    if (r === "none" || !r.known) {
      out.push(row("runhost", false, "Set up a runhost for your runs to go to", "Set up a runhost", "runhosts"));
    } else {
      if (r.reachable === false) out.push(row("runhost", false, `${r.name} does not answer`, "Check it", "runhosts"));
      else if (r.reachable) out.push(row("runhost", true, `Your runs go to ${r.name}, and it answers`, "", "runhosts"));
      out.push(r.signIns.length > 0
        ? row("runhost-model", true, `${r.name} is signed in to ${r.signIns.join(", ")}`, "", "runhosts")
        : row("runhost-model", false, `Sign ${r.name} in to a Claude, ChatGPT or Copilot plan, or give each run an API key`, "Sign it in", "runhosts"));
    }
  }
  if (v.start === "cloud") {
    if (v.signedIn !== null) {
      out.push(v.signedIn
        ? row("sign-in", true, "Signed in, so your runs can go In Agentry", "", "signIn")
        : row("sign-in", false, "Sign in: runs In Agentry go under your account", "Sign in", "signIn"));
    }
    if (v.signedIn && v.credits !== null) {
      out.push(v.credits > 0
        ? row("credits", true, `You have ${v.credits === 1 ? "1 credit" : `${v.credits.toLocaleString("en-US")} credits`} for runs In Agentry`, "", "topUp")
        : row("credits", false, "Top up credits: runs In Agentry are paid in them", "Top up", "topUp"));
    }
  }
  if (v.ranOnce !== null) {
    out.push(v.ranOnce
      ? row("first-run", true, "You have run a QuickAgent", "", "run")
      : row("first-run", false, "Run your first QuickAgent", "Find one", "run"));
  }
  out.push(row("draft", false, "Describe a job and let the Drafter write a QuickAgent for it", "Draft one", "draft"));
  if (v.packs !== null) {
    out.push(v.packs > 0
      ? row("knowledge", true, `You have ${v.packs === 1 ? "a knowledge pack" : `${v.packs} knowledge packs`} your runs can read`, "", "knowledge")
      : row("knowledge", false, "Make a knowledge pack from your documents for runs to read", "Create one", "knowledge"));
  }
  if (v.start !== "cloud" && v.signedIn === false) {
    out.push(row("sign-in", false, "Sign in to publish QuickAgents and to run them In Agentry", "Sign in", "signIn"));
  }
  return out;
}

/** The card's status line: how many of the rows that can be done are. The
 * Drafter's row is an invitation, never ticked, so it is not counted. */
export function getStartedStatus(rows: GetStartedRow[]): string {
  const counted = rows.filter((r) => r.id !== "draft");
  const done = counted.filter((r) => r.done).length;
  if (counted.length === 0) return "Checking your setup…";
  if (done === counted.length) return "All set. Here is what to try next.";
  return `${done} of ${counted.length} done`;
}
