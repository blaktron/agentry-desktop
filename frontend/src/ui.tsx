// Shared presentational pieces. The copy in here follows the product's
// honest posture: an agent is "scanned, N findings, signed" — never "safe".
import React from "react";
import { mcpview, trust } from "../wailsjs/go/models";

// The shared primitives and the brand live in their own modules; re-export
// them here so a page imports everything design-related from "../ui".
export * from "./primitives";
export { Mark, Wordmark } from "./brand";

export function Sev({ severity }: { severity: string }) {
  const s = severity.toLowerCase();
  return <span className={`sev ${s}`}>{s.toUpperCase()}</span>;
}

/** Run-mode badges (manifest v2 modes), in a fixed order. */
export function ModeBadges({ modes }: { modes?: string[] }) {
  const set = new Set(modes ?? []);
  const ordered = ["autonomous", "interactive"].filter((m) => set.has(m));
  if (ordered.length === 0) return <span className="badge kind" title="no run mode declared">no mode</span>;
  return (
    <>
      {ordered.map((m) => (
        <span key={m} className="badge kind" title={m === "autonomous" ? "runs from its files and ends" : "a chat session under the same controls"}>{m}</span>
      ))}
    </>
  );
}

export function QuarantineBadge({ state }: { state?: string }) {
  if (!state) return null;
  return <span className="badge bad">quarantined: {state}</span>;
}

/** Signed/unsigned/unverifiable badge from a trust verification result. */
export function SignedBadge({ att }: { att: trust.AttestationResult | null }) {
  if (!att) return null;
  switch (att.state) {
    case "verified":
      return <span className="badge ok" title={att.detail}>DSSE-signed · key {att.keyId}</span>;
    case "unsigned":
      return <span className="badge warn" title={att.detail}>unsigned</span>;
    case "unscanned":
      // Its own verdict (plan organization-subscriptions §4): the registry
      // admitted the version without a scan — not a missing signature, and
      // not a scan with a known gap.
      return <span className="badge unscanned" title={att.detail}>unscanned</span>;
    case "local":
      // Installed from a folder on this computer (agentry-desktop#425):
      // no pipeline scanned it and no one signed it.
      return <span className="badge warn" title="Installed from a folder on this computer">local, unsigned</span>;
    default:
      return <span className="badge bad" title={att.detail}>{att.state}</span>;
  }
}

/** Renders the declared §5.4 capability block (canonical manifest JSON) as
 * badges. A fully-undeclared block is the "pure text" case. */
export function CapabilityBadges({ caps }: { caps?: Record<string, any> }) {
  const badges: React.ReactNode[] = [];
  const c = caps ?? {};

  const egress = c.network?.egress;
  if (Array.isArray(egress) && egress.length > 0) {
    const unrestricted = egress.includes("*");
    badges.push(
      <span key="net" className={`badge ${unrestricted ? "bad" : "warn"}`}>
        network: {egress.join(", ")}{unrestricted ? " (unrestricted)" : ""}
      </span>,
    );
  }
  const fs = (dir: "read" | "write") => {
    const v = c.filesystem?.[dir];
    if (v == null || v === "none") return;
    // The manifest shape is "workspace" | "none" | a bare path array | an
    // object { paths: [...] } (write uses the object form). Render each as
    // the paths it grants — never the raw object, which used to read as
    // "[object Object]" in this security-consent badge (agentry-desktop#36).
    let text: string;
    if (Array.isArray(v)) text = `paths: ${v.join(", ")}`;
    else if (v && typeof v === "object" && Array.isArray((v as { paths?: unknown }).paths))
      text = `paths: ${((v as { paths: string[] }).paths).join(", ")}`;
    else text = String(v);
    badges.push(<span key={`fs-${dir}`} className="badge warn">fs {dir}: {text}</span>);
  };
  fs("read");
  fs("write");
  const exec = c.exec;
  if (Array.isArray(exec) && exec.length > 0) {
    badges.push(<span key="exec" className="badge bad">exec: {exec.join(", ")}</span>);
  }
  if (Array.isArray(c.secrets) && c.secrets.length > 0) {
    badges.push(<span key="sec" className="badge warn">secrets: {c.secrets.join(", ")}</span>);
  }
  if (Array.isArray(c.dataAccess) && c.dataAccess.length > 0) {
    badges.push(<span key="da" className="badge">data: {c.dataAccess.join(", ")}</span>);
  }

  if (badges.length === 0) {
    return <span className="badge ok">no capabilities declared (pure text)</span>;
  }
  return <span className="row">{badges}</span>;
}

