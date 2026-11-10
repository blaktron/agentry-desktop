// The modal dialog and the clickable row, driven from the keyboard: the
// dialog takes the focus and is named by its title, Tab and Shift+Tab stay
// inside it, Escape closes it (but first closes a menu open inside it, and
// never a dialog that cannot be closed), and the focus goes back to where it
// was; a group of radio buttons is one stop; a clickable row opens through
// a real button.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;

const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { AgentRow, Modal, OptionsMenu, useDialog } = await import("../src/primitives.tsx");

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

async function key(target: Element, k: string, shiftKey = false) {
  const e = new window.KeyboardEvent("keydown", { key: k, shiftKey, bubbles: true, cancelable: true });
  await React.act(async () => { target.dispatchEvent(e); });
  return e;
}

const focused = () => window.document.activeElement;

function WithOpener({ onClose, menu = false }: { onClose: () => void; menu?: boolean }) {
  const [open, setOpen] = React.useState(false);
  return React.createElement(React.Fragment, null,
    React.createElement("button", { id: "opener", onClick: () => setOpen(true) }, "Open"),
    open && React.createElement(Modal, { title: "Remove acme/agent?", onClose: () => { onClose(); setOpen(false); } },
      menu && React.createElement(OptionsMenu, { items: [{ label: "Copy", onSelect: () => {} }] }),
      React.createElement("button", { id: "first" }, "Cancel"),
      React.createElement("button", { id: "last" }, "Remove"),
    ),
  );
}

test("a modal takes the focus, is named by its title, keeps Tab inside, closes on Escape and gives the focus back", async () => {
  let closed = 0;
  const node = await render(React.createElement(WithOpener, { onClose: () => { closed++; } }));
  const opener = node.querySelector<HTMLButtonElement>("#opener")!;
  opener.focus();
  await React.act(async () => { opener.click(); });

  const dialog = node.querySelector('[role="dialog"]')!;
  assert.equal(dialog.getAttribute("aria-modal"), "true");
  assert.equal(window.document.getElementById(dialog.getAttribute("aria-labelledby")!)?.textContent, "Remove acme/agent?");
  assert.equal(focused(), dialog, "the dialog did not take the focus");

  const first = node.querySelector<HTMLButtonElement>("#first")!;
  const last = node.querySelector<HTMLButtonElement>("#last")!;
  last.focus();
  assert.ok((await key(last, "Tab")).defaultPrevented);
  assert.equal(focused(), first, "Tab from the last control did not wrap to the first");
  await key(first, "Tab", true);
  assert.equal(focused(), last, "Shift+Tab from the first control did not wrap to the last");

  await key(last, "Escape");
  assert.equal(closed, 1);
  assert.equal(node.querySelector('[role="dialog"]'), null);
  assert.equal(focused(), opener, "the focus did not return to the opener");
});

test("Escape closes a menu open inside a modal before the modal", async () => {
  let closed = 0;
  const node = await render(React.createElement(WithOpener, { onClose: () => { closed++; }, menu: true }));
  await React.act(async () => { node.querySelector<HTMLButtonElement>("#opener")!.click(); });
  const menuButton = node.querySelector<HTMLButtonElement>("[data-options-menu]")!;
  await React.act(async () => { menuButton.click(); });
  const item = node.querySelector('[role="menuitem"]')!;
  assert.ok(item, "the menu did not open");

  await key(item, "Escape");
  assert.equal(node.querySelector('[role="menuitem"]'), null, "the menu stayed open");
  assert.equal(closed, 0, "the modal closed with the menu");
  assert.ok(node.querySelector('[role="dialog"]'));
});

test("a dialog with nothing to close it keeps Escape to itself and still keeps Tab inside", async () => {
  let outside = 0;
  function Setup() {
    const dialog = useDialog<HTMLDivElement>();
    return React.createElement("div", { onKeyDown: () => { outside++; } },
      React.createElement("div", { ...dialog, role: "dialog", "aria-modal": "true", "aria-label": "Setup" },
        React.createElement("button", { id: "only" }, "Next")));
  }
  const node = await render(React.createElement(Setup));
  const only = node.querySelector<HTMLButtonElement>("#only")!;
  await key(node.querySelector('[role="dialog"]')!, "Escape");
  assert.equal(outside, 0, "Escape went past the dialog");
  assert.ok(node.querySelector('[role="dialog"]'));
  only.focus();
  await key(only, "Tab");
  assert.equal(focused(), only);
});

test("Tab in a dialog counts a group of radio buttons as one stop, the checked one", async () => {
  function Choice() {
    const dialog = useDialog<HTMLDivElement>(() => {});
    return React.createElement("div", { ...dialog, role: "dialog", "aria-modal": "true", "aria-label": "Install as" },
      React.createElement("input", { type: "radio", name: "target", id: "a", defaultChecked: false }),
      React.createElement("input", { type: "radio", name: "target", id: "b", defaultChecked: true }),
      React.createElement("button", { id: "next" }, "Next"));
  }
  const node = await render(React.createElement(Choice));
  const checked = node.querySelector<HTMLInputElement>("#b")!;
  const next = node.querySelector<HTMLButtonElement>("#next")!;
  next.focus();
  await key(next, "Tab");
  assert.equal(focused(), checked, "Tab from the last stop did not wrap to the checked radio button");
  await key(checked, "Tab", true);
  assert.equal(focused(), next, "Shift+Tab from the checked radio button did not wrap to the last stop");
});

test("a clickable row opens through its title, a button, and its actions stay their own", async () => {
  const opened: string[] = [];
  const node = await render(React.createElement(AgentRow, {
    title: "acme/agent",
    summary: "Summarises an incident.",
    onClick: () => opened.push("row"),
    actions: React.createElement("button", { onClick: () => opened.push("action") }, "Run"),
  }));
  const open = node.querySelector<HTMLButtonElement>("button.agent-row-open");
  assert.ok(open, "the row's title is not a button");
  assert.equal(open.textContent, "acme/agent");
  assert.equal(open.type, "button");
  await React.act(async () => { open.click(); });
  const action = Array.from(node.querySelectorAll("button")).find((b) => b.textContent === "Run")!;
  await React.act(async () => { action.click(); });
  assert.deepEqual(opened, ["row", "action"]);
});

test("a row that opens nothing has no button", async () => {
  const node = await render(React.createElement(AgentRow, { title: "acme/agent" }));
  assert.equal(node.querySelector("button"), null);
  assert.equal(node.querySelector(".agent-row-title")?.textContent, "acme/agent");
});
