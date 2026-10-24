// The run store, driven by scripted events and calls in
// place of Go: a second Run never touches the run being followed, a refused
// start changes nothing, and Stop and Dismiss reach the run they mean.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { app } from "../wailsjs/go/models";
import { createRunStore, type RunProgress } from "../src/localRun.ts";

type Cb = (v: never) => void;

function harness() {
  const listeners: Record<string, Cb[]> = {};
  const forgotten: string[] = [];
  let stops = 0;
  const store = createRunStore<string>({
    on: (event, cb) => {
      (listeners[event] ??= []).push(cb);
      return () => { listeners[event] = (listeners[event] ?? []).filter((c) => c !== cb); };
    },
    stop: async () => { stops++; },
    forget: (id) => { forgotten.push(id); },
  });
  const fire = (event: string, v: unknown) => (listeners[event] ?? []).forEach((cb) => cb(v as never));
  const report = (runId: string, ref: string, turns = 0) =>
    fire("runner:run-progress", { runId, ref, startedAt: 1, timeoutSeconds: 900, maxTurns: 60, turns, inputTokens: 0, outputTokens: 0, prepared: true } as RunProgress);
  const line = (runId: string, text: string) => fire("runner:run-line", { runId, line: text });
  return { store, fire, report, line, forgotten, stops: () => stops, listening: () => Object.values(listeners).flat().length };
}

// A call standing in for RunAPI.RunAgent, settled by the test.
function pending() {
  let resolve!: (r: app.RunResultView) => void;
  let reject!: (e: unknown) => void;
  const p = new Promise<app.RunResultView>((res, rej) => { resolve = res; reject = rej; });
  return { call: () => p, resolve, reject };
}
const result = (runId: string, status = "ok", exitCode = 0) => ({ runId, status, exitCode }) as unknown as app.RunResultView;
const tick = () => new Promise((r) => setTimeout(r, 0));

test("a run is followed from its first report to its result", async () => {
  const h = harness();
  const a = pending();
  const begun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  h.report("r1", "acme/a@1.0.0");
  await begun;
  h.line("r1", "one");
  h.report("r1", "acme/a@1.0.0", 3);
  let s = h.store.state();
  assert.equal(s.run?.runId, "r1");
  assert.equal(s.run?.progress?.turns, 3);
  assert.deepEqual(s.run?.log, ["one"]);
  assert.equal(s.setup, "setup A");
  assert.equal(s.shown, true);
  a.resolve(result("r1"));
  await tick();
  s = h.store.state();
  assert.equal(s.run?.over, true);
  assert.equal(s.run?.result?.runId, "r1");
  assert.equal(h.listening(), 0, "the run's listeners outlived it");
});

test("a second Run of the same release, refused, never touches the run in the background", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  h.report("r1", "acme/a@1.0.0");
  await aBegun;

  const b = pending();
  let bSettled = "";
  const bBegun = h.store.start("setup B", "acme/a@1.0.0", b.call).then(() => { bSettled = "begun"; }, (e) => { bSettled = `refused: ${e}`; });
  // While B waits for its consent and Go's answer, A goes on.
  h.report("r1", "acme/a@1.0.0", 5);
  h.line("r1", "A's line");
  await tick();
  assert.equal(bSettled, "", "B's setup closed on A's report");
  assert.equal(h.store.state().run?.progress?.turns, 5);
  b.reject("A run of acme/a@1.0.0 is under way on this computer, and one job runs here at a time: wait for it to finish, or stop it.");
  await bBegun;
  assert.match(bSettled, /^refused: A run of acme\/a@1\.0\.0 is under way/);
  const s = h.store.state();
  assert.equal(s.run?.runId, "r1");
  assert.equal(s.run?.over, false, "the refusal ended the run in the background");
  assert.equal(s.run?.error, null);
  assert.equal(s.setup, "setup A");
  assert.deepEqual(s.run?.log, ["A's line"]);
  a.resolve(result("r1"));
  await tick();
  assert.equal(h.store.state().run?.result?.runId, "r1", "A's own result was lost");
  assert.deepEqual(h.forgotten, [], "a run was forgotten");
});

