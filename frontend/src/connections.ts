// Connections in the run setup and Settings (plan mcp-oauth-connections.md
// M5; agentry-desktop#666): the pure parts. A connection is a person's OAuth
// grant to one declared MCP server, for one QuickAgent version, made on the
// vendor's own consent screen and kept on this computer for that version's
// later runs. Go keeps it (internal/run/connections.go); these are the words
// the page says about it.

/** One declared server that signs in by OAuth, as Go reports it
 *  (run.ServerConnection): never a token. */
export type ServerConn = {
  server: string;
  what: string;
  issuer?: string;
  scopes: string[];
  state: string;
  broken?: string;
  label?: string;
  client?: string;
  createdAt?: string;
  lastUsedAt?: string;
  unsupported?: string;
};

/** The host of an address, or the address itself where it does not parse. */
export function hostOf(address: string | undefined): string {
  if (!address) return "";
  try {
    return new URL(address).host;
  } catch {
    return address;
  }
}

/** A stamp as a date, in the person's own locale; "" for none. */
export function dateOf(stamp: string | undefined): string {
  if (!stamp) return "";
  const d = new Date(stamp);
  return Number.isNaN(d.getTime()) ? stamp : d.toLocaleDateString();
}

/** The scopes a consent asks for, in words. */
export function scopeWords(scopes: string[] | undefined): string {
  return scopes && scopes.length > 0 ? scopes.join(", ") : "the vendor's default access";
}

/** Who the vendor's consent screen named. */
export function clientWords(kind: string | undefined): string {
  switch (kind) {
    case "own":
      return "your own OAuth client";
    case "cimd":
    case "dcr":
      return "Agentry Desktop";
    default:
      return "";
  }
}

/** What a server's sign-in stands at, in one line: "Connected as …" where
 *  the vendor named the account, else connected with where and since when;
 *  a broken one says why; none says what Connect will do. */
export function connectionLine(c: ServerConn): string {
  if (c.unsupported) {
    return `This server cannot run on an access token alone, so it cannot be connected: run a version that declares ${c.unsupported} instead.`;
  }
  switch (c.state) {
    case "connected": {
      const where = c.issuer ? ` at ${hostOf(c.issuer)}` : "";
      const as = clientWords(c.client);
      const since = dateOf(c.createdAt);
      if (c.label) return `Connected as ${c.label}${where}.`;
      return `Connected${where}${as ? `, as ${as}` : ""}${since ? `, since ${since}` : ""}. The vendor did not say which account.`;
    }
    case "broken":
      return `The connection no longer works${c.broken ? ` (${c.broken})` : ""}. Connect again.`;
    default:
      return "Not connected yet. Connect opens the vendor's consent screen in your browser; this version's later runs reuse what you approve.";
  }
}

/** What still holds Run for the servers that sign in, as sentences the line
 *  above the button shows. On this computer, each server without a working
 *  connection; In Agentry, any such server at all: a run there cannot sign
 *  in to one yet (the plan's M9, agentry-desktop#667). */
export function connectionBlockers(conns: ServerConn[] | null, inAgentry: boolean): string[] {
  if (!conns || conns.length === 0) return [];
  if (inAgentry) {
    return ["A run In Agentry cannot sign in to an MCP server for you yet. Run it on this computer, where Connect tools signs in."];
  }
  const out: string[] = [];
  for (const c of conns) {
    if (c.unsupported) out.push(`${c.server} cannot run on an access token alone: run a version that declares ${c.unsupported} instead.`);
    else if (c.state !== "connected") out.push(`Connect ${c.server} on Connect tools.`);
  }
  return out;
}

/** The recap's line for the sign-ins. */
export function signInRecap(conns: ServerConn[]): string {
  const done = conns.filter((c) => c.state === "connected").length;
  if (done === conns.length) return conns.length === 1 ? "connected" : `all ${conns.length} connected`;
  return `${done} of ${conns.length} connected`;
}

/** A pasted address is the whole of what the browser's address bar held:
 *  one that is not an http(s) address with a query is refused here before
 *  Go is asked, with the reason. Go checks the sign-in's state. */
export function pastedProblem(address: string): string | null {
  const a = address.trim();
  if (a === "") return "Paste the whole address from the browser's address bar.";
  try {
    const u = new URL(a);
    if ((u.protocol !== "http:" && u.protocol !== "https:") || u.search === "") {
      return "That is not the address the browser ended on: it starts with http://127.0.0.1 and carries a code.";
    }
  } catch {
    return "That is not an address: paste the whole of what the browser's address bar shows.";
  }
  return null;
}
