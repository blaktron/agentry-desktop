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

/** The blockers a site answer raises before Run, in the setup's words. */
export function siteBlockers(qs: SiteQ[], values: Record<string, string>): string[] {
  const out: string[] = [];
  for (const q of qs) {
    const a = (values[q.id] ?? "").trim();
    if (a === "") continue; // a required one is the generic "Answer:" blocker
    const p = siteProblem(a);
    if (p) out.push(`The site for “${q.prompt}” ${p}.`);
  }
  return out;
}

/** The "what to have ready" line for a site question. */
export function siteNeed(q: SiteQ): string {
  return `The address of the site to test for “${q.prompt}”${q.required ? "" : " (optional)"}: https:// or http://, a staging server on your network included. Its browser opens it and its subdomains.`;
}
