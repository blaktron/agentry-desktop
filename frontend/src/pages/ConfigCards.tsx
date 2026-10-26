// A persistent agent's Configuration as grids of cards: Apps and servers,
// the Assistant's Mail and calendar, Accounts and secrets, Skills, and
// Abilities and folders. A card
// is an icon, a name, one line and a state: off greyed, on in full colour.
// A click switches it, and a switch that widens what the agent has asks
// first in Go's native dialog, never in the page. Its menu holds Set up,
// Details and Remove; an install, a sign-in, a set-up and a failure (with
// Retry) are drawn on it. Each grid ends in its Add card: a server (the
// agent's suggestions first, one click each), a skill from a folder or a
// .zip, a secret whose value goes to Go and is never read back, a mail
// provider to enable. A mail provider's server card can disable its
// provider, which keeps the sign-in.
import { useEffect, useState } from "react";
import * as PersistentAPI from "../../wailsjs/go/app/PersistentAPI";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import type { app, build } from "../../wailsjs/go/models";
import { addFormOf, cardIcon, cardMenu, cardState, cardSwitches, configGrids, disabledProviders, ENTRA_ID, fastmailWords, microsoftAdminWords, microsoftCanSignIn, googleScopeWords, microsoftScopeWords, microsoftWords, MICROSOFT_TENANTS, providerWords, googleWords, grantedWords, secretValueOK, setUpForm, SECRET_NAME, type AddForm, type AddKind, type GridKey, type RemoveAction, type SetUp } from "../configCards";
import { AddGlyph, ConfigIcon } from "../configGlyphs";
import { MeetingNotesCard } from "./MeetingNotesCard";
import { AGENT_STATES, SHARE_SWITCH_LABEL, agentCalled, canAddServer, portOpenable, retryChanges, serverRowOf, serverShareSwitch, shareSwitchNote, type ServerForm } from "../persistentAgents";
import { Button, Field, Input, Notice, OptionsMenu, Select } from "../ui";

export type Act = (f: () => Promise<app.PersistentAgentView>, what?: string) => Promise<boolean>;

/** The grids. withAdds draws the Add cards (the Configuration);
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
  const [panel, setPanel] = useState<"google" | "microsoft" | "fastmail" | null>(null);
  const setUp = (s: SetUp, grid: GridKey) => {
    const form = setUpForm(s, grid);
    if (!form) {
      setPanel(s.kind === "microsoft" || s.kind === "fastmail" ? s.kind : "google");
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
            {g.entries.map((e) => e.kind === "meetings" ? (
              <MeetingNotesCard key={e.id} agent={agent} entry={e} busy={busy} act={act} onError={onError} />
            ) : (
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
              {adding.kind === "provider" && <AddProvider agent={agent} busy={busy} act={act} onDone={() => setAdding(null)} />}
            </div>
          )}
          {panel === "google" && g.key === "mail" && agent.google && (
            <div className="config-add-panel">
              <GoogleSetUp agent={agent} busy={busy} act={act} onError={onError} onChanged={onChanged} onDone={() => setPanel(null)} />
            </div>
          )}
          {panel === "microsoft" && g.key === "mail" && agent.microsoft && (
            <div className="config-add-panel">
              <MicrosoftSetUp agent={agent} busy={busy} act={act} onError={onError} onChanged={onChanged} onDone={() => setPanel(null)} />
            </div>
          )}
          {panel === "fastmail" && g.key === "mail" && agent.fastmail && (
            <div className="config-add-panel">
              <FastmailSetUp agent={agent} busy={busy} act={act} onDone={() => setPanel(null)} />
            </div>
          )}
        </section>
      ))}
    </div>
  );
}

const ADD_TITLES: Record<AddKind, string> = { server: "Add a server", skill: "Add a skill", secret: "Add a secret", account: "Add an account", provider: "Add a provider" };
const ADD_LINES: Record<AddKind, string> = {
  provider: "A service whose mail and calendar it reaches",
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
  const [disabling, setDisabling] = useState(false);
  const state = cardState(agent, e);
  const switches = cardSwitches(e);
  const menu = cardMenu(e, agent);
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
    ...(menu.disable ? [{ key: "disable", label: `Disable ${menu.disable.name}`, onSelect: () => setDisabling(true), disabled: busy }] : []),
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
      {disabling && menu.disable && <DisableProvider agent={agent} provider={menu.disable} busy={busy} act={act} onCancel={() => setDisabling(false)} />}
    </div>
  );
}

/** Disable a mail provider, confirmed on its card: its cards leave the
 *  Configuration and its servers the agent's next start; its sign-in stays. */
