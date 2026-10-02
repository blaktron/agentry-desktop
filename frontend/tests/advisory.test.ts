import { test } from "node:test";
import assert from "node:assert/strict";
import { advisoryWords } from "../src/advisory.ts";

test("an advisory the gate rates as it reads names its own word and score", () => {
  assert.equal(advisoryWords({ severity: "high", rating: "high", cvssScore: 8.7 }), "high (CVSS 8.7)");
  assert.equal(advisoryWords({ severity: "critical", rating: "critical" }), "critical");
});

test("a High the gate rates medium reads as both", () => {
  assert.equal(advisoryWords({ severity: "high", rating: "medium", cvssScore: 7.1 }), "high (CVSS 7.1), which the gate rates medium");
});

test("a registry that serves no rating shows the advisory's own word", () => {
  assert.equal(advisoryWords({ severity: "high" }), "high");
});
