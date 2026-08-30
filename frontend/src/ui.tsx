// Shared presentational pieces. The copy in here follows the product's
// honest posture: an agent is "scanned, N findings, signed" — never "safe".
import React from "react";
import { trust } from "../wailsjs/go/models";

export function Spinner({ label }: { label?: string }) {
  return <div className="spinner">{label ?? "Loading…"}</div>;
}

export function ErrorBox({ error }: { error: string | null }) {
  if (!error) return null;
  return <div className="error-box selectable">{error}</div>;
}

export function Sev({ severity }: { severity: string }) {
  const s = severity.toLowerCase();
  return <span className={`sev ${s}`}>{s.toUpperCase()}</span>;
}

export function KindBadge({ kind }: { kind: string }) {
  return <span className="badge kind">{kind}</span>;
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
    const text = Array.isArray(v) ? `paths: ${v.join(", ")}` : String(v);
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
