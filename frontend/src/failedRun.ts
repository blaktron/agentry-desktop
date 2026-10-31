// What a run that did not finish shows, in one shape for a run on this
// computer and a run In Agentry, live or read back from the history: what
// failed, the exit class by name with its meaning, the cause, where the
// evidence is, and which actions make sense. Pure, with type-only imports,
// so node's own test runner holds it without a browser. The class's name,
// meaning and retry hint are Go's, from the CLI's exit-class table: nothing
// here keeps a copy of that table.
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";

/** The facts a failure view draws. */
export type Failure = {
  /** The QuickAgent (ns/name) or the built-in's job, and its version. */
  what: string;
  version: string;
  /** Where it ran, in the glossary's words. */
  where: string;
  /** The exit class's name, its meaning, and whether running again with the
   *  same inputs is likely to help; the name is "" when no class is known. */
  className: string;
  meaning: string;
  retryLikelyHelps: boolean;
  /** The runner's last error line, as the receipt records it; "" when the
   *  record has none. */
  cause: string;
  /** Where the evidence is: the receipts folder on this computer, the run
   *  page In Agentry, the kept log. One sentence each. */
  evidence: string[];
};

/** A run's facts as the failure view takes them, from whichever view holds
 *  them (a result view, a Recent runs row, a hosted run view). */
export type FailureInput = {
  ref?: string;
  kind?: string;
  where: "here" | "agentry";
  outcome?: string;
  status?: string;
  exitCode?: number;
  exitClass?: string;
  exitClassMeaning?: string;
  retryLikelyHelps?: boolean;
  /** The runner's error line: a result view's error, a row's cause. */
  cause?: string;
  /** The receipts folder (a run here) and the run folder, when known. */
  receiptsDir?: string;
  outDir?: string;
  /** The history keeps the run's log. */
  logKept?: boolean;
  /** The run was stopped or cancelled by the person: not a failure. */
  stopped?: boolean;
};

const BUILTIN: Record<string, string> = {
  draft: "A draft by the Drafter",
  review: "A review by the Examiner",
  knowledge: "A knowledge build",
  voice: "A Writing Voice build",
};

/** Whether the run failed: it ended, not by the person's hand, and not ok.
 *  A blocked run is a failure here too: the QuickAgent could not do the
 *  task, and the view says so in the class's words. */
export function didFail(r: Pick<FailureInput, "where" | "outcome" | "status" | "stopped">): boolean {
  if (r.stopped) return false;
  if (r.where === "agentry") {
    return !!r.outcome && r.outcome !== "succeeded" && r.outcome !== "cancelled" && r.status !== "cancelled";
  }
  const outcome = r.outcome ?? (r.status === "ok" ? "ok" : r.status ? "error" : "");
  return outcome === "error" || outcome === "blocked";
}

/** The failure's facts, or null when the run did not fail. */
export function failureOf(r: FailureInput): Failure | null {
  if (!didFail(r)) return null;
  const at = r.ref?.indexOf("@") ?? -1;
  const name = r.ref ? (at < 0 ? r.ref : r.ref.slice(0, at)) : "";
  const what = (r.kind && BUILTIN[r.kind]) || name || "A QuickAgent";
  const version = r.ref && at >= 0 ? r.ref.slice(at + 1) : "";
  const className = r.exitClass || (r.exitCode ? `exit ${r.exitCode}` : "");
  const evidence: string[] = [];
  if (r.where === "agentry") {
    evidence.push("The run page In Agentry holds the run's record and its receipt.");
    if (r.outDir) evidence.push(`Its outputs came down into ${r.outDir}.`);
  } else if (r.receiptsDir) {
    evidence.push(`The receipts are in ${r.receiptsDir}.`);
  } else if (r.outDir) {
    evidence.push(`The receipts are in the run folder, ${r.outDir}, under receipts/.`);
  } else {
    evidence.push("The run ended before it had a folder, so it left no receipts.");
  }
  if (r.logKept) evidence.push("The run's log is kept with it below.");
  return {
    what, version, where: r.where === "agentry" ? "In Agentry" : "On this computer",
    className, meaning: r.exitClassMeaning ?? "", retryLikelyHelps: !!r.retryLikelyHelps,
    cause: (r.cause ?? "").trim(), evidence,
  };
}

