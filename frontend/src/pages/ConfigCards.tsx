// A persistent agent's Configuration as four grids of cards: Apps and
// servers, Accounts and secrets, Skills, and Abilities and folders. A card
// is an icon, a name, one line and a state: off greyed, on in full colour.
// A click switches it, and a switch that widens what the agent has asks
// first in Go's native dialog, never in the page. Its menu holds Set up,
// Details and Remove; an install, a sign-in, a set-up and a failure (with
// Retry) are drawn on it. Each grid ends in its Add card: a server (the
// agent's suggestions first, one click each), a skill from a folder or a
// .zip, a secret whose value goes to Go and is never read back.
import { useEffect, useState } from "react";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import type { app, build } from "../../wailsjs/go/models";
import { addFormOf, cardIcon, cardMenu, cardState, cardSwitches, configGrids, googleScopeWords, googleWords, grantedWords, secretValueOK, setUpForm, SECRET_NAME, type AddForm, type AddKind, type GridKey, type RemoveAction, type SetUp } from "../configCards";
import { AddGlyph, ConfigIcon } from "../configGlyphs";
import { AGENT_STATES, SHARE_SWITCH_LABEL, agentCalled, canAddServer, portOpenable, retryChanges, serverRowOf, serverShareSwitch, shareSwitchNote, type ServerForm } from "../persistentAgents";
import { Button, Field, Input, Notice, OptionsMenu, Select } from "../ui";

type Act = (f: () => Promise<app.PersistentAgentView>, what?: string) => Promise<boolean>;

/** The four grids. withAdds draws the Add cards (the Configuration);
 *  without them the cards are only switched (the agent's page). */
