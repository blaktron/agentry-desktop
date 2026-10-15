// The Knowledge page's two places: which packs each page lists, the cards' lines, and
// the badges a row carries about the other place.
import { test } from "node:test";
import assert from "node:assert/strict";
import { allocationPhrase, cloudPacks, cloudStatus, localPacks, localStatus, otherPlace, sharedGroups } from "../src/knowledgeCards.ts";

const MB = 1024 * 1024;
const size = (n: number) => `${Math.round(n / MB)} MB`;
const pack = (name: string, over: Record<string, unknown> = {}) => ({
  name, local: false, inAgentry: false, shared: false, org: "", bytes: 1 * MB, updated: "", agentryUpdated: "", changed: false, ...over,
});
const packs = [
  pack("here-only", { local: true, bytes: 3 * MB, updated: "2026-10-01T09:00:00Z" }),
  pack("both", { local: true, inAgentry: true, bytes: 2 * MB, updated: "2026-10-04T14:02:00Z", agentryUpdated: "2026-10-03T09:15:00Z", changed: true }),
  pack("cloud-only", { inAgentry: true, bytes: 5 * MB, agentryUpdated: "2026-10-02T00:00:00Z" }),
  pack("handbook", { inAgentry: true, shared: true, org: "acme" }),
  pack("runbook", { local: true, inAgentry: true, shared: true, org: "acme", updated: "2026-09-30T00:00:00Z" }),
  pack("policies", { inAgentry: true, shared: true, org: "globex" }),
];

test("each place lists its own: a copy here on Local, the account's on Cloud, the shared by organization", () => {
  assert.deepEqual(localPacks(packs).map((p) => p.name), ["here-only", "both", "runbook"]);
  assert.deepEqual(cloudPacks(packs).map((p) => p.name), ["both", "cloud-only"]);
  assert.deepEqual(sharedGroups(packs).map((g) => [g.org, g.packs.map((p) => p.name)]), [["acme", ["handbook", "runbook"]], ["globex", ["policies"]]]);
});

test("the Local card counts the packs here, their size and the newest change", () => {
  assert.equal(localStatus(packs, size), "3 packs · 6 MB · updated 4 October 2026.");
  assert.equal(localStatus([pack("x", { local: true })], size), "1 pack · 1 MB.");
  assert.equal(localStatus(packs.filter((p) => !p.local), size), "No knowledge on this computer yet.");
});

test("the Cloud card: signed out, a list that did not answer, or the account's packs, allocation and shared", () => {
  const alloc = { enabled: true, bytes: 18 * MB, max: 100 * MB };
  assert.equal(cloudStatus({ signedIn: true }, packs, size, alloc), "2 packs · 18 MB of 100 MB · 2 shared by acme · 1 shared by globex.");
  assert.equal(cloudStatus({ signedIn: true }, packs, size), "2 packs · 2 shared by acme · 1 shared by globex.");
  assert.equal(cloudStatus({ signedIn: false }, packs, size, alloc), "Sign in to keep knowledge in your account.");
  assert.equal(cloudStatus({ signedIn: true, agentry: "Agentry's copies could not be listed: 503" }, packs, size), "Agentry's copies could not be listed: 503");
  assert.equal(cloudStatus({ signedIn: true }, [pack("x", { local: true })], size, alloc), "No knowledge in your account yet.");
  assert.equal(cloudStatus({ signedIn: true }, [pack("h", { inAgentry: true, shared: true, org: "acme" })], size), "No packs of your own · 1 shared by acme.");
  assert.equal(cloudStatus(null, packs, size), "");
});

test("the allocation as Settings says it", () => {
  assert.equal(allocationPhrase({ enabled: true, bytes: 0, max: 0 }, size), "0 MB of no cap");
  assert.equal(allocationPhrase({ enabled: false, bytes: 2 * MB, max: 0 }, size), "none may be added; 2 MB held");
  assert.equal(allocationPhrase({ enabled: false, bytes: 0, max: 0 }, size), "none may be added");
});

test("a row says it is kept in the other place too, and changed since it was copied; a shared pack says neither", () => {
  const [, both, cloudOnly, , runbook] = packs;
  assert.deepEqual(otherPlace(both, "local").map((b) => b.label), ["also in your account", "changed since it was copied"]);
  assert.deepEqual(otherPlace(both, "cloud").map((b) => b.label), ["also on this computer", "changed since it was copied"]);
  assert.deepEqual(otherPlace(cloudOnly, "cloud"), []);
  assert.deepEqual(otherPlace(runbook, "local"), []);
  const never = /\b(safe|safely|secure|securely|secured|private|privately)\b/i;
  for (const p of packs) for (const page of ["local", "cloud"] as const) for (const b of otherPlace(p, page)) assert.doesNotMatch(`${b.label} ${b.title}`, never);
});
