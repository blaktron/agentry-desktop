// Model setup's rules that are not React: which ways to pay each provider
// offers, which one the saved policy uses, what this computer keeps for
// each, and what a choice needs before it can be saved. Pure, so node's own
// test runner can hold them without a browser.
//
// The words are for someone who does not know what a key or OAuth is: a
// sign-in leads, an API key comes second, and no choice asks anyone to
// paste a code. Sign in with GitHub shows GitHub's own
// code, which the app copies for the person to paste on GitHub's page.

/** One way to pay for a provider's models. */
export type WayId = "chatgpt" | "claude" | "copilot" | "connect" | "key" | "credits";

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
  /** The tile's name: the label without its
   *  parenthesis, which the logo already says. */
  short: string;
  /** The table's default model ("" where the person types it). */
  model: string;
  apiKeyEnv: string;
  ways: Way[];
  /** How to get a key, shown above the key field when the provider is
   *  picked; none where the provider's own page is obvious. */
  keyGuide?: KeyGuide;
}

/** A short, numbered guide to getting a provider's key: an offer line
 *  above the steps, the page the first step opens, and the steps. */
export interface KeyGuide {
  title: string;
  offer: string;
  link: { label: string; url: string };
  steps: string[];
}

/** Vultr's referral link: a new account made through it gets Vultr's
 *  new-account credit, and Agentry a referral credit, which the offer
 *  line says. */
export const VULTR_SIGN_UP_URL = "https://www.vultr.com/?ref=9925159-9J";

/** The CLI's own wording for the Claude choice (setup.ClaudeSignInLabel):
 *  no logo, no "Sign in with Claude". */
export const CLAUDE_SIGN_IN_LABEL = "Use your Claude subscription (runs Claude Code)";

const key = (hint: string, label = "API key"): Way => ({ id: "key", label, hint, kind: "api" });
const connect = (vendor: string, name: string): Way => ({
  id: "connect", label: "Connect your account", vendor, kind: "api",
  hint: `${name} makes a key for Agentry in your browser, billed to your ${name} account. Nothing to copy or paste.`,
});

/** The Copilot model that lets Copilot choose, the only one on Copilot
 *  Free; always offered, first. */
export const COPILOT_AUTO = "auto";

/** The providers model setup offers, in the order shown. The defaults
 *  mirror the CLI's provider table (pkg/runner/setup), which fills any
 *  gap. */
export const PROVIDERS: ProviderChoice[] = [
  { id: "anthropic", label: "Anthropic (Claude)", short: "Anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY", ways: [
    { id: "claude", label: CLAUDE_SIGN_IN_LABEL, vendor: "claude", kind: "claude-signin",
      hint: "Claude Code signs you in itself, in Anthropic's own page in your browser. Your Claude plan pays for the runs on this computer." },
    key("The Max and Team plans include monthly API credits, spent through a key from the Console organization linked to the plan."),
  ] },
  { id: "openai", label: "OpenAI (ChatGPT)", short: "OpenAI", model: "gpt-5.2", apiKeyEnv: "OPENAI_API_KEY", ways: [
    { id: "chatgpt", label: "Sign in with ChatGPT", vendor: "chatgpt", kind: "chatgpt-plan",
      hint: "Sign in on OpenAI's page in your browser. Your ChatGPT plan pays for the runs on this computer." },
    key("A key from your OpenAI Platform account, billed there."),
  ] },
  { id: "copilot", label: "GitHub Copilot", short: "GitHub Copilot", model: COPILOT_AUTO, apiKeyEnv: "", ways: [
    { id: "copilot", label: "Sign in with GitHub", vendor: "copilot", kind: "copilot-signin",
      hint: "GitHub's page opens in your browser and asks for a short code, which the app copies for you. Your Copilot plan pays for the runs on this computer." },
  ] },
  { id: "openrouter", label: "OpenRouter", short: "OpenRouter", model: "", apiKeyEnv: "OPENROUTER_API_KEY", ways: [
    connect("openrouter", "OpenRouter"), key("A key from your OpenRouter account."),
  ] },
  { id: "poe", label: "Poe", short: "Poe", model: "", apiKeyEnv: "POE_API_KEY", ways: [
    connect("poe", "Poe"), key("A key from poe.com/api/keys."),
  ] },
  { id: "huggingface", label: "Hugging Face", short: "Hugging Face", model: "", apiKeyEnv: "HF_TOKEN", ways: [
    connect("huggingface", "Hugging Face"), key("An access token from your Hugging Face settings.", "Access token"),
  ] },
  { id: "vultr", label: "Vultr Serverless Inference", short: "Vultr", model: "", apiKeyEnv: "VULTR_INFERENCE_API_KEY", ways: [
    key("A key from your Vultr Serverless Inference subscription, billed to your Vultr account."),
  ], keyGuide: {
    title: "Get a Vultr key",
    offer: "New Vultr accounts get US$300 of free credit through our link (Vultr's terms apply; the credit is time-limited). Agentry receives a referral credit.",
    link: { label: "Sign up for Vultr with our link", url: VULTR_SIGN_UP_URL },
    steps: [
      "Sign up for Vultr with our link, below.",
      "In Vultr's portal, open Products, then Serverless, then Inference, and add a Serverless Inference subscription.",
      "Open the subscription, copy the API key from its Overview page, and paste it here.",
    ],
  } },
  { id: "google", label: "Google (Gemini)", short: "Google Gemini", model: "gemini-3-pro", apiKeyEnv: "GEMINI_API_KEY", ways: [
    key("A key from Google AI Studio."),
  ] },
  { id: "custom", label: "Custom endpoint (OpenAI-compatible)", short: "Custom endpoint", model: "", apiKeyEnv: "CUSTOM_API_KEY", ways: [
    key("The key your endpoint expects."),
  ] },
  { id: "agentry", label: "Agentry credits", short: "Agentry credits", model: "", apiKeyEnv: "", ways: [
    { id: "credits", label: "Your Agentry credits", kind: "",
      hint: "The runs on this computer are paid with the credits of the SecureAgentry account you are signed in to, at the rates of runs In Agentry. Agentry chooses the model." },
  ] },
];

