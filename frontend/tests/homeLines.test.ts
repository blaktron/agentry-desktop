// What Home's cards say: the Installed QuickAgents card's line and names,
// and the lead line.
import { test } from "node:test";
import assert from "node:assert/strict";
import { greeting, installedLine, recentInstalls } from "../src/homeLines.ts";

const rec = (namespace: string, name: string, installedAt: string, present = true) => ({ namespace, name, installedAt, present });

test("the Installed line counts what is on this computer and what has an update", () => {
  assert.equal(installedLine(0, 0), "Nothing installed on this computer yet.");
  assert.equal(installedLine(1, 0), "1 QuickAgent on this computer.");
  assert.equal(installedLine(7, 0), "7 QuickAgents on this computer.");
  assert.equal(installedLine(7, 1), "7 QuickAgents on this computer, 1 with an update.");
  assert.equal(installedLine(7, 3), "7 QuickAgents on this computer, 3 with an update.");
});

test("the names are the newest installs on disk, each QuickAgent once, at most the number asked", () => {
  const recs = [
    rec("acme", "pr-triage", "2026-08-30T14:00:00Z"),
    rec("demo1", "incident-summarizer", "2026-09-02T09:00:00Z"),
    rec("acme", "release-notes", "2026-10-01T10:00:00Z", false),
    rec("demo1", "incident-summarizer", "2026-09-20T09:00:00Z"),
    rec("me", "csv-quality-check", "2026-09-30T08:10:00Z"),
  ];
  const got = recentInstalls(recs, 2).map((r) => `${r.namespace}/${r.name}@${r.installedAt.slice(0, 10)}`);
  assert.deepEqual(got, ["me/csv-quality-check@2026-09-30", "demo1/incident-summarizer@2026-09-20"]);
  assert.equal(recentInstalls(recs, 5).length, 3);
});

test("the lead line greets by display name, then handle, else says nothing", () => {
  assert.equal(greeting({ displayName: "Chris", handle: "chris" }), "Hello, Chris.");
  assert.equal(greeting({ displayName: "  ", handle: "chris" }), "Hello, chris.");
  assert.equal(greeting(null), null);
});
