// Remote runhosts: the person's own Linux servers, set up from this app over
// SSH, that run their QuickAgents. The Server form sets one up (the first-run
// wizard's Remote path and Settings → Runhosts both use it); the plan
// sign-ins sign one in to a Claude, ChatGPT or Copilot plan from this
// computer's browser; Settings lists each one with its state, its default,
// its Unsafe Mode setting and its removal. The server's admin sign-in (a key
// file chosen in the native picker, or a password, and the sudo password)
// goes into Go for the one call and is kept nowhere; nothing comes back.
import { useEffect, useRef, useState } from "react";
import * as RunhostsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runhostsapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { Button, Card, ErrorBox, Field, Input, Notice } from "../primitives";
import { runhostNameProblem, signInWords, unsafeModeWords } from "../runhosts";

/** The narration of one runhost's calls, newest last. */
function useNarration(name: string): [string[], () => void] {
  const [lines, setLines] = useState<string[]>([]);
  useEffect(() => EventsOn("runhost:say", (v: { name: string; line: string }) => {
    if (v.name === name) setLines((l) => [...l.slice(-200), v.line]);
  }), [name]);
  return [lines, () => setLines([])];
}

function Narration({ lines }: { lines: string[] }) {
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => end.current?.scrollIntoView({ block: "nearest" }), [lines.length]);
  if (lines.length === 0) return null;
  return (
    <pre className="mono small runhost-log" aria-live="polite" data-runhost-log>
      {lines.join("\n")}
      <div ref={end} />
    </pre>
  );
}

type Admin = { user: string; keyFile: string; passphrase: string; password: string; sudoPassword: string };
const blankAdmin: Admin = { user: "", keyFile: "", passphrase: "", password: "", sudoPassword: "" };

const toAdmin = (a: Admin) => app.RunhostAdmin.createFrom({
  user: a.user.trim(), keyFile: a.keyFile, passphrase: a.passphrase, password: a.password, sudoPassword: a.sudoPassword,
});

/** The server's admin sign-in: the account, then a key file from the native
 *  picker or the account's password, and sudo's password when it asks. */
function AdminFields({ admin, setAdmin, host, busy }: { admin: Admin; setAdmin: (a: Admin) => void; host: string; busy: boolean }) {
  return (
    <>
      <Field label="Admin account" hint="root, or an account that can sudo. Used for this step alone, and kept nowhere.">
        <Input value={admin.user} disabled={busy} aria-label="Admin account" autoComplete="off" placeholder="root"
          onChange={(e) => setAdmin({ ...admin, user: e.target.value })} />
      </Field>
      <Field label="Sign in with" hint={admin.keyFile ? admin.keyFile : "Choose the account's private key, or type its password below."}>
        <span className="row">
          <Button disabled={busy} onClick={async () => {
            const p = await RunhostsAPI.ChooseAdminKeyFile(host);
            if (p) setAdmin({ ...admin, keyFile: p, password: "" });
          }}>{admin.keyFile ? "Choose another key" : "Choose a key file"}</Button>
          {admin.keyFile && <Button disabled={busy} onClick={() => setAdmin({ ...admin, keyFile: "", passphrase: "" })}>Use a password</Button>}
        </span>
      </Field>
      {admin.keyFile ? (
        <Field label="Key passphrase" hint="Only if the key has one.">
          <Input type="password" value={admin.passphrase} disabled={busy} aria-label="Key passphrase" autoComplete="off"
            onChange={(e) => setAdmin({ ...admin, passphrase: e.target.value })} />
        </Field>
      ) : (
        <Field label="Password">
          <Input type="password" value={admin.password} disabled={busy} aria-label="Admin password" autoComplete="off"
            onChange={(e) => setAdmin({ ...admin, password: e.target.value })} />
        </Field>
      )}
      <Field label="Sudo password" hint="When sudo asks for one. With a password sign-in, the same password is tried.">
        <Input type="password" value={admin.sudoPassword} disabled={busy} aria-label="Sudo password" autoComplete="off"
          onChange={(e) => setAdmin({ ...admin, sudoPassword: e.target.value })} />
      </Field>
    </>
  );
}

