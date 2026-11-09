// Managed settings: the values an organization's admins set for their
// members, which Go keeps current and the runner applies. These are the
// words Settings shows for them. Nothing here decides anything: Go refuses
// Unsafe Mode under the lock, and the runner refuses a run the settings
// forbid.

/** One organization's managed settings held on this computer; summary is
 *  Go's one-line name for them, "managed by acme (version 7)". */
export type ManagedHeld = { summary: string };

/** What Unsafe Mode's switch carries while managed settings refuse it. */
export type UnsafeLock = { managedBy?: string; reason: string; savedOn: boolean };

const joinAnd = (xs: string[]) => (xs.length < 2 ? xs.join("") : `${xs.slice(0, -1).join(", ")} and ${xs[xs.length - 1]}`);

const asSentence = (s: string) => (/[.!?]$/.test(s.trim()) ? s.trim() : `${s.trim()}.`);

/** The Settings landing's line: the organizations whose managed settings
 *  apply here, each with its version; null when none do. */
export function managedLine(held: ManagedHeld[] | null | undefined): string | null {
  const named = (held ?? []).map((d) => d.summary).filter(Boolean);
  if (!named.length) return null;
  return `Some settings are ${joinAnd(named)}.`;
}

/** The badge beside Unsafe Mode's switch under the lock. */
export function unsafeLockBadge(lock: UnsafeLock): string {
  return lock.managedBy ? `Managed by ${lock.managedBy}` : "Locked";
}

/** The lines under the locked switch: the reason, and what happened to a
 *  switch that was already on. */
export function unsafeLockLines(lock: UnsafeLock): string[] {
  const lines = [asSentence(lock.reason)];
  if (lock.savedOn) {
    lines.push(
      "Unsafe Mode was on here before. It stays off while these managed settings forbid it, and no run on this computer uses it. " +
        "If they stop forbidding it, it comes back on unless you keep it off.",
    );
  }
  return lines;
}

/** The Runs card's words for the switch under the lock. */
export function unsafeLockStatus(lock: UnsafeLock): string {
  return lock.managedBy ? `Unsafe Mode is off, managed by ${lock.managedBy}.` : "Unsafe Mode is off and locked.";
}

// ---- Every managed row ------------------------------------------------
//
// Go lists the rows the held documents set (ManagedRows); a part asks for the
// rows it governs and draws what this module words: a lock with the
// organization's name, an organization default, or nothing for a row no
// document sets.

/** One row a document sets, as Go lists it. value is the row's JSON text. */
export type ManagedRow = { id: string; state: "locked" | "default"; value: string; organization: string; applied: boolean };

/** The words and the control state of one row. */
export type RowNote = {
  id: string;
  state: "locked" | "default";
  /** "Managed by acme" for a lock, "Set by acme" for a default. */
  badge: string;
  /** What the organization set, then what it means for the control. */
  lines: string[];
  /** The control is disabled: the lock fixes a choice the member cannot
   *  change. A cap, a set and a lock that leaves the choice open keep it
   *  enabled. */
  disabled: boolean;
  /** The label of the button that forgets the member's own value so the
   *  default applies; "" for a lock. */
  use: string;
};

/** The parts that show managed rows, and the rows each governs. */
export type ManagedPart =
  | "budgets" | "where" | "runhosts" | "deliverables" | "destinations" | "malware" | "unsafe"
  | "applications" | "screen" | "agents" | "installs" | "scheduled" | "updates" | "bug-report";

export const PART_ROWS: Record<ManagedPart, string[]> = {
  budgets: ["run.timeout", "run.maxTurns", "run.maxCost"],
  where: ["run.where"],
  runhosts: ["run.where"],
  deliverables: ["deliverables.destinations"],
  destinations: ["deliverables.destinations"],
  malware: ["malwareCheck"],
  unsafe: ["run.unsafeMode"],
  applications: ["desktop.applications", "desktop.customServers"],
  screen: ["desktop.screen"],
  agents: ["agents.assistant", "agents.codingAgent", "messengers", "persistentReleases"],
  installs: ["install.sources", "install.acceptRisk"],
  scheduled: ["schedules"],
  updates: ["updates.minimumVersion"],
  "bug-report": ["bugReports"],
};

// What each row decides, as the part's sentence names it.
const LABELS: Record<string, string> = {
  "agents.assistant": "The Assistant",
  "agents.codingAgent": "The Coding agent",
  "browser.publicWeb": "A run's browser reaching the public web",
  bugReports: "Bug reports",
  "deliverables.destinations": "Where a run may send its result",
  "desktop.applications": "Runs using applications on this computer",
  "desktop.customServers": "Runs using applications you set up yourself",
  "desktop.screen": "Runs seeing and using your screen",
  "egress.wildcard": "Wildcard egress hosts in policies",
  "install.acceptRisk": "Accepting a release's risk at install",
  "install.sources": "Where QuickAgents may be installed from",
  malwareCheck: "The malware check",
  "mcpServers.allowVulnerable": "MCP servers rated high or critical",
  messengers: "Messengers for persistent agents",
  persistentReleases: "Persistent QuickAgents from the registry",
  "run.maxCost": "The most a run may cost",
  "run.maxTurns": "The most turns a run may take",
  "run.timeout": "The longest a run may take",
  "run.unsafeMode": "Unsafe Mode",
  "run.where": "Where runs may go",
  schedules: "Scheduled runs",
  "updates.minimumVersion": "The oldest Agentry client that may run",
};

