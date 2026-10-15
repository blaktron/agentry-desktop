// First-run wizard preview. Selects the step by
// location.hash (#0 welcome | #1 sign-in | #2 Local or Cloud | #3 runtime |
// #4 AI Review key | #5 coding agent | #6 done | #cloud the Cloud path's
// data page) and renders it over the shell chrome, the same
// pattern the run-surface modals use. Not shipped: it sits outside tsconfig
// include and the app's Vite entry.
//
// Suffixes pick a state: #5 shows the step after
// "Look for coding agents" with Claude Code found and chosen and Codex
// missing; #5-missing the same with neither on the machine; #5-unprobed
// the step as it opens, before the look; #6-nothing every step skipped —
// the Nothing set up ending instead of Done.
//
// The AI Review key: no key is saved unless the
// suffix is -saved or -keyonly, or the step is Done and something was set
// up; -refused has the keychain refuse the save, so the shot script types a
// key and clicks Save; -slow has the save never answer, so the footer's
// hold can be shot. #6-keyonly is Done after the key alone was saved: the
// policy exists, written from defaults by that save, and no provider is set
// up, so the ending is still Nothing set up.
import { createRoot } from "react-dom/client";
import "../src/style.css";

// wizard.html#3-pendingRestart (or #6-pendingRestart) previews the runtime
// step's docker card, and the done recap, in that docker state:
// installable | groupMissing | pendingRestart | manual.
// #3-models previews the runtime step after Get models filled the list;
// #3-nomodels the failed listing (the model stays typeable); #3-subscription
// a stored Anthropic subscription token and its sentence;
// #3-refused a save the backend refuses after the
// key was stored — the shot script types the custom provider's form and
// clicks Save; #3-preparing an image pull that never
// ends in the harness, #3-prepared one that ends, #3-prepare-refused one the
// backend refuses — the shot script clicks Prepare images now and shoots the
// step holding, then releasing, its Continue and Back.
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
// The save refused after the key was stored: the shot script
// switches to the custom provider and types a bad URL.
const refused = suffix === "refused";
// The image pull: in flight and never answering in the
// harness; answered after a beat; refused by the backend.
const preparing = suffix === "preparing";
const prepared = suffix === "prepared";
const prepareRefused = location.hash.endsWith("-prepare-refused");
const keyOnly = suffix === "keyonly";
const harnessesMissing = suffix === "missing" || nothing || keyOnly;
const dockerState = ["installable", "groupMissing", "pendingRestart", "manual"].includes(suffix) ? suffix : "";
// Model sign-ins: #3-chatgpt OpenAI with nothing
// kept (Sign in with ChatGPT leads, the key second); #3-claudekept the
// Claude subscription chosen and signed in; #3-waiting a ChatGPT sign-in
// waiting in the browser (the shot clicks Sign in with ChatGPT, which never
// answers here); #3-copilotcode Sign in with GitHub showing GitHub's code,
// copied (the shot clicks the button; the code arrives as the event);
// #3-connected an OpenRouter account connected, its model typed;
// #3-copilotkept a Copilot sign-in kept, its plan's models listed by
// Copilot CLI once asked (the shot clicks List).
const signInState = ["chatgpt", "claudekept", "waiting", "copilotcode", "connected", "copilotkept"].includes(suffix) ? suffix : "";
const signInProvider = { chatgpt: "openai", waiting: "openai", claudekept: "anthropic", copilotcode: "copilot", connected: "openrouter", copilotkept: "copilot" }[signInState] ?? "";
const signInKind = { claudekept: "claude-signin", copilotkept: "copilot-signin" }[signInState] ?? "";
const keptSignIns = signInState === "claudekept"
  ? [{ vendor: "claude", provider: "anthropic", kind: "claude-signin", connected: false, account: "you@example.com (Example Org)", image: true, createdAt: "2026-10-11T12:00:00Z" }]
  : signInState === "copilotkept"
    ? [{ vendor: "copilot", provider: "copilot", kind: "copilot-signin", connected: false, account: "octocat", image: true, createdAt: "2026-10-11T12:00:00Z" }]
  : signInState === "connected"
    ? [{ vendor: "openrouter", provider: "openrouter", kind: "api", connected: true, account: "you", image: true, createdAt: "2026-10-11T12:00:00Z" }]
    : [];
