// Settings → Models and keys → Harness: the two container harnesses a person
// chooses between, what each harness a run can name is called, and which
// models run on their vendor's harness whatever the choice says. Pure, so
// node's own test runner can hold them.

/** A container harness as the choice offers it. */
export interface HarnessOption { id: string; label: string; line: string }

/** The container harnesses, the default first: the CLI's
 *  policy.ContainerHarnesses, in the product's words. */
export const HARNESS_OPTIONS: HarnessOption[] = [
  { id: "docker-agent", label: "Docker Agent", line: "Docker's open-source agent runtime, in its own image. The default." },
  { id: "agentry-harness", label: "Agentry harness", line: "The harness built into Agentry's runner. It needs no image of its own." },
];

export const DEFAULT_HARNESS = HARNESS_OPTIONS[0].id;

/** What a harness a receipt names is called: a container harness by its
 *  product name, a vendor's harness (a model sign-in's) by its tool's, and
 *  any other by its own name. */
export function harnessName(adapter: string | undefined): string {
  switch (adapter) {
    case "docker-agent": return "Docker Agent";
    case "agentry-harness": return "Agentry harness";
    case "claude": case "claude-image": return "Claude Code";
    case "codex": case "codex-image": return "Codex";
    case "copilot-image": return "Copilot CLI";
  }
  return adapter ?? "";
}

/** A harness as a sentence names it: "the Agentry harness", else its name. */
export function harnessPhrase(adapter: string): string {
  return adapter === "agentry-harness" ? "the Agentry harness" : harnessName(adapter);
}

/** The vendor's harness a provider paid by a model sign-in runs on, null
 *  for a provider whose models follow the choice (a key, a connected
 *  account, Agentry credits, an endpoint of the person's own). */
export function vendorHarness(way: string): string | null {
  switch (way) {
    case "chatgpt": return "Codex";
    case "claude": return "Claude Code";
    case "copilot": return "Copilot CLI";
  }
  return null;
}

const SIGN_IN_PAYS: Record<string, string> = {
  chatgpt: "your ChatGPT plan",
  claude: "your Claude subscription",
  copilot: "your Copilot plan",
};

/** One line for each model sign-in among the providers: its models run on
 *  the vendor's harness whatever is chosen here. */
export function signInLines(providers: { way: string }[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const p of providers) {
    const h = vendorHarness(p.way);
    if (!h || seen.has(p.way)) continue;
    seen.add(p.way);
    out.push(`Models paid by ${SIGN_IN_PAYS[p.way]} run on ${h} whatever is chosen here.`);
  }
  return out;
}

/** What the default model's runs use when that is not the choice: a model
 *  paid by a sign-in, on its vendor's harness. Null when they follow it. */
export function runsOnLine(choice: { harness: string; runsOn: string }): string | null {
  if (!choice.runsOn || choice.runsOn === choice.harness) return null;
  return `Your default model is paid by a model sign-in, so runs on it use ${harnessName(choice.runsOn)} whatever is chosen here.`;
}

/** What a change of harness says once it is saved. */
export function changedLine(to: string): string {
  const base = `Runs, chats and persistent agents on this computer use ${harnessPhrase(to)} from their next start. One under way finishes on the harness it started on.`;
  return to === DEFAULT_HARNESS ? base : `${base} An Agentry CLI on this computer older than v0.7.57 cannot read the runner policy while it holds this choice: update it, or choose Docker Agent again.`;
}
