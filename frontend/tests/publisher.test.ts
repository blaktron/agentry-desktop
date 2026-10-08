// The Publisher section's rows (plan forks-and-publisher.md §4.2, D5, D9;
// agentry-desktop#705), held to the site's lib/publisher.ts.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  countsLine, deleteNameMatches, forkOfLabel, listCounts, notAdminLine, rowActions, rowBadges, visibilityLabel, visibleIn,
  type PublisherRow,
} from "../src/publisher.ts";

const row = (over: Partial<PublisherRow> = {}): PublisherRow => ({
  namespace: "pat", name: "kept", visibility: "public", quarantineState: "none", ...over,
});
const fork = { namespace: "demo1", name: "incident-summarizer", version: "1.0.0", available: true };

test("badges come in the plan's order: Private, Fork, Retired, then a quarantine by its state", () => {
  assert.deepEqual(rowBadges(row()), []);
  assert.deepEqual(
    rowBadges(row({ visibility: "private", forkedFrom: fork, retiredAt: "2026-10-02T10:00:00Z", quarantineState: "quarantined" })).map((b) => b.label),
    ["Private", "Fork", "Retired", "Quarantined"],
  );
  assert.deepEqual(rowBadges(row({ quarantineState: "blocked" })).map((b) => [b.label, b.tone]), [["Blocked", "bad"]]);
});

test("a fork's row names what it was forked from", () => {
  assert.equal(forkOfLabel(row({ forkedFrom: fork })), "Fork of demo1/incident-summarizer@1.0.0");
  assert.equal(forkOfLabel(row()), null);
});

test("a public QuickAgent offers Make private and Retire, a private one Make public", () => {
  assert.deepEqual(rowActions(row()), { edit: true, visibility: "private", retire: "retire", forkStaysPrivate: false });
  assert.deepEqual(rowActions(row({ visibility: "private" })), { edit: true, visibility: "public", retire: "retire", forkStaysPrivate: false });
  assert.equal(visibilityLabel("private"), "Make private");
  assert.equal(visibilityLabel("public"), "Make public");
});

test("a retired QuickAgent offers Restore alone, and no Edit, since it takes no new version", () => {
  assert.deepEqual(rowActions(row({ visibility: "private", retiredAt: "2026-10-02T10:00:00Z" })), { edit: false, visibility: null, retire: "restore", forkStaysPrivate: false });
});

test("a fork never offers Make public, and says why", () => {
  assert.deepEqual(rowActions(row({ visibility: "private", forkedFrom: fork })), { edit: true, visibility: null, retire: "retire", forkStaysPrivate: true });
  assert.equal(rowActions(row({ visibility: "private", forkedFrom: fork, retiredAt: "2026-10-02T10:00:00Z" })).forkStaysPrivate, true);
});

test("the visibility filter is applied to the list; the empty filter keeps every row", () => {
  const rows = [row({ name: "a" }), row({ name: "b", visibility: "private" })];
  assert.deepEqual(visibleIn(rows, "").map((r) => r.name), ["a", "b"]);
  assert.deepEqual(visibleIn(rows, "private").map((r) => r.name), ["b"]);
  assert.deepEqual(visibleIn(rows, "public").map((r) => r.name), ["a"]);
});

test("the counts leave a retired QuickAgent out of the private count", () => {
  const c = listCounts([row(), row({ visibility: "private" }), row({ visibility: "private", retiredAt: "2026-10-02T10:00:00Z" })]);
  assert.deepEqual(c, { total: 3, private: 1, retired: 1 });
  assert.equal(countsLine(c), "3 QuickAgents · 1 private · 1 retired");
  assert.equal(countsLine({ total: 1, private: 0, retired: 0 }), "1 QuickAgent");
});

test("Delete's typed name must match exactly, but for the edges' spaces", () => {
  assert.equal(deleteNameMatches(" kept ", "kept"), true);
  assert.equal(deleteNameMatches("Kept", "kept"), false);
  assert.equal(deleteNameMatches("kep", "kept"), false);
});

test("the words never say archive, and name the organization's admins", () => {
  assert.equal(notAdminLine("crew"), "Only admins of crew can make its QuickAgents public or private, retire, restore or delete them.");
  for (const s of [notAdminLine("crew"), visibilityLabel("public")]) assert.doesNotMatch(s, /archiv/i);
});
