// axe-core in the page, against WCAG 2.2 A and AA with colour contrast and
// target size, as a browser draws it.
import type { Page } from "@playwright/test";
import { createRequire } from "node:module";

const axePath = createRequire(import.meta.url).resolve("axe-core/axe.min.js");

export type Violation = { id: string; help: string; nodes: { target: string[]; failureSummary?: string }[] };

/** Opens a harness state, waits for its first reads, and answers what axe
 *  finds wrong with the whole page. */
export async function axeViolations(page: Page, url: string): Promise<Violation[]> {
  await page.goto(url);
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(500);
  await page.addScriptTag({ path: axePath });
  return page.evaluate(async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const axe = (window as any).axe;
    const r = await axe.run(document, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"] },
    });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return r.violations.map((v: any) => ({
      id: v.id, help: v.help,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      nodes: v.nodes.slice(0, 5).map((n: any) => ({ target: n.target, failureSummary: n.failureSummary })),
    }));
  });
}

/** The violations as lines a failed assertion can show. */
export const report = (vs: Violation[]): string =>
  "\n" + vs.map((v) => `${v.id}: ${v.help}\n${v.nodes.map((n) => `    ${n.target.join(" ")}\n      ${(n.failureSummary ?? "").split("\n").slice(1, 2).join("")}`).join("\n")}`).join("\n");