test("a run in the background that finishes while a second Run waits keeps its own result", async () => {
  const h = harness();
  const c = pending();
  const cBegun = h.store.start("setup C", "acme/c@1.0.0", c.call);
  h.report("r3", "acme/c@1.0.0");
  await cBegun;
  const d = pending();
  const dBegun = h.store.start("setup D", "acme/d@1.0.0", d.call).catch(() => "refused");
  c.resolve(result("r3"));
  await tick();
  d.reject("the consent was declined");
  assert.equal(await dBegun, "refused");
  const s = h.store.state();
  assert.equal(s.run?.runId, "r3");
  assert.equal(s.run?.over, true);
  assert.equal(s.run?.result?.runId, "r3");
  assert.equal(s.setup, "setup C");
});

test("a new run that begins over a card that is over forgets the earlier run", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  h.report("r1", "acme/a@1.0.0");
  await aBegun;
  a.resolve(result("r1"));
  await tick();
  const b = pending();
  const bBegun = h.store.start("setup B", "acme/b@1.0.0", b.call);
  h.report("r2", "acme/b@1.0.0");
  await bBegun;
  assert.equal(h.store.state().run?.runId, "r2");
  assert.equal(h.store.state().setup, "setup B");
  assert.deepEqual(h.forgotten, ["r1"]);
  // A's late events change nothing.
  h.line("r1", "late");
  assert.deepEqual(h.store.state().run?.log, []);
});

test("Stop reaches the run followed, and a result after it says it was stopped", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  h.report("r1", "acme/a@1.0.0");
  await aBegun;
  await h.store.stop();
  assert.equal(h.stops(), 1);
  assert.equal(h.store.state().run?.stopping, true);
  a.resolve(result("r1", "error", 130));
  await tick();
  assert.equal(h.store.state().run?.stopped, true);
});

test("Dismiss forgets the run, and the run's later reports and result change nothing", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  h.report("r1", "acme/a@1.0.0");
  await aBegun;
  h.store.dismiss();
  assert.deepEqual(h.forgotten, ["r1"]);
  assert.equal(h.store.state().run, null);
  assert.equal(h.store.state().shown, false);
  h.report("r1", "acme/a@1.0.0", 9);
  h.line("r1", "late");
  a.resolve(result("r1"));
  await tick();
  assert.equal(h.store.state().run, null, "a dismissed run came back");
});

test("a second start while the first waits for its run to begin is refused, and the first is followed", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  const b = pending();
  await assert.rejects(h.store.start("setup B", "acme/a@1.0.0", b.call), /A run is being started on this computer/);
  h.report("r1", "acme/a@1.0.0");
  await aBegun;
  assert.equal(h.store.state().setup, "setup A");
  a.resolve(result("r1"));
  await tick();
  assert.equal(h.store.state().run?.result?.runId, "r1");
  // A start refused by Go frees the way for the next one.
  const c = pending();
  const cBegun = h.store.start("setup C", "acme/c@1.0.0", c.call).catch(() => "refused");
  c.reject("the consent was declined");
  assert.equal(await cBegun, "refused");
  const d = pending();
  const dBegun = h.store.start("setup D", "acme/d@1.0.0", d.call);
  h.report("r4", "acme/d@1.0.0");
  await dBegun;
  assert.equal(h.store.state().run?.runId, "r4");
});

test("a call that throws before it returns is refused like any other, and the next start is not blocked", async () => {
  const h = harness();
  const threw = h.store.start("setup A", "acme/a@1.0.0", () => { throw new Error("binding missing"); });
  await assert.rejects(threw, /binding missing/);
  assert.equal(h.store.state().run, null);
  assert.equal(h.listening(), 0, "the refused start's listeners outlived it");
  const b = pending();
  const begun = h.store.start("setup B", "acme/b@1.0.0", b.call);
  h.report("r2", "acme/b@1.0.0");
  await begun;
  assert.equal(h.store.state().run?.runId, "r2");
});

