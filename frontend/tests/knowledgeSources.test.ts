// Create knowledge's sources (agentry-desktop#498): documents, a brief or
// both, the plan's warning, and Search the web's default and its words.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  MAX_BRIEF_BYTES, agentryWebLine, briefBytes, documentsRecap, localWebLine, readsLine, searches, sourcesProblem, sourcesWarning, webDefault, webRecap,
} from "../src/knowledgeSources.ts";

test("a build takes documents, a brief, or both, and a brief of at most 8 KiB", () => {
  assert.match(sourcesProblem(0, "") ?? "", /write a brief/);
  assert.match(sourcesProblem(0, "   \n ") ?? "", /write a brief/);
  assert.equal(sourcesProblem(0, "Espresso machine care, for new café staff."), null);
  assert.equal(sourcesProblem(2, ""), null);
  assert.equal(sourcesProblem(2, "and the winter menu"), null);
  assert.equal(sourcesProblem(0, "a".repeat(MAX_BRIEF_BYTES)), null);
  assert.match(sourcesProblem(0, "a".repeat(MAX_BRIEF_BYTES + 1)) ?? "", /8,193 bytes; a brief is at most 8 KiB/);
  // Bytes as Go counts them: UTF-8, once trimmed.
  assert.equal(briefBytes("  café  "), 5);
  assert.match(sourcesProblem(1, "é".repeat(4097)) ?? "", /8,194 bytes/);
});

test("Search the web is on by default only for a new pack with no documents", () => {
  assert.equal(webDefault(false, 0), true);
  assert.equal(webDefault(false, 1), false);
  assert.equal(webDefault(true, 0), false);
  assert.equal(webDefault(true, 3), false);
});

test("the warning is louder with no documents, and its words follow the web", () => {
  const none = sourcesWarning(0, true);
  assert.equal(none.tone, "warn");
  assert.equal(none.text, "Specific documents make the best knowledge. Without them the Creator researches the web and its own knowledge, which can be wrong or out of date.");
  assert.equal(sourcesWarning(2, true).tone, "info");
  assert.match(sourcesWarning(0, false).text, /writes from the brief and its own knowledge/);
});

test("on this computer the web takes the person's keys, or says why a build runs without it", () => {
  assert.match(localWebLine({ ready: true, provider: "Brave" }), /searches with Brave and your key, and AI Review reads every result first/);
  assert.equal(localWebLine({ ready: false, why: "no Brave key was found" }), "On this computer a build runs without the web: no Brave key was found.");
  assert.match(localWebLine(null), /your own AI Review key and a search key/);
  assert.match(agentryWebLine(true, "Searching the web can raise the credits a build uses."), /registry searches.*can raise the credits/);
  assert.match(agentryWebLine(false, undefined), /does not search the web/);
});

test("the Review step says whether the build will search, where it runs", () => {
  const ready = { ready: true, provider: "Brave" };
  const missing = { ready: false, why: "no search provider is set up" };
  assert.equal(webRecap(false, "local", ready, true), "No");
  assert.equal(webRecap(true, "local", ready, true), "Yes, with your own keys");
  assert.equal(webRecap(true, "local", missing, true), "No: no search provider is set up");
  assert.equal(webRecap(true, "agentry", missing, true), "Yes, through the registry");
  assert.match(webRecap(true, "agentry", ready, false), /^No: In Agentry/);
  assert.equal(searches(true, "local", ready, false), true);
  assert.equal(searches(true, "local", missing, true), false);
  assert.equal(searches(true, "agentry", missing, true), true);
  assert.equal(searches(true, "agentry", ready, false), false);
  assert.equal(searches(false, "agentry", ready, true), false);
});

test("what a build reads, in words", () => {
  assert.equal(documentsRecap([]), "None: the Creator works from the brief");
  assert.equal(documentsRecap(["/a/guide.pdf"]), "/a/guide.pdf");
  assert.equal(documentsRecap(["/a", "/b"]), "2 files and folders");
  assert.equal(readsLine(0, true), "your brief");
  assert.equal(readsLine(1, true), "1 document and your brief");
  assert.equal(readsLine(3, false), "3 documents");
});
