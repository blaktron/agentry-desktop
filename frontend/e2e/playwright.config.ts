// The browser checks over the design harness: an automated accessibility
// check with colour contrast in light and dark and in every theme, the same
// under Windows High Contrast (forced colours), and keyboard-only runs of the
// main flows. The harness renders the real pages against mocks, so no Go
// host is needed. scripts/acceptance/browser-a11y.sh runs them in
// Playwright's own image.
import { fileURLToPath } from "node:url";
import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.HARNESS_PORT ?? 5199);

export default defineConfig({
  testDir: ".",
  testMatch: /.*\.spec\.ts$/,
  fullyParallel: true,
  // A CI runner shares its host with other jobs: fewer pages at once, and
  // more time for one whose first load compiles the harness.
  workers: process.env.CI ? 4 : undefined,
  timeout: 60_000,
  forbidOnly: !!process.env.CI,
  // On CI one retry: the browser shares the runner host's network, and
  // another job creating or removing a Docker network there makes Chromium
  // abort a navigation (net::ERR_NETWORK_CHANGED). A real violation fails
  // both tries; Playwright reports a pass on the retry as flaky.
  retries: process.env.CI ? 1 : 0,
  reporter: [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1200, height: 800 } } }],
  webServer: {
    command: `npx vite harness --port ${port} --strictPort --host 127.0.0.1`,
    // frontend/, where the harness folder is.
    cwd: fileURLToPath(new URL("..", import.meta.url)),
    url: `http://127.0.0.1:${port}/pages.html`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
});
