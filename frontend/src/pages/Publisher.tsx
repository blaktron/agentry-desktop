import { useCallback, useEffect, useRef, useState } from "react";
import * as PublisherAPI from "../../wailsjs/go/main/PublisherAPI";
import * as BrowseAPI from "../../wailsjs/go/main/BrowseAPI";
import { forkProgressLine } from "../fork";
import { main } from "../../wailsjs/go/models";
import {
  AgentRow, Button, EmptyState, ErrorBox, Field, Input, Modal, Notice, PageHeader, Select, Spinner, Toolbar, fmtDate,
} from "../ui";
import {
  FORK_STAYS_PRIVATE, LIST_SORTS, SIGN_IN_AGAIN, countsLine, deleteNameMatches, deleteWords, forkOfLabel, listCounts, notAdminLine,
  rowActions, rowBadges, visibilityLabel, visibleIn,
  type ForksFilter, type ListSort, type StateFilter, type VisibilityFilter,
} from "../publisher";

// The Publisher section (plan forks-and-publisher.md §4.2, D12; M6,
// agentry-desktop#705): the QuickAgents in the signed-in person's namespace
// and their organizations', searched, filtered and sorted as the site's
// Publisher list is, with the owner actions on each. Every action is asked
// again in a native dialog by Go and decided by the registry; the page says
// what happened in the words Go returns.
// A fork just made from an agent page (agentry-desktop#706): the section
// opens on its namespace and follows it until its first version is
// published or stops.
export type FollowFork = { namespace: string; name: string; sentence: string };

// followEveryMs is how often a followed fork is read again.
const followEveryMs = 4000;

