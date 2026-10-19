// The Licences page's reader of the notices file.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { licenceCount, splitNotices } from "../src/licences.ts";

const RULE = "=".repeat(78);
const SUB = "-".repeat(78);

const sample = [
  "Third-party notices for Agentry Desktop",
  "",
  "What this is.",
  "",
  RULE,
  "1. Interface packages",
  RULE,
  "",
  "Some packages.",
  "",
  SUB,
  "left-pad 1.0.0 (MIT)",
  "",
  "MIT text",
  "",
  SUB,
  "right-pad 2.0.0 (ISC)",
  "",
  "ISC text",
  "",
  RULE,
  "2. Fonts",
  RULE,
  "",
  "Font text",
  "",
].join("\n");

test("the preface and each numbered section come apart, without the blank lines around them", () => {
  const n = splitNotices(sample);
  assert.equal(n.preface, "Third-party notices for Agentry Desktop\n\nWhat this is.");
  assert.deepEqual(n.sections.map((s) => s.title), ["1. Interface packages", "2. Fonts"]);
  assert.ok(n.sections[0].body.startsWith("Some packages.\n"));
  assert.ok(n.sections[0].body.endsWith("ISC text"));
  assert.equal(n.sections[1].body, "Font text");
});

test("a licence block is counted by its rule, and a section without blocks holds none", () => {
  const n = splitNotices(sample);
  assert.equal(licenceCount(n.sections[0].body), 2);
  assert.equal(licenceCount(n.sections[1].body), 0);
});

test("a text with no sections is shown whole as the preface, and Windows line ends read the same", () => {
  assert.deepEqual(splitNotices("just text\r\nmore\r\n"), { preface: "just text\nmore", sections: [] });
  assert.deepEqual(splitNotices(sample.replace(/\n/g, "\r\n")), splitNotices(sample));
});

test("a rule of equals signs that opens no title is text, not a heading", () => {
  const n = splitNotices(["intro", RULE, RULE, "after"].join("\n"));
  assert.deepEqual(n, { preface: ["intro", RULE, RULE, "after"].join("\n"), sections: [] });
});

test("the committed notices file reads as the six sections the page lists, none empty", () => {
  const n = splitNotices(readFileSync(new URL("../../THIRD_PARTY_NOTICES", import.meta.url), "utf8"));
  assert.deepEqual(
    n.sections.map((s) => s.title),
    ["1. Interface packages", "2. Go modules", "3. Fonts", "4. Provider logos", "5. Microsoft WebView2 (Windows)", "6. System libraries (Linux and macOS)"],
  );
  for (const s of n.sections) assert.ok(s.body.length > 100, `${s.title} is empty`);
  assert.ok(licenceCount(n.sections[0].body) > 5, "the interface packages carry licence blocks");
  assert.ok(n.sections[0].body.includes("pdfjs-dist"));
  assert.ok(n.sections[0].body.includes("@ungap/structured-clone"));
});
