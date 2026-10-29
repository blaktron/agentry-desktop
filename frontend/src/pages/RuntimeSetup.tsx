import React, { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/app/RunAPI";
import { OpenExternal } from "../../wailsjs/go/app/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { app, run } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, Select, Spinner } from "../ui";
import { baseURLFor, takesBaseURL } from "../providerForm";
import {
  COPILOT_PLANS, PROVIDERS, copilotListFailure, copilotModelAfterList, copilotModelLabel, keptFor, modelAsk, modelOptionLabel, providerChoice, providerLabel, savedWay, signInSaveHint, startingModel, waysFor,
  type KeptSignIn, type KeyGuide, type WayId,
} from "../modelSetup";
import { KeptSignIns, SignInWay } from "./ModelSignIn";
import { ProviderTiles } from "./ProviderTiles";
import { asSentence } from "../signIns";

// The provider table (src/modelSetup.ts) mirrors the CLI's for form
// prefills; the Go side (pkg/runner/setup) holds the authoritative copy and
// fills any gaps. The label is the product's name — a person picks
// "Anthropic", not a provider id.
export { providerLabel };

/** "Agent runs": the LLM provider QuickAgent runs use, written into the same
 * operator policy the CLI reads (~/.agentry/runner/policy.yaml), plus one-click
 * preparation of the docker MCP gateway plane.
 *
 * The form asks three things: the
 * provider (one logo tile each), the key, and — for a custom endpoint — its
 * URL. The key goes to this machine's keychain (the app's own store) and the
 * policy records only that the keychain is the source, never the value. "Get
 * models" asks the endpoint what it serves with that key, which is also the
 * endpoint test; the answer fills the model list. Settings (`advanced`) keeps
 * the environment-variable path under Advanced, where a person who sets
 * variables will look and one who does not never has to. */
export function RuntimeSetup({ heading, intro, advanced = false, onBusy, onOpenPrereqs }: {
  heading?: React.ReactNode;
  intro?: React.ReactNode;
  // Settings shows the advanced choices (the variable, forgetting the
  // stored key); the welcome does not.
  advanced?: boolean;
  // Told when an image pull starts and ends, so a wizard around the form
  // can hold its own Continue and Back for the pull: leaving
  // mid-pull unmounts the form while the Go side keeps pulling with nobody
  // watching the log. The shorter calls (Get models, the policy write)
  // orphan nothing and are not reported.
  onBusy?: (busy: boolean) => void;
  // Settings keeps the container engine's prerequisites in Run defaults:
  // given this, a missing engine is a pointer there rather than the
  // prerequisites card itself, which the welcome still shows.
  onOpenPrereqs?: () => void;
} = {}) {
  const [status, setStatus] = useState<app.SetupStatusView | null>(null);
  const [checkingDocker, setCheckingDocker] = useState(true);
  const dockerCheckPending = useRef(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [provider, setProvider] = useState("anthropic");
  const [model, setModel] = useState("");
  const [apiKeyEnv, setApiKeyEnv] = useState("");
  const [baseURL, setBaseURL] = useState("");
  // The typed key lives in this component's state until Save or Get
  // models sends it to the backend (StoreModelKey / ListModels, each a
  // call of its own), and is cleared on the way out of Save.
  const [apiKey, setApiKey] = useState("");
  // keyMode is the source the policy will record: the keychain (the
  // default, and the only choice the welcome offers) or the variable
  // (Settings → Advanced).
  const [keyMode, setKeyMode] = useState<"keyring" | "env">("keyring");
  const [forgetKey, setForgetKey] = useState(false);
  // What the endpoint listed, for the provider and URL it was asked
  // about; cleared when either changes so a stale list never offers the
  // wrong endpoint's models.
  const [catalog, setCatalog] = useState<app.CatalogView | null>(null);
  const [keyNotice, setKeyNotice] = useState<string | null>(null);
  // How the provider's models are paid for: a model
  // sign-in, a connected account, or a key; what this computer keeps; the
  // models a ChatGPT plan offers; and the Copilot plan.
  const [way, setWay] = useState<WayId>("key");
  const [kept, setKept] = useState<KeptSignIn[]>([]);
  const [planModels, setPlanModels] = useState<string[]>([]);
  const [copilotPlan, setCopilotPlan] = useState("individual");
  // The models the Copilot plan offers, asked of Copilot CLI: null until
  // listed; and why a list failed.
  const [copilotModels, setCopilotModels] = useState<run.CopilotModel[] | null>(null);
  const [copilotListError, setCopilotListError] = useState<string | null>(null);
  // Whether a list was asked for, failed or not: a plan change asks again.
  const [copilotAsked, setCopilotAsked] = useState(false);
  const [log, setLog] = useState<string[]>([]);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    let live = true;
    Promise.all([RunAPI.SetupStatus(), RunAPI.ModelSignIns().catch(() => [] as KeptSignIn[])])
      .then(([s, k]) => {
        if (!live) return;
        setStatus(s);
        setKept(k ?? []);
        setProvider(s.provider || "anthropic");
        const prov = s.provider || "anthropic";
        const w = savedWay(prov, s.provider, s.apiKeyKind ?? "", k ?? [], s.providerSetUp);
        setWay(w);
        // A sign-in way starts on the saved model where the policy already
        // pays that way, else on the way's own default (Copilot's auto).
        setModel(w === "key" ? s.model : startingModel(prov, w, { provider: s.provider, model: s.model, way: savedWay(prov, s.provider, s.apiKeyKind ?? "", k ?? []) }, []));
        setApiKeyEnv(s.apiKeyEnv);
        setBaseURL(s.baseUrl ?? "");
        // The keychain, unless this is Settings and the policy chose the
        // variable: a fresh machine has no source recorded, and the
        // welcome — which offers only the keychain — must not read that
        // as "the variable" and hide the key field.
        setKeyMode(advanced && s.apiKeySource === "env" ? "env" : "keyring");
        setKeyNotice(s.keyNotice ?? null);
      })
      .catch((e) => { if (live) setError(String(e)); })
      .finally(() => { if (live) setCheckingDocker(false); });
    return () => { live = false; };
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

  async function checkDockerAgain() {
    if (dockerCheckPending.current) return;
    dockerCheckPending.current = true;
    setCheckingDocker(true);
    setError(null);
    try {
      // Refresh readiness only; provider fields may hold unsaved edits.
      setStatus(await RunAPI.SetupStatus());
    } catch (e) {
      setError(String(e));
    } finally {
      dockerCheckPending.current = false;
      setCheckingDocker(false);
    }
  }

  // Ask Copilot CLI which models the plan offers the kept sign-in. It may build
  // the runner and Copilot CLI images first, so its progress is the setup
  // log's. A failure is said and the model is typed instead; it never
  // blocks Save.
  async function listCopilotModels(plan: string) {
    setBusy("Asking Copilot CLI which models your plan offers — a first list builds images and can take minutes…");
    setError(null);
    setNotice(null);
    setCopilotListError(null);
    setCopilotAsked(true);
    setLog([]);
    onBusy?.(true);
    try {
      const got = (await RunAPI.CopilotModels(plan)) ?? [];
      setCopilotModels(got);
      setModel((cur) => copilotModelAfterList(cur, got.map((m) => m.id)));
    } catch (e) {
      setCopilotModels(null);
      setCopilotListError(String(e));
    } finally {
      setBusy(null);
      onBusy?.(false);
    }
  }

  // A plan names the Copilot API host the list reaches: a list already
  // asked for is asked again for the new plan.
  function changeCopilotPlan(plan: string) {
    setCopilotPlan(plan);
    if (copilotAsked) void listCopilotModels(plan);
  }

  // A new account, its removal or another provider: no list stands.
  function resetCopilotList() {
    setCopilotModels(null);
    setCopilotListError(null);
    setCopilotAsked(false);
  }

  // The way the saved policy pays, for a choice to start on its model.
  const saved = { provider: status?.provider ?? "", model: status?.model ?? "", way: savedWay(status?.provider ?? "", status?.provider ?? "", status?.apiKeyKind ?? "", kept, status?.providerSetUp) };

  async function refreshKept() {
    try {
      setKept((await RunAPI.ModelSignIns()) ?? []);
    } catch { /* the list stays as it was */ }
  }

  function pickWay(id: string, w: WayId) {
    setWay(w);
    setError(null);
    setNotice(null);
    if (w !== "key") setModel(startingModel(id, w, saved, planModels));
    if (w === "chatgpt" && planModels.length === 0 && keptFor(waysFor(id)[0], kept)) {
      RunAPI.ChatGPTModels().then((m) => {
        setPlanModels(m ?? []);
        setModel((cur) => cur || (m ?? [])[0] || "");
      }).catch(() => undefined);
    }
  }

  function pickProvider(id: string) {
    setProvider(id);
    resetCopilotList();
    pickWay(id, savedWay(id, status?.provider ?? "", status?.apiKeyKind ?? "", kept, status?.providerSetUp));
    // A key is the provider's: one typed for Anthropic must not reach
    // OpenAI's endpoint on Get models, nor be stored under it on Save.
    setApiKey("");
    setForgetKey(false);
    setCatalog(null);
    setKeyNotice(null);
    const row = PROVIDERS.find((p) => p.id === id);
    if (row) {
      if (savedWay(id, status?.provider ?? "", status?.apiKeyKind ?? "", kept, status?.providerSetUp) === "key") setModel(row.model);
      setApiKeyEnv(row.apiKeyEnv);
    }
  }

  // Save for a sign-in or a connected account: the CLI's own setup makes
  // what is kept the active choice, and refuses one nothing is signed in to.
  async function applySignIn(pullImages: boolean) {
    setBusy(pullImages ? "Preparing images — pulls and a local build can take minutes…" : "Writing policy…");
    setError(null);
    setNotice(null);
    if (pullImages) {
      setLog([]);
      onBusy?.(true);
    }
    try {
      const w = waysFor(provider).find((x) => x.id === way);
      const s = await RunAPI.ApplySignInSetup(provider, model.trim(), w?.id === "connect" ? "" : w?.kind ?? "", w?.id === "connect",
        way === "copilot" ? copilotPlan : "", pullImages);
      setStatus(s);
      setNotice(pullImages
        ? "Setup complete — QuickAgents on this computer run on your sign-in, and the images are ready."
        : `Saved: QuickAgents on this computer run on ${w?.label ?? "your sign-in"}.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
      if (pullImages) onBusy?.(false);
    }
  }

  function changeBaseURL(v: string) {
    setBaseURL(v);
    setCatalog(null);
  }

  const needsBaseURL = takesBaseURL(provider);

  // Plain http off this machine is allowed and said out loud: the sentence is
  // the CLI's, asked as the URL changes, so both clients word it the same. It
  // gates nothing.
  const [endpointWarning, setEndpointWarning] = useState("");
  useEffect(() => {
    const u = baseURL.trim();
    if (!needsBaseURL || !u) {
      setEndpointWarning("");
      return;
    }
    let live = true;
    RunAPI.EndpointWarning(u).then((w) => { if (live) setEndpointWarning(w); }).catch(() => { if (live) setEndpointWarning(""); });
    return () => { live = false; };
  }, [baseURL, needsBaseURL]);
  // A stored key counts for the provider it is stored under — the status
  // lists them all — so a switch of provider needs a key
  // typed unless one is on this machine for the new one too.
  const keyStoredForProvider = !!status?.keysStored.includes(provider);
  const hasKey = !!apiKey.trim() || keyStoredForProvider;

  // Get models: the endpoint, asked with the typed key (or the stored
  // one), fills the model list — and proves the endpoint and the key.
  async function getModels() {
    setBusy("Asking the endpoint which models it serves…");
    setError(null);
    setNotice(null);
    try {
      const cat = await RunAPI.ListModels(provider, baseURLFor(provider, baseURL), apiKey);
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
    if (pullImages) {
      setLog([]);
      onBusy?.(true);
    }
    try {
      // A typed key travels once, in its own small call, straight to the
      // keychain; the field is cleared before the call, so a
      // stale value never lingers on screen — a refused store means typing
      // it again, and the error says so. A store that succeeded is kept
      // as the status says it: the key is on this machine for this
      // provider, so a refusal of the save that follows does not ask for
      // it again.
      const key = keyMode === "keyring" && !forgetKey ? apiKey : "";
      const source = keyMode === "keyring" ? "keyring" : "env";
      setApiKey("");
      if (key) {
        try {
          setStatus(await RunAPI.StoreModelKey(provider, key));
        } catch (e) {
          throw new Error(`${e} — the key you typed was cleared; type it again to retry.`);
        }
      }
      const s = await RunAPI.ApplySetup(provider, model.trim(), apiKeyEnv.trim(), baseURLFor(provider, baseURL), pullImages, source, forgetKey);
      setForgetKey(false);
      setStatus(s);
      setKeyMode(s.apiKeySource === "keyring" ? "keyring" : "env");
      setKeyNotice(s.keyNotice ?? null);
      setNotice(pullImages
        ? "Setup complete — gateway, harness, and runner images ready; QuickAgents can run."
        : `Provider settings written to ${s.policyPath}.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
      if (pullImages) onBusy?.(false);
    }
  }

  // Save needs a key behind the source it records: one typed now or one
  // stored already for the keychain, the variable's name for the
  // variable. The backend refuses the same cases; this just says so first.
  const ways = waysFor(provider);
  const currentWay = ways.find((w) => w.id === way) ?? ways[0];
  const copilotIds = (copilotModels ?? []).map((m) => m.id);
  const ask = modelAsk(provider, way, way === "copilot" ? copilotIds : planModels);
  const copilotKept = way === "copilot" && currentWay ? keptFor(currentWay, kept) : undefined;
  const signInHint = currentWay && way !== "key" ? signInSaveHint(currentWay, keptFor(currentWay, kept), model) : "";
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
            QuickAgent runs happen in isolated containers with tools served by a per-run Docker MCP
            Gateway. Choose the LLM provider those runs use and paste its key — the same operator
            policy the <span className="mono">agentry</span> CLI reads.
          </>
        )}
      </p>

      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}

      {status && !status.dockerOk && !checkingDocker && onOpenPrereqs && (
        <Notice tone="warn">
          The container engine is not ready on this computer, so runs cannot start and the images cannot be prepared.{" "}
          <Button variant="link" onClick={onOpenPrereqs}>Run defaults</Button> sets it up.
        </Notice>
      )}
      {status && !status.dockerOk && !checkingDocker && !onOpenPrereqs && (
        <PrereqCard
          onDone={async () => {
            try { setStatus(await RunAPI.SetupStatus()); } catch { /* badge stays */ }
          }}
        />
      )}

      {checkingDocker && <Spinner label={CHECKING_ENGINE} />}
      {(!status || (!status.dockerOk && onOpenPrereqs)) && !checkingDocker && (
        <Button onClick={checkDockerAgain} disabled={!!busy}>Check again</Button>
      )}
      {status && (
        <div className="row" style={{ marginBottom: 10 }}>
          {checkingDocker ? <span className="badge">checking the engine</span> : status.dockerOk
            ? <span className="badge ok" title={status.engine?.summary}>{status.engine?.kind ? `${status.engine.kind} ${status.engine.version ?? ""}`.trim() : "engine"} available</span>
            : <span className="badge bad" title={status.dockerErr}>no engine</span>}
          {status.dockerOk && (status.imagesReady
            ? <span className="badge ok">images ready</span>
            : <span className="badge warn">images not prepared</span>)}
          {status.policyExists
            ? <span className="badge">policy: <span className="mono">{status.policyPath}</span></span>
            : <span className="badge warn">no policy yet (written on first save or run)</span>}
        </div>
      )}

      <Card>
        <ProviderTiles value={provider} onPick={pickProvider} disabled={!!busy} />

        {ways.length > 1 && (
          <div className="field way-choice">
            <span className="field-label">How to pay for the models</span>
            {ways.map((w) => (
              <label key={w.id} className="row" style={{ gap: 6 }}>
                <input type="radio" name="way" checked={way === w.id} disabled={!!busy} onChange={() => pickWay(provider, w.id)} />
                <span>{w.label}</span>
              </label>
            ))}
          </div>
        )}

        {currentWay && way !== "key" && (
          <>
            <SignInWay
              way={currentWay} kept={kept} disabled={!!busy}
              onSignedIn={(models) => {
                if (models.length > 0) {
                  setPlanModels(models);
                  setModel((cur) => cur || models[0]);
                }
                // Another account may be offered other models.
                resetCopilotList();
                void refreshKept();
              }}
              onRemoved={() => { resetCopilotList(); void refreshKept(); }}
            />
            {way === "copilot" && (
              <label className="field">
                <span className="field-label">Your Copilot plan</span>
                <Select value={copilotPlan} onChange={(e) => changeCopilotPlan(e.target.value)} disabled={!!busy}>
                  {COPILOT_PLANS.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}
                </Select>
              </label>
            )}
            <label className="field">
              <span className="field-label">Model</span>
              {way === "copilot" && ask.mode === "plan" ? (
                // Only what the plan offers: a model it disables is not listed.
                <Select value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy}>
                  {ask.options.map((m) => <option key={m} value={m}>{copilotModelLabel(m, copilotModels ?? [])}</option>)}
                </Select>
              ) : ask.mode === "plan" && ask.options.length > 0 ? (
                <Select value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy}>
                  {ask.options.map((m) => <option key={m} value={m}>{m}</option>)}
                  {!ask.options.includes(model) && model && <option value={model}>{model}</option>}
                </Select>
              ) : ask.mode === "pick" ? (
                <>
                  <Input type="text" className="mono" list="claude-models" placeholder={ask.placeholder} value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy} />
                  <datalist id="claude-models">{ask.options.map((m) => <option key={m} value={m} />)}</datalist>
                </>
              ) : (
                <Input type="text" className="mono" placeholder={ask.placeholder} value={model} onChange={(e) => setModel(e.target.value)} disabled={!!busy} />
              )}
              {way === "connect" && <span className="muted small field-hint">Agentry does not list these models: type the id the provider shows. A model that cannot call tools will not work.</span>}
              {way === "copilot" && copilotListError && (
                <span className="muted small field-hint">{copilotListFailure(copilotListError)}</span>
              )}
            </label>
            {way === "copilot" && copilotKept && !copilotKept.broken && (
              <div className="row" style={{ gap: 8, alignItems: "center" }}>
                <Button
                  disabled={!!busy || !status?.dockerOk || checkingDocker}
                  title={!status?.dockerOk ? "docker must be available first" : "Copilot CLI lists the models your plan offers, in a run's container"}
                  onClick={() => void listCopilotModels(copilotPlan)}
                >
                  {copilotModels === null ? "List my plan's models" : "List again"}
                </Button>
                {!status?.dockerOk && <span className="muted small">Docker must be running to list your plan's models.</span>}
              </div>
            )}
            <p className="muted small">
              Runs on this computer use this sign-in; runs in Agentry never do. The policy records which sign-in to use,
              never what it holds.
            </p>
            <div className="row" style={{ justifyContent: "flex-end" }}>
              <Button disabled={!!busy || !!signInHint} title={signInHint} onClick={() => applySignIn(false)}>
                Save
              </Button>
              <Button
                variant="primary"
                disabled={!!busy || !!signInHint || !status?.dockerOk || checkingDocker}
                title={!status?.dockerOk ? "docker must be available first" : signInHint}
                onClick={() => applySignIn(true)}
              >
                Prepare images now
              </Button>
            </div>
          </>
        )}

        {way === "key" && (
          <>
          {needsBaseURL && (
            <label className="field">
              <span className="field-label">Endpoint URL</span>
              <Input
                type="text" className="mono" placeholder="https://llm.example.net/v1"
                value={baseURL} onChange={(e) => changeBaseURL(e.target.value)} disabled={!!busy}
              />
              <span className="muted small field-hint">The base URL of an OpenAI-compatible endpoint; the run can reach this host and nothing else. Use https where the endpoint offers it.</span>
            </label>
          )}
          {needsBaseURL && endpointWarning && <Notice tone="warn">{endpointWarning}</Notice>}
          {providerChoice(provider)?.keyGuide && <KeyGuideBox guide={providerChoice(provider)!.keyGuide!} />}

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
                  <option key={m.id} value={m.id}>{modelOptionLabel(m)}</option>
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
              disabled={!canSave || !status?.dockerOk || checkingDocker}
              title={!status?.dockerOk ? "docker must be available first" : saveHint}
              onClick={() => apply(true)}
            >
              Prepare images now
            </Button>
          </div>
          </>
        )}
        {busy && <Spinner label={busy} />}
        {log.length > 0 && (
          <pre ref={logRef} className="run-log selectable">{log.join("\n")}</pre>
        )}
        {advanced && <KeptSignIns kept={kept} onChanged={() => void refreshKept()} />}
      </Card>
    </div>
  );
}

