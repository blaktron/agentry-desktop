import React, { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as SelfUpdateAPI from "../../wailsjs/go/main/SelfUpdateAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, PageHeader, Toolbar } from "../ui";
import { rewrapLine } from "../knowledge";
import { RuntimeSetup, PrereqCard, providerLabel } from "./RuntimeSetup";
import { CodingAgentsCard, codingAgentName, exportLabel } from "./CodingAgents";
import { SecretsCard } from "./HostedRun";
import { AIReviewKeyCard } from "./AIReviewKey";

// Settings as five cards (plan settings-cards.md M3, D1, D3, D12;
// agentry-desktop#346): the landing is a card per section, each card the
// whole of a link to its section page, with one sentence, a status line
// and its parts as links; each section page holds the cards Settings held
// before, moved unchanged, under a "← Settings" link back.
export type SettingsSection = "registry" | "updates" | "coding-agents" | "run-defaults" | "provider";

type Part = { id: string; label: string };

// SECTIONS is the landing's order and each section's parts. A part's id is
// the anchor its section page scrolls to when the landing's link names it.
export const SECTIONS: { id: SettingsSection; title: string; sentence: string; parts: Part[] }[] = [
  {
    id: "registry", title: "Registry settings",
    sentence: "The registries this app talks to, your sign-in to each, and the secrets runs In Agentry use where a registry keeps them.",
    parts: [{ id: "registries", label: "Registries" }, { id: "add-registry", label: "Add a registry" }],
  },
  {
    id: "updates", title: "Update settings",
    sentence: "Whether this app looks for a newer Agentry Desktop, and how BriefAgents installed here are updated.",
    parts: [{ id: "desktop", label: "Agentry Desktop" }, { id: "briefagents", label: "BriefAgents" }],
  },
  {
    id: "coding-agents", title: "Coding agents",
    sentence: "The coding agent on this computer, and whether Install also puts a BriefAgent where it reads.",
    parts: [],
  },
  {
    id: "run-defaults", title: "Run defaults",
    sentence: "What a run on this computer needs before it can start: the Docker engine BriefAgent runs use.",
    parts: [],
  },
  {
    id: "provider", title: "Provider settings",
    sentence: "The AI provider and model runs use, and your own AI Review key.",
    parts: [{ id: "provider", label: "Model provider" }, { id: "ai-review-key", label: "AI Review key" }],
  },
];

// Where each place that opens Settings goes (agentry-desktop#346): straight
// to the section it means rather than to the landing.
export const SETTINGS_FOR = {
  // the sidebar's line for a newer Agentry Desktop
  selfUpdate: { section: "updates", part: "desktop" },
  // the "Setup isn't finished" notice on Browse
  setupNotice: { section: "provider" },
  // Knowledge's "Not signed in" notice
  knowledgeSignIn: { section: "registry", part: "registries" },
  // Build's Describe it, whose drafting needs the AI Review key
  describeKey: { section: "provider", part: "ai-review-key" },
} satisfies Record<string, { section: SettingsSection; part?: string }>;

export const sectionTitle = (s: SettingsSection) => SECTIONS.find((x) => x.id === s)?.title ?? "Settings";

export default function Settings({
  settings, activeRegistry, onChanged, onSelectRegistry, onRunSetup, section, part, onOpen,
}: {
  settings: main.SettingsView | null;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  // Re-open the first-run guided setup. Optional so the page renders in the
  // harness without the App shell (agentry-desktop#41).
  onRunSetup?: () => void;
  // The section page shown, or the landing when absent; part is the anchor
  // within it the landing's link named.
  section?: SettingsSection;
  part?: string;
  // Opens a section (and a part of it), or the landing with none.
  onOpen: (section?: SettingsSection, part?: string) => void;
}) {
  // A part named by the landing's link is scrolled to once its section has
  // rendered; the page's own scroll starts at the top otherwise.
  useEffect(() => {
    if (!section || !settings) return;
    const el = part ? document.getElementById(`settings-part-${part}`) : null;
    if (el) el.scrollIntoView({ block: "start" });
  }, [section, part, settings]);

  if (!settings) return <div className="empty">Loading settings…</div>;

  if (!section) {
    return <SettingsLanding settings={settings} activeRegistry={activeRegistry} onRunSetup={onRunSetup} onOpen={onOpen} />;
  }

  const meta = SECTIONS.find((s) => s.id === section);
  return (
    <div data-settings-section={section}>
      <p className="small settings-back"><Button variant="link" onClick={() => onOpen()}>← Settings</Button></p>
      <PageHeader title={meta?.title ?? "Settings"} sub={meta?.sentence} />
      {section === "registry" && (
        <RegistrySettings settings={settings} activeRegistry={activeRegistry} onChanged={onChanged} onSelectRegistry={onSelectRegistry} />
      )}
      {section === "updates" && (
        <>
          <SettingsPart id="desktop" title="Agentry Desktop"><SelfUpdateCard /></SettingsPart>
          <SettingsPart id="briefagents" title="BriefAgents">
            <Card>
              <label className="row gap">
                <input type="checkbox" checked={false} disabled onChange={() => {}} />
                <span>Update BriefAgents automatically</span>
              </label>
              <div className="muted small">
                Not offered yet. The app checks the ones installed here for newer versions on launch and when you
                open a BriefAgent; Update on its page is yours to click.
              </div>
            </Card>
          </SettingsPart>
        </>
      )}
      {section === "coding-agents" && <CodingAgentsCard />}
      {section === "run-defaults" && <RunDefaultsSettings />}
      {section === "provider" && (
        <>
          <div id="settings-part-provider" className="settings-part">
            <RuntimeSetup advanced heading="Model provider" onOpenPrereqs={() => onOpen("run-defaults")} />
          </div>
          <SettingsPart id="ai-review-key" title="AI Review key"><AIReviewKeyCard /></SettingsPart>
        </>
      )}
    </div>
  );
}

function SettingsPart({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section id={`settings-part-${id}`} className="settings-part">
      <h2>{title}</h2>
      {children}
    </section>
  );
}

// The landing's status lines, one per card, from bindings the page already
// had and that read only this computer: the settings file, the self-update
// status the background check left, the coding-agent export, the runner
// policy and Docker, the AI Review key's presence. None calls a registry,
// so none carries a token. A read that fails drops its line, never its card.
type Statuses = {
  update?: main.SelfUpdateView | null;
  coding?: main.CodingAgentsView | null;
  setup?: main.SetupStatusView | null;
  jev?: main.JevKeyView | null;
};

function SettingsLanding({ settings, activeRegistry, onRunSetup, onOpen }: {
  settings: main.SettingsView;
  activeRegistry: string;
  onRunSetup?: () => void;
  onOpen: (section?: SettingsSection, part?: string) => void;
}) {
  const [st, setSt] = useState<Statuses>({});
  useEffect(() => {
    let live = true;
    const put = <K extends keyof Statuses>(k: K) => (v: Statuses[K]) => { if (live) setSt((cur) => ({ ...cur, [k]: v })); };
    SelfUpdateAPI.SelfUpdateStatus().then(put("update")).catch(() => put("update")(null));
    SettingsAPI.CodingAgents().then(put("coding")).catch(() => put("coding")(null));
    RunAPI.SetupStatus().then(put("setup")).catch(() => put("setup")(null));
    RunAPI.JevKeyStatus().then(put("jev")).catch(() => put("jev")(null));
    return () => { live = false; };
  }, []);

  const status: Record<SettingsSection, React.ReactNode> = {
    registry: registryStatus(settings, activeRegistry),
    updates: st.update ? updateStatus(st.update) : null,
    "coding-agents": st.coding ? codingStatus(st.coding) : null,
    "run-defaults": st.setup ? runStatus(st.setup) : null,
    provider: st.setup || st.jev ? providerStatus(st.setup ?? null, st.jev ?? null) : null,
  };

  return (
    <div data-settings-section="landing">
      <PageHeader
        title="Settings"
        sub="Each card opens one group of settings."
        actions={onRunSetup && <Button onClick={onRunSetup}>Run setup again</Button>}
      />
      <div className="settings-cards">
        {SECTIONS.map((s) => (
          <div key={s.id} className="card settings-card">
            {/* The title's button is stretched over the whole card, so the
                card is one link; the part links sit above it. */}
            <button type="button" className="settings-card-link" onClick={() => onOpen(s.id)}>
              <strong>{s.title}</strong>
            </button>
            <p className="muted small">{s.sentence}</p>
            {status[s.id] && <p className="small settings-card-status" role="status">{status[s.id]}</p>}
            {s.parts.length > 1 && (
              <p className="small settings-card-parts">
                {s.parts.map((p, i) => (
                  <React.Fragment key={p.id}>
                    {i > 0 && " · "}
                    <Button variant="link" onClick={() => onOpen(s.id, p.id)}>{p.label}</Button>
                  </React.Fragment>
                ))}
              </p>
            )}
          </div>
        ))}
      </div>
      <p className="muted small selectable">
        Settings file: <span className="mono">{settings.configPath}</span>. Install records and pinned
        signing keys are shared with the CLI under the same directory.
      </p>
    </div>
  );
}

const hostOf = (url: string) => url.replace(/^https?:\/\//, "");

export function registryStatus(settings: main.SettingsView, activeRegistry: string): string {
  const r = settings.registries.find((x) => x.url === activeRegistry);
  if (!r) return settings.registries.length ? "No registry in use." : "No registry added yet.";
  return `${hostOf(r.url)}, ${r.hasToken ? "signed in" : "not signed in"}.`;
}

export function updateStatus(v: main.SelfUpdateView): string {
  if (!v.channel) return "This build has no update channel.";
  if (v.available) return v.available.retired
    ? `${v.available.version} exists, signed by a retired key, and will not be installed.`
    : `Agentry Desktop ${v.available.version} is available. You are running ${v.current}.`;
  if (!v.enabled) return "Not looking for newer versions.";
  if (v.err) return "The last check did not complete.";
  if (v.checked && !v.nothing) return `Up to date (${v.current}) when last checked.`;
  return `Running ${v.current}.`;
}

export function codingStatus(v: main.CodingAgentsView): string {
  const x = v.export;
  if (!x.enabled) return "Install puts BriefAgents in the runner home only.";
  return `Install also writes each BriefAgent as ${exportLabel(x.target)} for ${codingAgentName(v)}.`;
}

export function runStatus(s: main.SetupStatusView): string {
  if (!s.dockerOk) return "Docker is not ready, so BriefAgents cannot run on this computer yet.";
  if (!s.imagesReady) return "Docker answers. The run images are fetched on the first run.";
  return "Docker answers and the run images are ready.";
}

export function providerStatus(s: main.SetupStatusView | null, k: main.JevKeyView | null): string {
  const parts: string[] = [];
  if (s) parts.push(s.policyExists && s.model ? `${providerLabel(s.provider)}, ${s.model}` : "No provider chosen yet");
  if (k) parts.push(k.ready ? "AI Review key saved" : "no AI Review key");
  const line = parts.join(" · ");
  return line.charAt(0).toUpperCase() + line.slice(1) + ".";
}

// RunDefaultsSettings is Settings → Run defaults. Until M4 adds the defaults
// (agentry-desktop#347) it holds the Docker engine's prerequisites, which
// were in Agent runs before; the card says nothing when the engine answers,
// so the section says so itself.
function RunDefaultsSettings() {
  const [setup, setSetup] = useState<main.SetupStatusView | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const read = () => RunAPI.SetupStatus().then(setSetup).catch((e) => setErr(String(e)));
  useEffect(() => { read(); }, []);
  return (
    <>
      <h2>Docker engine</h2>
      <ErrorBox error={err} />
      {setup && (setup.dockerOk
        ? <Card><p className="small" style={{ margin: 0 }}>{runStatus(setup)}</p></Card>
        : <PrereqCard onDone={async () => { await read(); }} />)}
    </>
  );
}

// RegistrySettings is Settings → Registry settings: the registries with
// their sign-in, account and organizations lines, the account's secrets,
// and Add a registry — the cards that opened the one Settings page before.
function RegistrySettings({ settings, activeRegistry, onChanged, onSelectRegistry }: {
  settings: main.SettingsView;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
}) {
  const [newURL, setNewURL] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [signIn, setSignIn] = useState<{ url: string; token: string } | null>(null);
  const [busy, setBusy] = useState(false);
  // The registry a browser sign-in is waiting on; shows the Cancel control.
  const [browserWait, setBrowserWait] = useState<string | null>(null);
  // The account behind each stored credential and, where the registry has
  // subscription tiers, its tier and credits (plan subscription-tiers.md
  // §8). Read after the registries render, so a slow registry never holds
  // the page; a read that fails leaves the card as it was.
  const [accounts, setAccounts] = useState<Record<string, main.AccountView>>({});
  useEffect(() => {
    let cancelled = false;
    const signedIn = settings.registries.filter((r) => r.hasToken);
    Promise.all(signedIn.map(async (r) => [r.url, await SettingsAPI.Account(r.url).catch(() => null)] as const)).then((pairs) => {
      if (cancelled) return;
      const next: Record<string, main.AccountView> = {};
      for (const [url, acct] of pairs) if (acct) next[url] = acct;
      setAccounts(next);
    });
    return () => { cancelled = true; };
  }, [settings.registries]);

  async function wrap(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const addRegistry = () =>
    wrap(async () => {
      await SettingsAPI.AddRegistry(newURL.trim());
      setNewURL("");
      await onChanged();
    });

  const doSignIn = () =>
    wrap(async () => {
      if (!signIn) return;
      const res = await SettingsAPI.SignIn(signIn.url, signIn.token.trim());
      setNotice(
        `Signed in to ${signIn.url} as ${res.user.handle} — PAT stored in ${res.tokenStore === "keyring" ? "the OS keychain" : "the settings file (0600; no keychain available)"}. Signing keys: ${res.pinNote}${rewrapLine(res)}`,
      );
      setSignIn(null);
      await onChanged();
    });

  return (
    <>
      <p className="muted small">
        Credentials live in the OS keychain under the same convention the <span className="mono">agentry</span> CLI
        uses, so a sign-in here works there too.
      </p>
      <ErrorBox error={error} />
      {browserWait && (
        <Notice tone="info">
          Waiting for your browser to authorize this app for <span className="mono">{browserWait}</span>.
          Sign in there and click “Authorize the desktop app”. If the browser did not open, or you
          closed it, cancel and try again.{" "}
          <Button onClick={() => void SettingsAPI.CancelBrowserSignIn()}>Cancel sign-in</Button>
        </Notice>
      )}
      {notice && <Notice tone="ok">{notice}</Notice>}

      <section id="settings-part-registries" className="settings-part">
      <h2>Registries</h2>
      {settings.registries.map((r) => (
        <Card key={r.url}>
          <div className="spread">
            <div className="grow">
              <div className="row">
                <strong className="mono selectable">{r.url}</strong>
                {r.isDefault && <span className="badge kind">default</span>}
                {r.url === activeRegistry && <span className="badge ok">active</span>}
                {r.hasToken
                  ? <span className="badge">signed in ({r.tokenSource})</span>
                  : <span className="badge warn">anonymous</span>}
              </div>
              {accounts[r.url] && (
                <>
                  <AccountLine account={accounts[r.url]} onUpgrade={() => wrap(() => SettingsAPI.OpenUpgrade(r.url))} />
                  <OrgLine user={accounts[r.url].user} />
                </>
              )}
            </div>
            <div className="row nowrap">
              {r.url !== activeRegistry && (
                <Button onClick={() => onSelectRegistry(r.url)}>Use</Button>
              )}
              {!r.isDefault && (
                <Button disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.SetDefault(r.url); await onChanged(); })}>
                  Make default
                </Button>
              )}
              {r.hasToken ? (
                <Button disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.SignOut(r.url); await onChanged(); setNotice(`Signed out of ${r.url}.`); })}>
                  Sign out
                </Button>
              ) : (
                <>
                  <Button
                    variant="primary"
                    disabled={busy}
                    title="Opens the registry in your browser — sign in there (the directory service, MFA included) and approve the app"
                    onClick={() => wrap(async () => {
                      setBrowserWait(r.url);
                      try {
                        const res = await SettingsAPI.SignInViaBrowser(r.url);
                        setNotice(
                          `Signed in to ${r.url} as ${res.user.handle} — PAT stored in ${res.tokenStore === "keyring" ? "the OS keychain" : "the settings file (0600)"}. Signing keys: ${res.pinNote}${rewrapLine(res)}`,
                        );
                        await onChanged();
                      } catch (e) {
                        // Cancelled by the user (Cancel sign-in): a notice, not an error.
                        if (String(e).includes("cancelled")) {
                          setNotice(`Sign-in to ${r.url} cancelled.`);
                          return;
                        }
                        throw e;
                      } finally {
                        setBrowserWait(null);
                      }
                    })}
                  >
                    Sign in via browser
                  </Button>
                  <Button disabled={busy} onClick={() => setSignIn({ url: r.url, token: "" })}>
                    Paste token
                  </Button>
                </>
              )}
              <Button variant="danger" disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.RemoveRegistry(r.url); await onChanged(); })}>
                Remove
              </Button>
            </div>
          </div>
        </Card>
      ))}
      </section>
      {/* The account's secrets for runs In Agentry, where the active
          registry keeps them (agentry-desktop#300). */}
      {activeRegistry && <SecretsCard registry={activeRegistry} />}

      <section id="settings-part-add-registry" className="settings-part">
      <h2>Add a registry</h2>
      <Toolbar>
        <Input
          type="text"
          className="grow mono"
          placeholder="https://secureagentry.net — or http://agentry:8080 for a dev stack"
          value={newURL}
          onChange={(e) => setNewURL(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && newURL.trim() && addRegistry()}
        />
        <Button variant="primary" disabled={busy || !newURL.trim()} onClick={addRegistry}>Add</Button>
      </Toolbar>
      </section>

      {signIn && (
        <Modal title={`Sign in to ${signIn.url}`} onClose={() => setSignIn(null)}>
            <p className="muted">
              Paste a personal access token. Create one on the registry's site under Settings →
              Authentication (<span className="mono">/ui/account/tokens</span> on a registry without the
              site), or with <span className="mono">agentry login</span>. Signing in also pins the registry's
              signing keys on first use (TOFU).
            </p>
            <Input
              type="password"
              style={{ width: "100%" }}
              placeholder="agy_…"
              value={signIn.token}
              onChange={(e) => setSignIn({ ...signIn, token: e.target.value })}
              onKeyDown={(e) => e.key === "Enter" && signIn.token.trim() && doSignIn()}
              autoFocus
            />
            <div className="row" style={{ marginTop: 14, justifyContent: "flex-end" }}>
              <Button onClick={() => setSignIn(null)}>Cancel</Button>
              <Button variant="primary" disabled={busy || !signIn.token.trim()} onClick={doSignIn}>
                {busy ? "Checking…" : "Sign in"}
              </Button>
            </div>
        </Modal>
      )}
    </>
  );
}

