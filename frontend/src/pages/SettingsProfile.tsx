import React, { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Notice, fmtStorage } from "../ui";
import { capsSentence } from "../usage";
import { NOT_HERE_YET, REPORT_THEME_LOOKS, themeSavedSentence } from "../reportPdf";
import {
  bonusOf, changesPhrase, creditCount, creditNumber, creditsPhrase, dayOf, owedSentence, packBonus, packLabel, packPrice, runningPromo, tierLabel,
  topUpOffer,
} from "../subscription";

// Settings → Profile (agentry-desktop#423): what the site's
// /settings/profile shows in its Profile and Subscription cards, for the
// active registry's stored credential, in the site's words. The landing
// offers the card only while that registry holds a token. The app writes
// the display name and nothing else: buying, the billing portal, cancelling
// and deleting the account are the site's session's alone, so those open
// the browser.

// The field's limit is the registry's (MaxDisplayName in the Go host).
const MAX_DISPLAY_NAME = 100;

// Never below zero: a suspended account's balance is (agentry-desktop#511),
// and "-40 submissions left" says nothing a person can act on.
const submissionsLeft = (s: main.SubscriptionView) => (s.submissionCost > 0 ? Math.max(0, Math.floor(s.balance / s.submissionCost)) : 0);

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
      <ReportThemePart registry={registry} />
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

// ReportThemePart is the account's report theme (plan
// scratch-and-pdf-reports.md D18, D21; agentry-desktop#730): how the PDFs a
// run exports are laid out, the same setting as the site's Profile card. A
// local run from this app takes it; "Not chosen" is Plain here. Drawn only
// where the registry keeps the setting. Since the account keeps a report
// design (agentry#1026), the card offers the core's built-ins, which Go
// names, and saves one whole; a design edited on the site is said, since
// saving here replaces it (agentry-desktop#766, until #759's editor).
function ReportThemePart({ registry }: { registry: string }) {
  const [view, setView] = useState<main.ReportThemeView | null>(null);
  const [choice, setChoice] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    SettingsAPI.ReportTheme(registry)
      .then((v) => { if (live) { setView(v); setChoice(v.theme ?? ""); } })
      .catch((e) => { if (live) setErr(String(e)); });
    return () => { live = false; };
  }, [registry]);

  if (view && !view.available) return null;
  const save = async () => {
    setBusy(true);
    setErr(null);
    setSaved(null);
    try {
      const v = await SettingsAPI.SetReportTheme(registry, choice);
      setView(v);
      setChoice(v.theme ?? "");
      const c = v.choices?.find((x) => x.id === v.theme);
      setSaved(themeSavedSentence(v.theme ?? "", c?.name, c?.here ?? true));
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section id="settings-part-report-theme" className="settings-part">
      <h2>Report theme</h2>
      <Card>
        <p className="muted small">
          How the PDFs your runs export are laid out. A run makes a PDF when you ask for one, or when its report is
          meant for someone else. The QuickAgent never picks the theme, and asking it for a formal tone changes its
          words, not this.
        </p>
        {!view && !err && <p className="muted small">Reading your report theme…</p>}
        <ErrorBox error={err} />
        {view && (
          <fieldset className="report-themes" disabled={busy}>
            <legend className="sr-only">Report theme</legend>
            <label className="row">
              <input type="radio" name="report-theme" value="" checked={choice === ""} onChange={() => setChoice("")} />
              <span>Not chosen <span className="muted small">Plain, for runs from this app.</span></span>
            </label>
            {(view.choices ?? []).map((t) => (
              <label key={t.id} className="row" data-report-theme={t.id}>
                <input type="radio" name="report-theme" value={t.id} checked={choice === t.id} onChange={() => setChoice(t.id)} />
                <span>
                  {t.name} <span className="muted small">{REPORT_THEME_LOOKS[t.id] ?? ""}</span>
                  {!t.here && <span className="muted small" data-not-here> {NOT_HERE_YET}</span>}
                </span>
              </label>
            ))}
          </fieldset>
        )}
        {view?.edited && (
          <Notice tone="info">
            Your report design was changed on the site. It is shown here by the theme it is based on, and saving a theme
            here replaces it with that theme as it ships.
          </Notice>
        )}
        {view && (
          <Button busy={busy} busyLabel="Saving…" disabled={choice === (view.theme ?? "")} onClick={() => void save()}>
            Save report theme
          </Button>
        )}
        {saved && <Notice tone="ok">{saved}</Notice>}
      </Card>
    </section>
  );
}

