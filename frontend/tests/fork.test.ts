// Fork in the desktop, held to the site's fork rules.
import { test } from "node:test";
import assert from "node:assert/strict";
import { FORK_TERMS, forkable, forkCopyNotice, forkCountLine, forkOfLabel, forkProgressLine, type ForkAgent } from "../src/fork.ts";

const pub: ForkAgent = { visibility: "public" };
const origin = { namespace: "demo1", name: "incident-summarizer", version: "1.0.0", available: true };

test("Fork is offered on a public, published QuickAgent to a signed-in person, and on nothing else", () => {
  assert.equal(forkable(pub, true, true), true);
  assert.equal(forkable(pub, true, false), false, "signed out");
  assert.equal(forkable(pub, false, true), false, "nothing published");
  assert.equal(forkable({ visibility: "private" }, true, true), false, "private");
  assert.equal(forkable({ visibility: "public", quarantine: "quarantined" }, true, true), false, "quarantined");
  assert.equal(forkable({ visibility: "public", quarantine: "blocked" }, true, true), false, "blocked");
  assert.equal(forkable({ visibility: "private", forkedFrom: origin }, true, true), false, "a fork");
});

test("a fork's page names its origin and says how its copy stands", () => {
  const f: ForkAgent = { visibility: "private", forkedFrom: origin, forkState: "ready" };
  assert.equal(forkOfLabel(f), "Fork of demo1/incident-summarizer@1.0.0");
  assert.equal(forkCopyNotice(f), null);
  const copying = forkCopyNotice({ ...f, forkState: "copying" });
  assert.equal(copying?.title, "This fork is being made.");
  assert.match(copying?.text ?? "", /^Its history is being copied from demo1\/incident-summarizer and its first version submitted to the pipeline\. This page updates by itself\.$/);
  assert.deepEqual(forkCopyNotice({ ...f, forkState: "failed", forkDetail: "the origin's repository could not be copied" }), {
    title: "This fork could not be made.",
    text: "The origin's repository could not be copied. Delete this fork in the Publisher section, then fork the original again.",
    failed: true,
  });
  assert.match(forkCopyNotice({ ...f, forkState: "failed" })?.text ?? "", /^The copy did not finish\. /);
  assert.equal(forkOfLabel(pub), null);
});

test("the fork count is a number only, and absent at none", () => {
  assert.equal(forkCountLine(undefined), null);
  assert.equal(forkCountLine(0), null);
  assert.equal(forkCountLine(1), "1 fork");
  assert.equal(forkCountLine(1234), "1,234 forks");
});

test("the terms are the site's three facts", () => {
  assert.equal(FORK_TERMS.length, 3);
  assert.match(FORK_TERMS[0], /can never be made public/);
  assert.match(FORK_TERMS[1], /through the pipeline/);
  assert.match(FORK_TERMS[2], /Runs on your own computer pay nobody\./);
});

test("a new fork's progress: copying, the first version through the pipeline, then published or stopped", () => {
  const ref = "pat/incident-summarizer";
  assert.deepEqual(forkProgressLine(ref, { forkState: "copying", versions: [] }), { line: "Copying pat/incident-summarizer's history from its origin…", done: false });
  assert.deepEqual(forkProgressLine(ref, { forkState: "ready", versions: [{ version: "1.0.0", status: "scanning" }] }),
    { line: "pat/incident-summarizer's first version, v1.0.0, is going through the pipeline (scanning).", done: false });
  assert.equal(forkProgressLine(ref, { forkState: "ready", versions: [{ version: "1.0.0", status: "published" }] }).done, true);
  // The first version is the oldest, wherever later ones are.
  assert.match(forkProgressLine(ref, { forkState: "ready", versions: [{ version: "1.1.0", status: "scanning" }, { version: "1.0.0", status: "published" }] }).line, /v1\.0\.0, is published/);
  assert.equal(forkProgressLine(ref, { forkState: "ready", versions: [{ version: "1.0.0", status: "needs_review" }] }).done, false, "a review can still publish it");
  assert.deepEqual(forkProgressLine(ref, { forkState: "failed", forkDetail: "no repository", versions: [] }), { line: "The copy of pat/incident-summarizer did not finish. No repository.", done: true });
  assert.match(forkProgressLine(ref, { forkState: "ready", versions: [{ version: "1.0.0", status: "rejected" }] }).line, /stopped: rejected/);
});
