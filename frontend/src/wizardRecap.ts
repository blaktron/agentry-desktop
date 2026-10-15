import type { app } from "../wailsjs/go/models";
import type { KeyStanding } from "./keyRecommendation";

// The first-run wizard's recap rules that are worth holding in a test: the
// AI Review key's row on Done, what Browse's "Setup isn't finished" notice
// lists after "I'll do it later", and what counts as a provider set up.
// Pure, with type-only imports, so node's own test runner can load it: the
// key's standing is the caller's to work out (keyRecommendation.ts).

/** One line of the recap: a check for what is true, a bullet for what is
 *  not. aside: the row is a recommendation, not something the app needs
 *  before it can sign in, run or build, so the notice never lists it. */
export type RecapRow = { ok: boolean; text: string; aside?: boolean };

/** The AI Review key's row, or null when its status could not be read (a
 *  read that fails drops its line). It says where the key comes from and
 *  never what it is. */
export function keyRow(k: { standing: KeyStanding; envName: string } | null): RecapRow | null {
  if (!k) return null;
  switch (k.standing) {
    case "env":
      return { ok: true, text: `AI Review key from $${k.envName}`, aside: true };
    case "saved":
      return { ok: true, text: "AI Review key saved", aside: true };
    case "unread":
      return { ok: false, text: "An AI Review key is in the keychain, but the runner policy does not read it — save it again in Settings", aside: true };
    default:
      return { ok: false, text: "No AI Review key — drafting on this computer is off until you add one in Settings", aside: true };
  }
}

/** The provider's row. A policy that exists is not a provider set up:
 *  saving the AI Review key, choosing a coding agent and the first run each
 *  write the policy from defaults, so the row goes by providerSetUp (Go's
 *  run.Status: the policy's model block is not the default's). A policy
 *  that holds only the default says that and where its key is read from,
 *  and claims nothing about whether one is there. */
export function providerRow(s: app.SetupStatusView | null): RecapRow {
  const named = s ? `${s.provider}${s.model ? ` (${s.model})` : ""}` : "";
  if (s?.providerSetUp) return { ok: true, text: `Provider ${named} written to the runner policy` };
  if (s?.policyExists) {
    return { ok: false, text: `The runner policy has only the default provider, ${named}, with its key read from $${s.apiKeyEnv} — choose yours in Settings` };
  }
  return { ok: false, text: "Provider defaults will be written on your first run" };
}

/** Nothing set up: the person did none of
 *  the three things the wizard is for — a sign-in, a provider set up, a
 *  coding agent chosen. Docker being present is the machine's doing, and a
 *  saved AI Review key is a recommendation taken, so neither counts;
 *  nor does the policy the key's save wrote from defaults, nor a variable
 *  the shell happened to set. */
export function nothingSetUp(v: { signedIn: boolean; setup: app.SetupStatusView | null; codingAgentChosen: boolean }): boolean {
  return !v.signedIn && !v.setup?.providerSetUp && !v.codingAgentChosen;
}

/** What the notice lists: every pending row the app cannot work without. */
export function missingForNotice(rows: RecapRow[]): string[] {
  return rows.filter((r) => !r.ok && !r.aside).map((r) => r.text);
}
