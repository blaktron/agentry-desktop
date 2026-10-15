// Connections in the run setup and Settings: the pure parts. A connection
// is a person's OAuth grant to one declared MCP server, for one QuickAgent
// version, made on the vendor's own consent screen and kept on this
// computer for that version's later runs. Go keeps it
// (internal/run/connections.go); these are the words the page says about
// it.

import type { main, run } from "../wailsjs/go/models";

/** One declared server that signs in by OAuth, as Go reports it
 *  (run.ServerConnection): never a token. Its fields are the generated
 *  type's, so a change in Go fails the type-check here. */
export type ServerConn = Pick<run.ServerConnection, "server" | "what" | "scopes" | "state"> &
  Partial<Pick<run.ServerConnection, "issuer" | "broken" | "label" | "client" | "createdAt" | "lastUsedAt" | "unsupported">>;

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
 *  connection. In Agentry, this computer's connections do not count
 *  (Agentry keeps its own), and which of Agentry's the account keeps is
 *  not this app's to read, so nothing holds Run there but a server no
 *  connection can serve, or a registry that keeps none (`vault` false): the
 *  start finds out what is missing, and asks. */
export function connectionBlockers(conns: ServerConn[] | null, inAgentry: boolean, vault = true): string[] {
  if (!conns || conns.length === 0) return [];
  const out: string[] = [];
  for (const c of conns) {
    if (c.unsupported) out.push(`${c.server} cannot run on an access token alone: run a version that declares ${c.unsupported} instead.`);
    else if (!inAgentry && c.state !== "connected") out.push(`Connect ${c.server} on Connect tools.`);
  }
  if (inAgentry && !vault) {
    out.push("This registry keeps no connections for runs In Agentry, and this release's servers sign in. Run it on this computer, where Connect tools signs in.");
  }
  return out;
}

/** The ask a run In Agentry makes when Agentry keeps no connection to some
 *  of its servers for this version (run create's 409 connection-missing):
 *  where to connect them, and what happens then. */
export function hostedConnectAsk(missing: string[], registry: string, waiting: boolean): string {
  const which = missing.length === 0 ? "the servers this release signs in to" : missing.length === 1 ? missing[0] : `${missing.slice(0, -1).join(", ")} and ${missing[missing.length - 1]}`;
  const where = hostOf(registry) || "the registry";
  return (
    `Agentry keeps no connection to ${which} for this version yet. Connect ${missing.length === 1 ? "it" : "them"} on Connect tools, ` +
    `which opens in your browser on ${where}, signed in as the account this app uses: each server signs you in on the vendor's own page. ` +
    (waiting
      ? "The run starts by itself once Agentry holds every connection."
      : "Then wait again, and the run starts once Agentry holds every connection.")
  );
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

/** Settings → Connections' landing line: what is kept, never a token. */
export function connectionsStatus(v: Pick<main.KeptConnectionsView, "connections" | "clients">): string {
  const n = v.connections.length;
  const c = v.clients.length;
  const conns = n === 0 ? "No connections kept" : n === 1 ? "1 connection kept" : `${n} connections kept`;
  const broken = v.connections.filter((x) => x.state === "broken").length;
  const own = c === 0 ? "no OAuth client of your own" : c === 1 ? "1 OAuth client of your own" : `${c} OAuth clients of your own`;
  return `${conns}${broken > 0 ? ` (${broken} no longer working)` : ""} · ${own}.`;
}

/** The warning a Disconnect shows when other connections were made with the
 *  same client at the same vendor: a vendor such as Google revokes the
 *  whole grant to the client, so they stop working too. Disconnect still
 *  revokes it; null when nothing else shares the grant. */
export function sharedGrantWarning(vendorHost: string, shared: string[] | undefined): string | null {
  if (!shared || shared.length === 0) return null;
  const then = shared.length === 1
    ? "The other connection made with the same client then stops working too, and asks for your consent at its next run"
    : `The ${shared.length} other connections made with the same client then stop working too, and ask for your consent at their next run`;
  return `${vendorHost || "The vendor"} may revoke everything granted to that client, not this connection alone: Google does. ${then}: ${shared.join("; ")}.`;
}
