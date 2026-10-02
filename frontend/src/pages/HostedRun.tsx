// In Agentry: the parts of the run panel that belong to a hosted run
// (plan hosted-runs.md §4.7, agentry-desktop#77). The panel itself keeps
// the consent, the intake form and the task; these render what a hosted
// run costs before it starts, where it stands while it runs — queued,
// running, then done with pass or fail — and what it produced when it is
// over: the same facts a local run's result shows, from the registry's
// numbers rather than the app's arithmetic. A run In Agentry has no log
// to show (hosted runs in production, D12 and D16; agentry-desktop#182).
import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../wailsjs/go/main/RunAPI";
import { OpenExternal } from "../../wailsjs/go/main/App";
import { OpenUpgrade } from "../../wailsjs/go/main/SettingsAPI";
import { main } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Input, Notice, Textarea } from "../ui";
import { hostedUseLine, runCapsLine, runHoldSentence, sessionCreditsSentence, sessionTermsSentence } from "../usage";

/** What a run In Agentry holds and what the account has, before anything
 *  is created: the hold in credits, the time and the turns, never tokens or
 *  a rate (agentry-desktop#457). Every number comes from the registry. */
export function HostedCredits({ hosted }: { hosted: main.HostedAvailabilityView }) {
  return (
    <p className="muted small" style={{ margin: "4px 0" }}>
      {runHoldSentence(hosted.holdCredits)}{" "}
      {hosted.balanceKnown ? <>Your balance is <strong>{hosted.balance}</strong> credits.</> : "The balance could not be read."}
      {hosted.caps && <> {runCapsLine(hosted.tier ?? "", hosted.caps)}</>}
      {hosted.runnersOnline === 0 && <> No runner is online right now, so the run waits in the queue.</>}
    </p>
  );
}

/** What a session In Agentry costs, before it starts (plan
 *  hosted-sessions.md §7; agentry-desktop#403): it holds nothing, its first
 *  minutes are free, each minute after is pulled as it starts, and its
 *  model use settles when it ends, in credits. Every number is the
 *  registry's. */
export function SessionCredits({ hosted }: { hosted: main.HostedAvailabilityView }) {
  const s = hosted.session;
  if (!s) return null;
  // A run of this caller's that holds nothing is an unmetered
  // organization's (hosted runs D5): its sessions cost no credits either.
  const metered = !(hosted.caps && hosted.holdCredits === 0);
  return (
    <p className="muted small" style={{ margin: "4px 0" }}>
      {sessionTermsSentence(s, metered)}{" "}
      {metered && (hosted.balanceKnown ? <>Your balance is <strong>{hosted.balance}</strong> credits. </> : "The balance could not be read. ")}
      It closes after {idleWords(s.idleSeconds)} with nothing said or done.
    </p>
  );
}

/** Talk to it In Agentry, offered but not available: why, the way out,
 *  and — for a payer whose tier does not include it — the registry's
 *  subscription page, opened after Go checks its origin. */
export function SessionUnavailable({ registry, hosted }: { registry: string; hosted: main.HostedAvailabilityView }) {
  const s = hosted.session;
  const [err, setErr] = useState<string | null>(null);
  if (!s || !s.offered || s.available) return null;
  return (
    <Notice tone="warn">
      <strong>Talk to it In Agentry is unavailable here.</strong> {s.reason}
      {s.remedy ? ` ${s.remedy}` : ""}
      {s.upgrade && (
        <>
          {" "}
          <Button variant="link" onClick={() => OpenUpgrade(registry).catch((e) => setErr(String(e)))}>See Pro</Button>
        </>
      )}
      <ErrorBox error={err} />
    </Notice>
  );
}

/** The mode's unavailable-with-reason state: why it cannot be chosen here
 *  and what to do about it. */
export function HostedUnavailable({ hosted }: { hosted: main.HostedAvailabilityView }) {
  if (!hosted.offered || hosted.available) return null;
  return (
    <Notice tone="warn">
      <strong>In Agentry is unavailable for this release.</strong> {hosted.reason}
      {hosted.remedy ? ` ${hosted.remedy}` : ""}
    </Notice>
  );
}

/** What a release that declares secrets asks of a run In Agentry, said
 *  before the start (plan hosted-credentials-vault.md D12; agentry-desktop
 *  #300). Which of them the account keeps is not something this app can
 *  read (the list is a signed-in session's, DEV-165): the start finds out,
 *  and asks for the rest. */
export function HostedSecretsNote({ hosted }: { hosted: main.HostedAvailabilityView }) {
  const names = hosted.declaredSecrets ?? [];
  if (names.length === 0) return null;
  return (
    <p className="muted small" style={{ margin: "4px 0" }}>
      This release declares {names.length === 1 ? "the secret" : "the secrets"}{" "}
      {names.map((n, i) => (
        <span key={n}>{i > 0 ? ", " : ""}<code>{n}</code></span>
      ))}
      . A secret your account keeps in Agentry is used as it is. One it does not keep is asked for when you start,
      and Agentry removes it when the run ends unless you tick Keep it for later runs.
    </p>
  );
}