/** The headline of a run that failed: the class by name, and what it
 *  means. With no class known, what is known. */
export function failureHeadline(f: Pick<Failure, "className" | "meaning">): string {
  if (f.className && f.meaning) return `Failed (${f.className}): ${f.meaning.trim().replace(/\.?$/, ".")}`;
  if (f.className) return `Failed (${f.className}).`;
  return "The run did not finish.";
}

/** The line under the headline that says whether running again may help,
 *  from the class's retry hint: the class, not the person's guess. Nothing
 *  for a class the table does not know, which has no hint. */
export function retryLine(f: Pick<Failure, "retryLikelyHelps" | "className" | "meaning">): string {
  if (!f.className || !f.meaning) return "";
  return f.retryLikelyHelps
    ? "Running it again with the same answers may well succeed: this kind of failure is usually passing."
    : "Running it again with the same answers would most likely fail the same way: change what the run is given first.";
}

/** The text a bug report of the failure starts with: the facts, never the
 *  task, the answers or the result. */
export function reportText(f: Failure, runId: string): string {
  const lines = [
    `${f.what}${f.version ? ` ${f.version}` : ""} failed ${f.where === "In Agentry" ? "In Agentry" : "on this computer"}${f.className ? ` with exit class ${f.className}` : ""}.`,
  ];
  if (f.meaning) lines.push(`What that means: ${sentence(f.meaning)}`);
  if (f.cause) lines.push(`Cause: ${f.cause}`);
  if (runId) lines.push(`Run: ${runId}`);
  lines.push("", "What I was doing:", "");
  return lines.join("\n");
}

/** A phrase as a sentence: capitalised, ending in a full stop. */
export function sentence(s: string): string {
  const t = s.trim();
  if (!t) return "";
  return `${t.charAt(0).toUpperCase()}${t.slice(1)}${/[.!?]$/.test(t) ? "" : "."}`;
}

/** ns/name of a ref, without its version; "" for none. */
export function nameOf(ref?: string): string {
  if (!ref) return "";
  const at = ref.indexOf("@");
  return at < 0 ? ref : ref.slice(0, at);
}

/** What a result view says of a run on this computer, for the failure view. */
export function localFailureInput(res: app.RunResultView, stopped = false): FailureInput {
  return {
    ref: res.ref, kind: "run", where: "here", status: res.status, exitCode: res.exitCode, exitClass: res.exitClass,
    exitClassMeaning: res.exitClassMeaning, retryLikelyHelps: res.retryLikelyHelps, cause: res.error,
    receiptsDir: res.receiptsDir, outDir: res.outDir, stopped,
  };
}

/** What a Recent runs row says, for the run page's failure view. */
export function rowFailureInput(row: app.RecentRunView, folder?: string): FailureInput {
  return {
    ref: row.ref, kind: row.kind, where: row.where === "agentry" ? "agentry" : "here", outcome: row.outcome,
    exitCode: row.exitCode, exitClass: row.exitClass, exitClassMeaning: row.exitClassMeaning,
    retryLikelyHelps: row.retryLikelyHelps, cause: row.cause, outDir: folder, logKept: row.logKept,
    stopped: row.outcome === "stopped",
  };
}

/** What a run In Agentry's view says, for the failure view. */
export function hostedFailureInput(run: app.HostedRunView): FailureInput {
  return {
    ref: run.ref, kind: "run", where: "agentry", outcome: run.outcome, status: run.status, exitClass: run.exitClass,
    exitClassMeaning: run.exitClassMeaning, retryLikelyHelps: run.retryLikelyHelps, cause: run.error, outDir: run.outDir,
  };
}
