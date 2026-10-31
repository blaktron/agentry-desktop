// What the app shows of a registry's sign-in: signed in while the app holds
// a token the registry accepts, signed out when it holds none, and signed
// out with the reason when the registry answered 401 to the token it holds
// (the settings view's tokenRejected). Pure, so the tests hold it in node.
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";

export type SignInState = "signed-in" | "signed-out" | "rejected";

type Registry = Pick<app.RegistryView, "url" | "hasToken" | "tokenRejected">;

/** The registry's state as the page shows it. A registry the settings do
 *  not list is signed out. */
export function signInState(registries: Registry[], url: string): SignInState {
  const r = registries.find((x) => x.url === url);
  if (!r) return "signed-out";
  if (r.tokenRejected) return "rejected";
  return r.hasToken ? "signed-in" : "signed-out";
}

/** The sentence a rejected sign-in shows beside its Sign in button. */
export function rejectedLine(host: string): string {
  return `${host} no longer accepts this app's sign-in: it was revoked, or it expired. Sign in again to use your account here.`;
}
