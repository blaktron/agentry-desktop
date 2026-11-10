// The accessibility check's pieces: a design harness entry rendered in this
// document with the harness's own mocks, and axe-core run over it against
// WCAG 2.2 A and AA and axe's best practices. Colour contrast is left to the
// browser shots, since this document lays nothing out. Import it first, as
// tests/dom.ts.
import { window } from "./dom.ts";
import { after } from "node:test";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";

export { window };

// A harness entry starts timers (progress that moves, polls); they are
// cleared once a file's checks are done, so the run can end.
const timers = new Set<ReturnType<typeof setTimeout>>();
for (const name of ["setTimeout", "setInterval"] as const) {
  const real = globalThis[name] as (...a: unknown[]) => ReturnType<typeof setTimeout>;
  (globalThis as Record<string, unknown>)[name] = (...a: unknown[]) => { const t = real(...a); timers.add(t); return t; };
}
after(() => {
  for (const t of timers) { clearTimeout(t); clearInterval(t); }
  window.close();
});

// The harness sets its mocks on window; the bindings read globalThis.
const w = window as unknown as Record<string, unknown>;
Object.defineProperty(globalThis, "go", { get: () => w.go, configurable: true });
Object.defineProperty(globalThis, "runtime", { get: () => w.runtime, configurable: true });

/** Renders a harness entry ("pages", "modals", "wizard") in the state its
 *  hash names, and waits for its first reads to be answered. */
export async function renderHarness(entry: string, hash = ""): Promise<Element> {
  const root = window.document.createElement("div");
  root.id = "root";
  window.document.body.appendChild(root);
  if (hash) window.location.hash = hash;
  await import(`../harness/${entry}.tsx`);
  for (let i = 0; i < 50; i++) await new Promise((r) => setTimeout(r, 0));
  return root;
}

window.eval(readFileSync(createRequire(import.meta.url).resolve("axe-core/axe.min.js"), "utf8"));
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const axe = (window as any).axe;

export type Violation = { id: string; help: string; nodes: { target: string[]; html: string }[] };

/** What axe finds wrong under `context`. A page fragment is checked as a
 *  fragment: the rules about a whole page's landmarks and its one h1 are
 *  left out. */
export async function violations(context: Element): Promise<Violation[]> {
  const result = await axe.run(context, {
    runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa", "best-practice"] },
    rules: {
      "color-contrast": { enabled: false },
      "region": { enabled: false },
      "landmark-one-main": { enabled: false },
      "page-has-heading-one": { enabled: false },
      "landmark-unique": { enabled: false },
      "landmark-no-duplicate-banner": { enabled: false },
      "landmark-no-duplicate-contentinfo": { enabled: false },
      "landmark-no-duplicate-main": { enabled: false },
    },
  });
  return result.violations;
}

/** The violations as lines a failed assertion can show. */
export function report(vs: Violation[]): string {
  return "\n" + vs.map((v) => `${v.id}: ${v.help}\n${v.nodes.slice(0, 5).map((n) => `    ${n.target.join(" ")}  ${n.html.slice(0, 160)}`).join("\n")}`).join("\n");
}
