import React, { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { main, hostedrun } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Notice, fmtDuration, fmtStorage } from "../ui";

// Settings → Profile (agentry-desktop#423): what the site's
// /settings/profile shows in its Profile and Subscription cards, for the
// active registry's stored credential, in the site's words. The landing
// offers the card only while that registry holds a token. The app writes
// the display name and nothing else: buying, the billing portal, cancelling
// and deleting the account are the site's session's alone, so those open
// the browser.

// TIER_LABEL is the product's name for each tier: the two a person may
// hold, the two an organization may (plan organization-subscriptions.md §4),
// and the promo trial's (plan promo-codes.md §4.1; agentry-desktop#415).
const TIER_LABEL: Record<string, string> = {
  standard: "Standard",
  pro: "Pro",
  promo_pro: "Promo Pro",
  business: "Business",
  enterprise: "Enterprise",
};

const tierLabel = (t: string) => TIER_LABEL[t] ?? t;

// The field's limit is the registry's (MaxDisplayName in the Go host).
const MAX_DISPLAY_NAME = 100;

function dayOf(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
}

const creditCount = (n: number) => (n === 1 ? "1 credit" : `${n} credits`);

const runningPromo = (s: main.SubscriptionView) => (s.promo?.state === "running" ? s.promo : null);

// creditsPhrase: what the account has to spend; during a promo trial the
// credits are the code's.
function creditsPhrase(s: main.SubscriptionView): string {
  const promo = runningPromo(s);
  if (promo && promo.credits) return `${s.balance} of ${promo.credits} promo credits`;
  return `${s.balance} of ${s.allowance} credits`;
}

const submissionsLeft = (s: main.SubscriptionView) => (s.submissionCost > 0 ? Math.floor(s.balance / s.submissionCost) : 0);

// privateAgentsPhrase: "none" where the registry disables them, with the
// count still held; "4 of no cap" where max is 0; "4 of 10" otherwise.
function privateAgentsPhrase(p: { max: number; used: number; enabled?: boolean }): string {
  if (p.enabled === false) return p.used > 0 ? `none may be added; ${p.used} held` : "none";
  return `${p.used} of ${p.max === 0 ? "no cap" : p.max}`;
}

function knowledgePhrase(k: main.KnowledgeAllocation): string {
  if (!k.enabled) return k.bytes > 0 ? `none may be added; ${fmtStorage(k.bytes)} held` : "none";
  return `${fmtStorage(k.bytes)} of ${k.max === 0 ? "no cap" : fmtStorage(k.max)}`;
}

// knowledgeLimitSentence: what a full, overdrawn or disabled allocation
// means; nothing while there is room, or with no cap.
function knowledgeLimitSentence(k: main.KnowledgeAllocation, memberOf?: string): string | null {
  const kept = "Nothing is deleted: every pack kept here stays readable and can be exported.";
  if (!k.enabled) {
    const payer = memberOf ? `${memberOf}'s subscription` : "the subscription that pays for it";
    return `No knowledge pack may be added to Agentry or grown while ${payer} is not in force. ${kept}`;
  }
  if (k.max <= 0 || k.bytes < k.max) return null;
  const whose = memberOf ? `${memberOf}'s pool` : "your allocation";
  const head = k.bytes > k.max
    ? `The packs counted against ${whose} take ${fmtStorage(k.bytes)}, more than its ${fmtStorage(k.max)}.`
    : `${memberOf ? `${memberOf}'s pool` : "Your allocation"} of ${fmtStorage(k.max)} is full.`;
  const grows = memberOf ? `${memberOf} adds seats` : "you upgrade";
  return `${head} ${kept} Agentry refuses a new pack, or a larger copy of one, until enough is deleted or ${grows}. Delete a pack, or keep a new one on your computer.`;
}

const fmtNumber = (n: number) => n.toLocaleString("en-US");

// capsSentence: the caller's hosted-run caps and the rates a run settles at.
function capsSentence(c: hostedrun.Caller): string {
  return (
    `A hosted run holds ${c.holdCredits} credits for its cap — ${fmtDuration(c.caps.timeoutSeconds)}, ` +
    `${c.caps.maxTurns} turns, ${fmtNumber(c.caps.maxTokens)} tokens — and settles from the tokens it uses: ` +
    `one credit for every ${fmtNumber(c.inputTokensPerCredit)} tokens the model reads and every ${fmtNumber(c.outputTokensPerCredit)} it writes.`
  );
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
          <p key={o.handle} className="muted small">
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
          </p>
        );
      })}
    </>
  );
}