function SubscriptionPart({ profile, registry, onError }: { profile: main.ProfileView; registry: string; onError: (e: string) => void }) {
  const sub = profile.subscription;
  if (!sub) {
    return <Notice tone="warn">The subscription could not be read: {profile.subscriptionError} Open Settings again to retry.</Notice>;
  }
  const open = () => SettingsAPI.OpenUpgrade(registry).catch((e) => onError(String(e)));
  // Buy, and the owed notice's link, open the site's Top up credits card:
  // the app takes no money (plan credit-top-ups.md D7). The registry names
  // the page and the Go side checks it is on the registry's own origin.
  const openTopUp = () => SettingsAPI.OpenTopUp(registry).catch((e) => onError(String(e)));
  return sub.memberOf
    ? <MemberView sub={sub} runs={profile.hostedRuns} onOpen={open} />
    : <PersonView sub={sub} runs={profile.hostedRuns} onOpen={open} onTopUp={openTopUp} />;
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
function PersonView({ sub, runs, onOpen, onTopUp }: { sub: main.SubscriptionView; runs?: main.HostedRunsTermsView; onOpen: () => void; onTopUp: () => void }) {
  const promo = runningPromo(sub);
  const topUp = topUpOffer(sub);
  const left = submissionsLeft(sub);
  const limit = sub.knowledge ? knowledgeLimitSentence(sub.knowledge) : null;
  return (
    <>
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
          {changesPhrase(sub) && <Row label="Changes">{changesPhrase(sub)}</Row>}
        </dl>
        {limit && <Notice tone="warn">{limit}</Notice>}
        {sub.suspended && (
          <Notice tone="warn">
            <strong>This account is suspended for non-payment.</strong> {owedSentence(sub.suspended.owed)}{" "}
            {sub.topUp?.url && (
              <button type="button" className="link" onClick={onTopUp}>
                Top up credits on the site
              </button>
            )}
          </Notice>
        )}
        {promo && (
          <Notice tone="info">
            Your promo credits expire on {dayOf(promo.endsAt || sub.periodEnd)}, and this account moves to Standard then. You will not be charged.
          </Notice>
        )}
        <p className="muted small">
          {promo
            ? "Promo credits are the whole of this account's credits until the trial ends; Standard's monthly allowance starts then."
            : sub.bought
              ? "Unused monthly credits expire when the allowance resets; bought credits never do."
              : "Unused credits expire when the allowance resets."}{" "}
          A submission the pipeline itself fails is refunded; one the gate sends to review is not.
        </p>
        <UpgradeButton sub={sub} onOpen={onOpen} />
      </Card>
      {topUp && <TopUpCard sub={sub} topUp={topUp} onBuy={onTopUp} />}
    </>
  );
}

// TopUpCard is the site's Top up credits card (plan credit-top-ups.md
// §4.4, D6, D7; agentry-desktop#511): each pack's base number struck
// through and the credits it brings larger and bolder, its bonus, the
// provider's price from the registry, and a Buy that opens the site's card
// in the browser, where the purchase is made. A screen reader hears one
// sentence per pack (packLabel); the drawn numbers are hidden from it.
function TopUpCard({ sub, topUp, onBuy }: { sub: main.SubscriptionView; topUp: main.TopUpView; onBuy: () => void }) {
  return (
    <Card className="top-up">
      <p><strong>Top up credits</strong></p>
      <p className="small">
        A pack&apos;s credits are spent after {runningPromo(sub) ? "your promo credits" : "your monthly credits"} and never expire.
        {sub.bought ? ` This account holds ${creditNumber(sub.bought)} bought credits.` : ""}
      </p>
      <ul className="top-up-packs">
        {topUp.packs.map((p) => {
          const bonus = packBonus(p);
          const price = packPrice(p);
          return (
            <li key={p.id} className="top-up-pack">
              <p>
                <span className="sr-only">{packLabel(p)}</span>
                <span aria-hidden="true">
                  {bonusOf(p) > 0 && <><s className="muted">{creditNumber(p.baseCredits)}</s>{" "}</>}
                  <span className="top-up-credits">{creditNumber(p.credits)}</span> credits
                </span>
              </p>
              {bonus && <p className="muted small" aria-hidden="true">{bonus}</p>}
              <p className="row">
                <Button
                  variant="primary"
                  disabled={topUp.processing}
                  onClick={onBuy}
                  aria-label={`Buy ${creditNumber(p.credits)} credits for ${price} on the site`}
                  title="Opens the Top up credits card on the registry's site in your browser"
                >
                  Buy
                </Button>
                <span>{price}</span>
              </p>
            </li>
          );
        })}
      </ul>
      {topUp.processing && <p className="small">A pack&apos;s payment is still clearing. Another can be bought once it has.</p>}
      <p className="muted small">Buy opens the card on the site in your browser, where the payment is taken; this app never takes money or sees your card.</p>
    </Card>
  );
}

// MemberView is the site's member view: the organization pays for this
// person's work from its pool, and their own account is not in force while
// they belong to it.
function MemberView({ sub, runs, onOpen }: { sub: main.SubscriptionView; runs?: main.HostedRunsTermsView; onOpen: () => void }) {
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
