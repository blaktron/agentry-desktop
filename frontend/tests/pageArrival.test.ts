// A page change moves the focus to the new page's heading and names the
// window after it; the first page moves nothing, a page that focused one of
// its own controls keeps it, and a page without a heading takes the focus
// on the content area.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { pageTitle } from "../src/pageArrival.ts";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { usePageArrival } = await import("../src/pageArrival.ts");

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
});

const frame = () => new Promise((r) => setTimeout(r, 30));

function Shell({ page }: { page: string }) {
  const content = React.useRef<HTMLElement>(null);
  usePageArrival(page, content);
  return React.createElement(React.Fragment, null,
    React.createElement("nav", null, React.createElement("button", { id: "nav" }, "Library")),
    React.createElement("main", { ref: content, className: "content" },
      page === "plain" ? React.createElement("p", null, "No heading")
        : page === "search" ? React.createElement("div", null, React.createElement("h1", null, "Browse"), React.createElement("input", { id: "q", autoFocus: true, "aria-label": "Search" }))
        : React.createElement("h1", null, page)));
}

async function render(page: string) {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  await React.act(async () => { root!.render(React.createElement(Shell, { page })); });
  await React.act(frame);
  return el;
}
async function go(page: string) {
  await React.act(async () => { root!.render(React.createElement(Shell, { page })); });
  await React.act(frame);
}

test("the window is named after the page's heading", () => {
  assert.equal(pageTitle("Library"), "Library — Agentry Desktop");
  assert.equal(pageTitle("  Settings\n "), "Settings — Agentry Desktop");
  assert.equal(pageTitle(""), "Agentry Desktop");
  assert.equal(pageTitle(null), "Agentry Desktop");
});

test("the first page names the window and leaves the focus; the next takes it on its heading", async () => {
  const node = await render("Home");
  const nav = node.querySelector<HTMLButtonElement>("#nav")!;
  nav.focus();
  assert.equal(window.document.title, "Home — Agentry Desktop");
  assert.equal(window.document.activeElement, nav, "the first page moved the focus");

  await go("Library");
  const h1 = node.querySelector("h1")!;
  assert.equal(window.document.activeElement, h1);
  assert.equal(h1.getAttribute("tabindex"), "-1");
  assert.equal(window.document.title, "Library — Agentry Desktop");
});

test("a page that focused its own control keeps it, and one without a heading takes the focus on the content", async () => {
  const node = await render("Home");
  await go("search");
  assert.equal(window.document.activeElement, node.querySelector("#q"));
  await go("plain");
  const main = node.querySelector("main")!;
  assert.equal(window.document.activeElement, main);
  assert.equal(main.getAttribute("tabindex"), "-1");
});

test("an open dialog keeps the focus when the page behind it changes", async () => {
  await render("Home");
  const dialog = window.document.createElement("div");
  dialog.setAttribute("role", "dialog");
  const ok = window.document.createElement("button");
  dialog.appendChild(ok);
  window.document.body.appendChild(dialog);
  ok.focus();
  await go("Library");
  assert.equal(window.document.activeElement, ok);
  dialog.remove();
});
