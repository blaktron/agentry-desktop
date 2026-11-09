// Add from the library's list: the search, the grouping and the order, the
// add button's words, and the line the list says of itself.
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import type * as library from "../bindings/github.com/blaktron/agentry-desktop/internal/library/models";
import { addCheckedLabel, libraryGroups, libraryStatusLine } from "../src/yourComputer.ts";

const preset = (id: string, application: string, category: string, summary = ""): app.LibraryPreset =>
  ({ id, application, category, summary, package: "", steps: [], exec: [], asks: [], onList: false });

const LIB = [
  preset("unreal", "Unreal Engine", "Game engines", "Work in the open Unreal Editor."),
  preset("blender", "Blender", "3D and animation", "Build scenes and models."),
  preset("godot", "Godot", "Game engines"),
  preset("obsidian", "Obsidian", "Notes and writing", "Read and edit notes in your vault."),
];

test("the list is grouped by category, both in alphabetical order", () => {
  const groups = libraryGroups(LIB, "");
  assert.deepEqual(groups.map((g) => g.category), ["3D and animation", "Game engines", "Notes and writing"]);
  assert.deepEqual(groups[1].presets.map((p) => p.id), ["godot", "unreal"]);
});

test("the search matches the name, the id, the category and the summary, in any case", () => {
  const ids = (q: string) => libraryGroups(LIB, q).flatMap((g) => g.presets.map((p) => p.id));
  assert.deepEqual(ids("BLEN"), ["blender"]);
  assert.deepEqual(ids("game"), ["godot", "unreal"]);
  assert.deepEqual(ids("vault"), ["obsidian"]);
  assert.deepEqual(ids("  "), ["blender", "godot", "unreal", "obsidian"]);
  assert.deepEqual(libraryGroups(LIB, "photoshop"), []);
});

test("the add button counts what is checked", () => {
  assert.equal(addCheckedLabel(0), "Check the applications to add");
  assert.equal(addCheckedLabel(1), "Add 1 application");
  assert.equal(addCheckedLabel(12), "Add 12 applications");
});

test("the list says which copy of the library it is and why a read failed", () => {
  const st = (o: Partial<library.Status>): library.Status => ({ version: 7, presets: 98, origin: "built-in", checked: "", error: "", ...o }) as library.Status;
  assert.equal(libraryStatusLine(st({}), 90), "90 applications for this computer, from library version 7, the copy that came with this version of Agentry Desktop.");
  assert.match(libraryStatusLine(st({ origin: "repository" }), 1), /^1 application for this computer, from library version 7, updated from Agentry's repository\.$/);
  assert.match(libraryStatusLine(st({ error: "offline" }), 2), /could not be read just now: offline$/);
});
