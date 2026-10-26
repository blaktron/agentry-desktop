// A persistent agent's Configuration as cards: the grids, each card's icon,
// state, switch and menu, and the secret form's rules.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import {
  cardIcon,
  cardMenu,
  cardState,
  cardSwitches,
  configGrids,
  gridOf,
  monogram,
  removeAction,
  secretValueOK,
  SECRET_NAME,
  setUpOf,
  googleWords,
  googleScopeWords,
  addFormOf,
  setUpForm,
  disabledProviders,
  providerOf,
  providerWords,
  ENTRA_ID,
  microsoftCanSignIn,
  microsoftWords,
  microsoftAdminWords,
  microsoftScopeWords,
  fastmailWords,
} from "../src/configCards.ts";

const entry = (e: Partial<app.PersistentRosterEntry>): app.PersistentRosterEntry =>
  ({ id: "x", kind: "server", name: "x", grantedBy: "built-in", state: "on", on: true, line: "", ...e }) as app.PersistentRosterEntry;

const coder = {
  keepsSecrets: true,
  takesSkills: true,
  codes: true,
  roster: [
    entry({ id: "share", kind: "files", name: "Your share folder" }),
    entry({ id: "data", kind: "files", name: "Its own files", fixed: true }),
    entry({ id: "commands", kind: "tools", name: "Commands" }),
    entry({ id: "server:gh", kind: "server", name: "GitHub", iconKey: "git", state: "needs-set-up", on: false, setUpAccount: "github.com" }),
    entry({ id: "server:gh:writes", kind: "server-writes", name: "GitHub: changes", iconKey: "git", on: false, state: "off" }),
    entry({ id: "skill:code-review", kind: "skill", name: "code-review" }),
    entry({ id: "secret:API_KEY", kind: "secret", name: "Your secret API_KEY", grantedBy: "you" }),
    entry({ id: "port:web", kind: "port", name: "web", grantedBy: "you" }),
  ],
} as Pick<app.PersistentAgentView, "roster" | "keepsSecrets" | "takesSkills" | "codes">;

test("each entry is drawn in its grid, the grids in their order, each ending in its Add card", () => {
  const grids = configGrids(coder);
  assert.deepEqual(grids.map((g) => [g.title, g.entries.map((e) => e.id), g.adds]), [
    ["Apps and servers", ["server:gh", "server:gh:writes"], ["server"]],
    ["Accounts and secrets", ["secret:API_KEY"], ["account", "secret"]],
    ["Skills", ["skill:code-review"], ["skill"]],
    ["Abilities and folders", ["share", "data", "commands", "port:web"], []],
  ]);
  assert.equal(gridOf({ kind: "something-new" }), "abilities");
  // An image server's switches (Google Workspace's Allow changes) are drawn beside it.
  assert.equal(gridOf({ kind: "server-switch" }), "apps");
});

test("an agent that keeps no secrets and takes no skills has no Add card for them, and a grid left empty is left out", () => {
  const main = { keepsSecrets: false, takesSkills: false, codes: false, roster: [entry({ id: "share", kind: "files", name: "Your share folder" })] };
  assert.deepEqual(configGrids(main).map((g) => [g.key, g.adds]), [["apps", ["server"]], ["abilities", []]]);
  // The Assistant keeps secrets and has no accounts.
  assert.deepEqual(configGrids({ ...main, keepsSecrets: true }).map((g) => [g.key, g.adds])[1], ["accounts", ["secret"]]);
  // On the agent's page the cards are only shown: no Add card.
  assert.deepEqual(configGrids(coder, false).map((g) => g.adds), [[], [], [], []]);
});