// TIER_LABEL is the product's name for each tier — the two a person may
// hold and the two an organization may (plan
// organization-subscriptions.md §4).
const TIER_LABEL: Record<string, string> = {
  standard: "Standard",
  pro: "Pro",
  business: "Business",
  enterprise: "Enterprise",
};

const tierLabel = (t: string) => TIER_LABEL[t] ?? t;

// STANDING_NOTE glosses an organization's standing with the consequence
// the registry's own refusal gives it, so "restricted" reaches a person as
// what it means rather than as a column value.
const STANDING_NOTE: Record<string, string> = {
  active: "active",
  pending: "awaiting payment — nothing is admitted until the subscription starts",
  restricted: "the subscription has lapsed — nothing is admitted until it is paid again",
  decommissioning: "the organization is closing and admits nothing",
};

// creditsPhrase says what this account has to spend. A member spends from
// the organization's shared pool rather than their own credits, so the
// word changes with the thing.
//
// The order of the tests matters: a standing that is not active is
// answered first, because the extension grants an allowance only while
// the organization is paid for, and an unmetered organization that has
// lapsed still admits nothing — saying "not metered" there would promise
// work the registry will refuse.
function creditsPhrase(sub: main.SubscriptionView): string {
  const reset = `(a submission costs ${sub.submissionCost}; resets on ${dayOf(sub.periodEnd)})`;
  if (!sub.memberOf) return `${sub.balance} of ${sub.allowance} credits ${reset}`;
  if (sub.standing && sub.standing !== "active") {
    return `pool ${sub.balance} credits, and no allowance while the subscription is not active`;
  }
  if (sub.unmetered) return "pool not metered";
  return `pool ${sub.balance} of ${sub.allowance} credits ${reset}`;
}

