// Unsafe Mode's chrome (plan unsafe-mode.md D13; agentry-desktop#418,
// D-050): while Run defaults' switch is on, a still red glow runs round the
// whole window and a badge at the foot of the sidebar says "Unsafe Mode",
// so the state is never carried by colour alone. Shared by the app and the
// design harness, so the harness shows the chrome the app has.
//
// Neither decides anything: the switch lives in desktop.yaml, which only Go
// writes after a native confirmation (D-030), and a run reads it there.

/** The red edge. One fixed layer over everything the page draws, its own
 *  dialogs included, that takes no clicks (style.css .unsafe-glow). */
export function UnsafeGlow() {
  return <div className="unsafe-glow" aria-hidden="true" />;
}

/** The sidebar's badge. A button when it can open Run defaults. */
export function UnsafeBadge({ onOpen }: { onOpen?: () => void }) {
  return (
    <button
      type="button"
      className="badge bad unsafe-badge"
      title="Unsafe Mode is on: a run on this computer can use commands this computer has not approved. Open Run defaults to turn it off."
      onClick={onOpen}
    >
      Unsafe Mode
    </button>
  );
}
