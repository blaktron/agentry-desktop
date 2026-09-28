// The Agentry mark and wordmark, ported from the website (agentry-web
// lib/brand-mark.ts, drawn by its components/brand.tsx) so the desktop
// carries the same identity: a shield whose right half is a shade darker,
// with a delta A cut in the page ground (agentry-desktop#367), inline SVG
// taking the page's own tokens (--color-accent on --color-bg), and an
// "Agentry" wordmark. This is the generic Agentry mark, not a
// SecureAgentry-brand asset — the desktop is the open-core client. The app
// icon (build/appicon.svg) draws the same paths; brand_test.go holds them
// together.
const MARK_SHIELD = "M24 3 41 10.5 V 23 C 41 34.5 34 42.5 24 46.5 C 14 42.5 7 34.5 7 23 V 10.5 Z";
const MARK_FACET = "M24 3 41 10.5 V 23 C 41 34.5 34 42.5 24 46.5 Z";
const MARK_A = "M24 9.5 L35 38 Q29.5 32 24 32 Q18.5 32 13 38 Z M24 19.2 L27.2 27.4 H20.8 Z";

export function Mark({ size = 22, className = "" }: { size?: number; className?: string }) {
  return (
    <svg
      viewBox="0 0 48 48"
      width={size}
      height={size}
      aria-hidden="true"
      className={className}
      style={{ flex: "none" }}
    >
      <path d={MARK_SHIELD} style={{ fill: "var(--color-accent)" }} />
      <path d={MARK_FACET} style={{ fill: "#000", fillOpacity: 0.22 }} />
      <path d={MARK_A} fillRule="evenodd" style={{ fill: "var(--color-bg)" }} />
    </svg>
  );
}

/** The mark plus the product name, for the shell header and the wizard. */
export function Wordmark({ sub, size = 24 }: { sub?: string; size?: number }) {
  return (
    <span className="wordmark">
      <Mark size={size} />
      <span className="wordmark-text">
        <span className="wordmark-name">Agentry</span>
        {sub && <span className="wordmark-sub">{sub}</span>}
      </span>
    </span>
  );
}
