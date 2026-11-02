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
