import { useState } from "react";
import * as RunAPI from "../../../wailsjs/go/app/RunAPI";
import { app } from "../../../wailsjs/go/models";
import { CardFrame } from "../../cards";
import { cardStatus } from "../../deliverables";
import { nameOf, whenRan, whereRan } from "../../recentRuns";
import { ErrorBox } from "../../ui";
import { DeliverableItem } from "../Deliverables";

// Home's Deliverables card: the most recent files runs were for, across the
// runs this computer remembers (the history Go keeps, each run's envelope),
// newest first, with the actions a run's result gives them. It reads the
// history and the envelopes, never the Deliverables folder, so a file
// someone put there is not listed. Open Deliverables folder shows the folder
// itself; the card opens Run defaults, where the folder is chosen and the
// switch lives.
export function DeliverablesCard({ view, error, now, onOpenSettings }: {
  view: app.RecentDeliverablesView | null;
  error: string | null;
  now: Date;
  onOpenSettings: () => void;
}) {
  const files = view?.files ?? [];
  const [folderErr, setFolderErr] = useState<string | null>(null);
  const status = cardStatus({
    on: view?.on ?? true, remember: view?.remember ?? true, count: files.length, error, reading: !view && !error,
  });
  return (
    <CardFrame title="Deliverables" onOpen={onOpenSettings} className="home-card" status={status}>
      {files.length > 0 && (
        <ul className="home-card-list deliverables-list" data-deliverables>
          {files.map((f) => (
            <li key={`${f.runId}-${f.path}`} data-deliverable-run={f.runId}>
              <DeliverableItem runId={f.runId} file={f} on={view?.on ?? true} compact
                context={[nameOf(f.ref), whereRan(f), whenRan(f, now)].filter(Boolean).join(" · ")} />
            </li>
          ))}
        </ul>
      )}
      {view?.on && (
        <p className="small">
          <button type="button" className="link build-way-part" data-open-deliverables-folder
            onClick={() => { setFolderErr(null); RunAPI.OpenDeliverablesFolder().catch((e) => setFolderErr(String(e))); }}>
            Open Deliverables folder
          </button>
        </p>
      )}
      <ErrorBox error={folderErr} />
      {view?.error && <p className="small">{view.error}</p>}
    </CardFrame>
  );
}