/** How to get a provider's key, above its key field: the offer, the
 *  numbered steps, and the sign-up page, opened in the person's browser
 *  rather than in this window. */
function KeyGuideBox({ guide }: { guide: KeyGuide }) {
  return (
    <div className="key-guide">
      <p className="key-guide-title">{guide.title}</p>
      <p className="muted small">{guide.offer}</p>
      <ol className="small">
        {guide.steps.map((step) => <li key={step}>{step}</li>)}
      </ol>
      <Button variant="link" onClick={() => void OpenExternal(guide.link.url)}>{guide.link.label}</Button>
    </div>
  );
}

/** Prerequisites: QuickAgent runs need a container engine. The card
 * renders the machine's STATE, not an error string, with the engine found
 * named by kind and version: installable → the distribution's own engine
 * through pkexec (podman on Fedora and the RHEL family, Docker Engine
 * elsewhere; the exact command shown before consent); socketOff → podman is
 * in and its API socket is not, one click turns it on as you; groupMissing
 * → only the group step, through pkexec (Docker Engine); pendingRestart →
 * Docker Engine is in and the group is granted, but this session cannot
 * use it until it restarts: Restart now / Restart later; manual → on Linux
 * the hand-off to the official installer, with the engine's own words when
 * one is installed and still refuses; on macOS and Windows the engines to
 * choose from, Docker Desktop first, and on a Mac that can run it the one
 * click first: Agentry's engine, which the app installs and runs itself
 * (managedOffer), or, installed and silent, Start and Remove
 * (managedStopped; while the app starts it, a spinner). The traps
 * detection saw (a credential
 * helper off PATH, a socket file with nothing behind it) are said beside
 * the state. Output streams into the Agent-runs log (runner:setup-progress).
 * Elevation only through pkexec; a reboot is asked of the session manager,
 * never run as root. */
