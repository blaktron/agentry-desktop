import assert from "node:assert/strict";
import test from "node:test";

import { actions, agentOf, cardStatus, clickDoes, copiedLine, heading, iconDate, iconKind, iconName, settingsLine, typeLabel, withheldLine } from "../src/deliverables.ts";

const file = (over: Record<string, unknown> = {}) => ({ name: "report.pdf", title: "", type: "pdf", bytes: 10, canOpen: true, viewKind: "pdf", notOpened: "", copiedTo: "", ...over });

test("a type is named in a person's words, and an unknown one by its extension", () => {
  assert.equal(typeLabel("pdf"), "PDF");
  assert.equal(typeLabel("docx"), "Word document");
  assert.equal(typeLabel("html"), "Web page");
  assert.equal(typeLabel("exe"), ".exe file");
  assert.equal(typeLabel(""), "File");
});

test("a row is headed by the title, else by the file's name", () => {
  assert.equal(heading({ name: "a.md", title: "The week" }), "The week");
  assert.equal(heading({ name: "a.md", title: "  " }), "a.md");
  assert.equal(heading({ name: "a.md" }), "a.md");
});

test("a file a click opens offers Open, and View where the app can show it itself", () => {
  assert.deepEqual(actions(file()), { open: true, view: true, show: true, note: "" });
  assert.equal(actions(file({ type: "md", viewKind: "markdown" })).view, true);
  assert.equal(actions(file({ type: "csv", viewKind: "text" })).view, true);
  // A document another program opens has no View.
  const sheet = actions(file({ type: "xlsx", viewKind: "" }));
  assert.equal(sheet.open, true);
  assert.equal(sheet.view, false);
});

test("a type no click opens is Show in folder only, and says why", () => {
  const exe = actions(file({ type: "exe", canOpen: false, viewKind: "", notOpened: "type" }));
  assert.equal(exe.open, false);
  assert.equal(exe.view, false);
  assert.equal(exe.show, true);
  assert.match(exe.note, /A \.exe file can be acted on by your system, so it is not opened from here\. Show in folder finds it\./);
  const bare = actions(file({ type: "", canOpen: false, viewKind: "", notOpened: "type" }));
  assert.match(bare.note, /^A file with no type/);
});

test("a file whose bytes are not what its type says is not opened, and says so", () => {
  const a = actions(file({ canOpen: false, viewKind: "", notOpened: "contents" }));
  assert.equal(a.open, false);
  assert.match(a.note, /not what its file type says/);
});

test("the line under a file says where it is", () => {
  assert.equal(copiedLine({ copiedTo: "/home/me/Documents/Agentry/a/1" }, true), "Copied to /home/me/Documents/Agentry/a/1");
  assert.match(copiedLine({ copiedTo: "" }, true), /not copied to your Deliverables folder/);
  assert.match(copiedLine({ copiedTo: "" }, false), /Deliverables are off, so this file is in the run folder only/);
});

