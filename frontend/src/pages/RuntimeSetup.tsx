import React, { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { OpenExternal } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, Select, Spinner } from "../ui";

// The §P1 provider table, mirrored for form prefills; the Go side
// (pkg/runner/setup) holds the authoritative copy and fills any gaps. The
// label is the product's name — a person picks "Anthropic", not a
// provider id (agentry-desktop#131).
const PROVIDERS: { id: string; label: string; model: string; apiKeyEnv: string }[] = [
  { id: "anthropic", label: "Anthropic", model: "claude-sonnet-4-5", apiKeyEnv: "ANTHROPIC_API_KEY" },
  { id: "openai", label: "OpenAI", model: "gpt-5.2", apiKeyEnv: "OPENAI_API_KEY" },
  { id: "google", label: "Google", model: "gemini-3-pro", apiKeyEnv: "GEMINI_API_KEY" },
  { id: "custom", label: "Custom endpoint (OpenAI-compatible)", model: "", apiKeyEnv: "CUSTOM_API_KEY" },
];

const providerLabel = (id: string) => PROVIDERS.find((p) => p.id === id)?.label ?? id;

/** "Agent runs" (RUNNER-PLAN-2 §P1, req 5): the LLM provider BriefAgent
 * runs use, written into the same operator policy the CLI reads
 * (~/.agentry/runner/policy.yaml), plus one-click preparation of the
 * docker MCP gateway plane.
 *
 * The form asks three things (agentry-desktop#131, D-028): the provider,
 * the key, and — for a custom endpoint — its URL. The key goes to this
 * machine's keychain (the app's own store, D-024) and the policy records
 * only that the keychain is the source, never the value. "Get models"
 * asks the endpoint what it serves with that key, which is also the
 * endpoint test; the answer fills the model list. Settings (`advanced`)
 * keeps the environment-variable path under Advanced, where a person who
 * sets variables will look and one who does not never has to. */
