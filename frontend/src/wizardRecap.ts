import type { main } from "../wailsjs/go/models";

// The first-run wizard's recap rules that are worth holding in a test
// (agentry-desktop#462; plan desktop-review-key-and-run-cards.md D13): the
// AI Review key's row on Done, and what Browse's "Setup isn't finished"
// notice lists after "I'll do it later". Pure, with a type-only import, so
// node's own test runner can load it.

/** One line of the recap: a check for what is true, a bullet for what is
 *  not. aside: the row is a recommendation, not something the app needs
 *  before it can sign in, run or build, so the notice never lists it. */
export type RecapRow = { ok: boolean; text: string; aside?: boolean };

/** The AI Review key's row, or null when its status could not be read (a
 *  read that fails drops its line). It says where the key comes from and
 *  never what it is (D-040). */
export function keyRow(k: main.JevKeyView | null): RecapRow | null {
  if (!k) return null;
  if (k.envSet) return { ok: true, text: `AI Review key from $${k.envName}`, aside: true };
  if (k.ready) return { ok: true, text: "AI Review key saved", aside: true };
  if (k.stored) {
    return { ok: false, text: "An AI Review key is in the keychain, but the runner policy does not read it — save it again in Settings", aside: true };
  }
  return { ok: false, text: "No AI Review key — drafting on this computer is off until you add one in Settings", aside: true };
}

/** What the notice lists: every pending row the app cannot work without. */
export function missingForNotice(rows: RecapRow[]): string[] {
  return rows.filter((r) => !r.ok && !r.aside).map((r) => r.text);
}
