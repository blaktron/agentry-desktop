// The Knowledge section for packs a QuickAgent ships, and Describe it's
// Read / Read and ship choice: which packs may be ticked in a QuickAgent's
// namespace, the limit of five, the include list that packs them, and the
// notes beside a pack. The rules are internal/build's ShipOption.Reason.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  MAX_SHIPS, SHIPS_GLOB, afterDraftNote, canAddShip, folderOwn, isTicked, optionKey, readAndShipReason, shipPickLine, shipReason, shipSize, shipsOf, shipTotalLine,
  tick, tickProblem, untick, type ShipOption, type ShipState,
} from "../src/shipKnowledge.ts";

const opt = (name: string, over: Partial<ShipOption> = {}): ShipOption => ({
  name, kind: "own", bytes: 2048, pages: 4, ready: true, orgAdmin: false, ...over,
});
const org = (name: string, over: Partial<ShipOption> = {}): ShipOption => opt(name, { kind: "org", org: "acme", orgAdmin: true, ...over });

test("a pack ships once it is ready, within the cap, and an organization's only from its admin into its own namespace", () => {
  assert.equal(shipReason(opt("a"), "chris"), null);
  assert.equal(shipReason(opt("a", { ready: false }), "chris"), "wait until AI Review has read it, then it can ship");
  assert.equal(shipReason(opt("a", { bytes: 11 << 20 }), "chris"), "it holds 11 MB, and a release ships at most 10 MB of one pack");
  assert.equal(shipReason(opt("a", { bytes: 10 << 20 }), "chris"), null);
  assert.equal(shipReason(org("a"), "acme"), null);
  assert.equal(shipReason(org("a", { orgAdmin: false }), "acme"), "only an administrator of acme can ship its pack");
  assert.equal(shipReason(org("a"), "chris"), "ships only into a QuickAgent in the acme namespace, and this one is in chris");
  assert.equal(shipReason(org("a"), " "), "ships only into a QuickAgent in the acme namespace, and this one is in no namespace yet");
});

test("sizes are in the units the Knowledge pages count in", () => {
  assert.deepEqual([512, 1536, 10 << 20, 1 << 19].map(shipSize), ["512 B", "1.5 KB", "10 MB", "512 KB"]);
  assert.equal(shipSize((1 << 20) - 1), "1 MB");
});

test("ticking adds the name and the source, and the include list that packs it when it came with the workspace", () => {
  let m: ShipState = { knowledgeShips: [], ships: [], publishInclude: [] };
  m = { ...m, ...tick(m, opt("glossary", { id: "kp_1" }), "chris") };
  assert.deepEqual(m.knowledgeShips, ["glossary"]);
  assert.equal(m.ships?.[0].id, "kp_1");
  assert.deepEqual(m.publishInclude, [], "the form's own list packs it already");
  // Ticking it again refreshes the source and adds no name.
  m = { ...m, ...tick(m, opt("glossary", { bytes: 9 }), "chris") };
  assert.deepEqual(m.knowledgeShips, ["glossary"]);
  assert.equal(m.ships?.length, 1);
  assert.equal(m.ships?.[0].bytes, 9);

  const custom: ShipState = { knowledgeShips: [], ships: [], publishInclude: ["agent.yaml", "AGENT.md", "extra/**"] };
  const withPack = { ...custom, ...tick(custom, opt("glossary"), "chris") };
  assert.deepEqual(withPack.publishInclude, ["agent.yaml", "AGENT.md", "extra/**", SHIPS_GLOB]);
  assert.deepEqual(untick(withPack, "glossary").publishInclude, ["agent.yaml", "AGENT.md", "extra/**"]);
  assert.deepEqual(untick(withPack, "glossary").knowledgeShips, []);
});

test("a sixth pack, a pack that may not ship and a second pack under a name are refused, and change nothing", () => {
  let m: ShipState = { knowledgeShips: [], ships: [], publishInclude: [] };
  for (const n of ["a", "b", "c", "d", "e"]) m = { ...m, ...tick(m, opt(n), "chris") };
  assert.equal(m.knowledgeShips?.length, MAX_SHIPS);
  assert.equal(tickProblem(m, opt("f"), "chris"), "a release ships at most 5 knowledge packs");
  assert.deepEqual(tick(m, opt("f"), "chris").knowledgeShips, ["a", "b", "c", "d", "e"]);
  // An already ticked pack is not a sixth.
  assert.equal(tickProblem(m, opt("a"), "chris"), null);
  assert.deepEqual(tick(m, opt("g", { ready: false }), "chris").knowledgeShips, ["a", "b", "c", "d", "e"]);
  // An organization's pack under the name of a ticked own pack is another pack.
  assert.equal(tickProblem(m, org("a"), "acme"), "the form already ships a pack named a");
  assert.equal(isTicked(m, opt("a")), true);
  assert.equal(isTicked(m, org("a")), false);
  assert.notEqual(optionKey(opt("a")), optionKey(org("a")));
});

test("a pack the folder already held is the folder's own, and the line counts what is ticked", () => {
  const m: ShipState = { knowledgeShips: ["glossary", "by-hand"], ships: [{ name: "glossary", kind: "own", bytes: 1536, pages: 4 }] };
  assert.deepEqual(folderOwn(m), ["by-hand"]);
  assert.equal(shipTotalLine({ knowledgeShips: [] }), "None ticked. A release ships up to 5 packs, 10 MB each.");
  assert.equal(shipTotalLine(m), "Ticked 2 of up to 5, 1.5 KB in all.");
  assert.equal(shipTotalLine({ knowledgeShips: ["by-hand"], ships: [] }), "Ticked 1 of up to 5.");
});

test("a pack ticked after a draft carries the note that the instructions may not mention it", () => {
  const m: ShipState = { knowledgeShips: ["glossary", "late"] };
  assert.equal(afterDraftNote(m, "glossary", ["glossary"]), null);
  assert.match(afterDraftNote(m, "late", ["glossary"]) ?? "", /instructions may not mention it/);
  assert.equal(afterDraftNote(m, "unticked", []), null);
});

test("Describe it offers Read and ship for a pack of the person's own, on this computer", () => {
  assert.equal(readAndShipReason(opt("a"), "here"), null);
  assert.match(readAndShipReason(opt("a"), "agentry") ?? "", /after the draft/);
  assert.match(readAndShipReason(undefined, "here") ?? "", /only a pack you keep can ship/);
  assert.match(readAndShipReason(org("a"), "here") ?? "", /acme's pack ships into a QuickAgent in its namespace/);
  assert.equal(readAndShipReason(opt("a", { ready: false }), "here"), "wait until AI Review has read it, then it can ship");
});

test("the ships of a draft are among the packs it reads, and the line names them", () => {
  assert.deepEqual(shipsOf(["a", "b"], ["b", "c"]), ["b"]);
  assert.equal(shipPickLine([]), null);
  assert.equal(shipPickLine(["a"]), "a will ship with the QuickAgent: it goes into the release, readable by everyone who can download it, and you confirm it again when you submit.");
  assert.match(shipPickLine(["a", "b"]) ?? "", /^a, b will ship with the QuickAgent: they go into the release/);
  assert.equal(canAddShip(["a", "b", "c", "d"]), true);
  assert.equal(canAddShip(["a", "b", "c", "d", "e"]), false);
});
