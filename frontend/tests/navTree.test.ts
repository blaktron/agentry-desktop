// The sidebar's rows: their order, what is shown beneath each, and which is
// lit for the page that is open.
import { test } from "node:test";
import assert from "node:assert/strict";
import { inBuild, navRows, type NavRow, type NavState } from "../src/navTree.ts";

const SECTIONS = [{ id: "account", title: "Account" }, { id: "home", title: "Home" }, { id: "licences", title: "Licences" }];
const TOP = ["Home", "Agents", "Scheduled", "Build", "Library", "Plugins", "Settings"];

const rows = (s: Partial<NavState> & Pick<NavState, "page">) => navRows({ buildOpen: false, sections: SECTIONS, ...s });
// Every row drawn, depth first, as the sidebar lists them.
const labels = (rs: NavRow[]): string[] => rs.flatMap((r) => [r.label, ...labels(r.children ?? [])]);
const pick = (rs: NavRow[], f: (r: NavRow) => boolean): string[] => rs.flatMap((r) => [...(f(r) ? [r.label] : []), ...pick(r.children ?? [], f)]);
const lit = (rs: NavRow[]) => pick(rs, (r) => r.lit);
const current = (rs: NavRow[]) => pick(rs, (r) => r.current);
const row = (rs: NavRow[], key: string) => rs.find((r) => r.key === key)!;

test("from the top: Home, Agents, Scheduled, Build, Library, Plugins and Settings", () => {
  assert.deepEqual(labels(rows({ page: "home" })), TOP);
  assert.deepEqual(current(rows({ page: "home" })), ["Home"]);
  assert.deepEqual(row(rows({ page: "home" }), "agents").target, { kind: "page", page: "agents" });
  assert.deepEqual(row(rows({ page: "home" }), "plugins").target, { kind: "page", page: "plugins" });
});

test("the enabled persistent agents are always listed under Agents, each opening its own page", () => {
  const agents = [{ id: "assistant", label: "Ada", running: true }, { id: "coder", label: "Coding agent", running: false }];
  const home = rows({ page: "home", agents });
  assert.deepEqual(labels(home), ["Home", "Agents", "Ada", "Coding agent", "Scheduled", "Build", "Library", "Plugins", "Settings"]);
  assert.deepEqual(lit(home), ["Home"]);
  const rs = rows({ page: "persistent", agents, agent: "coder" });
  assert.deepEqual(current(rs), ["Coding agent"]);
  assert.deepEqual(lit(rs), ["Agents", "Coding agent"]);
  const a = row(rs, "agents");
  assert.equal(a.parent, true);
  assert.deepEqual(a.children?.[0].target, { kind: "agent", id: "assistant" });
  assert.equal(a.children?.[0].running, true);
  assert.equal(a.children?.[1].running, false);
});

test("Agents opens the page that manages them, and stays lit on an agent's page", () => {
  const agents = [{ id: "assistant", label: "Ada", running: true }];
  const page = rows({ page: "agents", agents });
  assert.deepEqual(current(page), ["Agents"]);
  assert.equal(row(page, "agents").parent, false);
  // An agent's page for one not listed (turned off since) lights Agents alone.
  const off = rows({ page: "persistent", agents, agent: "coder" });
  assert.deepEqual(lit(off), ["Agents"]);
  assert.deepEqual(current(off), []);
  // With no agent enabled there is nothing beneath it.
  assert.equal(row(rows({ page: "agents" }), "agents").children, undefined);
});

test("Scheduled is its own row", () => {
  const rs = rows({ page: "scheduled" });
  assert.deepEqual(current(rs), ["Scheduled"]);
  assert.deepEqual(row(rs, "scheduled").target, { kind: "page", page: "scheduled" });
});

test("Library opens Installed, with Browse beneath it, and Publisher only while signed in", () => {
  const lib = rows({ page: "library" });
  assert.deepEqual(labels(lib), ["Home", "Agents", "Scheduled", "Build", "Library", "Installed", "Browse", "Plugins", "Settings"]);
  assert.deepEqual(lit(lib), ["Library", "Installed"]);
  assert.deepEqual(current(lib), ["Installed"]);
  assert.equal(row(lib, "library").parent, true);
  assert.deepEqual(row(lib, "library").children?.[0].target, { kind: "page", page: "library" });
  assert.deepEqual(labels(rows({ page: "browse", signedIn: true })), ["Home", "Agents", "Scheduled", "Build", "Library", "Installed", "Browse", "Publisher", "Plugins", "Settings"]);
  const pub = rows({ page: "publisher", signedIn: true });
  assert.deepEqual(lit(pub), ["Library", "Publisher"]);
  assert.deepEqual(current(pub), ["Publisher"]);
});

