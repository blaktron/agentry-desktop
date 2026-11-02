// An agent that stays running in the Build tab: the summary's rows for it
// and when it is installed as a persistent agent.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  NO_CAPABILITIES,
  NO_SUBAGENTS,
  capabilityLine,
  offersAgentInstall,
  offersLibraryInstall,
  persistentActionNote,
  personaLine,
  showsPersistent,
  submittedRef,
  subagentLine,
} from "../src/builderPersistent.ts";

test("only a draft that supports the persistent mode has the persistent part", () => {
  assert.equal(showsPersistent({}), false);
  assert.equal(showsPersistent({ persistent: null }), false);
  assert.equal(showsPersistent({ persistent: { only: true, capabilities: [], personaName: "", subagents: [] } }), true);
});

test("a declared key is one line in the core's words, with the names a list key declares", () => {
  assert.equal(capabilityLine({ words: "the time on each turn, its own tasks and the heartbeat that fires them", names: [] }),
    "The time on each turn, its own tasks and the heartbeat that fires them.");
  assert.equal(capabilityLine({ words: "stored secrets you keep for it in your keychain", names: ["PENNY_KEY", "OTHER_KEY"] }),
    "Stored secrets you keep for it in your keychain: PENNY_KEY, OTHER_KEY.");
  assert.equal(capabilityLine({ words: "your mail, at the providers you sign in to", names: ["google"] }),
    "Your mail, at the providers you sign in to: google.");
});

test("the persona's name is said, or that the agent names none", () => {
  assert.equal(personaLine(" Penny "), "Takes on the persona Penny.");
  assert.match(personaLine(""), /no persona/i);
  assert.match(personaLine("   "), /no persona/i);
});

test("a subagent is its reference, the versions it accepts and the reason it gives", () => {
  assert.deepEqual(subagentLine({ ref: "acme/pdf-summary", version: "^1.2.0", why: " Summarises a PDF. " }),
    { head: "acme/pdf-summary (version ^1.2.0)", why: "Summarises a PDF." });
  assert.deepEqual(subagentLine({ ref: "acme/x", version: "", why: "" }), { head: "acme/x", why: "" });
});

test("what the summary says when nothing is declared is plain", () => {
  assert.match(NO_CAPABILITIES, /nothing beyond an ordinary run/);
  assert.match(NO_SUBAGENTS, /no other QuickAgent/);
});

test("a persistent-only draft has no local install and no run", () => {
  assert.equal(offersLibraryInstall(["persistent"]), false);
  assert.equal(offersLibraryInstall(["autonomous", "persistent"]), true);
  assert.equal(offersLibraryInstall(["autonomous"]), true);
  assert.equal(offersLibraryInstall(undefined), true);
  assert.doesNotMatch(persistentActionNote(true), /\bRun\b/);
  assert.match(persistentActionNote(true), /no run of its own/);
  assert.doesNotMatch(persistentActionNote(false), /no run of its own/);
});

test("Install as a persistent agent is offered once the pipeline published a persistent draft", () => {
  assert.equal(offersAgentInstall(["persistent"], "published"), true);
  assert.equal(offersAgentInstall(["autonomous", "persistent"], "published"), true);
  for (const status of ["needs_review", "rejected", "refused", undefined]) {
    assert.equal(offersAgentInstall(["persistent"], status), false, `${status}`);
  }
  assert.equal(offersAgentInstall(["autonomous"], "published"), false);
  assert.equal(offersAgentInstall(undefined, "published"), false);
});

test("the install names the version that was submitted", () => {
  assert.equal(submittedRef({ namespace: "alice", name: "notes-keeper" }, "0.1.0"), "alice/notes-keeper@0.1.0");
});
