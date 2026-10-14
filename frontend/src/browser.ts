// The browser (capabilities.browser, V15; plan browser-testing.md §5, D9;
// agentry-desktop#878): a headless Chromium a run drives, held to the sites
// the person names and the declared hosts, or any public site. These are
// the CLI's install consent words (agentry-cli cmd/agentry/caps.go
// browserLine), as internal/mcpview/browser.go has them for the consent
// dialog and the run setup's capability lines, and as the site's consent
// row has them (agentry-web#1644).

export type BrowserCap = { sites?: string[]; hosts?: string[]; scope?: string; secrets?: string[] };

// browserCap reads the line from the registry's capability JSON or the
// install snapshot, or null when the release declares none. A shape this
// build does not know reads as a browser that is declared, never as none,
// so the badge and the review still show it.
export function browserCap(v: unknown): BrowserCap | null {
  if (v === undefined || v === null || v === "") return null;
  if (typeof v !== "object" || Array.isArray(v)) return { sites: ["declared"] };
  const o = v as Record<string, unknown>;
  const names = (x: unknown): string[] =>
    Array.isArray(x) ? x.map((n) => (typeof n === "string" ? n : "declared")).filter((n) => n !== "") : x ? ["declared"] : [];
  const scope = o.scope === undefined || o.scope === null || o.scope === "" || o.scope === "site" ? "site" : "public";
  return { sites: names(o.sites), hosts: names(o.hosts), scope, secrets: names(o.secrets) };
}

// browserReach is where it may go, the CLI's words joined with ", and ".
export function browserReach(b: BrowserCap): string {
  const where: string[] = [];
  if (b.sites?.length) where.push(`the site you name for ${b.sites.join(", ")} and its subdomains`);
  if (b.hosts?.length) where.push(b.hosts.join(", "));
  if (b.scope === "public") where.push("ANY public site (policy-gated)");
  return where.length ? where.join(", and ") : "declared";
}

// browserLine is the CLI's consent line, word for word.
export function browserLine(b: BrowserCap): string {
  const typed = b.secrets?.length ? `; it can type ${b.secrets.join(", ")} into the site's sign-in forms (it never sees them)` : "";
  return `a web browser that can open ${browserReach(b)}${typed}`;
}

// browserBadge is the agent page's and the Library's short label: the site
// questions and the hosts, or any public site.
export function browserBadge(b: BrowserCap): string {
  if (b.scope === "public") return "browser: any public site";
  const named = [...(b.sites ?? []), ...(b.hosts ?? [])];
  return `browser: ${named.length ? named.join(", ") : "declared"}`;
}

// --- The browser's site in the run setup ---
//
// A url question the browser names as a site (the intake view's `site`)
// is the address the browser opens, never an input source fetched with a
// token: the CLI keeps the answer in intake.json and hands it to the
// browse proxy (agentry-cli#915, intake.CollectRun). The setup asks it as
// an address and holds it to what the CLI's browser.ParseSite takes.

type SiteQ = { id: string; prompt: string; type: string; required: boolean; site?: boolean };

/** Whether an intake question is the browser's site. */
export function isSiteQuestion(q: { type: string; site?: boolean }): boolean {
  return q.type === "url" && q.site === true;
}

// siteProblem is what the CLI's browser.ParseSite would refuse in an
// answer, as a clause, or null: an http or https address with a host, no
// user name or password, no fragment, at most 2,048 characters. A private
// address is taken, as a run on this computer may test a staging server on
// the person's network (plan D2).
export function siteProblem(answer: string): string | null {
  const a = answer.trim();
  if (a.length > 2048) return "is longer than 2,048 characters";
  let u: URL;
  try {
    u = new URL(a);
  } catch {
    return "is not a web address. It starts with https:// or http://";
  }
  if (u.protocol !== "http:" && u.protocol !== "https:") return "must start with https:// or http://";
  if (u.hostname === "") return "names no host";
  if (u.username !== "" || u.password !== "") return "carries a user name or password. Give the address alone; a sign-in is one of the QuickAgent's keys";
  if (a.includes("#")) return "carries a # part";
  return null;
}

