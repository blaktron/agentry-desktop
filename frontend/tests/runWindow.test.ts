// The run window every job is drawn through, rendered: while a job runs it
// is a dialog with its progress, the Run log folded, its Stop and Run in the
// background, which shrinks it to a card with the compact progress, the
// card's Stop and Open; once the job is over the dialog is its outcome with
// the Run log beneath, open when the job did not succeed, its primary action
// and Dismiss. The log shows invisible characters.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;
g.IS_REACT_ACT_ENVIRONMENT = true;

const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { RunWindow, RUN_LOG, useFollowed } = await import("../src/pages/RunWindow.tsx");

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
});

async function render(node: React.ReactElement) {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  await React.act(async () => { root!.render(node); });
  return el;
}
const text = (node: Element | null) => (node?.textContent ?? "").replace(/\s+/g, " ").trim();
const buttons = (node: Element) => Array.from(node.querySelectorAll("button")).map((b) => text(b));
const click = async (node: Element, label: RegExp) => {
  const b = Array.from(node.querySelectorAll("button")).find((b) => label.test(text(b)));
  assert.ok(b, `no button ${label}`);
  await React.act(async () => { b.dispatchEvent(new window.MouseEvent("click", { bubbles: true })); });
};

type Props = Parameters<typeof RunWindow>[0];

// Job is a store-driven job as a flow draws it: useFollowed decides the
// dialog or the card from the asks to open it.
function Job(p: Omit<Props, "minimized" | "minimize"> & { raised: number }) {
  const followed = useFollowed({ raised: p.raised });
  return React.createElement(RunWindow, { ...p, ...followed });
}

const acted: string[] = [];
const base = {
  raised: 1,
  label: "The Voice Writer on this computer",
  title: "Edit with AI",
  cardTitle: "The Voice Writer is running",
  spinner: "The Voice Writer is running on this computer…",
  about: "It reads your voice.",
  background: "In the background, it goes on in a card.",
  progress: (compact: boolean) => React.createElement("p", null, compact ? "compact progress" : "full progress"),
  stop: { label: "Stop the Voice Writer", cardLabel: "Stop", stopping: false, onStop: () => acted.push("stop") },
  log: ["reading the samples", "page‮txt.exe"],
  onOpen: () => acted.push("open"),
  onDismiss: () => acted.push("dismiss"),
};

test("a running job is a dialog with its progress, the Run log, Stop and Run in the background; the background is its card", async () => {
  acted.length = 0;
  const node = await render(React.createElement(Job, { ...base, running: true }));
  assert.ok(node.querySelector(".modal"), "the dialog is up");
  assert.match(text(node), /The Voice Writer is running on this computer…/);
  assert.match(text(node), /full progress/);
  const log = node.querySelector("details.knowledge-log");
  assert.ok(log, "the log is folded");
  assert.equal(text(log!.querySelector("summary")), RUN_LOG);
  assert.equal(log!.hasAttribute("open"), false);
  assert.match(text(log), /U\+202E/, "an invisible character is shown");
  assert.deepEqual(buttons(node), ["Stop the Voice Writer", "Run in the background"]);
  await click(node, /^Stop the Voice Writer$/);
  assert.deepEqual(acted, ["stop"]);

  await click(node, /^Run in the background$/);
  assert.equal(node.querySelector(".modal"), null, "the dialog is gone");
  const card = node.querySelector("aside.build-card");
  assert.ok(card, "the card is up");
  assert.match(text(card), /The Voice Writer is running/);
  assert.match(text(card), /compact progress/);
  assert.deepEqual(buttons(card!), ["Stop", "Open"]);
  await click(card!, /^Open$/);
  assert.deepEqual(acted, ["stop", "open"]);
});

test("a job that succeeded shows its outcome, the Run log folded, its primary action and Dismiss", async () => {
  acted.length = 0;
  const node = await render(React.createElement(Job, {
    ...base, running: false, line: "The Voice Writer kept your new voice on this computer.", tone: "ok",
    outcome: React.createElement("h3", null, "Kept it."),
    primary: { label: "Open your voice", onClick: () => acted.push("voice") },
  }));
  assert.match(text(node), /Kept it\./);
  assert.equal(node.querySelector("details.knowledge-log")?.hasAttribute("open"), false);
  assert.deepEqual(buttons(node), ["Dismiss", "Open your voice"]);
  await click(node, /^Open your voice$/);
  await click(node, /^Dismiss$/);
  assert.deepEqual(acted, ["voice", "dismiss"]);
});

test("a job that failed says how it ended and why, with the Run log open", async () => {
  const node = await render(React.createElement(Job, {
    ...base, running: false, line: "The Voice Writer did not finish.", tone: "bad", error: "the model answered 503",
  }));
  assert.match(text(node.querySelector("h3.run-outcome-bad")), /did not finish/);
  assert.match(text(node), /model answered 503/);
  assert.equal(node.querySelector("details.knowledge-log")?.hasAttribute("open"), true);
  assert.deepEqual(buttons(node).filter((b) => b !== "Try again"), ["Dismiss"]);
});

test("an over job's card says how it ended, with its primary action in place of Open", async () => {
  const node = await render(React.createElement(RunWindow, {
    ...base, running: false, minimized: true, minimize: () => {}, line: "Made it.", tone: "ok",
    cardActions: React.createElement("button", null, "Preview"),
    primary: { label: "Open the draft", onClick: () => {} },
  }));
  const card = node.querySelector("aside.build-card")!;
  assert.match(text(card), /Made it\./);
  assert.equal(card.querySelector(".build-card-dot"), null, "no running dot once it is over");
  assert.deepEqual(buttons(card), ["Preview", "Dismiss", "Open the draft"]);
});

test("a job In Agentry keeps no log here and is cancelled, not stopped", async () => {
  const node = await render(React.createElement(Job, {
    ...base, running: true, log: null, progress: undefined,
    stop: { label: "Cancel run", stopping: false, onStop: () => {} },
  }));
  assert.equal(node.querySelector("details.knowledge-log"), null);
  assert.deepEqual(buttons(node), ["Cancel run", "Run in the background"]);
});
