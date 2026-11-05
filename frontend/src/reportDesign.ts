// The Report design pages' model. The account keeps a library of report
// designs, each a theme document version 3; Settings → Report design lists
// them and edits one, and every rule the editor shows is the core's
// pkg/reportdesign answering through Go (SettingsAPI.CheckReportDesign), so
// nothing here decides what can be saved. This is what the pages draw from:
// the editor's controls, which are exactly the fields of version 3 a person
// may change, the words each choice is shown with, the token picker's
// tokens, and the faces and colours the preview sets.
//
// Pure: the pages and node's tests import it.

/** A report design, theme document version 3, as the core's JSON has it,
 *  without its images (the logo, the cover image and the custom cover come
 *  beside it from Go, never in the document the editor holds). */
export interface ReportDesign {
  agentryTheme: number;
  name: string;
  basedOn: string;
  version: number;
  paper: string;
  fonts: { headings: string; body: string; code: string };
  accent: string;
  secondary: string;
  titleBlock: string;
  headings: string;
  headingScale: string;
  margins: string;
  tables: { tinted: boolean; banded: boolean };
  quotes: string;
  brand: { name: string; strapline: string; contact: string[]; website: string; confidentiality: string };
  header: { style: string; slots: string[] };
  footer: { pageNumbers: string; slots: string[] };
  cover: { style: string; shows: string[]; when: string };
}

export type FontRole = keyof ReportDesign["fonts"];

/** A family the Typst image carries, as Go's ReportDesignOptions lists it. */
export interface ReportFamily {
  name: string;
  kind: string;
  headingsOnly?: boolean;
}

/** A cover style and which colours carry text on it, as Go lists them. */
export interface CoverStyleInfo {
  name: string;
  accentOnText: boolean;
  secondaryOnText: boolean;
}

/** The slots of a header or a footer: three, left, centre, right. */
export const SLOT_COUNT = 3;
export const SLOT_NAMES = ["Left", "Centre", "Right"];

/** What each kit looks like, by id; one this list lacks is shown by its
 *  name alone. */
export const KIT_LOOKS: Record<string, string> = {
  plain: "Near-monochrome Inter, a ruled title, plain headings, a framed cover.",
  classic: "Source Serif, navy small-capital headings, a ruled header.",
  modern: "Inter on a blue title band, accent bars, banded tables, a band cover.",
  editorial: "Playfair Display headings over Source Sans, warm red rules, a large-type cover.",
  technical: "IBM Plex, teal numbered headings, banded tables, a geometric cover.",
  boardroom: "Libre Baskerville over Source Serif, navy and gold, a split cover on every report.",
  studio: "Space Grotesk over DM Sans, black and yellow, the logo in the header.",
  field: "Fraunces over Nunito Sans, forest green and sand, a plate cover for your image.",
  civic: "Crimson Pro, burgundy and warm grey, a framed cover on every report.",
  lumen: "Raleway, violet and mint, a geometric cover on every report.",
};

