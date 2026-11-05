import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import {
  canonical, CONTROLS, COVER_FACT_LABELS, COVER_STYLE_LABELS, COVER_WHEN_LABELS, cssColour, cssFamily, designChoices, edit, fieldLabel,
  fontsFor, HEADER_STYLE_LABELS, HEADING_LABELS, HEADING_SCALE_LABELS, insertToken, isImageSrc, isLogoSrc, KIT_FIELDS, KIT_LOOKS,
  librarySentence, MARGIN_LABELS, newFromKit, PAGE_NUMBER_LABELS, PAPER_LABELS, QUOTE_LABELS, sameDesign, savedSentence, showFact,
  startFrom, TITLE_BLOCK_LABELS, toDocument, TOKEN_LABELS, type ReportDesign, type ReportFamily,
} from "../src/reportDesign.ts";

// The core's report designs as this app pins them: each kit's canonical
// document as text, the families, the closed lists, the tokens and the
// limits. The Go test TestTheFrontendsReportDesignFixtureIsTheCores holds the
// file to the pinned core, so these tests run against the documents the core
// ships.
const core = JSON.parse(readFileSync(new URL("./fixtures/reportDesigns.json", import.meta.url), "utf8")) as {
  ids: string[];
  kits: Record<string, string>;
  families: ReportFamily[];
  headingFonts: string[];
  bodyFonts: string[];
  codeFonts: string[];
  papers: string[];
  titleBlocks: string[];
  headingStyles: string[];
  headingScales: string[];
  margins: string[];
  quoteStyles: string[];
  pageNumbers: string[];
  headerStyles: string[];
  coverStyles: { name: string }[];
  coverFacts: string[];
  coverWhens: string[];
  tokens: string[];
  maxName: number;
  maxLine: number;
  maxContactLines: number;
  slotCount: number;
  maxLibrary: number;
  defaultKitId: string;
};
const kit = (id: string) => JSON.parse(core.kits[id]) as ReportDesign;

// The keys of a document in order, dotted, lists as one key.
function keys(v: unknown, at = ""): string[] {
  if (typeof v !== "object" || v === null || Array.isArray(v)) return [at];
  return Object.entries(v).flatMap(([k, x]) => keys(x, at ? `${at}.${k}` : k));
}

// Each kit, loaded into the editor, every control set to what it shows, and
// handed to the save, is byte for byte the document the core ships: the
// editor's controls cover every field of version 3 a person may change, and
// the document it sends has the core's keys in the core's order.
test("each kit loads into the editor and saves unchanged, byte for byte", () => {
  assert.deepEqual(core.ids, ["plain", "classic", "modern", "editorial", "technical", "boardroom", "studio", "field", "civic", "lumen"]);
  assert.equal(core.defaultKitId, "plain");
  for (const id of core.ids) {
    let d = startFrom(kit(id));
    for (const c of CONTROLS) d = edit(d, c.field, c.get(d));
    // Through the binding: Wails sends the argument as JSON.
    const sent = JSON.parse(JSON.stringify(toDocument(d))) as ReportDesign;
    assert.equal(canonical(sent), core.kits[id], id);
    assert.ok(sameDesign(sent, kit(id)), id);
    assert.deepEqual(keys(sent), keys(JSON.parse(core.kits[id])), `${id}: the keys and their order`);
  }
});

// Every leaf of the document but the three a kit sets has a control: the
// slots one each, the contact lines and the cover's facts one control for
// the list.
test("the editor's controls are exactly the fields of version 3 a person may change", () => {
  const fields = CONTROLS.map((c) => c.field);
  const leaves = keys(kit("boardroom")).filter((f) => !KIT_FIELDS.includes(f));
  const asControl = (f: string) => (f.startsWith("brand.contact") ? "brand.contact" : f.startsWith("cover.shows") ? "cover.shows" : f);
  assert.deepEqual(fields, [...new Set(leaves.map(asControl))].flatMap((f) =>
    f === "header.slots" || f === "footer.slots" ? Array.from({ length: core.slotCount }, (_, i) => `${f}.${i}`) : [f]));
  assert.deepEqual(KIT_FIELDS.filter((f) => !keys(kit("plain")).includes(f)), []);
  // A design with images has keys no control sets and the save never
  // sends: each image is uploaded on its own.
  const withImages = { ...kit("modern"), logo: { png: "iVBORw0KGgo=" }, coverImage: { jpeg: "/9j/" }, customCover: { source: "x", images: [] } } as ReportDesign;
  assert.equal("logo" in toDocument(withImages), false);
  assert.equal("coverImage" in toDocument(withImages), false);
  assert.equal("customCover" in toDocument(withImages), false);
  assert.equal(canonical(withImages), core.kits.modern);
});

