// First run: one name for each place a run can happen across the wizard,
// Settings and Home; the sign-in copy that says what an account is for;
// the Node.js release the coding agent step names, from one constant; and
// what Home's banner lists after the wizard is skipped.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

import { MCP_NODE_MAJOR, PLACES, SIGN_IN_OPTIONAL, placeInSentence, skippedItems } from "../src/firstRun.ts";
import { START_NAME } from "../src/homeCards.ts";

const src = (p: string) => readFileSync(new URL(`../src/${p}`, import.meta.url), "utf8");

test("the wizard, Settings and Home name the three places the same way", () => {
  const wizard = src("pages/Wizard.tsx");
  for (const old of ['title="Local"', 'title="Remote"', 'title="Cloud"', "Local, Remote or Cloud"]) {
    assert.ok(!wizard.includes(old), `the wizard still says ${old}`);
  }
  for (const p of ["local", "remote", "cloud"]) assert.ok(wizard.includes(`title={PLACES.${p}}`), `the wizard's ${p} card`);
  assert.match(src("pages/Settings.tsx"), /\["here", PLACES\.local\], \["agentry", PLACES\.cloud\]/);
  assert.deepEqual(START_NAME, { local: "this computer", remote: "a runhost", cloud: "In Agentry" });
  assert.deepEqual(PLACES, { local: "This computer", remote: "A runhost", cloud: "In Agentry" });
  assert.equal(placeInSentence("cloud"), "In Agentry");
});

test("no first-run copy says the app cannot sign in or needs an account to run", () => {
  for (const file of ["pages/Wizard.tsx", "App.tsx"]) {
    const text = src(file).replace(/\s+/g, " ");
    assert.ok(!/cannot sign in/.test(text), `${file} says the app cannot sign in`);
    assert.ok(!/Sign in to install and run/.test(text), `${file} says signing in is needed to run`);
  }
  assert.match(SIGN_IN_OPTIONAL, /optional/);
  assert.match(SIGN_IN_OPTIONAL, /running them on this computer need no account/);
});

test("the sign-in step has one button that goes on", () => {
  const wizard = src("pages/Wizard.tsx");
  assert.ok(!wizard.includes("Skip, browse anonymously"));
  assert.ok(wizard.includes('{signedIn ? "Continue" : "Continue without signing in"}'));
});

test("the coding agent step names the MCP server's Node.js release from the constant", () => {
  const wizard = src("pages/Wizard.tsx").replace(/\s+/g, " ");
  assert.ok(!/Node 20\b/.test(wizard));
  assert.ok(wizard.includes("{MCP_NODE_MAJOR} or newer"));
  assert.equal(MCP_NODE_MAJOR, 24);
});

test("a skipped wizard leaves a banner item for each thing not done, opening its step", () => {
  assert.deepEqual(skippedItems({ signedIn: true, providerSetUp: true }), []);
  const both = skippedItems({ signedIn: false, providerSetUp: false });
  assert.deepEqual(both.map((i) => i.step), ["where", "signin"]);
  assert.match(both[0].text, /on this computer/);
  assert.match(both[1].text, /optional/);
  assert.deepEqual(skippedItems({ signedIn: false, providerSetUp: true }).map((i) => i.step), ["signin"]);
  // The Welcome page's skip is its own outcome, so the App can raise it.
  assert.ok(src("pages/Wizard.tsx").includes('onSkip={() => onFinish({ kind: "skipped" })}'));
  assert.ok(src("App.tsx").includes('outcome.kind === "skipped"'));
});