function DisableProvider({ agent, provider, busy, act, onCancel }: { agent: app.PersistentAgentView; provider: app.PersistentMailProviderView; busy: boolean; act: Act; onCancel: () => void }) {
  const running = runsSomewhere(agent);
  return (
    <div className="config-card-confirm small" role="group" aria-label={`Disable ${provider.name}`}>
      <span>
        {running ? `Stop ${agentCalled(agent)} first: its mail servers hold their sign-ins while it runs.`
          : `Disable ${provider.name}? Its cards leave Mail and calendar and ${agentCalled(agent)} no longer reaches it; its sign-in is kept.`}
      </span>
      <Button className="small" disabled={busy || running} onClick={() => void act(() => PersistentAPI.SetMailProvider(agent.name, provider.key, false), `${provider.name} disabled`).then((ok) => ok && onCancel())}>Disable</Button>
      <Button variant="ghost" className="small" onClick={onCancel}>Cancel</Button>
    </div>
  );
}

/** runsSomewhere says the agent runs or starts, here or in another client:
 *  its mail providers change only while it is stopped. */
function runsSomewhere(agent: Pick<app.PersistentAgentView, "state">): boolean {
  return agent.state === AGENT_STATES.running || agent.state === AGENT_STATES.starting || agent.state === AGENT_STATES.elsewhere;
}

/** Add a provider: each disabled mail provider, enabled with one click;
 *  its cards then join Mail and calendar. */
