// A host intake question in the run setup's Details step: the server's
// address as user@host[:port], the hosts this computer's runs reached
// before, then the sign-in: the one saved on this computer for that account
// and host, or a private key file chosen in the native picker (with its
// passphrase) or the account's password, and an optional sudo password.
// What is typed here goes to Go with the run, for the host link alone, and
// can be saved in the system keychain as the run starts, where the CLI and
// a schedule's fire read it too. Go reads the address with the CLI's own
// parser and never returns a sign-in. The host key is checked when the run
// starts; one no known_hosts file holds is shown in a native dialog.
import { useState } from "react";
import * as RunAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import { Button } from "../ui";
import { recentOffers, signInWords, type HostForm, type TargetHost } from "../hosts";

export function HostQuestion({
  q, value, onChange, status, onStatus, form, onForm, recent, onRecent, disabled, onError, scheduling = false,
}: {
  q: { id: string; prompt: string; type: string; required: boolean };
  value: string;
  onChange: (value: string) => void;
  /** Go's word on the current answer; null while it is being checked. */
  status: TargetHost | null;
  /** A removal answers the host's new standing. */
  onStatus: (s: TargetHost) => void;
  /** The sign-in for the account the answer names; null until it parses. */
  form: HostForm | null;
  onForm: (f: HostForm) => void;
  /** This computer's recent hosts, newest first. */
  recent: string[];
  onRecent: (hosts: string[]) => void;
  disabled: boolean;
  onError: (e: string) => void;
  /** The run is a schedule: what is typed is kept in the keychain. */
  scheduling?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const answered = value.trim() !== "";
  const ok = !!status && !status.problem && !!status.account;
  const offers = recentOffers(recent, value);
  const set = (patch: Partial<HostForm>) => form && onForm({ ...form, ...patch });

  async function chooseKey() {
    try {
      const p = await RunAPI.ChooseHostKeyFile(status?.answer ?? "");
      if (p) set({ keyFile: p });
    } catch (e) {
      onError(String(e));
    }
  }

  async function forget() {
    setBusy(true);
    try {
      const s = await RunAPI.ForgetHostSignIn(value.trim());
      onStatus(s);
      if (form) onForm({ ...form, way: "key" });
    } catch (e) {
      onError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function forgetRecent(h: string) {
    try {
      onRecent(await RunAPI.ForgetRecentHost(h));
    } catch (e) {
      onError(String(e));
    }
  }

  const label = { minWidth: 260 };
  return (
    <div className="host-question" style={{ marginBottom: 12 }}>
      <div className="row" style={{ marginBottom: 6 }}>
        <span style={label}>{q.prompt}{q.required ? " *" : ""}</span>
        <input
          type="text"
          className="grow mono"
          autoComplete="off"
          spellCheck={false}
          placeholder="admin@server.example.com, or admin@192.168.1.20:2222"
          aria-label={`${q.prompt}: the server, as user@host`}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
        />
      </div>
      {offers.length > 0 && (
        <div className="row small" style={{ marginBottom: 6, gap: 6, flexWrap: "wrap" }}>
          <span className="muted" style={label}>Recent on this computer</span>
          {offers.map((h) => (
            <span key={h} className="row" style={{ gap: 0 }}>
              <Button variant="link" onClick={() => onChange(h)} disabled={disabled} aria-label={`Use ${h}`}>
                <span className="mono">{h}</span>
              </Button>
              <Button variant="ghost" className="muted" style={{ marginLeft: 4, padding: "0 6px" }} onClick={() => forgetRecent(h)} disabled={disabled} aria-label={`Forget ${h} from the recent hosts`} title="Forget it here">×</Button>
            </span>
          ))}
        </div>
      )}
      {answered && status?.problem && <p className="small" style={{ color: "var(--color-sev-high)", margin: "0 0 6px" }}>{status.problem}</p>}
      {answered && !status && <p className="muted small" style={{ margin: "0 0 6px" }}>Checking the address…</p>}
      {ok && form && status && (
        <>
          <p className="muted small" style={{ margin: "0 0 6px" }}>
            Signs in as <span className="mono">{status.user}</span> to <span className="mono">{status.host}</span>, port {status.port}.
          </p>
          <div className="row" role="radiogroup" aria-label={`${q.prompt}: how to sign in`} style={{ gap: 14, marginBottom: 6 }}>
            <span className="muted small" style={label}>Sign in with</span>
            {status.saved && (
              <label className="check-row" style={{ margin: 0 }}>
                <input type="radio" name={`host-way-${q.id}`} checked={form.way === "saved"} onChange={() => set({ way: "saved" })} disabled={disabled} />
                <span>the sign-in saved here</span>
              </label>
            )}
            <label className="check-row" style={{ margin: 0 }}>
              <input type="radio" name={`host-way-${q.id}`} checked={form.way === "key"} onChange={() => set({ way: "key" })} disabled={disabled} />
              <span>a private key file</span>
            </label>
            <label className="check-row" style={{ margin: 0 }}>
              <input type="radio" name={`host-way-${q.id}`} checked={form.way === "password"} onChange={() => set({ way: "password" })} disabled={disabled} />
              <span>a password</span>
            </label>
          </div>
          {form.way === "key" && (
            <>
              <div className="row" style={{ marginBottom: 6 }}>
                <span className="muted small" style={label}>Private key file</span>
                <input type="text" className="grow mono" value={form.keyFile} readOnly placeholder="choose it: ~/.ssh/id_ed25519, for one" aria-label={`${q.prompt}: the private key file`} />
                <Button onClick={chooseKey} disabled={disabled}>{form.keyFile ? "Another key…" : "Choose…"}</Button>
              </div>
              <div className="row" style={{ marginBottom: 6 }}>
                <span className="muted small" style={label}>Its passphrase</span>
                <input
                  type="password" className="grow mono" autoComplete="off" placeholder="only if the key has one"
                  aria-label={`${q.prompt}: the key's passphrase`} value={form.passphrase}
                  onChange={(e) => set({ passphrase: e.target.value })} disabled={disabled}
                />
              </div>
            </>
          )}
          {form.way === "password" && (
            <div className="row" style={{ marginBottom: 6 }}>
              <span className="muted small" style={label}>Password for {status.user}</span>
              <input
                type="password" className="grow mono" autoComplete="off"
                aria-label={`${q.prompt}: the password for ${status.answer}`} value={form.password}
                onChange={(e) => set({ password: e.target.value })} disabled={disabled}
              />
            </div>
          )}
          <div className="row" style={{ marginBottom: 6 }}>
            <span className="muted small" style={label}>sudo password</span>
            <input
              type="password" className="grow mono" autoComplete="off"
              placeholder={form.way === "saved" ? "the saved one, if any; or type one for this run" : "optional: only if the work needs sudo and sudo asks for one"}
              aria-label={`${q.prompt}: the sudo password`} value={form.sudo}
              onChange={(e) => set({ sudo: e.target.value })} disabled={disabled}
            />
          </div>
          {!status.keychainOff && !scheduling && (form.way !== "saved" || form.sudo !== "") && (
            <label className="check-row">
              <input type="checkbox" checked={form.save} onChange={(e) => set({ save: e.target.checked })} disabled={disabled} />
              <span>
                Save this sign-in on this computer for {status.answer}{" "}
                <span className="muted">(the system keychain, where later runs, schedules and the agentry command read it; a key is saved as its file's path)</span>
              </span>
            </label>
          )}
          <p className="muted small" style={{ margin: "0 0 4px" }}>
            This run signs in {signInWords(status, form, scheduling)}.{" "}
            The sign-in goes to the run's host link alone, never to the agent or its tools, and is masked in every file the run leaves.
            {" "}The host key is checked when the run starts: if neither your known_hosts nor Agentry's holds it, a dialog shows its fingerprint for you to accept.
            {scheduling && " A schedule keeps the sign-in in the system keychain, where its runs read it, and refuses a run whose host offers another key."}
          </p>
          {status.saved && (
            <div className="row" style={{ gap: 8 }}>
              <Button variant="link" onClick={forget} disabled={disabled || busy}>Remove the saved sign-in</Button>
            </div>
          )}
          {status.keychainOff && <p className="muted small">The system keychain is off on this computer (AGENTRY_NO_KEYRING), so no sign-in is saved or read.</p>}
          {status.keyStoreErr && <p className="muted small">The system keychain could not be read: {status.keyStoreErr}</p>}
        </>
      )}
    </div>
  );
}
