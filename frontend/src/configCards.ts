// A persistent agent's Configuration as cards, as logic: which of the
// grids each roster entry is drawn in, the icon it is drawn with, the words
// and the tone of its state, whether a click switches it, what its menu
// offers, and the Add card that ends each grid. Pure, so the components and
// their tests share it. Every name and line is data the runner, the agent
// or the person wrote, drawn as text.
import type { app } from "../wailsjs/go/models";
import { AGENT_STATES } from "./persistentAgents.ts";

/** The grids, in their order. Mail and calendar is the Assistant's alone:
 *  its enabled mail providers' servers and their switches. */
export type GridKey = "apps" | "mail" | "accounts" | "skills" | "abilities";

export const GRID_TITLES: Record<GridKey, string> = {
  apps: "Apps and servers",
  mail: "Mail and calendar",
  accounts: "Accounts and secrets",
  skills: "Skills",
  abilities: "Abilities and folders",
};

const GRID_ORDER: GridKey[] = ["apps", "mail", "accounts", "skills", "abilities"];

/** The agent's mail providers, as the page reads them. */
type Providers = Pick<app.PersistentAgentView, "mailProviders">;

/** providerOf is the mail provider whose server an entry is, or one of
 *  whose switches it is (server:<key>:<switch>), if any. */
export function providerOf(e: { id?: string }, agent: Providers): app.PersistentMailProviderView | undefined {
  const id = e.id ?? "";
  return (agent.mailProviders ?? []).find((p) => p.servers.some((s) => id === s || id.startsWith(`${s}:`)));
}

/** gridOf is the grid an entry is drawn in: a mail provider's server and
 *  its switches are mail and calendar; other servers and what they may
 *  change (an offered server's writes, an image server's switches) are apps; accounts and secrets are what the agent signs in with;
 *  skills are skills; the rest (folders, memory, the heartbeat, commands,
 *  egress, the browser, the dev server, ports, knowledge packs) are its
 *  abilities. A kind this build does not know is an ability. */
export function gridOf(e: Pick<app.PersistentRosterEntry, "kind"> & { id?: string }, agent: Providers = { mailProviders: [] }): GridKey {
  if (providerOf(e, agent)) return "mail";
  switch (e.kind) {
    case "server":
    case "server-writes":
    case "server-switch":
      return "apps";
    case "account":
    case "secret":
      return "accounts";
    case "skill":
      return "skills";
    default:
      return "abilities";
  }
}

/** What an Add card adds. */
export type AddKind = "server" | "skill" | "secret" | "account" | "provider";

/** One grid as the Configuration draws it: its entries in the roster's
 *  order and its Add cards, none where the agent takes nothing of that
 *  kind from the person. */
export type Grid = { key: GridKey; title: string; entries: app.PersistentRosterEntry[]; adds: AddKind[] };

/** configGrids are the agent's grids. A grid with no entry and no Add
 *  card is left out. withAdds is false where the cards are only shown, as
 *  on the agent's page. */
export function configGrids(agent: Pick<app.PersistentAgentView, "roster" | "keepsSecrets" | "takesSkills" | "codes" | "mailProviders">, withAdds = true): Grid[] {
  const adds: Record<GridKey, AddKind[]> = {
    apps: ["server"],
    mail: disabledProviders(agent).length > 0 ? ["provider"] : [],
    accounts: [...(agent.codes ? (["account"] as const) : []), ...(agent.keepsSecrets ? (["secret"] as const) : [])],
    skills: agent.takesSkills ? ["skill"] : [],
    abilities: [],
  };
  return GRID_ORDER.map((key) => ({
    key,
    title: GRID_TITLES[key],
    entries: agent.roster.filter((e) => gridOf(e, agent) === key),
    adds: withAdds ? adds[key] : [],
  })).filter((g) => g.entries.length > 0 || g.adds.length > 0);
}

/** What a card is drawn with: one of Agentry's own glyphs, a kept icon of a
 *  server the person added, or a monogram. */
export type CardIcon = { kind: "glyph"; key: string } | { kind: "image"; src: string } | { kind: "monogram"; text: string };

/** The glyphs this build draws: configGlyphs.tsx draws each of them, and
 *  its table is typed by this list, so neither can lack one. */