/** The start's ask for the declared secrets the account does not keep
 *  (a secret-missing refusal names them). Each value is sent once, in a
 *  call of its own (RunAPI.SetHostedSecret, D-024's rule), and cleared
 *  from this form before it is sent; "Keep it for later runs" is unticked,
 *  so a value is held for this run alone unless the person ticks it. */
export function HostedSecretsAsk({ registry, names, onSaved, onCancel }: {
  registry: string;
  names: string[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [keep, setKeep] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const ready = names.every((n) => (values[n] ?? "") !== "");
  // An ask that goes away without starting (the panel closed, the mode
  // changed) discards what it set; one that saved and starts leaves them
  // to the start, which keeps or discards them itself.
  const saved = useRef(false);
  useEffect(() => () => {
    if (!saved.current) void RunAPI.DiscardHostedSecrets(registry).catch(() => undefined);
  }, [registry]);

  async function save() {
    setBusy(true);
    setErr(null);
    // The values leave the form's state before any is sent: a refusal
    // asks for them again rather than keeping them here.
    const typed = { ...values };
    setValues({});
    try {
      for (const n of names) {
        await RunAPI.SetHostedSecret(registry, n, typed[n], !!keep[n]);
      }
      saved.current = true;
      onSaved();
    } catch (e) {
      setErr(`${e} The values you typed were cleared; type them again to retry.`);
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    await RunAPI.DiscardHostedSecrets(registry).catch(() => undefined);
    onCancel();
  }

  return (
    <Card>
      <h3 style={{ marginTop: 0 }}>Secrets for this run</h3>
      <p className="small">
        This release declares {names.length === 1 ? "a secret" : "secrets"} your account does not keep in Agentry.
        Enter {names.length === 1 ? "its value" : "each value"} to start the run. The value goes into that run&apos;s
        tools container only, never into the agent&apos;s own.
      </p>
      {names.map((n) => (
        <div key={n} style={{ marginBottom: 10 }}>
          <div className="field">
            <label className="field-label" htmlFor={`secret-${n}`}><code>{n}</code></label>
            {/* A text area, as the site's: a one-line field silently drops
                a pasted value's line breaks, where the registry refuses a
                multi-line value in words. */}
            <Textarea
              id={`secret-${n}`} rows={2} className="mono" autoComplete="off" spellCheck={false}
              value={values[n] ?? ""} disabled={busy}
              onChange={(e) => setValues((v) => ({ ...v, [n]: e.target.value }))}
            />
          </div>
          <label className="row gap small" style={{ marginTop: 6 }}>
            <input type="checkbox" checked={!!keep[n]} disabled={busy} onChange={(e) => setKeep((k) => ({ ...k, [n]: e.target.checked }))} />
            <span>Keep it for later runs</span>
          </label>
          <span className="muted small">
            {keep[n]
              ? <>Kept under your account for any run In Agentry that declares <code>{n}</code>, capitals included.</>
              : <>Agentry removes it when this run ends.</>}
          </span>
        </div>
      ))}
      <p className="muted small">
        A value is one line, with no space at either end: a multi-line value such as a PEM key cannot be kept yet.
        Once a value is saved, neither this app nor Agentry shows it again.
      </p>
      <ErrorBox error={err} />
      <div className="row">
        <Button onClick={cancel} disabled={busy}>Cancel</Button>
        <span className="grow" />
        <Button variant="primary" disabled={busy || !ready} onClick={save}>{busy ? "Saving…" : "Save and start"}</Button>
      </div>
    </Card>
  );
}

/** Settings' card for the account's secrets in Agentry: add or replace one
 *  by name, delete one by name. The list is on the site's Secrets page — a
 *  signed-in session's alone, never a token's (DEV-165) — so this card
 *  opens it rather than drawing one. A secret saved here is kept. */
export function SecretsCard({ registry }: { registry: string }) {
  const [keeps, setKeeps] = useState(false);
  useEffect(() => {
    let live = true;
    RunAPI.KeepsSecrets(registry).then((k) => { if (live) setKeeps(k); }).catch(() => undefined);
    return () => { live = false; };
  }, [registry]);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  // The manifest's grammar, as internal/hostedrun.ValidSecretName holds it.
  const validName = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(name.trim());

  async function save() {
    const n = name.trim(), v = value;
    setValue("");
    setBusy(true); setErr(null); setNote(null);
    try {
      await RunAPI.SetHostedSecret(registry, n, v, true);
      setNote(`${n} is saved under your account. It is not shown again.`);
    } catch (e) {
      setErr(`${e} The value you typed was cleared.`);
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    const n = name.trim();
    setConfirming(false);
    setBusy(true); setErr(null); setNote(null);
    try {
      await RunAPI.DeleteHostedSecret(registry, n);
      setNote(`${n} is deleted, if your account kept it. A run In Agentry that declares it asks for it again.`);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }
  async function openList() {
    try {
      await OpenExternal(await RunAPI.SecretsPageURL(registry));
    } catch (e) {
      setErr(String(e));
    }
  }

  if (!keeps) return null;
  return (
    <Card>
      <h3 style={{ marginTop: 0 }}>Secrets for runs In Agentry</h3>
      <p className="small">
        A secret is a value a run In Agentry needs, such as an API token, saved under a name in Agentry. When you
        start a run of a QuickAgent that declares a secret of that name, the value goes into that run&apos;s tools
        container only, never into the agent&apos;s own.
      </p>
      <p className="muted small">
        One saved here stays until you delete it. Once a value is saved, neither this app nor Agentry shows it again;
        to change one, save the name again. Your list of secrets is on Agentry&apos;s Secrets page, in your browser.
      </p>
      <div className="row" style={{ gap: 8, alignItems: "flex-end" }}>
        <label className="field grow">
          <span className="field-label">Name</span>
          <Input type="text" className="mono" value={name} disabled={busy} placeholder="GITHUB_TOKEN" spellCheck={false}
            onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="field grow">
          <span className="field-label">Value</span>
          <Textarea rows={1} className="mono" autoComplete="off" spellCheck={false} value={value} disabled={busy}
            onChange={(e) => setValue(e.target.value)} />
        </label>
      </div>
      <p className="muted small">A value is one line, with no space at either end.</p>
      {name.trim() !== "" && !validName && (
        <p className="muted small">A name is an environment variable&apos;s: a letter or _, then letters, digits or _.</p>
      )}
      {note && <Notice tone="ok">{note}</Notice>}
      {confirming && (
        <Notice tone="warn">
          <strong>Delete {name.trim()}?</strong> Its value is gone for good. A run In Agentry of a QuickAgent that declares
          it asks for it again, and a run already queued fails before it starts, with its credits returned.{" "}
          <Button onClick={remove} disabled={busy}>Delete {name.trim()}</Button>{" "}
          <Button onClick={() => setConfirming(false)} disabled={busy}>Keep it</Button>
        </Notice>
      )}
      <ErrorBox error={err} />
      <div className="row" style={{ gap: 8 }}>
        <Button onClick={openList}>See your secrets</Button>
        <span className="grow" />
        <Button onClick={() => setConfirming(true)} disabled={busy || !validName || confirming}>Delete</Button>
        <Button variant="primary" onClick={save} disabled={busy || !validName || value === ""}>Save</Button>
      </div>
    </Card>
  );
}


/** The badge class for a verdict's tone; a cancelled run, which has no
 *  outcome, reads as a warning. */
function toneClass(tone?: string): string {
  return tone === "ok" ? "ok" : tone === "bad" ? "bad" : "warn";
}

/** The finished run: done with pass or fail, the outcome, the receipt's
 *  numbers, what it cost, and the files that landed in the run folder. */
export function HostedResult({ run }: { run: main.HostedRunView }) {
  const enforced = Object.entries(run.capabilitiesEnforced ?? {});
  return (
    <Card className="selectable">
      <div className="row">
        <span className={`badge ${toneClass(run.outcomeTone)}`}>{run.state}</span>
        {run.outcome && <span className="badge kind">outcome: {run.outcome}</span>}
        {run.exitClass && <span className="badge kind">exit class: {run.exitClass}</span>}
        {run.attestationState && (
          <span className={`badge ${run.attestationState === "verified" ? "ok" : "warn"}`}>
            attestation: {run.attestationState}
          </span>
        )}
        <span className="muted small">{hostedUseLine(run)}</span>
      </div>
      <p className="muted small" style={{ margin: "6px 0" }}>{run.outcomeLine}</p>
      {run.error && <div className="error-box selectable">{run.error}</div>}
      <div className="kv">
        <span className="k">credits</span>
        {run.session ? (
          <span>{sessionCreditsSentence({ ...run, session: run.session })}</span>
        ) : (
          <span className="mono">
            {run.holdCredits} held · {run.settled ? `${run.settledCredits} settled` : "settlement not reported"}
          </span>
        )}
      </div>
      {!run.receiptKept && (
        <p className="muted small" style={{ margin: "6px 0 2px" }}>
          The registry kept no receipt for this run, so what it enforced is not listed here. A file in the run's
          outputs is not read as one, because the agent could have written it.
        </p>
      )}
      {enforced.length > 0 && (
        <>
          <p className="muted small" style={{ margin: "6px 0 2px" }}>What the run enforced, listed from its receipt:</p>
          {enforced.map(([k, v]) => (
            <div key={k} className="mono small">{k}: {v}</div>
          ))}
        </>
      )}
      <div className="kv"><span className="k">run</span><span className="mono">{run.id}</span></div>
      <div className="kv"><span className="k">release</span><span className="mono">{run.ref}</span></div>
      <div className="kv"><span className="k">run folder</span><span className="mono">{run.outDir}</span></div>
      {run.outputs.length > 0 ? (
        run.outputs.map((a) => (
          <div key={a.name} className="mono small">{a.name} — {a.bytes} bytes</div>
        ))
      ) : (
        <p className="muted small">This run wrote no outputs.</p>
      )}
    </Card>
  );
}

/** idleWords says an idle close as Go's quit dialog does. */
function idleWords(seconds: number): string {
  if (seconds % 60 !== 0) return `${seconds} seconds`;
  return seconds === 60 ? "a minute" : `${seconds / 60} minutes`;
}