// LOCAL_ONLY_SUFFIXES are names a public resolver never answers for a site
// on the internet: the registry's list (agentry-enterprise#565
// internal/runs/browser.go localOnlySuffixes) and the site's
// (agentry-web lib/run-setup.ts); a change there is a change here.
const LOCAL_ONLY_SUFFIXES = [".localhost", ".local", ".internal", ".home.arpa"];

// publicIPv4 says an IPv4 literal is on the public internet: none of the
// IPv4 ranges the runner's one public list refuses (agentry-cli
// runnerimage/netaddr, which the registry's browser-site check uses).
function publicIPv4(host: string): boolean {
  const [a, b, c] = host.split(".").map(Number);
  if (a === 0 || a === 10 || a === 127 || a >= 224) return false;
  if (a === 100 && b >= 64 && b <= 127) return false;
  if (a === 169 && b === 254) return false;
  if (a === 172 && b >= 16 && b <= 31) return false;
  if (a === 192 && b === 168) return false;
  if (a === 192 && b === 0 && (c === 0 || c === 2)) return false;
  if (a === 198 && (b === 18 || b === 19)) return false;
  if (a === 198 && b === 51 && c === 100) return false;
  if (a === 203 && b === 0 && c === 113) return false;
  return true;
}

// hostedSiteProblem is what a run In Agentry cannot open in a site answer,
// as a clause, or null: a port other than 80 and 443, which is all a
// sandbox host lets out, an address that is not public, or a name only a
// local network answers (plan D2: a private address is a run on this
// computer's alone; agentry-desktop#903). The registry refuses the same at
// create (409 browser-site), and the run's CLI resolves a name and refuses
// a private answer before anything starts.
export function hostedSiteProblem(answer: string): string | null {
  let u: URL;
  try {
    u = new URL(answer.trim());
  } catch {
    return null; // siteProblem says it
  }
  if (u.port !== "" && u.port !== "80" && u.port !== "443") return `names port ${u.port}, and a run In Agentry opens sites on ports 80 and 443 only`;
  const host = u.hostname.toLowerCase().replace(/\.$/, "");
  const local = ", and a run In Agentry opens public sites only: run it on this computer to test a site on your network";
  if (host.startsWith("[")) {
    const v6 = host.slice(1, -1);
    if (v6.startsWith("::") || /^64:ff9b:/.test(v6) || /^2002:/.test(v6) || /^f[c-d]/.test(v6) || /^fe[89ab]/.test(v6) || /^ff/.test(v6)) return `is not a public address${local}`;
    return null;
  }
  if (/^\d+\.\d+\.\d+\.\d+$/.test(host)) return publicIPv4(host) ? null : `is not a public address${local}`;
  if (!host.includes(".") || LOCAL_ONLY_SUFFIXES.some((s) => host.endsWith(s))) return `names a host only a local network answers${local}`;
  return null;
}

/** The blockers a site answer raises before Run, in the setup's words; In
 *  Agentry also what no sandbox host can open (hostedSiteProblem). */
export function siteBlockers(qs: SiteQ[], values: Record<string, string>, inAgentry = false): string[] {
  const out: string[] = [];
  for (const q of qs) {
    const a = (values[q.id] ?? "").trim();
    if (a === "") continue; // a required one is the generic "Answer:" blocker
    const p = siteProblem(a) ?? (inAgentry ? hostedSiteProblem(a) : null);
    if (p) out.push(`The site for “${q.prompt}” ${p}.`);
  }
  return out;
}

/** The "what to have ready" line for a site question. */
export function siteNeed(q: SiteQ): string {
  return `The address of the site to test for “${q.prompt}”${q.required ? "" : " (optional)"}: https:// or http://, a staging server on your network included. Its browser opens it and its subdomains.`;
}
