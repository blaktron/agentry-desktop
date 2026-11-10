// The QuickAgents an organization's managed settings give its members: the
// Library's "From <organization>" section, the badge and the refused
// Uninstall of a required one, the heading over the contract of a required
// install's first run, and the run setup's preset. Go lists the assignments
// and holds every rule; these are the words and the shapes the page draws.

/** What an organization gave the run setup of a QuickAgent: each field a
 *  default, or fixed when locked. */
export type Preset = {
  organization: string;
  locked: boolean;
  timeoutSeconds?: number;
  maxTurns?: number;
  where?: string;
  inputs?: Record<string, string>;
};

/** One QuickAgent a held document lists, as Go lists it. */
export type Assignment = {
  ref: string;
  version: string;
  selector: string;
  mode: string;
  required: boolean;
  organization: string;
  registry: string;
  installed: boolean;
  installedVersion?: string;
  problem?: string;
  preset?: Preset | null;
};

/** The assignments of one organization, for one heading of the section. */
export type OrganizationGroup = { organization: string; items: Assignment[] };

/** The assignments grouped by organization, in the order Go lists them,
 *  required ones before offered ones within a group. */
export function groupByOrganization(list: Assignment[] | null | undefined): OrganizationGroup[] {
  const groups: OrganizationGroup[] = [];
  for (const a of list ?? []) {
    let g = groups.find((x) => x.organization === a.organization);
    if (!g) groups.push((g = { organization: a.organization, items: [] }));
    g.items.push(a);
  }
  for (const g of groups) g.items = [...g.items.filter((a) => a.required), ...g.items.filter((a) => !a.required)];
  return groups;
}

/** The section's heading. */
export const fromOrganization = (organization: string) => `From ${organization}`;

/** The badge on a QuickAgent a document requires. */
export const requiredBadge = (organization: string) => `Required by ${organization}`;

/** The assignment that requires the installed QuickAgent ns/name, if any. */
export function requiredFor(list: Assignment[] | null | undefined, namespace: string, name: string): Assignment | null {
  const ref = `${namespace}/${name}`;
  return (list ?? []).find((a) => a.required && a.ref === ref) ?? null;
}

/** Why Uninstall is off for a required QuickAgent. */
export function uninstallExplanation(a: Assignment): string {
  return `${requiredBadge(a.organization)}: it stays installed while their managed settings require it.`;
}

/** What an assignment's button does: install an offered one that is not
 *  installed, try a required one again when its install did not happen, or
 *  open an installed one. */
export type AssignmentAction = "install" | "retry" | "open";

export function assignmentAction(a: Assignment): AssignmentAction {
  if (a.installed) return "open";
  return a.required ? "retry" : "install";
}

/** The line under an assignment: what it is and where it stands. */
export function assignmentLine(a: Assignment): string {
  const version = a.version === "latest" ? "the newest version" : `version ${a.version}`;
  if (a.installed) {
    const at = a.installedVersion ? ` (${a.installedVersion} is installed)` : "";
    return a.required ? `Required, kept at ${version}${at}.` : `Offered at ${version}${at}.`;
  }
  if (a.required) return a.problem ? a.problem : `Required, at ${version}. It installs the next time the settings are read.`;
  return `Offered at ${version}.`;
}

/** The note the preset adds to an assignment's row, or "". */
export function presetLine(p: Preset | null | undefined): string {
  if (!p) return "";
  const parts: string[] = [];
  if (p.timeoutSeconds) parts.push(`a timeout of ${p.timeoutSeconds} seconds`);
  if (p.maxTurns) parts.push(`${p.maxTurns} turns`);
  if (p.where) parts.push(`runs on ${PLACES[p.where] ?? p.where}`);
  const n = Object.keys(p.inputs ?? {}).length;
  if (n) parts.push(n === 1 ? "an answer" : `${n} answers`);
  if (!parts.length) return "";
  return `${p.organization} ${p.locked ? "fixes" : "suggests"} ${joinAnd(parts)} for its runs.`;
}

const PLACES: Record<string, string> = { local: "this computer", hosted: "In Agentry", runhost: "a runhost", "target-host": "a target host" };

function joinAnd(xs: string[]): string {
  return xs.length < 2 ? xs.join("") : `${xs.slice(0, -1).join(", ")} and ${xs[xs.length - 1]}`;
}

// ---- The run setup ---------------------------------------------------------

/** What a field of the run setup shows and whether it can be changed. */
export type Field = { value: string; disabled: boolean };

/** A number field (timeout, turns): the member's own value stands over a
 *  default; a locked preset fixes it. blank is what the field holds when
 *  the member has typed nothing. */
export function presetNumber(typed: string, preset: number | undefined, locked: boolean): Field {
  if (preset && preset > 0 && locked) return { value: String(preset), disabled: true };
  if (preset && preset > 0 && typed.trim() === "") return { value: String(preset), disabled: false };
  return { value: typed, disabled: false };
}

/** The answers the preset fills in: each question left blank takes its
 *  preset answer, and what the member typed stands. A locked preset fixes
 *  the answer whatever was typed. */
export function presetAnswers(typed: Record<string, string>, preset: Preset | null | undefined): Record<string, string> {
  if (!preset?.inputs) return typed;
  const out = { ...typed };
  for (const [id, want] of Object.entries(preset.inputs)) {
    if (preset.locked || (out[id] ?? "").trim() === "") out[id] = want;
  }
  return out;
}

/** Whether a locked preset fixes the answer to question id. */
export const answerLocked = (preset: Preset | null | undefined, id: string): boolean =>
  !!preset?.locked && !!preset.inputs && id in preset.inputs;

/** Whether a locked preset keeps runs from going to place ("local",
 *  "hosted", "runhost"). */
export const placeLockedOut = (preset: Preset | null | undefined, place: string): boolean =>
  !!preset?.locked && !!preset.where && preset.where !== place;

/** The note over the run setup's fixed or suggested values; "" when the
 *  preset says nothing the setup shows. */
export function presetNote(preset: Preset | null | undefined): string {
  return presetLine(preset);
}

// ---- The first run of a required install ------------------------------------

/** The heading over the contract of a QuickAgent an organization installed
 *  and whose contract the member has yet to accept; "What it may reach"
 *  otherwise. */
export function contractHeading(requiredBy: string | undefined): string {
  return requiredBy ? requiredBadge(requiredBy) : "What it may reach";
}

/** The sentence under that heading. */
export function contractAsk(requiredBy: string | undefined): string {
  return requiredBy
    ? `${requiredBy} installed this for you. It has not run here yet, or its contract changed, so starting a run asks you to accept what it may do.`
    : "";
}