const CAP_UNITS: Record<string, string> = { "run.timeout": "seconds", "run.maxTurns": "turns", "run.maxCost": "US dollars" };

const PLACES: Record<string, string> = { local: "this computer", hosted: "In Agentry", runhost: "a runhost", "target-host": "a target host" };
const DESTINATIONS: Record<string, string> = { folder: "a folder", webhook: "a webhook", s3: "an S3 bucket" };
const CHOICES: Record<string, string> = { allowed: "allowed", forbidden: "not allowed", optional: "optional", required: "required" };
// The values that restrict the member: a lock on one fixes the control.
const RESTRICTIVE = new Set(["forbidden", "required"]);

/** The row's value, parsed; undefined when the text is not JSON. */
export function rowValue(row: Pick<ManagedRow, "value">): unknown {
  try {
    return JSON.parse(row.value);
  } catch {
    return undefined;
  }
}

/** The row with this id, or null when no document sets it. */
export function rowOf(rows: ManagedRow[] | null | undefined, id: string): ManagedRow | null {
  return (rows ?? []).find((r) => r.id === id) ?? null;
}

/** The value in words: "required", "at most 900 seconds", "this computer
 *  and In Agentry", "version 0.7.30 or later". */
export function valueWords(row: ManagedRow): string {
  const v = rowValue(row);
  if (typeof v === "string") {
    if (row.id === "updates.minimumVersion") return `version ${v} or later`;
    return CHOICES[v] ?? v;
  }
  if (typeof v === "number") return `at most ${v} ${CAP_UNITS[row.id] ?? ""}`.trim();
  if (Array.isArray(v)) {
    const names = row.id === "run.where" ? PLACES : DESTINATIONS;
    return joinAnd(v.map((x) => names[String(x)] ?? String(x)));
  }
  if (v && typeof v === "object") {
    const o = v as { scope?: string; namespaces?: string[] };
    if (o.scope === "any") return "anywhere";
    if (o.scope === "organization-and-built-ins") {
      const ns = (o.namespaces ?? []).filter(Boolean);
      return ns.length
        ? `the organization, the built-ins and ${joinAnd(ns)}`
        : "the organization and the built-ins";
    }
  }
  return row.value;
}

const capital = (s: string) => (s ? s.charAt(0).toUpperCase() + s.slice(1) : s);

/** The cap a lock puts on a number row (timeout, turns, cost), or null. */
export function lockedCap(row: ManagedRow | null): number | null {
  if (!row || row.state !== "locked") return null;
  const v = rowValue(row);
  return typeof v === "number" ? v : null;
}

/** The members of a set row a lock still allows, or null when the row is not
 *  a locked set. */
export function lockedMembers(row: ManagedRow | null): string[] | null {
  if (!row || row.state !== "locked") return null;
  const v = rowValue(row);
  return Array.isArray(v) ? v.map(String) : null;
}

/** Whether a lock forbids what the row governs: a choice locked to its
 *  restrictive value. */
export function forbids(row: ManagedRow | null): boolean {
  if (!row || row.state !== "locked") return false;
  const v = rowValue(row);
  return typeof v === "string" && RESTRICTIVE.has(v);
}

/** The words and the control state of a row; null when no document sets it,
 *  so the part shows nothing. */
export function rowNote(row: ManagedRow | null | undefined): RowNote | null {
  if (!row) return null;
  const label = LABELS[row.id] ?? row.id;
  const org = row.organization || "your organization";
  const locked = row.state === "locked";
  const lines = [`${label}: ${valueWords(row)}.`];
  if (locked && !forbids(row) && typeof rowValue(row) === "string" && row.id !== "updates.minimumVersion") {
    lines.push(`${capital(org)} leaves this to you.`);
  }
  if (!row.applied) {
    lines.push(locked
      ? "This version of the agentry command cannot hold you to it, so no run starts on this computer until Agentry is updated."
      : "This version of the agentry command does not apply it yet.");
  }
  return {
    id: row.id,
    state: row.state,
    badge: locked ? `Managed by ${org}` : `Set by ${org}`,
    lines,
    disabled: forbids(row),
    use: locked ? "" : `Use ${org}'s setting`,
  };
}

/** The notes a part draws, in the part's order, for the rows a document sets. */
export function partNotes(rows: ManagedRow[] | null | undefined, part: ManagedPart): RowNote[] {
  return PART_ROWS[part].map((id) => rowNote(rowOf(rows, id))).filter((n): n is RowNote => n !== null);
}

/** The note of one row, or null. */
export const noteOf = (rows: ManagedRow[] | null | undefined, id: string) => rowNote(rowOf(rows, id));
