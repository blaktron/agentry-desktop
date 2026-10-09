// A remote MCP server and a package server's OAuth sign-in in the Builder
// (OAuth connections M11, agentry-desktop#668; the site's half is
// agentry-web#1183). The Go model (internal/build MCPServerRow.Declared)
// decides what is written; these are the page's readings of the same rules,
// pure, so node tests them without the app.

/** The parts of a server row these rules read. */
export interface ServerRow {
  source?: string;
  package?: string;
  version?: string;
  url?: string;
  auth?: string;
  scopes?: string[];
  tools?: string[];
  secrets?: string[];
  egress?: string[];
  env?: { name: string }[];
  oauth?: { issuer?: string; scopes?: string[] } | null;
}

/** A list's non-empty entries, trimmed. */
export const names = (xs?: string[]) => (xs ?? []).map((x) => x.trim()).filter(Boolean);

/** What a sign-in with no issuer yet says in its place (internal/build issuerNotGiven). */
export const ISSUER_NOT_GIVEN = "an issuer not given yet";

/** Whether the row declares a remote server, by its address. */
export const isRemote = (s: ServerRow) => s.source === "url";

/** A server's reference: source:package@version, or a remote server's address. */
export function serverRef(s: ServerRow): string {
  return isRemote(s) ? (s.url ?? "").trim() : `${s.source}:${(s.package ?? "").trim()}@${(s.version ?? "").trim()}`;
}

/** The keys a server sends: a package's, or a remote server's one key when it signs in with a key. */
export function serverKeys(s: ServerRow): string[] {
  if (!isRemote(s)) return names(s.secrets);
  return s.auth === "secret" ? names(s.secrets) : [];
}

/** The scopes a remote server's OAuth consent asks for; none for any other sign-in. */
export function remoteScopes(s: ServerRow): string[] {
  return isRemote(s) && s.auth === "oauth" ? names(s.scopes) : [];
}

/** The hosts a server reaches: a package's egress, or a remote server's own host. */
export function serverHosts(s: ServerRow): string[] {
  if (!isRemote(s)) return names(s.egress);
  try {
    const h = new URL((s.url ?? "").trim()).hostname;
    return h ? [h] : [];
  } catch {
    return [];
  }
}

/** How a server signs in, as the reach rows say it (the site's words); "" for a package with none. */
export function signInPhrase(s: ServerRow): string {
  const scopes = (list: string[]) => (list.length ? `scopes ${list.join(", ")}` : "the vendor's default scopes");
  if (isRemote(s)) {
    if (s.auth === "oauth") return `signs in with OAuth, asking for ${scopes(remoteScopes(s))}`;
    if (s.auth === "secret") return `signs in with the key ${serverKeys(s).join(", ") || "not named yet"}`;
    return s.auth === "none" ? "signs in with nothing" : "";
  }
  if (!s.oauth) return "";
  return `signs in with OAuth at ${(s.oauth.issuer ?? "").trim() || ISSUER_NOT_GIVEN}, asking for ${scopes(names(s.oauth.scopes))}`;
}

/** A server's row in "Everywhere it and its servers can reach". */
export function reachRow(s: ServerRow): { what: string; value: string } {
  const remote = isRemote(s);
  const what = remote ? (s.url ?? "").trim() || "a remote server, no address yet" : `${s.source}:${(s.package ?? "").trim() || "?"}@${(s.version ?? "").trim() || "?"}`;
  const hosts = serverHosts(s);
  const keys = serverKeys(s);
  const tools = names(s.tools);
  const env = remote ? [] : (s.env ?? []).map((e) => e.name.trim()).filter(Boolean);
  return {
    what,
    value: [
      remote ? `runs at its vendor, reached at ${hosts.join(", ") || "its address"}` : hosts.length ? `reaches ${hosts.join(", ")}` : "reaches no host",
      tools.length ? `tools ${tools.join(", ")}` : "all its tools",
      keys.length ? `keys ${keys.join(", ")}` : "",
      env.length ? `configuration ${env.join(", ")}` : "",
      signInPhrase(s),
    ].filter(Boolean).join("; "),
  };
}

/** What Review says of a remote server instead of the registry's check: there is no package to look up. */
export const REMOTE_CHECK =
  "It runs at its vendor, so there is no package to check. Agentry checks its address and what it asks for when you submit, not the vendor's code.";
