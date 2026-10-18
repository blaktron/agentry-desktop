// Settings → Destinations: the places a run's result and deliverables may be
// sent once it succeeds. A webhook is saved by name with its address, the *On
// my network* tick, a signing secret and, if the receiver wants one, an
// Authorization value. The CLI keeps the same list (`agentry destinations`):
// the fields in its configuration folder and each credential in the system
// keychain, so a run, a schedule's fire and the command line all send to what
// is saved here. A credential goes in through a field and into Go; Go's views
// carry none back, so a saved one is shown as kept, never as a value.
import { useEffect, useRef, useState } from "react";
import * as DestinationsAPI from "../../wailsjs/go/app/DestinationsAPI";
import { destinations } from "../../wailsjs/go/models";
import { Button, Card, ErrorBox, Field, Input, Modal, Notice } from "../ui";
import { credentialsLine, defaultLine, nameProblem, newSecret, problemSentence } from "../destinations";

type Draft = {
  /** The saved name being edited, "" for a new destination. */
  editing: string;
  name: string;
  url: string;
  onMyNetwork: boolean;
  secret: string;
  authorization: string;
  clearAuthorization: boolean;
};

const blank: Draft = { editing: "", name: "", url: "", onMyNetwork: false, secret: "", authorization: "", clearAuthorization: false };