export default function Publisher({ registry, onOpenAgent, onEdit, onOpenSettings, follow }: {
  registry: string;
  /** A fork to open on and follow. */
  follow?: FollowFork;
  onOpenAgent: (ns: string, name: string) => void;
  /** Edit opens the release in the Build tab (edit.ts). */
  onEdit: (ns: string, name: string) => void;
  /** Settings' registry part, where a sign-in is made or made again. */
  onOpenSettings: () => void;
}) {
  const [view, setView] = useState<main.PublisherView | null>(null);
  const [ns, setNs] = useState("");
  const [rows, setRows] = useState<main.PublisherAgent[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(follow?.sentence || null);
  // The followed fork's progress, in words, and whether it is over.
  const [progress, setProgress] = useState<{ line: string; done: boolean } | null>(null);
  const [q, setQ] = useState("");
  const [visibility, setVisibility] = useState<VisibilityFilter>("");
  const [state, setState] = useState<StateFilter>("");
  const [forks, setForks] = useState<ForksFilter>("");
  const [sort, setSort] = useState<ListSort>("newest");
  // The action in flight, "ns/name:which", so its own button shows it and
  // every other action waits.
  const [busy, setBusy] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<main.PublisherAgent | null>(null);
  const [typed, setTyped] = useState("");

  useEffect(() => {
    setView(null);
    setRows(null);
    setError(null);
    PublisherAPI.Overview(registry)
      .then((v) => {
        setView(v);
        setNs((cur) => {
          if (follow && v.namespaces.some((n) => n.handle === follow.namespace)) return follow.namespace;
          return v.namespaces.some((n) => n.handle === cur) ? cur : v.namespaces[0]?.handle ?? "";
        });
      })
      .catch((e) => setError(String(e)));
  }, [registry]);

  const load = useCallback(async () => {
    if (!ns) return;
    setError(null);
    try {
      setRows(await PublisherAPI.List(registry, ns, new main.PublisherQuery({ q, state, forks, sort })));
    } catch (e) {
      setRows([]);
      setError(String(e));
    }
  }, [registry, ns, q, state, forks, sort]);

  // A namespace, a filter or the sort reads the list again; typing waits
  // for Enter or the button, as Browse does.
  useEffect(() => {
    setRows(null);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [registry, ns, state, forks, sort]);

  // The newest load, for the follow's timer, which outlives the render
  // that started it (the namespace is set after the first one).
  const loadRef = useRef(load);
  loadRef.current = load;

  // Follow a new fork: its copy, then its first version through the
  // pipeline and any review. The list is read again whenever the line
  // changes, so the row shows what the line says. Three reads in a row
  // that do not answer (signed out, the fork deleted, the registry down)
  // end it, saying so; leaving the page ends it too.
  useEffect(() => {
    if (!follow) return;
    let stop = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let last = "";
    let misses = 0;
    const ref = `${follow.namespace}/${follow.name}`;
    const again = () => { if (!stop) timer = setTimeout(tick, followEveryMs); };
    const tick = async () => {
      try {
        const d = await BrowseAPI.Agent(registry, follow.namespace, follow.name);
        if (stop) return;
        misses = 0;
        const p = forkProgressLine(ref, { forkState: d.agent.forkState, forkDetail: d.agent.forkDetail, versions: d.versions });
        setProgress(p);
        if (p.line !== last) {
          last = p.line;
          void loadRef.current();
        }
        if (!p.done) again();
      } catch {
        if (stop) return;
        if (++misses < 3) return again();
        setProgress({ line: `${ref} could not be read, so it is no longer followed here. Its row below shows where it stands.`, done: true });
      }
    };
    void tick();
    return () => {
      stop = true;
      if (timer) clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [registry, follow?.namespace, follow?.name]);

  async function act(row: main.PublisherAgent, which: "visibility" | "retire" | "delete", run: () => Promise<main.PublisherDone>) {
    setBusy(`${row.namespace}/${row.name}:${which}`);
    setError(null);
    setNotice(null);
    try {
      const done = await run();
      if (done.cancelled) return;
      setNotice(done.sentence ?? null);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(null);
    }
  }

  async function confirmDelete() {
    const row = deleting;
    if (!row) return;
    setDeleting(null);
    await act(row, "delete", () => PublisherAPI.Delete(registry, row.namespace, row.name, typed));
    setTyped("");
  }

  if (error && !view) {
    return (
      <div>
        <PageHeader title="Publisher" />
        <ErrorBox error={error} />
      </div>
    );
  }
  if (!view) return <div><PageHeader title="Publisher" /><Spinner /></div>;
  if (!view.signedIn) {
    return (
      <div>
        <PageHeader title="Publisher" />
        <EmptyState title="Sign in to manage what you publish" action={<Button variant="primary" onClick={onOpenSettings}>Open Settings</Button>}>
          The Publisher section lists the QuickAgents in your namespace and your organizations', with Make private, Retire, Restore and Delete.
        </EmptyState>
      </div>
    );
  }

  const here = view.namespaces.find((n) => n.handle === ns);
  const admin = !!here?.admin;
  // A sign-in from before agentry#993 cannot take the owner actions; one
  // whose scopes the registry did not report is offered them, and the
  // registry decides.
  const scopeless = view.scopesKnown && !view.canManage;
  const owner = admin && !scopeless;
  const shown = rows ? visibleIn(rows, visibility) : null;
  const filtered = q.trim() !== "" || visibility !== "" || state !== "" || forks !== "";

  return (
    <div>
      <PageHeader
        title="Publisher"
        sub={<>The QuickAgents you publish, in your namespace and your organizations'. Make one private, retire it, restore it or delete it here, as on the site's Publisher page.</>}
      />
      {scopeless && (
        <Notice tone="warn">
          {SIGN_IN_AGAIN}{" "}
          <Button variant="ghost" onClick={onOpenSettings}>Open Settings</Button>
        </Notice>
      )}

      <Toolbar>
        {view.namespaces.length > 1 && (
          <Select value={ns} onChange={(e) => setNs(e.target.value)} title="Namespace" aria-label="Namespace">
            {view.namespaces.map((n) => (
              <option key={n.handle} value={n.handle}>{n.kind === "user" ? `${n.handle} (you)` : n.handle}</option>
            ))}
          </Select>
        )}
        <Input
          type="search"
          className="grow"
          placeholder={`Search ${ns}'s QuickAgents…`}
          aria-label={`Search ${ns}'s QuickAgents`}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && load()}
        />
        <Button variant="primary" onClick={() => load()}>Search</Button>
      </Toolbar>
      {/* The filters and the sort on a row of their own, so the search
          keeps its width. */}
      <Toolbar>
        <Select value={visibility} onChange={(e) => setVisibility(e.target.value as VisibilityFilter)} title="Visibility" aria-label="Visibility">
          <option value="">any visibility</option>
          <option value="public">public</option>
          <option value="private">private</option>
        </Select>
        <Select value={state} onChange={(e) => setState(e.target.value as StateFilter)} title="State" aria-label="State">
          <option value="">any state</option>
          <option value="active">active</option>
          <option value="retired">retired</option>
        </Select>
        <Select value={forks} onChange={(e) => setForks(e.target.value as ForksFilter)} title="Forks" aria-label="Forks">
          <option value="">forks or not</option>
          <option value="only">only forks</option>
          <option value="none">no forks</option>
        </Select>
        <Select value={sort} onChange={(e) => setSort(e.target.value as ListSort)} title="Sort" aria-label="Sort">
          {LIST_SORTS.map((s) => <option key={s.id} value={s.id}>{s.label}</option>)}
        </Select>
      </Toolbar>

      <ErrorBox error={error} />
      {notice && <Notice tone="ok">{notice}</Notice>}
      {progress && <Notice tone={progress.done ? "ok" : "info"}><span data-fork-progress>{progress.line}</span></Notice>}
      {!admin && here && <p className="muted small" data-not-admin>{notAdminLine(ns)}</p>}

      {!shown && <Spinner />}
      {shown && rows && (
        <>
          <p className="muted small" data-counts>{countsLine(listCounts(rows))}</p>
          {shown.length === 0 && (
            filtered
              ? <EmptyState title="Nothing matched">Clear the search or a filter to see more.</EmptyState>
              : <EmptyState title={`${ns} has no QuickAgents yet`}>Build one in the Build tab, then submit it.</EmptyState>
          )}
          {shown.map((a) => {
            const ref = `${a.namespace}/${a.name}`;
            const acts = rowActions(a);
            const forkOf = forkOfLabel(a);
            const meta = [
              forkOf,
              a.installs30d ? `${a.installs30d} install${a.installs30d === 1 ? "" : "s"} in 30 days` : null,
              a.lastSubmittedAt ? `last submitted ${fmtDate(a.lastSubmittedAt)}` : null,
              a.retiredAt ? `retired ${fmtDate(a.retiredAt)}` : null,
            ].filter(Boolean).join(" · ");
            const inFlight = (which: string) => busy === `${ref}:${which}`;
            return (
              <AgentRow
                key={ref}
                title={ref}
                badges={rowBadges(a).map((b) => <span key={b.label} className={`badge ${b.tone}`}>{b.label}</span>)}
                summary={a.summary}
                meta={meta || undefined}
                onClick={() => onOpenAgent(a.namespace, a.name)}
                actions={<>
                  {acts.edit && <Button onClick={() => onEdit(a.namespace, a.name)} disabled={!!busy}>Edit</Button>}
                  {owner && acts.visibility && (
                    <Button
                      onClick={() => act(a, "visibility", () => PublisherAPI.SetVisibility(registry, a.namespace, a.name, acts.visibility!))}
                      disabled={!!busy} busy={inFlight("visibility")} busyLabel="Asking…"
                    >
                      {visibilityLabel(acts.visibility)}
                    </Button>
                  )}
                  {owner && acts.forkStaysPrivate && !acts.visibility && a.visibility === "private" && (
                    <span className="muted small" title={FORK_STAYS_PRIVATE} data-fork-private>stays private</span>
                  )}
                  {owner && (
                    <Button
                      onClick={() => act(a, "retire", () => acts.retire === "retire"
                        ? PublisherAPI.Retire(registry, a.namespace, a.name)
                        : PublisherAPI.Restore(registry, a.namespace, a.name))}
                      disabled={!!busy} busy={inFlight("retire")} busyLabel="Asking…"
                    >
                      {acts.retire === "retire" ? "Retire" : "Restore"}
                    </Button>
                  )}
                  {owner && <Button variant="danger" onClick={() => { setTyped(""); setDeleting(a); }} disabled={!!busy} busy={inFlight("delete")} busyLabel="Asking…">Delete</Button>}
                </>}
              />
            );
          })}
          {shown.some((a) => rowActions(a).forkStaysPrivate) && owner && (
            <p className="muted small">{FORK_STAYS_PRIVATE}</p>
          )}
        </>
      )}

      {deleting && (
        <Modal title={`Delete ${deleting.namespace}/${deleting.name}?`} onClose={() => setDeleting(null)}>
          {deleteWords(deleting.namespace, deleting.name, !!deleting.lastSubmittedAt).map((p) => <p key={p}>{p}</p>)}
          <Field label={<>Type <span className="mono">{deleting.name}</span> to confirm</>} htmlFor="publisher-delete-name">
            <Input id="publisher-delete-name" autoFocus value={typed} onChange={(e) => setTyped(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && deleteNameMatches(typed, deleting.name) && confirmDelete()} />
          </Field>
          <div className="row" style={{ justifyContent: "flex-end" }}>
            <Button onClick={() => setDeleting(null)}>Cancel</Button>
            <Button variant="danger" onClick={confirmDelete} disabled={!deleteNameMatches(typed, deleting.name)}>Delete</Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