const adminReady = (a: Admin) => a.user.trim() !== "" && (a.keyFile !== "" || a.password !== "");

/** The Server step: name the runhost and the server, sign in as its
 *  administrator, and set it up. The preflight's findings and an unknown host
 *  key are put to the person in native dialogs. */
export function RunhostServerForm({ onAdded }: { onAdded: (v: app.RunhostView) => void }) {
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [admin, setAdmin] = useState<Admin>(blankAdmin);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lines, clear] = useNarration(name.trim());
  const nameBad = name === "" ? "" : runhostNameProblem(name.trim());
  const portNum = port.trim() === "" ? 0 : Number(port);
  const portBad = port.trim() !== "" && (!Number.isInteger(portNum) || portNum < 1 || portNum > 65535);
  const ready = !busy && name.trim() !== "" && !nameBad && host.trim() !== "" && !portBad && adminReady(admin);

  async function add() {
    setBusy(true);
    setError(null);
    clear();
    try {
      const v = await RunhostsAPI.Add(app.RunhostAddRequest.createFrom({ name: name.trim(), host: host.trim(), port: portNum, admin: toAdmin(admin) }));
      setAdmin(blankAdmin);
      if (v) onAdded(v);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="runhost-form" data-runhost-form>
      <Field label="Runhost name" hint="How this computer calls it: lower-case letters, digits and inner hyphens.">
        <Input value={name} disabled={busy} aria-label="Runhost name" autoComplete="off" placeholder="lab" onChange={(e) => setName(e.target.value)} />
      </Field>
      {nameBad && <p className="small" role="alert">{nameBad}</p>}
      <Field label="Server" hint="The server's address, as you reach it over SSH. Debian 12 or 13, or Ubuntu 22.04 or 24.04.">
        <span className="row">
          <Input value={host} disabled={busy} aria-label="Server address" autoComplete="off" placeholder="server.example.com" onChange={(e) => setHost(e.target.value)} />
          <Input value={port} disabled={busy} aria-label="SSH port" autoComplete="off" placeholder="22" style={{ width: "6em" }} onChange={(e) => setPort(e.target.value)} />
        </span>
      </Field>
      {portBad && <p className="small" role="alert">The port is a number from 1 to 65535.</p>}
      <AdminFields admin={admin} setAdmin={setAdmin} host={host.trim()} busy={busy} />
      <p className="row">
        <Button variant="primary" disabled={!ready} busy={busy} busyLabel="Setting it up…" onClick={add} data-runhost-add>Set it up</Button>
        {busy && <Button onClick={() => RunhostsAPI.Cancel()}>Stop</Button>}
      </p>
      <Narration lines={lines} />
      <ErrorBox error={error} />
    </div>
  );
}

/** The plan sign-ins of one runhost: each signs the runhost in from this
 *  computer's browser; the sign-in runs on the runhost and is kept there. */