test("an edit changes only the field it names, and never the kit it started from", () => {
  const plain = kit("plain");
  const start = startFrom(plain);
  let d = edit(start, "fonts.body", "Lora");
  d = edit(d, "tables.banded", true);
  d = edit(d, "accent", "  #1F6FEB ");
  d = edit(d, "secondary", "#8A6A00");
  d = edit(d, "footer.slots.0", "Acme & Co <internal>");
  d = edit(d, "header.slots.2", "{page} of {pages}");
  d = edit(d, "brand.contact", "1 Main Street\n\n  review@acme.example  \nline three\nline four\nline five");
  d = edit(d, "brand.website", " acme.example ");
  d = edit(d, "cover.when", "always");
  assert.equal(d.fonts.body, "Lora");
  assert.equal(d.fonts.headings, plain.fonts.headings);
  assert.equal(d.tables.banded, true);
  assert.equal(d.tables.tinted, plain.tables.tinted);
  assert.equal(d.accent, "#1f6feb", "the core keeps a colour in lower case");
  assert.equal(d.secondary, "#8a6a00");
  assert.deepEqual(d.footer.slots, ["Acme & Co <internal>", "", ""]);
  assert.deepEqual(d.header.slots, ["", "", "{page} of {pages}"]);
  assert.deepEqual(d.brand.contact, ["1 Main Street", "review@acme.example", "line three", "line four"], "four lines at most, blanks dropped");
  assert.equal(d.brand.website, "acme.example");
  assert.equal(d.cover.when, "always");
  assert.equal(d.footer.pageNumbers, plain.footer.pageNumbers);
  assert.equal(canonical(start), core.kits.plain, "the editor's starting copy is untouched");
  assert.equal(canonical(kit("plain")), core.kits.plain);
  assert.ok(!sameDesign(d, plain));
  assert.throws(() => edit(d, "basedOn", "modern"), /no control/);
});

test("the cover's facts keep the core's order whichever box was ticked last", () => {
  let d = kit("plain"); // title, subtitle, author, date
  d = showFact(d, core.coverFacts, "logo", true);
  d = showFact(d, core.coverFacts, "brand", true);
  d = showFact(d, core.coverFacts, "subtitle", false);
  assert.deepEqual(d.cover.shows, ["title", "author", "date", "brand", "logo"]);
  d = showFact(d, core.coverFacts, "brand", true);
  assert.deepEqual(d.cover.shows, ["title", "author", "date", "brand", "logo"], "a fact is named once");
});

test("the token picker writes a token where the caret is", () => {
  assert.equal(insertToken("Page  of", 5, "page"), "Page {page} of");
  assert.equal(insertToken("", 0, "brand"), "{brand}");
  assert.equal(insertToken("abc", 99, "date"), "abc{date}");
  assert.equal(insertToken("abc", -1, "date"), "{date}abc");
});

test("a new design from a kit is the kit's document under the person's name", () => {
  const d = newFromKit(kit("lumen"), "  Quarterly  ");
  assert.equal(d.name, "Quarterly");
  assert.equal(canonical({ ...d, name: "Lumen" }), core.kits.lumen);
  assert.equal(newFromKit(kit("lumen"), "   ").name, "Lumen");
});

test("the save sends the document's fields and nothing else", () => {
  const d = { ...kit("modern"), logo: "x", extra: 1 } as unknown as ReportDesign;
  assert.deepEqual(Object.keys(toDocument(d)), Object.keys(kit("modern")));
  assert.equal(canonical(d), core.kits.modern);
});