// The words each closed list's values are shown with. A value a later core
// adds is shown as itself.
export const PAPER_LABELS: Record<string, string> = { a4: "A4", letter: "US Letter" };
export const TITLE_BLOCK_LABELS: Record<string, string> = {
  band: "Band: the title in white on the accent",
  bar: "Bar: an accent bar beside the title",
  ruled: "Ruled: accent rules above and below the title",
  none: "None: the title alone",
};
export const HEADING_LABELS: Record<string, string> = {
  rule: "Rule: an accent rule under each heading",
  bar: "Bar: a short accent bar above each heading",
  smallcaps: "Small capitals, in the accent",
  numbered: "Numbered, in the accent",
  plain: "Plain: bold and near-black",
};
export const HEADING_SCALE_LABELS: Record<string, string> = { compact: "Compact", regular: "Regular", generous: "Generous" };
export const MARGIN_LABELS: Record<string, string> = { narrow: "Narrow", regular: "Regular", wide: "Wide" };
export const QUOTE_LABELS: Record<string, string> = {
  callout: "Callout: a tinted box with an accent edge",
  plain: "Plain: a thin grey edge",
};
export const PAGE_NUMBER_LABELS: Record<string, string> = { none: "None", n: "1, 2, 3", nOfN: "1 of 3, 2 of 3" };
export const HEADER_STYLE_LABELS: Record<string, string> = {
  none: "None",
  rule: "Rule: an accent rule over the slots",
  brandLine: "Brand line: your brand's name at the left, a slot at the right",
  titleAndPage: "Title and page: the title at the left, the page at the right",
  logoLeft: "Logo left: your logo small at the left, a slot at the right",
};
export const COVER_STYLE_LABELS: Record<string, string> = {
  band: "Band: the title in white on an accent panel",
  split: "Split: a secondary panel down the left with your brand, the title on white",
  geometric: "Geometric: an ink title with accent and secondary shapes",
  largeType: "Large type: the title at display size in the accent",
  framed: "Framed: a hairline frame, the title centred",
  plate: "Plate: your cover image across the top half",
  custom: "Custom: your own cover, drawn by its code",
};
export const COVER_FACT_LABELS: Record<string, string> = {
  title: "The title",
  subtitle: "The subtitle",
  author: "The author",
  date: "The date",
  brand: "Your brand",
  logo: "Your logo",
  confidentiality: "The confidentiality line",
};
export const COVER_WHEN_LABELS: Record<string, string> = {
  long: "On a long report",
  always: "On every titled report",
  never: "Never",
};
export const TOKEN_LABELS: Record<string, string> = {
  title: "the title",
  author: "the author",
  date: "the date",
  page: "the page number",
  pages: "the page count",
  brand: "your brand's name",
  section: "the current section",
  confidentiality: "the confidentiality line",
};

export const labelOf = (labels: Record<string, string>, value: string) => labels[value] ?? value;

/** A token as a slot writes it. */
export const tokenText = (token: string) => `{${token}}`;

/** A control of the editor: one field of the document, read and set. */
export interface Control {
  field: string;
  get: (d: ReportDesign) => string | boolean;
  set: (d: ReportDesign, v: string | boolean) => ReportDesign;
}

const text = (v: string | boolean) => (typeof v === "string" ? v : String(v));
const flag = (v: string | boolean) => v === true;
const slot = (slots: string[], i: number, v: string): string[] => {
  const out = Array.from({ length: SLOT_COUNT }, (_, k) => slots[k] ?? "");
  out[i] = v;
  return out;
};

/** The contact lines as one control holds them: one per line, at most four
 *  as the core allows, an empty line dropped. */
export const CONTACT_JOIN = "\n";
export function contactLines(v: string): string[] {
  return v.split(/\r?\n/).map((l) => l.trim()).filter((l) => l !== "").slice(0, 4);
}

/** The editor's controls, one per field of version 3 the person may change:
 * the name, the paper, the three faces, the two colours, the title block,
 * the heading treatment and scale, the margins, the two table styles, the
 * quotes, the brand's lines, the header's style and three slots, the
 * footer's page numbers and three slots, and the cover's style, facts and
 * when. The document's version, its kit and its own revision are not
 * controls: choosing a kit sets them. */
