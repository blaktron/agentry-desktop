import * as knowledge from "../../../bindings/github.com/blaktron/agentry-desktop/internal/knowledge/models";
import { CardFrame, CouldNotRead } from "../../cards";

// How many packs the card lists.
const ROWS = 5;

// Home's Knowledge packs card: the packs on this computer and In Agentry,
// the newest first, with Create. The card opens the Knowledge page; Create
// opens the pack creator there.
export function KnowledgeCard({ list, error, onRetry, onOpen, onCreate }: {
  list: knowledge.List | null;
  error: string | null;
  onRetry?: () => void;
  onOpen: () => void;
  onCreate: () => void;
}) {
  const packs = [...(list?.packs ?? [])].sort((a, b) => String(b.updated ?? b.agentryUpdated ?? "").localeCompare(String(a.updated ?? a.agentryUpdated ?? "")));
  const status = error
    ? <CouldNotRead what="Your knowledge packs" error={error} onRetry={onRetry} />
    : !list
      ? "Reading your knowledge packs…"
      : packs.length === 0
        ? "No knowledge pack yet. A pack is a wiki made from your documents that a run can read."
        : packs.length === 1 ? "1 knowledge pack" : `${packs.length} knowledge packs`;
  return (
    <CardFrame title="Knowledge packs" onOpen={onOpen} className="home-card" status={status}>
      {packs.length > 0 && (
        <ul className="home-card-list">
          {packs.slice(0, ROWS).map((p) => (
            <li key={`${p.org ?? ""}/${p.name}/${p.agentryId ?? ""}`}>
              <span className="mono">{p.name}</span>
              <span className="muted small"> {p.local && p.inAgentry ? "here and In Agentry" : p.inAgentry ? "In Agentry" : "on this computer"}</span>
            </li>
          ))}
        </ul>
      )}
      {list && (
        <p className="home-card-actions">
          <button type="button" className="link build-way-part" onClick={onCreate}>Create a knowledge pack</button>
        </p>
      )}
    </CardFrame>
  );
}