test("a Run Go queues closes its setup at once, leaves the run followed alone, and takes the card over when it begins", async () => {
  const h = harness();
  const a = pending();
  const aBegun = h.store.start("setup A", "acme/a@1.0.0", a.call);
  h.report("r1", "acme/a@1.0.0");
  await aBegun;
  const raised = h.store.state().raised;

  const b = pending();
  const bBegun = h.store.start("setup B", "acme/b@1.0.0", b.call);
  h.fire("runner:run-queue", [{ runId: "r2", ref: "acme/b@1.0.0", position: 1, waitingFor: "A run of acme/a@1.0.0" }]);
  await bBegun;
  let s = h.store.state();
  assert.equal(s.run?.runId, "r1", "the queued run took the card before it began");
  assert.equal(s.setup, "setup A");

  // A's reports still reach A; a third start may begin waiting at once.
  h.report("r1", "acme/a@1.0.0", 4);
  assert.equal(h.store.state().run?.progress?.turns, 4);

  a.resolve(result("r1"));
  await tick();
  assert.equal(h.store.state().run?.over, true);
  h.report("r2", "acme/b@1.0.0");
  await tick();
  s = h.store.state();
  assert.equal(s.run?.runId, "r2", "the queued run did not take the card when it began");
  assert.equal(s.setup, "setup B");
  assert.equal(s.raised, raised, "a queued run's start raised the dialog");
  assert.deepEqual(h.forgotten, ["r1"]);
  b.resolve(result("r2"));
  await tick();
  assert.equal(h.store.state().run?.result?.runId, "r2");
  assert.equal(h.listening(), 0, "a run's listeners outlived it");
});

test("a queue entry already known, or of another release, is not this start's", async () => {
  const h = harness();
  h.store.setQueue([{ runId: "r5", ref: "acme/a@1.0.0", position: 1, waitingFor: "The container engine install" }]);
  const b = pending();
  let settled = false;
  const begun = h.store.start("setup", "acme/a@1.0.0", b.call).then(() => { settled = true; });
  h.fire("runner:run-queue", [
    { runId: "r5", ref: "acme/a@1.0.0", position: 1, waitingFor: "The container engine install" },
    { runId: "r6", ref: "acme/z@1.0.0", position: 2, waitingFor: "The container engine install" },
  ]);
  await tick();
  assert.equal(settled, false, "another run's queue entry closed this setup");
  // The known run beginning is not this start's either.
  h.report("r5", "acme/a@1.0.0");
  await tick();
  assert.equal(settled, false, "a known queued run's first report was taken as this start's");
  h.fire("runner:run-queue", [
    { runId: "r6", ref: "acme/z@1.0.0", position: 1, waitingFor: "A run of acme/a@1.0.0" },
    { runId: "r7", ref: "acme/a@1.0.0", position: 2, waitingFor: "A run of acme/a@1.0.0" },
  ]);
  await begun;
  assert.equal(settled, true);
  b.reject("the run was cancelled before it started");
  await tick();
  assert.equal(h.store.state().run, null, "a cancelled queued run left a card");
});

test("a queued run cancelled while another start waits leaves that start waiting", async () => {
  const h = harness();
  const a = pending();
  const aQueued = h.store.start("A", "acme/a@1.0.0", a.call);
  h.fire("runner:run-queue", [{ runId: "r1", ref: "acme/a@1.0.0", position: 1, waitingFor: "x" }]);
  await aQueued;
  const b = pending();
  void h.store.start("B", "acme/b@1.0.0", b.call).catch(() => {});
  a.reject("the run was cancelled before it started");
  await tick();
  await assert.rejects(h.store.start("C", "acme/c@1.0.0", pending().call), /being started/);
});
