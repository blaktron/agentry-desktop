// Remote runhosts: the person's own Linux servers, set up from this app over
// SSH, that run their QuickAgents. The Server form sets one up (the first-run
// wizard's Remote path and Settings → Runs → Runhosts both use it); the plan
// sign-ins sign one in to a Claude, ChatGPT or Copilot plan from this
// computer's browser; Settings lists each one with its state, its last
// update, its default, its Unsafe Mode setting, its unpairing and its
// removal, and pairs this computer with a runhost set up from another. The server's admin sign-in (a key
// file chosen in the native picker, or a password, and the sudo password)
// goes into Go for the one call and is kept nowhere; nothing comes back.
import { useEffect, useRef, useState } from "react";
import * as RunhostsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/runhostsapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { EventsOn } from "../host";
import { Button, Card, ErrorBox, Field, Input, Notice } from "../primitives";
import { runhostNameProblem, signInWords, unsafeModeWords, updateWords, type RunhostUpdate } from "../runhosts";
import { ModelPicker } from "./ModelPicker";
import { modelSaid, type PickModel } from "../modelPick";

/** The saved models runhost name holds, asked of it: null until it
 *  answers, with the error when it could not. A runhost's agents and
 *  schedules name only these. */
function useRunhostModels(name: string, key: unknown = 0): { models: PickModel[] | null; error: string | null } {
  const [state, setState] = useState<{ models: PickModel[] | null; error: string | null }>({ models: null, error: null });
  useEffect(() => {
    let live = true;
    Promise.resolve().then(() => RunhostsAPI.Models(name))
      .then((v) => live && setState({ models: (v?.models ?? []).map((m) => ({ ...m, refusal: m.refusal ?? "" })), error: null }))
      .catch((e) => live && setState({ models: [], error: String(e) }));
    return () => { live = false; };
  }, [name, key]);
  return state;
}

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
 *  key are put to the person in native dialogs. With pair, the same form
 *  pairs this computer with a runhost set up from another computer instead:
 *  its keys alone go to the server. */
