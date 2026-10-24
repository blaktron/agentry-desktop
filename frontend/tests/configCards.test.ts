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
  assert.deepEqual(cardMenu(coder.roster[3]), { setUp: { kind: "account", host: "github.com" }, remove: null });
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