export function RunhostSignIns({ runhost, onChange }: { runhost: app.RunhostView; onChange: (v: app.RunhostView) => void }) {
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [code, setCode] = useState<{ page: string; code: string } | null>(null);
  useEffect(() => {
    const offs = [
      EventsOn("signin:say", (v: { vendor: string; line: string }) => { if (v.vendor.startsWith("runhost:")) setLines((l) => [...l.slice(-100), v.line]); }),
      EventsOn("signin:code", (v: { vendor: string; page: string; code: string }) => { if (v.vendor.startsWith("runhost:")) setCode({ page: v.page, code: v.code }); }),
    ];
    return () => offs.forEach((off) => off());
  }, []);
  const kept = (vendor: string) => runhost.signIns.find((s) => s.vendor === vendor);

  async function signIn(vendor: string) {
    setBusy(vendor);
    setError(null);
    setLines([]);
    setCode(null);
    try {
      const v = await RunhostsAPI.SignIn(runhost.name, vendor);
      if (v) onChange(v);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
      setCode(null);
    }
  }

  async function remove(vendor: string) {
    setBusy(vendor);
    setError(null);
    try {
      const v = await RunhostsAPI.RemoveSignIn(runhost.name, vendor);
      if (v) onChange(v);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="runhost-signins" data-runhost-signins={runhost.name}>
      {(["claude", "chatgpt", "copilot"] as const).map((vendor) => {
        const k = kept(vendor);
        const w = signInWords(vendor);
        return (
          <div key={vendor} className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
            <span>
              <strong>{w.title}</strong>{" "}
              <span className="muted small">{k ? `signed in as ${k.label || "an account the vendor did not name"}` : w.blurb}</span>
            </span>
            <span className="row nowrap">
              <Button disabled={!!busy} busy={busy === vendor} busyLabel="Waiting for your browser…" onClick={() => signIn(vendor)} data-runhost-signin={vendor}>
                {k ? "Sign in again" : "Sign in"}
              </Button>
              {k && <Button variant="danger" disabled={!!busy} onClick={() => remove(vendor)}>Sign out</Button>}
            </span>
          </div>
        );
      })}
      {code && (
        <Notice>Enter the code <strong className="mono">{code.code}</strong> on GitHub&apos;s page, which opened in your browser.</Notice>
      )}
      {lines.length > 0 && <pre className="mono small runhost-log">{lines.join("\n")}</pre>}
      <ErrorBox error={error} />
    </div>
  );
}

/** A runhost's Agentry credits: a token of its own, on the person's account,
 *  that can only pay for model use, approved in the browser and kept on the
 *  runhost. A run there whose model is Agentry credits pays with it. */
export function RunhostCredits({ runhost, onChange }: { runhost: app.RunhostView; onChange: (v: app.RunhostView) => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function act(f: () => Promise<app.RunhostView | null>) {
    setBusy(true);
    setError(null);
    try {
      const v = await f();
      if (v) onChange(v);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="runhost-credits" data-runhost-credits={runhost.name}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <span>
          <strong>Agentry credits</strong>{" "}
          <span className="muted small">{runhost.credits
            ? `paid with the token "${runhost.credits}", which can do nothing else`
            : "a token that can only pay for model use with your credits, approved in your browser and kept on the runhost"}</span>
        </span>
        <span className="row nowrap">
          <Button disabled={busy} busy={busy} busyLabel="Waiting for your browser…" onClick={() => act(() => RunhostsAPI.Credits(runhost.name))}>
            {runhost.credits ? "Replace the token" : "Use my credits"}
          </Button>
          {runhost.credits && <Button variant="danger" disabled={busy} onClick={() => act(() => RunhostsAPI.RemoveCredits(runhost.name))}>Revoke</Button>}
        </span>
      </div>
      <ErrorBox error={error} />
    </div>
  );
}

/** Settings → Runhosts. */
export function RunhostsSettings() {
  const [list, setList] = useState<app.RunhostView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const load = () => RunhostsAPI.List().then(setList).catch((e) => setError(String(e)));
  useEffect(() => { load(); }, []);
  const put = (v: app.RunhostView) => setList((l) => (l ?? []).map((x) => (x.name === v.name ? v : x)));

  return (
    <section id="settings-part-runhosts" className="settings-part">
      <h2>Runhosts</h2>
      <p className="muted small">
        A runhost is a Linux server of yours that runs your QuickAgents. Agentry sets it up over SSH: its own account
        there, Docker Engine where there is none, gVisor, and a service that opens no network port and is reached only
        with this computer&apos;s key for it. A run on it is verified here, sent, verified again there, runs under gVisor,
        and its output comes back sealed to this computer.
      </p>
      <ErrorBox error={error} />
      {list && list.length === 0 && !adding && <p className="muted small">No runhosts yet.</p>}
      {list?.map((r) => <RunhostCard key={r.name} runhost={r} onChange={put} onGone={load} />)}
      {!adding && <p className="row"><Button variant="primary" onClick={() => setAdding(true)} data-runhost-new>Add a runhost</Button></p>}
      {adding && (
        <Card>
          <h3 style={{ marginTop: 0 }}>Add a runhost</h3>
          <RunhostServerForm onAdded={() => { setAdding(false); load(); }} />
          <p className="row"><Button onClick={() => setAdding(false)}>Close</Button></p>
        </Card>
      )}
    </section>
  );
}

function RunhostCard({ runhost, onChange, onGone }: { runhost: app.RunhostView; onChange: (v: app.RunhostView) => void; onGone: () => void }) {
  const [status, setStatus] = useState<app.RunhostStatusView | null>(null);
  const [asking, setAsking] = useState<"unsafe-on" | "unsafe-off" | "remove" | null>(null);
  const [admin, setAdmin] = useState<Admin>(blankAdmin);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [lines, clear] = useNarration(runhost.name);
  const refresh = () => RunhostsAPI.Status(runhost.name).then(setStatus).catch((e) => setError(String(e)));
  useEffect(() => { refresh(); }, [runhost.name]);

  async function withAdmin() {
    if (!asking) return;
    setBusy(true);
    setError(null);
    setNote(null);
    clear();
    try {
      if (asking === "remove") {
        await RunhostsAPI.Remove(runhost.name, toAdmin(admin), false, false);
        onGone();
        return;
      }
      await RunhostsAPI.SetUnsafeMode(runhost.name, asking === "unsafe-on", toAdmin(admin));
      setNote(asking === "unsafe-on" ? "Unsafe Mode is on for this runhost." : "Unsafe Mode is off for this runhost.");
      setAsking(null);
      setAdmin(blankAdmin);
      refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }} data-runhost={runhost.name}>
        <span className="selectable">
          <strong className="mono">{runhost.name}</strong>{" "}
          <span className="muted small">{runhost.host}{runhost.port && runhost.port !== 22 ? `:${runhost.port}` : ""} · {runhost.os || "Linux"} · CLI {status?.cliVersion || runhost.cliVersion}</span>
        </span>
        <span className="row nowrap">
          {runhost.default
            ? <span className="muted small">Runs go here by default</span>
            : <Button disabled={busy} onClick={async () => { try { await RunhostsAPI.SetDefault(runhost.name); onChange({ ...runhost, default: true } as app.RunhostView); } catch (e) { setError(String(e)); } }}>Run here by default</Button>}
          <Button variant="danger" disabled={busy} onClick={() => setAsking("remove")}>Remove</Button>
        </span>
      </div>
      <p className="muted small">
        Host key {runhost.fingerprint}. Owned by {runhost.owner || "the paired devices"}.{" "}
        {status === null ? "Checking it…" : status.reachable
          ? `Reached over this computer's key${status.running ? `; ${status.running} running` : ""}${status.waiting ? `; ${status.waiting} waiting to be collected` : ""}.`
          : `Not reached: ${status.problem}`}
        {status?.reachable && !status.sameVersion && " It runs another CLI version than this app: a run on it waits for its update."}
      </p>
      {status?.reachable && (
        <p className="small" data-runhost-unsafe={status.unsafeMode ? "on" : "off"}>
          <strong>Unsafe Mode:</strong> {unsafeModeWords(status.unsafeMode)}{" "}
          <Button disabled={busy} onClick={() => setAsking(status.unsafeMode ? "unsafe-off" : "unsafe-on")}>
            {status.unsafeMode ? "Turn it off" : "Turn it on"}
          </Button>
        </p>
      )}
      <RunhostSignIns runhost={runhost} onChange={onChange} />
      <RunhostCredits runhost={runhost} onChange={onChange} />
      {asking && (
        <Card>
          <p className="small">
            {asking === "remove"
              ? `Removing ${runhost.name} takes Agentry off ${runhost.host} with its admin sign-in. Its credits token is revoked and its sign-ins end first.`
              : `The setting is kept on ${runhost.host}, where only its administrator can change it: sign in as one.`}
          </p>
          <AdminFields admin={admin} setAdmin={setAdmin} host={runhost.host} busy={busy} />
          <p className="row">
            <Button variant={asking === "remove" ? "danger" : "primary"} disabled={busy || !adminReady(admin)} busy={busy} onClick={withAdmin}>
              {asking === "remove" ? "Remove it" : asking === "unsafe-on" ? "Turn Unsafe Mode on" : "Turn Unsafe Mode off"}
            </Button>
            <Button disabled={busy} onClick={() => { setAsking(null); setAdmin(blankAdmin); }}>Cancel</Button>
          </p>
        </Card>
      )}
      <Narration lines={lines} />
      {note && <Notice tone="ok">{note}</Notice>}
      <ErrorBox error={error} />
    </Card>
  );
}
