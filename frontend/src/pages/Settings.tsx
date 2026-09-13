import { useEffect, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, PageHeader, Toolbar } from "../ui";
import { RuntimeSetup } from "./RuntimeSetup";

export default function Settings({
  settings, activeRegistry, onChanged, onSelectRegistry, onRunSetup,
}: {
  settings: main.SettingsView | null;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
  // Re-open the first-run guided setup. Optional so the page renders in the
  // harness without the App shell (agentry-desktop#41).
  onRunSetup?: () => void;
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
    const signedIn = (settings?.registries ?? []).filter((r) => r.hasToken);
    Promise.all(signedIn.map(async (r) => [r.url, await SettingsAPI.Account(r.url).catch(() => null)] as const)).then((pairs) => {
      if (cancelled) return;
      const next: Record<string, main.AccountView> = {};
      for (const [url, acct] of pairs) if (acct) next[url] = acct;
      setAccounts(next);
    });
    return () => { cancelled = true; };
  }, [settings?.registries]);

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
        `Signed in to ${signIn.url} as ${res.user.handle} — PAT stored in ${res.tokenStore === "keyring" ? "the OS keychain" : "the settings file (0600; no keychain available)"}. Signing keys: ${res.pinNote}`,
      );
      setSignIn(null);
      await onChanged();
    });

  if (!settings) return <div className="empty">Loading settings…</div>;

  return (
    <div>
      <PageHeader
        title="Settings"
        sub={<>Registries this app talks to. Credentials live in the OS keychain under the same convention the <span className="mono">agentry</span> CLI uses, so a sign-in here works there too.</>}
        actions={onRunSetup && <Button onClick={onRunSetup}>Run setup again</Button>}
      />

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
                <AccountLine account={accounts[r.url]} onUpgrade={() => wrap(() => SettingsAPI.OpenUpgrade(r.url))} />
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
                          `Signed in to ${r.url} as ${res.user.handle} — PAT stored in ${res.tokenStore === "keyring" ? "the OS keychain" : "the settings file (0600)"}. Signing keys: ${res.pinNote}`,
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
      <p className="muted small selectable">
        Settings file: <span className="mono">{settings.configPath}</span>. Install records and pinned
        signing keys are shared with the CLI under the same directory.
      </p>

      <RuntimeSetup />

      {signIn && (
        <Modal title={`Sign in to ${signIn.url}`} onClose={() => setSignIn(null)}>
            <p className="muted">
              Paste a personal access token (create one in the web app under Settings → Tokens,
              or with <span className="mono">agentry login</span>). Signing in also pins the registry's
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
    </div>
  );
}

// AccountLine: the handle and email of the credential and, where the
// registry has tiers, the subscription; Upgrade opens the site in the
// browser — the app takes no payment.
function AccountLine({ account, onUpgrade }: { account: main.AccountView; onUpgrade: () => void }) {
  const sub = account.subscription;
  const tier = sub ? (sub.tier === "pro" ? "Pro" : "Standard") : null;
  return (
    <div className="muted small" style={{ marginTop: 4 }}>
      <span className="selectable">{account.user.handle}</span>
      {account.user.email && <> · <span className="selectable">{account.user.email}</span></>}
      {sub && (
        <>
          {" "}· {tier} · {sub.balance} of {sub.allowance} credits (a submission costs {sub.submissionCost}; resets on {dayOf(sub.periodEnd)})
          {sub.pendingTier && <> · {sub.pendingTier === "pro" ? "Pro" : "Standard"} from {dayOf(sub.periodEnd)}</>}
          {sub.upgradeUrl && (
            <>
              {" "}
              <Button onClick={onUpgrade} title="Opens the subscription page of the registry's site in your browser">
                Upgrade
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

