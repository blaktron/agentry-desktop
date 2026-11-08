// The built-ins' job store, driven by scripted events and calls in place of
// Go: a job on this computer is followed from its first report to what it
// made, keeping its log as every run window does; a No in the native dialog
// changes nothing; a second job is refused while one runs; Stop reaches the
// job on this computer, Cancel the one In Agentry; a job In Agentry is
// followed by its own id to its final word; and Dismiss waits for the end.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createBuiltinJobStore } from "../src/builtinJob.ts";
import { MAX_RUN_LOG_LINES } from "../src/runLog.ts";

type Cb = (v: never) => void;
type Hosted = { id: string; final: boolean; applied: boolean; status: string };
type Meta = { registry: string };

function harness() {
  const listeners: Record<string, Cb[]> = {};
  let stops = 0;
  let cancels = 0;
  const store = createBuiltinJobStore<{ made: string }, Hosted, Meta>({
    on: (event, cb) => {
      (listeners[event] ??= []).push(cb);
      return () => { listeners[event] = (listeners[event] ?? []).filter((c) => c !== cb); };
    },
    logEvent: "job:log",
    progressEvent: "job:progress",
    hostedEvent: "job:hosted",
    stop: async () => { stops++; },
    cancel: async () => { cancels++; return { id: "h1", final: false, applied: false, status: "running" }; },
    applied: (h) => h.applied,
  });
  const fire = (event: string, v: unknown) => (listeners[event] ?? []).forEach((cb) => cb(v as never));
  const progress = (turns: number) => fire("job:progress", { startedAt: 1, timeoutSeconds: 0, maxTurns: 0, turns, inputTokens: 10, outputTokens: 2 });
  return { store, fire, progress, stops: () => stops, cancels: () => cancels, listening: (e: string) => (listeners[e] ?? []).length };
}

function pending<T>() {
  let resolve!: (r: T) => void;
  let reject!: (e: unknown) => void;
  const p = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { call: () => p, resolve, reject };
}
const tick = () => new Promise((r) => setTimeout(r, 0));
const meta = { registry: "https://agentry.example" };

test("a job on this computer opens once Go reports it has begun, and lands with what it made", async () => {
  const h = harness();
  const p = pending<{ made: string }>();
  let started = false;
  const start = h.store.startHere(meta, p.call).then(() => { started = true; });
  await tick();
  assert.equal(h.store.state().shown, false, "nothing shows before the job has begun");
  h.progress(0);
  await start;
  assert.ok(started);
  let s = h.store.state();
  assert.equal(s.shown, true);
  assert.equal(s.raised, 1);
  assert.equal(s.job?.where, "here");
  assert.equal(s.job?.over, false);
  h.fire("job:log", "reading the samples");
  h.progress(2);
  s = h.store.state();
  assert.deepEqual(s.job?.log.lines, ["reading the samples"]);
  assert.equal(s.job?.progress?.turns, 2);
  p.resolve({ made: "VOICE.md" });
  await tick();
  s = h.store.state();
  assert.equal(s.job?.over, true);
  assert.deepEqual(s.job?.result, { made: "VOICE.md" });
  assert.equal(s.landed, 1);
  assert.equal(h.listening("job:log") + h.listening("job:progress"), 0, "the job's listeners go with it");
});

test("a job refused before it began (a No in the native dialog) rejects and shows nothing", async () => {
  const h = harness();
  const p = pending<{ made: string }>();
  const start = h.store.startHere(meta, p.call);
  p.reject(new Error("not confirmed"));
  await assert.rejects(start, /not confirmed/);
  assert.equal(h.store.state().shown, false);
  assert.equal(h.store.state().job, null);
  // and the next start is not held by it
  const q = pending<{ made: string }>();
  const again = h.store.startHere(meta, q.call);
  h.progress(0);
  await again;
  assert.equal(h.store.state().shown, true);
});

test("a job that fails after it began says why, keeps its log, and lands nothing", async () => {
  const h = harness();
  const p = pending<{ made: string }>();
  const start = h.store.startHere(meta, p.call);
  h.fire("job:log", "decode: 1 files");
  await start;
  p.reject(new Error("the Voice Writer wrote no VOICE.md"));
  await tick();
  const s = h.store.state();
  assert.equal(s.job?.over, true);
  assert.equal(s.job?.error, "the Voice Writer wrote no VOICE.md");
  assert.deepEqual(s.job?.log.lines, ["decode: 1 files"]);
  assert.equal(s.landed, 0);
});

test("the log keeps its newest lines and counts the rest, as a run's does", async () => {
  const h = harness();
  const start = h.store.startHere(meta, pending<{ made: string }>().call);
  for (let i = 0; i < MAX_RUN_LOG_LINES + 3; i++) h.fire("job:log", `line ${i}`);
  await start;
  const log = h.store.state().job!.log;
  assert.equal(log.lines.length, MAX_RUN_LOG_LINES);
  assert.equal(log.dropped, 3);
  assert.equal(log.lines[0], "line 3");
});

test("a second job is refused while one runs, and Dismiss waits for the end", async () => {
  const h = harness();
  const p = pending<{ made: string }>();
  const start = h.store.startHere(meta, p.call);
  h.progress(0);
  await start;
  await assert.rejects(h.store.startHere(meta, pending<{ made: string }>().call), /already running/);
  h.store.dismiss();
  assert.equal(h.store.state().shown, true, "a running job is not dismissed");
  p.resolve({ made: "x" });
  await tick();
  h.store.dismiss();
  assert.equal(h.store.state().shown, false);
  assert.equal(h.store.state().job, null);
});

test("Stop reaches the job on this computer, and the job ends stopped", async () => {
  const h = harness();
  const p = pending<{ made: string }>();
  const start = h.store.startHere(meta, p.call);
  h.progress(0);
  await start;
  await h.store.stop();
  assert.equal(h.stops(), 1);
  assert.equal(h.store.state().job?.stopping, true);
  p.reject(new Error("context canceled"));
  await tick();
  assert.equal(h.store.state().job?.stopped, true);
  await h.store.cancel();
  assert.equal(h.cancels(), 0, "Cancel is for a job In Agentry");
});

test("a job In Agentry is followed by its id to its final word, and Cancel asks the registry", async () => {
  const h = harness();
  h.store.followHosted(meta, { id: "h1", final: false, applied: false, status: "running" }, false);
  let s = h.store.state();
  assert.equal(s.shown, true);
  assert.equal(s.asCard, false);
  assert.equal(s.job?.where, "agentry");
  // another run's word changes nothing
  h.fire("job:hosted", { id: "other", final: true, applied: true, status: "finished" });
  assert.equal(h.store.state().job?.over, false);
  await h.store.stop();
  assert.equal(h.stops(), 0, "Stop is for a job on this computer");
  await h.store.cancel();
  assert.equal(h.cancels(), 1);
  h.fire("job:hosted", { id: "h1", final: true, applied: true, status: "finished" });
  s = h.store.state();
  assert.equal(s.job?.over, true);
  assert.equal(s.job?.hosted?.applied, true);
  assert.equal(s.landed, 1);
});

test("a job In Agentry found under way is followed as its card; asking to show it opens the dialog", () => {
  const h = harness();
  const run = { id: "h2", final: false, applied: false, status: "running" };
  h.store.followHosted(meta, run, true);
  assert.equal(h.store.state().asCard, true);
  assert.ok(h.store.follows("h2"));
  const raised = h.store.state().raised;
  h.store.followHosted(meta, run, false);
  assert.equal(h.store.state().asCard, false);
  assert.equal(h.store.state().raised, raised + 1);
});
