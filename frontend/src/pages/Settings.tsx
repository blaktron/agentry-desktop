import { useEffect, useRef, useState } from "react";
import * as SettingsAPI from "../../wailsjs/go/main/SettingsAPI";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { OpenExternal } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, PageHeader, Select, Spinner, Toolbar } from "../ui";

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
  // The registry a browser sign-in is waiting on; shows the Cancel control.
  const [browserWait, setBrowserWait] = useState<string | null>(null);

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

      <AgentRunSettings />

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
              placeholder="agentry_pat_…"
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

// The §P1 provider table, mirrored for form prefills; the Go side
// (pkg/runner/setup) holds the authoritative copy and fills any gaps.
const PROVIDERS: { id: string; model: string; apiKeyEnv: string }[] = [
  { id: "anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY" },
  { id: "openai", model: "gpt-5.2", apiKeyEnv: "OPENAI_API_KEY" },
  { id: "google", model: "gemini-3-pro", apiKeyEnv: "GEMINI_API_KEY" },
  { id: "custom", model: "", apiKeyEnv: "" },
];

/** "Agent runs" (RUNNER-PLAN-2 §P1, req 5): the LLM provider BriefAgent
 * runs use, written into the same operator policy the CLI reads
 * (~/.agentry/runner/policy.yaml), plus one-click preparation of the
 * docker MCP gateway plane. Only the env var NAME of the api key is ever
 * stored — never the key. */
