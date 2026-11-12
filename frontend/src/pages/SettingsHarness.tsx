import { useEffect, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as run from "../../bindings/github.com/blaktron/agentry-desktop/internal/run/models";
import { Card, ErrorBox, Notice, Spinner } from "../ui";
import { HARNESS_OPTIONS, changedLine, runsOnLine, signInLines } from "../containerHarness";

/** Settings → Models and keys → Harness: which container harness drives the
 *  model in runs, chats and persistent agents on this computer. The choice
 *  is the runner policy's containerHarness, the key `agentry harness use`
 *  sets, so the CLI and the app agree; Docker Agent, the default, is written
 *  as no key at all. Models paid by a model sign-in run on their vendor's
 *  harness whatever it says, and the card says which. */
export function HarnessSettings() {
  const [choice, setChoice] = useState<run.HarnessChoice | null>(null);
  const [providers, setProviders] = useState<{ way: string }[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let live = true;
    Promise.resolve().then(() => RunAPI.ContainerHarness())
      .then((c) => live && setChoice(c))
      .catch((e) => live && setError(String(e)));
    // The providers, for which of them run on their vendor's harness; a list
    // that cannot be read leaves those lines out.
    Promise.resolve().then(() => RunAPI.SavedModels())
      .then((l) => live && setProviders(l?.setUp ? (l.providers ?? []) : []))
      .catch(() => {});
    return () => { live = false; };
  }, []);

  async function choose(id: string) {
    if (!choice || id === choice.harness) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const next = await RunAPI.SetContainerHarness(id);
      setChoice(next);
      setNotice(changedLine(next.harness));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const vendorLines = [...(choice ? [runsOnLine(choice)] : []), ...signInLines(providers)].filter((l): l is string => !!l);

  return (
    <section id="settings-part-harness" className="settings-part" data-harness-setting={choice?.harness}>
      <h2>Harness</h2>
      <p className="muted small">
        The harness is the program that drives the model through a run: it sends the conversation, makes the tool
        calls the model asks for and reports what happened. Both run the same QuickAgents under the same policy, and
        a run&apos;s details name the harness that ran it.
      </p>
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      <Card>
        {!choice && !error && <Spinner label="Reading the harness…" />}
        {choice && (
          <div role="radiogroup" aria-label="Harness">
            {HARNESS_OPTIONS.map((h) => (
              <label key={h.id} className="check-row" data-harness-option={h.id}>
                <input type="radio" name="container-harness" value={h.id} checked={choice.harness === h.id} disabled={busy}
                  onChange={() => void choose(h.id)} />
                <span>
                  <strong>{h.label}</strong>
                  <span className="muted small"> — {h.line}</span>
                </span>
              </label>
            ))}
          </div>
        )}
        {busy && <Spinner label="Saving…" />}
        {vendorLines.map((l) => <Notice key={l} tone="info">{l}</Notice>)}
        <p className="muted small" style={{ marginBottom: 0 }}>
          Runs, chats and persistent agents on this computer use the harness chosen here from their next start; one
          under way finishes on the harness it started on. Runs In Agentry and on your runhosts use the harness
          their own service or computer chooses. The choice is kept in the runner policy
          {choice?.policyPath ? <> (<span className="mono selectable">{choice.policyPath}</span>)</> : null}, where{" "}
          <span className="mono">agentry harness use</span> sets it too.
        </p>
      </Card>
    </section>
  );
}