export function ConfigCards({ agent, onChanged, onError, onSaved, withAdds = true }: {
  agent: app.PersistentAgentView;
  onChanged: (v: app.PersistentAgentView) => void;
  onError: (e: string | null) => void;
  onSaved?: (what: string) => void;
  withAdds?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState<AddForm | null>(null);
  // One change at a time: every card waits while one is applied.
  const act: Act = async (f, what) => {
    setBusy(true);
    onError(null);
    try {
      onChanged(await f());
      if (what) onSaved?.(what);
      return true;
    } catch (e) {
      onError(String(e));
      return false;
    } finally {
      setBusy(false);
    }
  };
  const [google, setGoogle] = useState(false);
  const setUp = (s: SetUp, grid: GridKey) => {
    const form = setUpForm(s, grid);
    if (!form) {
      setGoogle(true);
      return;
    }
    setAdding(form);
  };
  return (
    <div className="config-grids">
      {configGrids(agent, withAdds).map((g) => (
        <section key={g.key} className="config-grid-section" aria-label={g.title}>
          <h3>{g.title}</h3>
          <div className="config-grid">
            {g.entries.map((e) => (
              <ToggleCard key={e.id} agent={agent} entry={e} busy={busy} act={act} onSetUp={(s) => setUp(s, g.key)} />
            ))}
            {g.adds.map((kind) => (
              <button key={kind} type="button" className={`config-card config-card-add${adding?.kind === kind && adding.grid === g.key ? " open" : ""}`} disabled={busy}
                aria-expanded={adding?.kind === kind && adding.grid === g.key} onClick={() => setAdding(adding?.kind === kind && adding.grid === g.key ? null : addFormOf(kind, g.key))}>
                <AddGlyph />
                <span className="config-card-text">
                  <span className="config-card-name">{ADD_TITLES[kind]}</span>
                  <span className="config-card-line">{ADD_LINES[kind]}</span>
                </span>
              </button>
            ))}
          </div>
          {adding && adding.grid === g.key && (
            <div className="config-add-panel">
              {adding.kind === "server" && <AddServer agent={agent} busy={busy} act={act} onDone={() => setAdding(null)} />}
              {adding.kind === "skill" && <AddSkill agent={agent} busy={busy} act={act} onDone={() => setAdding(null)} />}
              {adding.kind === "secret" && <AddSecret agent={agent} busy={busy} act={act} name={adding.secretName ?? ""} onDone={() => setAdding(null)} />}
              {adding.kind === "account" && <AddAccount key={adding.host ?? ""} agent={agent} busy={busy} act={act} host={adding.host ?? ""} onDone={() => setAdding(null)} />}
            </div>
          )}
          {google && g.key === "apps" && agent.google && (
            <div className="config-add-panel">
              <GoogleSetUp agent={agent} busy={busy} act={act} onError={onError} onChanged={onChanged} onDone={() => setGoogle(false)} />
            </div>
          )}
        </section>
      ))}
    </div>
  );
}

const ADD_TITLES: Record<AddKind, string> = { server: "Add a server", skill: "Add a skill", secret: "Add a secret", account: "Add an account" };
const ADD_LINES: Record<AddKind, string> = {
  account: "An account on GitHub, GitLab or another git host, its own or yours",
  server: "A suggested one, or an npm or PyPI package, or a remote address",
  skill: "From a folder or a .zip holding a SKILL.md",
  secret: "A key or token it may give a server or a command",
};

/** One entry's card. */
function ToggleCard({ agent, entry: e, busy, act, onSetUp }: {
  agent: app.PersistentAgentView;
  entry: app.PersistentRosterEntry;
  busy: boolean;
  act: Act;
  onSetUp: (s: SetUp) => void;
}) {
  const [details, setDetails] = useState(false);
  const [removing, setRemoving] = useState(false);
  const state = cardState(agent, e);
  const switches = cardSwitches(e);
  const menu = cardMenu(e);
  // A server never given the share draws no switch; why is in Details.
  const shareSwitch = serverShareSwitch(e);
  const share = shareSwitch && !shareSwitch.disabled ? shareSwitch : null;
  const shareRefused = shareSwitch?.disabled ? shareSwitch.reason : undefined;
  const note = share && shareSwitchNote(agent, share);
  const flip = (id: string, on: boolean) => act(() => PersistentAPI.Configure(agent.name, { switch: { id, on } } as app.PersistentChange));
  const retry = () =>
    act(async () => {
      const [off, on] = retryChanges(e.id);
      await PersistentAPI.Configure(agent.name, off as app.PersistentChange);
      return PersistentAPI.Configure(agent.name, on as app.PersistentChange);
    });
  const items = [
    ...(menu.setUp ? [{ key: "setup", label: "Set up", onSelect: () => menu.setUp && onSetUp(menu.setUp) }] : []),
    { key: "details", label: details ? "Hide details" : "Details", onSelect: () => setDetails(!details) },
    ...(menu.remove ? [{ key: "remove", label: "Remove", onSelect: () => setRemoving(true), disabled: busy }] : []),
  ];
  return (
    <div className={`config-card tone-${state.tone}${e.on ? " on" : " off"}`}>
      <button type="button" className="config-card-main" role="switch" aria-checked={e.on} aria-label={`${e.name}: ${state.words}`}
        disabled={busy || !switches} onClick={() => void flip(e.id, !e.on)}>
        <ConfigIcon icon={cardIcon(e)} />
        <span className="config-card-text">
          <span className="config-card-name">{e.name}</span>
          <span className="config-card-line">{e.line}</span>
          <span className="config-card-state">{state.words}</span>
        </span>
      </button>
      <div className="config-card-menu">
        <OptionsMenu label="⋯" ariaLabel={`${e.name}: options`} items={items} />
      </div>
      {state.retry && (
        <div className="config-card-failed">
          <span className="small" role="alert">{state.reason ? `Could not be installed: ${state.reason}` : "Could not be installed"}</span>
          <Button variant="ghost" className="small" disabled={busy} onClick={() => void retry()}>Retry</Button>
        </div>
      )}
      {share && (
        <label className="check-row config-card-share">
          <input type="checkbox" role="switch" aria-label={`${e.name}: ${SHARE_SWITCH_LABEL}: ${share.on ? "on" : "off"}`}
            checked={share.on} disabled={busy} onChange={(ev) => void flip(share.id, ev.target.checked)} />
          <span className="small">
            {SHARE_SWITCH_LABEL}
            {note && <><br /><span className="muted">{note}</span></>}
          </span>
        </label>
      )}
      {portOpenable(agent, e) && (
        <Button variant="ghost" className="small" onClick={() => void PersistentAPI.OpenPort(agent.name, e.id.replace(/^port:/, "")).catch(() => undefined)}>
          Open in browser
        </Button>
      )}
      {details && (
        <div className="config-card-details small">
          <p>{e.line}</p>
          <p className="muted">{grantedWords(e)} · {state.words}</p>
          {state.reason && !state.retry && <p>{state.reason}</p>}
          {shareRefused && <p>Never given your share folder: {shareRefused}</p>}
        </div>
      )}
      {removing && menu.remove && <RemoveConfirm agent={agent} action={menu.remove} busy={busy} act={act} onCancel={() => setRemoving(false)} />}
    </div>
  );
}

/** Remove, confirmed on the card: the server, pack, port, skill or secret
 *  goes, and a secret's value leaves the keychain with it. */
function RemoveConfirm({ agent, action, busy, act, onCancel }: { agent: app.PersistentAgentView; action: RemoveAction; busy: boolean; act: Act; onCancel: () => void }) {
  const run = () => {
    switch (action.kind) {
      case "change":
        return act(() => PersistentAPI.Configure(agent.name, action.change as app.PersistentChange), `${action.what}'s removal`);
      case "skill":
        return act(() => PersistentAPI.RemoveSkill(agent.name, action.name), `${action.what}'s removal`);
      case "secret":
        return act(() => PersistentAPI.RemoveSecret(agent.name, action.name), `${action.what}'s removal`);
      case "account":
        return act(() => PersistentAPI.RemoveAccount(agent.name, action.host), `${action.what}'s removal`);
    }
  };
  return (
    <div className="config-card-confirm small" role="group" aria-label="Remove">
      <span>
        {action.kind === "secret" ? `Remove ${action.what.toLowerCase()} and its value from your keychain?`
          : action.kind === "account" ? `Remove ${action.what.toLowerCase()} and its token from your keychain?`
            : `Remove ${action.what.toLowerCase()}?`}
      </span>
      <Button variant="danger" className="small" disabled={busy} onClick={() => void run()}>Remove</Button>
      <Button variant="ghost" className="small" onClick={onCancel}>Cancel</Button>
    </div>
  );
}

/** The server Add card's panel: the agent's suggestions, one click each,
 *  then a package or a remote address of the person's own. */
function AddServer({ agent, busy, act, onDone }: { agent: app.PersistentAgentView; busy: boolean; act: Act; onDone: () => void }) {
  const [server, setServer] = useState<ServerForm>({ source: "npm", package: "", version: "", url: "", tools: "" });
  return (
    <>
      {(agent.suggested ?? []).length > 0 && (
        <div className="config-suggestions">
          <h4>Suggested for the {agent.title}</h4>
          {(agent.suggested ?? []).map((s) => (
            <div key={s.key} className="config-suggestion">
              <ConfigIcon icon={s.iconKey ? { kind: "glyph", key: s.iconKey } : { kind: "monogram", text: s.name.slice(0, 1).toUpperCase() }} />
              <span className="config-card-text">
                <span className="config-card-name">{s.name}</span>
                <span className="config-card-line">{s.line}</span>
              </span>
              <Button disabled={busy} onClick={() => void act(() => PersistentAPI.AddSuggested(agent.name, s.key), `The server ${s.name}`).then((ok) => ok && onDone())}>Add</Button>
            </div>
          ))}
        </div>
      )}
      <div className="persistent-server-form">
        <Field label="Source">
          <Select value={server.source} onChange={(e) => setServer({ ...server, source: e.target.value })}>
            <option value="npm">npm package</option>
            <option value="pypi">PyPI package</option>
            <option value="url">Remote server (https)</option>
          </Select>
        </Field>
        {server.source === "url" ? (
          <Field label="Address"><Input value={server.url} placeholder="https://…" onChange={(e) => setServer({ ...server, url: e.target.value })} /></Field>
        ) : (
          <>
            <Field label="Package"><Input value={server.package} onChange={(e) => setServer({ ...server, package: e.target.value })} /></Field>
            <Field label="Version" hint="An exact version, as a QuickAgent pins it."><Input value={server.version} onChange={(e) => setServer({ ...server, version: e.target.value })} /></Field>
          </>
        )}
        <Field label="Tools" hint="Only these tools, comma-separated; empty is all of them."><Input value={server.tools} onChange={(e) => setServer({ ...server, tools: e.target.value })} /></Field>
        <Button disabled={busy || !canAddServer(server)}
          onClick={() => void act(() => PersistentAPI.AddServer(agent.name, serverRowOf(server) as build.MCPServerRow), "The server").then((ok) => ok && onDone())}>
          Add server
        </Button>
        <p className="small muted config-add-note">Its card's icon is looked up once in the MCP Registry, by the package's name or a public address's host, when you add it.</p>
      </div>
    </>
  );
}

/** The skill Add card's panel: the native picker opens in Go, which checks
 *  the files and asks before the skill is on. */
function AddSkill({ agent, busy, act, onDone }: { agent: app.PersistentAgentView; busy: boolean; act: Act; onDone: () => void }) {
  const add = (from: string) =>
    void act(() => PersistentAPI.AddSkill(agent.name, from)).then((ok) => ok && onDone());
  return (
    <div className="row gap">
      <p className="small muted">
        A skill is a folder holding a SKILL.md, or a .zip of one.{" "}
        {agent.malwareCheck ? `ClamAV checks its files before the ${agent.title} has it.` : "The malware check is off on this computer, so its files are not checked."}
      </p>
      <Button disabled={busy} onClick={() => add("folder")}>From a folder…</Button>
      <Button disabled={busy} onClick={() => add("zip")}>From a .zip…</Button>
    </div>
  );
}

/** The secret Add card's panel: a name and a value. The value goes to Go,
 *  into your system keychain, and the field is emptied; it never comes back
 *  to the page. */
function AddSecret({ agent, busy, act, name: given, onDone }: { agent: app.PersistentAgentView; busy: boolean; act: Act; name: string; onDone: () => void }) {
  const [name, setName] = useState(given);
  const [value, setValue] = useState("");
  const nameOK = SECRET_NAME.test(name.trim());
  const valueOK = secretValueOK(value);
  return (
    <div className="persistent-server-form">
      <Field label="Name" hint="As a program reads it: capitals, digits and underscores, such as BRAVE_API_KEY.">
        <Input value={name} maxLength={64} autoComplete="off" spellCheck={false} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label="Value" hint="Kept in your system keychain, never shown again. At least 8 characters, on one line.">
        <Input type="password" value={value} autoComplete="off" onChange={(e) => setValue(e.target.value)} />
      </Field>
      <Button disabled={busy || !nameOK || !valueOK} onClick={() => {
        const v = value;
        setValue("");
        void act(() => PersistentAPI.SetSecret(agent.name, name.trim(), v), `The secret ${name.trim()}`).then((ok) => ok && onDone());
      }}>
        Keep the secret
      </Button>
    </div>
  );
}

/** The account Add card's panel: whose account it is first, with what that
 *  means (Go's own words for the host's kind), then the host, the user, the
 *  commit identity and the token. The token goes to Go, into your system
 *  keychain, and the field is emptied; it never comes back to the page. */
function AddAccount({ agent, busy, act, host: given, onDone }: { agent: app.PersistentAgentView; busy: boolean; act: Act; host: string; onDone: () => void }) {
  const existing = (agent.accounts ?? []).find((g) => g.host === given);
  const [own, setOwn] = useState<boolean | null>(existing ? existing.own : null);
  const [host, setHost] = useState(given || "github.com");
  const [known, setKnown] = useState("");
  const [kind, setKind] = useState(existing?.kind ?? "");
  const [user, setUser] = useState(existing?.user ?? "");
  const [name, setName] = useState(existing && existing.name !== existing.user ? existing.name : "");
  const [email, setEmail] = useState(existing?.email ?? "");
  const [token, setToken] = useState("");
  const [guidance, setGuidance] = useState("");
  const theKind = known || kind;
  useEffect(() => {
    let on = true;
    PersistentAPI.AccountKind(host).then((k) => on && setKnown(k)).catch(() => on && setKnown(""));
    return () => { on = false; };
  }, [host]);
  useEffect(() => {
    let on = true;
    if (own === null) { setGuidance(""); return; }
    PersistentAPI.AccountGuidance(theKind, own).then((g) => on && setGuidance(g)).catch(() => on && setGuidance(""));
    return () => { on = false; };
  }, [own, theKind]);
  const ready = own !== null && host.trim() !== "" && theKind !== "" && user.trim() !== "" && email.trim() !== "" && secretValueOK(token);
  return (
    <div className="config-account-form">
      <fieldset className="config-account-whose">
        <legend className="small">Whose account is it?</legend>
        <label className="check-row"><input type="radio" name="account-whose" checked={own === true} onChange={() => setOwn(true)} /> Its own account (recommended)</label>
        <label className="check-row"><input type="radio" name="account-whose" checked={own === false} onChange={() => setOwn(false)} /> Your account</label>
      </fieldset>
      {guidance && <p className="small">{guidance}</p>}
      <div className="persistent-server-form">
        <Field label="Host" hint="github.com, gitlab.com, or your own host's name.">
          <Input value={host} disabled={!!existing} spellCheck={false} onChange={(e) => setHost(e.target.value)} />
        </Field>
        {!known && (
          <Field label="Kind of host">
            <Select value={kind} onChange={(e) => setKind(e.target.value)}>
              <option value="">Choose…</option>
              <option value="github">GitHub Enterprise Server</option>
              <option value="gitlab">GitLab, self-managed</option>
              <option value="gitea">Gitea or Forgejo</option>
              <option value="git">Another git host</option>
            </Select>
          </Field>
        )}
        <Field label="User name" hint="The account's user name on the host.">
          <Input value={user} autoComplete="off" spellCheck={false} onChange={(e) => setUser(e.target.value)} />
        </Field>
        <Field label="Commit name" hint="Empty is the user name.">
          <Input value={name} autoComplete="off" onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Commit email" hint="The account's own, or the host's private address for it.">
          <Input value={email} autoComplete="off" spellCheck={false} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Token" hint="Kept in your system keychain, never shown again.">
          <Input type="password" value={token} autoComplete="off" onChange={(e) => setToken(e.target.value)} />
        </Field>
        <Button disabled={busy || !ready} onClick={() => {
          const t = token;
          setToken("");
          const form = { host: host.trim(), kind: known ? "" : kind, own: own === true, user: user.trim(), name: name.trim(), email: email.trim(), token: t };
          void act(() => PersistentAPI.SetAccount(agent.name, form as app.PersistentAccountForm), `The account on ${form.host}`).then((ok) => ok && onDone());
        }}>
          Keep the account
        </Button>
      </div>
    </div>
  );
}

/** Google Workspace's set-up: the person's Google Cloud client from a file
 *  the native picker returns, read in Go and never by the page, then the
 *  sign-in in the browser. It reads signed in only once the server names
 *  the account. Neither runs while the agent runs. */
function GoogleSetUp({ agent, busy, act, onError, onChanged, onDone }: {
  agent: app.PersistentAgentView; busy: boolean; act: Act;
  onError: (e: string | null) => void; onChanged: (v: app.PersistentAgentView) => void; onDone: () => void;
}) {
  const g = agent.google;
  const [email, setEmail] = useState(g?.account ?? "");
  const [started, setStarted] = useState(false);
  const [address, setAddress] = useState<string | null>(null);
  const waiting = started || !!g?.waiting;
  const running = agent.state === AGENT_STATES.running || agent.state === AGENT_STATES.starting;
  const emailOK = /^[^@\s-][^@\s]*@[^@\s]+\.[a-z]{2,}$/i.test(email.trim());
  // Go tells the page a waiting sign-in's address, Google's own, to show
  // as text for when the browser did not open.
  useEffect(() => EventsOn("persistent:google-sign-in", (v: { agent?: string; url?: string }) => {
    if (v?.agent === agent.name && typeof v.url === "string") setAddress(v.url);
  }), [agent.name]);
  const signIn = async () => {
    setStarted(true);
    setAddress(null);
    onError(null);
    try {
      onChanged(await PersistentAPI.GoogleSignIn(agent.name, email.trim()));
    } catch (e) {
      onError(String(e));
    } finally {
      setStarted(false);
      setAddress(null);
    }
  };
  return (
    <div className="config-google-form">
      <p className="small">{googleWords(g)}</p>
      {running && <Notice>Stop {agentCalled(agent)} first: its Google Workspace server holds what the set-up writes.</Notice>}
      <p className="small muted">
        Google Workspace signs in with a Google Cloud client of your own: a Desktop app client you download from Google&apos;s
        console. The google-workspace-setup skill walks you through making one: ask {agentCalled(agent)} to help you set up
        Google Workspace. The file stays on this computer: Agentry hands it to the Google Workspace server, which keeps its
        secret in its own encrypted keyring.
      </p>
      <div className="row gap">
        <Button disabled={busy || waiting || running} onClick={() => void act(() => PersistentAPI.SetGoogleClient(agent.name), "Google Workspace's client")}>
          {g?.client ? "Choose another client file…" : "Choose the client file…"}
        </Button>
      </div>
      <Field label="Your Google account" hint="The address you sign in to Google with.">
        <Input value={email} autoComplete="off" spellCheck={false} disabled={!g?.client || waiting} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <p className="small muted">{googleScopeWords(agent.roster)}</p>
      <div className="row gap">
        <Button variant="primary" disabled={busy || waiting || running || !g?.client || !emailOK} onClick={() => void signIn()}>
          {g?.account ? "Sign in again" : "Sign in with Google"}
        </Button>
        {waiting && <Button onClick={() => void PersistentAPI.CancelGoogleSignIn(agent.name)}>Cancel</Button>}
        <Button variant="ghost" disabled={started} onClick={onDone}>Close</Button>
      </div>
      {waiting && (
        <div className="small" role="status">
          <p>Waiting for Google in your browser. Sign in there and allow Agentry; this carries on when Google answers.</p>
          {address && <p>If your browser did not open, go to: <span className="mono selectable config-google-address">{address}</span></p>}
        </div>
      )}
    </div>
  );
}