test("Library's rows are hidden elsewhere, and on a QuickAgent's page Library is lit without them", () => {
  const rs = rows({ page: "agent", signedIn: true });
  assert.deepEqual(labels(rs), TOP);
  assert.deepEqual(lit(rs), ["Library"]);
  assert.deepEqual(current(rs), []);
});

test("Plugins opens its landing, with Your computer beneath it, and Meeting notes only where this copy records", () => {
  const without = rows({ page: "plugins" });
  assert.deepEqual(labels(without), [...TOP.slice(0, 6), "Your computer", "Settings"]);
  assert.deepEqual(current(without), ["Plugins"]);
  const withMeetings = rows({ page: "plugins", meetings: true });
  assert.deepEqual(labels(withMeetings), [...TOP.slice(0, 6), "Meeting notes", "Your computer", "Settings"]);
  const m = rows({ page: "meetings", meetings: true });
  assert.deepEqual(lit(m), ["Plugins", "Meeting notes"]);
  assert.deepEqual(current(m), ["Meeting notes"]);
  const c = rows({ page: "computer" });
  assert.deepEqual(lit(c), ["Plugins", "Your computer"]);
  assert.deepEqual(row(c, "plugins").children?.[0].target, { kind: "page", page: "computer" });
  // Elsewhere Plugins lists nothing beneath it.
  assert.deepEqual(labels(rows({ page: "home", meetings: true })), TOP);
});

test("Home stays lit on a run page", () => {
  assert.deepEqual(lit(rows({ page: "run" })), ["Home"]);
});

test("Build opens no page: it toggles QuickAgent, Voice and Knowledge", () => {
  const closed = rows({ page: "home" });
  const build = row(closed, "build");
  assert.deepEqual(build.target, { kind: "toggle", id: "build" });
  assert.equal(build.expanded, false);
  assert.equal(build.children, undefined);
  const open = rows({ page: "home", buildOpen: true });
  assert.deepEqual(labels(open), ["Home", "Agents", "Scheduled", "Build", "QuickAgent", "Voice", "Knowledge", "Library", "Plugins", "Settings"]);
  assert.equal(row(open, "build").expanded, true);
  assert.deepEqual(lit(open), ["Home"]);
});

test("QuickAgent is the page that builds one, with Validate beneath it while either is open", () => {
  const b = rows({ page: "build", buildOpen: true });
  assert.deepEqual(labels(b), ["Home", "Agents", "Scheduled", "Build", "QuickAgent", "Validate", "Voice", "Knowledge", "Library", "Plugins", "Settings"]);
  assert.deepEqual(lit(b), ["Build", "QuickAgent"]);
  assert.deepEqual(current(b), ["QuickAgent"]);
  const v = rows({ page: "validate", buildOpen: true });
  assert.deepEqual(lit(v), ["Build", "QuickAgent", "Validate"]);
  assert.deepEqual(current(v), ["Validate"]);
});

test("Build stays lit on its pages when the person has collapsed it", () => {
  const rs = rows({ page: "knowledge", buildOpen: false });
  assert.deepEqual(labels(rs), TOP);
  assert.deepEqual(lit(rs), ["Build"]);
});

test("a page of Build's opens it", () => {
  for (const p of ["build", "validate", "voice", "knowledge"] as const) assert.equal(inBuild(p), true, p);
  for (const p of ["home", "agents", "library", "plugins", "settings", "scheduled"] as const) assert.equal(inBuild(p), false, p);
});

test("Settings opens its landing and lists every section beneath it while a Settings page is open", () => {
  const landing = rows({ page: "settings" });
  assert.deepEqual(labels(landing), [...TOP, "Account", "Home", "Licences"]);
  assert.deepEqual(current(landing), ["Settings"]);
  const sec = rows({ page: "settings", section: "home" });
  assert.deepEqual(current(sec), ["Home"]);
  const settings = row(sec, "settings");
  assert.equal(settings.lit, true);
  assert.equal(settings.parent, true);
  assert.deepEqual(settings.children?.find((r) => r.current)?.target, { kind: "section", id: "home" });
  // Elsewhere the sections are not listed.
  assert.equal(row(rows({ page: "home" }), "settings").children, undefined);
});

test("the Notifications page lights no row: the bell above the rows is its", () => {
  assert.deepEqual(lit(rows({ page: "notifications" })), []);
  assert.deepEqual(current(rows({ page: "notifications" })), []);
});
