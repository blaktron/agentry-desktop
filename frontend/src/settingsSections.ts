// Settings' sections: the landing's cards in order, each section's parts,
// where each place that opens Settings goes, and what an older section name
// now means. Pure, so the node tests can import it; Settings.tsx draws them.
//
// Eight sections, each grouping what a person sets together:
//   Account          the registries and the sign-in to each; signed in, the
//                    profile, the subscription and the account's Writing Voice
//   Report design    how the PDFs runs export look
//   Models and keys  the saved models runs use and the providers that pay
//                    for them, and the person's own service keys
//   Runs             how runs behave, where they go (runhosts included), what
//                    they keep and the container engine they use
//   Connections      the sign-ins to MCP servers and the destinations a run
//                    may send its result to: every outside service this
//                    computer keeps credentials for
//   Updates and installs  this app's updates, QuickAgents' updates, the
//                    coding tool Install also writes to, and the Agentry
//                    plugin in each coding tool
//   Home             the theme, accessibility (text size and motion) and
//                    Home's cards
//   Licences
//
// The persistent agents are managed on the Agents page, Meeting notes on its
// own page, and the applications and computer use on Plugins → Your computer.

export type SettingsSection = "account" | "report-design" | "provider" | "run-defaults" | "connections" | "updates" | "home" | "licences";

// The parts a landing link or a deep link can name; each section page
// renders its part under the anchor settings-part-<id>.
export type SettingsPart =
  | "profile" | "subscription" | "account-voice" | "registries" | "add-registry"
  | "library" | "design-editor" | "design-preview" | "design-images"
  | "models" | "ai-review-key" | "web-search" | "hybrid-analysis-key"
  | "budgets" | "chats" | "where" | "runhosts" | "output" | "deliverables" | "history" | "voice" | "unsafe" | "malware" | "docker" | "images"
  | "connections" | "own-clients" | "destinations"
  | "desktop" | "quickagents" | "coding-tools"
  | "theme" | "accessibility" | "home-agent" | "home-cards" | "pins" | "reset-home";

/** A part as the landing links it. needsSignIn: linked only while the
 *  active registry is signed in, as the section page shows it. */
export type Part = { id: SettingsPart; label: string; needsSignIn?: boolean };

export type SectionMeta = { id: SettingsSection; title: string; sentence: string; parts: Part[] };

// SECTIONS is the landing's order and each section's parts. A part's id is
// the anchor its section page scrolls to when the landing's link names it.
export const SECTIONS: SectionMeta[] = [
  {
    id: "account", title: "Account",
    sentence: "The registries this app talks to and your sign-in to each, and, while you are signed in, your profile, your subscription and your account's Writing Voice.",
    parts: [
      { id: "profile", label: "Profile", needsSignIn: true }, { id: "subscription", label: "Subscription", needsSignIn: true },
      { id: "account-voice", label: "Writing Voice", needsSignIn: true },
      { id: "registries", label: "Registries" }, { id: "add-registry", label: "Add a registry" },
    ],
  },
  {
    id: "report-design", title: "Report design",
    sentence: "How the PDFs your runs export look: your library of designs, each a whole brand with its header, footer and cover page, kept on your account.",
    parts: [{ id: "library", label: "Library" }],
  },
  {
    id: "provider", title: "Models and keys",
    sentence: "Your saved models, the default runs use and the providers that pay for them, and your own AI Review, web search and Hybrid Analysis keys.",
    parts: [
      { id: "models", label: "Models" }, { id: "ai-review-key", label: "AI Review key" },
      { id: "web-search", label: "Web search" }, { id: "hybrid-analysis-key", label: "Hybrid Analysis key" },
    ],
  },
  {
    id: "run-defaults", title: "Runs",
    sentence: "How long a run may take and when a chat ends, where runs go (your runhosts included), where their files land and are sent, which runs are remembered, Unsafe Mode, and the container engine runs on this computer use.",
    parts: [
      { id: "budgets", label: "Timeout and turns" }, { id: "chats", label: "Chat idle timeout" }, { id: "where", label: "Where to run" },
      { id: "runhosts", label: "Runhosts" }, { id: "output", label: "Output folder" }, { id: "deliverables", label: "Deliverables" },
      { id: "history", label: "Recent runs" }, { id: "unsafe", label: "Unsafe Mode" }, { id: "malware", label: "Malware check" },
      { id: "docker", label: "Container engine" }, { id: "images", label: "Container images" }, { id: "voice", label: "Writing Voice" },
    ],
  },
  {
    id: "connections", title: "Connections",
    sentence: "The outside services this computer keeps your credentials for: sign-ins to MCP servers, with Disconnect, your own OAuth clients, and the webhooks and S3 buckets a run may send its result to.",
    parts: [{ id: "connections", label: "Sign-ins" }, { id: "own-clients", label: "Your OAuth clients" }, { id: "destinations", label: "Destinations" }],
  },
  {
    id: "updates", title: "Updates and installs",
    sentence: "Whether this app looks for a newer Agentry Desktop, the agentry command it carries on a Mac, how QuickAgents installed here are updated, the coding tool Install also puts them in, and the Agentry plugin in each coding tool on this computer.",
    parts: [{ id: "desktop", label: "Agentry Desktop" }, { id: "quickagents", label: "QuickAgents" }, { id: "coding-tools", label: "Coding tools" }],
  },
  {
    id: "home", title: "Home",
    sentence: "The theme Agentry Desktop is drawn in, its text size and motion, which cards Home shows, the QuickAgents pinned to it, and Reset Home.",
    parts: [{ id: "theme", label: "Theme" }, { id: "accessibility", label: "Accessibility" }, { id: "home-agent", label: "Agent on Home" }, { id: "home-cards", label: "Cards" }, { id: "pins", label: "Pinned" }, { id: "reset-home", label: "Reset Home" }],
  },
  {
    id: "licences", title: "Licences",
    sentence: "The licences of the open-source software, fonts and logos inside Agentry Desktop.",
    parts: [],
  },
];