export const CONTROLS: Control[] = [
  { field: "name", get: (d) => d.name, set: (d, v) => ({ ...d, name: text(v) }) },
  { field: "paper", get: (d) => d.paper, set: (d, v) => ({ ...d, paper: text(v) }) },
  ...(["headings", "body", "code"] as FontRole[]).map((role): Control => ({
    field: `fonts.${role}`,
    get: (d) => d.fonts[role],
    set: (d, v) => ({ ...d, fonts: { ...d.fonts, [role]: text(v) } }),
  })),
  // The core keeps a colour in lower case; the editor writes it so.
  { field: "accent", get: (d) => d.accent, set: (d, v) => ({ ...d, accent: text(v).trim().toLowerCase() }) },
  { field: "secondary", get: (d) => d.secondary, set: (d, v) => ({ ...d, secondary: text(v).trim().toLowerCase() }) },
  { field: "titleBlock", get: (d) => d.titleBlock, set: (d, v) => ({ ...d, titleBlock: text(v) }) },
  { field: "headings", get: (d) => d.headings, set: (d, v) => ({ ...d, headings: text(v) }) },
  { field: "headingScale", get: (d) => d.headingScale, set: (d, v) => ({ ...d, headingScale: text(v) }) },
  { field: "margins", get: (d) => d.margins, set: (d, v) => ({ ...d, margins: text(v) }) },
  { field: "tables.tinted", get: (d) => d.tables.tinted, set: (d, v) => ({ ...d, tables: { ...d.tables, tinted: flag(v) } }) },
  { field: "tables.banded", get: (d) => d.tables.banded, set: (d, v) => ({ ...d, tables: { ...d.tables, banded: flag(v) } }) },
  { field: "quotes", get: (d) => d.quotes, set: (d, v) => ({ ...d, quotes: text(v) }) },
  { field: "brand.name", get: (d) => d.brand.name, set: (d, v) => ({ ...d, brand: { ...d.brand, name: text(v) } }) },
  { field: "brand.strapline", get: (d) => d.brand.strapline, set: (d, v) => ({ ...d, brand: { ...d.brand, strapline: text(v) } }) },
  { field: "brand.contact", get: (d) => d.brand.contact.join(CONTACT_JOIN), set: (d, v) => ({ ...d, brand: { ...d.brand, contact: contactLines(text(v)) } }) },
  { field: "brand.website", get: (d) => d.brand.website, set: (d, v) => ({ ...d, brand: { ...d.brand, website: text(v).trim() } }) },
  { field: "brand.confidentiality", get: (d) => d.brand.confidentiality, set: (d, v) => ({ ...d, brand: { ...d.brand, confidentiality: text(v) } }) },
  { field: "header.style", get: (d) => d.header.style, set: (d, v) => ({ ...d, header: { ...d.header, style: text(v) } }) },
  ...Array.from({ length: SLOT_COUNT }, (_, i): Control => ({
    field: `header.slots.${i}`,
    get: (d) => d.header.slots[i] ?? "",
    set: (d, v) => ({ ...d, header: { ...d.header, slots: slot(d.header.slots, i, text(v)) } }),
  })),
  { field: "footer.pageNumbers", get: (d) => d.footer.pageNumbers, set: (d, v) => ({ ...d, footer: { ...d.footer, pageNumbers: text(v) } }) },
  ...Array.from({ length: SLOT_COUNT }, (_, i): Control => ({
    field: `footer.slots.${i}`,
    get: (d) => d.footer.slots[i] ?? "",
    set: (d, v) => ({ ...d, footer: { ...d.footer, slots: slot(d.footer.slots, i, text(v)) } }),
  })),
  { field: "cover.style", get: (d) => d.cover.style, set: (d, v) => ({ ...d, cover: { ...d.cover, style: text(v) } }) },
  // The facts as one control: a comma-separated list in the core's order,
  // each fact once; the page toggles them with showFact.
  { field: "cover.shows", get: (d) => d.cover.shows.join(","), set: (d, v) => ({ ...d, cover: { ...d.cover, shows: text(v).split(",").filter((f) => f !== "") } }) },
  { field: "cover.when", get: (d) => d.cover.when, set: (d, v) => ({ ...d, cover: { ...d.cover, when: text(v) } }) },
];

/** The fields of version 3 no control sets: choosing a kit does. */
export const KIT_FIELDS = ["agentryTheme", "basedOn", "version"];

export function control(field: string): Control {
  const c = CONTROLS.find((x) => x.field === field);
  if (!c) throw new Error(`no control for ${field}`);
  return c;
}

/** Sets one field through its control, as the page's inputs do. */
export function edit(d: ReportDesign, field: string, v: string | boolean): ReportDesign {
  return control(field).set(d, v);
}

/** The cover's facts with one shown or not, kept in the core's order (the
 *  order facts names) so a design reads the same whichever box was ticked
 *  last. */