export function RuntimeSetup({ heading, intro, advanced = false }: {
  heading?: React.ReactNode;
  intro?: React.ReactNode;
  // Settings shows the advanced choices (the variable, forgetting the
  // stored key); the welcome does not.
  advanced?: boolean;
} = {}) {
  const [status, setStatus] = useState<main.SetupStatusView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [provider, setProvider] = useState("anthropic");
  const [model, setModel] = useState("");
  const [apiKeyEnv, setApiKeyEnv] = useState("");
  const [baseURL, setBaseURL] = useState("");
  // The typed key lives in this component's state until Save or Get
  // models sends it to the backend (StoreModelKey / ListModels, each a
  // call of its own, desktop#102), and is cleared on the way out of Save.
  const [apiKey, setApiKey] = useState("");
  // keyMode is the source the policy will record: the keychain (the
  // default, and the only choice the welcome offers) or the variable
  // (Settings → Advanced).
  const [keyMode, setKeyMode] = useState<"keyring" | "env">("keyring");
  const [forgetKey, setForgetKey] = useState(false);
  // What the endpoint listed, for the provider and URL it was asked
  // about; cleared when either changes so a stale list never offers the
  // wrong endpoint's models.
  const [catalog, setCatalog] = useState<main.CatalogView | null>(null);
  const [keyNotice, setKeyNotice] = useState<string | null>(null);
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
        // The keychain, unless this is Settings and the policy chose the
        // variable: a fresh machine has no source recorded, and the
        // welcome — which offers only the keychain — must not read that
        // as "the variable" and hide the key field (the review of #131
        // caught exactly that).
        setKeyMode(advanced && s.apiKeySource === "env" ? "env" : "keyring");
        setKeyNotice(s.keyNotice ?? null);
      })
      .catch((e) => setError(String(e)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
    // A key is the provider's: one typed for Anthropic must not reach
    // OpenAI's endpoint on Get models, nor be stored under it on Save.
    setApiKey("");
    setForgetKey(false);
    setCatalog(null);
    setKeyNotice(null);
    const row = PROVIDERS.find((p) => p.id === id);
    if (row) {
      setModel(row.model);
      setApiKeyEnv(row.apiKeyEnv);
    }
  }

  function changeBaseURL(v: string) {
    setBaseURL(v);
    setCatalog(null);
  }

  const needsBaseURL = provider === "custom";
  // A stored key counts for the provider the status reports; a switch of
  // provider needs a key typed.
  const keyStoredForProvider = !!status?.keyStored && status?.provider === provider;
  const hasKey = !!apiKey.trim() || keyStoredForProvider;

  // Get models: the endpoint, asked with the typed key (or the stored
  // one), fills the model list — and proves the endpoint and the key.
  async function getModels() {
    setBusy("Asking the endpoint which models it serves…");
    setError(null);
    setNotice(null);
    try {
      const cat = await RunAPI.ListModels(provider, baseURL.trim(), apiKey);
      setCatalog(cat);
      setKeyNotice(cat.notice || null);
      // The current model stays if the endpoint lists it; the table's
      // default or nothing yet becomes the first listed; an id the person
      // typed themselves is kept as its own entry rather than replaced.
      const typed = model.trim() && !PROVIDERS.some((p) => p.model === model);
      if (!typed && !cat.models.some((m) => m.id === model)) {
        setModel(cat.models[0]?.id ?? model);
      }
      setNotice(`${cat.models.length} model${cat.models.length === 1 ? "" : "s"} listed — the endpoint and the key work.`);
    } catch (e) {
      setCatalog(null);
      setError(`Could not list models: ${e instanceof Error ? e.message : e}. You can still type a model id and save.`);
    } finally {
      setBusy(null);
    }
  }

  async function apply(pullImages: boolean) {
    setBusy(pullImages ? "Preparing images — pulls and a local build can take minutes…" : "Writing policy…");
    setError(null);
    setNotice(null);
    if (pullImages) setLog([]);
    try {
      // A typed key travels once, in its own small call, straight to the
      // keychain (desktop#102); the field is cleared before the call, so a
      // stale value never lingers on screen — a refused store means typing
      // it again, and the error says so.
      const key = keyMode === "keyring" && !forgetKey ? apiKey : "";
      const source = keyMode === "keyring" ? "keyring" : "env";
      setApiKey("");
      if (key) {
        try {
          await RunAPI.StoreModelKey(provider, key);
        } catch (e) {
          throw new Error(`${e} — the key you typed was cleared; type it again to retry.`);
        }
      }
      const s = await RunAPI.ApplySetup(provider, model.trim(), apiKeyEnv.trim(), baseURL.trim(), pullImages, source, forgetKey);
      setForgetKey(false);
      setStatus(s);
      setKeyMode(s.apiKeySource === "keyring" ? "keyring" : "env");
      setKeyNotice(s.keyNotice ?? null);
      setNotice(pullImages
        ? "Setup complete — gateway, harness, and runner images ready; BriefAgents can run."
        : `Provider settings written to ${s.policyPath}.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  // Save needs a key behind the source it records: one typed now or one
  // stored already for the keychain, the variable's name for the
  // variable. The backend refuses the same cases; this just says so first.
  const needsKey = keyMode === "keyring" && !forgetKey && !hasKey;
  const needsVar = keyMode === "env" && !apiKeyEnv.trim();
  const canSave = !busy && !(needsBaseURL && !baseURL.trim()) && !needsKey && !needsVar && !!model.trim();
  const saveHint = needsKey ? "paste your API key first" : needsVar ? "name the variable" : needsBaseURL && !baseURL.trim() ? "enter the endpoint's URL" : !model.trim() ? "pick or type a model" : "";

  return (
    <div>
      <h2>{heading ?? "Agent runs"}</h2>
      <p className="page-sub">
        {intro ?? (
          <>
            BriefAgent runs happen in isolated containers with tools served by a per-run Docker MCP
            Gateway. Choose the LLM provider those runs use and paste its key — the same operator
            policy the <span className="mono">agentry</span> CLI reads.
          </>
        )}
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
        <label className="field">
          <span className="field-label">Provider</span>
          <Select value={provider} onChange={(e) => pickProvider(e.target.value)} disabled={!!busy}>
            {PROVIDERS.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          </Select>
        </label>

        {needsBaseURL && (
          <label className="field">
            <span className="field-label">Endpoint URL</span>
            <Input
              type="text" className="mono" placeholder="https://llm.example.net/v1"
              value={baseURL} onChange={(e) => changeBaseURL(e.target.value)} disabled={!!busy}
            />
            <span className="muted small field-hint">The base URL of an OpenAI-compatible endpoint; the run can reach this host and nothing else.</span>
          </label>
        )}

        {keyMode === "keyring" && (
          <label className="field">
            <span className="field-label">API key</span>
            <div className="row">
              <Input
                type="password" className="grow mono" autoComplete="off"
                placeholder={keyStoredForProvider ? "stored on this machine — paste a new one to replace it" : "Paste your API key"}
                value={apiKey} onChange={(e) => { setApiKey(e.target.value); setKeyNotice(status?.keyNotice ?? null); }} disabled={!!busy || forgetKey}
              />
              <Button
                disabled={!!busy || !hasKey || (needsBaseURL && !baseURL.trim())}
                title={!hasKey ? "paste the key first" : needsBaseURL && !baseURL.trim() ? "enter the endpoint's URL first" : "ask the endpoint which models it serves"}
                onClick={getModels}
              >
                Get models
              </Button>
            </div>
            <span className="muted small field-hint">
              Kept in this machine's keychain, next to your registry sign-in — not in a file, and never shown here.
              {status?.keyStoreErr && keyMode === "keyring" ? <> The keychain could not be asked: {status.keyStoreErr}.</> : null}
            </span>
          </label>
        )}
        {keyNotice && <Notice tone="warn">{keyNotice.charAt(0).toUpperCase() + keyNotice.slice(1)}.</Notice>}

        <label className="field">
          <span className="field-label">Model</span>
          {catalog && catalog.models.length > 0 ? (
            <Select value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy}>
              {catalog.models.map((m) => (
                <option key={m.id} value={m.id}>{m.displayName ? `${m.displayName} — ${m.id}` : m.id}</option>
              ))}
              {!catalog.models.some((m) => m.id === model) && model && <option value={model}>{model}</option>}
            </Select>
          ) : (
            <Input type="text" className="mono" placeholder="pick one with Get models, or type a model id" value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy} />
          )}
        </label>

        {advanced && (
          <details className="advanced">
            <summary className="muted small">Advanced</summary>
            <div className="key-source">
              <label className="row" style={{ gap: 6 }}>
                <input type="radio" name="keyMode" checked={keyMode === "keyring"} disabled={!!busy} onChange={() => setKeyMode("keyring")} />
                <span>Keep the key on this machine (the keychain). Anything running as you while your session is unlocked can read it, as it can your registry sign-in.</span>
              </label>
              <label className="row" style={{ gap: 6 }}>
                <input type="radio" name="keyMode" checked={keyMode === "env"} disabled={!!busy} onChange={() => { setKeyMode("env"); setApiKey(""); }} />
                <span>Read the key from an environment variable instead — for a key you set yourself in the environment this app is launched from (a dock- or menu-launched app has none).</span>
              </label>
              {keyMode === "env" && (
                <label className="row grow key-store" style={{ gap: 6 }}>
                  <span className="muted small nowrap">variable name</span>
                  <Input type="text" className="grow mono" placeholder="e.g. ANTHROPIC_API_KEY" value={apiKeyEnv} onChange={(e) => setApiKeyEnv(e.target.value)} disabled={!!busy} />
                  <Button disabled={!!busy || !apiKeyEnv.trim() || (needsBaseURL && !baseURL.trim())} title="ask the endpoint with the saved policy's variable, as a run would (save first after renaming it)" onClick={getModels}>Get models</Button>
                </label>
              )}
              {keyStoredForProvider && (
                <label className="inline muted small key-store">
                  <input type="checkbox" checked={forgetKey} disabled={!!busy} onChange={(e) => { setForgetKey(e.target.checked); if (e.target.checked) setApiKey(""); }} />{" "}
                  Forget the stored key for {providerLabel(provider)} on save
                </label>
              )}
            </div>
          </details>
        )}

        <p className="muted small">
          The policy records the provider, the model and where the key comes from — never the key. The agent
          container&apos;s only route out is the endpoint this names.
        </p>
        <div className="row" style={{ justifyContent: "flex-end" }}>
          <Button disabled={!canSave} title={saveHint} onClick={() => apply(false)}>
            Save provider settings
          </Button>
          <Button
            variant="primary"
            disabled={!canSave || !status?.dockerOk}
            title={!status?.dockerOk ? "docker must be available first" : saveHint}
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

/** Prerequisites (D-014, agentry-desktop#64): BriefAgent runs need a docker
 * engine. The card renders the machine's STATE, not an error string:
 * installable → the install through pkexec (the exact command shown before
 * consent); groupMissing → only the group step, through pkexec;
 * pendingRestart → the engine is in and the group is granted, but this
 * session cannot use it until it restarts: Restart now / Restart later;
 * manual → the hand-off to the official installer, with the engine's own
 * words when one is installed and still refuses. Output streams into the
 * Agent-runs log (runner:setup-progress). Elevation only through pkexec; a
 * reboot is asked of the session manager, never run as root. */
export function PrereqCard({ onDone }: { onDone: () => Promise<void> }) {
  const [prereq, setPrereq] = useState<main.PrereqView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [confirmRestart, setConfirmRestart] = useState(false);
  const [deferred, setDeferred] = useState(false);

  const refresh = () => RunAPI.PrereqStatus().then(setPrereq).catch((e) => setError(String(e)));
  useEffect(() => { refresh(); }, []);

  async function act(label: string, call: () => Promise<main.PrereqView>) {
    setBusy(label);
    setError(null);
    try {
      setPrereq(await call());
      await onDone();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function restartNow() {
    setConfirmRestart(false);
    setBusy("Asking the session manager to restart…");
    setError(null);
    try {
      await RunAPI.RestartNow();
    } catch (e) {
      setError(String(e));
      setBusy(null);
    }
  }

  // Restart later keeps nothing: the state is re-detected on every launch,
  // so a person who chose later meets the same card until they restart.
  function restartLater() {
    setDeferred(true);
  }

  if (!prereq || prereq.state === "ready") return null;
  return (
    <Card>
      <strong>Docker engine</strong>
      <ErrorBox error={error} />

      {prereq.state === "pendingRestart" && (
        <>
          <p className="small">
            Docker is installed and your user is in the <span className="mono">docker</span> group.
            Your session has to restart before it can use it.
          </p>
          {deferred ? (
            <p className="muted small">Restart when you are ready; this card stays until the engine answers.</p>
          ) : (
            <div className="row" style={{ justifyContent: "flex-end" }}>
              <Button onClick={restartLater} disabled={!!busy}>Restart later</Button>
              <Button variant="primary" onClick={() => setConfirmRestart(true)} disabled={!!busy}>Restart now</Button>
            </div>
          )}
        </>
      )}

      {prereq.state === "groupMissing" && (
        <>
          <p className="small">
            Docker is installed, but your user is not in the <span className="mono">docker</span> group,
            so runs cannot reach it.
          </p>
          {prereq.groupCommand ? (
            <Elevated
              command={prereq.groupCommand}
              note="Afterwards your session has to restart before the group counts."
              label="Add me to the docker group"
              busy={!!busy}
              onClick={() => act("Adding you to the docker group…", RunAPI.AddToDockerGroup)}
            />
          ) : (
            <p className="muted small">
              The app cannot ask for elevation here. As root, run{" "}
              <span className="mono">usermod -aG docker &lt;your user&gt;</span>, then restart your session.
            </p>
          )}
        </>
      )}

      {prereq.state === "installable" && (
        <>
          <p className="muted small">
            BriefAgent runs need a docker engine; the MCP gateway and harness images follow
            automatically once it answers (&quot;Prepare images now&quot; below).
          </p>
          <Elevated
            command={prereq.command ?? ""}
            note={prereq.note}
            label="Install the docker engine"
            busy={!!busy}
            onClick={() => act("Installing…", RunAPI.InstallDocker)}
          />
        </>
      )}

      {prereq.state === "manual" && (
        <p className="small">
          {prereq.engineInstalled ? (
            <>
              Docker is installed and your user is in its group, but the engine still refuses:{" "}
              <span className="mono">{prereq.dockerErr}</span>. Check that the service is running
              (<span className="mono">systemctl status docker</span>), then come back.
            </>
          ) : (
            <>
              Automatic installation is not available on this system ({prereq.os}
              {prereq.distro ? `/${prereq.distro}` : ""}). Install docker with the official
              instructions, then come back:{" "}
              <Button variant="link" onClick={() => OpenExternal(prereq.downloadUrl)}>
                {prereq.downloadUrl}
              </Button>
            </>
          )}
        </p>
      )}

      {busy && <Spinner label={busy} />}

      {confirmRestart && (
        <Modal title="Restart this machine now?" onClose={() => setConfirmRestart(false)}>
          <p>
            This asks the system to reboot: every open application closes, and unsaved work in
            them is lost. The app does not run as root — the request goes to the session
            manager (<span className="mono">systemctl reboot</span>), and if it refuses, the
            system&apos;s own elevation prompt appears.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmRestart(false)}>Cancel</Button>
            <Button variant="danger" onClick={restartNow}>Restart now</Button>
          </div>
        </Modal>
      )}
    </Card>
  );
}

/** One elevated command, shown in full before consent, with the button that
 * runs it through pkexec (the OS's own dialog; this process never sees a
 * password). */
function Elevated({ command, note, label, busy, onClick }: {
  command: string; note?: string; label: string; busy: boolean; onClick: () => void;
}) {
  return (
    <>
      <p className="small">This will run, with the system&apos;s own elevation prompt:</p>
      <pre className="run-log selectable">pkexec sh -c &apos;{command}&apos;</pre>
      {note && <p className="muted small">{note}</p>}
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button variant="primary" disabled={busy} onClick={onClick}>{label}</Button>
      </div>
    </>
  );
}