function AgentRunSettings() {
  const [status, setStatus] = useState<main.SetupStatusView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [provider, setProvider] = useState("anthropic");
  const [model, setModel] = useState("");
  const [apiKeyEnv, setApiKeyEnv] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [log, setLog] = useState<string[]>([]);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    RunAPI.SetupStatus()
      .then((s) => {
        setStatus(s);
        setProvider(s.provider || "anthropic");
        setModel(s.model);
        setApiKeyEnv(s.apiKeyEnv);
        setBaseURL(s.baseUrl ?? "");
      })
      .catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    const off = EventsOn("runner:setup-progress", (line: string) => {
      setLog((prev) => [...prev, line]);
    });
    return off;
  }, []);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [log]);

  function pickProvider(id: string) {
    setProvider(id);
    const row = PROVIDERS.find((p) => p.id === id);
    if (row && row.model) {
      setModel(row.model);
      setApiKeyEnv(row.apiKeyEnv);
    }
  }

  async function apply(pullImages: boolean) {
    setBusy(pullImages ? "Preparing images — pulls and a local build can take minutes…" : "Writing policy…");
    setError(null);
    setNotice(null);
    if (pullImages) setLog([]);
    try {
      const s = await RunAPI.ApplySetup(provider, model.trim(), apiKeyEnv.trim(), baseURL.trim(), pullImages);
      setStatus(s);
      setNotice(pullImages
        ? "Setup complete — gateway, harness, and runner images ready; BriefAgents can run."
        : `Provider preference written to ${s.policyPath}.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  const needsBaseURL = provider === "custom";

  return (
    <div>
      <h2>Agent runs</h2>
      <p className="page-sub">
        BriefAgent runs happen in isolated containers with tools served by a per-run Docker MCP
        Gateway. Configure the LLM provider those runs drive — written to the same operator
        policy the <span className="mono">agentry</span> CLI uses.
      </p>

      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}

      {status && !status.dockerOk && (
        <PrereqCard
          onDone={async () => {
            try { setStatus(await RunAPI.SetupStatus()); } catch { /* badge stays */ }
          }}
        />
      )}

      {status && (
        <div className="row" style={{ marginBottom: 10 }}>
          {status.dockerOk
            ? <span className="badge ok">docker available</span>
            : <span className="badge bad" title={status.dockerErr}>docker unavailable</span>}
          {status.dockerOk && (status.imagesReady
            ? <span className="badge ok">images ready</span>
            : <span className="badge warn">images not prepared</span>)}
          {status.policyExists
            ? <span className="badge">policy: <span className="mono">{status.policyPath}</span></span>
            : <span className="badge warn">no policy yet (written on first save or run)</span>}
        </div>
      )}

      <Card>
        <div className="row" style={{ marginBottom: 8 }}>
          <label className="row" style={{ gap: 6 }}>
            <span className="muted small">provider</span>
            <Select value={provider} onChange={(e) => pickProvider(e.target.value)} disabled={!!busy}>
              {PROVIDERS.map((p) => <option key={p.id} value={p.id}>{p.id}</option>)}
            </Select>
          </label>
          <label className="row grow" style={{ gap: 6 }}>
            <span className="muted small">model</span>
            <Input type="text" className="grow mono" value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy} />
          </label>
        </div>
        <div className="row" style={{ marginBottom: 8 }}>
          <label className="row grow" style={{ gap: 6 }}>
            <span className="muted small nowrap">api key env var</span>
            <Input
              type="text" className="grow mono" placeholder="e.g. ANTHROPIC_API_KEY"
              value={apiKeyEnv} onChange={(e) => setApiKeyEnv(e.target.value)} disabled={!!busy}
            />
          </label>
        </div>
        {needsBaseURL && (
          <div className="row" style={{ marginBottom: 8 }}>
            <label className="row grow" style={{ gap: 6 }}>
              <span className="muted small nowrap">base URL</span>
              <Input
                type="text" className="grow mono" placeholder="https://llm.internal:8443/v1"
                value={baseURL} onChange={(e) => setBaseURL(e.target.value)} disabled={!!busy}
              />
            </label>
          </div>
        )}
        <p className="muted small">
          Only the env var NAME is stored; the key itself stays in your environment. The agent
          container&apos;s only egress is the model endpoint this derives.
        </p>
        <div className="row" style={{ justifyContent: "flex-end" }}>
          <Button disabled={!!busy || (needsBaseURL && !baseURL.trim())} onClick={() => apply(false)}>
            Save provider settings
          </Button>
          <Button
            variant="primary"
            disabled={!!busy || !status?.dockerOk || (needsBaseURL && !baseURL.trim())}
            title={status?.dockerOk ? "" : "docker must be available first"}
            onClick={() => apply(true)}
          >
            Prepare images now
          </Button>
        </div>
        {busy && <Spinner label={busy} />}
        {log.length > 0 && (
          <pre ref={logRef} className="run-log selectable">{log.join("\n")}</pre>
        )}
      </Card>
    </div>
  );
}

/** Prerequisites (D-014): BriefAgent runs need a docker engine. On Linux
 * with a recognized package manager the app installs it through pkexec —
 * the OS shows its own elevation dialog and the exact command is shown
 * before consent. Elsewhere, a hand-off to the official installer. Install
 * output streams into the Agent-runs log (runner:setup-progress). */
function PrereqCard({ onDone }: { onDone: () => Promise<void> }) {
  const [prereq, setPrereq] = useState<main.PrereqView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = () => RunAPI.PrereqStatus().then(setPrereq).catch((e) => setError(String(e)));
  useEffect(() => { refresh(); }, []);

  async function install() {
    setBusy(true);
    setError(null);
    try {
      const p = await RunAPI.InstallDocker();
      setPrereq(p);
      await onDone();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  if (!prereq) return null;
  return (
    <Card>
      <strong>Install prerequisites</strong>
      <p className="muted small">
        BriefAgent runs need a docker engine; the MCP gateway and harness images follow
        automatically once it answers (&quot;Prepare images now&quot; below).
      </p>
      <ErrorBox error={error} />
      {prereq.canInstall ? (
        <>
          <p className="small">
            This will run, with the system&apos;s own elevation prompt:
          </p>
          <pre className="run-log selectable">pkexec sh -c &apos;{prereq.command}&apos;</pre>
          {prereq.note && <p className="muted small">{prereq.note}</p>}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button variant="primary" disabled={busy} onClick={install}>
              {busy ? "Installing…" : "Install the docker engine"}
            </Button>
          </div>
        </>
      ) : (
        <p className="small">
          Automatic installation is not available on this system ({prereq.os}
          {prereq.distro ? `/${prereq.distro}` : ""}). Install docker with the official
          instructions, then come back:{" "}
          <Button variant="link" onClick={() => OpenExternal(prereq.downloadUrl)}>
            {prereq.downloadUrl}
          </Button>
        </p>
      )}
    </Card>
  );
}