export const GLYPH_KEYS = [
  "git", "encyclopedia", "docs", "scan", "weather", "map", "mail", "search", "server",
  "folder", "box", "memory", "clock", "terminal", "globe", "browser", "host", "plug", "book", "skill", "key", "person",
] as const;
export type GlyphKey = (typeof GLYPH_KEYS)[number];
const GLYPHS: ReadonlySet<string> = new Set(GLYPH_KEYS);

// A kept icon is a data: URL of a PNG, JPEG or WebP and nothing else: the
// page loads no image from anywhere.
const IMAGE = /^data:image\/(png|jpeg|webp);base64,[A-Za-z0-9+/]+=*$/;

/** cardIcon is the icon an entry is drawn with: a server the person added
 *  by its kept icon, a built-in's server by Agentry's glyph for it, an
 *  ability by the glyph of what it is, and anything else by a monogram. */
export function cardIcon(e: Pick<app.PersistentRosterEntry, "id" | "kind" | "name" | "icon" | "iconKey">): CardIcon {
  if (e.icon && IMAGE.test(e.icon)) return { kind: "image", src: e.icon };
  if (e.iconKey && GLYPHS.has(e.iconKey)) return { kind: "glyph", key: e.iconKey };
  const byID: Record<string, string> = { share: "folder", data: "box", memory: "memory", heartbeat: "clock", "dependency-scan": "scan", commands: "terminal", egress: "globe", browser: "browser" };
  if (Object.hasOwn(byID, e.id)) return { kind: "glyph", key: byID[e.id] };
  const byKind: Record<string, string> = { files: "folder", memory: "memory", tasks: "clock", host: "host", port: "plug", knowledge: "book", skill: "skill", secret: "key", account: "person" };
  if (Object.hasOwn(byKind, e.kind)) return { kind: "glyph", key: byKind[e.kind] };
  return { kind: "monogram", text: monogram(e.name) };
}

/** monogram is one or two capitals for a name: the first letters of its
 *  first two words, a package's scope and version left out ("npm:@acme/pdf-tools@1.2.0"
 *  is "PT"). */
