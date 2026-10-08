// The provider form's rules that are not React (agentry-desktop#724).
// Pure, so node's own test runner can hold them without a browser.

/** The base URL the form sends for provider: the field's value for the
 *  custom provider, "" for any other. The field keeps what was typed
 *  across a switch, so going back to custom finds it, but a known
 *  provider reaches its own endpoint, and ApplySetup refuses a URL for
 *  one (D-030). */
export function baseURLFor(provider: string, baseURL: string): string {
  return provider === "custom" ? baseURL.trim() : "";
}