test("every value of the core's lists has words, every kit its look, every token its words", () => {
  const lists: [string[], Record<string, string>][] = [
    [core.papers, PAPER_LABELS], [core.titleBlocks, TITLE_BLOCK_LABELS], [core.headingStyles, HEADING_LABELS],
    [core.headingScales, HEADING_SCALE_LABELS], [core.margins, MARGIN_LABELS], [core.quoteStyles, QUOTE_LABELS],
    [core.pageNumbers, PAGE_NUMBER_LABELS], [core.headerStyles, HEADER_STYLE_LABELS],
    [core.coverStyles.map((c) => c.name), COVER_STYLE_LABELS], [core.coverFacts, COVER_FACT_LABELS], [core.coverWhens, COVER_WHEN_LABELS],
    [core.tokens, TOKEN_LABELS],
  ];
  for (const [values, labels] of lists) assert.deepEqual(Object.keys(labels), values);
  assert.deepEqual(Object.keys(KIT_LOOKS), core.ids);
  for (const c of CONTROLS) assert.doesNotMatch(fieldLabel(c.field), /\./, c.field);
  assert.equal(fieldLabel("footer.slots.2"), "the footer's right slot");
  assert.equal(fieldLabel("brand.contact.1"), "contact line 2");
  assert.equal(fieldLabel("cover.shows.3"), "the cover's facts");
});

test("each role offers the core's faces", () => {
  assert.deepEqual(fontsFor("headings", core), core.families.map((f) => f.name));
  assert.ok(!fontsFor("body", core).includes("Playfair Display"));
  assert.ok(!fontsFor("body", core).includes("Fraunces"));
  assert.deepEqual(fontsFor("code", core), core.families.filter((f) => f.kind === "mono").map((f) => f.name));
  assert.equal(core.families.length, 22);
});

// The pages draw an image only from a data: URL of a PNG or a JPEG: never
// a URL, another type, or anything that is not base64.
test("the pages draw a logo only from a PNG data: URL, and a cover image from a PNG or a JPEG", () => {
  assert.equal(isLogoSrc("data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="), true);
  assert.equal(isImageSrc("data:image/jpeg;base64,/9j/4AAQSkZJRg=="), true);
  assert.equal(isLogoSrc("data:image/jpeg;base64,/9j/4AAQSkZJRg=="), false);
  for (const bad of [
    "https://evil.example/logo.png", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,R0lGODlh",
    'data:image/png;base64,iVBORw0KGgo"><script>', "data:image/png,iVBORw0KGgo", "", undefined, null, 42,
  ]) {
    assert.equal(isLogoSrc(bad), false, String(bad));
    assert.equal(isImageSrc(bad), false, String(bad));
  }
});

// The preview's styles name only the core's families and a hex colour: a
// design's text is never written into a style.
test("the preview sets only the core's families, as the app's renamed web fonts, and only a hex colour", () => {
  assert.equal(cssFamily("Lora", core.families), '"Report Lora", Georgia, serif');
  assert.equal(cssFamily("Fira Code", core.families), '"Report Fira Code", ui-monospace, monospace');
  assert.equal(cssFamily('Inter"; background: url(x)', core.families), "system-ui, sans-serif");
  assert.equal(cssColour("#0f766e"), "#0f766e");
  assert.equal(cssColour("red; background: url(x)"), "#000000");
  assert.equal(cssColour("", "#ffffff"), "#ffffff");
});

// The run setup's picker: the library's designs, the default preselected,
// and what leaving it means; no library or none is no picker.
test("the picker preselects the account's default design", () => {
  const lib = { default: "b", designs: [{ id: "a", name: "Field notes", default: false }, { id: "b", name: "House", default: true }] };
  const p = designChoices(lib);
  assert.equal(p.defaultId, "b");
  assert.equal(p.none, "Your default, House");
  assert.deepEqual(p.choices.map((c) => c.label), ["Field notes", "House (your default)"]);
  const noDefault = designChoices({ default: "", designs: [{ id: "a", name: "Field notes", default: false }] });
  assert.equal(noDefault.defaultId, "");
  assert.match(noDefault.none, /Plain/);
  assert.deepEqual(designChoices(null).choices, []);
  assert.deepEqual(designChoices({ default: "", designs: [] }).choices, []);
});

test("what a save and the library say", () => {
  assert.equal(savedSentence("House (from Boardroom)", true), "Saved. Runs that pick no design use House (from Boardroom).");
  assert.equal(savedSentence("House", false), "Saved. Pick House in a run's setup, or make it the default.");
  assert.match(librarySentence(0, null, 10), /no report design yet: runs from this app use Plain/);
  assert.equal(librarySentence(3, "House", 10), "3 of 10 designs. Runs that pick no design use House; the run setup lets you pick another.");
  assert.equal(librarySentence(2, null, 10), "2 of 10 designs. No default: runs that pick no design use Plain.");
});

// The preview's web fonts (src/reportFonts/, which scripts/fonts/check.mjs
// holds to its provenance as well): every file is the one the provenance
// names, every family of the core's list has a face here under its "Report "
// name, and each has its licence.
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