export function DestinationsSettings() {
  const [status, setStatus] = useState<destinations.Status | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [problem, setProblem] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<string | null>(null);
  // The address is checked by Go as it is typed, with the CLI's own rule; a
  // slower answer for an earlier address never overwrites a later one.
  const asked = useRef(0);

  useEffect(() => {
    DestinationsAPI.List().then(setStatus).catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    if (!draft || draft.url.trim() === "") { setProblem(""); return; }
    const n = ++asked.current;
    DestinationsAPI.Check(draft.url, draft.onMyNetwork)
      .then((p) => { if (n === asked.current) setProblem(p); })
      .catch(() => { if (n === asked.current) setProblem(""); });
  }, [draft?.url, draft?.onMyNetwork]);

  const saved = draft ? status?.destinations.find((d) => d.name === draft.editing) : undefined;
  const nameBad = draft && draft.editing === "" ? nameProblem(draft.name) : "";
  const needsSecret = !!draft && (!saved || !saved.hasSecret);
  const canSave = !!draft && !busy && !nameBad && draft.url.trim() !== "" && !problem && (!needsSecret || draft.secret.trim() !== "");

  async function save() {
    if (!draft) return;
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      const next = await DestinationsAPI.Save(destinations.Input.createFrom({
        name: draft.name.trim(), url: draft.url.trim(), onMyNetwork: draft.onMyNetwork,
        secret: draft.secret.trim(), authorization: draft.authorization.trim(), clearAuthorization: draft.clearAuthorization,
      }));
      setStatus(next);
      setNote(`Saved ${draft.name.trim()}.`);
      setDraft(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(name: string) {
    setConfirm(null);
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      setStatus(await DestinationsAPI.Remove(name));
      setNote(`Removed ${name}, its signing secret and its place in every QuickAgent's default set.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const edit = (d: destinations.View) =>
    setDraft({ ...blank, editing: d.name, name: d.name, url: "", onMyNetwork: d.onMyNetwork });

  return (
    <>
      <section id="settings-part-destinations" className="settings-part">
        <h2>Destinations</h2>
        <p className="muted small">
          A webhook receives one HTTPS request when a run succeeds: the run&apos;s id and release, its result and each
          deliverable with its checksum and content, signed with your secret so the receiver can check who sent it. Choose
          where a run sends in its setup under Send deliverables to. The sending runs after the agent has ended, in a
          container that reaches that address alone, and it never changes whether the run succeeded.
        </p>
        {status?.keychainOff && (
          <Notice tone="warn">The system keychain is turned off for Agentry here, and a signing secret is kept there alone, so no destination can be saved.</Notice>
        )}
        {status && status.destinations.length === 0 && <p className="muted small">No destinations saved.</p>}
        {status && status.destinations.length > 0 && (
          <div className="sign-ins">
            {status.destinations.map((d) => (
              <Card key={d.name}>
                <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
                  <span className="selectable" data-destination={d.name}>
                    <strong className="mono">{d.name}</strong> <span className="muted small">webhook to {d.where}{d.onMyNetwork ? ", on my network" : ""}</span>
                  </span>
                  <span className="row nowrap">
                    <Button onClick={() => edit(d)} disabled={busy}>Edit</Button>
                    <Button variant="danger" onClick={() => setConfirm(d.name)} disabled={busy}>Remove</Button>
                  </span>
                </div>
                <p className="muted small" style={{ marginBottom: 0 }}>
                  {credentialsLine(d)} {defaultLine(d)}
                </p>
              </Card>
            ))}
          </div>
        )}
        {!draft && (
          <p><Button variant="primary" disabled={busy || !!status?.keychainOff} onClick={() => setDraft({ ...blank })}>Add a destination</Button></p>
        )}
        {draft && (
          <Card>
            <h3 style={{ marginTop: 0 }}>{draft.editing ? `Edit ${draft.editing}` : "Add a destination"}</h3>
            <Field label="Name" hint="Lower-case letters, digits, dots, hyphens and underscores.">
              <Input value={draft.name} disabled={busy || !!draft.editing} aria-label="Destination name" onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
            </Field>
            {nameBad && draft.name !== "" && <p className="small" role="alert">{nameBad}</p>}
            <Field
              label="Webhook address"
              hint={draft.editing ? "Type the address again to change it. It is not shown, since it may carry the receiver's own token." : "An https address. A token the receiver put in it is kept with it and sent to it."}
            >
              <Input value={draft.url} disabled={busy} aria-label="Webhook address" autoComplete="off" placeholder="https://hooks.example.com/webhook/…" onChange={(e) => setDraft({ ...draft, url: e.target.value })} />
            </Field>
            {problem && <p className="small" role="alert" data-destination-problem>{problemSentence(problem)}</p>}
            <label className="row gap">
              <input type="checkbox" checked={draft.onMyNetwork} disabled={busy} data-on-my-network onChange={(e) => setDraft({ ...draft, onMyNetwork: e.target.checked })} />
              <span>On my network</span>
            </label>
            <p className="muted small">
              Without this, the address is reached at a public address on port 443 over https. With it, the address may be on
              your own network (a server on your LAN or your tailnet), in http as well as https, on any port, and the run&apos;s
              record says so. An address in link-local space, where cloud metadata answers, is never reached.
            </p>
            <Field
              label="Signing secret"
              hint={saved?.hasSecret ? "Kept in your system keychain. Type a new one to replace it." : "Required. The receiver uses it to check each request's signature."}
            >
              <div className="row">
                <Input
                  className="grow mono" type={draft.secret ? "text" : "password"} autoComplete="off" aria-label="Signing secret"
                  placeholder={saved?.hasSecret ? "kept — type a new one to replace it" : "Paste a secret, or make one"}
                  value={draft.secret} disabled={busy} onChange={(e) => setDraft({ ...draft, secret: e.target.value })}
                />
                <Button disabled={busy} onClick={() => setDraft({ ...draft, secret: newSecret() })}>Make one</Button>
              </div>
            </Field>
            {draft.secret && (
              <p className="muted small">The secret is shown only until you save. Copy it to the receiver now: after Save it is never shown again.</p>
            )}
            <Field
              label="Authorization value (optional)"
              hint={saved?.hasAuthorization ? "Kept in your system keychain. Type a new one to replace it, or tick Remove below." : "The whole value of an Authorization header the receiver wants, for example Bearer abc123."}
            >
              <Input
                type="password" className="mono" autoComplete="off" aria-label="Authorization value"
                placeholder={saved?.hasAuthorization ? "kept — type a new one to replace it" : ""}
                value={draft.authorization} disabled={busy || draft.clearAuthorization} onChange={(e) => setDraft({ ...draft, authorization: e.target.value })}
              />
            </Field>
            {saved?.hasAuthorization && (
              <label className="row gap">
                <input type="checkbox" checked={draft.clearAuthorization} disabled={busy} onChange={(e) => setDraft({ ...draft, clearAuthorization: e.target.checked, authorization: "" })} />
                <span>Remove the saved Authorization value</span>
              </label>
            )}
            {draft.editing && (
              <p className="muted small">An Authorization value saved for one host is never sent to another: changing the address to another host drops it.</p>
            )}
            <div className="row" style={{ justifyContent: "flex-end" }}>
              <Button disabled={busy} onClick={() => { setDraft(null); setProblem(""); }}>Cancel</Button>
              <Button variant="primary" disabled={!canSave} onClick={save}>{busy ? "Saving…" : "Save"}</Button>
            </div>
          </Card>
        )}
      </section>
      {note && <Notice tone="ok">{note}</Notice>}
      <ErrorBox error={error} />
      {confirm && (
        <Modal title="Remove this destination?" onClose={() => setConfirm(null)}>
          <p>
            <span className="mono">{confirm}</span> is removed from this computer with its signing secret and any Authorization
            value, and from every QuickAgent&apos;s default set. A schedule that sends to it fails its runs&apos; delivery until you
            edit the schedule. Nothing already sent is touched.
          </p>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setConfirm(null)}>Cancel</Button>
            <Button variant="danger" onClick={() => void remove(confirm)}>Remove</Button>
          </div>
        </Modal>
      )}
    </>
  );
}

/** The landing card's status line. */
export function destinationsStatus(v: Pick<destinations.Status, "destinations">): string {
  const n = v.destinations.length;
  if (n === 0) return "No destinations saved.";
  return n === 1 ? "1 destination saved." : `${n} destinations saved.`;
}
