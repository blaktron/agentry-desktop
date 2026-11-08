// Browse pages through every result: it shows how many there are, offers
// Show more while the registry has more, and reaches the last one.
import { window } from "./dom.ts";
import { afterEach, test } from "node:test";
import assert from "node:assert/strict";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const g = globalThis as any;
g.IS_REACT_ACT_ENVIRONMENT = true;

const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { default: Browse, BROWSE_PAGE, hasMore } = await import("../src/pages/Browse.tsx");

let root: ReturnType<typeof createRoot> | null = null;
let el: HTMLElement | null = null;
afterEach(async () => {
  if (root) await React.act(async () => root?.unmount());
  el?.remove();
  root = null;
  el = null;
  delete g.go;
});

const text = (node: Element | null) => (node?.textContent ?? "").replace(/\s+/g, " ").trim();
const rows = (node: Element) => Array.from(node.querySelectorAll("[role=button], .agent-row, button")).filter((b) => /^acme\/agent-\d+/.test(text(b)));

/** A registry of n results, answering each page; total is sent unless told otherwise. */
function mockRegistry(n: number, sendTotal = true) {
  const asked: number[] = [];
  g.go = {
    app: {
      BrowseAPI: {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        Search: (_reg: string, q: any) => {
          asked.push(q.offset ?? 0);
          const from = q.offset ?? 0;
          const results = Array.from({ length: Math.max(0, Math.min(q.limit, n - from)) }, (_, i) => ({
            namespace: "acme", name: `agent-${from + i}`, modes: ["autonomous"], summary: "s", installs30d: 0, quarantine: "none",
          }));
          return sendTotal ? { total: n, results } : { results };
        },
      },
    },
  };
  return asked;
}

async function render() {
  el = window.document.createElement("div");
  window.document.body.appendChild(el);
  root = createRoot(el);
  await React.act(async () => { root!.render(React.createElement(Browse, { registry: "https://r.example", onOpenAgent: () => {} })); });
  await React.act(async () => {});
  return el;
}

async function showMore(node: Element) {
  const b = Array.from(node.querySelectorAll("button")).find((x) => text(x) === "Show more");
  assert.ok(b, "no Show more button");
  await React.act(async () => { b.dispatchEvent(new window.MouseEvent("click", { bubbles: true })); });
  await React.act(async () => {});
}

const hasShowMore = (node: Element) => Array.from(node.querySelectorAll("button")).some((x) => text(x) === "Show more");

test("Browse pages through every result and says how many there are", async () => {
  const asked = mockRegistry(65);
  const node = await render();
  assert.equal(rows(node).length, BROWSE_PAGE);
  assert.match(text(node), /Showing 30 of 65 results/);
  await showMore(node);
  assert.equal(rows(node).length, 60);
  await showMore(node);
  assert.equal(rows(node).length, 65);
  assert.match(text(node), /65 results/);
  assert.ok(!hasShowMore(node), "Show more after the last result");
  assert.deepEqual(asked, [0, 30, 60]);
  assert.match(text(node), /acme\/agent-64/);
});

test("a registry that sends no total is paged until a short page", async () => {
  mockRegistry(45, false);
  const node = await render();
  assert.equal(rows(node).length, 30);
  await showMore(node);
  assert.equal(rows(node).length, 45);
  assert.ok(!hasShowMore(node));
});

test("hasMore reads the total when there is one, else a full last page", () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const view = (n: number, total?: number) => ({ results: Array.from({ length: n }), total }) as any;
  assert.equal(hasMore(view(30, 31), 30), true);
  assert.equal(hasMore(view(31, 31), 1), false);
  assert.equal(hasMore(view(30), 30), true);
  assert.equal(hasMore(view(40), 10), false);
  assert.equal(hasMore(null, 30), false);
});
