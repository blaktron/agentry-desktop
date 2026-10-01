// Describe it's Knowledge pick (agentry-desktop#497): which packs a draft
// may read where it is made, the 10 MB total and its refusal, and the
// warning that a draft on this computer scans an unscanned pack first.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  keepPickable, packSize, pickNote, pickProblem, pickable, scanFirstLine, scansFirst, totalLine, whyNot, type PickPack,
} from "../src/describeKnowledge.ts";

const pack = (name: string, over: Partial<PickPack> = {}): PickPack => ({
  name, bytes: 6903, local: true, inAgentry: true, agentryReady: true, scanState: "scanned", changed: false, ...over,
});
const bounds = { maxPacks: 5, maxBytes: 10 << 20 };

test("a draft on this computer reads packs kept here; In Agentry, packs Agentry keeps and has scanned", () => {
  const onlyHere = pack("notes", { inAgentry: false, agentryReady: false, scanState: "not-scanned" });
  const onlyThere = pack("handbook", { local: false });
  const scanning = pack("rescanned", { local: false, agentryReady: false, scanState: "scanning" });
  assert.equal(pickable(onlyHere, "here"), true);
  assert.equal(pickable(onlyHere, "agentry"), false);
  assert.match(whyNot(onlyHere, "agentry") ?? "", /upload it to Agentry/);
  assert.equal(pickable(onlyThere, "here"), false);
  assert.match(whyNot(onlyThere, "here") ?? "", /download it/);
  assert.equal(pickable(onlyThere, "agentry"), true);
  assert.match(whyNot(scanning, "agentry") ?? "", /once Agentry's scan of it ends/);
  assert.equal(whyNot(pack("runbook"), "here"), null);
  assert.match(whyNot(pack("x", { local: false, agentryReady: false, scanState: "failed" }), "agentry") ?? "", /could not read it/);
  assert.match(pickNote(pack("y", { changed: true }), "agentry") ?? "", /older than this computer's/);
  assert.equal(pickNote(pack("y", { changed: true }), "here"), null);
});

test("the total states the bound, then what the picked packs hold, in binary units", () => {
  const packs = [pack("runbook"), pack("big", { bytes: 6 << 20 })];
  assert.equal(totalLine([], packs, bounds), "None picked. A draft reads up to 5 packs, 10 MB in all.");
  assert.equal(totalLine(["runbook"], packs, bounds), "Picked 1 of up to 5, 6.7 KB of 10 MB.");
  assert.equal(packSize(10 << 20), "10 MB");
});

test("a pick past the bytes or the number is refused with the site's sentence", () => {
  const packs = [pack("a", { bytes: 6 << 20 }), pack("b", { bytes: 5 << 20 }), pack("c"), pack("d"), pack("e"), pack("f")];
  assert.equal(pickProblem(["a"], packs, bounds), null);
  assert.equal(pickProblem(["a", "b"], packs, bounds), "These packs hold 11 MB, and a draft reads at most 10 MB of knowledge in all. Untick one to go on.");
  assert.equal(pickProblem(["a", "c", "d", "e", "f", "b"], packs, { maxPacks: 5, maxBytes: 100 << 20 }), "A draft reads at most 5 knowledge packs. Untick one to go on.");
});

test("on this computer, a pack with no scan in Agentry for this copy is scanned first, and the warning names it", () => {
  const scanned = pack("runbook");
  const never = pack("notes", { inAgentry: false, agentryReady: false, scanState: "not-scanned" });
  const changed = pack("handbook", { changed: true });
  assert.equal(scansFirst(scanned, "here"), false);
  assert.equal(scansFirst(never, "here"), true);
  assert.equal(scansFirst(changed, "here"), true);
  assert.equal(scansFirst(never, "agentry"), false);
  const packs = [scanned, never, changed];
  assert.equal(scanFirstLine(["runbook"], packs, "here"), null);
  assert.equal(scanFirstLine(["runbook", "notes"], packs, "here"),
    "notes has not been scanned in Agentry, so AI Review reads every file of it with your key before the draft starts, and withholds what it flags.");
  assert.match(scanFirstLine(["notes", "handbook"], packs, "here") ?? "", /^notes, handbook have not been scanned in Agentry, so AI Review reads every file of them/);
  assert.equal(scanFirstLine(["notes"], packs, "agentry"), null);
});

test("switching where the draft is made keeps only the picks it can still read", () => {
  const packs = [pack("runbook"), pack("notes", { inAgentry: false, agentryReady: false })];
  assert.deepEqual(keepPickable(["runbook", "notes", "gone"], packs, "agentry"), ["runbook"]);
  assert.deepEqual(keepPickable(["runbook", "notes"], packs, "here"), ["runbook", "notes"]);
});