test("a card's icon: a kept image, a glyph of Agentry's own, or a monogram, and nothing loaded from anywhere", () => {
  const png = "data:image/png;base64,iVBORw0KGgo=";
  assert.deepEqual(cardIcon(entry({ name: "npm:@acme/pdf-tools@1.2.0", grantedBy: "you", icon: png })), { kind: "image", src: png });
  assert.deepEqual(cardIcon(entry({ name: "GitHub", iconKey: "git" })), { kind: "glyph", key: "git" });
  assert.deepEqual(cardIcon(entry({ name: "npm:@acme/pdf-tools@1.2.0", grantedBy: "you" })), { kind: "monogram", text: "PT" });
  for (const bad of ["data:image/svg+xml;base64,PHN2Zz4=", "https://example.com/i.png", "data:image/png;base64,<script>", "javascript:alert(1)"]) {
    assert.equal(cardIcon(entry({ name: "acme", grantedBy: "you", icon: bad })).kind, "monogram", bad);
  }
  assert.equal(cardIcon(entry({ name: "x", iconKey: "not-a-glyph" })).kind, "monogram");
  assert.deepEqual(cardIcon(entry({ id: "heartbeat", kind: "tasks", name: "Its heartbeat and tasks" })), { kind: "glyph", key: "clock" });
  assert.deepEqual(cardIcon(entry({ id: "secret:API_KEY", kind: "secret", name: "Your secret API_KEY" })), { kind: "glyph", key: "key" });
});

test("a monogram is the first letters of a name's first two words, a package's scope and version left out", () => {
  assert.equal(monogram("pypi:mcp-server-fetch@0.6.2"), "MS");
  assert.equal(monogram("https://mcp.linear.app/mcp"), "ML");
  assert.equal(monogram("Weather"), "W");
  assert.equal(monogram("—"), "?");
});

test("a card's state: off greyed, on in colour, an install busy, a sign-in or set-up wanting attention, a failure with Retry", () => {
  const stopped = { state: "stopped" };
  const running = { state: "running" };
  assert.deepEqual(cardState(stopped, entry({ state: "off", on: false })), { words: "Off", tone: "off", retry: false });
  assert.deepEqual(cardState(stopped, entry({ state: "on" })), { words: "On", tone: "on", retry: false });
  assert.deepEqual(cardState(running, entry({ state: "on" })), { words: "Ready", tone: "on", retry: false });
  // Only the supervisor in this app knows a server is installed: an agent
  // running elsewhere has its server only on.
  assert.equal(cardState({ state: "elsewhere" }, entry({ state: "on" })).words, "On");
  assert.equal(cardState(running, entry({ state: "needs-sign-in", on: false })).words, "Needs a sign-in");
  assert.equal(cardState(running, entry({ state: "installing" })).tone, "busy");
  assert.deepEqual(cardState(stopped, entry({ state: "failed", reason: "npm answered 404" })), { words: "Failed", tone: "failed", retry: true, reason: "npm answered 404" });
  assert.equal(cardState(stopped, entry({ state: "needs-sign-in" })).tone, "attention");
  assert.equal(cardState(stopped, entry({ state: "needs-set-up", reason: "it needs an account on github.com" })).words, "Needs setting up");
  assert.equal(cardState(running, entry({ state: "pending", on: true })).words, "Ready, joins at your next message");
  assert.equal(cardState(running, entry({ kind: "skill", state: "pending", on: false })).words, "Off at the next turn");
});

test("a click switches a card, but not a fixed one, nor one that is off and waits on an install, a sign-in or a set-up", () => {
  assert.equal(cardSwitches(entry({ state: "on" })), true);
  assert.equal(cardSwitches(entry({ state: "failed" })), true);
  for (const state of ["installing", "needs-sign-in", "needs-set-up"]) {
    assert.equal(cardSwitches(entry({ state, on: false })), false, state);
    // On and waiting: it can always be turned off.
    assert.equal(cardSwitches(entry({ state, on: true })), true, state);
  }
  assert.equal(cardSwitches(entry({ fixed: true })), false);
});

test("Remove is offered on what the person added, never on a built-in's entry", () => {
  assert.equal(removeAction(entry({ id: "server:gh", name: "GitHub" })), null);
  assert.equal(removeAction(entry({ id: "skill:code-review", kind: "skill" })), null);
  assert.deepEqual(removeAction(entry({ id: "server:abc", name: "npm:x@1.0.0", grantedBy: "you" })), { kind: "change", change: { removeServer: "server:abc" }, what: "The server npm:x@1.0.0" });
  assert.deepEqual(removeAction(entry({ id: "skill:say-hello", kind: "skill", grantedBy: "you" })), { kind: "skill", name: "say-hello", what: "The skill say-hello" });
  assert.deepEqual(removeAction(entry({ id: "secret:API_KEY", kind: "secret", grantedBy: "you" })), { kind: "secret", name: "API_KEY", what: "The secret API_KEY" });
  assert.deepEqual(removeAction(entry({ id: "port:web", kind: "port", grantedBy: "you" })), { kind: "change", change: { removePort: "web" }, what: "The port web" });
  assert.deepEqual(removeAction(entry({ id: "knowledge:docs", kind: "knowledge", grantedBy: "you" })), { kind: "change", change: { removeKnowledge: "docs" }, what: "The pack docs" });
  assert.deepEqual(removeAction(entry({ id: "account:github.com", kind: "account", grantedBy: "you" })), { kind: "account", host: "github.com", what: "The account on github.com" });
});

