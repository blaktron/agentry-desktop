// The run In Agentry store (agentry-desktop#466), driven by scripted events
// and calls in place of Go: the run is followed by its id for the app's
// life, a run that ends with its dialog closed lands in its card with its
// result, and a refused start follows nothing.
import { test } from "node:test";
import assert from "node:assert/strict";
import type { main } from "../wailsjs/go/models";
import { createHostedRunStore, hostedHeadline, hostedProgress, lostTouch } from "../src/hostedRun.ts";

type Cb = (v: never) => void;
type View = main.HostedRunView;

function harness() {
  const listeners: Record<string, Cb[]> = {};
  const forgotten: string[] = [];
  let cancels = 0;
  let cancelAnswer: (() => Promise<View>) | null = null;
  const store = createHostedRunStore<string>({
    on: (event, cb) => {
      (listeners[event] ??= []).push(cb);
      return () => { listeners[event] = (listeners[event] ?? []).filter((c) => c !== cb); };
    },
    cancel: () => {
      cancels++;
      return cancelAnswer ? cancelAnswer() : Promise.reject("no run In Agentry is in progress");
    },
    forget: (id) => { forgotten.push(id); },
  });
  const report = (v: View) => (listeners["runner:hosted-run"] ?? []).forEach((cb) => cb(v as never));
  return { store, report, forgotten, cancels: () => cancels, answerCancel: (f: () => Promise<View>) => { cancelAnswer = f; } };
}

const view = (id: string, status: string, extra: Partial<View> = {}) =>
  ({ id, ref: "acme/reporter@1.0.0", status, state: status, statusLine: status, final: false, outputs: [], turns: 0,
    inputTokens: 0, outputTokens: 0, totalTokens: 0, holdCredits: 20, ...extra }) as unknown as View;
const REG = "https://dev.example.test";
const started = (v: View) => async () => ({ run: v }) as main.HostedStartView;

test("a started run is followed by its id, and another run's reports change nothing", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "queued", { queuePosition: 2 })));
  let s = h.store.state();
  assert.equal(s.run?.id, "r1");
  assert.equal(s.setup, "setup A");
  assert.equal(s.shown, true);
  assert.equal(s.asCard, false);
  h.report(view("r9", "running"));
  h.report(view("r1", "running", { startedAt: "2026-09-30T12:00:00Z" }));
  s = h.store.state();
  assert.equal(s.run?.status, "running");
  assert.equal(hostedProgress(s.run!)?.startedAt, Date.parse("2026-09-30T12:00:00Z"));
});

test("a run that ends while its dialog is closed lands in its card with its result", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "running")));
  // The flow was minimized: nothing in the store says the dialog is open,
  // and the final event still lands.
  h.report(view("r1", "done", { final: true, outcome: "succeeded", outcomeTone: "ok", resultJson: "\"the summary\"" }));
  const s = h.store.state();
  assert.equal(s.run?.final, true);
  assert.equal(s.run?.resultJson, "\"the summary\"");
  assert.equal(hostedHeadline(s.run!), "Finished.");
  // A report after the last word changes nothing.
  h.report(view("r1", "running"));
  assert.equal(h.store.state().run?.status, "done");
});

test("a refused start follows nothing, and the setup gets the refusal", async () => {
  const h = harness();
  const v = await h.store.start("setup A", REG, async () => ({ refusal: "credits-exhausted", detail: "No credits left." }) as main.HostedStartView);
  assert.equal(v.refusal, "credits-exhausted");
  const s = h.store.state();
  assert.equal(s.run, null);
  assert.equal(s.shown, false);
});

test("a report that arrives before the registry's answer is not lost to it", async () => {
  const h = harness();
  let answer!: (v: main.HostedStartView) => void;
  const begun = h.store.start("setup A", REG, () => new Promise((res) => { answer = res; }));
  await Promise.resolve();
  h.report(view("r1", "running"));
  answer({ run: view("r1", "queued") } as main.HostedStartView);
  await begun;
  assert.equal(h.store.state().run?.status, "running");
});

