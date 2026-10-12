// Target hosts in the run setup (plan runtime-hosts.md §6, §8, D6, D7,
// D12, D14; agentry-desktop#812): a QuickAgent that declares a target host
// asks for a server with an intake question of type host, answered as
// [user@]host[:port]. Beside the address the setup takes the sign-in (a
// private key file chosen in the native picker, with its passphrase, or the
// account's password) and an optional sudo password, or uses the sign-in
// saved on this computer for that account and host, in the system keychain
// where the CLI and a schedule's fire read it too. The host key is checked
// when the run starts, and one no known_hosts file holds is put to the
// person in a native dialog. These are the setup's pure parts; Go reads
// each address with the CLI's own parser (RunAPI.TargetHostStatus) and
// never returns a sign-in.

/** The intake question type that asks for a target host. */
export const HOST_TYPE = "host";

export function isHostType(type: string): boolean {
  return type === HOST_TYPE;
}

/** What Go says of an answer (main.TargetHostView): a problem, or the
 *  account and host it names and whether a sign-in is saved for them.
 *  Never a credential. */
export type TargetHost = {
  problem?: string;
  answer?: string;
  user?: string;
  host?: string;
  port?: number;
  account?: string;
  saved: boolean;
  keychainOff: boolean;
  keyStoreErr?: string;
};

/** How the run signs in: the saved sign-in, a key file, or a password. */
export type SignInWay = "saved" | "key" | "password";

/** The sign-in typed in the setup for one host question, with the
 *  keychain account (user@host:port) it was typed for: an address edited to
 *  another account leaves it behind, neither shown nor sent there. */
export type HostForm = {
  account: string;
  way: SignInWay;
  keyFile: string;
  passphrase: string;
  password: string;
  sudo: string;
  save: boolean;
};

export function emptyHostForm(account: string, saved: boolean): HostForm {
  return { account, way: saved ? "saved" : "key", keyFile: "", passphrase: "", password: "", sudo: "", save: false };
}

/** The form that applies to a question now: the one typed for the account
 *  its answer names, else a fresh one. */
export function formFor(forms: Record<string, HostForm>, id: string, key: TargetHost | null): HostForm | null {
  if (!key || key.problem || !key.account) return null;
  const f = forms[id];
  return f && f.account === key.account ? f : emptyHostForm(key.account, key.saved);
}

/** What the sign-in form gives: a way to sign in, or none yet. */
export function givesSignIn(form: HostForm, key: TargetHost): boolean {
  switch (form.way) {
    case "saved":
      return key.saved;
    case "key":
      return form.keyFile.trim() !== "";
    case "password":
      return form.password !== "";
  }
}

type Q = { id: string; prompt: string; type: string; required: boolean };

/** The "what to have ready" line for a host question. */
export function hostNeed(q: Q): string {
  const opt = q.required ? "" : " (optional)";
  return `The server for “${q.prompt}”${opt}, as user@host (with :port when SSH is not on 22), and a way to sign in to it: a private key file, or the account's password; and its sudo password if the work needs sudo. A sign-in can be saved on this computer. It runs on this computer only.`;
}

/** The address as the setup says it: user@host[:port]. */
function shown(key: TargetHost): string {
  return key.answer || key.host || "the host";
}

/** Where the sign-in comes from, for the line under the form and for Review. */
export function signInWords(key: TargetHost | null, form: HostForm | null, scheduling = false): string {
  if (!key || key.problem || !form) return "";
  const kept = scheduling ? ", kept in the system keychain for the schedule's runs" : form.save ? ", saved on this computer when the run starts" : "";
  let how: string;
  switch (form.way) {
    case "saved":
      how = key.saved ? "with the sign-in saved on this computer" : "with no sign-in yet";
      break;
    case "key":
      how = form.keyFile.trim() === "" ? "with no key file chosen yet"
        : `with the key file ${form.keyFile.trim()}${form.passphrase ? " and its passphrase" : ""}${kept}`;
      break;
    case "password":
      how = form.password === "" ? "with no password typed yet" : `with the password typed here${kept}`;
      break;
  }
  const sudo = form.sudo !== "" ? "; a sudo password is given" : form.way === "saved" && key.saved ? "" : "; no sudo password";
  return how + sudo;
}