export const sectionTitle = (s: SettingsSection) => SECTIONS.find((x) => x.id === s)?.title ?? "Settings";

/** The parts the landing links for a section, by the sign-in. */
export const partsShown = (s: SectionMeta, signedIn: boolean) => s.parts.filter((p) => !p.needsSignIn || signedIn);

// Where each place that opens Settings goes: straight to the section it
// means rather than to the landing.
export const SETTINGS_FOR = {
  // the sidebar's line for a newer Agentry Desktop
  selfUpdate: { section: "updates", part: "desktop" },
  // the "Setup isn't finished" notice on Home
  setupNotice: { section: "provider" },
  // Home's Profile, Sign in and Run defaults cards
  homeProfile: { section: "account", part: "profile" },
  homeSignIn: { section: "account", part: "registries" },
  homeRunDefaults: { section: "run-defaults" },
  // Home's Deliverables card
  homeDeliverables: { section: "run-defaults", part: "deliverables" },
  // Home's Recent runs card
  homeRunHistory: { section: "run-defaults", part: "history" },
  // Home's own Home settings link, and the line when every card is hidden
  homeSettings: { section: "home" },
  // Knowledge's "Not signed in" notice
  knowledgeSignIn: { section: "account", part: "registries" },
  // the Publisher section's Sign in, and its "sign in again" notice for a
  // sign-in without agent:manage
  publisherSignIn: { section: "account", part: "registries" },
  // the Report bug card's Sign in, signed out or refused
  bugReportSignIn: { section: "account", part: "registries" },
  // Build's Describe it, whose drafting needs the AI Review key
  describeKey: { section: "provider", part: "ai-review-key" },
  // Review Your QuickAgents locally, which needs the AI Review key
  reviewKey: { section: "provider", part: "ai-review-key" },
  // the sidebar's Unsafe Mode badge
  unsafeMode: { section: "run-defaults", part: "unsafe" },
  // Home's Runhost card, and Get started's runhost rows
  homeRunhosts: { section: "run-defaults", part: "runhosts" },
  // Home's Main card's "Set up a model" link
  mainModel: { section: "provider", part: "models" },
} satisfies Record<string, { section: SettingsSection; part?: SettingsPart }>;

/** Where a Settings link lands: a section (and a part of it), or one of the
 *  pages a former section moved to. */
export type SettingsPlace =
  | { page: "settings"; section: SettingsSection; part?: SettingsPart }
  | { page: "agents" }
  | { page: "meetings" }
  | { page: "computer" };

// The sections an older app, a notification kept from one, or the
// Assistant may still name, and what holds their settings now.
const MOVED: Record<string, SettingsPlace> = {
  profile: { page: "settings", section: "account", part: "profile" },
  registry: { page: "settings", section: "account", part: "registries" },
  "coding-agents": { page: "settings", section: "updates", part: "coding-tools" },
  runhosts: { page: "settings", section: "run-defaults", part: "runhosts" },
  destinations: { page: "settings", section: "connections", part: "destinations" },
  "persistent-agents": { page: "agents" },
  "meeting-notes": { page: "meetings" },
  "your-computer": { page: "computer" },
};

/** Where a link naming a Settings section, and maybe a part of it, opens.
 *  A section of today's opens with the part it names, if any; a former
 *  section's name opens what holds it now, at the part the link named when
 *  that part is still there. Null for a name Settings never had. */
export function settingsPlace(section: string | undefined, part?: string): SettingsPlace | null {
  if (!section) return null;
  const now = SECTIONS.find((s) => s.id === section);
  if (now) return part ? { page: "settings", section: now.id, part: part as SettingsPart } : { page: "settings", section: now.id };
  const moved = MOVED[section];
  if (!moved) return null;
  if (moved.page !== "settings" || !part) return moved;
  const meta = SECTIONS.find((s) => s.id === moved.section);
  return meta?.parts.some((p) => p.id === part) ? { ...moved, part: part as SettingsPart } : moved;
}