function AddProvider({ agent, busy, act, onDone }: { agent: app.PersistentAgentView; busy: boolean; act: Act; onDone: () => void }) {
  const running = runsSomewhere(agent);
  return (
    <div className="stack">
      {running && <Notice>Stop {agentCalled(agent)} first: its mail servers hold their sign-ins while it runs.</Notice>}
      {disabledProviders(agent).map((p) => (
        <div key={p.key} className="row">
          <Button disabled={busy || running} onClick={() => void act(() => PersistentAPI.SetMailProvider(agent.name, p.key, true), `${p.name} added`).then((ok) => ok && onDone())}>
            Add {p.name}
          </Button>
          <span className="small muted">{providerWords(p)}</span>
        </div>
      ))}
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
  const [server, setServer] = useState<ServerForm>({ source: "npm", package: "", version: "", args: [], url: "", tools: "" });
  const setArgs = (args: string[]) => setServer({ ...server, args });
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
            <Field label="Arguments" hint="The words its command starts with, one per field, in order, for a server that is a subcommand of a larger program (such as mcp).">
              <div className="row" style={{ flexWrap: "wrap" }}>
                {server.args.map((a, k) => (
                  <span className="row" key={k} style={{ gap: 2 }}>
                    <Input className="mono" aria-label={`argument ${k + 1}`} placeholder="word" style={{ width: 140 }} value={a} spellCheck={false}
                      onChange={(e) => setArgs(server.args.map((x, j) => (j === k ? e.target.value : x)))} />
                    <Button type="button" variant="ghost" className="small" aria-label={`remove argument ${k + 1}`} onClick={() => setArgs(server.args.filter((_, j) => j !== k))}>Remove</Button>
                  </span>
                ))}
                <Button type="button" className="small" onClick={() => setArgs([...server.args, ""])}>Add an argument</Button>
              </div>
            </Field>
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

/** Microsoft 365's set-up: the person's own Entra app, its id typed here and
 *  checked as an id before Go takes it, the tenant, then the sign-in by a
 *  device code, whose code and Microsoft's page are shown here while Go
 *  waits; the browser opens only that page. It reads signed in only once the
 *  server lists the account. The signoff is a line mail it sends or drafts
 *  ends with. None of it runs while the agent runs. */
function MicrosoftSetUp({ agent, busy, act, onError, onChanged, onDone }: {
  agent: app.PersistentAgentView; busy: boolean; act: Act;
  onError: (e: string | null) => void; onChanged: (v: app.PersistentAgentView) => void; onDone: () => void;
}) {
  const m = agent.microsoft;
  const [clientID, setClientID] = useState(m?.clientID ?? "");
  const [tenant, setTenant] = useState(m?.tenant && m.tenant !== "consumers" && m.tenant !== "organizations" ? "org" : (m?.tenant ?? ""));
  const [tenantID, setTenantID] = useState(m?.tenant && ENTRA_ID.test(m.tenant) ? m.tenant : "");
  const [email, setEmail] = useState(m?.account ?? "");
  const [signoff, setSignoff] = useState(m?.signoff ?? "");
  const [started, setStarted] = useState(false);
  const [code, setCode] = useState<{ url: string; code: string } | null>(null);
  const waiting = started || !!m?.waiting;
  const running = agent.state === AGENT_STATES.running || agent.state === AGENT_STATES.starting || agent.state === AGENT_STATES.elsewhere;
  const clientOK = ENTRA_ID.test(clientID.trim());
  const tenantValue = tenant === "org" ? tenantID.trim() : tenant;
  const tenantOK = tenant !== "org" || ENTRA_ID.test(tenantID.trim());
  const emailOK = /^[^@\s-][^@\s]*@[^@\s]+\.[a-z]{2,}$/i.test(email.trim());
  // Go tells the page a waiting sign-in's code and the page to enter it at,
  // Microsoft's own.
  useEffect(() => EventsOn("persistent:microsoft-sign-in", (v: { agent?: string; url?: string; code?: string }) => {
    if (v?.agent === agent.name && typeof v.url === "string" && typeof v.code === "string") setCode({ url: v.url, code: v.code });
  }), [agent.name]);
  const signIn = async () => {
    setStarted(true);
    setCode(null);
    onError(null);
    try {
      onChanged(await PersistentAPI.MicrosoftSignIn(agent.name, email.trim()));
    } catch (e) {
      onError(String(e));
    } finally {
      setStarted(false);
      setCode(null);
    }
  };
  return (
    <div className="config-google-form">
      <p className="small">{microsoftWords(m)}</p>
      {running && <Notice>Stop {agentCalled(agent)} first: its Microsoft 365 server holds what the set-up writes.</Notice>}
      {m?.agentryAppOffered && !m.clientID ? (
        <p className="small muted">
          Microsoft 365 can sign in with Agentry&apos;s own Microsoft app, so you need no app of your own: enter your account
          below and sign in. To use an Entra app of your own instead, keep its id here; it then comes first.
        </p>
      ) : (
        <p className="small muted">
          Microsoft 365 signs in with an Entra app of your own, registered as a public client with Allow public client flows on. The
          microsoft-365-setup skill walks you through it: ask {agentCalled(agent)} to help you set up Microsoft 365. Only the
          app&apos;s id is kept; there is no secret.
        </p>
      )}
      <Field label="Application (client) ID" hint="From your app's Overview in Microsoft Entra.">
        <Input value={clientID} autoComplete="off" spellCheck={false} disabled={waiting} onChange={(e) => setClientID(e.target.value)} />
      </Field>
      <Field label="Account kind" hint="Which tenant the sign-in uses.">
        <Select value={tenant} disabled={waiting} onChange={(e) => setTenant(e.target.value)}>
          {MICROSOFT_TENANTS.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
          <option value="org">One organization (its tenant id)</option>
        </Select>
      </Field>
      {microsoftAdminWords(tenant, agentCalled(agent)) && (
        <Notice>{microsoftAdminWords(tenant, agentCalled(agent))}</Notice>
      )}
      {tenant === "org" && (
        <Field label="Directory (tenant) ID" hint="From your app's Overview, for an app limited to one organization.">
          <Input value={tenantID} autoComplete="off" spellCheck={false} disabled={waiting} onChange={(e) => setTenantID(e.target.value)} />
        </Field>
      )}
      <div className="row gap">
        <Button disabled={busy || waiting || running || !clientOK || !tenantOK}
          onClick={() => void act(() => PersistentAPI.SetMicrosoftClient(agent.name, clientID.trim(), tenantValue), "Microsoft 365's app")}>
          {m?.clientID ? "Keep this app" : "Keep the app"}
        </Button>
      </div>
      <Field label="Your Microsoft account" hint="The address you sign in to Microsoft with.">
        <Input value={email} autoComplete="off" spellCheck={false} disabled={!microsoftCanSignIn(m) || waiting} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <p className="small muted">{microsoftScopeWords(agent.roster)}</p>
      <div className="row gap">
        <Button variant="primary" disabled={busy || waiting || running || !microsoftCanSignIn(m) || !emailOK} onClick={() => void signIn()}>
          {m?.account ? "Sign in again" : "Sign in with Microsoft"}
        </Button>
        {waiting && <Button onClick={() => void PersistentAPI.CancelMicrosoftSignIn(agent.name)}>Cancel</Button>}
        <Button variant="ghost" disabled={started} onClick={onDone}>Close</Button>
      </div>
      {waiting && (
        <div className="small" role="status">
          {code ? (
            <>
              <p>Enter this code at Microsoft&apos;s sign-in page, then sign in and allow the app. This carries on when Microsoft answers.</p>
              <p className="mono selectable config-google-address">{code.code}</p>
              <p>If your browser did not open, go to: <span className="mono selectable config-google-address">{code.url}</span></p>
            </>
          ) : (
            <p>Asking Microsoft for a sign-in code…</p>
          )}
        </div>
      )}
      <Field label="Signoff (optional)" hint="A line every mail it sends or drafts ends with, such as “Sent by my assistant”. Empty for none.">
        <Input value={signoff} maxLength={200} disabled={waiting} onChange={(e) => setSignoff(e.target.value)} />
      </Field>
      <div className="row gap">
        <Button disabled={busy || waiting || running || signoff.trim() === (m?.signoff ?? "")}
          onClick={() => void act(() => PersistentAPI.SetMicrosoftSignoff(agent.name, signoff.trim()), "the signoff")}>
          Save the signoff
        </Button>
      </div>
    </div>
  );
}

/** Fastmail's set-up: the person's own Fastmail API token, typed here and
 *  handed to Go, which asks Fastmail whether it accepts it before keeping
 *  it in the system keychain. The token never comes back to the page. */
function FastmailSetUp({ agent, busy, act, onDone }: {
  agent: app.PersistentAgentView; busy: boolean; act: Act; onDone: () => void;
}) {
  const f = agent.fastmail;
  const [token, setToken] = useState("");
  const [removing, setRemoving] = useState(false);
  const running = agent.state === AGENT_STATES.running || agent.state === AGENT_STATES.starting || agent.state === AGENT_STATES.elsewhere;
  const keep = async () => {
    if (await act(() => PersistentAPI.SetFastmailToken(agent.name, token.trim()), "the Fastmail token")) setToken("");
  };
  return (
    <div className="config-google-form">
      <p className="small">{fastmailWords(f)}</p>
      {running && <Notice>Stop {agentCalled(agent)} first.</Notice>}
      <p className="small muted">
        Fastmail&apos;s own server reaches your mail, contacts and calendars with an API token you make in Fastmail, under
        Settings, Privacy &amp; Security: choose the MCP type, and Read only unless you want {agentCalled(agent)} to change or
        send things, since it can do whatever the token allows. Fastmail&apos;s Basic plan has no API tokens. The fastmail-setup
        skill walks you through it: ask {agentCalled(agent)} to help you set up Fastmail. Never paste the token into the chat.
      </p>
      <Field label={f?.token ? "A new Fastmail API token" : "Your Fastmail API token"} hint="Fastmail is asked whether it accepts it before it is kept.">
        <Input type="password" value={token} autoComplete="off" spellCheck={false} onChange={(e) => setToken(e.target.value)} />
      </Field>
      <div className="row gap">
        <Button variant="primary" disabled={busy || running || !secretValueOK(token.trim())} onClick={() => void keep()}>
          {f?.token ? "Replace the token" : "Keep the token"}
        </Button>
        {f?.token && !removing && <Button disabled={busy || running} onClick={() => setRemoving(true)}>Remove the token</Button>}
        <Button variant="ghost" onClick={onDone}>Close</Button>
      </div>
      {removing && (
        <div className="row gap" role="group" aria-label="Remove the Fastmail token">
          <p className="small">This removes the token from this computer. It stays valid at Fastmail until you delete it there.</p>
          <Button variant="primary" disabled={busy} onClick={() => void act(() => PersistentAPI.RemoveFastmailToken(agent.name), "the Fastmail token's removal").then(() => setRemoving(false))}>Remove</Button>
          <Button variant="ghost" onClick={() => setRemoving(false)}>Keep it</Button>
        </div>
      )}
    </div>
  );
}
