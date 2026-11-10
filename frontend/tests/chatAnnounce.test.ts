// A conversation is heard one complete reply at a time: what was there when
// it opened is not read, the person's lines, tool calls and notes are not
// said, and each new reply or warning is said once.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { newAnnouncement, sayTermBlock } from "../src/chatAnnounce.tsx";
import type { TermBlock } from "../src/persistentAgents.ts";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { ChatAnnouncer } = await import("../src/chatAnnounce.tsx");

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
});

const say = sayTermBlock("Assistant");

test("only an agent's answers and the warnings are said, by the agent's name", () => {
  const blocks: TermBlock[] = [
    { kind: "person", text: "hello" },
    { kind: "tools", names: ["read_file"] },
    { kind: "note", text: "compacted" },
    { kind: "agent", text: "Hi there." },
    { kind: "warn", text: "The model refused the key." },
  ];
  assert.equal(newAnnouncement(0, blocks, say), "Assistant: Hi there. The model refused the key.");
  assert.equal(newAnnouncement(4, blocks, say), "The model refused the key.");
  assert.equal(newAnnouncement(5, blocks, say), "");
});

test("the announcer says nothing of the opening conversation and each new answer once", async () => {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  const draw = async (items: TermBlock[]) => { await React.act(async () => { root!.render(React.createElement(ChatAnnouncer<TermBlock>, { items, say })); }); };
  const said = () => el!.querySelector("[data-chat-said]")!;

  const history: TermBlock[] = [{ kind: "person", text: "earlier" }, { kind: "agent", text: "An earlier answer." }];
  await draw(history);
  assert.equal(said().getAttribute("role"), "status");
  assert.equal(said().textContent, "", "the opening conversation was read");

  const asked = [...history, { kind: "person" as const, text: "What is new?" }, { kind: "tools" as const, names: ["search"] }];
  await draw(asked);
  assert.equal(said().textContent, "", "the person's own line or a tool call was said");

  await draw([...asked, { kind: "agent", text: "Two things." }]);
  assert.equal(said().textContent, "Assistant: Two things.");
});
