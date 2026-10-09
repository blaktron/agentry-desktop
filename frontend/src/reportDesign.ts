// The Report design editor's model (plan report-designs.md §4 "The editor",
// D2, D8, D10, D14, D17, D18; agentry-desktop#759, D-075). The account keeps
// a report design whole, a theme document version 2; Settings → Report
// design edits it, and every rule the page shows is the core's
// pkg/reportdesign answering through Go (SettingsAPI.CheckReportDesign), so
// nothing here decides what can be saved. This is what the page draws from:
// its controls, which are exactly the fields of version 2 and nothing else
// (D8), the words each choice is shown with, and the faces the preview sets.
//
// Pure: the page and node's tests import it.

/** A report design, theme document version 2, as the core's JSON has it. */
export interface ReportDesign {
  agentryTheme: number;
  name: string;
  basedOn: string;
  version: number;
  paper: string;
  fonts: { headings: string; body: string; code: string };
  accent: string;
  titleBlock: string;
  headings: string;
  tables: { tinted: boolean; banded: boolean };
  quotes: string;
  footer: { pageNumbers: string; text: string };
}

export type FontRole = keyof ReportDesign["fonts"];

/** A family the Typst image carries, as Go's ReportDesignOptions lists it. */
export interface ReportFamily {
  name: string;
  kind: string;
  headingsOnly?: boolean;
}

/** What each theme looks like, by id (D14); one this list lacks is shown by
 *  its name alone. */
export const THEME_LOOKS: Record<string, string> = {
  plain: "Near-monochrome Inter, a ruled title, plain headings.",
  classic: "Source Serif, navy small-capital headings.",
  modern: "Inter on a blue title band, accent bars, banded tables.",
  editorial: "Playfair Display headings over Source Sans, warm red rules.",
  technical: "IBM Plex, teal numbered headings, banded tables.",
};

// The words each closed list's values are shown with (the site's). A value
// a later core adds is shown as itself.
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
export const QUOTE_LABELS: Record<string, string> = {
  callout: "Callout: a tinted box with an accent edge",
  plain: "Plain: a thin grey edge",
};
export const PAGE_NUMBER_LABELS: Record<string, string> = { none: "None", n: "1, 2, 3", nOfN: "1 of 3, 2 of 3" };

export const labelOf = (labels: Record<string, string>, value: string) => labels[value] ?? value;

/** A control of the editor: one field of the document, read and set. */
export interface Control {
  field: string;
  get: (d: ReportDesign) => string | boolean;
  set: (d: ReportDesign, v: string | boolean) => ReportDesign;
}

const text = (v: string | boolean) => (typeof v === "string" ? v : String(v));
const flag = (v: string | boolean) => v === true;

/**
 * The editor's controls, one per field of version 2 the person may change
 * (D8): the name, the paper, the three faces, the accent, the title block,
 * the heading treatment, the two table styles, the quotes, and the footer's
 * page numbers and line. The document's version, its theme and its own
 * revision are not controls: choosing a theme sets them.
 */
export const CONTROLS: Control[] = [
  { field: "name", get: (d) => d.name, set: (d, v) => ({ ...d, name: text(v) }) },
  { field: "paper", get: (d) => d.paper, set: (d, v) => ({ ...d, paper: text(v) }) },
  ...(["headings", "body", "code"] as FontRole[]).map((role): Control => ({
    field: `fonts.${role}`,
    get: (d) => d.fonts[role],
    set: (d, v) => ({ ...d, fonts: { ...d.fonts, [role]: text(v) } }),
  })),
  // The core keeps an accent in lower case; the editor writes it so.
  { field: "accent", get: (d) => d.accent, set: (d, v) => ({ ...d, accent: text(v).trim().toLowerCase() }) },
  { field: "titleBlock", get: (d) => d.titleBlock, set: (d, v) => ({ ...d, titleBlock: text(v) }) },
  { field: "headings", get: (d) => d.headings, set: (d, v) => ({ ...d, headings: text(v) }) },
  { field: "tables.tinted", get: (d) => d.tables.tinted, set: (d, v) => ({ ...d, tables: { ...d.tables, tinted: flag(v) } }) },
  { field: "tables.banded", get: (d) => d.tables.banded, set: (d, v) => ({ ...d, tables: { ...d.tables, banded: flag(v) } }) },
  { field: "quotes", get: (d) => d.quotes, set: (d, v) => ({ ...d, quotes: text(v) }) },
  { field: "footer.pageNumbers", get: (d) => d.footer.pageNumbers, set: (d, v) => ({ ...d, footer: { ...d.footer, pageNumbers: text(v) } }) },
  { field: "footer.text", get: (d) => d.footer.text, set: (d, v) => ({ ...d, footer: { ...d.footer, text: text(v) } }) },
];

/** The fields of version 2 no control sets: choosing a theme does. */
export const THEME_FIELDS = ["agentryTheme", "basedOn", "version"];

export function control(field: string): Control {
  const c = CONTROLS.find((x) => x.field === field);
  if (!c) throw new Error(`no control for ${field}`);
  return c;
}

/** Sets one field through its control, as the page's inputs do. */
export function edit(d: ReportDesign, field: string, v: string | boolean): ReportDesign {
  return control(field).set(d, v);
}

/** A theme's document as the editor starts from it: a copy, so an edit
 *  never reaches the theme. */
export function startFrom(theme: ReportDesign): ReportDesign {
  return structuredClone(theme);
}

/**
 * The design as the page sends it to SaveReportDesign: the fields of
 * version 2 in the core's order, and nothing else the object happens to
 * carry.
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
    titleBlock: d.titleBlock,
    headings: d.headings,
    tables: { tinted: d.tables.tinted, banded: d.tables.banded },
    quotes: d.quotes,
    footer: { pageNumbers: d.footer.pageNumbers, text: d.footer.text },
  };
}

/** The canonical document, as pkg/reportdesign's Canonical writes it: the
 *  fields in order, indented by two spaces, a final newline. */
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
 * web fonts are renamed "Report <family>", src/reportFonts/), with the
 * generic of its kind behind it. Only a family of the core's list is
 * named; anything else is the generic alone, so no string from a design is
 * written into a style.
 */
export function cssFamily(name: string, families: ReportFamily[]): string {
  const f = families.find((x) => x.name === name);
  const generic = f?.kind === "mono" ? "ui-monospace, monospace" : f?.kind === "serif" ? "Georgia, serif" : "system-ui, sans-serif";
  return f ? `"Report ${f.name}", ${generic}` : generic;
}

const HEX = /^#[0-9a-f]{6}$/;

/** The accent when it is #rrggbb, else black: the preview's styles take
 *  only a hex colour. */
export function cssColour(accent: string): string {
  return HEX.test(accent) ? accent : "#000000";
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
    titleBlock: "the title block",
    headings: "the headings",
    "tables.tinted": "the tinted header row",
    "tables.banded": "the banded rows",
    quotes: "the quotes",
    "footer.pageNumbers": "the page numbers",
    "footer.text": "the footer line",
  };
  return words[field] ?? (field || "the design");
}

/** What a save or a reset answers with. */
export function savedSentence(label: string | null): string {
  return label
    ? `Saved. Your runs' PDF reports use ${label}.`
    : "Reset. You have no report design: runs from this app use Plain.";
}

/** The line under the editor: what runs from this app draw now. */
export function standingSentence(label: string | null): string {
  return label
    ? `Your runs use ${label}. Reset forgets it, and runs from this app then use Plain.`
    : "You have not saved a design: runs from this app use Plain.";
}
