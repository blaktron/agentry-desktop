// Report bug's words (plan report-bug.md §6, D1, D2, D7;
// agentry-desktop#841): what the card says before Submit about the text,
// and after it about the registry's answer, from the one word the binding
// returns (services_bugreport.go). Pure, so the sentences are tested in node.

export const BUG_REPORT_MIN = 10;
export const BUG_REPORT_MAX = 4000;

/** The text as the registry counts it: trimmed, line breaks as \n, in characters. */
export function bugReportLength(text: string): number {
  return [...text.replace(/\r\n?/g, "\n").trim()].length;
}

/** Why Submit is held back, or null when the text is within bounds. */
export function bugTextProblem(text: string): string | null {
  const n = bugReportLength(text);
  if (n === 0) return null;
  if (n < BUG_REPORT_MIN) return `A bug report is at least ${BUG_REPORT_MIN} characters.`;
  if (n > BUG_REPORT_MAX) return `A bug report is at most ${BUG_REPORT_MAX.toLocaleString("en-US")} characters; this one is ${n.toLocaleString("en-US")}.`;
  return null;
}

export const BUG_REPORT_THANKS = "Thanks. Your report reached the Agentry team, and we will email you when it is fixed.";

export interface BugReportAnswer {
  filed: boolean;
  reason?: string;
  message?: string;
}

/** What the card says after Submit, and whether it offers Sign in. */
export function bugReportOutcome(r: BugReportAnswer): { tone: "ok" | "warn"; sentence: string; signIn?: true } {
  if (r.filed) return { tone: "ok", sentence: BUG_REPORT_THANKS };
  switch (r.reason) {
    case "signin":
      return { tone: "warn", sentence: "Sign in again to report a bug: this registry did not accept the sign-in this app holds.", signIn: true };
    case "absent":
      return { tone: "warn", sentence: "This registry does not take bug reports." };
    case "limited":
      return { tone: "warn", sentence: r.message || "You have sent ten bug reports in the last hour. Try again later." };
    case "text":
      return { tone: "warn", sentence: `A bug report is ${BUG_REPORT_MIN} to ${BUG_REPORT_MAX.toLocaleString("en-US")} characters.` };
    case "unreachable":
      return { tone: "warn", sentence: "The registry did not take the report just now. Try again in a moment." };
    default:
      return { tone: "warn", sentence: r.message ? `Not sent: ${r.message}` : "Not sent: the registry refused the report." };
  }
}
