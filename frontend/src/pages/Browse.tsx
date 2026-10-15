import { useCallback, useEffect, useState } from "react";
import * as BrowseAPI from "../../wailsjs/go/app/BrowseAPI";
import { app } from "../../wailsjs/go/models";
import {
  AgentRow, Button, EmptyState, ErrorBox, Input, ModeBadges, PageHeader,
  QuarantineBadge, Select, Spinner, Toolbar,
} from "../ui";

const MODES = ["", "autonomous", "interactive"];
const CAPABILITIES = ["", "network", "filesystem", "exec", "secrets", "dataAccess"];
const SORTS = ["relevance", "installs", "newest", "updated"];

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
  const [error, setError] = useState<string | null>(null);

  const search = useCallback(async () => {
    if (!registry) {
      setError("No registry is configured. Add one in Settings.");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const query = new app.SearchQuery({ q, mode, capability, sort, limit: 30, offset: 0 });
      setResults(await BrowseAPI.Search(registry, query));
    } catch (e) {
      setResults(null);
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [registry, q, mode, capability, sort]);

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
        sub={<span className="selectable">QuickAgents on {registry || "(no registry)"}. Every published version is scanned, and its report is public.</span>}
      />

      <Toolbar>
        <Input
          type="search"
          className="grow"
          placeholder="Search agents…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && search()}
        />
        <Select value={mode} onChange={(e) => setMode(e.target.value)} title="Run mode">
          {MODES.map((k) => <option key={k} value={k}>{k === "" ? "any mode" : k}</option>)}
        </Select>
        <Select value={capability} onChange={(e) => setCapability(e.target.value)} title="Declared capability">
          {CAPABILITIES.map((c) => <option key={c} value={c}>{c === "" ? "any capability" : `asks for ${c}`}</option>)}
        </Select>
        <Select value={sort} onChange={(e) => setSort(e.target.value)} title="Sort">
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
            <p className="muted small">{results.total} result{results.total === 1 ? "" : "s"}</p>
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
        </div>
      )}
    </div>
  );
}