export function ProfileSettings({ registry, signedIn, onOpenRegistry }: {
  registry: string;
  signedIn: boolean;
  onOpenRegistry: () => void;
}) {
  const [profile, setProfile] = useState<main.ProfileView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [saved, setSaved] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!signedIn) return;
    let live = true;
    SettingsAPI.Profile(registry)
      .then((p) => { if (live) { setProfile(p); setName(p.user.displayName ?? ""); } })
      .catch((e) => { if (live) setError(String(e)); });
    return () => { live = false; };
  }, [registry, signedIn]);

  if (!signedIn) {
    return (
      <Notice tone="info">
        Sign in to <span className="mono">{registry}</span> to see your profile and subscription.{" "}
        <Button variant="link" onClick={onOpenRegistry}>Registry settings</Button>
      </Notice>
    );
  }
  if (error && !profile) return <ErrorBox error={error} />;
  if (!profile) return <div className="empty">Loading your profile…</div>;

  const user = profile.user;
  const save = async () => {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      const u = await SettingsAPI.SetDisplayName(registry, name);
      // A plain object, as every binding answers one: rebuilding the
      // generated class with createFrom threw "null is not a constructor" in
      // the production bundle after a save the registry had taken
      // (agentry-desktop#423's live check).
      setProfile((cur) => (cur ? ({ ...cur, user: u } as main.ProfileView) : cur));
      setName(u.displayName ?? "");
      setSaved("Display name saved.");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <ErrorBox error={error} />
      <section id="settings-part-profile" className="settings-part">
        <h2>Profile</h2>
        <Card>
          <p className="muted small selectable">
            {user.handle} · {user.email} {user.emailVerified ? "(verified)" : "(unverified)"}
          </p>
          {profile.signedUpWith && <p className="muted small">Signed up with {profile.signedUpWith}</p>}
          <OrgLine user={user} />
          <label className="field">
            <span className="field-label">Display name</span>
            <div className="row">
              <Input type="text" className="grow" value={name} maxLength={MAX_DISPLAY_NAME} disabled={busy} aria-label="Display name"
                onChange={(e) => setName(e.target.value)} />
              <Button disabled={busy || name.trim() === (user.displayName ?? "")} onClick={() => void save()}>
                {busy ? "Saving…" : "Save profile"}
              </Button>
            </div>
          </label>
          {saved && <Notice tone="ok">{saved}</Notice>}
          <p className="muted small">The sign-in service holds your handle and email, and this app cannot change them.</p>
        </Card>
      </section>
      {/* The Subscription part is drawn where the registry sells
          subscriptions, as the site's is: a registry without billing has
          none (the site's SubscriptionPart returns nothing there). */}
      {(profile.subscription || profile.subscriptionError) && (
        <section id="settings-part-subscription" className="settings-part">
          <h2>Subscription</h2>
          <SubscriptionPart profile={profile} registry={registry} onError={setError} />
        </section>
      )}
    </>
  );
}

function SubscriptionPart({ profile, registry, onError }: { profile: main.ProfileView; registry: string; onError: (e: string) => void }) {
  const sub = profile.subscription;
  if (!sub) {
    return <Notice tone="warn">The subscription could not be read: {profile.subscriptionError} Open Settings again to retry.</Notice>;
  }
  const open = () => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)));
  return sub.memberOf
    ? <MemberView sub={sub} runs={profile.hostedRuns} onOpen={open} />
    : <PersonView sub={sub} runs={profile.hostedRuns} onOpen={open} />;
}

const Row = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <>
    <dt>{label}</dt>
    <dd>{children}</dd>
  </>
);

// UpgradeButton opens the page the registry named for this subscription,
// in the browser; the app takes no payment.
function UpgradeButton({ sub, onOpen }: { sub: main.SubscriptionView; onOpen: () => void }) {
  if (!sub.upgrade?.url) return null;
  return (
    <p className="row">
      <Button
        variant={sub.upgrade.available ? "primary" : undefined}
        onClick={onOpen}
        title={sub.memberOf ? "Opens the organization's console on the registry's site in your browser" : "Opens the subscription page of the registry's site in your browser"}
      >
        {sub.upgrade.available ? "Upgrade" : sub.upgrade.manage ? "Manage billing" : "The subscription"}
      </Button>
      <span className="muted small">Buying, billing and cancelling open in your browser.</span>
    </p>
  );
}

