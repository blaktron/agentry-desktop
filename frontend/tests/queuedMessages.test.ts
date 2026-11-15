// A message sent while an agent is on a turn waits for its next one: the
// queue's rules, and a persistent agent's store keeping its queue against
// mocked bound calls and the agent's events.
import { test } from "node:test";
import assert from "node:assert/strict";
import { editText, personLines, queueJoin } from "../src/queued.ts";
import { emit } from "./wails-mock.ts";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;
const tick = () => new Promise((r) => setTimeout(r, 0));
const t = "2026-11-15T10:00:00Z";

test("one message waits at a time, each part on its own line", () => {
  assert.equal(queueJoin(null, "  hi "), "hi");
  assert.equal(queueJoin("first", "second"), "first\nsecond");
  assert.equal(queueJoin("first", "  "), "first");
  assert.equal(queueJoin(null, null), null);
});

test("Edit puts the queued text before what was typed since, on one line in a one-line field", () => {
  assert.equal(editText("use UTC\nand be brief", "", true), "use UTC\nand be brief");
  assert.equal(editText("use UTC\nand be brief", "also", false), "use UTC and be brief also");
  assert.equal(editText("use UTC", "  ", false), "use UTC");
});

test("the person's lines are counted apart from the runner's, a task's and upkeep's", () => {
  assert.equal(personLines([
    { kind: "user", text: "a", synthetic: false },
    { kind: "user", text: "input added", synthetic: true },
    { kind: "user", text: "Task: x", synthetic: false, task: "t1" },
    { kind: "user", text: "Upkeep", synthetic: false, upkeep: true },
    { kind: "assistant", text: "b" },
  ]), 1);
});

test("a persistent agent queues a message during a turn, and Send now, Edit and Add to this turn act on it", async () => {
  const calls: string[] = [];
  let answerSend: () => void = () => {};
  let steerAnswer = "sent";
  g.go = {
    app: {
      PersistentAPI: {
        List: async () => [{ name: "sandra", state: "running", roster: [], volumes: { session: "", data: "" } }],
        Transcript: async () => [],
        Send: (_name: string, text: string) => {
          calls.push(`send ${text}`);
          emit("runner:agent-event", { agent: "sandra", event: { kind: "user_turn", time: t, text } });
          return new Promise<void>((r) => {
            answerSend = () => {
              emit("runner:agent-event", { agent: "sandra", event: { kind: "assistant_turn", time: t, text: "ok" } });
              r();
            };
          });
        },
        Steer: async (_name: string, text: string) => { calls.push(`steer ${text}`); return steerAnswer; },
        Interrupt: async () => { calls.push("interrupt"); answerSend(); },
      },
    },
  };
  const agents = await import("../src/agents.ts");
  await agents.refreshAgents();
  await agents.loadChat("sandra");
  const rec = () => agents.agentRecord("sandra");

  // The person's first message is a turn; the next waits for it.
  const first = agents.sendToAgent("sandra", "Summarise the files");
  await tick();
  assert.equal(await agents.sendToAgent("sandra", "use UTC"), "queued");
  assert.deepEqual(calls, ["send Summarise the files"]);
  assert.equal(rec().queued, "use UTC");

  // Edit takes it back; nothing is sent.
  assert.equal(agents.takeQueued("sandra"), "use UTC");

  // Add to this turn hands it to the turn in flight.
  await agents.sendToAgent("sandra", "and be brief");
  await agents.addQueuedToTurn("sandra");
  assert.deepEqual(calls, ["send Summarise the files", "steer and be brief"]);

  // A turn that takes no message keeps it queued, and says so.
  await agents.sendToAgent("sandra", "then list them");
  steerAnswer = "between-turns";
  await agents.addQueuedToTurn("sandra");
  assert.equal(calls.at(-1), "steer then list them");
  assert.equal(rec().queued, "then list them");
  assert.match(rec().queueNote ?? "", /takes no message/);

  // Send now stops the turn, and the queued message is the next turn.
  assert.equal(await agents.sendQueuedNow("sandra"), null);
  await first;
  await tick();
  await tick();
  assert.deepEqual(calls.slice(-2), ["interrupt", "send then list them"]);
  assert.equal(agents.takeQueued("sandra"), null, "nothing waits any more");

  // When the turn ends by itself, a queued message goes as the next turn.
  await agents.sendToAgent("sandra", "thanks");
  assert.deepEqual(calls.slice(-1), ["send then list them"]);
  answerSend();
  await tick();
  await tick();
  assert.deepEqual(calls.slice(-1), ["send thanks"]);
  answerSend();
  delete g.go;
});
