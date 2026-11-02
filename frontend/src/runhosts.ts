// The words and rules of remote runhosts in the app, pure so node's test
// runner holds them.

/** Why name cannot name a runhost, "" when it can: the CLI's rule, 1 to 32
 *  lower-case letters, digits and inner hyphens. */
export function runhostNameProblem(name: string): string {
  return /^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$/.test(name)
    ? ""
    : "Use 1 to 32 lower-case letters, digits and inner hyphens, starting and ending with a letter or a digit.";
}

/** How a plan sign-in on a runhost is named and described. */
export function signInWords(vendor: "claude" | "chatgpt" | "copilot"): { title: string; blurb: string } {
  switch (vendor) {
    case "claude":
      return { title: "Your Claude subscription", blurb: "Claude Code signs in on the runhost itself and keeps its sign-in there." };
    case "chatgpt":
      return { title: "Your ChatGPT plan", blurb: "The sign-in's refresh token is kept in the runhost's store, encrypted." };
    case "copilot":
      return { title: "Your GitHub Copilot plan", blurb: "The sign-in's refresh token is kept in the runhost's store, encrypted." };
  }
}

/** What a runhost's Unsafe Mode setting means, as its Settings card says. */
export function unsafeModeWords(on: boolean): string {
  return on
    ? "on. Every run on this runhost installs a release's declared commands from the Unsafe Mode image, without the command catalogue."
    : "off. A release's declared commands must be in the runhost's command catalogue.";
}

/** The runhost a run-defaults value names, "" for none. */
export function runhostOf(where: string | undefined): string {
  return where?.startsWith("runhost:") ? where.slice("runhost:".length) : "";
}

/** A runhost's last update by this app, as its fields come from Go. */
export type RunhostUpdate = {
  at: string;
  from?: string;
  to?: string;
  updated: boolean;
  waitsForJobs?: number;
  deviceBehind?: string;
  needsAttention?: string[];
  problem?: string;
};

/** What a runhost's Settings card says of its last update: the sentence, and
 *  whether the person has something to do ("attention"). */
export function updateWords(u: RunhostUpdate | undefined | null): { line: string; attention: boolean } {
  if (!u) return { line: "", attention: false };
  if (u.problem) return { line: `Its last update did not go through: ${u.problem}`, attention: true };
  if (u.needsAttention && u.needsAttention.length > 0) {
    return { line: `Needs attention: ${u.needsAttention.join("; ")}`, attention: true };
  }
  if (u.deviceBehind) {
    return { line: `It runs CLI ${u.deviceBehind}, newer than this app's: update this app to run there.`, attention: true };
  }
  if (u.updated && u.waitsForJobs) {
    return { line: `Updated to CLI ${u.to}; it restarts into it once its ${u.waitsForJobs} running job${u.waitsForJobs === 1 ? "" : "s"} finish.`, attention: false };
  }
  if (u.updated) return { line: `Updated from CLI ${u.from} to ${u.to}.`, attention: false };
  return { line: `Up to date at CLI ${u.to}.`, attention: false };
}
