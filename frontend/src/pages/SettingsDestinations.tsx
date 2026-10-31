// Settings → Destinations: the places a run's result and deliverables may be
// sent once it succeeds. A webhook is saved by name with its address, the *On
// my network* tick, a signing secret and, if the receiver wants one, an
// Authorization value. An S3 bucket (Amazon's, or any service that speaks its
// API) is saved with its bucket, prefix, region, an endpoint when it is not
// Amazon's, the tick, and an access key id and secret access key. The CLI keeps
// the same list (`agentry destinations`): the fields in its configuration
// folder and each credential in the system keychain, so a run, a schedule's fire
// and the command line all send to what is saved here. A credential goes in
// through a field and into Go; Go's views carry none back, so a saved one is
// shown as kept, never as a value.
import { useEffect, useRef, useState } from "react";
import * as DestinationsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/destinationsapi";
import * as destinations from "../../bindings/github.com/blaktron/agentry-desktop/internal/destinations/models";
import { Button, Card, ErrorBox, Field, Input, Modal, Notice } from "../ui";
import { credentialsLine, defaultLine, kindPhrase, keyPairLine, nameProblem, newSecret, objectPath, problemSentence } from "../destinations";

type Kind = "webhook" | "s3";

type Draft = {
  /** The saved name being edited, "" for a new destination. */
  editing: string;
  kind: Kind;
  name: string;
  url: string;
  onMyNetwork: boolean;
  secret: string;
  authorization: string;
  clearAuthorization: boolean;
  bucket: string;
  prefix: string;
  region: string;
  endpoint: string;
  accessKeyId: string;
};

const blank: Draft = {
  editing: "", kind: "webhook", name: "", url: "", onMyNetwork: false, secret: "", authorization: "", clearAuthorization: false,
  bucket: "", prefix: "", region: "", endpoint: "", accessKeyId: "",
};

const toInput = (d: Draft) =>
  destinations.Input.createFrom({
    name: d.name.trim(), kind: d.kind, url: d.url.trim(), onMyNetwork: d.onMyNetwork,
    bucket: d.bucket.trim(), prefix: d.prefix.trim(), region: d.region.trim(), endpoint: d.endpoint.trim(),
    secret: d.secret.trim(), accessKeyId: d.accessKeyId.trim(), authorization: d.authorization.trim(), clearAuthorization: d.clearAuthorization,
  });

