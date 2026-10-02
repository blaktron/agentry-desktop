// The guided run setup's pure parts (plan guided-runs.md §4, M3;
// agentry-desktop#226): which steps a release gets, and the "what to have
// ready" list derived from the manifest while no release carries its
// author's own guide (plan D2). The site's run setup reads the same
// manifest the same way (agentry-web lib/run-setup), so a release asks for
// the same things on either client.

export type StepKey = "start" | "files" | "knowledge" | "tools" | "details" | "review";

export const STEP_LABEL: Record<StepKey, string> = {
  start: "Before you start",
  files: "Files",
  knowledge: "Knowledge",
  tools: "Connect tools",
  details: "Details",
  review: "Review and start",
};

// setupSteps leaves Connect tools out when the release declares neither an
// MCP server nor a key, and Knowledge out when no pack can be attached (none
// is kept on this computer, or the run is In Agentry, which attaches packs
// from Knowledge M7): a step with nothing on it is not shown.
export function setupSteps(release: { servers: number; secrets: number; knowledge?: boolean }): StepKey[] {
  const steps: StepKey[] = ["start", "files"];
  if (release.knowledge) steps.push("knowledge");
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

// Run defaults' "where to run" (plan settings-cards.md D13;
// agentry-desktop#347) only pre-selects the Where step. inAgentryByDefault
// says whether it may pick In Agentry for this run: the default says so, the
// mode is offered and available here, and the release is not one admitted
// without a scan (which is never run In Agentry from this app). Nothing is
// started by it: Run is still the person's click on Review and start, after
// the credits the run holds are shown.
export type HostedOffer = { offered: boolean; available: boolean; signedIn?: boolean } | null;
// RunWhere is Run defaults' "where to run", as Go keeps it in desktop.yaml
// (settings.RunWhereHere, settings.RunWhereAgentry).
export type RunWhere = "here" | "agentry";

export function inAgentryByDefault(where: string | undefined, hosted: HostedOffer, unscanned: boolean): boolean {
  return where === "agentry" && !!hosted?.offered && hosted.available && !unscanned;
}

// whereFallback is the sentence the Where step shows when the default says
// In Agentry and this run starts on this computer instead; null otherwise.
// hosted is null when the offer could not be read. A mode that is offered but
// unavailable has its own notice with the reason.
export function whereFallback(where: string | undefined, hosted: HostedOffer, unscanned: boolean): string | null {
  if (where !== "agentry" || inAgentryByDefault(where, hosted, unscanned)) return null;
  const why = unscanned
    ? "a release admitted without a scan is not run In Agentry from this app"
    : hosted === null
      ? "the app could not tell whether this registry offers a run In Agentry for it"
      : hosted.signedIn === false
        ? "a run In Agentry needs you signed in to this registry (Settings → Registry settings)"
        : !hosted.offered
          ? "a run In Agentry is not offered for this QuickAgent on this registry"
          : "In Agentry is unavailable for it (the notice below says why)";
  return `Your Run defaults say In Agentry, but ${why}, so this run starts on this computer.`;
}

/** The run setup's answers, as the person left them: kept with a run on
 *  this computer so that Back to the setup reopens the setup as it was
 *  (agentry-desktop#465). */
export type RunForm = {
  mode: string;
  task: string;
  inputs: string[];
  pickedFiles: string[];
  intakeValues: Record<string, string>;
  attach: string[];
  outDir: string;
  timeoutSec: string;
  maxTurns: string;
  keep: boolean;
  keepTouched: boolean;
  acceptUnsigned: boolean;
  acceptUnscanned: boolean;
  allowVulnerableMCP: boolean;
};

/** What the run setup does with a declared MCP server the registry's gate
 *  rates high (agentry-desktop#459, D-055). On this computer it runs only
 *  in Unsafe Mode, and there only with the person's consent for the run,
 *  which Go asks again in a native dialog; with the switch off the setup
 *  offers no consent and says how to get through. A run In Agentry has no
 *  way through at all. Medium and low never reach here: the registry's
 *  verdict carries no refusal for them. */
export type VulnerableServerStep = {
  /** Offer the "Allow vulnerable MCP servers for this run" consent. */
  offerConsent: boolean;
  /** What holds Run, or null when nothing does. */
  blocker: string | null;
  /** Whether the consent is sent to Go with the run. */
  send: boolean;
};

export function vulnerableServerStep(s: { refused: boolean; inAgentry: boolean; unsafeOn: boolean; consented: boolean }): VulnerableServerStep {
  if (!s.refused) return { offerConsent: false, blocker: null, send: false };
  if (s.inAgentry) {
    return {
      offerConsent: false,
      send: false,
      blocker: "A run In Agentry refuses a declared MCP server the registry's gate rates high, with no way through: run it on this computer in Unsafe Mode, or run a release that pins a fixed version.",
    };
  }
  if (!s.unsafeOn) {
    return {
      offerConsent: false,
      send: false,
      blocker: "A declared MCP server the registry's gate rates high runs on this computer only in Unsafe Mode: turn it on in Settings → Run defaults, or run a release that pins a fixed version.",
    };
  }
  return {
    offerConsent: true,
    send: s.consented,
    blocker: s.consented ? null : "Tick “Allow vulnerable MCP servers for this run”.",
  };
}
