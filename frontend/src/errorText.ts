// What an error box says of what it was given: Go's error text, or a
// rejected promise's, read as one sentence a person can act on. Pure, so
// the tests hold it in node.

/** The cause as the box shows it: the text alone, with a thrown Error's
 *  class name taken off the front, a capital first letter and a full
 *  stop. An empty error is "". */
export function causeOf(error: unknown): string {
  let text = error instanceof Error ? error.message : String(error ?? "");
  text = text.replace(/^(Runtime)?Error:\s*/, "").trim();
  if (!text) return "";
  text = `${text.charAt(0).toUpperCase()}${text.slice(1)}`;
  return /[.!?…)]$/.test(text) ? text : `${text}.`;
}
