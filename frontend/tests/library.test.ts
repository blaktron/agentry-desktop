// The Library's search (plan desktop-home-and-library.md D3;
// agentry-desktop#657): typing filters what is installed on this computer,
// and the way to the registry always carries what was typed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { filterInstalled, registrySearchLabel, registryQuery } from "../src/library.ts";

const rec = (namespace: string, name: string) => ({ namespace, name });
const recs = [rec("agentry", "changelog-from-git"), rec("demo1", "incident-summarizer"), rec("me", "csv-quality-check")];

test("an empty search keeps every record, in order", () => {
  assert.deepEqual(filterInstalled(recs, ""), recs);
  assert.deepEqual(filterInstalled(recs, "   "), recs);
});

test("the search matches the namespace/name anywhere, ignoring case and the edges' spaces", () => {
  assert.deepEqual(filterInstalled(recs, "INCIDENT").map((r) => r.name), ["incident-summarizer"]);
  assert.deepEqual(filterInstalled(recs, " me/csv ").map((r) => r.name), ["csv-quality-check"]);
  assert.deepEqual(filterInstalled(recs, "git").map((r) => r.name), ["changelog-from-git"]);
  assert.deepEqual(filterInstalled(recs, "-"), recs);
  assert.deepEqual(filterInstalled(recs, "nothing-like-it"), []);
});

test("the registry button names what will be searched, or offers more when nothing is typed", () => {
  assert.equal(registrySearchLabel(""), "Find more QuickAgents");
  assert.equal(registrySearchLabel("  "), "Find more QuickAgents");
  assert.equal(registrySearchLabel(" pager duty "), "Search the registry for “pager duty”");
});

test("the query handed to Browse is trimmed, and absent when nothing is typed", () => {
  assert.equal(registryQuery(" pager "), "pager");
  assert.equal(registryQuery("   "), undefined);
});