export function monogram(name: string): string {
  let n = name.replace(/^(npm|pypi):/i, "").replace(/@[^@/]*$/, "");
  n = n.replace(/^@[^/]+\//, "").replace(/^https?:\/\//i, "");
  const words = n.split(/[^A-Za-z0-9]+/).filter(Boolean);
  const letters = words.slice(0, 2).map((w) => w[0].toUpperCase()).join("");
  return letters || "?";
}

/** How a card's state reads: on (full colour), off (greyed), busy (an
 *  install or a change waiting for the next turn), attention (it needs a
 *  sign-in or a set-up) or failed. */
export type Tone = "on" | "off" | "busy" | "attention" | "failed";

export type CardState = { words: string; tone: Tone; retry: boolean; reason?: string };

/** cardState is an entry's state as its card draws it. A server installs
 *  in the agent's background while it runs, and is ready only while the
 *  agent runs here; a failed install keeps its reason and offers Retry. */
export function cardState(agent: Pick<app.PersistentAgentView, "state">, e: Pick<app.PersistentRosterEntry, "kind" | "state" | "on" | "reason">): CardState {
  const live = agent.state === AGENT_STATES.running;
  switch (e.state) {
    case "installing":
      return { words: "Installing…", tone: "busy", retry: false };
    case "failed":
      return { words: "Failed", tone: "failed", retry: true, reason: e.reason || undefined };
    case "needs-sign-in":
      return { words: "Needs a sign-in", tone: "attention", retry: false, reason: e.reason || undefined };
    case "needs-set-up":
      return { words: "Needs setting up", tone: "attention", retry: false, reason: e.reason || undefined };
    case "pending":
      return e.on
        ? { words: e.kind === "server" && live ? "Ready, joins at your next message" : "On at the next turn", tone: "busy", retry: false }
        : { words: "Off at the next turn", tone: "off", retry: false };
    case "on":
      return { words: e.kind === "server" ? (live ? "Ready" : "On") : "On", tone: "on", retry: false };
    default:
      return { words: "Off", tone: "off", retry: false };
  }
}

/** Whether a click on the card switches it: never a fixed entry; one that
 *  is on always, so it can be turned off whatever it waits for; one that is
 *  off unless it waits on an install, a sign-in or a set-up. */
export function cardSwitches(e: Pick<app.PersistentRosterEntry, "fixed" | "state" | "on">): boolean {
  if (e.fixed) return false;
  return e.on || (e.state !== "installing" && e.state !== "needs-sign-in" && e.state !== "needs-set-up");
}

/** What Remove does to an entry the person added, or null where it has
 *  none: a built-in's entry is switched, never removed. */
export type RemoveAction =
  | { kind: "change"; change: { removeServer?: string; removeKnowledge?: string; removePort?: string }; what: string }
  | { kind: "skill"; name: string; what: string }
  | { kind: "secret"; name: string; what: string }
  | { kind: "account"; host: string; what: string };

// after is what an entry's id holds after its kind's prefix.
const after = (id: string, prefix: string) => (id.startsWith(prefix) ? id.slice(prefix.length) : "");

/** removeAction is what Remove does to an entry, or null for one the
 *  person did not add. */
export function removeAction(e: Pick<app.PersistentRosterEntry, "id" | "kind" | "name" | "grantedBy">): RemoveAction | null {
  if (e.grantedBy !== "you") return null;
  switch (e.kind) {
    case "server":
      return { kind: "change", change: { removeServer: e.id }, what: `The server ${e.name}` };
    case "knowledge": {
      const pack = after(e.id, "knowledge:") || e.name;
      return { kind: "change", change: { removeKnowledge: pack }, what: `The pack ${pack}` };
    }
    case "port": {
      const port = after(e.id, "port:");
      return port ? { kind: "change", change: { removePort: port }, what: `The port ${port}` } : null;
    }
    case "skill": {
      const name = after(e.id, "skill:");
      return name ? { kind: "skill", name, what: `The skill ${name}` } : null;
    }
    case "secret": {
      const name = after(e.id, "secret:");
      return name ? { kind: "secret", name, what: `The secret ${name}` } : null;
    }
    case "account": {
      const host = after(e.id, "account:");
      return host ? { kind: "account", host, what: `The account on ${host}` } : null;
    }
  }
  return null;
}

/** What an entry's Set up asks for: the secret an offered server needs, or
 *  an account on a host: the secret's or the account's Add form, its name
 *  or its host filled in. */
export type SetUp = { kind: "secret"; name: string } | { kind: "account"; host: string } | { kind: "google" } | { kind: "microsoft" };

/** An open Add form: what it adds, what it is filled in with, and the grid
 *  it is drawn under. A Set up's form is drawn under the grid of the card
 *  that asked, so it opens where the person is looking, and on a page that
 *  draws no Add card too. */
export type AddForm = { kind: AddKind; secretName?: string; host?: string; grid: GridKey };

/** setUpForm is the form a Set up opens from a card in grid, or null for
 *  Google Workspace and Microsoft 365, whose set-ups are panels of their
 *  own. */
export function setUpForm(s: SetUp, grid: GridKey): AddForm | null {
  switch (s.kind) {
    case "secret":
      return { kind: "secret", secretName: s.name, grid };
    case "account":
      return { kind: "account", host: s.host, grid };
    default:
      return null;
  }
}

/** addFormOf is the form an Add card in grid opens. */
export function addFormOf(kind: AddKind, grid: GridKey): AddForm {
  return { kind, grid };
}

/** setUpOf is what an entry's Set up asks for, or null for one that
 *  needs none now. */
export function setUpOf(e: Pick<app.PersistentRosterEntry, "state" | "setUpSecret" | "setUpAccount" | "setUpGoogle" | "setUpMicrosoft">): SetUp | null {
  if (e.state !== "needs-set-up") return null;
  if (e.setUpSecret) return { kind: "secret", name: e.setUpSecret };
  if (e.setUpAccount) return { kind: "account", host: e.setUpAccount };
  if (e.setUpGoogle) return { kind: "google" };
  if (e.setUpMicrosoft) return { kind: "microsoft" };
  return null;
}

/** googleWords is what the Google Workspace set-up says it holds: the client,
 *  then the account gog named, never anything of the client itself. */
export function googleWords(g: Pick<app.PersistentGoogleView, "client" | "account" | "stale"> | undefined): string {
  if (!g || !g.client) return "No Google Cloud client yet.";
  if (!g.account) return "Your Google Cloud client is kept. Sign in next.";
  if (g.stale) return `Signed in as ${g.account} with your earlier client: sign in again with the new one.`;
  return `Signed in as ${g.account}.`;
}

/** googleScopeWords says what a sign-in asks Google for: read-only unless
 *  the server's "allow changes" switch is on, so switching it on later
 *  needs a new sign-in. */
export function googleScopeWords(roster: Pick<app.PersistentRosterEntry, "id" | "on">[]): string {
  const writes = roster.some((e) => e.id === "server:gog:writes" && e.on);
  return writes
    ? "The sign-in asks Google to let it change things as well as read them, since Allow changes is on."
    : "The sign-in asks Google for read-only access. To let it change things, switch Allow changes on first, then sign in again.";
}

/** A card's menu: Set up where it needs one, Details always, Remove where
 *  the person added it. */
/** What a card's menu offers: Set up, Remove, and on a mail provider's
 *  server, Disable that provider. */
export type CardMenu = { setUp: SetUp | null; remove: RemoveAction | null; disable: app.PersistentMailProviderView | null };

export function cardMenu(e: app.PersistentRosterEntry, agent: Providers = { mailProviders: [] }): CardMenu {
  const p = providerOf(e, agent);
  return { setUp: setUpOf(e), remove: removeAction(e), disable: p && p.servers.includes(e.id) ? p : null };
}

/** disabledProviders are the mail providers the Add a provider card offers. */
export function disabledProviders(agent: Providers): app.PersistentMailProviderView[] {
  return (agent.mailProviders ?? []).filter((p) => !p.enabled);
}

/** providerWords is what the Add a provider panel says of a provider: the
 *  account it is still signed in as, or that it needs setting up. */
export function providerWords(p: Pick<app.PersistentMailProviderView, "accounts">): string {
  return p.accounts.length > 0 ? `Still signed in as ${p.accounts.join(", ")}: enabling it needs no new sign-in.` : "Set it up after enabling it.";
}

/** Who granted an entry, in words. */
export function grantedWords(e: Pick<app.PersistentRosterEntry, "grantedBy">): string {
  return e.grantedBy === "you" ? "Added by you" : "Built in";
}

/** A secret's name as the CLI takes it: an upper-case letter, then
 *  upper-case letters, digits and underscores, at most 64. The runner
 *  holds the rest of the rule (names it sets itself are refused there). */
export const SECRET_NAME = /^[A-Z][A-Z0-9_]{0,63}$/;

/** Whether the secret form's value is one the CLI takes: 8 bytes to 16 KiB
 *  on one line, no control character. */
export function secretValueOK(v: string): boolean {
  const n = new TextEncoder().encode(v).length;
  // eslint-disable-next-line no-control-regex
  return n >= 8 && n <= 16 * 1024 && !/[\u0000-\u001f\u007f]/.test(v);
}

/** An Entra application (client) id, as the CLI takes it: a GUID. The CLI
 *  also refuses the server's own default client. */
export const ENTRA_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The tenants the set-up offers: "" asks Microsoft which kind the address
 *  is; a tenant's own id is typed instead. */
export const MICROSOFT_TENANTS: { value: string; label: string }[] = [
  { value: "", label: "Ask Microsoft (from my address)" },
  { value: "consumers", label: "A personal account (Outlook.com, Hotmail, Live)" },
  { value: "organizations", label: "A work or school account" },
];

/** microsoftWords is what the Microsoft 365 set-up says it holds: the app,
 *  then the account the server named, never anything secret. */
export function microsoftWords(m: Pick<app.PersistentMicrosoftView, "clientID" | "account" | "stale"> | undefined): string {
  if (!m || !m.clientID) return "No Entra app yet.";
  if (!m.account) return "Your Entra app is kept. Sign in next.";
  if (m.stale) return `Signed in as ${m.account} with your earlier app: sign in again with the new one.`;
  return `Signed in as ${m.account}.`;
}

/** microsoftScopeWords says what a sign-in asks Microsoft for: reading
 *  alone unless Allow changes is on. */
export function microsoftScopeWords(roster: Pick<app.PersistentRosterEntry, "id" | "on">[]): string {
  const writes = roster.some((e) => e.id === "server:ms365:writes" && e.on);
  return writes
    ? "The sign-in asks Microsoft to let it change things as well as read them, since Allow changes is on."
    : "The sign-in asks Microsoft for reading alone. To let it change things, switch Allow changes on first, then sign in again.";
}
