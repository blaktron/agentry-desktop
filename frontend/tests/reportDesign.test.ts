import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import {
  canonical, CONTROLS, cssColour, cssFamily, edit, fieldLabel, fontsFor, HEADING_LABELS, PAGE_NUMBER_LABELS,
  PAPER_LABELS, QUOTE_LABELS, savedSentence, sameDesign, startFrom, THEME_FIELDS, THEME_LOOKS, TITLE_BLOCK_LABELS,
  toDocument, isLogoSrc, type ReportDesign, type ReportFamily,
} from "../src/reportDesign.ts";

// The core's report designs as this app pins them: each theme's canonical
// document as text, the families and the closed lists. The Go test
// TestTheFrontendsReportDesignFixtureIsTheCores holds the file to the
// pinned core, so these tests run against the documents the core ships.
const core = JSON.parse(readFileSync(new URL("./fixtures/reportDesigns.json", import.meta.url), "utf8")) as {
  ids: string[];
  themes: Record<string, string>;
  families: ReportFamily[];
  headingFonts: string[];
  bodyFonts: string[];
  codeFonts: string[];
  papers: string[];
  titleBlocks: string[];
  headingStyles: string[];
  quoteStyles: string[];
  pageNumbers: string[];
};
const theme = (id: string) => JSON.parse(core.themes[id]) as ReportDesign;

// The document's leaf fields, dotted, in its order.
function leaves(v: unknown, at = ""): string[] {
  if (typeof v !== "object" || v === null) return [at];
  return Object.entries(v).flatMap(([k, x]) => leaves(x, at ? `${at}.${k}` : k));
}

// The editor changes the starting theme, the faces, the accent, the
// title block, the headings, the tables, the quotes, the footer and the
// paper, and nothing else; with the name, that is every field of version 2
// but the three a theme sets.
test("the editor's controls are exactly the fields of version 2 a person may change", () => {
  for (const id of core.ids) {
    const fields = leaves(theme(id)).filter((f) => !THEME_FIELDS.includes(f));
    assert.deepEqual(CONTROLS.map((c) => c.field), fields, id);
  }
  assert.deepEqual(THEME_FIELDS.filter((f) => !leaves(theme("plain")).includes(f)), []);
  // A design with a logo has one field more, which no
  // control sets and the save never sends: the logo is uploaded on its own.
  const withLogo = { ...theme("modern"), logo: { png: "iVBORw0KGgo=" } } as ReportDesign;
  const fields = leaves(withLogo).filter((f) => !THEME_FIELDS.includes(f) && f.split(".")[0] !== "logo");
  assert.deepEqual(CONTROLS.map((c) => c.field), fields);
  assert.deepEqual(leaves(withLogo).filter((f) => f.split(".")[0] === "logo"), ["logo.png"]);
  assert.equal("logo" in toDocument(withLogo), false);
  assert.equal(canonical(withLogo), canonical(theme("modern")));
});

// The logo is drawn only as a PNG data: URL: never a
// URL, another type, or anything that is not base64.
test("the page draws a logo only from a PNG data: URL", () => {
  assert.equal(isLogoSrc("data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="), true);
  for (const bad of [
    "https://evil.example/logo.png", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,R0lGODlh",
    'data:image/png;base64,iVBORw0KGgo"><script>', "data:image/png,iVBORw0KGgo", "", undefined, null, 42,
  ]) {
    assert.equal(isLogoSrc(bad), false, String(bad));
  }
});

// Invariant 8: each theme, loaded into the editor, every control set to
// what it shows, and handed to the save, is byte for byte the document the
// core ships.
test("each theme loads into the editor and saves unchanged, byte for byte (invariant 8)", () => {
  assert.deepEqual(core.ids, ["plain", "classic", "modern", "editorial", "technical"]);
  for (const id of core.ids) {
    let d = startFrom(theme(id));
    for (const c of CONTROLS) d = edit(d, c.field, c.get(d));
    // Through the binding: Wails sends the argument as JSON.
    const sent = JSON.parse(JSON.stringify(toDocument(d))) as ReportDesign;
    assert.equal(canonical(sent), core.themes[id], id);
    assert.ok(sameDesign(sent, theme(id)), id);
  }
});

