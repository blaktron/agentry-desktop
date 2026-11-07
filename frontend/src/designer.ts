// Design with AI's rules, apart from the dialog so they can be tested: what
// stops a start, where a run goes first, and the sentences the dialog shows.
// Go checks every one of them again before anything runs or is uploaded;
// these only let the dialog say it first.

/** The Report Designer's two jobs. */
export type DesignerJob = "design" | "cover";
/** Where a run goes. */
export type DesignerWhere = "agentry" | "here";

/** What the dialog holds when the person presses Start. */
export interface DesignerInputs {
  job: DesignerJob;
  brief: string;
  /** The id of a design of the library: a Design starts from it, a Cover is for it. */
  from: string;
  logo: string;
  images: string[];
  urls: string[];
  document: string;
}

/** The limits Go answers for both places. */
export interface DesignerLimits {
  maxBriefBytes: number;
  maxReferences: number;
  maxCoverImages: number;
  maxUrls: number;
  reviewReady: boolean;
  reviewWhy?: string;
  offered: boolean;
  available: boolean;
  hostedMaxImages: number;
  hostedUrls: boolean;
  hostedMaxUrls?: number;
  documentHere: boolean;
  documentInAgentry: boolean;
}

/** A brief's size as the registry and the CLI count it: UTF-8 bytes. */
export function briefBytes(s: string): number {
  return new TextEncoder().encode(s).length;
}

/** The addresses in the field, which holds one a line: each trimmed, blank
 *  lines dropped. */
export function addressLines(text: string): string[] {
  return text.split(/\r?\n/).map((l) => l.trim()).filter((l) => l !== "");
}

/** How many images a job takes in a place: reference images for a Design,
 *  images the cover may draw for a Cover; In Agentry the registry's own
 *  count when it says one. */
export function maxImages(job: DesignerJob, where: DesignerWhere, l: DesignerLimits): number {
  const here = job === "cover" ? l.maxCoverImages : l.maxReferences;
  if (where === "agentry" && l.hostedMaxImages > 0) return Math.min(here, l.hostedMaxImages);
  return here;
}

/** Whether the document's place is drawn: only where the Report Designer
 *  reads one, for a Design. */
export function takesDocument(job: DesignerJob, where: DesignerWhere, l: DesignerLimits): boolean {
  return job === "design" && (where === "agentry" ? l.documentInAgentry : l.documentHere);
}

/** Where the dialog starts: In Agentry when it can be chosen, so a person
 *  without a model of their own has a run that works, else this computer. */
export function firstWhere(l: Pick<DesignerLimits, "offered" | "available">): DesignerWhere {
  return l.offered && l.available ? "agentry" : "here";
}

/** What stops a start, as a sentence, or null. */
export function designerProblem(i: DesignerInputs, where: DesignerWhere, l: DesignerLimits, addressProblem?: string | null): string | null {
  const brief = i.brief.trim();
  if (brief === "") return "Say what your reports are and how they should look: the Report Designer works from a brief.";
  if (briefBytes(i.brief) > l.maxBriefBytes) return `A brief is at most ${Math.round(l.maxBriefBytes / 1024)} KiB.`;
  if (i.job === "cover" && i.from === "") return "Choose the design the cover is for.";
  const most = maxImages(i.job, where, l);
  if (i.images.length > most) {
    return i.job === "cover" ? `A cover draws at most ${most} images.` : `A design takes at most ${most} reference images.`;
  }
  if (i.job === "cover" && (i.logo !== "" || i.urls.length > 0)) return "A cover takes the design's own logo and no web addresses.";
  if (i.document !== "" && !takesDocument(i.job, where, l)) {
    return where === "agentry"
      ? "The Report Designer In Agentry is not given a document from this app: run it on this computer, or remove the document."
      : "The Report Designer does not read a document here.";
  }
  if (i.urls.length > 0) {
    if (addressProblem) return addressProblem;
    if (where === "agentry") {
      if (!l.hostedUrls) return "The Report Designer In Agentry does not read web addresses here: leave them out, or run it on this computer.";
      if (l.hostedMaxUrls && i.urls.length > l.hostedMaxUrls) return `The Report Designer In Agentry takes at most ${l.hostedMaxUrls} web addresses.`;
    } else {
      if (i.urls.length > l.maxUrls) return `A design takes at most ${l.maxUrls} web addresses.`;
      if (!l.reviewReady) return `${l.reviewWhy ?? "Reading web addresses on this computer takes your own TypeSafe key"}.`;
    }
  }
  if (where === "agentry" && !l.available) return "The Report Designer cannot run In Agentry now.";
  return null;
}

/** The price In Agentry before the start: what it costs, when it is
 *  refunded, and a balance short of what it holds. */
export function designerPriceLine(a: { credits: number; holdCredits: number; balance: number; balanceKnown: boolean; pool?: string }): string {
  if (a.holdCredits === 0) {
    return "It holds no credits: the organization that pays for your runs is not metered.";
  }
  const n = a.credits || a.holdCredits;
  let s = `It costs ${n.toLocaleString("en-GB")} credits, held when it starts and refunded in full if it saves no design.`;
  if (a.balanceKnown && a.balance < a.holdCredits) {
    s += ` ${a.pool ? "Your organization's pool has" : "This account has"} ${a.balance.toLocaleString("en-GB")} credits, fewer than it holds, so the registry will refuse it.`;
  }
  return s;
}

/** The finished run In Agentry in one sentence: the design it saved, or why
 *  it saved none with the refund, or what went wrong. */
export function hostedOutcome(v: { applied: boolean; name?: string; note?: string; error?: string; outcomeLine?: string; statusLine: string }): string {
  if (v.applied) return v.outcomeLine || `The Report Designer saved “${v.name ?? "its design"}” to your library as a new design.`;
  return v.error || v.note || v.outcomeLine || v.statusLine;
}

/** What the Designer said of its own looking, when it could not. */
export function lookedLine(looked: boolean): string | null {
  return looked ? null : "The Report Designer says it could not look at its renders or your references, so it designed by the checks alone. A model that reads images lets it look.";
}

/** The line under a result made on this computer before the person's yes. */
export function savePrompt(job: DesignerJob): string {
  return job === "cover"
    ? "Nothing is in your library yet. Add it, and it is a new design with this custom cover page; the design it was written for stays as it is."
    : "Nothing is in your library yet. Add it, and it is a new design, not your default.";
}

/** What a run on this computer took from the person's document, or null
 *  for a run given none. */
export function documentLine(r: {
  documentPages?: number;
  logoFromDocument?: boolean;
  coverImageFromDocument?: boolean;
  coverFromDocument?: boolean;
  coverPicturesFromDocument?: number;
}): string | null {
  const n = r.documentPages ?? 0;
  const took: string[] = [];
  if (r.logoFromDocument) took.push("your logo");
  if (r.coverImageFromDocument) took.push("a cover image");
  const p = r.coverPicturesFromDocument ?? 0;
  if (n === 0 && took.length === 0 && !r.coverFromDocument) return null;
  let s = n > 0 ? `It looked at ${n} ${n === 1 ? "page" : "pages"} of your document` : "It read your document";
  if (took.length > 0) s += `, and took ${took.join(" and ")} from it`;
  s += ".";
  if (r.coverFromDocument) {
    s += p > 0 ? ` Its cover is modelled on your cover page, with ${p === 1 ? "1 picture" : `${p} pictures`} from it.` : " Its cover is modelled on your cover page.";
  }
  return s;
}