/** Escapes-to-markup: excerpts arrive with invisible characters already
 * rendered as <U+200B>-style escapes by the scanner. Highlight them. */
export function Excerpt({ text }: { text?: string }) {
  if (!text) return null;
  const parts = text.split(/(<U\+[0-9A-Fa-f]{4,6}>)/g);
  return (
    <div className="excerpt mono">
      {parts.map((p, i) =>
        /^<U\+[0-9A-Fa-f]{4,6}>$/.test(p) ? (
          <span key={i} className="agy-invisible">{p}</span>
        ) : (
          <React.Fragment key={i}>{p}</React.Fragment>
        ),
      )}
    </div>
  );
}

/** A size in bytes, as the run folder and the Knowledge panel show one. */
export function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

/** A size in the binary units the CLI's consent and the native dialog use
 * ("4.2 MiB"), for a line of the contract a person consents to. */
export function fmtBinaryBytes(n: number): string {
  if (n < 1024) return `${n} bytes`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`;
}

/** A storage size in the units the site's knowledge lines use ("20 MB of
 * 100 MB", "5 GB"), for an allocation rather than a single file. */
export function fmtStorage(n: number): string {
  const units = ["bytes", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return i === 0 ? `${n} bytes` : `${v.toFixed(v >= 10 || Number.isInteger(v) ? 0 : 1)} ${units[i]}`;
}

/** A number of seconds the way a person says it: "15 minutes", "2 hours". */
export function fmtDuration(sec: number): string {
  if (sec % 3600 === 0) return `${sec / 3600} hour${sec === 3600 ? "" : "s"}`;
  if (sec % 60 === 0) return `${sec / 60} minute${sec === 60 ? "" : "s"}`;
  return `${sec} second${sec === 1 ? "" : "s"}`;
}

export function fmtDate(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

export function shortDigest(d?: string): string {
  if (!d) return "";
  const hex = d.replace(/^sha256:/, "");
  return `sha256:${hex.slice(0, 12)}…`;
}

/** A declared MCP server's plain env as NAME=value rows, sorted by name in
 * Go (internal/mcpview, agentry-desktop#62). env is configuration the
 * manifest declares, never a secret: secrets are names only, and a name
 * declared as a secret is left out before it gets here. Nothing when the
 * server declares no env. */
export function ServerEnv({ env }: { env?: mcpview.EnvVar[] }) {
  if (!env || env.length === 0) return null;
  return (
    <div className="server-env small" title="Plain configuration this server's declaration carries in the manifest; not a secret">
      <span className="muted">env</span>
      <ul className="mono">
        {env.map((e) => <li key={e.name}>{e.name}={e.value}</li>)}
      </ul>
    </div>
  );
}

/** The declared MCP servers of a release: pin, tools and plain env, one row
 * per server (the install review, agentry-desktop#62). */
export function DeclaredServersTable({ servers }: { servers?: mcpview.Server[] }) {
  if (!servers || servers.length === 0) return null;
  return (
    <table className="plain selectable declared-servers">
      <thead>
        <tr><th>server</th><th>tools</th><th>env</th></tr>
      </thead>
      <tbody>
        {servers.map((s) => (
          <tr key={s.id}>
            <td className="mono">{s.id}</td>
            <td className="small">{s.tools.length > 0 ? <span className="mono">{s.tools.join(", ")}</span> : <span className="muted">all of the server&apos;s tools</span>}</td>
            <td className="small">
              {s.env.length > 0
                ? <ul className="mono server-env-rows">{s.env.map((e) => <li key={e.name}>{e.name}={e.value}</li>)}</ul>
                : <span className="muted">none</span>}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
