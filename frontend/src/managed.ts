// Managed settings: the values an organization's admins set for their
// members, which Go keeps current and the runner applies. These are the
// words Settings shows for them. Nothing here decides anything: Go refuses
// Unsafe Mode under the lock, and the runner refuses a run the settings
// forbid.

/** One organization's managed settings held on this computer. */
export type ManagedHeld = { organization: string; version: number };

/** What Unsafe Mode's switch carries while managed settings refuse it. */
export type UnsafeLock = { managedBy?: string; reason: string; savedOn: boolean };

const joinAnd = (xs: string[]) => (xs.length < 2 ? xs.join("") : `${xs.slice(0, -1).join(", ")} and ${xs[xs.length - 1]}`);

const asSentence = (s: string) => (/[.!?]$/.test(s.trim()) ? s.trim() : `${s.trim()}.`);

/** The Settings landing's line: the organizations whose managed settings
 *  apply here, each with its version; null when none do. */
export function managedLine(held: ManagedHeld[] | null | undefined): string | null {
  if (!held?.length) return null;
  return `Some settings are managed by ${joinAnd(held.map((d) => `${d.organization} (version ${d.version})`))}.`;
}

/** The badge beside Unsafe Mode's switch under the lock. */
export function unsafeLockBadge(lock: UnsafeLock): string {
  return lock.managedBy ? `Managed by ${lock.managedBy}` : "Locked";
}

/** The lines under the locked switch: the reason, and what happened to a
 *  switch that was already on. */
export function unsafeLockLines(lock: UnsafeLock): string[] {
  const lines = [asSentence(lock.reason)];
  if (lock.savedOn) {
    lines.push(
      "Unsafe Mode was on here before. It stays off while these managed settings forbid it, and no run on this computer uses it. " +
        "If they stop forbidding it, it comes back on unless you keep it off.",
    );
  }
  return lines;
}

/** The Runs card's words for the switch under the lock. */
export function unsafeLockStatus(lock: UnsafeLock): string {
  return lock.managedBy ? `Unsafe Mode is off, managed by ${lock.managedBy}.` : "Unsafe Mode is off and locked.";
}
