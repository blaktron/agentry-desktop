import React, { useEffect, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as RunhostsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runhostsapi";
import { OpenExternal } from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/app";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import * as run from "../../bindings/github.com/blaktron/agentry-desktop/internal/run/models";
import { Button, Card, ErrorBox, Input, Modal, Notice, Select, Spinner } from "../ui";
import {
  CLAUDE_SUGGESTED, COPILOT_PLANS, CREDITS_DOCS_URL, CREDITS_LOCK, PROVIDERS, copilotModelLabel, copilotOffered, creditsLines, keptFor, modelAsk, modelOptionLabel, providerChoice, signInSaveHint, waysFor,
  type KeptSignIn, type WayId,
} from "../modelSetup";
import { parseContextWindow } from "../providerForm";
import {
  addHeld, assignedLine, availableLine, availableOn, countLine, runhostMoveTargets, fixIsKey, fixIsSignIn, paidWith, providerName, refusalLine, removalAsk, removalHeld, removalNeedsTarget, stateBadge, suggestedName, userLine,
  type ProviderRow, type RunhostModels, type SavedModelRow,
} from "../savedModels";
import { ProviderLogo } from "../providerLogos";
import { KeptSignIns, SignInWay } from "./ModelSignIn";
import { ProviderTiles } from "./ProviderTiles";
import { ManagedNotes, useManagedRows } from "./ManagedNotes";
import { providerLocks } from "../managed";

/** Settings → Models: the saved models runs on this computer use, the
 *  default starred, and the providers that pay for them. A saved model is a
 *  provider plus a model under a name; a second model on a provider asks for
 *  no new key or sign-in. Every save is checked first, and every rule (the
 *  cap of ten, the default, a removal refused until what names a model is
 *  moved) is the CLI's, held on the Go side. */
export function ModelsSettings({ onOpenAccount }: { onOpenAccount?: () => void }) {
  const [list, setList] = useState<run.ModelsList | null>(null);
  const [kept, setKept] = useState<KeptSignIn[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const managedRows = useManagedRows();
  const [adding, setAdding] = useState(false);
  const [addOn, setAddOn] = useState<ProviderRow | null>(null);
  const [editing, setEditing] = useState<SavedModelRow | null>(null);
  const [removing, setRemoving] = useState<{ provider?: ProviderRow; model?: SavedModelRow } | null>(null);
  const [keyFor, setKeyFor] = useState<ProviderRow | null>(null);
  const [signInFor, setSignInFor] = useState<ProviderRow | null>(null);
  const [checking, setChecking] = useState<{ model: SavedModelRow; notice: string } | null>(null);
  // The runhosts this computer is paired with, each with the saved models it
  // holds as last read: "Available on".
  const [runhosts, setRunhosts] = useState<RunhostModels[]>([]);
  const [runhostBusy, setRunhostBusy] = useState<string | null>(null);
  const [removingThere, setRemovingThere] = useState<{ model: SavedModelRow; runhost: RunhostModels; error?: string } | null>(null);

  async function refresh() {
    try {
      setList(await RunAPI.SavedModels());
    } catch (e) {
      setError(String(e));
    }
    try {
      setKept(((await RunAPI.ModelSignIns()) ?? []) as KeptSignIn[]);
    } catch { /* the list stays */ }
  }
  useEffect(() => { void refresh(); }, []);
  // The runhosts, as this computer last read their saved models; each one
  // not read yet is asked once.
  useEffect(() => {
    let live = true;
    Promise.resolve().then(() => RunhostsAPI.List()).then((l) => {
      if (!live) return;
      const rows = (l ?? []).map((r) => ({ name: r.name, models: r.models }));
      setRunhosts(rows);
      for (const r of rows.filter((x) => x.models == null)) {
        RunhostsAPI.Models(r.name)
          .then((v) => live && v && setRunhosts((prev) => prev.map((x) => x.name === r.name ? { ...x, models: v.models } : x)))
          .catch(() => {});
      }
    }).catch(() => {});
    return () => { live = false; };
  }, []);
  const tookModels = (name: string, v: app.RunhostModelsView | null) => {
    if (v) setRunhosts((prev) => prev.map((x) => x.name === name ? { ...x, models: v.models } : x));
  };

  if (!list) {
    return (
      <section id="settings-part-models" className="settings-part">
        <h2>Models</h2>
        <ErrorBox error={error} onRetry={() => { setError(null); void refresh(); }} />
        {!error && <Spinner label="Reading the saved models…" />}
      </section>
    );
  }

  const models = (list.models ?? []) as SavedModelRow[];
  const providers = (list.providers ?? []) as ProviderRow[];
  const held = addHeld(models.length, list.max);
  // What the last act did, in place of any error an earlier one left.
  const say = (line: string) => { setError(null); setNotice(line); };
  const providerOf = (m: SavedModelRow) => providers.find((p) => p.id === m.provider);

  async function makeDefault(m: SavedModelRow) {
    setBusy(m.id);
    setError(null);
    setNotice(null);
    try {
      await RunAPI.SetDefaultModel(m.id);
      setNotice(`${m.name} is your default model now.`);
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  // Available on: the runhost checks the model before it keeps it; a key
  // goes there only once the person agrees in a native dialog.
  async function makeAvailable(m: SavedModelRow, rh: string) {
    setRunhostBusy(`${m.id}@${rh}`);
    setError(null);
    setNotice(null);
    try {
      tookModels(rh, await RunhostsAPI.MakeModelAvailable(rh, m.id));
      setNotice(`${rh} checked ${m.name} and holds it now: its agents and schedules can run on it.`);
    } catch (e) {
      if (!/not confirmed/i.test(String(e))) setError(String(e));
    } finally {
      setRunhostBusy(null);
    }
  }
  async function removeThere(m: SavedModelRow, rh: RunhostModels, moveTo: string) {
    setRunhostBusy(`${m.id}@${rh.name}`);
    setError(null);
    setNotice(null);
    try {
      tookModels(rh.name, await RunhostsAPI.RemoveRunhostModel(rh.name, m.id, moveTo, false));
      setRemovingThere(null);
      setNotice(`${rh.name} no longer holds ${m.name}.`);
    } catch (e) {
      setRemovingThere({ model: m, runhost: rh, error: String(e) });
    } finally {
      setRunhostBusy(null);
    }
  }

  async function check(m: SavedModelRow, agreed: boolean) {
    setBusy(m.id);
    setError(null);
    setNotice(null);
    try {
      const res = await RunAPI.CheckSavedModel(m.id, agreed);
      if (res?.needsAgreement) {
        setChecking({ model: m, notice: res.needsAgreement });
      } else if (res?.refusal) {
        setError(refusalLine(res.refusal));
      } else {
        setNotice(`${m.name} passed its check.${res?.resolved && res.resolved !== m.model ? ` ${m.model} is ${res.resolved} today.` : ""}`);
      }
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <section id="settings-part-models" className="settings-part" data-saved-models>
      <h2>Models</h2>
      <p className="muted small">
        Runs, schedules and agents on this computer use your default model unless you pick another for them. A model
        is a provider and one of its models, under a name you choose. You can keep up to {list.max}.
      </p>
      <ManagedNotes rows={managedRows} part="provider" />
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {list.usersErr && <Notice tone="warn">What uses each model could not be read: {list.usersErr}</Notice>}

      {!list.setUp ? (
        <Card>
          <p>No model is saved on this computer yet. Add a provider to save your first model. It becomes your default.</p>
          <p className="row"><Button variant="primary" onClick={() => setAdding(true)}>Add a provider…</Button></p>
        </Card>
      ) : (
        <Card pad={false}>
          <ul className="saved-models" aria-label="Saved models">
            {models.map((m) => {
              const p = providerOf(m);
              const badge = p ? stateBadge(p) : null;
              return (
                <li key={m.id} className="saved-model" data-saved-model={m.name} data-default={m.default ? "yes" : undefined}>
                  <div className="saved-model-head">
                    <span className={`saved-model-star${m.default ? " on" : ""}`} aria-label={m.default ? "Default" : undefined} title={m.default ? "Your default model" : undefined}>{m.default ? "★" : "☆"}</span>
                    <strong className="selectable">{m.name}</strong>
                    {m.default && <span className="badge kind">Default</span>}
                    {badge && badge.tone === "warn" && <span className="badge warn" data-model-state>{badge.label}</span>}
                    <span className="grow" />
                    <span className="row nowrap">
                      {!m.default && <Button variant="link" disabled={!!busy} busy={busy === m.id} busyLabel="…" onClick={() => void makeDefault(m)}>Make default</Button>}
                      <Button variant="link" disabled={!!busy} onClick={() => void check(m, false)}>Check</Button>
                      <Button variant="link" disabled={!!busy} onClick={() => setEditing(m)}>Edit…</Button>
                      <Button variant="link" disabled={!!busy} onClick={() => setRemoving({ model: m })}>Remove…</Button>
                    </span>
                  </div>
                  <div className="muted small selectable">
                    {p ? providerName(p) : m.provider} · <span className="mono">{m.model}</span>
                    {m.contextWindow ? <> · context {m.contextWindow.toLocaleString("en-US")} tokens</> : null}
                  </div>
                  {badge && badge.tone === "warn" && p && (
                    <p className="small saved-model-fix">
                      {badge.fix}{" "}
                      {fixIsKey(p) && <Button variant="link" onClick={() => setKeyFor(p)}>Add the key…</Button>}
                      {fixIsSignIn(p) && <Button variant="link" onClick={() => setSignInFor(p)}>Sign in again…</Button>}
                      {p.way === "credits" && onOpenAccount && <Button variant="link" onClick={onOpenAccount}>Sign in…</Button>}
                    </p>
                  )}
                  <p className="muted small">{assignedLine(m)}</p>
                  {runhosts.length > 0 && (
                    <div className="small saved-model-runhosts" data-available-on={m.name}>
                      <span className="muted">{availableLine(m.id, runhosts)}</span>{" "}
                      {availableOn(m.id, runhosts).map((a) => {
                        const rh = runhosts.find((x) => x.name === a.runhost)!;
                        const key = `${m.id}@${a.runhost}`;
                        return a.holds
                          ? <Button key={a.runhost} variant="link" disabled={!!runhostBusy} busy={runhostBusy === key} busyLabel="…"
                              data-remove-from={a.runhost} onClick={() => setRemovingThere({ model: m, runhost: rh })}>Remove from {a.runhost}…</Button>
                          : <Button key={a.runhost} variant="link" disabled={!!runhostBusy || a.unknown} busy={runhostBusy === key} busyLabel={`Checking on ${a.runhost}…`}
                              data-make-available-on={a.runhost} onClick={() => void makeAvailable(m, a.runhost)}>Make available on {a.runhost}</Button>;
                      })}
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
          <p className="muted small saved-models-count">{countLine(models.length, list.max)}{held ? ` ${held}` : ""}</p>
        </Card>
      )}

      {list.setUp && (
        <>
          <h3>Providers</h3>
          <p className="muted small">
            How your models are paid for: a sign-in to a plan you have, a connected account, an API key kept in this
            computer&apos;s keychain, your Agentry credits, or an endpoint of your own. Add a model on a provider here
            and it asks for nothing again.
          </p>
          <Card pad={false}>
            <ul className="saved-providers" aria-label="Providers">
              {providers.map((p) => {
                const badge = stateBadge(p);
                const on = models.filter((m) => p.models.includes(m.id));
                return (
                  <li key={p.id} className="saved-provider" data-provider-row={p.id}>
                    <div className="saved-model-head">
                      <ProviderLogo provider={p.vendor} size={24} />
                      <strong>{providerName(p)}</strong>
                      <span className={`badge ${badge.tone}`}>{badge.label}</span>
                      <span className="grow" />
                      <span className="row nowrap">
                        <Button disabled={!!busy || !!held} title={held} onClick={() => setAddOn(p)}>Add a model</Button>
                        <Button variant="link" disabled={!!busy} onClick={() => setRemoving({ provider: p })}>Remove…</Button>
                      </span>
                    </div>
                    <div className="muted small selectable">
                      {paidWith(p)}{p.baseUrl ? <> · <span className="mono">{p.baseUrl}</span></> : null}
                      {" · "}{on.length === 0 ? "no saved model" : on.map((m) => m.name).join(", ")}
                    </div>
                    {badge.fix && (
                      <p className="small saved-model-fix">
                        {badge.fix}{" "}
                        {fixIsKey(p) && <Button variant="link" onClick={() => setKeyFor(p)}>Add the key…</Button>}
                        {fixIsSignIn(p) && <Button variant="link" onClick={() => setSignInFor(p)}>Sign in again…</Button>}
                        {p.way === "credits" && onOpenAccount && <Button variant="link" onClick={onOpenAccount}>Sign in…</Button>}
                      </p>
                    )}
                  </li>
                );
              })}
            </ul>
          </Card>
          <p className="row">
            <Button disabled={!!busy || !!held} title={held} onClick={() => setAdding(true)}>Add a provider…</Button>
            {held && <span className="muted small">{held}</span>}
          </p>
          <KeptSignIns kept={kept} onChanged={() => void refresh()} />
        </>
      )}

      {adding && (
        <AddProviderDialog
          taken={providers.map((p) => p.vendor).filter((v) => v !== "custom")}
          names={models.map((m) => m.name)}
          first={!list.setUp}
          kept={kept}
          onKept={() => void refresh()}
          onClose={() => setAdding(false)}
          onSaved={(sm) => { setAdding(false); say(savedNotice(sm)); void refresh(); }}
        />
      )}
      {addOn && (
        <AddModelDialog
          provider={addOn}
          names={models.map((m) => m.name)}
          onClose={() => setAddOn(null)}
          onSaved={(sm) => { setAddOn(null); say(savedNotice(sm)); void refresh(); }}
        />
      )}
      {editing && (
        <EditModelDialog
          model={editing}
          provider={providerOf(editing)}
          onClose={() => setEditing(null)}
          onSaved={(sm) => { setEditing(null); say(`${sm.saved?.name ?? "The model"} is saved.`); void refresh(); }}
        />
      )}
      {removing && (
        <RemoveDialog
          models={models}
          model={removing.model}
          provider={removing.provider}
          onClose={() => setRemoving(null)}
          onRemoved={(line) => { setRemoving(null); say(line); void refresh(); }}
        />
      )}
      {removingThere && (
        <RemoveFromRunhost model={removingThere.model} runhost={removingThere.runhost} error={removingThere.error}
          busy={runhostBusy === `${removingThere.model.id}@${removingThere.runhost.name}`}
          onRemove={(moveTo) => void removeThere(removingThere.model, removingThere.runhost, moveTo)}
          onClose={() => setRemovingThere(null)} />
      )}
      {keyFor && (
        <KeyDialog provider={keyFor} onClose={() => setKeyFor(null)} onSaved={() => { setKeyFor(null); say(`The key for ${providerName(keyFor)} is kept in this computer's keychain.`); void refresh(); }} />
      )}
      {signInFor && (
        <Modal title={`Sign in again: ${providerName(signInFor)}`} onClose={() => setSignInFor(null)}>
          {(() => {
            const way = waysFor(signInFor.vendor).find((w) => w.id === signInFor.way);
            return way ? (
              <SignInWay way={way} kept={kept} onSignedIn={() => { setSignInFor(null); void refresh(); }} onRemoved={() => void refresh()} />
            ) : <p>This provider has no sign-in here.</p>;
          })()}
          <p className="row"><Button onClick={() => setSignInFor(null)}>Close</Button></p>
        </Modal>
      )}
      {checking && (
        <ClaudeAgreement
          notice={checking.notice}
          onCancel={() => setChecking(null)}
          onAgree={() => { const m = checking.model; setChecking(null); void check(m, true); }}
        />
      )}
    </section>
  );
}

/** What a save says once it is done: the default when it became so, the
 *  window an endpoint reported, the alias Claude Code resolved. */
function savedNotice(res: run.SaveResult): string {
  const sm = res.saved;
  if (!sm) return "Saved.";
  const parts = [sm.default ? `${sm.name} is saved, and it is your default model.` : `${sm.name} is saved.`];
  if (res.resolved && res.resolved !== sm.model) parts.push(`${sm.model} is ${res.resolved} today.`);
  if (res.windowSource && sm.contextWindow) parts.push(`Its context window is ${sm.contextWindow.toLocaleString("en-US")} tokens, as ${res.windowSource} reports it. Edit changes it.`);
  if (res.unknownWindow) parts.push(res.unknownWindow.charAt(0).toUpperCase() + res.unknownWindow.slice(1) + ".");
  return parts.join(" ");
}

/** The dialog before a Claude subscription's check: the check sends one
 *  message, which counts toward the plan's usage. */
function ClaudeAgreement({ notice, onAgree, onCancel }: { notice: string; onAgree: () => void; onCancel: () => void }) {
  return (
    <Modal title="Check this model?" onClose={onCancel}>
      <p data-claude-agreement>{notice}</p>
      <p className="row" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onCancel}>Cancel</Button>
        <Button variant="primary" onClick={onAgree}>Send it and save</Button>
      </p>
    </Modal>
  );
}

/** A save's outcome the dialogs share: the check's refusal with Retry where
 *  the same save may be tried again, and the ids the provider does offer to
 *  pick from. */
function RefusalBox({ refusal, onRetry, onPick, busy }: { refusal: run.Refusal; onRetry: () => void; onPick: (id: string) => void; busy: boolean }) {
  const offered = (refusal.offered ?? []).slice(0, 30);
  return (
    <div className="error-box selectable" role="alert" data-check-refusal={refusal.kind}>
      <span className="error-box-cause">{refusalLine(refusal)}</span>
      {refusal.retry && <Button className="error-box-retry" onClick={onRetry} busy={busy} busyLabel="Checking…">Retry</Button>}
      {offered.length > 0 && (
        <div className="row wrap" style={{ gap: 6, marginTop: 6 }}>
          {offered.map((id) => <Button key={id} variant="link" className="mono" onClick={() => onPick(id)}>{id}</Button>)}
        </div>
      )}
    </div>
  );
}

/** The model, name, window and default fields every save asks for. */
function ModelFields({ provider, way, model, setModel, name, setName, window, setWindow, makeDefault, setMakeDefault, first, options, suggested, busy, listHint }: {
  provider: string; way: string;
  model: string; setModel: (v: string) => void;
  name: string; setName: (v: string) => void;
  window: string; setWindow: (v: string) => void;
  makeDefault: boolean; setMakeDefault: (v: boolean) => void;
  first: boolean;
  options: { id: string; label: string }[];
  suggested: string[];
  busy: boolean;
  listHint?: React.ReactNode;
}) {
  const { error: windowError } = parseContextWindow(window);
  const own = provider === "custom";
  return (
    <>
      <label className="field">
        <span className="field-label">Model</span>
        {options.length > 0 ? (
          <Select value={model} onChange={(e) => setModel(e.target.value)} disabled={busy} aria-label="Model">
            {!options.some((o) => o.id === model) && <option value={model}>{model || "pick a model"}</option>}
            {options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}
          </Select>
        ) : (
          <>
            <Input type="text" className="mono" list="saved-model-suggestions" value={model} onChange={(e) => setModel(e.target.value)} disabled={busy}
              placeholder={way === "claude" ? "sonnet, opus, or a Claude model id" : way === "credits" ? "agentry" : "the model id"} aria-label="Model" />
            <datalist id="saved-model-suggestions">{suggested.map((s) => <option key={s} value={s} />)}</datalist>
          </>
        )}
        {listHint && <span className="muted small field-hint">{listHint}</span>}
      </label>
      {own && (
        <label className="field">
          <span className="field-label">Context window (optional)</span>
          <Input type="text" inputMode="numeric" className="mono" value={window} onChange={(e) => setWindow(e.target.value)} disabled={busy}
            placeholder="tokens, as the server runs the model" aria-label="Context window" />
          <span className="muted small field-hint">
            {windowError ? `The window must be ${windowError}. ` : null}
            Leave it empty and the check fills it from what the endpoint reports. Type a number to use your own.
          </span>
        </label>
      )}
      <label className="field">
        <span className="field-label">Name</span>
        <Input type="text" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} maxLength={64} placeholder="what you call this model" aria-label="Name" />
      </label>
      <label className="row" style={{ gap: 6 }}>
        <input type="checkbox" checked={first || makeDefault} disabled={busy || first} onChange={(e) => setMakeDefault(e.target.checked)} />
        <span>{first ? "This is your first model, so it is your default." : "Make it my default model"}</span>
      </label>
    </>
  );
}

/** How a save goes: refused names and the cap are errors; the check's
 *  refusal is a result with Retry; a Claude subscription's check asks first. */
function useSave(onSaved: (res: run.SaveResult) => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refusal, setRefusal] = useState<run.Refusal | null>(null);
  const [agreement, setAgreement] = useState<string | null>(null);
  async function save(call: (agreed: boolean) => Promise<run.SaveResult | null>, agreed = false) {
    setBusy(true);
    setError(null);
    setRefusal(null);
    try {
      const res = await call(agreed);
      if (!res) return;
      if (res.needsAgreement) setAgreement(res.needsAgreement);
      else if (res.refusal) setRefusal(res.refusal);
      else if (res.saved) onSaved(res);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return { busy, error, setError, refusal, agreement, setAgreement, save };
}

/** Add a provider: its tile, how it is paid, the credential, and its first
 *  saved model, saved in one act once the model is checked. */
function AddProviderDialog({ taken, names, first, kept, onKept, onClose, onSaved }: {
  taken: string[]; names: string[]; first: boolean; kept: KeptSignIn[];
  onKept: () => void; onClose: () => void; onSaved: (res: run.SaveResult) => void;
}) {
  const managedLocks = providerLocks(useManagedRows());
  const startOn = PROVIDERS.find((p) => !taken.includes(p.id) && p.id !== "agentry")?.id ?? "custom";
  const [vendor, setVendor] = useState(startOn);
  const [way, setWay] = useState<WayId>(waysFor(startOn)[0].id);
  const [key, setKey] = useState("");
  const [endpointName, setEndpointName] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [copilotPlan, setCopilotPlan] = useState("individual");
  const [model, setModel] = useState(providerChoice(startOn)?.model ?? "");
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [window, setWindow] = useState("");
  const [makeDefault, setMakeDefault] = useState(false);
  const [catalog, setCatalog] = useState<app.CatalogView | null>(null);
  const [planModels, setPlanModels] = useState<string[]>([]);
  const [copilotModels, setCopilotModels] = useState<run.CopilotModel[] | null>(null);
  const [listing, setListing] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [credits, setCredits] = useState<app.CreditsOfferView | null>(null);
  const [status, setStatus] = useState<app.SetupStatusView | null>(null);
  const [warning, setWarning] = useState("");
  const s = useSave(onSaved);

  useEffect(() => {
    RunAPI.CreditsOffer().then(setCredits).catch(() => undefined);
    RunAPI.SetupStatus().then(setStatus).catch(() => undefined);
  }, []);
  useEffect(() => {
    const u = baseURL.trim();
    if (vendor !== "custom" || !u) { setWarning(""); return; }
    let live = true;
    RunAPI.EndpointWarning(u).then((w) => { if (live) setWarning(w); }).catch(() => { if (live) setWarning(""); });
    return () => { live = false; };
  }, [baseURL, vendor]);

  const ways = waysFor(vendor);
  const currentWay = ways.find((w) => w.id === way) ?? ways[0];
  const keptHere = currentWay ? keptFor(currentWay, kept) : undefined;
  const keyStored = !!status?.keysStored?.includes(vendor);
  const pickName = (v: string) => { setName(v); setNameTouched(true); };
  const takeModel = (v: string) => { setModel(v); if (!nameTouched) setName(suggestedName(v, names)); };

  function pickVendor(id: string) {
    setVendor(id);
    const w = waysFor(id)[0].id;
    setWay(w);
    setKey("");
    setCatalog(null);
    setListError(null);
    setCopilotModels(null);
    takeModel(w === "key" ? providerChoice(id)?.model ?? "" : modelAsk(id, w, []).defaultModel);
  }
  function pickWay(w: WayId) {
    setWay(w);
    setCatalog(null);
    setListError(null);
    takeModel(w === "key" ? providerChoice(vendor)?.model ?? "" : modelAsk(vendor, w, planModels).defaultModel);
    if (w === "chatgpt") {
      RunAPI.ChatGPTModels().then((m) => { setPlanModels(m ?? []); if ((m ?? []).length > 0) takeModel((m ?? [])[0]); }).catch(() => undefined);
    }
  }

  async function getModels() {
    setListing("Asking the endpoint which models it serves…");
    setListError(null);
    try {
      // An endpoint being added is asked with the key typed for it or none,
      // never one kept for another provider.
      const cat = vendor === "custom" ? await RunAPI.EndpointModels(baseURL.trim(), key) : await RunAPI.ListModels(vendor, "", key);
      setCatalog({ models: cat.models ?? [] } as app.CatalogView);
      if (!(cat.models ?? []).some((m) => m.id === model) && cat.models?.[0]) takeModel(cat.models[0].id);
    } catch (e) {
      setCatalog(null);
      setListError(`Could not list the models: ${e instanceof Error ? e.message : e}. You can still type a model id.`);
    } finally {
      setListing(null);
    }
  }
  async function listCopilot() {
    setListing("Asking Copilot CLI which models your plan offers. A first list builds images and can take minutes…");
    setListError(null);
    try {
      const got = (await RunAPI.CopilotModels(copilotPlan)) ?? [];
      setCopilotModels(got);
    } catch (e) {
      setListError(`Could not list your plan's models (${e}). Type the model id instead.`);
    } finally {
      setListing(null);
    }
  }

  const np: run.NewProvider = { vendor, way, name: endpointName.trim(), baseUrl: vendor === "custom" ? baseURL.trim() : "", takesKey: vendor === "custom" && !!key.trim(), copilotPlan: way === "copilot" ? copilotPlan : "", id: "" };
  const { tokens: windowTokens, error: windowError } = parseContextWindow(window);
  const held = vendor === "custom" && (!endpointName.trim() || !baseURL.trim()) ? "name the endpoint and give its address"
    : way === "key" && vendor !== "custom" && !key.trim() && !keyStored ? "paste the key"
      : way === "credits" && !credits?.signedIn ? "sign in to your account first"
        : way !== "key" && way !== "credits" && currentWay ? signInSaveHint(currentWay, keptHere, model)
          : !model.trim() ? "pick or type a model" : !name.trim() ? "name the model" : windowError ? `the context window must be ${windowError}` : "";

  async function add(agreed: boolean) {
    await s.save(async (ok) => {
      if (way === "key" && key.trim()) {
        const k = key;
        setKey("");
        try {
          await RunAPI.StoreProviderKey(np, k);
        } catch (e) {
          throw new Error(`${e}. The key you typed was cleared; type it again to retry.`);
        }
        RunAPI.SetupStatus().then(setStatus).catch(() => undefined);
      }
      return RunAPI.AddProviderModel(np, { name: name.trim(), model: model.trim(), contextWindow: windowTokens, makeDefault, agreedToMessage: ok });
    }, agreed);
  }

  const options = way === "chatgpt" ? planModels.map((m) => ({ id: m, label: m }))
    : way === "copilot" && copilotModels ? copilotOffered(copilotModels.map((m) => m.id)).map((id) => ({ id, label: copilotModelLabel(id, copilotModels) }))
      : way === "credits" ? [{ id: "agentry", label: "agentry — the model Agentry chooses" }]
        : catalog ? catalog.models.map((m) => ({ id: m.id, label: modelOptionLabel(m) })) : [];
  const locked: Record<string, string> = {};
  for (const v of taken) locked[v] = "Added";
  for (const [id, why] of Object.entries(managedLocks)) if (!locked[id]) locked[id] = why;
  if (!credits?.signedIn && !locked.agentry) locked.agentry = CREDITS_LOCK;

  return (
    <Modal title="Add a provider" onClose={onClose} wide>
      <div data-add-provider>
        <ProviderTiles value={vendor} onPick={pickVendor} disabled={s.busy} locked={locked} />
        {ways.length > 1 && (
          <div className="field way-choice">
            <span className="field-label">How to pay for the models</span>
            {ways.map((w) => (
              <label key={w.id} className="row" style={{ gap: 6 }}>
                <input type="radio" name="add-provider-way" checked={way === w.id} disabled={s.busy} onChange={() => pickWay(w.id)} />
                <span>{w.label}</span>
              </label>
            ))}
          </div>
        )}
        {way === "credits" && (
          <div className="field credits-offer">
            {creditsLines(credits ?? { signedIn: false, balance: 0, balanceKnown: false, model: "", unavailable: "" }).map((line) => <p key={line} className="small">{line}</p>)}
            <span className="muted small field-hint"><Button variant="link" onClick={() => void OpenExternal(CREDITS_DOCS_URL)}>How paying with credits works</Button></span>
          </div>
        )}
        {currentWay && way !== "key" && way !== "credits" && (
          <>
            <SignInWay way={currentWay} kept={kept} disabled={s.busy}
              onSignedIn={(m) => { if (m.length > 0) { setPlanModels(m); takeModel(m[0]); } onKept(); }}
              onRemoved={onKept} />
            {way === "copilot" && (
              <label className="field">
                <span className="field-label">Your Copilot plan</span>
                <Select value={copilotPlan} onChange={(e) => { setCopilotPlan(e.target.value); setCopilotModels(null); }} disabled={s.busy}>
                  {COPILOT_PLANS.map((c) => <option key={c.id} value={c.id}>{c.label}</option>)}
                </Select>
              </label>
            )}
          </>
        )}
        {way === "key" && (
          <>
            {vendor === "custom" && (
              <>
                <label className="field">
                  <span className="field-label">Endpoint name</span>
                  <Input type="text" value={endpointName} onChange={(e) => setEndpointName(e.target.value)} disabled={s.busy} placeholder="Home GPU" aria-label="Endpoint name" />
                </label>
                <label className="field">
                  <span className="field-label">Endpoint URL</span>
                  <Input type="text" className="mono" value={baseURL} onChange={(e) => { setBaseURL(e.target.value); setCatalog(null); }} disabled={s.busy} placeholder="https://llm.example.net/v1" aria-label="Endpoint URL" />
                  <span className="muted small field-hint">An OpenAI-compatible endpoint. A run on it can reach this host and nothing else.</span>
                </label>
                {warning && <Notice tone="warn">{warning}</Notice>}
              </>
            )}
            <label className="field">
              <span className="field-label">{vendor === "custom" ? "API key (if the endpoint takes one)" : "API key"}</span>
              <div className="row">
                <Input type="password" className="grow mono" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} disabled={s.busy}
                  placeholder={keyStored && vendor !== "custom" ? "a key is kept on this computer; paste a new one to replace it" : "Paste the key"} aria-label="API key" />
                <Button disabled={s.busy || !!listing || (vendor === "custom" ? !baseURL.trim() : !key.trim() && !keyStored)} onClick={() => void getModels()}>Get models</Button>
              </div>
              <span className="muted small field-hint">Kept in this computer&apos;s keychain, never in a file, and never shown here.</span>
            </label>
          </>
        )}
        <ModelFields provider={vendor} way={way} model={model} setModel={takeModel} name={name} setName={pickName}
          window={window} setWindow={setWindow} makeDefault={makeDefault} setMakeDefault={setMakeDefault} first={first}
          options={options} suggested={way === "claude" ? CLAUDE_SUGGESTED : way === "copilot" ? ["auto"] : []}
          busy={s.busy}
          listHint={way === "copilot" && keptHere && !keptHere.broken ? <Button variant="link" disabled={!!listing} onClick={() => void listCopilot()}>{copilotModels ? "List again" : "List my plan's models"}</Button>
            : way === "connect" ? "Agentry does not list these models: type the id the provider shows." : undefined} />
        {listing && <Spinner label={listing} />}
        {listError && <p className="muted small">{listError}</p>}
        <ErrorBox error={s.error} />
        {s.refusal && <RefusalBox refusal={s.refusal} busy={s.busy} onRetry={() => void add(false)} onPick={(id) => takeModel(id)} />}
        <p className="muted small">Saving checks the model first: the provider must offer it. Nothing is saved if the check fails.</p>
        <p className="row" style={{ justifyContent: "flex-end" }}>
          <Button onClick={onClose} disabled={s.busy}>Cancel</Button>
          <Button variant="primary" disabled={s.busy || !!held} title={held} busy={s.busy} busyLabel="Checking…" onClick={() => void add(false)}>Check and save</Button>
        </p>
        {held && !s.busy && <p className="muted small" style={{ textAlign: "right" }}>To save: {held}.</p>}
      </div>
      {s.agreement && <ClaudeAgreement notice={s.agreement} onCancel={() => s.setAgreement(null)} onAgree={() => { s.setAgreement(null); void add(true); }} />}
    </Modal>
  );
}

/** Add a model on a provider this computer has: its models from the
 *  credential it keeps, nothing asked again. */
function AddModelDialog({ provider, names, onClose, onSaved }: {
  provider: ProviderRow; names: string[]; onClose: () => void; onSaved: (res: run.SaveResult) => void;
}) {
  const [offered, setOffered] = useState<run.ProviderModels | null>(null);
  const [copilotModels, setCopilotModels] = useState<run.CopilotModel[] | null>(null);
  const [listing, setListing] = useState<string | null>("Asking the provider which models it offers…");
  const [model, setModel] = useState("");
  const [name, setName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [window, setWindow] = useState("");
  const [makeDefault, setMakeDefault] = useState(false);
  const s = useSave(onSaved);
  const takeModel = (v: string) => { setModel(v); if (!nameTouched) setName(suggestedName(v, names)); };

  useEffect(() => {
    let live = true;
    RunAPI.ProviderModels(provider.id)
      .then((o) => {
        if (!live || !o) return;
        setOffered(o);
        takeModel(o.models?.[0]?.id ?? o.suggested?.[0] ?? "");
      })
      .catch((e) => { if (live) setOffered({ models: [], suggested: [], error: String(e), warning: "" } as run.ProviderModels); })
      .finally(() => { if (live) setListing(null); });
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [provider.id]);

  async function listCopilot() {
    setListing("Asking Copilot CLI which models your plan offers. A first list builds images and can take minutes…");
    try {
      setCopilotModels((await RunAPI.CopilotModels(provider.copilotPlan || "individual")) ?? []);
    } catch (e) {
      s.setError(`Could not list your plan's models (${e}). Type the model id instead.`);
    } finally {
      setListing(null);
    }
  }

  const { tokens: windowTokens, error: windowError } = parseContextWindow(window);
  const held = !model.trim() ? "pick or type a model" : !name.trim() ? "name the model" : windowError ? `the context window must be ${windowError}` : "";
  const add = (agreed: boolean) => s.save((ok) => RunAPI.AddSavedModel(provider.id, { name: name.trim(), model: model.trim(), contextWindow: windowTokens, makeDefault, agreedToMessage: ok }), agreed);
  const options = provider.way === "copilot" && copilotModels ? copilotOffered(copilotModels.map((m) => m.id)).map((id) => ({ id, label: copilotModelLabel(id, copilotModels) }))
    : (offered?.models ?? []).map((m) => ({ id: m.id, label: modelOptionLabel(m) }));

  return (
    <Modal title={`Add a model on ${providerName(provider)}`} onClose={onClose}>
      <div data-add-model={provider.id}>
        <p className="muted small">{paidWith(provider)}. Nothing is asked again: the model is paid the same way.</p>
        {listing ? <Spinner label={listing} /> : (
          <ModelFields provider={provider.vendor} way={provider.way} model={model} setModel={takeModel} name={name} setName={(v) => { setName(v); setNameTouched(true); }}
            window={window} setWindow={setWindow} makeDefault={makeDefault} setMakeDefault={setMakeDefault} first={false}
            options={options} suggested={offered?.suggested ?? []} busy={s.busy}
            listHint={provider.way === "copilot" ? <Button variant="link" onClick={() => void listCopilot()}>{copilotModels ? "List again" : "List my plan's models"}</Button>
              : offered?.error ? `The provider's list could not be had (${offered.error}). Type the model id.` : provider.way === "connect" ? "Agentry does not list these models: type the id the provider shows." : undefined} />
        )}
        {offered?.warning && <Notice tone="warn">{offered.warning}</Notice>}
        <ErrorBox error={s.error} />
        {s.refusal && <RefusalBox refusal={s.refusal} busy={s.busy} onRetry={() => void add(false)} onPick={(id) => takeModel(id)} />}
        <p className="row" style={{ justifyContent: "flex-end" }}>
          <Button onClick={onClose} disabled={s.busy}>Cancel</Button>
          <Button variant="primary" disabled={s.busy || !!held || !!listing} title={held} busy={s.busy} busyLabel="Checking…" onClick={() => void add(false)}>Check and save</Button>
        </p>
      </div>
      {s.agreement && <ClaudeAgreement notice={s.agreement} onCancel={() => s.setAgreement(null)} onAgree={() => { s.setAgreement(null); void add(true); }} />}
    </Modal>
  );
}

/** Edit a saved model: its name, its model id (checked again) or, for an
 *  endpoint of the person's own, its context window. */
function EditModelDialog({ model: m, provider, onClose, onSaved }: {
  model: SavedModelRow; provider?: ProviderRow; onClose: () => void; onSaved: (res: run.SaveResult) => void;
}) {
  const [name, setName] = useState(m.name);
  const [model, setModel] = useState(m.model);
  const [window, setWindow] = useState(m.contextWindow ? String(m.contextWindow) : "");
  const s = useSave(onSaved);
  const { tokens: windowTokens, error: windowError } = parseContextWindow(window);
  const own = provider?.vendor === "custom";
  const held = !name.trim() ? "name the model" : !model.trim() ? "type a model id" : windowError ? `the context window must be ${windowError}` : "";
  const save = (agreed: boolean) => s.save((ok) => RunAPI.EditSavedModel(m.id, { name: name.trim(), model: model.trim(), contextWindow: own && windowTokens !== (m.contextWindow ?? 0) ? windowTokens : 0, makeDefault: false, agreedToMessage: ok }), agreed);
  return (
    <Modal title={`Edit ${m.name}`} onClose={onClose}>
      <div data-edit-model={m.id}>
        <label className="field">
          <span className="field-label">Name</span>
          <Input type="text" value={name} onChange={(e) => setName(e.target.value)} disabled={s.busy} maxLength={64} aria-label="Name" />
        </label>
        <label className="field">
          <span className="field-label">Model</span>
          <Input type="text" className="mono" value={model} onChange={(e) => setModel(e.target.value)} disabled={s.busy} aria-label="Model" />
          <span className="muted small field-hint">A new model id is checked before it is saved.</span>
        </label>
        {own && (
          <label className="field">
            <span className="field-label">Context window</span>
            <Input type="text" inputMode="numeric" className="mono" value={window} onChange={(e) => setWindow(e.target.value)} disabled={s.busy} aria-label="Context window" />
            {windowError && <span className="muted small field-hint">The window must be {windowError}.</span>}
          </label>
        )}
        <ErrorBox error={s.error} />
        {s.refusal && <RefusalBox refusal={s.refusal} busy={s.busy} onRetry={() => void save(false)} onPick={setModel} />}
        <p className="row" style={{ justifyContent: "flex-end" }}>
          <Button onClick={onClose} disabled={s.busy}>Cancel</Button>
          <Button variant="primary" disabled={s.busy || !!held} title={held} busy={s.busy} busyLabel="Saving…" onClick={() => void save(false)}>Save</Button>
        </p>
      </div>
      {s.agreement && <ClaudeAgreement notice={s.agreement} onCancel={() => s.setAgreement(null)} onAgree={() => { s.setAgreement(null); void save(true); }} />}
    </Modal>
  );
}

/** Remove a saved model, or a provider and its models. What names them is
 *  listed, and nothing moves until the person picks where it goes: the
 *  default that stays, or another saved model. Removing the default asks
 *  for the new one, which they move to as well. */
function RemoveDialog({ models, model, provider, onClose, onRemoved }: {
  models: SavedModelRow[]; model?: SavedModelRow; provider?: ProviderRow;
  onClose: () => void; onRemoved: (line: string) => void;
}) {
  const removedIds = model ? [model.id] : (provider?.models ?? []);
  const ask = removalAsk(models, removedIds);
  const [choice, setChoice] = useState<"default" | "other" | "">("");
  const [other, setOther] = useState("");
  const [forget, setForget] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const target = ask.takesDefault ? other : choice === "default" ? ask.stayingDefault?.id ?? "" : choice === "other" ? other : "";
  const heldWhy = removalHeld(ask, target);
  const subject = model ? model.name : provider ? providerName(provider) : "";
  const others = ask.targets.filter((m) => m !== ask.stayingDefault);

  async function remove() {
    setBusy(true);
    setError(null);
    try {
      const out = model
        ? await RunAPI.RemoveSavedModel(model.id, removalNeedsTarget(ask) ? target : "")
        : await RunAPI.RemoveProvider(provider!.id, removalNeedsTarget(ask) ? target : "", !forget);
      if (out && (out.inUse ?? []).length > 0) {
        setError(`Something else names it now: ${(out.inUse ?? []).map((u) => userLine(u)).join(", ")}. Choose where it goes, then remove it.`);
        return;
      }
      const to = models.find((m) => m.id === out?.movedTo);
      const moved = (out?.moved ?? []).length;
      const parts = [`${subject} is removed.`];
      if (moved > 0 && to) parts.push(`${moved === 1 ? "One thing that named it now names" : `${moved} things that named it now name`} ${to.name}.`);
      if (ask.takesDefault && to) parts.push(`${to.name} is your default model.`);
      if (provider && out?.credential) parts.push(`Its credential: ${out.credential}.`);
      onRemoved(parts.join(" "));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={`Remove ${subject}?`} onClose={onClose}>
      <div data-remove-dialog>
        {provider && (
          <p>{ask.removed.length === 0 ? "No saved model is on it." : `Its saved models go with it: ${ask.removed.map((m) => m.name).join(", ")}.`}</p>
        )}
        {ask.users.length > 0 && (
          <>
            <p>{ask.users.length === 1 ? "This uses it:" : "These use it:"}</p>
            <ul className="small" data-removal-users>
              {ask.users.map((u) => <li key={`${u.kind}:${u.id}`}>{userLine(u).charAt(0).toUpperCase() + userLine(u).slice(1)}</li>)}
            </ul>
          </>
        )}
        {ask.takesDefault ? (
          <label className="field">
            <span className="field-label">Your new default</span>
            <Select value={other} onChange={(e) => setOther(e.target.value)} disabled={busy} aria-label="Your new default">
              <option value="">Choose a saved model…</option>
              {ask.targets.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
            </Select>
            <span className="muted small field-hint">
              {model ? "It is your default model now." : "Your default model is on it."} Runs, schedules and agents that name no model use the new one{ask.users.length > 0 ? ", and what uses it moves there too" : ""}.
            </span>
          </label>
        ) : ask.users.length > 0 ? (
          <div className="field way-choice" data-removal-choice>
            <span className="field-label">Where they go</span>
            {ask.stayingDefault && (
              <label className="row" style={{ gap: 6 }}>
                <input type="radio" name="remove-move" checked={choice === "default"} disabled={busy} onChange={() => setChoice("default")} />
                <span>Move them to {ask.stayingDefault.name}</span>
              </label>
            )}
            {others.length > 0 && (
              <>
                <label className="row" style={{ gap: 6 }}>
                  <input type="radio" name="remove-move" checked={choice === "other"} disabled={busy} onChange={() => setChoice("other")} />
                  <span>Move them to another saved model</span>
                </label>
                {choice === "other" && (
                  <Select value={other} onChange={(e) => setOther(e.target.value)} disabled={busy} aria-label="Move them to">
                    <option value="">Choose a saved model…</option>
                    {others.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
                  </Select>
                )}
              </>
            )}
            <span className="muted small field-hint">Nothing moves until you choose.</span>
          </div>
        ) : null}
        {provider && provider.way !== "credits" && !provider.takesNoKey && (
          <label className="row" style={{ gap: 6 }}>
            <input type="checkbox" checked={forget} disabled={busy} onChange={(e) => setForget(e.target.checked)} />
            <span>{provider.way === "key" ? "Also remove its key from this computer's keychain" : provider.way === "connect" ? "Also disconnect the account on this computer" : "Also sign out on this computer"}</span>
          </label>
        )}
        <ErrorBox error={error} />
        <p className="row" style={{ justifyContent: "flex-end" }}>
          <Button onClick={onClose} disabled={busy}>Cancel</Button>
          <Button variant="danger" disabled={busy || !!heldWhy} title={heldWhy} busy={busy} busyLabel="Removing…" onClick={() => void remove()}>Remove</Button>
        </p>
        {heldWhy && <p className="muted small" style={{ textAlign: "right" }}>{heldWhy}</p>}
      </div>
    </Modal>
  );
}

/** Add a missing key: kept in this computer's keychain for the provider,
 *  which reads it there from then on. */
function KeyDialog({ provider, onClose, onSaved }: { provider: ProviderRow; onClose: () => void; onSaved: () => void }) {
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function save() {
    const k = key;
    setKey("");
    setBusy(true);
    setError(null);
    try {
      await RunAPI.StoreProviderKey({ id: provider.id, vendor: provider.vendor, way: "key", name: provider.name ?? "", baseUrl: "", takesKey: true, copilotPlan: "" }, k);
      onSaved();
    } catch (e) {
      setError(`${e}. The key you typed was cleared; type it again to retry.`);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal title={`Add the key for ${providerName(provider)}`} onClose={onClose}>
      <label className="field">
        <span className="field-label">API key</span>
        <Input type="password" className="mono" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} disabled={busy} placeholder="Paste the key" aria-label="API key" />
        <span className="muted small field-hint">Kept in this computer&apos;s keychain, never in a file, and never shown here.</span>
      </label>
      <ErrorBox error={error} />
      <p className="row" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onClose} disabled={busy}>Cancel</Button>
        <Button variant="primary" disabled={busy || !key.trim()} busy={busy} busyLabel="Saving…" onClick={() => void save()}>Keep the key</Button>
      </p>
    </Modal>
  );
}

/** Remove from a runhost: the runhost refuses while its agents or schedules
 *  name the model, or while it is its default, unless the person picks
 *  another of the runhost's saved models to move them to. Its provider's
 *  credential leaves the runhost with its last saved model. */
function RemoveFromRunhost({ model, runhost, error, busy, onRemove, onClose }: {
  model: SavedModelRow;
  runhost: RunhostModels;
  error?: string;
  busy: boolean;
  onRemove: (moveTo: string) => void;
  onClose: () => void;
}) {
  const targets = runhostMoveTargets(model.id, runhost);
  const [moveTo, setMoveTo] = useState("");
  return (
    <Modal title={`Remove ${model.name} from ${runhost.name}?`} onClose={onClose}>
      <p className="small">
        {runhost.name} stops holding {model.name}. Its provider's key, sign-in or credits token leaves {runhost.name} too
        when no other saved model there uses it. This computer keeps {model.name}.
      </p>
      {targets.length > 0 && (
        <label className="field">
          <span className="field-label">Move what names it to</span>
          <Select value={moveTo} onChange={(e) => setMoveTo(e.target.value)} aria-label="Move what names it to" data-move-to>
            <option value="">Nothing: refuse if something names it</option>
            {targets.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </Select>
          <span className="muted small">Its agents and schedules that name {model.name} move there, and it is {runhost.name}'s default if {model.name} was.</span>
        </label>
      )}
      <ErrorBox error={error ?? null} />
      <div className="row" style={{ justifyContent: "flex-end", gap: 8 }}>
        <Button onClick={onClose} disabled={busy}>Cancel</Button>
        <Button variant="danger" busy={busy} busyLabel="Removing…" onClick={() => onRemove(moveTo)}>Remove</Button>
      </div>
    </Modal>
  );
}