test("Set up asks for what an offered server needs, only while it needs it", () => {
  assert.deepEqual(setUpOf(entry({ state: "needs-set-up", setUpSecret: "BRAVE_API_KEY" })), { kind: "secret", name: "BRAVE_API_KEY" });
  assert.deepEqual(setUpOf(entry({ state: "needs-set-up", setUpAccount: "github.com" })), { kind: "account", host: "github.com" });
  assert.equal(setUpOf(entry({ state: "on", setUpSecret: "BRAVE_API_KEY" })), null);
  assert.deepEqual(setUpOf(entry({ state: "needs-set-up", setUpGoogle: true })), { kind: "google" });
  assert.equal(setUpOf(entry({ state: "off", setUpGoogle: true })), null);
  assert.deepEqual(cardMenu(coder.roster[3]), { setUp: { kind: "account", host: "github.com" }, remove: null, disable: null });
});

test("the secret form takes the CLI's names and values", () => {
  for (const ok of ["API_KEY", "A", "OPENWEATHER_KEY_2"]) assert.ok(SECRET_NAME.test(ok), ok);
  for (const bad of ["api_key", "1KEY", "KEY-1", "", "A".repeat(65)]) assert.ok(!SECRET_NAME.test(bad), bad);
  assert.ok(secretValueOK("abcdefgh"));
  assert.ok(!secretValueOK("short"));
  assert.ok(!secretValueOK("two\nlines-here"));
  assert.ok(!secretValueOK("x".repeat(16 * 1024 + 1)));
});

test("the Google set-up says what it holds: no client, a client, then the account the server named", () => {
  assert.equal(googleWords(undefined), "No Google Cloud client yet.");
  assert.equal(googleWords({ client: false, account: "" }), "No Google Cloud client yet.");
  assert.equal(googleWords({ client: true, account: "" }), "Your Google Cloud client is kept. Sign in next.");
  assert.equal(googleWords({ client: true, account: "me@example.com" }), "Signed in as me@example.com.");
  assert.equal(googleWords({ client: true, account: "me@example.com", stale: true }), "Signed in as me@example.com with your earlier client: sign in again with the new one.");
});

test("the sign-in asks for read-only access unless Allow changes is on", () => {
  assert.match(googleScopeWords([{ id: "server:gog:writes", on: false }]), /read-only/);
  assert.match(googleScopeWords([]), /read-only/);
  assert.match(googleScopeWords([{ id: "server:gog:writes", on: true }]), /change things/);
});

test("Set up opens its form under the asking card's grid, whether or not the page draws Add cards", () => {
  const brave = entry({ id: "server:npm-brave-brave-search-mcp-server", kind: "server", state: "needs-set-up", setUpSecret: "BRAVE_API_KEY" });
  const set = setUpOf(brave);
  assert.ok(set);
  const form = setUpForm(set, gridOf(brave));
  assert.deepEqual(form, { kind: "secret", secretName: "BRAVE_API_KEY", grid: "apps" });
  // The agent's page draws no Add card, and the form is still drawn, under
  // the Apps grid where the card that asked is.
  const grids = configGrids({ ...coder, roster: [brave] }, false);
  assert.deepEqual(grids.map((g) => g.adds), [[]]);
  assert.deepEqual(grids.map((g) => g.key === form?.grid), [true]);
  assert.deepEqual(setUpForm({ kind: "account", host: "github.com" }, "apps"), { kind: "account", host: "github.com", grid: "apps" });
  assert.equal(setUpForm({ kind: "google" }, "apps"), null);
});

