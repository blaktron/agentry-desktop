// "Set up tools": the guided MCP-profile configuration as a
// standalone modal — the same form the run panel embeds, reachable from
// an installed QuickAgent without starting a run. Masked inputs, secret
// NAMES only; an empty field means "resolve from the environment at run
// time"; values leave this component the moment they are stored.
import { useEffect, useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { Button, Card, ErrorBox, Field, Input, Modal, Notice, Spinner, shortDigest } from "../ui";
import { connectHelp } from "../GuideText";
import { AuthorHelp } from "../KeyHelp";

export default function ToolsPanel({
  registry, ns, name, version, onClose,
}: {
  registry: string;
  ns: string;
  name: string;
  version: string;
  onClose: () => void;
}) {
  const agentRef = `${ns}/${name}@${version}`;
  const [profile, setProfile] = useState<app.ProfileStatusView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [keep, setKeep] = useState(true);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    RunAPI.ProfileStatus(registry, agentRef)
      .then((p) => { setProfile(p); setKeep(p.keep); })
      .catch((e) => setError(String(e)));
  }, [registry, agentRef]);

  async function save() {
    setSaving(true);
    setError(null);
    try {
      const p = await RunAPI.SaveProfile(registry, agentRef, values, keep);
      setProfile(p);
      setValues({});
      setNotice("Stored. The next run of this exact release uses the authenticated resources.");
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal title={`Set up tools — ${agentRef}`} onClose={onClose}>
      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {!profile && !error && <Spinner />}
      {profile && (
        <>
          <Card className="selectable">
            <div className="kv"><span className="k">release</span><span className="mono">{profile.ref}</span></div>
            <div className="kv"><span className="k">digest</span><span className="mono" title={profile.digest}>{shortDigest(profile.digest)}</span></div>
            {profile.capabilityLines.map((l) => (
              <div key={l} className="mono small">{l}</div>
            ))}
          </Card>

          {profile.declaredSecrets.length === 0 && (
            <p className="muted">
              This release declares no keys or identities — its gateway needs no
              configuration. Runs get exactly the declared tools, nothing else.
            </p>
          )}

          {profile.declaredSecrets.length > 0 && (
            <>
              <p className="muted small">
                Stored once per exact release (a different digest is a different trust
                decision). {profile.exists && (
                  <>Currently stored: <span className="mono">
                    {profile.storedSecretNames.length ? profile.storedSecretNames.join(", ") : "none — all resolve from the environment"}
                  </span>.</>
                )}
              </p>
              {profile.declaredSecrets.map((s) => (
                <Field
                  key={s}
                  label={<span className="mono">{s}{(profile.optionalSecrets ?? []).includes(s) && <span className="muted small"> optional</span>}</span>}
                  hint={connectHelp(profile.guide?.connect, s) ? <AuthorHelp name={s} help={connectHelp(profile.guide?.connect, s)} /> : undefined}
                >
                  <Input
                    type="password"
                    placeholder={(profile.optionalSecrets ?? []).includes(s)
                      ? "empty = from the environment, or the run starts without it"
                      : "empty = resolve from the environment at run time"}
                    value={values[s] ?? ""}
                    onChange={(e) => setValues({ ...values, [s]: e.target.value })}
                  />
                </Field>
              ))}
              <label className="check-row" style={{ marginTop: 10 }}>
                <input type="checkbox" checked={keep} onChange={(e) => setKeep(e.target.checked)} />
                <span>keep this profile after runs (uncheck: deleted when a run completes)</span>
              </label>
              <div className="row" style={{ marginTop: 12, justifyContent: "flex-end" }}>
                <Button onClick={onClose}>Close</Button>
                <Button variant="primary" busy={saving} busyLabel="Storing…" onClick={save}>
                  {profile.exists ? "Update profile" : "Store profile"}
                </Button>
              </div>
            </>
          )}
        </>
      )}
    </Modal>
  );
}
