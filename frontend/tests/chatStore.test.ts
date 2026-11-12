// The chat store, driven by scripted events and calls
// in place of Go: the store starts the session once however often the flow
// switches between dialog and card, collects a chat the engine ended with
// nothing open, and follows a chat In Agentry by its run's id.
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { applyEvent, createChatStore, newChat, working } from "../src/sessionChat.ts";

type Cb = (v: never) => void;

function harness() {
  const listeners: Record<string, Cb[]> = {};
  const calls: string[] = [];
  const forgotten: string[] = [];
  let endAnswer: () => Promise<app.RunResultView> = async () => ({ runId: "c1", status: "ok", exitCode: 0 }) as unknown as app.RunResultView;
  let sendAnswer: () => Promise<void> = async () => undefined;
  let steerAnswer: () => Promise<string> = async () => "sent";
  let interruptAnswer: () => Promise<void> = async () => undefined;
  const store = createChatStore<string>({
    on: (event, cb) => {
      (listeners[event] ??= []).push(cb);
      return () => { listeners[event] = (listeners[event] ?? []).filter((c) => c !== cb); };
    },
    sendLocal: async (t) => { calls.push(`send ${t}`); return sendAnswer(); },
    steerLocal: async (t) => { calls.push(`steer ${t}`); return steerAnswer(); },
    interruptLocal: async () => { calls.push("interrupt"); return interruptAnswer(); },
    addLocal: async (p) => { calls.push(`add ${p}`); },
    endLocal: async (f) => { calls.push(`end ${f}`); return endAnswer(); },
    sendHosted: async (t) => { calls.push(`hsend ${t}`); },
    addHosted: async (p) => { calls.push(`hadd ${p}`); },
    endHosted: async (f) => { calls.push(`hend ${f}`); },
    replay: async () => { calls.push("replay"); },
    forget: (id) => { forgotten.push(id); },
  });
  const fire = (event: string, v: unknown) => (listeners[event] ?? []).forEach((cb) => cb(v as never));
  return {
    store, fire, calls, forgotten,
    answerEnd: (f: () => Promise<app.RunResultView>) => { endAnswer = f; },
    answerSend: (f: () => Promise<void>) => { sendAnswer = f; },
    answerSteer: (f: () => Promise<string>) => { steerAnswer = f; },
    answerInterrupt: (f: () => Promise<void>) => { interruptAnswer = f; },
  };
}
const tick = () => new Promise((r) => setTimeout(r, 0));
const hostedRun = (id: string, extra: Partial<app.HostedRunView> = {}) =>
  ({ id, ref: "acme/helper@1.0.0", status: "running", state: "running", mode: "interactive", final: false, outputs: [], ...extra }) as unknown as app.HostedRunView;

test("the store starts a chat once, however often the flow moves between dialog and card", async () => {
  const h = harness();
  let starts = 0;
  h.store.startLocal("setup", "acme/helper@1.0.0", "", "Hello, what are we looking at?", async () => { starts++; });
  await tick();
  h.store.show();
  h.store.show();
  await tick();
  assert.equal(starts, 1);
  const s = h.store.state();
  assert.equal(s.chat?.starting, false);
  assert.equal(s.chat?.chat.lines.at(-1)?.kind, "assistant", "the opening is the first bubble once the session is up");
  assert.throws(() => h.store.startLocal("other", "acme/other@1.0.0", "", "", async () => {}), /already open/);
});

test("a chat the engine ended is collected with nothing open, so its lock and containers are released", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "summarise", "", async () => {});
  await tick();
  // The flow is not mounted at all here: only the store hears the end.
  h.fire("runner:session-event", { kind: "session_end", reason: "idle", time: new Date().toISOString() });
  await tick();
  const s = h.store.state();
  assert.deepEqual(h.calls, ["end false"]);
  assert.equal(s.chat?.over, true);
  assert.equal(s.chat?.result?.runId, "c1");
  // A second end event changes nothing.
  h.fire("runner:session-event", { kind: "session_end", reason: "idle" });
  await tick();
  assert.deepEqual(h.calls, ["end false"]);
});

test("a start Go refuses is the chat's error, over, with its setup to go back to", async () => {
  const h = harness();
  h.store.startLocal("setup A", "acme/helper@1.0.0", "", "", async () => { throw "A run of acme/x@1.0.0 is under way on this computer"; });
  await tick();
  const s = h.store.state();
  assert.equal(s.chat?.over, true);
  assert.match(s.chat?.error ?? "", /under way/);
  assert.equal(s.setup, "setup A");
  // Another chat can start now.
  h.store.startLocal("setup B", "acme/helper@1.0.0", "", "", async () => {});
  assert.equal(h.store.state().setup, "setup B");
});

