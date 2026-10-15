// The review store, driven by scripted events and calls
// in place of Go: a review is followed from its first report to its report;
// a No in the native dialog and a refusal change nothing; a second review
// never touches the one being followed until its own has begun; Stop and
// Dismiss reach the review they mean.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import { createReviewStore, outcome, outcomeTone, type ReviewProgress } from "../src/review.ts";

type Cb = (v: never) => void;

function harness() {
  const listeners: Record<string, Cb[]> = {};
  const forgotten: string[] = [];
  let stops = 0;
  const store = createReviewStore({
    on: (event, cb) => {
      (listeners[event] ??= []).push(cb);
      return () => { listeners[event] = (listeners[event] ?? []).filter((c) => c !== cb); };
    },
    stop: async () => { stops++; },
    forget: (id) => { forgotten.push(id); },
  });
  const fire = (event: string, v: unknown) => (listeners[event] ?? []).forEach((cb) => cb(v as never));
  const report = (runId: string, stage: ReviewProgress["stage"] = "rules", turns = 0) =>
    fire("build:review-progress", { runId, stage, startedAt: 1, timeoutSeconds: 1800, maxTurns: 60, turns, inputTokens: 0, outputTokens: 0 } as ReviewProgress);
  const line = (text: string) => fire("build:review-log", text);
  return { store, report, line, forgotten, stops: () => stops, listening: () => Object.values(listeners).flat().length };
}

function pending() {
  let resolve!: (r: app.ReviewView | null) => void;
  let reject!: (e: unknown) => void;
  const p = new Promise<app.ReviewView | null>((res, rej) => { resolve = res; reject = rej; });
  return { call: () => p, resolve, reject };
}
const view = (runId: string, o = "needs_review", words = "held for review") => ({ runId, outcome: o, outcomeWords: words, ref: "example/x@0.1.0" }) as unknown as app.ReviewView;
const tick = () => new Promise((r) => setTimeout(r, 0));

test("a review is followed from its first report to its report", async () => {
  const h = harness();
  const a = pending();
  const begun = h.store.start("/q/x", a.call);
  assert.equal(h.store.state().shown, false, "the flow opened before the review began");
  h.line("a line before it began");
  h.report("r1");
  assert.equal(await begun, true);
  h.line("one");
  h.report("r1", "examining", 4);
  let s = h.store.state();
  assert.equal(s.review?.runId, "r1");
  assert.equal(s.review?.progress?.stage, "examining");
  assert.deepEqual(s.review?.log, ["one"]);
  assert.equal(s.shown, true);
  a.resolve(view("r1"));
  await tick();
  s = h.store.state();
  assert.equal(s.review?.over, true);
  assert.equal(s.review?.view?.runId, "r1");
  assert.deepEqual(outcome(s.review!), { line: "Expected at Submit: held for review", tone: "warn" });
  assert.equal(h.listening(), 0, "the review's listeners outlived it");
  h.store.dismiss();
  assert.deepEqual(h.forgotten, ["r1"]);
  assert.equal(h.store.state().review, null);
});

test("a No in the native dialog shows and forgets nothing", async () => {
  const h = harness();
  const a = pending();
  const begun = h.store.start("/q/x", a.call);
  a.resolve(null);
  assert.equal(await begun, false);
  assert.equal(h.store.state().shown, false);
  assert.equal(h.store.state().review, null);
  await tick();
  assert.equal(h.listening(), 0);
  // The No left nothing starting: the next review begins.
  const b = pending();
  const again = h.store.start("/q/x", b.call);
  h.report("r2");
  assert.equal(await again, true);
});

test("a refused start leaves the review being followed as it is", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("/q/a", a.call);
  h.report("r1");
  await aBegun;

  const b = pending();
  let bSettled = "";
  const bBegun = h.store.start("/q/b", b.call).then(() => { bSettled = "begun"; }, (e) => { bSettled = `refused: ${e}`; });
  // A's reports go on while B waits; they are A's, never B's.
  h.report("r1", "examining", 3);
  h.line("A's line");
  await tick();
  assert.equal(bSettled, "");
  assert.equal(h.store.state().review?.dir, "/q/a");
  assert.deepEqual(h.store.state().review?.log, ["A's line"]);
  b.reject("A review by the Examiner is under way on this computer, and one job runs here at a time: wait for it to finish, or stop it.");
  await bBegun;
  assert.match(bSettled, /^refused: A review by the Examiner is under way/);
  assert.equal(h.store.state().review?.runId, "r1");
  assert.deepEqual(h.forgotten, []);
});

test("a second review, once begun, takes over from one that is over, which is forgotten", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("/q/a", a.call);
  h.report("r1");
  await aBegun;
  a.resolve(view("r1", "published", "published"));
  await tick();
  const b = pending();
  const bBegun = h.store.start("/q/b", b.call);
  h.report("r2");
  await bBegun;
  assert.equal(h.store.state().review?.runId, "r2");
  assert.deepEqual(h.forgotten, ["r1"]);
});

test("Stop marks the review stopped, and Dismiss waits until it is over", async () => {
  const h = harness();
  const a = pending();
  const begun = h.store.start("/q/x", a.call);
  h.report("r1");
  await begun;
  h.store.dismiss();
  assert.notEqual(h.store.state().review, null, "Dismiss forgot a review still running");
  await h.store.stop();
  assert.equal(h.stops(), 1);
  assert.equal(h.store.state().review?.stopping, true);
  a.resolve(view("r1", "undetermined", "not determined here"));
  await tick();
  const r = h.store.state().review!;
  assert.equal(r.stopped, true);
  assert.equal(outcome(r).tone, "warn");
});

test("a review that fails after it began ends with Go's words", async () => {
  const h = harness();
  const a = pending();
  const begun = h.store.start("/q/x", a.call);
  h.report("r1");
  await begun;
  a.reject("the report could not be written");
  await tick();
  const r = h.store.state().review!;
  assert.equal(r.over, true);
  assert.equal(r.error, "the report could not be written");
  assert.deepEqual(outcome(r), { line: "The review did not finish.", tone: "bad" });
});

test("an outcome's tone says what the pipeline would be expected to do, never a verdict", () => {
  assert.equal(outcomeTone("published"), "ok");
  assert.equal(outcomeTone("needs_review"), "warn");
  assert.equal(outcomeTone("undetermined"), "warn");
  assert.equal(outcomeTone("rejected"), "bad");
  assert.equal(outcomeTone("refused_at_submit"), "bad");
  assert.equal(outcomeTone("anything else"), "warn");
});
