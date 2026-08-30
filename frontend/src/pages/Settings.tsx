import { useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import { main } from "../../wailsjs/go/models";
import { ErrorBox } from "../ui";

export default function Settings({
  settings, activeRegistry, onChanged, onSelectRegistry,
}: {
  settings: main.SettingsView | null;
  activeRegistry: string;
  onChanged: () => Promise<main.SettingsView>;
  onSelectRegistry: (url: string) => void;
}) {
  const [newURL, setNewURL] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [signIn, setSignIn] = useState<{ url: string; token: string } | null>(null);
  const [busy, setBusy] = useState(false);

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
      <h1>Settings</h1>
      <p className="page-sub">
        Registries this app talks to. Credentials live in the OS keychain under the same
        convention the <span className="mono">agentry</span> CLI uses, so a sign-in here works there too.
      </p>

      <ErrorBox error={error} />
      {notice && <div className="notice selectable">{notice}</div>}

      {settings.registries.map((r) => (
        <div key={r.url} className="card">
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
            </div>
            <div className="row nowrap">
              {r.url !== activeRegistry && (
                <button onClick={() => onSelectRegistry(r.url)}>Use</button>
              )}
              {!r.isDefault && (
                <button disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.SetDefault(r.url); await onChanged(); })}>
                  Make default
                </button>
              )}
              {r.hasToken ? (
                <button disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.SignOut(r.url); await onChanged(); setNotice(`Signed out of ${r.url}.`); })}>
                  Sign out
                </button>
              ) : (
                <button className="primary" disabled={busy} onClick={() => setSignIn({ url: r.url, token: "" })}>
                  Sign in
                </button>
              )}
              <button className="danger" disabled={busy} onClick={() => wrap(async () => { await SettingsAPI.RemoveRegistry(r.url); await onChanged(); })}>
                Remove
              </button>
            </div>
          </div>
        </div>
      ))}

      <h2>Add a registry</h2>
      <div className="row">
        <input
          type="text"
          className="grow mono"
          placeholder="https://secureagentry.net — or http://agentry:8080 for a dev stack"
          value={newURL}
          onChange={(e) => setNewURL(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && newURL.trim() && addRegistry()}
        />
        <button className="primary" disabled={busy || !newURL.trim()} onClick={addRegistry}>Add</button>
      </div>
      <p className="muted small selectable">
        Settings file: <span className="mono">{settings.configPath}</span>. Install records and pinned
        signing keys are shared with the CLI under the same directory.
      </p>

      {signIn && (
        <div className="modal-backdrop" onClick={() => setSignIn(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2 style={{ marginTop: 0 }}>Sign in to {signIn.url}</h2>
            <p className="muted">
              Paste a personal access token (create one in the web app under Settings → Tokens,
              or with <span className="mono">agentry login</span>). Signing in also pins the registry's
              signing keys on first use (TOFU).
            </p>
            <input
              type="password"
              style={{ width: "100%" }}
              placeholder="agentry_pat_…"
              value={signIn.token}
              onChange={(e) => setSignIn({ ...signIn, token: e.target.value })}
              onKeyDown={(e) => e.key === "Enter" && signIn.token.trim() && doSignIn()}
              autoFocus
            />
            <div className="row" style={{ marginTop: 14, justifyContent: "flex-end" }}>
              <button onClick={() => setSignIn(null)}>Cancel</button>
              <button className="primary" disabled={busy || !signIn.token.trim()} onClick={doSignIn}>
                {busy ? "Checking…" : "Sign in"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
