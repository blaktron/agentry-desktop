// Unsafe Mode's chrome: while Run defaults' switch is on, a still red glow runs
// round the whole window and a badge at the foot of the sidebar says "Unsafe
// Mode", so the state is never carried by colour alone. Shared by the app and
// the design harness, so the harness shows the chrome the app has.
//
// Neither decides anything: the switch lives in desktop.yaml, which only Go
// writes after a native confirmation, and a run reads it there. Both
// also stay while a run or chat compiled in Unsafe Mode is live after the
// switch went off: Go says when it ends.

/** The red edge. One fixed layer over everything the page draws, its own
 *  dialogs included, that takes no clicks (style.css .unsafe-glow). */
export function UnsafeGlow() {
  return <div className="unsafe-glow" aria-hidden="true" />;
}

/** The sidebar's badge. A button when it can open Run defaults. running:
 *  the switch is off, and a run or chat started while it was on is still
 *  live in Unsafe Mode. */
export function UnsafeBadge({ onOpen, running }: { onOpen?: () => void; running?: boolean }) {
  return (
    <button
      type="button"
      className="badge bad unsafe-badge"
      title={
        running
          ? "Unsafe Mode is off for the next run, but the run or chat that started while it was on keeps it until it ends. Settings → Runs has the switch."
          : "Unsafe Mode is on: a run on this computer can use commands this computer has not approved. Settings → Runs has the switch."
      }
      onClick={onOpen}
    >
      {running ? "Unsafe Mode: running" : "Unsafe Mode"}
    </button>
  );
}

/** A padlock, beside the words that say what is locked; it carries no
 *  meaning of its own. */
export function LockGlyph() {
  return (
    <svg className="lock-glyph" width="11" height="12" viewBox="0 0 11 12" aria-hidden="true" focusable="false">
      <rect x="1" y="5" width="9" height="6.5" rx="1.5" fill="currentColor" />
      <path d="M3 5V3.5a2.5 2.5 0 0 1 5 0V5" fill="none" stroke="currentColor" strokeWidth="1.4" />
    </svg>
  );
}
