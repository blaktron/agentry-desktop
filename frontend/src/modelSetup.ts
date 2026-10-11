// Model setup's rules that are not React (plan model-sign-ins.md §4.1;
// agentry-desktop#734, M9): which ways to pay each provider offers, which
// one the saved policy uses, what this computer keeps for each, and what a
// choice needs before it can be saved. Pure, so node's own test runner can
// hold them without a browser.
//
// The words are for someone who does not know what a key or OAuth is
// (operator, 2026-10-11): a sign-in leads, an API key comes second, and no
// choice asks anyone to paste a code. Sign in with GitHub shows GitHub's own
// code, which the app copies for the person to paste on GitHub's page.

/** One way to pay for a provider's models. */
export type WayId = "chatgpt" | "claude" | "copilot" | "connect" | "key";

export interface Way {
  id: WayId;
  /** The button's and the choice's words. */
  label: string;
  /** One sentence under the choice. */
  hint: string;
  /** The kept sign-in's vendor (chatgpt, claude, copilot, or the
   *  provider's own id for a connected account); none for a key. */
  vendor?: string;
  /** The policy's apiKeyKind for the way ("" for a key or a connect). */
  kind: string;
}

export interface ProviderChoice {
  id: string;
  label: string;
  /** The table's default model ("" where the person types it). */
  model: string;
  apiKeyEnv: string;
  ways: Way[];
}

/** The CLI's own wording for the Claude choice (setup.ClaudeSignInLabel,
 *  plan D24): no logo, no "Sign in with Claude". */
export const CLAUDE_SIGN_IN_LABEL = "Use your Claude subscription (runs Claude Code)";

const key = (hint: string, label = "API key"): Way => ({ id: "key", label, hint, kind: "api" });
const connect = (vendor: string, name: string): Way => ({
  id: "connect", label: "Connect your account", vendor, kind: "api",
  hint: `${name} makes a key for Agentry in your browser, billed to your ${name} account. Nothing to copy or paste.`,
});

/** The providers model setup offers, in the order shown. The defaults
 *  mirror the CLI's provider table (pkg/runner/setup), which fills any
 *  gap. */
export const PROVIDERS: ProviderChoice[] = [
  { id: "anthropic", label: "Anthropic (Claude)", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY", ways: [
    { id: "claude", label: CLAUDE_SIGN_IN_LABEL, vendor: "claude", kind: "claude-signin",
      hint: "Claude Code signs you in itself, in Anthropic's own page in your browser. Your Claude plan pays for the runs on this computer." },
    key("The Max and Team plans include monthly API credits, spent through a key from the Console organization linked to the plan."),
  ] },
  { id: "openai", label: "OpenAI (ChatGPT)", model: "gpt-5.2", apiKeyEnv: "OPENAI_API_KEY", ways: [
    { id: "chatgpt", label: "Sign in with ChatGPT", vendor: "chatgpt", kind: "chatgpt-plan",
      hint: "Sign in on OpenAI's page in your browser. Your ChatGPT plan pays for the runs on this computer." },
    key("A key from your OpenAI Platform account, billed there."),
  ] },
  { id: "copilot", label: "GitHub Copilot", model: "auto", apiKeyEnv: "", ways: [
    { id: "copilot", label: "Sign in with GitHub", vendor: "copilot", kind: "copilot-signin",
      hint: "GitHub's page opens in your browser and asks for a short code, which the app copies for you. Your Copilot plan pays for the runs on this computer." },
  ] },
  { id: "openrouter", label: "OpenRouter", model: "", apiKeyEnv: "OPENROUTER_API_KEY", ways: [
    connect("openrouter", "OpenRouter"), key("A key from your OpenRouter account."),
  ] },
  { id: "poe", label: "Poe", model: "", apiKeyEnv: "POE_API_KEY", ways: [
    connect("poe", "Poe"), key("A key from poe.com/api/keys."),
  ] },
  { id: "huggingface", label: "Hugging Face", model: "", apiKeyEnv: "HF_TOKEN", ways: [
    connect("huggingface", "Hugging Face"), key("An access token from your Hugging Face settings.", "Access token"),
  ] },
  { id: "google", label: "Google (Gemini)", model: "gemini-3-pro", apiKeyEnv: "GEMINI_API_KEY", ways: [
    key("A key from Google AI Studio."),
  ] },
  { id: "custom", label: "Custom endpoint (OpenAI-compatible)", model: "", apiKeyEnv: "CUSTOM_API_KEY", ways: [
    key("The key your endpoint expects."),
  ] },
];

export const providerChoice = (id: string): ProviderChoice | undefined => PROVIDERS.find((p) => p.id === id);

export const providerLabel = (id: string) => providerChoice(id)?.label ?? id;

/** The ways provider offers, a sign-in first; a provider not in the list
 *  (one a hand-written policy names) is paid by a key. */
export function waysFor(provider: string): Way[] {
  return providerChoice(provider)?.ways ?? [key("The key the provider expects.")];
}

/** What this computer keeps, as RunAPI.ModelSignIns gives it: never a
 *  token or a key. */
