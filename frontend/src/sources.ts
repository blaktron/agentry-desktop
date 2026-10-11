// Input sources in the run setup (plan input-sources.md §5, D3, D5, D13;
// agentry-desktop#794): a QuickAgent asks for a repository or a file at a
// web address with an intake question of that type, and the setup asks for
// it in the run's own flow, with an optional access token beside it. The
// token is used by the run's fetch step alone; it can be saved for its host
// in the system keychain, where the CLI and a schedule's fire read it too.
// These are the setup's pure parts; Go checks each address with the CLI's
// own parser (RunAPI.SourceKeyStatus) and never returns a token.

/** The intake question types that ask for an input source. */
export const SOURCE_TYPES = ["repository", "url"] as const;

export function isSourceType(type: string): boolean {
  return (SOURCE_TYPES as readonly string[]).includes(type);
}

/** What Go says of an answer (main.SourceKeyView): a problem, or its host
 *  and whether a key is saved for that host. Never a token. */
export type SourceKey = {
  problem?: string;
  host?: string;
  shown?: string;
  saved: boolean;
  keychainOff: boolean;
  keyStoreErr?: string;
};

/** A repository answer is its address with the ref after "#", as the CLI's
 *  --intake takes it (Docker's build-context form). */
export function splitRepository(answer: string): { url: string; ref: string } {
  const i = answer.indexOf("#");
  return i < 0 ? { url: answer, ref: "" } : { url: answer.slice(0, i), ref: answer.slice(i + 1) };
}

export function joinRepository(url: string, ref: string): string {
  const r = ref.trim();
  return r === "" ? url : `${url}#${r}`;
}

type Q = { id: string; prompt: string; type: string; required: boolean };

/** The "what to have ready" line for a source question. */
export function sourceNeed(q: Q): string {
  const opt = q.required ? "" : " (optional)";
  return q.type === "repository"
    ? `The https address of the repository for “${q.prompt}”${opt}, and the branch, tag or commit if not the default one. A private repository needs an access token, which can be saved on this computer for its host.`
    : `The https address of the file for “${q.prompt}”${opt}. A file behind a sign-in needs an access token, which can be saved on this computer for its host.`;
}

/** The sentence beside the token field: which token the run uses. */
export function tokenLine(key: SourceKey | null, typed: string, isRepository: boolean): string {
  if (!key || key.problem || !key.host) return "";
  const host = key.host;
  if (typed.trim() !== "") {
    return key.saved
      ? `This run uses the token typed here, not the key saved for ${host}. It is not saved.`
      : `This run uses the token typed here. It is not saved unless you save it for ${host}.`;
  }
  if (key.saved) return `This run uses the key saved on this computer for ${host}.`;
  if (key.keychainOff) return `The system keychain is off on this computer (AGENTRY_NO_KEYRING), so no key is saved or read for ${host}.`;
  return isRepository
    ? `No key is saved for ${host}. A public repository needs none; a private one needs an access token.`
    : `No key is saved for ${host}. A public file needs none.`;
}

/** Where a source's token comes from, for Review and start. */
export function tokenWords(key: SourceKey | null, typed: string): string {
  if (typed.trim() !== "") return "with the access token typed for this run";
  if (key?.saved && key.host) return `with the key saved for ${key.host}`;
  return "with no access token";
}

/** A source answer as Review and start shows it. */
export function sourceRecap(q: Q, answer: string, key: SourceKey | null, typed: string): string {
  const a = answer.trim();
  if (a === "") return "no answer";
  if (q.type === "repository") {
    const { url, ref } = splitRepository(a);
    return `${key?.shown || url}, ${ref.trim() ? `at ${ref.trim()}` : "its default branch"}, ${tokenWords(key, typed)}`;
  }
  return `${key?.shown || a}, ${tokenWords(key, typed)}`;
}

/** What still holds Run for the source questions, each in a sentence. */
export function sourceBlockers(
  qs: Q[],
  values: Record<string, string>,
  keys: Record<string, SourceKey | null>,
  typed: Record<string, string>,
  at: { inAgentry: boolean; scheduling: boolean },
): string[] {
  const out: string[] = [];
  const given = qs.filter((q) => isSourceType(q.type) && (values[q.id] ?? "").trim() !== "");
  if (given.length === 0) return out;
  if (at.inAgentry) {
    // A hosted run refuses a source until agentry-enterprise#516.
    out.push("A run In Agentry does not take a repository or a file address yet: run it on this computer.");
    return out;
  }
  for (const q of given) {
    const key = keys[q.id];
    if (!key) {
      out.push(`Checking the address for “${q.prompt}”.`);
      continue;
    }
    if (key.problem) {
      out.push(`Fix the address for “${q.prompt}”: ${key.problem}.`);
      continue;
    }
    if (!at.scheduling) continue;
    const host = key.host ?? "its host";
    // A schedule's fire has no setup to type into: it reads the saved key
    // (D13), so a token typed here would be lost.
    // A schedule that runs while you are signed out has the saved key
    // sealed beside it (agentry-cli#854), so it holds nothing more.
    if ((typed[q.id] ?? "").trim() !== "") {
      out.push(`A schedule uses the key saved for ${host}: save the token for ${host}, or clear it.`);
    }
  }
  return out;
}

/** A token typed in the setup, with the host it was typed for. */
export type TypedToken = { host: string; token: string };

/** The typed tokens that still apply, by question id: each one's host is the
 *  host its question's address names now. An address edited to another host
 *  leaves its token behind, so it is neither shown nor sent there. */
export function typedTokens(typed: Record<string, TypedToken>, keys: Record<string, SourceKey | null>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [id, t] of Object.entries(typed)) {
    const host = keys[id]?.host;
    if (host && t.host === host) out[id] = t.token;
  }
  return out;
}

/** The tokens a run on this computer is given: those typed for an answered
 *  source question, by its id. */
export function tokensForRun(qs: Q[], values: Record<string, string>, typed: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const q of qs) {
    if (!isSourceType(q.type) || (values[q.id] ?? "").trim() === "") continue;
    const t = (typed[q.id] ?? "").trim();
    if (t !== "") out[q.id] = t;
  }
  return out;
}
