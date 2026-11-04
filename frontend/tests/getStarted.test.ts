// Home's Get started card: the rows for each place runs go, ticked from
// what the app has read, and nothing guessed while a read is under way.
import { test } from "node:test";
import assert from "node:assert/strict";
import { getStartedRows, getStartedStatus, type GetStartedInput } from "../src/getStarted.ts";

const base: GetStartedInput = { start: "local", signedIn: true, setup: null, reviewKey: null, runhost: null, credits: null, ranOnce: null, packs: null };
const ids = (v: GetStartedInput) => getStartedRows(v).map((r) => `${r.id}${r.done ? "+" : "-"}`);

test("a new person on this computer: a model, Docker, the key, a first run, the Drafter and a pack", () => {
  const v = { ...base, setup: { dockerOk: false, providerSetUp: false }, reviewKey: "none" as const, ranOnce: false, packs: 0 };
  assert.deepEqual(ids(v), ["model-", "runtime-", "review-key-", "first-run-", "draft-", "knowledge-"]);
  assert.equal(getStartedStatus(getStartedRows(v)), "0 of 5 done");
  const rows = getStartedRows(v);
  assert.deepEqual(rows.map((r) => r.action), ["provider", "provider", "reviewKey", "run", "draft", "knowledge"]);
  assert.ok(rows.every((r) => r.done || r.button.length > 0));
});

test("on this computer, everything done reads All set, and a key from the environment counts", () => {
  const v = { ...base, setup: { dockerOk: true, providerSetUp: true }, reviewKey: "env" as const, ranOnce: true, packs: 2 };
  const rows = getStartedRows(v);
  assert.ok(rows.filter((r) => r.id !== "draft").every((r) => r.done));
  assert.equal(getStartedStatus(rows), "All set. Here is what to try next.");
  assert.match(rows.find((r) => r.id === "knowledge")!.text, /2 knowledge packs/);
});

test("a read still under way leaves its row out rather than guessing", () => {
  assert.deepEqual(ids(base), ["draft-"]);
  assert.equal(getStartedStatus(getStartedRows(base)), "Checking your setup…");
  // A key in the keychain the policy does not read is not a key set.
  assert.deepEqual(ids({ ...base, reviewKey: "unread" }), ["review-key-", "draft-"]);
  assert.equal(getStartedRows({ ...base, reviewKey: "unread" })[0].button, "Save it again");
});

test("a runhost: none set up, one that answers and is signed in, one that does not answer", () => {
  const remote = { ...base, start: "remote" as const };
  assert.deepEqual(ids({ ...remote, runhost: "none" }), ["runhost-", "draft-"]);
  assert.deepEqual(ids({ ...remote, runhost: { name: "box", known: true, reachable: true, signIns: ["Claude"] } }), ["runhost+", "runhost-model+", "draft-"]);
  const down = getStartedRows({ ...remote, runhost: { name: "box", known: true, reachable: false, signIns: [] } });
  assert.deepEqual(down.map((r) => `${r.id}${r.done ? "+" : "-"}`), ["runhost-", "runhost-model-", "draft-"]);
  assert.equal(down[0].text, "box does not answer");
  // Asking it: no row about whether it answers until it does.
  assert.deepEqual(ids({ ...remote, runhost: { name: "box", known: true, reachable: null, signIns: [] } }), ["runhost-model-", "draft-"]);
  // The local setup's rows are not a runhost's concern.
  assert.deepEqual(ids({ ...remote, runhost: "none", setup: { dockerOk: false, providerSetUp: false }, reviewKey: "none" }), ["runhost-", "draft-"]);
});

test("In Agentry: signing in comes first, then credits; signed out, no credits row", () => {
  const cloud = { ...base, start: "cloud" as const };
  assert.deepEqual(ids({ ...cloud, signedIn: false, credits: 0 }), ["sign-in-", "draft-"]);
  assert.deepEqual(ids({ ...cloud, credits: 0 }), ["sign-in+", "credits-", "draft-"]);
  const rows = getStartedRows({ ...cloud, credits: 1200, ranOnce: false });
  assert.deepEqual(rows.map((r) => `${r.id}${r.done ? "+" : "-"}`), ["sign-in+", "credits+", "first-run-", "draft-"]);
  assert.match(rows[1].text, /1,200 credits/);
});

test("signed out on this computer or a runhost, signing in is offered last, for publishing and In Agentry", () => {
  const rows = getStartedRows({ ...base, signedIn: false, ranOnce: true });
  assert.equal(rows.at(-1)?.id, "sign-in");
  assert.equal(rows.at(-1)?.done, false);
  assert.equal(getStartedRows({ ...base, signedIn: null }).some((r) => r.id === "sign-in"), false);
});