export function showFact(d: ReportDesign, facts: string[], fact: string, on: boolean): ReportDesign {
  const shows = facts.filter((f) => (f === fact ? on : d.cover.shows.includes(f)));
  return edit(d, "cover.shows", shows.join(","));
}

/** A slot with a token written in at a position, as the token picker
 *  does: the text before it, the token, the text after it. */
export function insertToken(slotText: string, at: number, token: string): string {
  const i = Math.max(0, Math.min(at, slotText.length));
  return slotText.slice(0, i) + tokenText(token) + slotText.slice(i);
}

/** A kit's document as the editor starts from it: a copy, so an edit never
 *  reaches the kit. */
export function startFrom(kit: ReportDesign): ReportDesign {
  return structuredClone(kit);
}

/** A kit's document as a new design of the person's own starts from it:
 *  the kit's look under the name they gave, or the kit's own name. */
export function newFromKit(kit: ReportDesign, name: string): ReportDesign {
  const d = startFrom(kit);
  d.name = name.trim() || kit.name;
  return d;
}

/**
 * The design as the page sends it to Go: the fields of version 3 in the
 * core's order, and nothing else the object happens to carry. Never an
 * image: the logo, the cover image and the custom cover are uploaded and
 * removed on their own, Go's view gives each beside the design rather than
 * in it, and the core keeps a stored image when a design is saved without
 * one.
 */
export function toDocument(d: ReportDesign): ReportDesign {
  return {
    agentryTheme: d.agentryTheme,
    name: d.name,
    basedOn: d.basedOn,
    version: d.version,
    paper: d.paper,
    fonts: { headings: d.fonts.headings, body: d.fonts.body, code: d.fonts.code },
    accent: d.accent,
    secondary: d.secondary,
    titleBlock: d.titleBlock,
    headings: d.headings,
    headingScale: d.headingScale,
    margins: d.margins,
    tables: { tinted: d.tables.tinted, banded: d.tables.banded },
    quotes: d.quotes,
    brand: {
      name: d.brand.name,
      strapline: d.brand.strapline,
      contact: [...d.brand.contact],
      website: d.brand.website,
      confidentiality: d.brand.confidentiality,
    },
    header: { style: d.header.style, slots: Array.from({ length: SLOT_COUNT }, (_, i) => d.header.slots[i] ?? "") },
    footer: { pageNumbers: d.footer.pageNumbers, slots: Array.from({ length: SLOT_COUNT }, (_, i) => d.footer.slots[i] ?? "") },
    cover: { style: d.cover.style, shows: [...d.cover.shows], when: d.cover.when },
  };
}

/** The canonical document, as pkg/reportdesign's Canonical writes it for a
 *  design without images: the fields in order, indented by two spaces, a
 *  final newline. The page compares designs by it; the hash a receipt
 *  names is Go's, over the document with its images. */
export function canonical(d: ReportDesign): string {
  return JSON.stringify(toDocument(d), null, 2) + "\n";
}

/** Whether two designs are the same document. */
export const sameDesign = (a: ReportDesign | null | undefined, b: ReportDesign | null | undefined) =>
  !!a && !!b && canonical(a) === canonical(b);

/** The family names a role offers, from Go's lists. */
export function fontsFor(role: FontRole, o: { headingFonts: string[]; bodyFonts: string[]; codeFonts: string[] }): string[] {
  return role === "headings" ? o.headingFonts : role === "body" ? o.bodyFonts : o.codeFonts;
}

/**
 * A family as the preview's CSS font-family: the app's copy of it (the
 * web fonts are named "Report <family>", src/reportFonts/), with the
 * generic of its kind behind it. Only a family of the core's list is
 * named; anything else is the generic alone, so no string from a design is
 * written into a style.
 */
export function cssFamily(name: string, families: ReportFamily[]): string {
  const f = families.find((x) => x.name === name);
  const generic = f?.kind === "mono" ? "ui-monospace, monospace" : f?.kind === "serif" ? "Georgia, serif" : "system-ui, sans-serif";
  return f ? `"Report ${f.name}", ${generic}` : generic;
}

