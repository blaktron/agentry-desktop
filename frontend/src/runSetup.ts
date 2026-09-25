// The guided run setup's pure parts (plan guided-runs.md §4, M3;
// agentry-desktop#226): which steps a release gets, and the "what to have
// ready" list derived from the manifest while no release carries its
// author's own guide (plan D2). The site's run setup reads the same
// manifest the same way (agentry-web lib/run-setup), so a release asks for
// the same things on either client.

export type StepKey = "start" | "files" | "tools" | "details" | "review";

export const STEP_LABEL: Record<StepKey, string> = {
  start: "Before you start",
  files: "Files",
  tools: "Connect tools",
  details: "Details",
  review: "Review and start",
};

// setupSteps leaves Connect tools out when the release declares neither an
// MCP server nor a key: a step with nothing on it is not shown.
export function setupSteps(release: { servers: number; secrets: number }): StepKey[] {
  const steps: StepKey[] = ["start", "files"];
  if (release.servers > 0 || release.secrets > 0) steps.push("tools");
  steps.push("details", "review");
  return steps;
}

interface Question {
  id: string;
  prompt: string;
  type: string;
  required: boolean;
}

// needList is what to gather first: a file for each question that asks for
// one (else that files are optional), the keys the release declares, the
// other questions by their own prompts, the task, which on this machine is
// optional (the packaged instruction runs without one), and, where MCP
// servers are declared with no key, that there is none to set up.
export function needList(release: { intake: Question[]; secrets: string[]; servers?: number; files?: string }): string[] {
  const out: string[] = [];
  const fileQs = release.intake.filter((q) => q.type === "file");
  if (fileQs.length > 0) {
    for (const q of fileQs) out.push(`A file for “${q.prompt}”${q.required ? "" : " (optional)"}.`);
  } else if (release.files) {
    // The author's guide says which files (guided-runs.md §4).
    out.push(release.files);
  } else {
    out.push("Any files the task is about. This is optional: a run can start with none.");
  }
  if (release.secrets.length > 0) {
    out.push(
      `The key${release.secrets.length === 1 ? "" : "s"} it declares: ${release.secrets.join(", ")}. ` +
        "You can also leave a key to this machine's environment.",
    );
  }
  for (const q of release.intake) {
    if (q.type === "file") continue;
    out.push(`Your answer to “${q.prompt}”${q.required ? "" : " (optional)"}.`);
  }
  out.push("A sentence or two saying what you want done, if you want more than its own instruction.");
  const servers = release.servers ?? 0;
  if (servers > 0 && release.secrets.length === 0) {
    out.push(
      servers === 1
        ? "No key to set up: the release declares none for the MCP server it uses."
        : `No key to set up: the release declares none for the ${servers} MCP servers it uses.`,
    );
  }
  return out;
}
