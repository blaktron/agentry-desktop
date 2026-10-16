// A secret intake question In Agentry. The registry keeps a hosted run's
// answers with the run, shows them on its page and replays them on a rerun
// and on every fire of a schedule, so it never takes a secret as an answer
// (it refuses one, 422 naming the question). A hosted run takes a secret
// question's value from the account's secrets instead, under the name the
// CLI gives it, where the release declares that secret. On this computer a
// secret answer is injected like a declared key and never shown to the
// agent, so it is asked as before.

export type IntakeQuestion = { id: string; type: string; prompt?: string };

/** The secret that carries a secret question's value: its id, upper-cased. */
export function secretName(id: string): string {
  return id.toUpperCase();
}

/** Whether a question is asked here: every one on this computer, every one
 *  but a secret In Agentry. */
export function askedHere(q: IntakeQuestion, inAgentry: boolean): boolean {
  return !(inAgentry && q.type === "secret");
}

/** The answers a run sends: every non-empty one, less a secret question's
 *  In Agentry. */
export function intakeToSend(values: Record<string, string>, intake: IntakeQuestion[], inAgentry: boolean): Record<string, string> {
  const secret = new Set(inAgentry ? intake.filter((q) => q.type === "secret").map((q) => q.id) : []);
  const answers: Record<string, string> = {};
  for (const [k, v] of Object.entries(values)) if (v.trim() !== "" && !secret.has(k)) answers[k] = v;
  return answers;
}

/** What the details step says In Agentry in place of a secret question's
 *  field: where the run takes the value from, or that it does not. */
export function secretQuestionLine(q: IntakeQuestion, declaredSecrets: string[]): string {
  const name = secretName(q.id);
  const what = q.prompt?.trim() || q.id;
  return declaredSecrets.includes(name)
    ? `${what}: a secret, so a run In Agentry takes it from your account's secrets there (${name}), never as an answer here.`
    : `${what}: a secret, which a run In Agentry does not take. It is answered only when the run is on this computer.`;
}

/** A refusal of an answer to a secret question, framed: a request, or a
 *  schedule's kept intake, that answered one. null for any other refusal. */
export function secretAnswerRefusal(
  v: { refusal?: string; question?: string; detail?: string },
  intake: IntakeQuestion[] = [],
): { heading: string; detail: string } | null {
  if (v.refusal !== "validation" || !v.question) return null;
  const q = intake.find((q) => q.id === v.question);
  const what = q?.prompt?.trim() || v.question;
  return {
    heading: `“${what}” was answered, and it is a secret, which a run In Agentry never takes as an answer.`,
    detail: (v.detail ?? "").replace(/^Validation failed:\s*/, ""),
  };
}
