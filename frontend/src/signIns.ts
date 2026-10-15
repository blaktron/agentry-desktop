// The app's two sign-ins, as the screens name them: one for the SecureAgentry
// registry this build is for, and one for a self-hosted registry the person
// adds. Each is sent only to its own registry; Go decides (settings.SignInFor
// and Admit), and these only say it. Pure, so node's test runner holds it.

export type SignInKind = "secureagentry" | "self-hosted" | "";

// The parts of app.SettingsView these read.
export interface SignInSettings {
  secureAgentryRegistry: string;
  selfHostedRegistry: string;
  registries: { url: string; signIn: string }[];
}

// origin is a URL's scheme, host and port as a browser compares them
// (lower-cased, default port dropped, a non-ASCII name in punycode), ""
// for anything that is not one.
export function origin(url: string): string {
  try {
    const u = new URL(url.trim());
    return u.host ? `${u.protocol}//${u.host.replace(/\.(?=:|$)/, "")}` : "";
  } catch {
    return "";
  }
}

const sameOrigin = (a: string, b: string) => {
  const o = origin(a);
  return o !== "" && o === origin(b);
};

// usable says whether Settings would take the address at all: https, or
// http on this machine only (settings.ValidateURL).
function usable(url: string): boolean {
  const o = origin(url);
  if (!o) return false;
  const u = new URL(o);
  if (u.protocol === "https:") return true;
  if (u.protocol !== "http:") return false;
  const h = u.hostname;
  return h === "localhost" || h.endsWith(".localhost") || /^127\./.test(h) || h === "[::1]";
}

// isSecureAgentry: secureagentry.net or any subdomain of it.
export function isSecureAgentry(url: string): boolean {
  const o = origin(url);
  if (!o) return false;
  const host = new URL(o).hostname;
  return host === "secureagentry.net" || host.endsWith(".secureagentry.net");
}

// signInKindFor is the sign-in a registry address would have: the one Go
// gave a listed registry, else what its origin says. "" when the app keeps
// no sign-in for it.
export function signInKindFor(s: SignInSettings, url: string): SignInKind {
  if (!usable(url)) return "";
  const listed = s.registries.find((r) => sameOrigin(r.url, url));
  if (listed) return listed.signIn as SignInKind;
  if (s.secureAgentryRegistry && sameOrigin(url, s.secureAgentryRegistry)) return "secureagentry";
  if (isSecureAgentry(url)) return "";
  if (!s.selfHostedRegistry || sameOrigin(url, s.selfHostedRegistry)) return origin(url) ? "self-hosted" : "";
  return "";
}

// asSentence shows a refusal Go wrote as an error (lower-case, no full stop)
// as a sentence on its own line.
export function asSentence(s: string): string {
  const t = s.trim();
  if (!t) return "";
  return t.charAt(0).toUpperCase() + t.slice(1) + (/[.!?]$/.test(t) ? "" : ".");
}

// signInPhrase names the sign-in inside a sentence.
export function signInPhrase(kind: SignInKind): string {
  switch (kind) {
    case "secureagentry": return "your SecureAgentry sign-in";
    case "self-hosted": return "your self-hosted sign-in";
    default: return "no sign-in";
  }
}

// signInLabel is the badge on a registry's card.
export function signInLabel(kind: SignInKind): string {
  switch (kind) {
    case "secureagentry": return "SecureAgentry sign-in";
    case "self-hosted": return "Self-hosted sign-in";
    default: return "No sign-in";
  }
}

const hostOf = (url: string) => url.replace(/^https?:\/\//, "");

// twoSignIns is the sentence over the registries: which two sign-ins this
// app keeps, and where each goes.
export function twoSignIns(s: SignInSettings): string {
  const self = s.selfHostedRegistry
    ? `one for your self-hosted registry, ${hostOf(s.selfHostedRegistry)}`
    : "one for a self-hosted registry you add";
  if (!s.secureAgentryRegistry) {
    return `This build of Agentry Desktop signs in to no SecureAgentry registry. It keeps ${self}, and sends it only there.`;
  }
  return `Agentry Desktop keeps two sign-ins: one for SecureAgentry at ${hostOf(s.secureAgentryRegistry)}, and ${self}. Each is sent only to its own registry.`;
}

// signInLine says, under a registry address someone is about to sign in
// to, which sign-in it will be, or why there is none.
export function signInLine(s: SignInSettings, url: string): string {
  if (!origin(url)) return "";
  if (!usable(url)) return "Registries off this computer use https, so the sign-in never travels in clear.";
  const kind = signInKindFor(s, url);
  if (kind) return `This is ${signInPhrase(kind)}, sent only to ${hostOf(origin(url))}.`;
  if (isSecureAgentry(url)) {
    return s.secureAgentryRegistry
      ? `Agentry Desktop signs in to SecureAgentry at ${hostOf(s.secureAgentryRegistry)} only.`
      : "This build of Agentry Desktop signs in to no SecureAgentry registry.";
  }
  return `Agentry Desktop keeps one self-hosted registry, ${hostOf(s.selfHostedRegistry)}. Remove it in Settings to use another.`;
}