test("an Add card opens its form under its own grid", () => {
  assert.deepEqual(addFormOf("secret", "accounts"), { kind: "secret", grid: "accounts" });
});

const google = (enabled: boolean, accounts: string[] = []): app.PersistentMailProviderView =>
  ({ key: "google", name: "Google", enabled, servers: ["server:gog"], accounts, setUp: [] }) as app.PersistentMailProviderView;

const assistant = (enabled: boolean, accounts: string[] = []) => ({
  keepsSecrets: true,
  takesSkills: true,
  codes: false,
  mailProviders: [google(enabled, accounts)],
  roster: [
    entry({ id: "server:wiki", kind: "server", name: "Wikipedia" }),
    ...(enabled
      ? [
          entry({ id: "server:gog", kind: "server", name: "Google Workspace (gog)", iconKey: "mail", on: false, state: "off" }),
          entry({ id: "server:gog:writes", kind: "server-switch", name: "Google Workspace: allow changes", iconKey: "mail", on: false, state: "off" }),
        ]
      : []),
  ],
}) as Pick<app.PersistentAgentView, "roster" | "keepsSecrets" | "takesSkills" | "codes" | "mailProviders">;

test("an enabled mail provider's server and switches are drawn in Mail and calendar, after Apps and servers", () => {
  const grids = configGrids(assistant(true));
  assert.deepEqual(grids.slice(0, 2).map((g) => [g.title, g.entries.map((e) => e.id), g.adds]), [
    ["Apps and servers", ["server:wiki"], ["server"]],
    ["Mail and calendar", ["server:gog", "server:gog:writes"], []],
  ]);
  assert.equal(gridOf({ id: "server:gog:send", kind: "server-switch" }, assistant(true)), "mail");
  assert.equal(providerOf({ id: "server:gogx" }, assistant(true)), undefined);
});

test("a disabled provider leaves its section an Add a provider card, which offers it with what it still holds", () => {
  const off = assistant(false, ["person@example.com"]);
  const mail = configGrids(off).find((g) => g.key === "mail");
  assert.deepEqual(mail && [mail.entries.length, mail.adds], [0, ["provider"]]);
  assert.deepEqual(disabledProviders(off).map((p) => p.key), ["google"]);
  assert.match(providerWords(google(false, ["person@example.com"])), /Still signed in as person@example.com: enabling it needs no new sign-in/);
  assert.match(providerWords(google(false)), /Set it up after enabling it/);
  // Every provider enabled: no Add card, and the section stays for its cards.
  assert.deepEqual(configGrids(assistant(true)).find((g) => g.key === "mail")?.adds, []);
  // On the agent's page no Add card is drawn, so an empty section is left out.
  assert.equal(configGrids(off, false).some((g) => g.key === "mail"), false);
  // An agent with no providers has no such section.
  assert.equal(configGrids(coder).some((g) => g.key === "mail"), false);
});

test("a mail provider's server card offers to disable the provider; its switches and other cards do not", () => {
  const a = assistant(true);
  assert.equal(cardMenu(a.roster[1], a).disable?.key, "google");
  assert.equal(cardMenu(a.roster[2], a).disable, null);
  assert.equal(cardMenu(a.roster[0], a).disable, null);
});

