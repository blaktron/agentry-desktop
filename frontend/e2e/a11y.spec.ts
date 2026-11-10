// Every harness state passes axe (WCAG 2.2 A and AA, colour contrast and
// target size included) in the light and the dark palette, and the pages
// pass in every theme Settings → Home offers.
import { expect, test } from "@playwright/test";
import { axeViolations, report } from "./axe";
import { STATES, THEMES } from "./states";

for (const scheme of ["light", "dark"] as const) {
  test.describe(`${scheme} palette`, () => {
    test.use({ colorScheme: scheme });
    for (const url of STATES) {
      test(`${url} has no accessibility violations`, async ({ page }) => {
        const vs = await axeViolations(page, url);
        expect(vs, report(vs)).toEqual([]);
      });
    }
    for (const theme of THEMES) {
      test(`the pages in the ${theme} theme have no accessibility violations`, async ({ page }) => {
        const vs = await axeViolations(page, `/pages.html?theme=${theme}`);
        expect(vs, report(vs)).toEqual([]);
      });
    }
  });
}
