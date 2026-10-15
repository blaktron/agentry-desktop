// The provider form's rules that are not React. Pure, so node's own test runner
// can hold them without a browser.

/** Whether provider takes a base URL: the custom provider alone; every
 *  other reaches its own endpoint. The form shows the field for it, and
 *  only for it does the URL travel. */
export function takesBaseURL(provider: string): boolean {
  return provider === "custom";
}

/** The base URL the form sends for provider: the field's value for the
 *  custom provider, "" for any other. The field keeps what was typed
 *  across a switch, so going back to custom finds it, but a known
 *  provider reaches its own endpoint, and ApplySetup refuses a URL for
 *  one. */
export function baseURLFor(provider: string, baseURL: string): string {
  return takesBaseURL(provider) ? baseURL.trim() : "";
}

/** The tile a key moves the provider tiles to, or
 *  null for a key they leave alone. The tiles are one radio group, so the
 *  arrows walk them in order and wrap, as a radio group's do, whatever row
 *  a tile sits on; Home and End go to the ends. With no tile chosen (a
 *  provider only a hand-written policy names) an arrow forward starts at
 *  the first and one back at the last. */
export function tileAfterKey(ids: string[], current: string, key: string): string | null {
  if (ids.length === 0) return null;
  const at = ids.indexOf(current);
  switch (key) {
    case "ArrowRight":
    case "ArrowDown":
      return ids[at < 0 ? 0 : (at + 1) % ids.length];
    case "ArrowLeft":
    case "ArrowUp":
      return ids[at < 0 ? ids.length - 1 : (at - 1 + ids.length) % ids.length];
    case "Home":
      return ids[0];
    case "End":
      return ids[ids.length - 1];
    default:
      return null;
  }
}
