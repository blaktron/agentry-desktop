// The Agent tab's second card (agentry-desktop#835): the release's
// instruction file under its README, shown as text. A module of its own,
// with no JSX, so node's test runner can import it.

/** The tab that holds the README and the instruction file. */
export const AGENT_TAB_LABEL = "Agent";

/** What BrowseAPI.Instructions answers (main.InstructionsView). */
export type Instructions = { path: string; content: string; truncated: boolean; binary: boolean };

/** The line the card shows instead of, or under, the text: null when the
 *  whole file is there to read. `failed` is a read that did not answer. */
export function instructionsNote(v: Instructions | null, failed: string | null): string | null {
  if (failed) return `The instruction file could not be read: ${failed}`;
  if (!v) return null;
  if (v.binary) return `${v.path} could not be read as text.`;
  if (!v.content.trim()) return `${v.path} is empty.`;
  if (v.truncated) return `${v.path} is longer than the registry shows; this is its beginning.`;
  return null;
}
