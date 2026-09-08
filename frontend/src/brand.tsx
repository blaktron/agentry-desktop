// The Agentry mark and wordmark, ported from the website (agentry-web
// components/brand.tsx) so the desktop carries the same identity: a shield
// with a check, inline SVG taking the page's own tokens (--color-accent on
// --color-bg), and an "Agentry" wordmark. This is the generic Agentry mark,
// not a SecureAgentry-brand asset — the desktop is the open-core client.
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
      <path
        d="M24 3 41 10.5 V 23 C 41 34.5 34 42.5 24 46.5 C 14 42.5 7 34.5 7 23 V 10.5 Z"
        style={{ fill: "var(--color-accent)" }}
      />
      <path
        d="M15.5 24.5 L 21.5 30.5 L 32.5 18.5"
        style={{ fill: "none", stroke: "var(--color-bg)", strokeWidth: 4, strokeLinecap: "round", strokeLinejoin: "round" }}
      />
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
