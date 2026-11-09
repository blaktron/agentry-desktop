// The Build form's capabilities.desktop: turning an application on and off,
// narrowing its tools, and setting computer use keep the row the Go side
// writes, and a row with nothing left in it is no line at all.
import { test } from "node:test";
import assert from "node:assert/strict";
import * as build from "../bindings/github.com/blaktron/agentry-desktop/internal/build/models";
import { DESKTOP_ID, withChooser, withComputerUse, withNarrowing, withServer, withTools, withUse } from "../src/pages/BuildDesktop";

const ALL = ["execute_blender_code", "look", "get_scene_info"];

test("an application turned on starts with every tool, and off leaves no line", () => {
  const on = withServer(undefined, "blender", true)!;
  assert.deepEqual(on.servers.map((s) => [s.id, s.tools]), [["blender", []]]);
  assert.equal(withServer(on, "blender", true)!.servers.length, 1);
  assert.equal(withServer(on, "blender", false), undefined);
});

test("a narrowing keeps the catalogue's order, and every tool is no narrowing", () => {
  const on = withServer(undefined, "blender", true)!;
  const narrowed = withTools(on, "blender", ["get_scene_info", "look"], ALL);
  assert.deepEqual(narrowed.servers[0].tools, ["look", "get_scene_info"]);
  const whole = withTools(narrowed, "blender", [...ALL].reverse(), ALL);
  assert.deepEqual(whole.servers[0].tools, []);
});

test("computer use alone is a line; clearing both is none", () => {
  const cu = withComputerUse(undefined, "app")!;
  assert.equal(cu.computerUse, "app");
  assert.deepEqual(cu.servers, []);
  assert.equal(withComputerUse(cu, ""), undefined);
  const both = withServer(cu, "blender", true)!;
  const off = withServer(both, "blender", false)!;
  assert.equal(off.computerUse, "app");
  assert.ok(withComputerUse(both, "") instanceof build.DesktopRow);
});

test("any application by id, its use, and an application the person chooses", () => {
  let row = withServer(undefined, "my-cad", true)!;
  row = withNarrowing(row, "my-cad", "export_drawing, open_file  export_drawing");
  assert.deepEqual(row.servers[0].tools, ["export_drawing", "open_file"]);
  row = withComputerUse(row, "app")!;
  row = withUse(row, "my-cad", "computer");
  assert.equal(row.servers[0].use, "computer");
  assert.deepEqual(row.servers[0].tools, [], "a windows-only entry carries no narrowing");
  row = withChooser(row, true)!;
  assert.equal(row.servers.filter((s) => s.choose).length, 1);
  assert.equal(withChooser(row, true)!.servers.filter((s) => s.choose).length, 1, "at most one chooser");
  row = withUse(row, "", "both");
  assert.equal(row.servers.find((s) => s.choose)!.use, "both");
  assert.equal(row.servers.find((s) => !s.choose)!.use, "computer", "the chooser's use leaves the others");
  row = withServer(row, "my-cad", false)!;
  assert.equal(row.servers.length, 1, "removing an id keeps the chooser");
  assert.ok(DESKTOP_ID.test("my-cad") && !DESKTOP_ID.test("My CAD") && !DESKTOP_ID.test("-x"));
});