test("the QuickAgent is working while a message is out, and a second message goes into its turn", async () => {
  const h = harness();
  let answer!: () => void;
  h.answerSend(() => new Promise((r) => { answer = r; }));
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  const sent = h.store.send("first");
  await tick();
  assert.equal(working(h.store.state().chat!), true);
  assert.equal(await h.store.send("second"), "sent");
  answer();
  await sent;
  assert.deepEqual(h.calls, ["send first", "steer second"]);
  assert.equal(working(h.store.state().chat!), false);
});

test("a harness that takes a message only between turns holds it, and a turn that ended as it went takes it as the next", async () => {
  const h = harness();
  let answer!: () => void;
  h.answerSend(() => new Promise((r) => { answer = r; }));
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  const sent = h.store.send("first");
  await tick();
  h.answerSteer(async () => "between-turns");
  assert.equal(await h.store.send("second"), "held");
  // The turn ends just as the next message goes: it is sent as a turn.
  h.answerSteer(async () => { answer(); await sent; return "no-turn"; });
  h.answerSend(async () => undefined);
  assert.equal(await h.store.send("third"), "sent");
  assert.deepEqual(h.calls, ["send first", "steer second", "steer third", "send third"]);
});

test("a message In Agentry still waits for the answer", async () => {
  const h = harness();
  h.store.followHosted({ registry: "r", run: hostedRun("h1"), terms: null, opening: "", task: "" }, "setup", false);
  h.fire("runner:hosted-session-event", { kind: "user_turn", time: new Date().toISOString(), text: "first" });
  assert.equal(working(h.store.state().chat!), true);
  assert.equal(await h.store.send("second"), "ignored");
  assert.deepEqual(h.calls.filter((c) => c.startsWith("hsend") || c.startsWith("steer")), []);
});

test("Stop turn stops the turn in flight on this computer, and the chat goes on", async () => {
  const h = harness();
  h.answerSend(() => new Promise(() => {}));
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  void h.store.send("first");
  await tick();
  await h.store.stopTurn();
  assert.deepEqual(h.calls, ["send first", "interrupt"]);
  const r = h.store.state().chat!;
  assert.equal(r.stopping, false);
  assert.equal(r.over, false);
  assert.equal(r.error, null);
  // A turn that would not stop says so, and the chat stays.
  h.answerInterrupt(async () => { throw new Error("the turn did not stop: timeout"); });
  await h.store.stopTurn();
  assert.match(h.store.state().chat!.error ?? "", /did not stop/);
  assert.equal(h.store.state().chat!.over, false);
});

test("Finish on this computer brings its envelope back; Dismiss then forgets it", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  h.store.dismiss();
  assert.ok(h.store.state().chat, "dismissed while it was live");
  await h.store.end(true);
  assert.equal(h.store.state().chat?.over, true);
  h.store.dismiss();
  assert.deepEqual(h.forgotten, ["c1"]);
  assert.equal(h.store.state().chat, null);
});

test("a chat In Agentry is followed by its run's id: its frames once each, its state, and its last word", async () => {
  const h = harness();
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h1"), terms: null, opening: "Hi there.", task: "" }, "setup", false);
  assert.deepEqual(h.calls, ["replay"], "the host is asked for the frames it holds");
  const turn = { seq: 1, kind: "user_turn", text: "Summarise the thread.", time: new Date().toISOString() };
  h.fire("runner:hosted-session-event", turn);
  h.fire("runner:hosted-session-event", turn); // a replay gives it again
  h.fire("runner:hosted-session-state", { status: "running", startedAt: "2026-09-30T12:00:00Z" });
  let s = h.store.state();
  assert.equal(s.chat?.chat.turns, 1);
  assert.equal(working(s.chat!), true, "a message the agent has not answered");
  h.fire("runner:hosted-run", hostedRun("other", { final: true }));
  assert.equal(h.store.state().chat?.over, false, "another run's last word");
  h.fire("runner:hosted-run", hostedRun("h1", { status: "finished", final: true, outcome: "succeeded" }));
  s = h.store.state();
  assert.equal(s.chat?.over, true);
  h.store.dismiss();
  assert.deepEqual(h.forgotten, ["h1"]);
});

test("following the chat In Agentry already followed opens it rather than following it twice", () => {
  const h = harness();
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h1"), terms: null, opening: "", task: "" }, null, true);
  assert.equal(h.store.state().asCard, true);
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h1"), terms: null, opening: "", task: "" }, null, false);
  assert.deepEqual(h.calls, ["replay"]);
  assert.equal(h.store.state().asCard, false);
});

test("a chat's answer that comes back after it was dismissed and another began changes nothing", async () => {
  const h = harness();
  let answer!: () => void;
  h.store.startLocal("setup A", "acme/helper@1.0.0", "", "", () => new Promise<void>((r) => { answer = r; }).then(() => { throw "refused"; }));
  await tick();
  // It never answered; the person cannot dismiss a live chat, but a new
  // generation begins once it failed.
  answer();
  await tick();
  h.store.dismiss();
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h2"), terms: null, opening: "", task: "" }, null, false);
  assert.equal(h.store.state().chat?.kind, "hosted");
  assert.equal(h.store.state().chat?.error, null);
});