test("withheld files are counted in the singular and the plural", () => {
  assert.equal(withheldLine(0), "");
  assert.match(withheldLine(1), /^One file the run's record names is not shown: it is not in the run folder, or its contents differ/);
  assert.match(withheldLine(3), /^3 files the run's record names are not shown: they are not in the run folder, or their contents differ/);
});

test("the card's status line names what is missing, and is empty when it lists files", () => {
  const c = { on: true, remember: true, count: 0, error: null, reading: false };
  assert.equal(cardStatus({ ...c, count: 2 }), null);
  assert.match(cardStatus(c)!, /^Nothing yet\./);
  assert.match(cardStatus({ ...c, on: false })!, /^Deliverables are off\./);
  assert.match(cardStatus({ ...c, remember: false })!, /Remember recent runs/);
  assert.equal(cardStatus({ ...c, error: "boom" }), "Deliverables could not be read.");
  assert.equal(cardStatus({ ...c, reading: true }), "Reading deliverables…");
});

test("a card with files lists them even while the history or the switch is off", () => {
  assert.equal(cardStatus({ on: false, remember: false, count: 1, error: null, reading: false }), null);
});

test("Run defaults says what the switch does, and what is wrong when no folder can be named", () => {
  assert.match(settingsLine({ deliverables: true, deliverablesDefault: true, deliverablesDir: "/d" }), /^On\. .*Documents folder/);
  assert.match(settingsLine({ deliverables: true, deliverablesDefault: false, deliverablesDir: "/d" }), /the folder you chose/);
  assert.match(settingsLine({ deliverables: false, deliverablesDefault: true, deliverablesDir: "/d" }), /^Off\./);
  assert.match(settingsLine({ deliverables: true, deliverablesDefault: true, deliverablesError: "no home" }), /cannot name a Documents folder/);
});

const OPENS = { canOpen: true };

test("an icon is drawn by what the file is, and a file no click opens gets the plain one", () => {
  const kinds: Record<string, string> = {
    pdf: "pdf", html: "html", htm: "html", docx: "document", md: "text", txt: "text", json: "text",
    xlsx: "spreadsheet", csv: "spreadsheet", pptx: "presentation", png: "image", jpg: "image", jpeg: "image",
  };
  for (const [type, kind] of Object.entries(kinds)) assert.equal(iconKind({ type, ...OPENS }), kind, type);
  // A type on no list, a macro document, an executable and a file that is not what its name says.
  assert.equal(iconKind({ type: "exe", canOpen: false }), "other");
  assert.equal(iconKind({ type: "docm", canOpen: false }), "other");
  assert.equal(iconKind({ type: "pdf", canOpen: false }), "other");
  assert.equal(iconKind({ type: "html", canOpen: false }), "other");
  assert.equal(iconKind({ type: "", canOpen: false }), "other");
  // The envelope's word for a type is no more than the extension, read as lower case.
  assert.equal(iconKind({ type: "PDF", ...OPENS }), "pdf");
});

test("a click opens a file the app lists as openable and otherwise shows it in its folder", () => {
  assert.equal(clickDoes({ canOpen: true }), "open");
  assert.equal(clickDoes({ canOpen: false }), "show");
});

test("the agent under an icon is the QuickAgent's name, without its publisher or version", () => {
  assert.equal(agentOf("acme/pr-triage@0.4.1"), "pr-triage");
  assert.equal(agentOf("pr-triage"), "pr-triage");
  assert.equal(agentOf("a/b/c@1"), "c");
  assert.equal(agentOf(""), "QuickAgent");
  assert.equal(agentOf(undefined), "QuickAgent");
  assert.equal(agentOf("acme/@1.0.0"), "QuickAgent");
});

test("the date under an icon is the day the run ended", () => {
  const now = new Date("2026-10-18T12:00:00Z");
  assert.equal(iconDate("2026-10-17T09:30:00Z", now, "en-GB", "UTC"), "17 Oct");
  assert.equal(iconDate("2025-12-31T23:30:00Z", now, "en-GB", "UTC"), "31 Dec 2025");
  assert.equal(iconDate("", now, "en-GB", "UTC"), "");
  assert.equal(iconDate("not a date", now, "en-GB", "UTC"), "");
  assert.equal(iconDate(undefined, now, "en-GB", "UTC"), "");
});

test("an icon's name says the file, the agent and the date, and what a click does", () => {
  const n = iconName({ name: "weekly.pdf", canOpen: true }, "pr-triage", "17 Oct");
  assert.equal(n, "weekly.pdf, from pr-triage, 17 Oct. Opens in your system's app.");
  const x = iconName({ name: "setup.exe", canOpen: false }, "pr-triage", "17 Oct");
  assert.match(x, /^setup\.exe, from pr-triage, 17 Oct\. /);
  assert.match(x, /shows it in its folder/i);
  // No date: nothing stands in for it.
  assert.equal(iconName({ name: "a.md", canOpen: true }, "x", ""), "a.md, from x. Opens in your system's app.");
});
