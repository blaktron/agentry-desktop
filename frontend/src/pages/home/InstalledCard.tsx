import { install } from "../../../wailsjs/go/models";
import { CardFrame } from "../../cards";
import { installedLine, recentInstalls } from "../../homeLines";

// Home's Installed QuickAgents card (agentry-desktop#658): how many
// QuickAgents are on this computer and how many have an update (the launch
// check's snapshot, never a new check), and the newest five. The card
// opens the Library; a name opens its page, or the Library for one
// installed from a folder, which has no page.
export function InstalledCard({ records, updates, error, onOpenLibrary, onOpenBrowse, onOpenAgent }: {
  records: install.Item[] | null;
  updates: number;
  error: string | null;
  onOpenLibrary: () => void;
  onOpenBrowse: () => void;
  onOpenAgent: (registry: string, ns: string, name: string) => void;
}) {
  const onComputer = (records ?? []).filter((r) => r.present);
  const recent = recentInstalls(
    onComputer.map((r) => ({ ...r, installedAt: String(r.installedAt ?? "") })),
    5,
  );
  return (
    <CardFrame
      title="Installed QuickAgents"
      onOpen={onOpenLibrary}
      className="home-card"
      status={error ? "The Library could not be read." : records ? installedLine(onComputer.length, updates) : "Reading the Library…"}
    >
      {records && recent.length > 0 && (
        <ul className="home-card-list">
          {recent.map((r) => (
            <li key={`${r.namespace}/${r.name}`}>
              <button type="button" className="link build-way-part mono" onClick={() => (r.local ? onOpenLibrary() : onOpenAgent(r.origin, r.namespace, r.name))}>
                {r.namespace}/{r.name}
              </button>
              <span className="muted small"> v{r.version}</span>
            </li>
          ))}
        </ul>
      )}
      {records && onComputer.length === 0 && (
        <button type="button" className="link build-way-part" onClick={onOpenBrowse}>Find more QuickAgents</button>
      )}
    </CardFrame>
  );
}