export function RunhostServerForm({ onAdded, pair = false }: { onAdded: (v: app.RunhostView) => void; pair?: boolean }) {
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
      const req = app.RunhostAddRequest.createFrom({ name: name.trim(), host: host.trim(), port: portNum, admin: toAdmin(admin) });
      const v = pair ? await RunhostsAPI.Pair(req) : await RunhostsAPI.Add(req);
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
      <Field label="Server" hint={pair
        ? "The runhost's address, as you reach it over SSH. It must belong to the Agentry account this app is signed in to."
        : "The server's address, as you reach it over SSH. Debian 12 or 13, Ubuntu 22.04 or 24.04, Fedora 43 or 44, or RHEL, AlmaLinux or Rocky Linux 9 or 10."}>
        <span className="row">
          <Input value={host} disabled={busy} aria-label="Server address" autoComplete="off" placeholder="server.example.com" onChange={(e) => setHost(e.target.value)} />
          <Input value={port} disabled={busy} aria-label="SSH port" autoComplete="off" placeholder="22" style={{ width: "6em" }} onChange={(e) => setPort(e.target.value)} />
        </span>
      </Field>
      {portBad && <p className="small" role="alert">The port is a number from 1 to 65535.</p>}
      <AdminFields admin={admin} setAdmin={setAdmin} host={host.trim()} busy={busy} />
      <p className="row">
        <Button variant="primary" disabled={!ready} busy={busy} busyLabel={pair ? "Pairing…" : "Setting it up…"} onClick={add} data-runhost-add={pair ? undefined : true} data-runhost-pair={pair ? true : undefined}>
          {pair ? "Pair this computer" : "Set it up"}
        </Button>
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

/** Settings → Runs → Runhosts. */
export function RunhostsSettings() {
  const [list, setList] = useState<app.RunhostView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState<"add" | "pair" | null>(null);
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
      {!adding && (
        <p className="row">
          <Button variant="primary" onClick={() => setAdding("add")} data-runhost-new>Add a runhost</Button>
          <Button onClick={() => setAdding("pair")} data-runhost-pair-new>Pair with a runhost set up on another computer</Button>
        </p>
      )}
      {adding && (
        <Card>
          <h3 style={{ marginTop: 0 }}>{adding === "pair" ? "Pair with a runhost" : "Add a runhost"}</h3>
          {adding === "pair" && (
            <p className="muted small">
              This computer gets its own key for a runhost you set up from another of your computers. The server&apos;s
              administrator signs in for this step alone, and nothing else on the server changes. A runhost on an older
              CLI than this app&apos;s is then updated to it.
            </p>
          )}
          <RunhostServerForm pair={adding === "pair"} onAdded={() => { setAdding(null); load(); }} />
          <p className="row"><Button onClick={() => setAdding(null)}>Close</Button></p>
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
  const [updating, setUpdating] = useState(false);
  const refresh = () => RunhostsAPI.Status(runhost.name).then(setStatus).catch((e) => setError(String(e)));
  useEffect(() => { refresh(); }, [runhost.name]);
  const update = updateWords(runhost.update as RunhostUpdate | undefined);

  async function updateNow() {
    setBusy(true);
    setUpdating(true);
    setError(null);
    clear();
    try {
      const v = await RunhostsAPI.Update(runhost.name);
      if (v) onChange(v);
      refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
      setUpdating(false);
    }
  }

  async function unpair() {
    setBusy(true);
    setError(null);
    clear();
    try {
      await RunhostsAPI.Unpair(runhost.name);
      onGone();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

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
          {runhost.linkedHere && (
            <span className="muted small" data-runhost-linked title="Its agents' use of the applications on this computer is answered by this app while it runs">
              {" "}· Linked to this computer
            </span>
          )}
        </span>
        <span className="row nowrap">
          {runhost.default
            ? <span className="muted small">Runs go here by default</span>
            : <Button disabled={busy} onClick={async () => { try { await RunhostsAPI.SetDefault(runhost.name); onChange({ ...runhost, default: true } as app.RunhostView); } catch (e) { setError(String(e)); } }}>Run here by default</Button>}
          <Button disabled={busy} onClick={unpair} data-runhost-unpair>Unpair</Button>
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
      {(update.line || (status?.reachable && !status.sameVersion)) && (
        <p className="small" data-runhost-update={update.attention ? "attention" : "ok"}>
          {update.attention ? <strong>{update.line}</strong> : update.line}{" "}
          {status?.reachable && !status.sameVersion && (
            <Button disabled={busy} busy={busy && updating} busyLabel="Updating…" onClick={updateNow} data-runhost-update-now>Update it now</Button>
          )}
        </p>
      )}
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
      {status?.reachable && <RunhostSchedules name={runhost.name} />}
      {status?.reachable && <RunhostAgents name={runhost.name} />}
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

/** The schedules a runhost runs with nobody present, from its store: each
 *  one's next run and what it keeps there, never a value, and Remove, which
 *  takes it off the runhost with everything it kept. */
function RunhostSchedules({ name }: { name: string }) {
  const [store, setStore] = useState<app.RunhostStoreView | null>(null);
  const { models } = useRunhostModels(name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    RunhostsAPI.Store(name).then(setStore).catch((e) => setError(String(e)));
  }, [name]);
  if (error) return <ErrorBox error={error} />;
  if (!store) return null;
  return (
    <div className="small" data-runhost-schedules={name}>
      <p style={{ marginBottom: 4 }}>
        <strong>Schedules:</strong>{" "}
        {!store.sealed
          ? "this runhost takes none: its systemd is older than 256, which seals its store's key to the server."
          : store.schedules.length === 0
            ? "none. Choose this runhost and Later or Repeat in a run's setup to make one."
            : `${store.schedules.length}, each run collected here when this computer reaches the runhost.`}
      </p>
      {store.schedules.map((sc) => (
        <p key={sc.id} className="row" style={{ justifyContent: "space-between", margin: "2px 0" }} data-runhost-schedule={sc.id}>
          <span className="selectable">
            <span className="mono">{sc.name}</span> <span className="muted">{sc.ref}</span>{" "}
            <span className="muted">
              {sc.ended ? "ended" : sc.next ? `next ${new Date(sc.next).toLocaleString()}` : "no run left"} · keeps {sc.keeps}
              {sc.model
                ? <span data-runhost-schedule-model> · on {models ? modelSaid(models, sc.model) : sc.model === "default" ? "the default" : "a saved model"}</span>
                : <span data-runhost-schedule-model> · on the model it was made with</span>}
            </span>
          </span>
          <Button disabled={busy} onClick={async () => {
            setBusy(true);
            setError(null);
            try {
              const v = await RunhostsAPI.RemoveSchedule(name, sc.id);
              if (v) setStore(v);
            } catch (e) {
              if (!/not confirmed/i.test(String(e))) setError(String(e));
            } finally {
              setBusy(false);
            }
          }}>Remove</Button>
        </p>
      ))}
    </div>
  );
}

/** The persistent agents on a runhost: each one runs there under the
 *  runhost's service, on one of the saved models the runhost holds (its
 *  Model list offers only those), and is started, stopped and talked to from
 *  here through the runhost's run key. A model change is taken at its next
 *  turn there. What one would be given for the first time there is asked in
 *  a native dialog. Agentry Helper stays on this computer. */
function RunhostAgents({ name }: { name: string }) {
  const [agents, setAgents] = useState<app.RunhostAgentView[] | null>(null);
  const { models, error: modelsError } = useRunhostModels(name, agents);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [talkTo, setTalkTo] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [reply, setReply] = useState<string | null>(null);
  useEffect(() => {
    RunhostsAPI.Agents(name).then(setAgents).catch((e) => setError(String(e)));
  }, [name]);
  const act = async (agent: string, f: () => Promise<app.RunhostAgentView[] | null>) => {
    setBusy(agent);
    setError(null);
    try {
      const v = await f();
      if (v) setAgents(v);
    } catch (e) {
      if (!/not confirmed|refusing/i.test(String(e))) setError(String(e));
    } finally {
      setBusy(null);
    }
  };
  const send = async (agent: string) => {
    setBusy(agent);
    setError(null);
    setReply(null);
    try {
      setReply(await RunhostsAPI.SendAgent(name, agent, text));
      setText("");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  };
  if (error && !agents) return <ErrorBox error={error} />;
  if (!agents) return null;
  return (
    <div className="small" data-runhost-agents={name}>
      <p style={{ marginBottom: 4 }}>
        <strong>Persistent agents:</strong> they run on {name}, each on a saved model it holds, and keep their conversation there.
      </p>
      {modelsError && <p className="muted small">{name}'s saved models could not be read: {modelsError}</p>}
      {models && models.length === 0 && !modelsError && (
        <p className="muted small" data-runhost-no-models>{name} holds no saved model: make one available on it in Settings → Models.</p>
      )}
      {agents.map((a) => (
        <div key={a.name} data-runhost-agent={a.name}>
          <p className="row" style={{ justifyContent: "space-between", margin: "2px 0" }}>
            <span className="selectable">
              {a.title || a.name} <span className="muted">{a.desktopOnly ? "runs on this computer only" : a.state}</span>
            </span>
            {!a.desktopOnly && (
              <span className="row">
                {a.state === "running" ? (
                  <>
                    <Button disabled={busy !== null} onClick={() => { setTalkTo(talkTo === a.name ? null : a.name); setReply(null); }}>Talk</Button>
                    <Button disabled={busy !== null} busy={busy === a.name} onClick={() => act(a.name, () => RunhostsAPI.StopAgent(name, a.name))}>Stop</Button>
                  </>
                ) : (
                  <Button disabled={busy !== null} busy={busy === a.name} busyLabel="Starting…" onClick={() => act(a.name, () => RunhostsAPI.StartAgent(name, a.name))}>Start</Button>
                )}
              </span>
            )}
          </p>
          {!a.desktopOnly && models && models.length > 0 && (
            <div data-runhost-agent-model={a.name}>
              <ModelPicker models={models} value={a.model?.id ?? ""} disabled={busy !== null} testId={`runhost-${a.name}`}
                label={`Model for ${a.title || a.name}`}
                hint={a.state === "running" ? "A change is taken at its next turn." : undefined}
                onChange={(id) => act(a.name, () => RunhostsAPI.SetAgentModel(name, a.name, id))} />
              {a.model?.refusal && <p className="run-step-problem">{a.model.refusal}</p>}
            </div>
          )}
          {talkTo === a.name && a.state === "running" && (
            <div style={{ margin: "4px 0 8px" }}>
              <p className="row">
                <Input value={text} onChange={(e) => setText(e.target.value)} placeholder={`A message to ${a.title || a.name} on ${name}`} disabled={busy !== null} />
                <Button disabled={busy !== null || text.trim() === ""} busy={busy === a.name} busyLabel="Waiting for its answer…" onClick={() => send(a.name)}>Send</Button>
              </p>
              {reply !== null && <p className="selectable" data-runhost-agent-reply={a.name} style={{ whiteSpace: "pre-wrap" }}>{reply}</p>}
            </div>
          )}
        </div>
      ))}
      <ErrorBox error={error} />
    </div>
  );
}

