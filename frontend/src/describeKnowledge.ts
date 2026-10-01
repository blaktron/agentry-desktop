// Describe it's Knowledge pick (agentry-desktop#497; plan
// knowledge-and-drafter.md K4, D3, D4): which of the person's packs a draft
// may read where it is made, what the picked ones hold against the draft's
// bound, and which a draft on this computer scans first. Pure, so its rules
// are tested without the app; the words are the site's
// (agentry-web components/run-setup KnowledgePick, lib/run-setup).
//
// On this computer a draft reads packs kept here; one that has no scan in
// Agentry for this copy is scanned first by AI Review with the person's own
// key (Go's dialog names exactly which, from the registry's confirmation).
// In Agentry a draft reads packs Agentry keeps and has scanned, and Agentry's
// scan stands in.

export type Where = "here" | "agentry";

/** A pack as the Knowledge list gives it (knowledge.Pack's fields used here). */
export type PickPack = {
  name: string;
  bytes: number;
  local: boolean;
  inAgentry: boolean;
  agentryReady: boolean;
  scanState: string;
  changed: boolean;
  shared?: boolean;
  org?: string;
};

/** A draft's bounds where it is made: Go's DescribeStatus for this
 *  computer, the registry's draft.knowledge for In Agentry. */
export type Bounds = { maxPacks: number; maxBytes: number };

/** Whether a draft made at where can read the pack. */
export function pickable(p: PickPack, where: Where): boolean {
  return where === "here" ? p.local : p.inAgentry && p.agentryReady;
}

/** Why a pack cannot be picked at where, in a sentence for its row, or null. */
export function whyNot(p: PickPack, where: Where): string | null {
  if (pickable(p, where)) return null;
  if (where === "here") return "kept in Agentry only: download it to draft with it on this computer";
  if (p.shared && !p.inAgentry) return `${p.org ?? "its organization"} does not list this pack in Agentry now`;
  if (!p.inAgentry) return "kept on this computer only: upload it to Agentry to draft with it there";
  if (p.scanState === "scanning") return "pick it once Agentry's scan of it ends";
  if (p.scanState === "failed") return "Agentry's scan could not read it: upload it again to draft with it there";
  return "Agentry holds no pages of it yet: upload it to draft with it there";
}

/** A note on a pack that can be picked, or null: In Agentry the draft
 *  reads Agentry's copy, which may be older than this computer's. */
export function pickNote(p: PickPack, where: Where): string | null {
  return where === "agentry" && p.local && p.changed ? "the draft reads the copy Agentry keeps, which is older than this computer's" : null;
}

/** Whether a draft on this computer scans the pack first: no scan in
 *  Agentry stands in for this copy. Nothing is scanned first In Agentry. */
export function scansFirst(p: PickPack, where: Where): boolean {
  return where === "here" && !(p.inAgentry && p.agentryReady && p.scanState === "scanned" && !p.changed);
}

/** What the picked packs hold, by the sizes the list gives. */
export function pickedBytes(names: string[], packs: PickPack[]): number {
  return names.reduce((sum, n) => sum + (packs.find((p) => p.name === n)?.bytes ?? 0), 0);
}

/** A size in the binary units the Knowledge pages count in, "10 MB". */
export function packSize(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB"];
  let v = n / 1024;
  let i = 0;
  while (Number(v.toFixed(1)) >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const s = v.toFixed(1);
  return `${s.endsWith(".0") ? s.slice(0, -2) : s} ${units[i]}`;
}

/** The first thing that stops the pick, or null: at most the bound's
 *  number of packs, holding at most its bytes in all. */
export function pickProblem(names: string[], packs: PickPack[], b: Bounds): string | null {
  if (b.maxPacks > 0 && names.length > b.maxPacks) {
    const over = names.length - b.maxPacks;
    return `A draft reads at most ${b.maxPacks} knowledge ${b.maxPacks === 1 ? "pack" : "packs"}. Untick ${over === 1 ? "one" : over} to go on.`;
  }
  const total = pickedBytes(names, packs);
  if (b.maxBytes > 0 && total > b.maxBytes) {
    return `These packs hold ${packSize(total)}, and a draft reads at most ${packSize(b.maxBytes)} of knowledge in all. Untick one to go on.`;
  }
  return null;
}

/** The line under the pick: the bound, and what the picked ones hold. */
export function totalLine(names: string[], packs: PickPack[], b: Bounds): string {
  if (names.length === 0) return `None picked. A draft reads up to ${b.maxPacks} packs, ${packSize(b.maxBytes)} in all.`;
  return `Picked ${names.length} of up to ${b.maxPacks}, ${packSize(pickedBytes(names, packs))} of ${packSize(b.maxBytes)}.`;
}

/** The warning before a draft on this computer: which picked packs AI
 *  Review scans first with the person's key, or null for none. */
export function scanFirstLine(names: string[], packs: PickPack[], where: Where): string | null {
  const first = names.filter((n) => {
    const p = packs.find((q) => q.name === n);
    return p !== undefined && scansFirst(p, where);
  });
  if (first.length === 0) return null;
  // The dialog's own sentence (Go's knowledgeConsent), which says it again,
  // exactly, from the registry's answer before the draft starts.
  return first.length === 1
    ? `${first[0]} has not been scanned in Agentry, so AI Review reads every file of it with your key before the draft starts, and withholds what it flags.`
    : `${first.join(", ")} have not been scanned in Agentry, so AI Review reads every file of them with your key before the draft starts, and withholds what it flags.`;
}

/** The picks that still hold where the draft is now made: a switch from
 *  this computer to Agentry drops a pack Agentry cannot give it. */
export function keepPickable(names: string[], packs: PickPack[], where: Where): string[] {
  return names.filter((n) => {
    const p = packs.find((q) => q.name === n);
    return p !== undefined && pickable(p, where);
  });
}
