// The sidebar's rows: their order, what is shown beneath each, and which is
// lit for the page that is open.
import { test } from "node:test";
import assert from "node:assert/strict";
import { inBuild, navRows, type NavRow, type NavState } from "../src/navTree.ts";

const SECTIONS = [{ id: "profile", title: "Profile" }, { id: "home", title: "Home" }, { id: "licences", title: "Licences" }];

const rows = (s: Partial<NavState> & Pick<NavState, "page">) => navRows({ buildOpen: false, sections: SECTIONS, ...s });
// Every row drawn, depth first, as the sidebar lists them.
const labels = (rs: NavRow[]): string[] => rs.flatMap((r) => [r.label, ...labels(r.children ?? [])]);
const pick = (rs: NavRow[], f: (r: NavRow) => boolean): string[] => rs.flatMap((r) => [...(f(r) ? [r.label] : []), ...pick(r.children ?? [], f)]);
const lit = (rs: NavRow[]) => pick(rs, (r) => r.lit);
const current = (rs: NavRow[]) => pick(rs, (r) => r.current);

test("from the top: Home, Library, Schedule, Build and Settings", () => {
  assert.deepEqual(labels(rows({ page: "home" })), ["Home", "Library", "Schedule", "Build", "Settings"]);
  assert.deepEqual(current(rows({ page: "home" })), ["Home"]);
});

test("the enabled persistent agents come straight after Home, each opening its own page", () => {
  const agents = [{ id: "assistant", label: "Ada", running: true }, { id: "coder", label: "Coding agent", running: false }];
  const rs = rows({ page: "persistent", agents, agent: "coder" });
  assert.deepEqual(labels(rs), ["Home", "Ada", "Coding agent", "Library", "Schedule", "Build", "Settings"]);
  assert.deepEqual(current(rs), ["Coding agent"]);
  assert.deepEqual(rs[1].target, { kind: "agent", id: "assistant" });
  assert.equal(rs[1].running, true);
  assert.equal(rs[2].running, false);
});

test("Library shows Browse beneath it, and Publisher only while signed in", () => {
  assert.deepEqual(labels(rows({ page: "library" })), ["Home", "Library", "Browse", "Schedule", "Build", "Settings"]);
  assert.deepEqual(labels(rows({ page: "library", signedIn: true })), ["Home", "Library", "Browse", "Publisher", "Schedule", "Build", "Settings"]);
  const pub = rows({ page: "publisher", signedIn: true });
  assert.deepEqual(lit(pub), ["Library", "Publisher"]);
  assert.deepEqual(current(pub), ["Publisher"]);
  assert.equal(pub.find((r) => r.label === "Library")?.parent, true);
});

test("Library's rows are hidden elsewhere, and on an agent's page Library is lit without them", () => {
  const rs = rows({ page: "agent", signedIn: true });
  assert.deepEqual(labels(rs), ["Home", "Library", "Schedule", "Build", "Settings"]);
  assert.deepEqual(lit(rs), ["Library"]);
  assert.deepEqual(current(rs), []);
});

test("Home stays lit on a run page", () => {
  assert.deepEqual(lit(rows({ page: "run" })), ["Home"]);
});

test("Build opens no page: it toggles QuickAgent, Voice and Knowledge", () => {
  const closed = rows({ page: "home" });
  const build = closed.find((r) => r.label === "Build")!;
  assert.deepEqual(build.target, { kind: "toggle", id: "build" });
  assert.equal(build.expanded, false);
  assert.equal(build.children, undefined);
  const open = rows({ page: "home", buildOpen: true });
  assert.deepEqual(labels(open), ["Home", "Library", "Schedule", "Build", "QuickAgent", "Voice", "Knowledge", "Settings"]);
  assert.equal(open.find((r) => r.label === "Build")?.expanded, true);
  assert.deepEqual(lit(open), ["Home"]);
});

test("QuickAgent is today's Build page, with Validate beneath it while either is open", () => {
  const b = rows({ page: "build", buildOpen: true });
  assert.deepEqual(labels(b), ["Home", "Library", "Schedule", "Build", "QuickAgent", "Validate", "Voice", "Knowledge", "Settings"]);
  assert.deepEqual(lit(b), ["Build", "QuickAgent"]);
  assert.deepEqual(current(b), ["QuickAgent"]);
  const v = rows({ page: "validate", buildOpen: true });
  assert.deepEqual(lit(v), ["Build", "QuickAgent", "Validate"]);
  assert.deepEqual(current(v), ["Validate"]);
  assert.deepEqual(labels(rows({ page: "voice", buildOpen: true })), ["Home", "Library", "Schedule", "Build", "QuickAgent", "Voice", "Knowledge", "Settings"]);
});

test("Build stays lit on its pages when the person has collapsed it", () => {
  const rs = rows({ page: "knowledge", buildOpen: false });
  assert.deepEqual(labels(rs), ["Home", "Library", "Schedule", "Build", "Settings"]);
  assert.deepEqual(lit(rs), ["Build"]);
});

test("a page of Build's opens it", () => {
  for (const p of ["build", "validate", "voice", "knowledge"] as const) assert.equal(inBuild(p), true, p);
  for (const p of ["home", "library", "settings", "scheduled"] as const) assert.equal(inBuild(p), false, p);
});

test("Settings opens its landing and lists every section beneath it while a Settings page is open", () => {
  const landing = rows({ page: "settings" });
  assert.deepEqual(labels(landing), ["Home", "Library", "Schedule", "Build", "Settings", "Profile", "Home", "Licences"]);
  assert.deepEqual(current(landing), ["Settings"]);
  const sec = rows({ page: "settings", section: "home" });
  assert.deepEqual(current(sec), ["Home"]);
  const settings = sec.find((r) => r.key === "settings")!;
  assert.equal(settings.lit, true);
  assert.equal(settings.parent, true);
  assert.deepEqual(settings.children?.find((r) => r.current)?.target, { kind: "section", id: "home" });
  // Elsewhere the sections are not listed.
  assert.equal(rows({ page: "home" }).find((r) => r.key === "settings")?.children, undefined);
});