// OrgLine: the memberships the registry reports. They are the core's own
// facts, so a registry that sells nothing still says which organization a
// person belongs to and in what role; a membership an admin has yet to
// approve says so and names the door, because until then they hold no
// seat and spend from their own account.
function OrgLine({ user }: { user: main.UserView }) {
  if (!user.orgs?.length) return null;
  return (
    <>
      {user.orgs.map((o) => {
        const pending = o.state === "pending";
        const via = user.pendingOrg?.handle === o.handle ? user.pendingOrg.via : "";
        return (
          <div key={o.handle} className="muted small">
            organization <span className="selectable">{o.handle}</span> · {o.role}
            {pending && (
              <>
                {" "}· <span className="badge warn">pending</span>{" "}
                {via === "domain"
                  ? "your address is at the organization's domain; an admin has yet to approve you"
                  : via === "scim"
                    ? "the organization's directory provisioned you; an admin has yet to approve you"
                    : "an admin has yet to approve you"}
              </>
            )}
          </div>
        );
      })}
    </>
  );
}

// AccountLine: the handle and email of the credential and, where the
// registry has tiers, the subscription. For a member of an organization
// that is the organization's: the tier names it, the balance is the
// shared pool's, and the seats and the standing come with them. Manage
// opens the page the registry named — its console for a member, the
// subscription page for a person — and the app takes no payment.
function AccountLine({ account, onUpgrade }: { account: main.AccountView; onUpgrade: () => void }) {
  const sub = account.subscription;
  return (
    <div className="muted small" style={{ marginTop: 4 }}>
      <span className="selectable">{account.user.handle}</span>
      {account.user.email && <> · <span className="selectable">{account.user.email}</span></>}
      {sub && (
        <>
          {" "}· {tierLabel(sub.tier)}
          {sub.memberOf && <>, through <span className="selectable">{sub.memberOf}</span></>}
          {" "}· {creditsPhrase(sub)}
          {sub.seats && <> · {sub.seats.max > 0 ? `${sub.seats.used} of ${sub.seats.max} seats` : `${sub.seats.used} seats (no cap)`}</>}
          {sub.standing && <> · {STANDING_NOTE[sub.standing] ?? sub.standing}</>}
          {sub.pendingTier && <> · {tierLabel(sub.pendingTier)} from {dayOf(sub.periodEnd)}</>}
          {sub.upgrade?.url && (
            <>
              {" "}
              <Button
                onClick={onUpgrade}
                title={
                  sub.memberOf
                    ? "Opens the organization's console on the registry's site in your browser"
                    : "Opens the subscription page of the registry's site in your browser"
                }
              >
                {sub.upgrade.available ? "Upgrade" : sub.upgrade.manage ? "Manage billing" : "The subscription"}
              </Button>
            </>
          )}
        </>
      )}
      {!sub && account.subscriptionError && <> · subscription not read: {account.subscriptionError}</>}
    </div>
  );
}

