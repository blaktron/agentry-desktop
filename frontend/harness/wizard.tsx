// First-run wizard preview (agentry-desktop#41). Selects the step by
// location.hash (#0 welcome | #1 sign-in | #2 runtime | #3 done) and renders
// it over the shell chrome, the same pattern the run-surface modals use. Not
// shipped: it sits outside tsconfig include and the app's Vite entry.
import { createRoot } from "react-dom/client";
import "../src/style.css";

// wizard.html#2-pendingRestart (or #3-pendingRestart) previews the runtime
// step's docker card, and the done recap, in that docker state
// (agentry-desktop#64): installable | groupMissing | pendingRestart | manual.
const dockerState = (location.hash.match(/-(\w+)$/) || [])[1] || "";
const setup = {
  dockerOk: dockerState === "", imagesReady: true, policyExists: true,
  policyPath: "~/.agentry/runner/policy.yaml",
  provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY",
  baseUrl: "", timeoutSeconds: 900, maxTurns: 60,
};

const settingsView = {
  defaultRegistry: "https://secureagentry.net",
  configPath: "~/.config/agentry/config.yaml",
  onboarded: false,
  registries: [
    { url: "https://secureagentry.net", isDefault: true, hasToken: false, tokenSource: "" },
  ],
};

const g = window as any;
g.go = { main: {
  RunAPI: {
    SetupStatus: async () => setup,
    PrereqStatus: async () => ({
      dockerOk: dockerState === "", state: dockerState || "ready", os: "linux", distro: "fedora",
      canInstall: dockerState !== "manual", engineInstalled: dockerState !== "installable" && dockerState !== "",
      inDockerGroup: dockerState === "pendingRestart" || dockerState === "manual",
      downloadUrl: "https://docs.docker.com/engine/install/",
      command: "dnf install -y moby-engine && systemctl enable --now docker && usermod -aG docker me",
      groupCommand: "usermod -aG docker me",
      note: "After installation, log out and back in (or reboot) so your user joins the docker group.",
      dockerErr: "permission denied while trying to connect to the docker daemon socket",
    }),
    AddToDockerGroup: async () => ({ state: "pendingRestart" }),
    RestartNow: async () => undefined,
  },
  SettingsAPI: {
    AddRegistry: async () => settingsView,
    SignIn: async () => ({ user: { handle: "you" }, tokenStore: "keyring", pinNote: "pinned" }),
    SignInViaBrowser: async () => ({ user: { handle: "you" }, tokenStore: "keyring", pinNote: "pinned" }),
    CancelBrowserSignIn: async () => true,
    MarkOnboarded: async () => settingsView,
  },
} };
g.runtime = { EventsOn: () => () => {}, EventsOnMultiple: () => () => {}, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {} };

import Wizard from "../src/pages/Wizard";
import { Wordmark } from "../src/ui";

function Backdrop() {
  return (
    <div className="shell" style={{ height: "100vh" }}>
      <div className="sidebar"><div className="brand"><Wordmark sub="Desktop" /></div>
        {["Browse", "Installed", "Build", "Validate", "Settings"].map((n) => <button key={n} className={`nav-item ${n === "Browse" ? "active" : ""}`}>{n}</button>)}
      </div>
      <div className="content" />
    </div>
  );
}

const step = Number(location.hash.replace("#", "").split("-")[0]) || 0;
createRoot(document.getElementById("root")!).render(
  <>
    <Backdrop />
    <Wizard
      settings={settingsView as any}
      activeRegistry={settingsView.defaultRegistry}
      onChanged={async () => settingsView as any}
      onSelectRegistry={() => {}}
      onFinish={() => {}}
      initialStep={step}
    />
  </>,
);
