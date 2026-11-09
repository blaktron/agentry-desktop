// What the page says about the applications on this computer a run may use
// through Agentry Desktop: each application's state line, the blockers the
// run setup's Run button reads, the class words, and a run's record. Pure,
// with type-only imports, so node's own test runner can hold it.
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
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
