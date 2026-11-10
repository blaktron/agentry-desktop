import { useCallback, useEffect, useState } from "react";
import * as BrowseAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/browseapi";
import * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import {
  AgentRow, Button, EmptyState, ErrorBox, Input, ModeBadges, PageHeader,
  QuarantineBadge, Select, Spinner, Toolbar,
} from "../ui";

const MODES = ["", "autonomous", "interactive", "persistent"];
const CAPABILITIES = ["", "network", "filesystem", "exec", "secrets", "dataAccess"];
const SORTS = ["relevance", "installs", "newest", "updated"];
/** Results asked for at a time; Show more asks for the next page. */
export const BROWSE_PAGE = 30;

/** Whether the registry has results past those shown: by its total when it
 *  sends one, else by a full last page. */
export function hasMore(view: app.SearchView | null, lastPage: number): boolean {
  if (!view) return false;
  if (typeof view.total === "number") return view.results.length < view.total;
  return lastPage === BROWSE_PAGE;
}

export default function Browse({
  registry,
  initialQuery = "",
  onOpenAgent,
}: {
  registry: string;
  /** What the Library's search held when it led here: searched at once. */
  initialQuery?: string;
  onOpenAgent: (ns: string, name: string) => void;
}) {
  const [q, setQ] = useState(initialQuery);
  const [mode, setMode] = useState("");
  const [capability, setCapability] = useState("");
  const [sort, setSort] = useState("relevance");
  const [results, setResults] = useState<app.SearchView | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [lastPage, setLastPage] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const search = useCallback(async () => {
    if (!registry) {
      setError("No registry is configured. Add one in Settings.");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const query = new app.SearchQuery({ q, mode, capability, sort, limit: BROWSE_PAGE, offset: 0 });
      const view = await BrowseAPI.Search(registry, query);
      setResults(view);
      setLastPage(view?.results.length ?? 0);
    } catch (e) {
      setResults(null);
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [registry, q, mode, capability, sort]);

  // The next page of the same search, appended. A result already shown (the
  // registry's order moved between pages) is not shown twice.
  const more = useCallback(async () => {
    if (!results) return;
    setLoadingMore(true);
    setError(null);
    try {
      const query = new app.SearchQuery({ q, mode, capability, sort, limit: BROWSE_PAGE, offset: results.results.length });
      const page = await BrowseAPI.Search(registry, query);
      const seen = new Set(results.results.map((h) => `${h.namespace}/${h.name}`));
      const fresh = (page?.results ?? []).filter((h) => !seen.has(`${h.namespace}/${h.name}`));
      setResults(new app.SearchView({ ...results, total: page?.total ?? results.total, results: [...results.results, ...fresh] }));
      setLastPage(page?.results.length ?? 0);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoadingMore(false);
    }
  }, [registry, q, mode, capability, sort, results]);

  // First render and registry/facet changes trigger a search; typing wants
  // the button or Enter.
  useEffect(() => {
    if (registry) void search();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [registry, mode, capability, sort]);

  return (
    <div>
      <PageHeader
        title="Browse"
        sub={<span className="selectable">QuickAgents on {registry || "(no registry)"}. A published version lists what its scan found, in a public report, and carries its signature; an unscanned or unsigned release installs only if you accept it.</span>}
      />

      <Toolbar>
        <Input
          type="search"
          className="grow"
          placeholder="Search agents…"
          aria-label="Search agents"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && search()}
        />
        <Select value={mode} onChange={(e) => setMode(e.target.value)} title="Run mode" aria-label="Run mode">
          {MODES.map((k) => <option key={k} value={k}>{k === "" ? "any mode" : k}</option>)}
        </Select>
        <Select value={capability} onChange={(e) => setCapability(e.target.value)} title="Declared capability" aria-label="Declared capability">
          {CAPABILITIES.map((c) => <option key={c} value={c}>{c === "" ? "any capability" : `asks for ${c}`}</option>)}
        </Select>
        <Select value={sort} onChange={(e) => setSort(e.target.value)} title="Sort" aria-label="Sort">
          {SORTS.map((s) => <option key={s} value={s}>{s}</option>)}
        </Select>
        <Button variant="primary" onClick={search}>Search</Button>
      </Toolbar>

      <ErrorBox error={error} />
      {loading && <Spinner label="Searching…" />}

      {!loading && results && (
        <div>
          {/* The count is the registry's or nothing: a core that sent no
              total gets a list without a number, not
              "0 results" above a full list. */}
          {typeof results.total === "number" && (
            <p className="muted small">
              {results.results.length < results.total
                ? `Showing ${results.results.length} of ${results.total} results`
                : `${results.total} result${results.total === 1 ? "" : "s"}`}
            </p>
          )}
          {results.results.length === 0 && (
            <EmptyState title="Nothing matched">Try a broader query, or clear the mode and capability filters.</EmptyState>
          )}
          {results.results.map((hit) => (
            <AgentRow
              key={`${hit.namespace}/${hit.name}`}
              title={`${hit.namespace}/${hit.name}`}
              badges={<><QuarantineBadge state={hit.quarantine} /><ModeBadges modes={hit.modes} /></>}
              summary={hit.summary}
              meta={hit.installs30d > 0 ? `${hit.installs30d} installs in 30 days` : undefined}
              onClick={() => onOpenAgent(hit.namespace, hit.name)}
            />
          ))}
          {hasMore(results, lastPage) && (
            <div className="browse-more">
              {loadingMore
                ? <Spinner label="Loading more…" />
                : <Button onClick={more}>Show more</Button>}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
