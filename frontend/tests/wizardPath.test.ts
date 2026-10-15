// The first-run wizard's Local and Cloud paths: Local
// keeps every step the wizard had, Cloud ends on its data page, and Cloud is
// open only to someone signed in to the registry the app browses.
import { test } from "node:test";
import assert from "node:assert/strict";
import { cloudOpen, pathAt, pathOf, runWhereFor, stepAfter, stepBefore, stepsFor } from "../src/wizardPath.ts";

test("Local keeps the setup as it was, after the choice", () => {
  assert.deepEqual(stepsFor("local"), ["welcome", "signin", "where", "runtime", "reviewkey", "codingagent", "done"]);
});

test("Cloud ends on its data page", () => {
  assert.deepEqual(stepsFor("cloud"), ["welcome", "signin", "where", "cloud"]);
});

test("before a path is picked the dots show Local's", () => {
  assert.deepEqual(stepsFor(null), stepsFor("local"));
});

test("Sign in leads to the choice on either path", () => {
  assert.equal(stepAfter("signin", null), "where");
  assert.equal(stepAfter("signin", "cloud"), "where");
});

test("each path walks its own steps, and back to the choice", () => {
  assert.equal(stepAfter("where", "local"), "runtime");
  assert.equal(stepAfter("where", "cloud"), "cloud");
  assert.equal(stepBefore("runtime", "local"), "where");
  assert.equal(stepBefore("cloud", "cloud"), "where");
  assert.equal(stepAfter("codingagent", "local"), "done");
  assert.equal(stepAfter("done", "local"), null);
  assert.equal(stepAfter("cloud", "cloud"), null);
  assert.equal(stepBefore("welcome", null), null);
});

test("a step opened directly finds its own path", () => {
  assert.equal(pathOf("welcome"), null);
  assert.equal(pathOf("where"), null);
  assert.equal(pathOf("reviewkey"), "local");
  assert.equal(pathOf("cloud"), "cloud");
  assert.equal(stepBefore("cloud", null), "where");
  assert.equal(stepAfter("runtime", null), "reviewkey");
});

test("Cloud needs a sign-in to the registry being browsed", () => {
  const regs = [
    { url: "https://secureagentry.net", hasToken: false },
    { url: "https://dev.secureagentry.net/", hasToken: true },
  ];
  assert.equal(cloudOpen(regs, "https://secureagentry.net"), false);
  assert.equal(cloudOpen(regs, "https://dev.secureagentry.net"), true);
  assert.equal(cloudOpen([{ url: "https://secureagentry.net", hasToken: false }], ""), false);
  assert.equal(cloudOpen(regs, ""), true);
  assert.equal(cloudOpen([], ""), false);
});

test("each path sets where runs start", () => {
  assert.equal(runWhereFor("local"), "here");
  assert.equal(runWhereFor("cloud"), "agentry");
});

test("back on the choice, the path is open again and the dots show Local's", () => {
  assert.equal(pathAt("where", "cloud"), null);
  assert.equal(pathAt("signin", "local"), null);
  assert.equal(pathAt("runtime", "local"), "local");
  assert.equal(pathAt("cloud", "cloud"), "cloud");
});
