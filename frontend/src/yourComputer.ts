// What the page says about the applications on this computer a run may use
// through Agentry Desktop: each application's state line, the blockers the
// run setup's Run button reads, the class words, and a run's record. Pure,
// with type-only imports, so node's own test runner can hold it.
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as library from "../bindings/github.com/blaktron/agentry-desktop/internal/library/models";
import type * as linkservice from "../bindings/github.com/blaktron/agentry-desktop/internal/linkservice/models";

/** Where an application's set-up came from, as its card says it. */
export function fromWords(s: linkservice.AppState): string {
  return s.fromWords || (s.from === "library" ? "from Agentry's library" : "set up by you; Agentry has not vetted it");
}

/** The state line under an application's name. */
export function stateLine(s: linkservice.AppState): string {
  if (s.ready) {
    return s.port > 0
      ? `Ready: ${s.application} answers on port ${s.port}, and ${s.launcher || "its server"} is on this computer.`
      : `Ready: ${s.launcher || "its server"} is on this computer.`;
  }
  return s.missing || `${s.application} is not ready on this computer.`;
}

/** The name the run setup gives an application a release names: the
 *  person's application it resolved to, else the release's id, else "an
 *  application you choose". */
export function runAppName(a: app.RunApplication): string {
  if (a.resolved) return a.resolved.application;
  if (a.preset) return a.preset;
  return a.entry || "An application you choose";
}

/** The blockers the run setup adds for the applications a release
 *  names: one per application that is not ready, none while they are
 *  still being read (the caller says so) or for a run In Agentry, which
 *  the setup does not offer for such a release. */
export function applicationBlockers(apps: app.RunApplication[] | null, inAgentry: boolean): string[] {
  if (inAgentry || !apps) return [];
  return apps.filter((a) => !a.ready).map((a) => `${runAppName(a)} on your computer: ${a.missing || "not ready"}`);
}

/** A tool class in the person's words. */
export function classWords(cls: string): string {
  switch (cls) {
    case "read":
      return "reads";
    case "act":
      return "changes";
    case "exec":
      return "runs code or is not classed, asked first";
  }
  return cls;
}

/** How many of an application's tools run code inside it. */
export function execCount(s: linkservice.AppState): number {
  return (s.tools ?? []).filter((t) => t.class === "exec").length;
}

/** A recent run's one line: its calls by class, the exec questions and
 *  answers, and how its use of the computer ended. */
export function recentLine(r: linkservice.RecentRun): string {
  const parts: string[] = [];
  for (const s of r.servers ?? []) {
    const calls = Object.entries(s.calls ?? {}).map(([c, n]): [string, number] => [c, n ?? 0]).filter(([, n]) => n > 0);
    const total = calls.reduce((a, [, n]) => a + n, 0);
    let line = `${s.application}: ${total} ${total === 1 ? "call" : "calls"}`;
    if (calls.length > 0) line += ` (${calls.map(([c, n]) => `${n} ${c}`).join(", ")})`;
    const refused = Object.entries(s.refused ?? {}).map(([c, n]): [string, number] => [c, n ?? 0]).filter(([, n]) => n > 0);
    if (refused.length > 0) line += `; refused: ${refused.map(([why, n]) => `${n} ${why}`).join(", ")}`;
    if (s.errors > 0) line += `; ${s.errors} not answered`;
    parts.push(line);
  }
  if (r.execAsked > 0) {
    parts.push(`asked ${r.execAsked} ${r.execAsked === 1 ? "time" : "times"} to run code: ${r.execAllowed} allowed, ${r.execDeclined} declined${r.execForRun ? ", then every exec call allowed for the run" : ""}`);
  }
  const how = {
    stopped: "stopped",
    ended: "ended",
    paused: "paused",
    interrupted: "Agentry Desktop quit before the run ended",
  }[r.state] ?? "using it now";
  parts.push(how);
  return parts.join("; ");
}

/** One category of Add from the library's list, its presets by name. */
export interface LibraryGroup {
  category: string;
  presets: app.LibraryPreset[];
}

/** Add from the library's list: the presets matching the query (by name,
 *  id, category or summary, without regard to case), grouped by category,
 *  categories and applications in alphabetical order. */
export function libraryGroups(presets: app.LibraryPreset[], query: string): LibraryGroup[] {
  const q = query.trim().toLowerCase();
  const hit = (p: app.LibraryPreset) =>
    !q || [p.application, p.id, p.category, p.summary].some((s) => (s ?? "").toLowerCase().includes(q));
  const byCategory = new Map<string, app.LibraryPreset[]>();
  for (const p of presets) {
    if (!hit(p)) continue;
    const c = p.category || "Other";
    byCategory.set(c, [...(byCategory.get(c) ?? []), p]);
  }
  const name = (a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: "base" });
  return [...byCategory.entries()]
    .sort(([a], [b]) => name(a, b))
    .map(([category, ps]) => ({ category, presets: [...ps].sort((a, b) => name(a.application, b.application)) }));
}

/** The label of the button that adds the checked presets. */
export function addCheckedLabel(n: number): string {
  if (n === 0) return "Check the applications to add";
  return n === 1 ? "Add 1 application" : `Add ${n} applications`;
}

/** What the library's list says of itself: how many applications it offers
 *  here, which copy it is, and why the last read failed, if it did. */
export function libraryStatusLine(st: library.Status | undefined, offered: number): string {
  const n = offered === 1 ? "1 application" : `${offered} applications`;
  if (!st) return `${n} for this computer.`;
  const from = st.origin === "repository" ? "updated from Agentry's repository" : "the copy that came with this version of Agentry Desktop";
  let line = `${n} for this computer, from library version ${st.version}, ${from}.`;
  if (st.error) line += ` The newest library could not be read just now: ${st.error}`;
  return line;
}