test("a start while one waits for the registry is refused", async () => {
  const h = harness();
  let answer!: (v: main.HostedStartView) => void;
  const first = h.store.start("setup A", REG, () => new Promise((res) => { answer = res; }));
  await assert.rejects(h.store.start("setup B", REG, started(view("r2", "queued"))), /being started/);
  answer({ run: view("r1", "queued") } as main.HostedStartView);
  await first;
  assert.equal(h.store.state().run?.id, "r1");
});

test("a call that throws before it returns leaves the store able to start again", async () => {
  const h = harness();
  await assert.rejects(h.store.start("setup A", REG, () => { throw new Error("binding missing"); }), /binding missing/);
  await h.store.start("setup B", REG, started(view("r2", "queued")));
  assert.equal(h.store.state().run?.id, "r2");
});

test("a run attached at start opens as its card, with no setup to go back to", async () => {
  const h = harness();
  assert.equal(await h.store.attach(REG, started(view("r1", "running"))), true);
  const s = h.store.state();
  assert.equal(s.run?.id, "r1");
  assert.equal(s.asCard, true);
  assert.equal(s.setup, null);
  assert.equal(await h.store.attach(REG, async () => ({}) as main.HostedStartView), false, "a second attach over a moving run");
});

test("Cancel asks the registry and shows what it answered; a refused cancel says why", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "running")));
  await h.store.cancel();
  assert.equal(h.cancels(), 1);
  assert.match(h.store.state().note ?? "", /no run In Agentry/);
  h.answerCancel(async () => view("r1", "running", { cancelRequested: true }));
  await h.store.cancel();
  const s = h.store.state();
  assert.equal(s.run?.cancelRequested, true);
  assert.equal(s.note, null);
});

test("Dismiss forgets a run that is over, and is refused while it moves", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "running")));
  h.store.dismiss();
  assert.equal(h.store.state().run?.id, "r1", "dismissed while it moved");
  h.report(view("r1", "done", { final: true, outcome: "failed", outcomeTone: "bad", outcomeLine: "the agent failed" }));
  h.store.dismiss();
  assert.deepEqual(h.forgotten, ["r1"]);
  assert.equal(h.store.state().run, null);
  assert.equal(h.store.state().shown, false);
});

test("a new run over a card that is over forgets the earlier run", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "running")));
  h.report(view("r1", "done", { final: true, outcome: "succeeded", outcomeTone: "ok" }));
  await h.store.start("setup B", REG, started(view("r2", "queued")));
  assert.deepEqual(h.forgotten, ["r1"]);
  assert.equal(h.store.state().setup, "setup B");
});

test("a run the app lost touch with can be followed again, and keeps its setup", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "running")));
  h.report(view("r1", "running", { final: true, downloadError: "lost touch with the registry while the run was running" }));
  let s = h.store.state();
  assert.equal(lostTouch(s.run!), true);
  assert.match(hostedHeadline(s.run!), /lost touch/);
  assert.equal(await h.store.attach(REG, started(view("r1", "running")), false), true);
  s = h.store.state();
  assert.equal(s.run?.final, false);
  assert.equal(s.setup, "setup A");
  assert.deepEqual(h.forgotten, [], "following it again forgot it");
  h.report(view("r1", "done", { final: true, outcome: "succeeded", outcomeTone: "ok" }));
  assert.equal(h.store.state().run?.outcome, "succeeded");
});

test("following again a run Agentry no longer lists says so", async () => {
  const h = harness();
  await h.store.start("setup A", REG, started(view("r1", "running")));
  h.report(view("r1", "running", { final: true, downloadError: "lost touch" }));
  assert.equal(await h.store.attach(REG, async () => ({}) as main.HostedStartView, false), false);
  assert.match(h.store.state().note ?? "", /no run of this account in progress/);
});

test("the store keeps the registry a run is followed on, and says while a start waits", async () => {
  const h = harness();
  let answer!: (v: main.HostedStartView) => void;
  const begun = h.store.start("setup A", "https://one.example.test", () => new Promise((res) => { answer = res; }));
  await Promise.resolve();
  assert.equal(h.store.state().starting, true);
  answer({ run: view("r1", "running") } as main.HostedStartView);
  await begun;
  const s = h.store.state();
  assert.equal(s.starting, false);
  assert.equal(s.registry, "https://one.example.test");
});