/** The words across the Agentry credits tile while no SecureAgentry
 *  account is signed in, as on the wizard's Cloud card. */
export const CREDITS_LOCK = "Please sign in";

/** What the Agentry credits choice says once picked: the account's balance
 *  and the model Agentry uses now, each only as far as the registry said
 *  it, or why it serves none now. */
export function creditsLines(o: { signedIn: boolean; balance: number; balanceKnown: boolean; model: string; unavailable: string }): string[] {
  if (!o.signedIn) return ["Sign in to your SecureAgentry account first: the runs are paid with its credits."];
  const lines = [o.balanceKnown ? `Your account has ${o.balance.toLocaleString("en-US")} ${o.balance === 1 ? "credit" : "credits"}.` : "Your balance could not be read just now."];
  if (o.model) lines.push(`Agentry runs them on ${o.model} now.`);
  else if (o.unavailable) lines.push(`Agentry cannot run them just now: ${o.unavailable}`);
  lines.push("A run stops when the credits are used up; a persistent agent stays up and says so in its chat.");
  return lines;
}

export const providerChoice = (id: string): ProviderChoice | undefined => PROVIDERS.find((p) => p.id === id);

export const providerLabel = (id: string) => providerChoice(id)?.label ?? id;

/** What a provider's tile says it can be paid with, from its ways, so the
 *  tiles show every choice before one is picked:
 *  "Claude plan or API key", "Your account or access token". */
export function tileOffer(p: ProviderChoice): string {
  const words = p.ways.map((w) => {
    switch (w.id) {
      case "claude": return "Claude plan";
      case "chatgpt": return "ChatGPT plan";
      case "copilot": return "Copilot plan";
      case "connect": return "your account";
      case "credits": return "your Agentry credits";
      default: return w.label === "API key" ? w.label : w.label.toLowerCase();
    }
  }).join(" or ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** A listed model as the model field shows it: its name and id, then its
 *  price a million tokens where the provider lists one
 *  ("Qwen 3.8 Flash Next — qwen3.8-flash-next · US$0.10 in, US$0.20 out
 *  per million tokens"). */
export function modelOptionLabel(m: { id: string; displayName?: string; inputUsd?: string; outputUsd?: string }): string {
  let label = m.displayName ? `${m.displayName} — ${m.id}` : m.id;
  if (m.inputUsd && m.outputUsd) label += ` · US$${m.inputUsd} in, US$${m.outputUsd} out per million tokens`;
  else if (m.outputUsd) label += ` · US$${m.outputUsd} out per million tokens`;
  return label;
}

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
 *  policy written from defaults names a provider nobody chose). */
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
 *  (ChatGPT, and Copilot once its list is asked of Copilot CLI), from
 *  Claude Code's aliases, or typed (the big catalogues, and Copilot
 *  before its list).
 *  planModels is the way's own list: ChatGPT's, or Copilot's ids. */
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
      if (planModels.length > 0) {
        return { mode: "plan", options: copilotOffered(planModels), placeholder: "", defaultModel: COPILOT_AUTO };
      }
      return { mode: "typed", options: [], placeholder: "auto lets Copilot choose (the only choice on Copilot Free)", defaultModel: COPILOT_AUTO };
    case "connect":
      return { mode: "typed", options: [], placeholder: `the model id, such as ${CONNECT_EXAMPLES[provider] ?? "provider/model"}`, defaultModel: "" };
    default:
      return { mode: "key", options: [], placeholder: "", defaultModel: providerChoice(provider)?.model ?? "" };
  }
}

/** What Copilot's model field offers: auto first, then each listed id
 *  once, in Copilot CLI's order. A model the plan disables is never in the
 *  list (the CLI leaves it out). */
export function copilotOffered(ids: string[]): string[] {
  const out = [COPILOT_AUTO];
  for (const id of ids) if (id && !out.includes(id)) out.push(id);
  return out;
}

/** A listed Copilot model as the field names it: the id, GitHub's name
 *  where it differs, and the premium-request multiplier where GitHub gave
 *  one (0 is "not given", never "free"). */
export interface CopilotModelInfo { id: string; name: string; multiplier: number }
export function copilotModelLabel(id: string, models: CopilotModelInfo[]): string {
  if (id === COPILOT_AUTO) return "auto — Copilot chooses";
  const m = models.find((x) => x.id === id);
  const parts: string[] = [];
  if (m?.name && m.name !== id) parts.push(m.name);
  if (m && m.multiplier > 0) parts.push(`${m.multiplier}× premium requests`);
  return parts.length > 0 ? `${id} — ${parts.join(", ")}` : id;
}

/** The model to keep when a Copilot list arrives: the one chosen while
 *  the plan still offers it, else auto (as `agentry setup` does). */
export function copilotModelAfterList(current: string, ids: string[]): string {
  return copilotOffered(ids).includes(current.trim()) ? current.trim() : COPILOT_AUTO;
}

/** Why a Copilot list failed, said for this page: GitHub refusing the
 *  sign-in points at the page's own Sign in again, not at the CLI's
 *  command; anything else is said as it came. */
export function copilotListFailure(err: string): string {
  if (/sign in again/i.test(err)) return "GitHub refused the kept sign-in: use Sign in again above, then list again.";
  return `Could not list your plan's models (${err}). Type the model id instead.`;
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