export interface KeptSignIn {
  vendor: string;
  provider: string;
  kind: string;
  connected: boolean;
  account?: string;
  broken?: string;
  keyExpiresAt?: string;
  image: boolean;
}

/** The sign-in this computer keeps for way, if any. A Claude sign-in
 *  counts only where the Claude Code image is signed in: the installed
 *  Claude Code's own (the CLI's --installed) is not what a run here uses. */
export function keptFor(way: Way, kept: KeptSignIn[]): KeptSignIn | undefined {
  if (!way.vendor) return undefined;
  return kept.find((k) => k.vendor === way.vendor && (way.id !== "claude" || k.image));
}

/** The way the saved policy pays for provider: its sign-in kind, a
 *  connected account where one is kept for it (a pasted key forgets the
 *  connect), else a key; or the provider's first way, a sign-in, when the
 *  policy names another provider or nobody has set a provider up yet (a
 *  policy written from defaults names a provider nobody chose,
 *  agentry-desktop#462). */
export function savedWay(provider: string, policyProvider: string, apiKeyKind: string, kept: KeptSignIn[], setUp = true): WayId {
  const ways = waysFor(provider);
  if (provider !== policyProvider || !setUp) return ways[0].id;
  const byKind = ways.find((w) => w.kind === apiKeyKind && w.id !== "key" && w.id !== "connect");
  if (byKind) return byKind.id;
  const c = ways.find((w) => w.id === "connect");
  if (c && keptFor(c, kept)) return "connect";
  return ways.some((w) => w.id === "key") ? "key" : ways[0].id;
}

/** "Signed in as …", "Connected as …", or what to do about it. */
export function keptLine(k: KeptSignIn): string {
  if (k.broken) return `Sign in again — the last refresh was refused (${k.broken})`;
  const verb = k.connected ? "Connected" : "Signed in";
  let line = k.account ? `${verb} as ${k.account}` : verb;
  if (k.keyExpiresAt) line += ` — the key works until ${k.keyExpiresAt.slice(0, 10)}`;
  return line;
}

/** Remove for a sign-in, Disconnect for a connected account. */
export const removeLabel = (k: KeptSignIn) => (k.connected ? "Disconnect" : "Remove");

/** How model setup asks for the model on a way: from the plan's own list
 *  (ChatGPT), from Claude Code's aliases, or typed (the big catalogues,
 *  plan D33, and Copilot until its list is asked of Copilot CLI,
 *  agentry-cli#808). */
export interface ModelAsk {
  mode: "plan" | "pick" | "typed" | "key";
  options: string[];
  placeholder: string;
  defaultModel: string;
}

const CONNECT_EXAMPLES: Record<string, string> = {
  openrouter: "anthropic/claude-sonnet-4.5",
  poe: "Claude-Sonnet-4.6",
  huggingface: "openai/gpt-oss-120b:groq",
};

export function modelAsk(provider: string, way: WayId, planModels: string[]): ModelAsk {
  switch (way) {
    case "chatgpt":
      return { mode: "plan", options: planModels, placeholder: "sign in to see your plan's models", defaultModel: planModels[0] ?? "" };
    case "claude":
      return { mode: "pick", options: ["sonnet", "opus", "haiku"], placeholder: "sonnet, opus, or a Claude model id", defaultModel: "sonnet" };
    case "copilot":
      return { mode: "typed", options: [], placeholder: "auto lets Copilot choose (the only choice on Copilot Free)", defaultModel: "auto" };
    case "connect":
      return { mode: "typed", options: [], placeholder: `the model id, such as ${CONNECT_EXAMPLES[provider] ?? "provider/model"}`, defaultModel: "" };
    default:
      return { mode: "key", options: [], placeholder: "", defaultModel: providerChoice(provider)?.model ?? "" };
  }
}

/** The Copilot plans, by the name GitHub sells them under; each names the
 *  Copilot API host runs reach. */
export const COPILOT_PLANS: { id: string; label: string }[] = [
  { id: "individual", label: "Copilot Free, Pro, Pro+ or Max" },
  { id: "business", label: "Copilot Business" },
  { id: "enterprise", label: "Copilot Enterprise" },
];

/** What a sign-in way still needs before Save: "" when it can be saved. */
export function signInSaveHint(way: Way, kept: KeptSignIn | undefined, model: string): string {
  if (!kept) return `${way.label} first`;
  if (kept.broken) return "sign in again first";
  if (!model.trim()) return way.id === "connect" ? "type the model id" : "pick a model";
  return "";
}

/** The model a way starts with: the saved one where the policy already
 *  pays this way, else the way's own default. */
export function startingModel(provider: string, way: WayId, saved: { provider: string; model: string; way: WayId }, planModels: string[]): string {
  if (saved.provider === provider && saved.way === way && saved.model) return saved.model;
  return modelAsk(provider, way, planModels).defaultModel;
}

/** What pays for the runs, where a sign-in does: Settings' summary line. */
export function paidBy(apiKeyKind: string): string {
  switch (apiKeyKind) {
    case "chatgpt-plan": return "your ChatGPT plan";
    case "claude-signin": return "your Claude subscription";
    case "copilot-signin": return "your Copilot plan";
  }
  return "";
}
