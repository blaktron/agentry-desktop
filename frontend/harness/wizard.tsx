// First-run wizard preview (agentry-desktop#41). Selects the step by
// location.hash (#0 welcome | #1 sign-in | #2 runtime | #3 AI Review key |
// #4 coding agent | #5 done) and renders it over the shell chrome, the same
// pattern the run-surface modals use. Not shipped: it sits outside tsconfig
// include and the app's Vite entry.
//
// Suffixes pick a state (agentry-desktop#89): #4 shows the step after
// "Look for coding agents" with Claude Code found and chosen and Codex
// missing; #4-missing the same with neither on the machine; #4-unprobed
// the step as it opens, before the look; #5-nothing every step skipped —
// the Nothing set up ending instead of Done.
//
// The AI Review key (agentry-desktop#462): no key is saved unless the
// suffix is -saved or -keyonly, or the step is Done and something was set
// up; -refused has the keychain refuse the save, so the shot script types a
// key and clicks Save; -slow has the save never answer, so the footer's
// hold can be shot. #5-keyonly is Done after the key alone was saved: the
// policy exists, written from defaults by that save, and no provider is set
// up, so the ending is still Nothing set up.
import { createRoot } from "react-dom/client";
import "../src/style.css";

// wizard.html#2-pendingRestart (or #5-pendingRestart) previews the runtime
// step's docker card, and the done recap, in that docker state
// (agentry-desktop#64): installable | groupMissing | pendingRestart | manual.
// #2-models previews the runtime step after Get models filled the list;
// #2-nomodels the failed listing (the model stays typeable); #2-subscription
// a stored Anthropic subscription token and its sentence
// (agentry-desktop#131); #2-refused a save the backend refuses after the
// key was stored — the shot script types the custom provider's form and
// clicks Save (agentry-desktop#141); #2-preparing an image pull that never
// ends in the harness, #2-prepared one that ends, #2-prepare-refused one the
// backend refuses — the shot script clicks Prepare images now and shoots the
// step holding, then releasing, its Continue and Back (agentry-desktop#145).
const suffix = (location.hash.match(/-(\w+)$/) || [])[1] || "";
const modelsFail = suffix === "nomodels";
const subscription = suffix === "subscription";
const catalog = {
  provider: "anthropic", kind: subscription ? "subscription" : "api",
  notice: subscription ? "an Anthropic subscription token works with Claude Code only (agentry run --adapter claude); runs in the container harness need a Console API key (sk-ant-api…)" : "",
  models: [
    { id: "claude-opus-5", displayName: "Claude Opus 5" },
    { id: "claude-sonnet-5", displayName: "Claude Sonnet 5" },
    { id: "claude-sonnet-4-5", displayName: "Claude Sonnet 4.5" },
    { id: "claude-haiku-4-5", displayName: "Claude Haiku 4.5" },
  ],
};
const nothing = suffix === "nothing";
// The save refused after the key was stored (desktop#141): the shot script
// switches to the custom provider and types a bad URL.
const refused = suffix === "refused";
// The image pull (desktop#145): in flight and never answering in the
// harness; answered after a beat; refused by the backend.
const preparing = suffix === "preparing";
const prepared = suffix === "prepared";
const prepareRefused = location.hash.endsWith("-prepare-refused");
const keyOnly = suffix === "keyonly";
const harnessesMissing = suffix === "missing" || nothing || keyOnly;
const dockerState = ["installable", "groupMissing", "pendingRestart", "manual"].includes(suffix) ? suffix : "";
const setup = {
  dockerOk: dockerState === "", imagesReady: !nothing && !keyOnly, policyExists: !nothing,
  providerSetUp: !nothing && !keyOnly,
  policyPath: "~/.agentry/runner/policy.yaml",
  provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY",
  apiKeySource: nothing || keyOnly ? "" : "keyring", keysStored: nothing || keyOnly ? [] : ["anthropic"], keyStoreErr: "",
  apiKeyKind: subscription ? "subscription" : "", keyNotice: subscription ? catalog.notice : "",
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

const harnesses = [
  {
    name: "claude", displayName: "Claude Code", executable: "claude", installUrl: "https://github.com/anthropics/claude-code",
    found: !harnessesMissing, path: harnessesMissing ? "" : "/home/me/.local/bin/claude",
    pathFromPolicy: false, source: harnessesMissing ? "" : "known location", version: harnessesMissing ? "" : "2.1.263 (Claude Code)", versionMatches: !harnessesMissing,
    warnings: [], reason: harnessesMissing ? "claude is not installed on this machine (not on PATH) — install it, or set harness.claude.path in the policy" : "",
    chosen: !harnessesMissing && !nothing,
    mcpCommand: "claude mcp add agentry -- node /path/to/agentry-mcp/dist/index.js",
  },
  {
    name: "codex", displayName: "Codex", executable: "codex", installUrl: "https://github.com/openai/codex",
    found: false, path: "", pathFromPolicy: false, source: "", version: "", versionMatches: false, warnings: [],
    reason: "codex is not installed on this machine (not on PATH) — install it, or set harness.codex.path in the policy",
    chosen: false,
    mcpCommand: "codex mcp add agentry -- node /path/to/agentry-mcp/dist/index.js",
  },
];

const step = Number(location.hash.replace("#", "").split("-")[0]) || 0;
const keySaved = suffix === "saved" || keyOnly || (step === 5 && !nothing);
const noKey = { stored: false, envName: "TYPESAFE_API_KEY", envSet: false, ready: false };
const savedKey = { stored: true, source: "keyring", envName: "TYPESAFE_API_KEY", envSet: false, ready: true };

const g = window as any;
g.go = { main: {
  App: { OpenExternal: async (url: string) => { (g.__opened ??= []).push(url); } },
  RunAPI: {
    JevKeyStatus: async () => (keySaved ? savedKey : noKey),
    StoreJevKey: async () => {
      if (refused) throw new Error("store the key in the OS keychain: The name org.freedesktop.secrets was not provided by any .service files");
      if (suffix === "slow") await new Promise(() => {});
      return savedKey;
    },
    ForgetJevKey: async () => noKey,
    TestJevKey: async () => ({ ok: true, message: "The key works: TypeSafe answered one question with it.", from: "the keychain" }),
    SetupStatus: async () => setup,
    ApplySetup: async () => {
      if (refused) throw new Error("the base URL must be an absolute http(s) URL: \"llm.example.net/v1\"");
      if (preparing) await new Promise(() => {});
      if (prepared) await new Promise((r) => setTimeout(r, 400));
      if (prepareRefused) throw new Error("gateway/docker environment: pull docker/mcp-gateway: permission denied while trying to connect to the docker daemon socket");
      return { ...setup, apiKeySource: "keyring", keysStored: ["anthropic"] };
    },
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
    HarnessStatus: async () => harnesses,
    ChosenHarnesses: async () => harnesses.filter((h) => h.chosen),
    // The key-only call answers with the provider it stored under listed,
    // the policy untouched (desktop#139, #141).
    StoreModelKey: async (provider: string) => ({ ...setup, keysStored: [...setup.keysStored, provider] }),
    // The CLI's sentence for plain http off this machine, as pages.tsx
    // answers it (agentry-desktop#317); without it the step's URL field
    // throws and the wizard unmounts (agentry-desktop#378).
    EndpointWarning: async (u: string) => {
      const m = /^http:\/\/([^/:]+)/i.exec(u.trim());
      if (!m || m[1] === "localhost" || m[1].startsWith("127.")) return "";
      return `${u.trim()} uses plain http: the API key and the prompts travel unencrypted to ${m[1]}. Use https if the endpoint supports it.`;
    },
    ListModels: async () => {
      if (modelsFail) throw new Error("custom: list models: 404 Not Found");
      return catalog;
    },
    ChooseHarness: async (name: string) => harnesses.map((h) => ({ ...h, chosen: h.name === name, pathFromPolicy: h.name === name })),
  },
  SettingsAPI: {
    AddRegistry: async () => settingsView,
    SignIn: async () => ({ user: { handle: "you" }, tokenStore: "keyring", pinNote: "pinned" }),
    SignInViaBrowser: async () => ({ user: { handle: "you" }, tokenStore: "keyring", pinNote: "pinned" }),
    CancelBrowserSignIn: async () => true,
    MarkOnboarded: async () => settingsView,
  },
} };
g.runtime = { EventsOn: () => () => {}, EventsOnMultiple: () => () => {}, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {}, ClipboardSetText: async () => true };

import Wizard from "../src/pages/Wizard";
import { Wordmark } from "../src/ui";
import { SidebarNav } from "../src/nav";

function Backdrop() {
  return (
    <div className="shell" style={{ height: "100vh" }}>
      <div className="sidebar"><div className="brand"><Wordmark sub="Desktop" /></div>
        <SidebarNav page="browse" />
      </div>
      <div className="content" />
    </div>
  );
}

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
      probeOnEntry={step === 4 && suffix !== "unprobed"}
    />
  </>,
);