// PersonView is the site's personal Subscription card.
function PersonView({ sub, runs, onOpen }: { sub: main.SubscriptionView; runs?: hostedrun.Caller; onOpen: () => void }) {
  const promo = runningPromo(sub);
  const left = submissionsLeft(sub);
  const limit = sub.knowledge ? knowledgeLimitSentence(sub.knowledge) : null;
  return (
    <Card>
      <p>
        <span className={`badge ${sub.tier === "standard" ? "" : "ok"}`}>{tierLabel(sub.tier)}</span>
        {sub.tier === "pro" && sub.tierSince && <span className="muted small"> This account has been on Pro since {dayOf(sub.tierSince)}.</span>}
      </p>
      <dl className="profile-facts">
        <Row label="Credits">{creditsPhrase(sub)} · {left} submission{left === 1 ? "" : "s"} left this cycle</Row>
        <Row label="A submission costs">{creditCount(sub.submissionCost)}, spent when the registry accepts it</Row>
        {promo
          ? <Row label="Promo trial">code <span className="mono">{promo.code}</span>, {creditCount(promo.credits ?? 0)} granted; until {dayOf(promo.endsAt || sub.periodEnd)}</Row>
          : <Row label="Allowance">{sub.allowance} credits a month; resets on {dayOf(sub.periodEnd)}</Row>}
        <Row label="Private QuickAgents">{privateAgentsPhrase(sub.privateAgents)}</Row>
        {sub.knowledge && <Row label="Knowledge storage">{knowledgePhrase(sub.knowledge)}</Row>}
        {runs && <Row label="Hosted runs">{capsSentence(runs)}</Row>}
        {sub.pendingTier && <Row label="Changes">{tierLabel(sub.pendingTier)} from {dayOf(sub.periodEnd)}</Row>}
      </dl>
      {limit && <Notice tone="warn">{limit}</Notice>}
      {promo && (
        <Notice tone="info">
          Your promo credits expire on {dayOf(promo.endsAt || sub.periodEnd)}, and this account moves to Standard then. You will not be charged.
        </Notice>
      )}
      <p className="muted small">
        {promo
          ? "Promo credits are the whole of this account's credits until the trial ends; Standard's monthly allowance starts then."
          : "Unused credits expire when the allowance resets."}{" "}
        A submission the pipeline itself fails is refunded; one the gate sends to review is not.
      </p>
      <UpgradeButton sub={sub} onOpen={onOpen} />
    </Card>
  );
}

// MemberView is the site's member view: the organization pays for this
// person's work from its pool, and their own account is not in force while
// they belong to it.
function MemberView({ sub, runs, onOpen }: { sub: main.SubscriptionView; runs?: hostedrun.Caller; onOpen: () => void }) {
  const org = sub.memberOf!;
  const lapsed = sub.standing && sub.standing !== "active";
  const limit = sub.knowledge ? knowledgeLimitSentence(sub.knowledge, org) : null;
  return (
    <>
      <Card>
        <p>
          <span className="badge ok">{tierLabel(sub.tier)}</span>
          {lapsed && <> <span className="badge warn">{sub.standing}</span></>}
        </p>
        <p className="small">
          <strong className="selectable">{org}</strong> covers your work: as its member you have everything Pro has, and every
          submission and hosted run of yours — in its namespace or your own — is paid from its pool.
        </p>
        <dl className="profile-facts">
          <Row label="The pool">
            {sub.unmetered ? "not metered: nothing is debited" : `${creditsPhrase(sub)} this cycle; resets on ${dayOf(sub.periodEnd)}`}
          </Row>
          <Row label="A submission costs">{creditCount(sub.submissionCost)}, spent when the registry accepts it</Row>
          <Row label="Private QuickAgents">{privateAgentsPhrase(sub.privateAgents)}</Row>
          {sub.knowledge && <Row label="Knowledge storage">{knowledgePhrase(sub.knowledge)}, in {org}'s pool</Row>}
          {runs && <Row label="Hosted runs">{capsSentence(runs)}</Row>}
        </dl>
        {limit && <Notice tone="warn">{limit}</Notice>}
        {lapsed && (
          <Notice tone="warn">
            {sub.standing === "pending"
              ? `${org} is awaiting payment: until its subscription starts nothing is admitted from its members.`
              : `${org}'s subscription has lapsed: until it is paid again nothing is admitted from its members; everything stays readable.`}{" "}
            Its administrators see the details on its console.
          </Notice>
        )}
        <UpgradeButton sub={sub} onOpen={onOpen} />
      </Card>
      <Card>
        <p><strong>Your own account</strong></p>
        <p className="small">
          While you belong to {org} your own subscription is not in force: the credits you had joined its pool when you were
          seated, and any Pro subscription of your own was cancelled then and is not charged again. If you leave the
          organization you go back to your own account and its tier.
        </p>
        <p className="muted small">The organization's seats and billing are its administrators'.</p>
      </Card>
    </>
  );
}
