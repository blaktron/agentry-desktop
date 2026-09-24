// In Agentry: the parts of the run panel that belong to a hosted run
// (plan hosted-runs.md §4.7, agentry-desktop#77). The panel itself keeps
// the consent, the intake form and the task; these render what a hosted
// run costs before it starts, where it stands while it runs — queued,
// running, then done with pass or fail — and what it produced when it is
// over: the same facts a local run's result shows, from the registry's
// numbers rather than the app's arithmetic. A run In Agentry has no log
// to show (hosted runs in production, D12 and D16; agentry-desktop#182).
import { main } from "../../wailsjs/go/models";
import { Button, Card, Notice } from "../ui";

/** What a run In Agentry holds and what the account has, before anything
 *  is created. Both numbers come from the registry. */
export function HostedCredits({ hosted }: { hosted: main.HostedAvailabilityView }) {
  return (
    <p className="muted small" style={{ margin: "4px 0" }}>
      This run holds <strong>{hosted.holdCredits}</strong> credits while it is queued and running, and settles from the
      tokens it uses.{" "}
      {hosted.balanceKnown ? <>Your balance is <strong>{hosted.balance}</strong> credits.</> : "The balance could not be read."}
      {hosted.caps && (
        <> Ceilings for the {hosted.tier} tier: {hosted.caps.timeoutSeconds}s, {hosted.caps.maxTurns} turns,{" "}
        {hosted.caps.maxTokens.toLocaleString()} tokens.</>
      )}
      {hosted.runnersOnline === 0 && <> No runner is online right now, so the run waits in the queue.</>}
    </p>
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

/** Where a run In Agentry stands: its identity, its state, its place in
 *  the queue, and the way to stop it. There is no log to follow: the
 *  panel re-reads the run and this card moves with it. */
export function HostedProgress({ run, onCancel, cancelling }: {
  run: main.HostedRunView; onCancel: () => void; cancelling: boolean;
}) {
  const active = run.status === "queued" || run.status === "claimed" || run.status === "running";
  return (
    <Card className="selectable">
      <div className="row">
        <span className={`badge ${active ? "kind" : toneClass(run.outcomeTone)}`}>{run.state}</span>
        {(run.queuePosition ?? 0) > 0 && run.status === "queued" && (
          <span className="badge kind">queue position {run.queuePosition}</span>
        )}
        <span className="muted small grow">{run.statusLine}</span>
        {active && (
          <Button variant="danger" onClick={onCancel} disabled={cancelling || run.cancelRequested}>
            {run.cancelRequested ? "Cancelling…" : "Cancel run"}
          </Button>
        )}
      </div>
      <div className="kv"><span className="k">run</span><span className="mono">{run.id}</span></div>
      <div className="kv"><span className="k">release</span><span className="mono">{run.ref}</span></div>
      <div className="kv"><span className="k">credits held</span><span className="mono">{run.holdCredits}</span></div>
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
export function HostedResult({ run, children }: { run: main.HostedRunView; children?: React.ReactNode }) {
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
        <span className="muted small">
          {run.turns} turns · {run.totalTokens.toLocaleString()} tokens ({run.inputTokens.toLocaleString()} in /{" "}
          {run.outputTokens.toLocaleString()} out) · {(run.wallMs / 1000).toFixed(1)}s
        </span>
      </div>
      <p className="muted small" style={{ margin: "6px 0" }}>{run.outcomeLine}</p>
      {run.error && <div className="error-box selectable">{run.error}</div>}
      {children}
      <div className="kv">
        <span className="k">credits</span>
        <span className="mono">
          {run.holdCredits} held · {run.settled ? `${run.settledCredits} settled` : "settlement not reported"}
        </span>
      </div>
      {!run.receiptKept && (
        <p className="muted small" style={{ margin: "6px 0 2px" }}>
          The registry kept no receipt for this run, so what it enforced is not listed here. A file in the run's
          outputs is not read as one, because the agent could have written it (agentry-enterprise#121).
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
      <div className="kv"><span className="k">run folder</span><span className="mono">{run.outDir}</span></div>
      {run.outputs.length > 0 ? (
        run.outputs.map((a) => (
          <div key={a.name} className="mono small">{a.name} — {a.bytes} bytes</div>
        ))
      ) : (
        <p className="muted small">This run wrote no outputs.</p>
      )}
      {run.downloadError && <div className="error-box selectable">{run.downloadError}</div>}
    </Card>
  );
}