test("Microsoft 365's cards open its own set-up, which says what it holds and what the sign-in asks for", () => {
  const ms = entry({ id: "server:ms365", kind: "server", name: "Microsoft 365 and Outlook.com", iconKey: "mail", state: "needs-set-up", on: true, setUpMicrosoft: true });
  assert.deepEqual(setUpOf(ms), { kind: "microsoft" });
  assert.equal(setUpForm({ kind: "microsoft" }, "mail"), null);
  assert.equal(microsoftWords(undefined), "No Entra app yet.");
  assert.equal(microsoftWords({ clientID: "11111111-2222-3333-4444-555555555555", account: "", stale: false }), "Your Entra app is kept. Sign in next.");
  assert.equal(microsoftWords({ clientID: "11111111-2222-3333-4444-555555555555", account: "ana@outlook.com", stale: false }), "Signed in as ana@outlook.com.");
  assert.match(microsoftWords({ clientID: "x", account: "ana@outlook.com", stale: true }), /earlier app/);
  // Agentry's own Microsoft app, offered where no Entra app is kept.
  assert.equal(microsoftWords({ clientID: "", account: "", stale: false }, true), "Agentry's Microsoft app is ready. Sign in next.");
  assert.equal(microsoftWords({ clientID: "", account: "", stale: false }, false), "No Entra app yet.");
  assert.equal(microsoftWords({ clientID: "", account: "ana@outlook.com", stale: false, signedInWithAgentryApp: true }), "Signed in as ana@outlook.com with Agentry's Microsoft app.");
  assert.match(microsoftWords({ clientID: "x", account: "ana@outlook.com", stale: true, signedInWithAgentryApp: true }), /sign in again to use your own app/);
  assert.equal(microsoftCanSignIn(undefined, false), false);
  assert.equal(microsoftCanSignIn({ clientID: "" }, false), false);
  assert.equal(microsoftCanSignIn({ clientID: "" }, true), true);
  assert.equal(microsoftCanSignIn({ clientID: "x" }, false), true);
  assert.match(microsoftScopeWords([{ id: "server:ms365:writes", on: false }]), /reading alone/);
  assert.match(microsoftScopeWords([{ id: "server:ms365:writes", on: true }]), /change things as well/);
  for (const ok of ["11111111-2222-3333-4444-555555555555", "ABCDEF01-2222-3333-4444-555555555555"]) assert.ok(ENTRA_ID.test(ok), ok);
  for (const bad of ["my-app", "11111111-2222-3333-4444-55555555555", " 11111111-2222-3333-4444-555555555555", ""]) assert.ok(!ENTRA_ID.test(bad), bad);
});

test("an enabled Microsoft's server and switches are drawn in Mail and calendar with Disable Microsoft", () => {
  const a = {
    ...assistant(true),
    mailProviders: [google(true), { key: "microsoft", name: "Microsoft", enabled: true, servers: ["server:ms365"], accounts: [], setUp: [] } as app.PersistentMailProviderView],
    roster: [
      ...assistant(true).roster,
      entry({ id: "server:ms365", kind: "server", name: "Microsoft 365 and Outlook.com", iconKey: "mail", on: false, state: "off", setUpMicrosoft: true }),
      entry({ id: "server:ms365:send", kind: "server-switch", name: "Microsoft 365: allow sending mail", iconKey: "mail", on: false, state: "off", setUpMicrosoft: true }),
    ],
  };
  const mail = configGrids(a).find((g) => g.key === "mail");
  assert.deepEqual(mail?.entries.map((e) => e.id), ["server:gog", "server:gog:writes", "server:ms365", "server:ms365:send"]);
  assert.equal(cardMenu(a.roster[3], a).disable?.key, "microsoft");
  assert.equal(cardMenu(a.roster[4], a).disable, null);
});

test("Fastmail's Set up is its own panel, before or after its token is kept, never the secret's form", () => {
  const fm = { setUpFastmail: true, setUpSecret: "FASTMAIL_TOKEN" };
  assert.deepEqual(setUpOf(entry({ state: "needs-set-up", ...fm })), { kind: "fastmail" });
  assert.deepEqual(setUpOf(entry({ state: "on", ...fm })), { kind: "fastmail" });
  assert.equal(setUpForm({ kind: "fastmail" }, "mail"), null);
});

test("the Fastmail set-up says whether a token is kept, never the token", () => {
  assert.equal(fastmailWords(undefined), "No Fastmail token yet.");
  assert.equal(fastmailWords({ token: false }), "No Fastmail token yet.");
  assert.match(fastmailWords({ token: true }), /kept in your system keychain/);
});

test("the Microsoft set-up says a work or school account needs its administrator's approval, and a personal one nothing", () => {
  assert.equal(microsoftAdminWords("consumers", "the Assistant"), "");
  assert.match(microsoftAdminWords("organizations", "the Assistant"), /administrator will most likely have to approve your app/);
  assert.match(microsoftAdminWords("org", "Ada"), /Ask Ada to help you set up Microsoft 365 for a work account/);
  assert.match(microsoftAdminWords("", "the Assistant"), /^A work or school account usually needs its administrator/);
});
