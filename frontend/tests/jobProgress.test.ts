// A job's clock, as the progress view and its card read it
// (agentry-desktop#464): minutes and seconds, then hours past the hour.
import { test } from "node:test";
import assert from "node:assert/strict";
import { fmtClock } from "../src/jobProgress.ts";

test("the clock reads m:ss under an hour and h:mm:ss past it", () => {
  assert.equal(fmtClock(0), "0:00");
  assert.equal(fmtClock(5.9), "0:05");
  assert.equal(fmtClock(135), "2:15");
  assert.equal(fmtClock(1800), "30:00");
  assert.equal(fmtClock(3600), "1:00:00");
  assert.equal(fmtClock(3600 + 12 * 60 + 35), "1:12:35");
});

test("a negative elapsed time reads as none", () => {
  assert.equal(fmtClock(-3), "0:00");
});