export function DestinationsSettings() {
  const [status, setStatus] = useState<destinations.Status | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [problem, setProblem] = useState("");
  const [keeps, setKeeps] = useState(false);
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

  const isBucket = draft?.kind === "s3";
  useEffect(() => {
    if (!draft) { setProblem(""); return; }
    const n = ++asked.current;
    if (draft.kind === "s3") {
      if (draft.bucket.trim() === "" && draft.endpoint.trim() === "" && draft.prefix.trim() === "" && draft.region.trim() === "") { setProblem(""); return; }
      DestinationsAPI.CheckBucket(draft.bucket, draft.prefix, draft.region, draft.endpoint, draft.onMyNetwork)
        .then((p) => { if (n === asked.current) setProblem(p); })
        .catch(() => { if (n === asked.current) setProblem(""); });
      return;
    }
    if (draft.url.trim() === "") { setProblem(""); return; }
    DestinationsAPI.Check(draft.url, draft.onMyNetwork)
      .then((p) => { if (n === asked.current) setProblem(p); })
      .catch(() => { if (n === asked.current) setProblem(""); });
  }, [draft?.kind, draft?.url, draft?.onMyNetwork, draft?.bucket, draft?.prefix, draft?.region, draft?.endpoint]);

  // Whether the saved key pair stays with the bucket as it is now typed: Go
  // says, by the CLI's own rule, since a pair is kept only for the same service.
  useEffect(() => {
    if (!draft || draft.kind !== "s3" || !draft.editing) { setKeeps(false); return; }
    let live = true;
    DestinationsAPI.KeepsKeyPair(toInput(draft)).then((k) => live && setKeeps(k)).catch(() => live && setKeeps(false));
    return () => { live = false; };
  }, [draft?.kind, draft?.editing, draft?.bucket, draft?.region, draft?.endpoint, draft?.onMyNetwork]);

  const saved = draft ? status?.destinations.find((d) => d.name === draft.editing) : undefined;
  const taken = !!draft && draft.editing === "" && !!status?.destinations.some((d) => d.name === draft.name.trim());
  const nameBad = draft && draft.editing === "" ? (taken ? "A destination of that name is saved: edit it, or choose another name." : nameProblem(draft.name)) : "";
  const needsSecret = !!draft && (isBucket ? !keeps : !saved || !saved.hasSecret);
  const needsKeyId = !!draft && isBucket && !keeps;
  const where = isBucket ? draft.bucket.trim() !== "" : !!draft && draft.url.trim() !== "";
  const canSave = !!draft && !busy && !nameBad && where && !problem
    && (!needsSecret || draft.secret.trim() !== "") && (!needsKeyId || draft.accessKeyId.trim() !== "");

  async function save() {
    if (!draft) return;
    setBusy(true);
    setError(null);
    setNote(null);
    try {
      const next = await DestinationsAPI.Save(toInput(draft));
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
      setNote(`Removed ${name}, its credentials and its place in every QuickAgent's default set.`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const edit = (d: destinations.View) =>
    setDraft(d.kind === "s3"
      ? { ...blank, editing: d.name, kind: "s3", name: d.name, onMyNetwork: d.onMyNetwork, bucket: d.bucket ?? "", prefix: d.prefix ?? "", region: d.region ?? "", endpoint: d.endpoint ?? "" }
      : { ...blank, editing: d.name, name: d.name, url: "", onMyNetwork: d.onMyNetwork });
  const confirmed = status?.destinations.find((d) => d.name === confirm);

  return (
    <>
      <section id="settings-part-destinations" className="settings-part">
        <h2>Destinations</h2>
        <p className="muted small">
          A webhook receives one HTTPS request when a run succeeds: the run&apos;s id and release, its result and each
          deliverable with its checksum and content, signed with your secret so the receiver can check who sent it. An S3
          bucket (Amazon&apos;s, or any service that speaks its API, such as Cloudflare R2, MinIO or Backblaze B2) receives each
          deliverable as an object, signed with your access key. Choose where a run sends in its setup under Send
          deliverables to. The sending runs after the agent has ended, in a container that reaches that address alone, and
          it never changes whether the run succeeded.
        </p>
        {status?.keychainOff && (
          <Notice tone="warn">The system keychain is turned off for Agentry here, and a destination&apos;s credentials are kept there alone, so no destination can be saved.</Notice>
        )}
        {status && status.destinations.length === 0 && <p className="muted small">No destinations saved.</p>}
        {status && status.destinations.length > 0 && (
          <div className="sign-ins">
            {status.destinations.map((d) => (
              <Card key={d.name}>
                <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
                  <span className="selectable" data-destination={d.name} data-destination-kind={d.kind}>
                    <strong className="mono">{d.name}</strong> <span className="muted small">{kindPhrase(d)}{d.onMyNetwork ? ", on my network" : ""}</span>
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
          <p className="row">
            <Button variant="primary" disabled={busy || !!status?.keychainOff} onClick={() => setDraft({ ...blank })}>Add a webhook</Button>
            <Button disabled={busy || !!status?.keychainOff} onClick={() => setDraft({ ...blank, kind: "s3" })} data-add-bucket>Add an S3 bucket</Button>
          </p>
        )}
        {draft && (
          <Card>
            <h3 style={{ marginTop: 0 }}>{draft.editing ? `Edit ${draft.editing}` : isBucket ? "Add an S3 bucket" : "Add a webhook"}</h3>
            <Field label="Name" hint="Lower-case letters, digits, dots, hyphens and underscores.">
              <Input value={draft.name} disabled={busy || !!draft.editing} aria-label="Destination name" onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
            </Field>
            {nameBad && draft.name !== "" && <p className="small" role="alert">{nameBad}</p>}
            {!isBucket && (
              <Field
                label="Webhook address"
                hint={draft.editing ? "Type the address again to change it. It is not shown, since it may carry the receiver's own token." : "An https address. A token the receiver put in it is kept with it and sent to it."}
              >
                <Input value={draft.url} disabled={busy} aria-label="Webhook address" autoComplete="off" placeholder="https://hooks.example.com/webhook/…" onChange={(e) => setDraft({ ...draft, url: e.target.value })} />
              </Field>
            )}
            {isBucket && (
              <>
                <Field label="Bucket" hint="3 to 63 lower-case letters, digits, dots and hyphens.">
                  <Input value={draft.bucket} disabled={busy} aria-label="Bucket" autoComplete="off" placeholder="my-reports" onChange={(e) => setDraft({ ...draft, bucket: e.target.value })} />
                </Field>
                <Field label="Folder in the bucket (optional)" hint={`Each deliverable is put at ${objectPath(draft)}. Its name is made plain so it cannot leave that folder; an object already there is replaced.`}>
                  <Input value={draft.prefix} disabled={busy} aria-label="Folder in the bucket" autoComplete="off" placeholder="agentry" onChange={(e) => setDraft({ ...draft, prefix: e.target.value })} />
                </Field>
                <Field label="Region" hint={draft.endpoint.trim() ? "The signing region. us-east-1 when left empty; auto for Cloudflare R2." : "The bucket's region, for example us-east-1 or eu-west-1."}>
                  <Input value={draft.region} disabled={busy} aria-label="Region" autoComplete="off" placeholder="us-east-1" onChange={(e) => setDraft({ ...draft, region: e.target.value })} />
                </Field>
                <Field label="Service address (optional)" hint="Only when the service is not Amazon's: https://<account>.r2.cloudflarestorage.com, or a MinIO's address. No path.">
                  <Input value={draft.endpoint} disabled={busy} aria-label="Service address" autoComplete="off" placeholder="https://…" onChange={(e) => setDraft({ ...draft, endpoint: e.target.value })} />
                </Field>
              </>
            )}
            {problem && <p className="small" role="alert" data-destination-problem>{problemSentence(problem)}</p>}
            <label className="row gap">
              <input type="checkbox" checked={draft.onMyNetwork} disabled={busy} data-on-my-network onChange={(e) => setDraft({ ...draft, onMyNetwork: e.target.checked })} />
              <span>On my network</span>
            </label>
            <p className="muted small">
              {isBucket
                ? "Without this, the service is reached at a public address on port 443 over https. With it (and a service address), the service may be on your own network, such as a MinIO on your LAN or your tailnet, in http as well as https, on any port, and the run's record says so. An address in link-local space, where cloud metadata answers, is never reached."
                : "Without this, the address is reached at a public address on port 443 over https. With it, the address may be on your own network (a server on your LAN or your tailnet), in http as well as https, on any port, and the run's record says so. An address in link-local space, where cloud metadata answers, is never reached."}
            </p>
            {isBucket ? (
              <>
                <Field label="Access key id" hint={saved ? keyPairLine(keeps, !!saved.hasAccessKey && !!saved.hasSecret) : "Required."}>
                  <Input
                    type="password" className="mono" autoComplete="off" aria-label="Access key id"
                    placeholder={keeps ? "kept — type a new one to replace it" : ""}
                    value={draft.accessKeyId} disabled={busy} onChange={(e) => setDraft({ ...draft, accessKeyId: e.target.value })}
                  />
                </Field>
                <Field label="Secret access key" hint={keeps ? "Kept in your system keychain. Type a new one to replace it." : "Required. Kept in your system keychain and sent to the service by the delivery step alone."}>
                  <Input
                    type="password" className="mono" autoComplete="off" aria-label="Secret access key"
                    placeholder={keeps ? "kept — type a new one to replace it" : ""}
                    value={draft.secret} disabled={busy} onChange={(e) => setDraft({ ...draft, secret: e.target.value })}
                  />
                </Field>
                <p className="muted small">A key pair saved for one service is never sent to another: changing the service address or the tick asks for the pair again.</p>
              </>
            ) : (
              <>
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
              </>
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
            <span className="mono">{confirm}</span> is removed from this computer with its {confirmed?.kind === "s3" ? "access key id and secret access key" : "signing secret and any Authorization value"}, and from every QuickAgent&apos;s default set. A schedule that sends to it fails its runs&apos; delivery until you
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
