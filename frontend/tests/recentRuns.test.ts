// What Recent runs and the run page say of a run.
import { test } from "node:test";
import assert from "node:assert/strict";
import { builtinPlace, emptyLine, headline, howItEnded, nameOf, rowLine, versionOf, whatRan, whenRan, whereRan } from "../src/recentRuns.ts";

const now = new Date("2026-10-05T12:00:00Z");

test("a row names the QuickAgent, a chat with it, or the built-in's job", () => {
  assert.equal(whatRan({ kind: "run", ref: "acme/summarise@1.2.0" }), "acme/summarise");
  assert.equal(whatRan({ kind: "chat", ref: "acme/summarise@1.2.0" }), "A chat with acme/summarise");
  assert.equal(whatRan({ kind: "run" }), "A QuickAgent");
  assert.equal(whatRan({ kind: "draft" }), "A draft by the Drafter");
  assert.equal(whatRan({ kind: "review" }), "A review by the Examiner");
  assert.equal(whatRan({ kind: "knowledge" }), "A knowledge build");
  assert.equal(whatRan({ kind: "voice" }), "A Writing Voice build");
  assert.equal(nameOf("acme/x@2.0.0"), "acme/x");
  assert.equal(versionOf("acme/x@2.0.0"), "2.0.0");
  assert.equal(versionOf(undefined), "");
});

test("where and how it ended, here in the app's words and In Agentry in the site's", () => {
  assert.equal(whereRan({ where: "here" }), "On this computer");
  assert.equal(whereRan({ where: "agentry" }), "In Agentry");
  assert.equal(howItEnded({ where: "here", outcome: "ok" }), "Finished");
  assert.equal(howItEnded({ where: "here", outcome: "stopped" }), "Stopped");
  // A run that ended blocked (exit 18) is neither an error nor a pass.
  assert.equal(howItEnded({ where: "here", outcome: "blocked", exitCode: 18 }), "Blocked");
  assert.equal(headline({ kind: "run", outcome: "blocked", exitCode: 18 }), "It was blocked: the QuickAgent could not do the task with what it had.");
  assert.equal(howItEnded({ where: "here", outcome: "error", exitCode: 3 }), "Ended in an error (exit 3)");
  assert.equal(howItEnded({ where: "here", outcome: "error" }), "Did not finish");
  assert.equal(howItEnded({ where: "agentry", outcome: "succeeded", state: "done — pass" }), "done — pass");
  assert.equal(howItEnded({ where: "agentry", outcome: "running", state: "running" }), "running");
  assert.equal(headline({ kind: "run", outcome: "error", exitCode: 2 }), "It ended in an error (exit 2).");
  assert.equal(headline({ kind: "draft", outcome: "error" }), "The job did not finish.");
  assert.equal(headline({ kind: "review", outcome: "stopped" }), "The job was stopped.");
});

test("when is relative, and a run still moving says when it started", () => {
  assert.equal(whenRan({ endedAt: "2026-10-05T11:59:40Z" }, now), "just now");
  assert.equal(whenRan({ endedAt: "2026-10-05T11:55:00Z" }, now), "5 minutes ago");
  assert.equal(whenRan({ endedAt: "2026-10-05T11:59:00Z" }, now), "1 minute ago");
  assert.equal(whenRan({ endedAt: "2026-10-05T09:00:00Z" }, now), "3 hours ago");
  assert.equal(whenRan({ endedAt: "2026-10-04T10:00:00Z" }, now), "yesterday");
  assert.equal(whenRan({ endedAt: "2026-10-01T12:00:00Z" }, now), "4 days ago");
  assert.equal(whenRan({ startedAt: "2026-10-05T11:50:00Z" }, now), "started 10 minutes ago");
  assert.equal(whenRan({}, now), "");
  assert.equal(whenRan({ endedAt: "not a time" }, now), "");
  assert.equal(
    rowLine({ id: "x", kind: "run", where: "here", outcome: "ok", tone: "ok", endedAt: "2026-10-05T11:55:00Z" }, now),
    "On this computer · 5 minutes ago · Finished",
  );
});

test("a built-in's run page links to its own place, a review's report is on the page", () => {
  assert.deepEqual(builtinPlace("draft"), { label: "Open in Build", page: "build" });
  assert.deepEqual(builtinPlace("knowledge"), { label: "Open in Knowledge", page: "knowledge" });
  assert.deepEqual(builtinPlace("voice"), { label: "Open in Voice", page: "voice" });
  assert.equal(builtinPlace("review"), null);
  assert.equal(builtinPlace("run"), null);
});

test("the empty card says why it is empty", () => {
  assert.match(emptyLine({ remember: true, signedIn: false }), /^No runs yet/);
  assert.match(emptyLine({ remember: false, signedIn: false }), /Remember recent runs is off/);
  assert.match(emptyLine({ remember: false, signedIn: true }), /No run In Agentry yet/);
});