// #3-unlisted: a hand-written policy names a provider no tile shows:
// no tile lit, the line under them saying so, and
// the form paid by a key.
const unlisted = suffix === "unlisted";
// The runtime's events, so a binding can answer through them as Go does.
const handlers: Record<string, ((...a: any[]) => void)[]> = {};
const setup = {
  dockerOk: dockerState === "", imagesReady: !nothing && !keyOnly, policyExists: !nothing,
  providerSetUp: !nothing && !keyOnly,
  policyPath: "~/.agentry/runner/policy.yaml",
  provider: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY",
  apiKeySource: nothing || keyOnly ? "" : "keyring", keysStored: nothing || keyOnly ? [] : ["anthropic"], keyStoreErr: "",
  apiKeyKind: subscription ? "subscription" : "", keyNotice: subscription ? catalog.notice : "",
  baseUrl: "", timeoutSeconds: 900, maxTurns: 60,
  ...(signInProvider ? {
    provider: signInProvider, apiKeyKind: signInKind, providerSetUp: signInState !== "chatgpt" && signInState !== "copilotcode",
    model: { claudekept: "sonnet", connected: "anthropic/claude-sonnet-4.5", copilotkept: "auto" }[signInState] ?? "",
    apiKeyEnv: "", keysStored: [],
  } : {}),
  ...(unlisted ? { provider: "mistral", model: "mistral-large-latest", apiKeyEnv: "MISTRAL_API_KEY", apiKeySource: "env", keysStored: [] } : {}),
};

// #2 is the Local or Cloud step signed out, Cloud locked; #2-signedin the
// same signed in, Cloud open; #cloud the Cloud path's data page, signed in.
const cloudPage = location.hash === "#cloud";
const signedIn = suffix === "signedin" || cloudPage;
const settingsView = {
  defaultRegistry: "https://secureagentry.net",
  configPath: "~/.config/agentry/config.yaml",
  onboarded: false,
  registries: [
    { url: "https://secureagentry.net", isDefault: true, hasToken: signedIn, tokenSource: signedIn ? "keyring" : "" },
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
const keySaved = suffix === "saved" || keyOnly || (step === 6 && !nothing);
const noKey = { stored: false, envName: "TYPESAFE_API_KEY", envSet: false, ready: false };
const savedKey = { stored: true, source: "keyring", envName: "TYPESAFE_API_KEY", envSet: false, ready: true };

// Engine wakes between the parent check and prerequisite detection.
let dockerChecks = 0;
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
    SetupStatus: async () => suffix === "waking"
      ? { ...setup, dockerOk: ++dockerChecks > 1, dockerErr: "engine is waking" }
      : setup,
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
    // the policy untouched.
    StoreModelKey: async (provider: string) => ({ ...setup, keysStored: [...setup.keysStored, provider] }),
    // The CLI's sentence for plain http off this machine, as pages.tsx
    // answers it; without it the step's URL field
    // throws and the wizard unmounts.
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
    ModelSignIns: async () => keptSignIns,
    ChatGPTModels: async () => ["gpt-5.2", "gpt-5.2-codex", "gpt-5-mini"],
    CopilotModels: async () => [
      { id: "claude-sonnet-5.5", name: "Claude Sonnet 5.5", multiplier: 1 },
      { id: "claude-opus-5.5", name: "Claude Opus 5.5", multiplier: 10 },
      { id: "gpt-5-mini", name: "GPT-5 mini", multiplier: 0 },
    ],
    SignInChatGPT: async () => new Promise(() => {}),
    SignInClaude: async () => ({ signedIn: { vendor: "claude", account: "you@example.com", models: [] } }),
    SignInCopilot: async () => {
      setTimeout(() => (handlers["signin:code"] ?? []).forEach((h) => h({ vendor: "copilot", page: "https://github.com/login/device", code: "WDJB-MJHT" })), 50);
      return new Promise(() => {});
    },
    ConnectAccount: async () => new Promise(() => {}),
    CancelSignIn: async () => true,
    RemoveModelSignIn: async () => "revoked at the vendor",
    ApplySignInSetup: async () => setup,
  },
  SettingsAPI: {
    AddRegistry: async () => settingsView,
    SignIn: async () => ({ user: { handle: "you" }, tokenStore: "keyring", pinNote: "pinned" }),
    SignInViaBrowser: async () => ({ user: { handle: "you" }, tokenStore: "keyring", pinNote: "pinned" }),
    CancelBrowserSignIn: async () => true,
    MarkOnboarded: async () => settingsView,
    SetRunWhere: async (where: string) => { g.__runWhere = where; return { where }; },
  },
} };
g.runtime = {
  EventsOn: (name: string, cb: (...a: any[]) => void) => { (handlers[name] ??= []).push(cb); return () => { handlers[name] = (handlers[name] ?? []).filter((h) => h !== cb); }; },
  EventsOnMultiple: (name: string, cb: (...a: any[]) => void) => { (handlers[name] ??= []).push(cb); return () => { handlers[name] = (handlers[name] ?? []).filter((h) => h !== cb); }; }, EventsOnce: () => () => {}, EventsOff: () => {}, EventsEmit: () => {}, LogPrint: () => {}, ClipboardSetText: async () => true };

import Wizard from "../src/pages/Wizard";
import { stepsFor } from "../src/wizardPath";
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
      initialStep={cloudPage ? "cloud" : stepsFor("local")[step] ?? "welcome"}
      probeOnEntry={step === 5 && suffix !== "unprobed"}
    />
  </>,
);