test("an end Go refuses on this computer leaves the chat over, so it can be dismissed and another started", async () => {
  const h = harness();
  h.answerEnd(async () => { throw "no session is open"; });
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  await h.store.end(true);
  const s = h.store.state();
  assert.equal(s.chat?.over, true);
  assert.match(s.chat?.error ?? "", /no session/);
  h.store.dismiss();
  assert.equal(h.store.state().chat, null);
});

test("the QuickAgent is working on the first turn the engine sends for a task, not only on the person's messages", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "summarise the thread", "", async () => {});
  await tick();
  h.fire("runner:session-event", { kind: "user_turn", text: "summarise the thread", time: new Date().toISOString() });
  assert.equal(working(h.store.state().chat!), true);
  h.fire("runner:session-event", { kind: "assistant_turn", text: "Done.", time: new Date().toISOString() });
  assert.equal(working(h.store.state().chat!), false);
});

test("a chat In Agentry found while another chat is live waits, and is followed once that one is dismissed", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  assert.equal(h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h9"), terms: null, opening: "", task: "" }, null, true), false);
  assert.equal(h.store.state().chat?.kind, "local");
  await h.store.end(true);
  h.store.dismiss();
  const s = h.store.state();
  assert.equal(s.chat?.kind, "hosted");
  assert.equal(s.chat?.run?.id, "h9");
  assert.equal(s.asCard, true);
});

test("a waiting chat In Agentry that ends before it is shown is followed already over, with its result", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h9"), terms: null, opening: "", task: "" }, null, true);
  // Go follows it meanwhile, and its last word arrives while the local
  // chat is still the one shown.
  h.fire("runner:hosted-run", hostedRun("h9", { status: "finished", final: true, outcome: "succeeded" }));
  await h.store.end(true);
  h.store.dismiss();
  const s = h.store.state();
  assert.equal(s.chat?.run?.id, "h9");
  assert.equal(s.chat?.over, true);
  assert.ok(!h.calls.includes("replay"), "a chat already over asked the host to replay");
  h.store.dismiss();
  assert.equal(h.store.state().chat, null);
});

test("a turn that fails ends the agent's turn: the chat is waiting for the person again", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "summarise", "", async () => {});
  await tick();
  h.fire("runner:session-event", { kind: "user_turn", text: "summarise", time: new Date().toISOString() });
  h.fire("runner:session-event", { kind: "error", text: "HTTP 401 from the model", time: new Date().toISOString() });
  assert.equal(working(h.store.state().chat!), false);
});

test("a waiting chat In Agentry that has ended keeps its last word when a later attach finds it again", async () => {
  const h = harness();
  h.store.startLocal("setup", "acme/helper@1.0.0", "go", "", async () => {});
  await tick();
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h9"), terms: null, opening: "", task: "" }, null, true);
  h.fire("runner:hosted-run", hostedRun("h9", { status: "finished", final: true }));
  h.store.followHosted({ registry: "https://dev.example.test", run: hostedRun("h9"), terms: null, opening: "", task: "" }, null, true);
  await h.store.end(true);
  h.store.dismiss();
  assert.equal(h.store.state().chat?.over, true);
});

test("a message sent during a turn is drawn where it was sent, counted, and marked joined once the harness took it", () => {
  const t = new Date().toISOString();
  let chat = newChat("", "go");
  chat = applyEvent(chat, { kind: "user_turn", time: t, text: "Summarise INC-4821." }, Date.now());
  chat = applyEvent(chat, { kind: "assistant_text", time: t, text: "INC-4821 began" }, Date.now());
  chat = applyEvent(chat, { kind: "steer", time: t, text: "Use the UTC times only." }, Date.now());
  chat = applyEvent(chat, { kind: "steer", time: t, text: "input added: /inputs/pager.csv", synthetic: true }, Date.now());
  assert.equal(chat.turns, 2, "the person's message counts, the runner's notice does not");
  assert.equal(chat.busy, true, "the turn goes on");
  assert.deepEqual(chat.lines.slice(1).map((l) => l.kind === "user" ? [l.text, l.synthetic, l.during] : l.kind), [
    ["Use the UTC times only.", false, "waiting"],
    ["input added: /inputs/pager.csv", true, "waiting"],
  ]);
  chat = applyEvent(chat, { kind: "steered", time: t }, Date.now());
  assert.deepEqual(chat.lines.filter((l) => l.kind === "user").map((l) => l.kind === "user" && l.during), [undefined, "joined", "joined"]);
  // A steered event with nothing waiting changes no line.
  const again = applyEvent(chat, { kind: "steered", time: t }, Date.now());
  assert.equal(again.lines, chat.lines);
});
