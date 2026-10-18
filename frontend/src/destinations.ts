// What the run setup, Settings → Destinations and a run's result say of a
// destination, a place a run's result is sent. Pure and self-contained, so the
// node tests can import it: Go decides what is saved, what a send reached and
// whether a retry is on offer (DestinationsAPI, RunAPI.RunDeliverables); this
// says it. A credential is typed into a field and goes to Go; nothing here is
// ever handed one back.

export type SendFacts = {
  destination: string;
  host: string;
  onMyNetwork: boolean;
  delivered: number;
  refused: number;
  failed: number;
  /** The run's result alone was sent (a run that left no file). */
  result: boolean;
  /** The closed word of the first failure, "" when none failed. */
  error?: string;
  /** The closed word of the first file left out. */
  reason?: string;
  canRetry: boolean;
};

const REASONS: Record<string, string> = {
  destination: "the saved address no longer passes the rule for an address",
  credential: "the signing secret is missing or cannot be sent",
  address: "the host, or an address it resolves to, is not one a delivery may reach",
  unreachable: "nothing answered",
  tls: "its certificate was not accepted",
  timeout: "it did not answer in time",
  redirect: "it answered with a redirect, which is never followed",
  status: "it answered with an error",
  step: "the delivery could not be made",
  "too-large": "it is over the 10 MiB a delivery carries, so it is listed with its checksum and no content",
  changed: "the file is not the one the run recorded",
  missing: "the file is not in the run folder",
  link: "the file is a link",
  unreadable: "the file could not be read",
  "outside-reports": "the file is not in the run's reports folder",
};

/** A closed word from the receipt in a person's words; a word nobody knows is said plainly, never as the receiver's text. */
export function reasonWords(code: string | undefined): string {
  if (!code) return "";
  return REASONS[code] ?? "it did not arrive";
}

function files(n: number): string {
  return n === 1 ? "1 file" : `${n} files`;
}

/** The line under "Sent to": what reached the destination and what did not. */
export function sendLine(s: SendFacts): string {
  const parts: string[] = [];
  if (s.delivered > 0) parts.push(s.result && s.delivered === 1 ? "The result was sent." : `${files(s.delivered)} sent.`);
  if (s.refused > 0) parts.push(`${files(s.refused)} listed without content: ${reasonWords(s.reason)}.`);
  if (s.failed > 0) parts.push(`${files(s.failed)} not sent: ${reasonWords(s.error)}.`);
  return parts.join(" ") || "Nothing was sent.";
}

/** The status word beside a destination's name. */
export function sendStatus(s: Pick<SendFacts, "failed" | "delivered" | "refused">): "failed" | "partly" | "sent" {
  if (s.failed > 0) return s.delivered > 0 ? "partly" : "failed";
  return "sent";
}

/** The destination's name as a run's setup lists it: with where it goes. */
export function destinationLabel(d: { name: string; where: string; onMyNetwork: boolean }): string {
  return `${d.name} (${d.where}${d.onMyNetwork ? ", on my network" : ""})`;
}

const NAME = /^[a-z0-9][a-z0-9._-]{0,39}$/;

/** Why a name cannot name a destination, "" when it can. */
export function nameProblem(name: string): string {
  if (name === "") return "Give it a name.";
  if (!NAME.test(name)) return "Use 1 to 40 lower-case letters, digits, dots, hyphens or underscores, beginning with a letter or digit.";
  return "";
}

/** A signing secret to give the receiver: whsec_ and 32 random bytes in base64url. */
export function newSecret(random: (bytes: Uint8Array) => Uint8Array = (b) => crypto.getRandomValues(b)): string {
  const bytes = random(new Uint8Array(32));
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  const b64 = btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `whsec_${b64}`;
}

export type DestinationFacts = { hasSecret: boolean; hasAuthorization: boolean; defaultFor?: string[] };

/** The credentials line of a saved destination: whether each is kept, never what it is. */
export function credentialsLine(d: DestinationFacts): string {
  const secret = d.hasSecret ? "A signing secret is kept in your system keychain" : "No signing secret is kept, so a run cannot send to it: edit it and set one";
  const auth = d.hasAuthorization ? "; an Authorization value is kept too." : ".";
  return secret + auth;
}

/** The default-set line: the QuickAgents a destination starts selected for. */
export function defaultLine(d: Pick<DestinationFacts, "defaultFor">): string {
  const n = d.defaultFor?.length ?? 0;
  if (n === 0) return "";
  return `Selected when you run ${(d.defaultFor ?? []).join(", ")}.`;
}

/** The setup's line about the destinations its run will send to. */
export function chosenLine(names: string[]): string {
  if (names.length === 0) return "Nowhere beyond the run folder and your Deliverables folder.";
  if (names.length === 1) return `The result and each deliverable go to ${names[0]} once the run succeeds.`;
  return `The result and each deliverable go to ${names.join(", ")} once the run succeeds.`;
}

/** A problem the CLI's rule found, as a sentence: its first letter in capitals and a full stop. */
export function problemSentence(p: string): string {
  const t = p.trim();
  if (t === "") return "";
  const s = t.charAt(0).toUpperCase() + t.slice(1);
  return /[.!?]$/.test(s) ? s : s + ".";
}
