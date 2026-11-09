// The words for the applications on this computer: the state line, the run
// setup's blockers, a recent run's record.
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as linkservice from "../bindings/github.com/blaktron/agentry-desktop/internal/linkservice/models";
import { applicationBlockers, classWords, execCount, fromWords, recentLine, runAppName, stateLine } from "../src/yourComputer.ts";

const blender = (v: Partial<linkservice.AppState>) => ({
  id: "blender", application: "Blender", offered: true, package: "pypi:mcp-for-blender@2.2.7", attachment: "stdio",
  launcher: "uvx", launcherFound: true, port: 9876, listening: true, screeningOff: false,
  tools: [{ name: "execute_blender_code", class: "exec" }, { name: "look", class: "read" }], egress: [], steps: [],
  ready: true, ...v,
}) as unknown as linkservice.AppState;

test("a ready application says where it answers; one that is not says the step to take", () => {
  assert.equal(stateLine(blender({})), "Ready: Blender answers on port 9876, and uvx is on this computer.");
  const closed = blender({ ready: false, listening: false, missing: "Blender does not answer on port 9876 on this computer: open Blender and start its MCP server." });
  assert.equal(stateLine(closed), closed.missing);
});

const runApp = (v: Partial<app.RunApplication>) => ({ entry: "blender", use: "mcp", chosen: false, candidates: [], ready: true, ...v }) as unknown as app.RunApplication;

test("the run setup blocks Run for each application that is not ready, and for none In Agentry or while reading", () => {
  const closed = runApp({ resolved: blender({ ready: false }), ready: false, missing: "Blender does not answer on port 9876 on this computer." });
  assert.deepEqual(applicationBlockers([closed], false), ["Blender on your computer: Blender does not answer on port 9876 on this computer."]);
  assert.deepEqual(applicationBlockers([runApp({ resolved: blender({}) })], false), []);
  assert.deepEqual(applicationBlockers([closed], true), []);
  assert.deepEqual(applicationBlockers(null, false), []);
  const unset = runApp({ entry: "", ready: false, missing: "This QuickAgent uses an application you choose: choose one of yours." });
  assert.deepEqual(applicationBlockers([unset], false), ["An application you choose on your computer: This QuickAgent uses an application you choose: choose one of yours."]);
});

test("an application the run names is called by the person's application, the preset, or the release's id", () => {
  assert.equal(runAppName(runApp({ entry: "cad", resolved: { ...blender({}), application: "My CAD" } as linkservice.AppState })), "My CAD");
  assert.equal(runAppName(runApp({ preset: "Blender" })), "Blender");
  assert.equal(runAppName(runApp({ entry: "my-cad" })), "my-cad");
  assert.equal(fromWords(blender({ from: "person", fromWords: "" })), "set up by you; Agentry has not vetted it");
});

test("the classes in the person's words, and the tools that run code", () => {
  assert.equal(classWords("exec"), "runs code or is not classed, asked first");
  assert.equal(classWords("read"), "reads");
  assert.equal(execCount(blender({})), 1);
});

test("a recent run's record names its calls, the exec questions and how it ended", () => {
  const r = {
    run: "R", started: "2026-11-07T12:00:00Z", state: "ended", execAsked: 2, execAllowed: 1, execDeclined: 1, execForRun: true,
    servers: [{ id: "blender", application: "Blender", package: "", launch: "", calls: { read: 3, exec: 2 }, refused: { "desktop-declined": 1 }, errors: 0 }],
  } as unknown as linkservice.RecentRun;
  assert.equal(recentLine(r), "Blender: 5 calls (3 read, 2 exec); refused: 1 desktop-declined; asked 2 times to run code: 1 allowed, 1 declined, then every exec call allowed for the run; ended");
});

test("a run Agentry Desktop quit under says so, never that it is still using the computer", () => {
  const r = { run: "R", started: "2026-11-07T12:00:00Z", state: "interrupted", execAsked: 0, execAllowed: 0, execDeclined: 0, execForRun: false, servers: [] } as unknown as linkservice.RecentRun;
  assert.equal(recentLine(r), "Agentry Desktop quit before the run ended");
});