// dayOf renders an RFC 3339 instant as its UTC day, the way the site does.
export function dayOf(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
}



// SelfUpdateCard shows whether a newer Agentry Desktop exists and lets the
// person stop the app looking (agentry-desktop#87).
//
// Notify only, deliberately: there is no Install button here. Applying an
// update is M7 on Windows and macOS, and on Linux it stays the package
// manager's job (D7) — so the card says where the release is and leaves the
// choice where it already was.
function SelfUpdateCard() {
  const [view, setView] = useState<main.SelfUpdateView | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    SelfUpdateAPI.SelfUpdateStatus().then(setView).catch(() => setView(null));
  }, []);

  const run = async (fn: () => Promise<main.SelfUpdateView>) => {
    setBusy(true);
    try {
      setView(await fn());
    } catch {
      // The card keeps what it had; a failed read is not worth a dialog.
    } finally {
      setBusy(false);
    }
  };

  if (!view) return null;

  // Installing needs a platform that installs AND a copy this person can
  // replace; the Go side has already decided both.
  const canInstall = view.canApply && !view.available?.retired;

  // A build anyone can make from the open core has no channel to check, which
  // is a fact to state rather than a failure to report.
  if (!view.channel) {
    return (
      <Card>
        <div className="muted small">
          This build has no update channel, so it does not check for a newer Agentry Desktop.
          Builds from the open core are like this; the ones on the download page are not.
        </div>
      </Card>
    );
  }

  return (
    <Card>
      <label className="row gap">
        <input
          type="checkbox"
          checked={view.enabled}
          disabled={busy}
          onChange={(e) => run(() => SelfUpdateAPI.SetSelfUpdateCheckEnabled(e.target.checked))}
        />
        <span>Check for newer versions</span>
      </label>
      <div className="muted small">
        On launch and once a day, from <span className="mono">{view.channel}</span>. The signature on
        a release is checked against the key this build carries before anything is shown.
      </div>

      {view.available && (
        <Notice>
          <strong>Agentry Desktop {view.available.version} is available.</strong>{" "}
          You are running {view.current}. Signed with the {view.available.signedBy} key
          {view.available.retired && <> — which has since been retired, so this release is genuine but out of date and will not be installed</>}
          {"."}
          {/* Installing is offered only where the app can do it: not on Linux,
              where the package manager owns the installed files (D7), and not
              on a copy an administrator installed for everyone — there the
              card explains instead of offering a button that would fail. */}
          {canInstall && (
            <div className="row gap install-row">
              <Button variant="primary" disabled={busy} onClick={() => run(SelfUpdateAPI.InstallSelfUpdate)}>
                {view.applying ? "Installing…" : "Install and relaunch"}
              </Button>
              <span className="muted small">The app closes and reopens on the new version.</span>
            </div>
          )}
          {!canInstall && view.cannotApplyWhy && (
            <div className="muted small">{view.cannotApplyWhy} Download it instead:{" "}
              <span className="mono selectable">{view.downloadUrl}</span>.
            </div>
          )}
          {!canInstall && !view.cannotApplyWhy && (
            <div className="muted small">
              {view.downloadUrl
                ? <>Get it from <span className="mono selectable">{view.downloadUrl}</span>.</>
                : <>Get it from the site you downloaded this copy from.</>}{" "}
              On this platform a newer version is installed by your package manager, not by the app.
            </div>
          )}
        </Notice>
      )}

      {canInstall && (
        <label className="row gap">
          <input
            type="checkbox"
            checked={view.onQuit}
            disabled={busy}
            onChange={(e) => run(() => SelfUpdateAPI.SetInstallOnQuit(e.target.checked))}
          />
          <span>Install automatically when I quit</span>
        </label>
      )}

      {view.err && <ErrorBox error={`The last check did not complete: ${view.err}`} />}

      {view.nothing && (
        <div className="muted small">
          This channel publishes no signed release yet, so there is nothing to compare against.
        </div>
      )}

      {view.enabled && !view.available && !view.err && !view.nothing && view.checked && (
        <div className="muted small">No newer version when last checked.</div>
      )}

      <Toolbar>
        <Button disabled={busy || !view.enabled} onClick={() => run(SelfUpdateAPI.SelfUpdateCheckNow)}>
          {busy ? "Checking…" : "Check now"}
        </Button>
      </Toolbar>
    </Card>
  );
}
