// An installed release whose files changed after it was laid down: what the
// run setup says about it beside Repair.

export type ChangedInstall = {
  ref: string;
  changed?: string[] | null;
  added?: string[] | null;
  removed?: string[] | null;
  record?: string;
  local: boolean;
  repair?: string;
};

const RECORD_TEXT: Record<string, string> = {
  "install-json-changed": "Its install.json is not the one laid down.",
  "record-invalid": "Its integrity record does not verify under this computer's key.",
};

/** One line per kind of change, each naming every path. */
export function changedLines(c: ChangedInstall): string[] {
  const lines: string[] = [];
  if (c.record && RECORD_TEXT[c.record]) lines.push(RECORD_TEXT[c.record]);
  for (const [label, paths] of [["Changed", c.changed], ["Added", c.added], ["Removed", c.removed]] as const) {
    if (paths && paths.length > 0) lines.push(`${label}: ${paths.join(", ")}`);
  }
  if (lines.length === 0) lines.push("Its record no longer matches its files.");
  return lines;
}

/** What the record shows, and what it cannot. */
export const RECORD_PROVES =
  "This computer sealed the release's files when it installed them, after the signature was checked. The files no longer match that seal, so the run is refused. The seal cannot say who changed them, or whether the change is harmful.";

/** The way back: Repair for a registry release, the folder for a local one. */
export function repairLine(c: ChangedInstall): string {
  if (c.local) {
    return `It was installed from a folder, so it is laid down again from there: ${c.repair || "Build → Open folder… → Install"}.`;
  }
  return "Repair downloads the release again, checks it as any install is, and replaces the changed files. Nothing of them is kept.";
}
