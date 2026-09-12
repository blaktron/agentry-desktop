import React, { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { OpenExternal } from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, Select, Spinner } from "../ui";

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
export function RuntimeSetup({ heading, intro }: {
  heading?: React.ReactNode;
  intro?: React.ReactNode;
} = {}) {
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
      <h2>{heading ?? "Agent runs"}</h2>
      <p className="page-sub">
        {intro ?? (
          <>
            BriefAgent runs happen in isolated containers with tools served by a per-run Docker MCP
            Gateway. Configure the LLM provider those runs drive — written to the same operator
            policy the <span className="mono">agentry</span> CLI uses.
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
