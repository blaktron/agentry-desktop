// What the result view and Home's Deliverables card say of a deliverable, a
// file the run was for. Pure and self-contained, so the node tests can
// import it: Go decides what is listed, what a click opens and where the
// copy is (RunAPI.RunDeliverables); this says it.

import { nameOf } from "./recentRuns.ts";

export type DeliverableFacts = {
  name: string;
  title?: string;
  type: string;
  bytes: number;
  canOpen: boolean;
  viewKind: string;
  /** "type" or "contents" when a click does not open it. */
  notOpened?: string;
  /** The folder holding a copy of these bytes, "" for none. */
  copiedTo?: string;
};

const TYPE_LABELS: Record<string, string> = {
  pdf: "PDF", md: "Markdown", txt: "Text", csv: "CSV", json: "JSON", png: "PNG image", jpg: "JPEG image",
  jpeg: "JPEG image", docx: "Word document", xlsx: "Excel workbook", pptx: "PowerPoint presentation",
  html: "Web page", htm: "Web page",
};

/** A type in a person's words. */
export function typeLabel(type: string): string {
  return TYPE_LABELS[type] ?? (type ? `.${type} file` : "File");
}

/** What heads a row: the title the agent or runner gave, else the file's name. */
export function heading(f: Pick<DeliverableFacts, "name" | "title">): string {
  return f.title?.trim() ? f.title : f.name;
}

export type Actions = {
  open: boolean;
  view: boolean;
  /** Show in folder, always offered. */
  show: true;
  /** Why a click does not open it; "" when it does. */
  note: string;
};

/** What a row offers. A type no click opens is Show in folder only, and says so. */
export function actions(f: Pick<DeliverableFacts, "type" | "canOpen" | "viewKind" | "notOpened">): Actions {
  if (f.canOpen) return { open: true, view: f.viewKind === "pdf" || f.viewKind === "markdown" || f.viewKind === "text", show: true, note: "" };
  if (f.notOpened === "contents") {
    return { open: false, view: false, show: true, note: "Its contents are not what its file type says, so it is not opened from here. Show in folder finds it." };
  }
  const what = f.type ? `A .${f.type} file` : "A file with no type";
  return { open: false, view: false, show: true, note: `${what} can be acted on by your system, so it is not opened from here. Show in folder finds it.` };
}

/** Where the file is: its copy in the Deliverables folder, or only the run folder. */
export function copiedLine(f: Pick<DeliverableFacts, "copiedTo">, on: boolean): string {
  if (f.copiedTo) return `Copied to ${f.copiedTo}`;
  return on ? "In the run folder; not copied to your Deliverables folder." : "Deliverables are off, so this file is in the run folder only.";
}

/** A line for files the run's record names that are not offered. */
export function withheldLine(n: number): string {
  if (n <= 0) return "";
  const them = n === 1 ? "One file the run's record names is" : `${n} files the run's record names are`;
  return `${them} not shown: ${n === 1 ? "it is" : "they are"} not in the run folder, or ${n === 1 ? "its" : "their"} contents differ from what the run recorded.`;
}

export type IconKind = "pdf" | "html" | "document" | "spreadsheet" | "presentation" | "image" | "text" | "other";

const ICON_KINDS: Record<string, IconKind> = {
  pdf: "pdf", html: "html", htm: "html", docx: "document", md: "text", txt: "text", json: "text",
  xlsx: "spreadsheet", csv: "spreadsheet", pptx: "presentation", png: "image", jpg: "image", jpeg: "image",
};

/** What an icon is drawn as. Only a file a click opens is drawn by its type:
 * a type on no list, a macro document and a file whose bytes are not what its
 * name says all get the plain icon, so a card never dresses one up as a PDF. */
export function iconKind(f: Pick<DeliverableFacts, "type" | "canOpen">): IconKind {
  if (!f.canOpen) return "other";
  return ICON_KINDS[f.type.toLowerCase()] ?? "other";
}

/** What a click on an icon does: opens the copy in the system's app, or, for a
 * file that is not opened from here, shows it in its folder. */
export function clickDoes(f: Pick<DeliverableFacts, "canOpen">): "open" | "show" {
  return f.canOpen ? "open" : "show";
}

/** The QuickAgent under an icon: its name without publisher and version. */
export function agentOf(ref?: string): string {
  const name = nameOf(ref).split("/").pop()?.trim() ?? "";
  return name || "QuickAgent";
}

/** The day a run ended, short: "17 Oct", with the year when it is not this one. */
export function iconDate(iso: string | undefined, now: Date, locale?: string, timeZone?: string): string {
  if (!iso) return "";
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return "";
  const year = (d: Date) => new Intl.DateTimeFormat("en", { year: "numeric", timeZone }).format(d);
  return new Intl.DateTimeFormat(locale, {
    day: "numeric", month: "short", year: year(t) === year(now) ? undefined : "numeric", timeZone,
  }).format(t);
}

/** An icon's accessible name: the file, the agent, the date, and what a click does. */
export function iconName(f: Pick<DeliverableFacts, "name" | "canOpen">, agent: string, date: string): string {
  const what = f.canOpen ? "Opens in your system's app." : "A click shows it in its folder; it is not opened from here.";
  return `${f.name}, from ${agent}${date ? `, ${date}` : ""}. ${what}`;
}

export type CardFacts = { on: boolean; remember: boolean; count: number; error?: string | null; reading: boolean };

/** The Deliverables card's status line, null when it lists files. */
export function cardStatus(c: CardFacts): string | null {
  if (c.error) return "Deliverables could not be read.";
  if (c.reading) return "Reading deliverables…";
  if (c.count > 0) return null;
  if (!c.remember) return "Recent runs are not remembered, so there is nothing to list here. Turn on Remember recent runs in Run defaults.";
  if (!c.on) return "Deliverables are off. Turn them on in Run defaults to have each run's files copied to a folder.";
  return "Nothing yet. The files a run was for are listed here, newest first.";
}

/** The Run defaults line under the switch. */
export function settingsLine(v: { deliverables: boolean; deliverablesDir?: string; deliverablesDefault: boolean; deliverablesError?: string }): string {
  if (v.deliverablesError) return `Agentry Desktop cannot name a Documents folder on this computer (${v.deliverablesError}). Choose a folder for deliverables.`;
  if (!v.deliverables) return "Off. A run's files stay in its run folder.";
  return v.deliverablesDefault ? "On. Each run copies the files it was for into your Documents folder." : "On. Each run copies the files it was for into the folder you chose.";
}