/** A host answer as Review and start shows it. */
export function hostRecap(answer: string, key: TargetHost | null, form: HostForm | null, scheduling = false): string {
  if (answer.trim() === "") return "no answer";
  if (!key) return `${answer.trim()} (being checked)`;
  if (key.problem) return `${answer.trim()}: ${key.problem}`;
  return `${shown(key)}, ${signInWords(key, form, scheduling)}`;
}

/** What still holds Run for the host questions, each in a sentence. */
export function hostBlockers(
  qs: Q[],
  values: Record<string, string>,
  keys: Record<string, TargetHost | null>,
  forms: Record<string, HostForm>,
  at: { inAgentry: boolean; scheduling: boolean },
): string[] {
  const out: string[] = [];
  const hostQs = qs.filter((q) => isHostType(q.type));
  if (hostQs.length > 0 && at.inAgentry) {
    out.push("A run In Agentry never reaches a server you name: run it on this computer.");
    return out;
  }
  for (const q of hostQs) {
    if ((values[q.id] ?? "").trim() === "") {
      // The run reaches every target host the release declares, so one
      // left empty refuses it, required or not (the engine's rule).
      if (!q.required) out.push(`Answer: ${q.prompt}. A run reaches every server this QuickAgent asks for.`);
      continue;
    }
    const key = keys[q.id];
    if (!key) {
      out.push(`Checking the address for “${q.prompt}”.`);
      continue;
    }
    if (key.problem) {
      out.push(`Fix the address for “${q.prompt}”: ${key.problem}.`);
      continue;
    }
    const form = formFor(forms, q.id, key);
    if (form && !givesSignIn(form, key)) {
      out.push(`Give a way to sign in to ${shown(key)} for “${q.prompt}”: choose a private key file or type the account's password.`);
      continue;
    }
    // A schedule's runs read the sign-in from the keychain, where making
    // the schedule keeps it (plan D12).
    if (at.scheduling && key.keychainOff) {
      out.push(`A schedule's runs read the sign-in for ${shown(key)} from the system keychain, which is off on this computer (AGENTRY_NO_KEYRING).`);
    }
  }
  return out;
}

/** A sign-in as Go takes it (run.HostSignIn): sent inward with the run,
 *  never returned. */
export type HostSignIn = { keyFile?: string; passphrase?: string; password?: string; sudo?: string; save?: boolean };

/** The sign-ins a run on this computer is given, by question id: each
 *  answered host question's, for the account its answer names now. The
 *  saved sign-in sends nothing but a sudo password and the save, if any. */
export function signInsForRun(
  qs: Q[],
  values: Record<string, string>,
  keys: Record<string, TargetHost | null>,
  forms: Record<string, HostForm>,
): Record<string, HostSignIn> {
  const out: Record<string, HostSignIn> = {};
  for (const q of qs) {
    if (!isHostType(q.type) || (values[q.id] ?? "").trim() === "") continue;
    const key = keys[q.id];
    const form = formFor(forms, q.id, key);
    if (!key || !form) continue;
    const s: HostSignIn = {};
    if (form.way === "key" && form.keyFile.trim() !== "") {
      s.keyFile = form.keyFile.trim();
      if (form.passphrase !== "") s.passphrase = form.passphrase;
    } else if (form.way === "password" && form.password !== "") {
      s.password = form.password;
    }
    if (form.sudo !== "") s.sudo = form.sudo;
    if (form.save && !key.keychainOff) s.save = true;
    if (Object.keys(s).length > 0) out[q.id] = s;
  }
  return out;
}

/** The recent hosts to offer under an address field: those that are not
 *  the answer already, newest first. */
export function recentOffers(recent: string[], answer: string, max = 5): string[] {
  const a = answer.trim();
  return recent.filter((h) => h !== a).slice(0, max);
}
