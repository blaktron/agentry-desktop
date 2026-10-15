import { useEffect, useState } from "react";
import * as PublisherAPI from "../../wailsjs/go/main/PublisherAPI";
import { Button, ErrorBox, Field, Input, Modal, Select, Spinner } from "../ui";
import { FORK_TERMS } from "../fork";

// The Fork dialog, as the site's Fork form: the namespace the fork
// goes into, the three facts about a fork, and a name field only once the
// registry said the name was taken or held there. Go asks again in a
// native dialog and the core decides; what this draws only saves a click.
export default function ForkDialog({ registry, ns, name, version, onClose, onForked }: {
  registry: string;
  ns: string;
  name: string;
  /** The published version a fork starts from, as the site's form says it. */
  version: string;
  onClose: () => void;
  /** The fork was made: its namespace, name and the sentence that says so. */
  onForked: (namespace: string, name: string, sentence: string) => void;
}) {
  const [targets, setTargets] = useState<string[] | null>(null);
  const [target, setTarget] = useState("");
  const [askName, setAskName] = useState(false);
  const [newName, setNewName] = useState(name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    PublisherAPI.ForkTargets(registry)
      .then((t) => {
        setTargets(t);
        setTarget(t[0] ?? "");
      })
      .catch((e) => setError(String(e)));
  }, [registry]);

  async function fork() {
    setBusy(true);
    setError(null);
    try {
      const v = await PublisherAPI.Fork(registry, ns, name, target, askName ? newName : "");
      if (v.cancelled) return;
      if (v.refusal) {
        setError(v.refusal);
        if (v.askName) setAskName(true);
        return;
      }
      onForked(v.namespace ?? target, v.name ?? name, v.sentence ?? "");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={`Fork ${ns}/${name}`} onClose={onClose}>
      <p className="muted">
        A fork is your own copy of {ns}/{name}, its whole history included, starting from v{version}. The fork keeps
        the name {name} unless that name is taken in the namespace you choose.
      </p>
      {!targets && !error && <Spinner />}
      {targets && targets.length === 0 && <p>Sign in under Settings to fork a QuickAgent.</p>}
      {targets && targets.length > 0 && (
        <>
          <Field label="Fork into" htmlFor="fork-target">
            <Select id="fork-target" value={target} onChange={(e) => setTarget(e.target.value)}>
              {targets.map((t, i) => <option key={t} value={t}>{i === 0 ? `${t} (yours)` : t}</option>)}
            </Select>
          </Field>
          {askName && (
            <Field label="Name for the fork" htmlFor="fork-name" hint="Lower-case letters, digits and hyphens, starting and ending with a letter or digit.">
              <Input id="fork-name" className="mono" value={newName} onChange={(e) => setNewName(e.target.value)} autoFocus />
            </Field>
          )}
          <ul className="small" data-fork-terms>
            {FORK_TERMS.map((t) => <li key={t}>{t}</li>)}
          </ul>
        </>
      )}
      <ErrorBox error={error} />
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" onClick={fork} disabled={!target || busy || (askName && !newName.trim())} busy={busy} busyLabel="Asking…">
          Fork into {target || "…"}{askName && newName.trim() ? `/${newName.trim()}` : ""}
        </Button>
      </div>
    </Modal>
  );
}
