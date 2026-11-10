// The harness states the browser checks open: every entry, the Settings
// sections, the dialogs and the first-run wizard's steps. A state that
// needs its own look gets its own line.
import { THEMES as OFFERED } from "../src/theme";

// Every theme Settings → Home offers but System, which is the palette the
// light and dark runs already draw.
export const THEMES = OFFERED.map((t) => t.id).filter((id) => id !== "system");

export const SETTINGS = ["account", "report-design", "provider", "run-defaults", "connections", "updates", "home", "licences"];

export const STATES: string[] = [
  "/index.html",
  "/pages.html",
  ...SETTINGS.map((s) => `/pages.html?settings=${s}`),
  "/modals.html#runpanel", "/modals.html#runpanel-hosted", "/modals.html#runpanel-keys", "/modals.html#runpanel-file-intake",
  "/modals.html#runpanel-step-files", "/modals.html#runpanel-step-tools", "/modals.html#runpanel-step-details", "/modals.html#runpanel-step-review",
  "/modals.html#runpanel-schedule",
  "/modals.html#install-unscanned", "/modals.html#install-mcp-env",
  "/modals.html#toolspanel", "/modals.html#chat",
  "/modals.html#bugreport", "/modals.html#bugreport-signedout",
  "/modals.html#whatsnew", "/modals.html#whatsnew-many",
  "/modals.html#card-confirm", "/modals.html#card-exec", "/modals.html#card-notice",
  ...["0", "1", "2", "3", "4", "5", "6", "7", "cloud"].map((s) => `/wizard.html#${s}`),
  "/knowledge.html#knowledge", "/knowledge.html#knowledge-local", "/knowledge.html#knowledge-cloud",
  "/drafts.html#draft-local", "/drafts.html#draft-landed", "/drafts.html#draft-failed",
  "/reviews.html#validate", "/reviews.html#review-running", "/reviews.html#report-held",
  "/runs.html#run-running", "/runs.html#run-finished", "/runs.html#run-failed", "/runs.html#hosted-running",
];