test("an edit changes only the field it names, and never the theme it started from", () => {
  const plain = theme("plain");
  const start = startFrom(plain);
  let d = edit(start, "fonts.body", "Lora");
  d = edit(d, "tables.banded", true);
  d = edit(d, "accent", "  #1F6FEB ");
  d = edit(d, "footer.text", "Acme & Co <internal>");
  assert.equal(d.fonts.body, "Lora");
  assert.equal(d.fonts.headings, plain.fonts.headings);
  assert.equal(d.tables.banded, true);
  assert.equal(d.tables.tinted, plain.tables.tinted);
  assert.equal(d.accent, "#1f6feb", "the core keeps an accent in lower case");
  assert.equal(d.footer.pageNumbers, plain.footer.pageNumbers);
  assert.equal(canonical(start), core.themes.plain, "the editor's starting copy is untouched");
  assert.equal(canonical(theme("plain")), core.themes.plain);
  assert.ok(!sameDesign(d, plain));
  assert.throws(() => edit(d, "basedOn", "modern"), /no control/);
});

test("the save sends the document's fields and nothing else", () => {
  const d = { ...theme("modern"), logo: "x", extra: 1 } as unknown as ReportDesign;
  assert.deepEqual(Object.keys(toDocument(d)), Object.keys(theme("modern")));
  assert.equal(canonical(d), core.themes.modern);
});

test("every value of the core's lists has words, and every theme its look", () => {
  const lists: [string[], Record<string, string>][] = [
    [core.papers, PAPER_LABELS], [core.titleBlocks, TITLE_BLOCK_LABELS], [core.headingStyles, HEADING_LABELS],
    [core.quoteStyles, QUOTE_LABELS], [core.pageNumbers, PAGE_NUMBER_LABELS],
  ];
  for (const [values, labels] of lists) assert.deepEqual(Object.keys(labels), values);
  assert.deepEqual(Object.keys(THEME_LOOKS), core.ids);
  for (const c of CONTROLS) assert.doesNotMatch(fieldLabel(c.field), /\./, c.field);
});

test("each role offers the core's faces", () => {
  assert.deepEqual(fontsFor("headings", core), core.families.map((f) => f.name));
  assert.ok(!fontsFor("body", core).includes("Playfair Display"));
  assert.deepEqual(fontsFor("code", core), core.families.filter((f) => f.kind === "mono").map((f) => f.name));
});

// The preview's styles name only the core's families and a hex accent: a
// design's text is never written into a style.
test("the preview sets only the core's families, as the app's renamed web fonts, and only a hex colour", () => {
  assert.equal(cssFamily("Lora", core.families), '"Report Lora", Georgia, serif');
  assert.equal(cssFamily("JetBrains Mono", core.families), '"Report JetBrains Mono", ui-monospace, monospace');
  assert.equal(cssFamily('Inter"; background: url(x)', core.families), "system-ui, sans-serif");
  assert.equal(cssColour("#0f766e"), "#0f766e");
  assert.equal(cssColour("red; background: url(x)"), "#000000");
});

test("what a save and a reset say", () => {
  assert.equal(savedSentence("House (from Modern)"), "Saved. Your runs' PDF reports use House (from Modern).");
  assert.match(savedSentence(null), /no report design, and no logo: runs from this app use Plain/);
});

// The preview's web fonts (scripts/sync-report-fonts.sh): every file is
// the one the provenance names, every family of the core's list has a
// face here under its "Report " name, and each has its licence.
test("the preview's web fonts cover every family, each file as its provenance says, each with its licence", () => {
  const dir = new URL("../src/reportFonts/", import.meta.url);
  const prov = JSON.parse(readFileSync(new URL("PROVENANCE.json", dir), "utf8")) as { files: { family: string; file: string; sha256: string }[] };
  for (const f of prov.files) {
    const sum = createHash("sha256").update(readFileSync(new URL(f.file, dir))).digest("hex");
    assert.equal(sum, f.sha256, f.file);
  }
  const css = readFileSync(new URL("fonts.css", dir), "utf8");
  for (const fam of core.families) {
    assert.ok(prov.files.some((f) => f.family === fam.name), `${fam.name} has no file`);
    assert.ok(css.includes(`font-family: "Report ${fam.name}";`), `${fam.name} has no face`);
  }
  assert.doesNotMatch(css, /font-family: "(?!Report )/, "every face is renamed");
  for (const slug of new Set(prov.files.map((f) => f.file.split("-")[0]))) {
    assert.ok(existsSync(new URL(`licenses/${slug}.OFL.txt`, dir)), `${slug} has no licence`);
  }
});