export function PrereqCard({ onDone }: { onDone: () => Promise<void> }) {
  const [prereq, setPrereq] = useState<app.PrereqView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [confirmRestart, setConfirmRestart] = useState(false);
  const [deferred, setDeferred] = useState(false);

  const checkPending = useRef(false);
  useEffect(() => {
    let live = true;
    setBusy(CHECKING_ENGINE);
    RunAPI.PrereqStatus()
      .then(async (p) => {
        if (!live) return;
        setPrereq(p);
        if (p.dockerOk) await onDone();
      })
      .catch((e) => { if (live) setError(String(e)); })
      .finally(() => { if (live) setBusy(null); });
    return () => { live = false; };
  }, []);

  // Re-detect: the whole order again from nothing (an engine adopted earlier
  // that stopped answering is let go), then the card's state. A run in
  // flight refuses it, in Go's words.
  async function checkAgain() {
    if (checkPending.current) return;
    checkPending.current = true;
    setBusy(CHECKING_ENGINE);
    setError(null);
    try {
      await RunAPI.RedetectEngine();
      const p = await RunAPI.PrereqStatus();
      setPrereq(p);
      if (p.dockerOk) await onDone();
    } catch (e) {
      setError(String(e));
    } finally {
      checkPending.current = false;
      setBusy(null);
    }
  }

  async function act(label: string, call: () => Promise<app.PrereqView>) {
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

  if (!prereq) return <Card>
    <strong>Container engine</strong>
    <ErrorBox error={error} />
    {busy ? <Spinner label={busy} /> : <Button onClick={checkAgain}>Re-detect</Button>}
  </Card>;
  if (prereq.state === "ready") return null;
  const podman = prereq.installs === "podman";
  return (
    <Card>
      <strong>Container engine</strong>
      <ErrorBox error={error} />
      <EngineWarnings warnings={prereq.warnings} />

      {prereq.state === "pendingRestart" && (
        <>
          <p className="small">
            Docker Engine is installed and your user is in the <span className="mono">docker</span> group.
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
            Docker Engine is installed, but your user is not in the <span className="mono">docker</span> group,
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

      {prereq.state === "socketOff" && (
        <>
          <p className="small">
            podman is installed, but its API socket is off, and runs reach podman through it.
            Turning it on runs as you, with no elevation, no group to join and no restart:
          </p>
          <pre className="run-log selectable">{prereq.socketCommand}</pre>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button variant="primary" disabled={!!busy} onClick={() => act("Turning podman's API socket on…", RunAPI.EnablePodmanSocket)}>
              Turn the socket on
            </Button>
          </div>
        </>
      )}

      {prereq.state === "installable" && (
        <>
          <p className="muted small">
            QuickAgent runs need a container engine; the MCP gateway and harness images follow
            automatically once it answers (&quot;Prepare images now&quot; below).
            {podman
              ? " This distribution's own engine is podman: one package, no group to join and no restart."
              : " This distribution's own engine is Docker Engine: the package, its service and the docker group."}
          </p>
          <Elevated
            command={prereq.command ?? ""}
            note={prereq.note}
            label={podman ? "Install podman" : "Install Docker Engine"}
            busy={!!busy}
            onClick={() => act("Installing…", RunAPI.InstallEngine)}
          />
        </>
      )}

      {prereq.state === "manual" && prereq.os === "linux" && (
        <p className="small">
          {prereq.engineInstalled ? (
            <>
              A container engine is installed, and it still refuses:{" "}
              <span className="mono">{prereq.dockerErr}</span>. Check that its service is running
              (<span className="mono">systemctl status docker</span>, or for podman{" "}
              <span className="mono">systemctl --user status podman.socket</span>), then re-detect.
            </>
          ) : (
            <>
              No container engine answered on this system ({prereq.os}
              {prereq.distro ? `/${prereq.distro}` : ""}). If one is installed, start it and re-detect. Otherwise, install
              podman or Docker Engine with your distribution&apos;s packages, or follow Docker&apos;s instructions:{" "}
              <Button variant="link" onClick={() => OpenExternal(prereq.downloadUrl)}>
                {prereq.downloadUrl}
              </Button>
            </>
          )}
        </p>
      )}

      {prereq.state === "managedOffer" && (
        <ManagedOffer
          managed={prereq.managed}
          busy={!!busy}
          onInstall={() => act("Installing Agentry's engine…", RunAPI.InstallManagedEngine)}
        />
      )}
      {prereq.state === "managedStopped" && (
        <ManagedStopped
          managed={prereq.managed}
          err={prereq.dockerErr}
          busy={!!busy}
          onStart={() => act("Starting Agentry's engine…", RunAPI.StartManagedEngine)}
          onRemove={() => act("Removing Agentry's engine…", RunAPI.RemoveManagedEngine)}
        />
      )}
      {prereq.state === "windowsOffer" && (
        <WindowsOffer
          windows={prereq.windows}
          busy={!!busy}
          onInstall={() => act(prereq.windows.wsl === "missing" ? "Installing Windows Subsystem for Linux…" : "Installing Rancher Desktop…", RunAPI.InstallWindowsEngine)}
        />
      )}
      {prereq.state === "windowsRestart" && (
        <>
          <p className="small">
            Windows Subsystem for Linux is installed. Windows has to restart before Rancher Desktop can use it;
            after the restart, open this card and click <strong>Install Rancher Desktop</strong> again to finish.
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
      {prereq.state === "windowsStopped" && (
        <WindowsStopped
          windows={prereq.windows}
          err={prereq.dockerErr}
          busy={!!busy}
          onStart={() => act("Starting Rancher Desktop…", RunAPI.StartWindowsEngine)}
        />
      )}
      {prereq.state === "chosenSilent" && (
        <ChosenSilent
          err={prereq.dockerErr}
          busy={!!busy}
          onAutomatic={() => act(CHECKING_ENGINE, async () => { await RunAPI.SetEngineChoice(""); return RunAPI.PrereqStatus(); })}
        />
      )}
      {(prereq.state === "manual" || prereq.state === "managedOffer" || prereq.state === "windowsOffer") && prereq.os !== "linux" && (
        <EngineChoices os={prereq.os} err={prereq.state === "manual" ? prereq.dockerErr : undefined} intro={prereq.state === "managedOffer" || prereq.state === "windowsOffer" ? "Or install one of these yourself; the app finds whichever answers:" : undefined} />
      )}

      {prereq.dockerErr && !prereq.engineInstalled && prereq.os === "linux" && prereq.state !== "chosenSilent" && <p className="small mono">{prereq.dockerErr}</p>}
      <EngineTried tried={prereq.engine?.tried} />
      {busy ? <Spinner label={busy} /> : <Button onClick={checkAgain}>Re-detect</Button>}

      {confirmRestart && (
        <Modal title="Restart this machine now?" onClose={() => setConfirmRestart(false)}>
          {prereq.os === "windows" ? (
            <p>
              This restarts Windows (<span className="mono">shutdown /r /t 0</span>): every open
              application closes, and unsaved work in them is lost.
            </p>
          ) : (
            <p>
              This asks the system to reboot: every open application closes, and unsaved work in
              them is lost. The app does not run as root — the request goes to the session
              manager (<span className="mono">systemctl reboot</span>), and if it refuses, the
              system&apos;s own elevation prompt appears.
            </p>
          )}
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirmRestart(false)}>Cancel</Button>
            <Button variant="danger" onClick={restartNow}>Restart now</Button>
          </div>
        </Modal>
      )}
    </Card>
  );
}

/** The spinner's label while the engine is asked. */
export const CHECKING_ENGINE = "Checking the container engine…";

/** The traps detection saw, each a warning of its own: a credential helper
 * the docker configuration names and PATH lacks, a socket file with nothing
 * behind it. */
export function EngineWarnings({ warnings }: { warnings?: string[] }) {
  if (!warnings || warnings.length === 0) return null;
  return <>{warnings.map((w) => <Notice key={w} tone="warn">{w}</Notice>)}</>;
}

/** The engine chosen in Settings does not answer: the app asks it alone, so
 * nothing else is offered in its place; start it, or go back to automatic
 * detection, which adopts the first engine that answers. */
function ChosenSilent({ err, busy, onAutomatic }: { err?: string; busy: boolean; onAutomatic: () => void }) {
  return (
    <>
      <p className="small">
        {err ? asSentence(err) : "The container engine you chose does not answer."} Start it and re-detect, or choose
        another under <strong>Settings → Run defaults → Container engine</strong>. Runs on this computer wait for it: the
        app does not switch to another engine on its own.
      </p>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button disabled={busy} onClick={onAutomatic} data-testid="engine-automatic"
          title="Forgets the choice and adopts the first engine that answers, in the app's order">Use automatic detection</Button>
      </div>
    </>
  );
}

/** Each place detection asked and what it said, under the state. */
function EngineTried({ tried }: { tried?: string[] }) {
  if (!tried || tried.length === 0) return null;
  return (
    <details className="small">
      <summary>Where the app looked</summary>
      <ul className="muted small">{tried.map((t) => <li key={t}>{t}</li>)}</ul>
    </details>
  );
}

/** The engines a Mac or a Windows machine can run QuickAgents on, each
 * one sentence and a link, Docker Desktop first; the licence sentence is
 * neutral and links Docker's terms. On Windows the CLI runs inside WSL 2
 * against a Docker Engine there as well. */
const ENGINE_CHOICES: Record<string, { name: string; url: string; blurb: string }[]> = {
  darwin: [
    { name: "Docker Desktop", url: "https://www.docker.com/products/docker-desktop/", blurb: "Docker's own application; the engine most people have." },
    { name: "Colima", url: "https://github.com/abiosoft/colima", blurb: "A Docker Engine in a Lima VM, started from a terminal, with the docker CLI from Homebrew." },
    { name: "Rancher Desktop", url: "https://rancherdesktop.io/", blurb: "SUSE's application; choose its dockerd (moby) engine." },
    { name: "Podman Desktop", url: "https://podman-desktop.io/", blurb: "Red Hat's application, running a podman machine." },
  ],
  windows: [
    { name: "Docker Desktop", url: "https://www.docker.com/products/docker-desktop/", blurb: "Docker's own application; the engine most people have." },
    { name: "Rancher Desktop", url: "https://rancherdesktop.io/", blurb: "SUSE's application; choose its dockerd (moby) engine." },
  ],
};

const DOCKER_LICENCE_SENTENCE = "Docker Desktop is free for personal use and small companies and licensed per seat for others; ";
const DOCKER_TERMS_URL = "https://www.docker.com/legal/docker-subscription-service-agreement/";

function EngineChoices({ os, err, intro }: { os: string; err?: string; intro?: string }) {
  const choices = os === "windows" ? ENGINE_CHOICES.windows : ENGINE_CHOICES.darwin;
  return (
    <>
      <p className="small">
        {intro ?? (
          <>
            No container engine answered on this computer. If one is installed, start it and re-detect.
            Otherwise, install one of these; the app finds whichever answers:
          </>
        )}
      </p>
      <ul className="small">
        {choices.map((c) => (
          <li key={c.name}>
            <Button variant="link" onClick={() => OpenExternal(c.url)}>{c.name}</Button> — {c.blurb}
          </li>
        ))}
      </ul>
      <p className="muted small">
        {DOCKER_LICENCE_SENTENCE}
        <Button variant="link" onClick={() => OpenExternal(DOCKER_TERMS_URL)}>see Docker&apos;s terms</Button>.
      </p>
      {os === "windows" && (
        <p className="muted small">
          The agentry CLI also runs inside WSL 2 against a Docker Engine installed there.
        </p>
      )}
      {err && <p className="small mono">{err}</p>}
    </>
  );
}

/** The one click on a Mac with no engine: the app installs an engine it
 * runs itself (Colima's Docker Engine in a Lima virtual machine, driven by
 * the docker command), three pinned release binaries checked against their
 * sums under the app's own directory, started with the app and stopped
 * with it; no Homebrew, no administrator password, the person's docker
 * context untouched. */
function ManagedOffer({ managed, busy, onInstall }: { managed: app.PrereqView["managed"]; busy: boolean; onInstall: () => void }) {
  if (managed.socketPathError) {
    return (
      <div className="managed-engine">
        <p className="small">
          The app cannot run an engine of its own on this computer: <span className="mono">{managed.socketPathError}</span>
        </p>
      </div>
    );
  }
  return (
    <div className="managed-engine">
      <p className="small">
        No container engine answered on this computer. The app can install one it runs itself:{" "}
        <strong>Agentry&apos;s engine</strong>, a Docker Engine in a Linux virtual machine on this Mac
        (Colima and Lima), with the docker command that drives it.
      </p>
      <ul className="muted small">
        <li>{managed.versions} ({managed.licences}), about {managed.downloadMb ?? 75} MB, each file checked against the sum this app pins.</li>
        <li>
          Installed under <span className="mono">{managed.dir}</span>
          {managed.vmDir && <>, its virtual machine&apos;s disk under <span className="mono">{managed.vmDir}</span></>}; the app starts it
          when it opens and stops it when it quits. Remove takes both away, from here, any time.
        </li>
        <li>No Homebrew, no administrator password, and your own docker context is left as it is.</li>
      </ul>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button variant="primary" disabled={busy} onClick={onInstall} data-testid="install-managed-engine">Install Agentry&apos;s engine</Button>
      </div>
    </div>
  );
}

/** The app's engine installed and not answering: start it, or remove it
 * with its virtual machine and data. */
function ManagedStopped({ managed, err, busy, onStart, onRemove }: {
  managed: app.PrereqView["managed"]; err?: string; busy: boolean; onStart: () => void; onRemove: () => void;
}) {
  if (managed.starting) {
    return (
      <div className="managed-engine">
        <p className="small"><strong>Agentry&apos;s engine</strong> is starting; its virtual machine takes a moment to boot.</p>
        <Spinner label="Starting Agentry's engine…" />
      </div>
    );
  }
  return (
    <div className="managed-engine">
      <p className="small">
        <strong>Agentry&apos;s engine</strong> ({managed.versions}) is installed under{" "}
        <span className="mono">{managed.dir}</span> and is not running
        {managed.current ? "" : "; this app carries newer versions and updates it at its next start"}.
        {err && <> <span className="mono">{err}</span></>}
      </p>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button variant="danger" disabled={busy} onClick={onRemove} data-testid="remove-managed-engine">Remove</Button>
        <Button variant="primary" disabled={busy} onClick={onStart} data-testid="start-managed-engine">Start</Button>
      </div>
    </div>
  );
}

/** The one click on Windows with no engine: Rancher Desktop with its
 * dockerd engine, from SUSE's installer checked against the SHA-512 its
 * release publishes, installed for this user alone. Where Windows Subsystem
 * for Linux is not on, the click installs it first (Windows asks for
 * administrator consent), and Windows restarts before the second click. */
function WindowsOffer({ windows, busy, onInstall }: { windows: app.PrereqView["windows"]; busy: boolean; onInstall: () => void }) {
  const wslMissing = windows.wsl === "missing";
  return (
    <div className="managed-engine">
      <p className="small">
        No container engine answered on this computer. The app can install <strong>Rancher Desktop</strong> for
        you, with its dockerd engine, which serves the Docker API runs need.
      </p>
      <ul className="muted small">
        <li>Rancher Desktop {windows.version} (SUSE, {windows.licence}), about {windows.downloadMb ?? 640} MB from its GitHub release, checked against the SHA-512 the release publishes.</li>
        <li>Installed for your account alone, with no administrator password; Kubernetes stays off. It is your own application afterwards, listed in Windows&apos; installed apps, and the app starts it when it opens.</li>
        {wslMissing && (
          <li>
            It needs Windows Subsystem for Linux, which is not on yet: the click first asks Windows to install it,
            which needs administrator consent once and then a restart. After the restart, click again to finish.
          </li>
        )}
      </ul>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button variant="primary" disabled={busy} onClick={onInstall} data-testid="install-windows-engine">
          {wslMissing ? "Install WSL, then Rancher Desktop" : "Install Rancher Desktop"}
        </Button>
      </div>
    </div>
  );
}

/** Rancher Desktop installed and its engine silent: start it. */
function WindowsStopped({ windows, err, busy, onStart }: {
  windows: app.PrereqView["windows"]; err?: string; busy: boolean; onStart: () => void;
}) {
  if (windows.starting) {
    return (
      <div className="managed-engine">
        <p className="small"><strong>Rancher Desktop</strong> is starting; its first start sets up its WSL distributions and takes a few minutes.</p>
        <Spinner label="Starting Rancher Desktop…" />
      </div>
    );
  }
  return (
    <div className="managed-engine">
      <p className="small">
        <strong>Rancher Desktop</strong> is installed and its engine is not answering.
        {err && <> <span className="mono">{err}</span></>}
      </p>
      <p className="muted small">
        Start quits Rancher Desktop first if it is running, then starts it again.
      </p>
      <p className="muted small">
        If it runs its containerd engine, choose dockerd (moby) under Rancher Desktop&apos;s Preferences, Container Engine:
        containerd serves no Docker API.
      </p>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button variant="primary" disabled={busy} onClick={onStart} data-testid="start-windows-engine">Start Rancher Desktop</Button>
      </div>
    </div>
  );
}

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