const PNG_SRC = /^data:image\/png;base64,iVBORw0KGgo[A-Za-z0-9+/]*={0,2}$/;
const JPEG_SRC = /^data:image\/jpeg;base64,\/9j\/[A-Za-z0-9+/]*={0,2}$/;

/** Whether src is a PNG data: URL, the only logo and thumbnail the preview
 *  and the pages draw (Go's view builds it from the core's answer). */
export function isLogoSrc(src: unknown): src is string {
  return typeof src === "string" && PNG_SRC.test(src);
}

/** Whether src is a PNG or JPEG data: URL, the only cover image the pages
 *  draw. */
export function isImageSrc(src: unknown): src is string {
  return typeof src === "string" && (PNG_SRC.test(src) || JPEG_SRC.test(src));
}

const HEX = /^#[0-9a-f]{6}$/;

/** A colour when it is #rrggbb, else the fallback: the preview's styles
 *  take only a hex colour. */
export function cssColour(colour: string, fallback = "#000000"): string {
  return HEX.test(colour) ? colour : fallback;
}

/** A ratio as the core's refusal writes it (%.2f). */
export const ratioText = (r: number) => `${r.toFixed(2)}:1`;

/** A field the core refuses, as a sentence names it. */
export function fieldLabel(field: string): string {
  const words: Record<string, string> = {
    name: "the name",
    paper: "the paper",
    "fonts.headings": "the headings font",
    "fonts.body": "the body font",
    "fonts.code": "the code font",
    accent: "the accent",
    secondary: "the secondary colour",
    titleBlock: "the title block",
    headings: "the headings",
    headingScale: "the heading scale",
    margins: "the margins",
    "tables.tinted": "the tinted header row",
    "tables.banded": "the banded rows",
    quotes: "the quotes",
    "brand.name": "the brand name",
    "brand.strapline": "the strapline",
    "brand.contact": "the contact lines",
    "brand.website": "the website",
    "brand.confidentiality": "the confidentiality line",
    "header.style": "the header style",
    "footer.pageNumbers": "the page numbers",
    "cover.style": "the cover style",
    "cover.when": "when the cover is drawn",
  };
  if (words[field]) return words[field];
  const m = /^(header|footer)\.slots\.(\d)$/.exec(field);
  if (m) return `the ${m[1]}'s ${(SLOT_NAMES[Number(m[2])] ?? "").toLowerCase()} slot`;
  const c = /^brand\.contact\.(\d)$/.exec(field);
  if (c) return `contact line ${Number(c[1]) + 1}`;
  if (field.startsWith("cover.shows")) return "the cover's facts";
  return field || "the design";
}

/** What a save answers with. */
export function savedSentence(label: string, isDefault: boolean): string {
  return isDefault
    ? `Saved. Runs that pick no design use ${label}.`
    : `Saved. Pick ${label} in a run's setup, or make it the default.`;
}

/** The line under the library: what runs from this app draw now. */
export function librarySentence(count: number, defaultName: string | null, max: number): string {
  if (count === 0) return "You have no report design yet: runs from this app use Plain. Start one from a kit.";
  const n = `${count} of ${max} designs`;
  return defaultName
    ? `${n}. Runs that pick no design use ${defaultName}; the run setup lets you pick another.`
    : `${n}. No default: runs that pick no design use Plain.`;
}

/** The run setup's picker: the library's designs with the default first,
 *  and what the empty choice means. */
export interface DesignChoice {
  id: string;
  label: string;
}
export function designChoices(lib: { designs: { id: string; name: string; default: boolean }[]; default: string } | null): {
  choices: DesignChoice[];
  defaultId: string;
  none: string;
} {
  if (!lib || lib.designs.length === 0) return { choices: [], defaultId: "", none: "Plain: you have no report design" };
  const def = lib.designs.find((d) => d.id === lib.default);
  return {
    choices: lib.designs.map((d) => ({ id: d.id, label: d.default ? `${d.name} (your default)` : d.name })),
    defaultId: def?.id ?? "",
    none: def ? `Your default, ${def.name}` : "Plain: you have no default design",
  };
}
