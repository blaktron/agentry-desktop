import { useEffect, useRef, useState } from "react";
import * as RunAPI from "../../../bindings/github.com/blaktron/agentry-desktop/internal/app/runapi";
import * as app from "../../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { CardFrame, CouldNotRead } from "../../cards";
import { agentOf, cardStatus, clickDoes, iconDate, iconKind, iconName } from "../../deliverables";
import { FileIcon } from "../../fileIcons";
import { RevealText, plainName } from "../../reveal";
import { ErrorBox } from "../../ui";

// Home's Deliverables card: the most recent files runs were for, across the
// runs this computer remembers (the history Go keeps, each run's envelope),
// newest first, one icon each with the QuickAgent and the day under it. It
// reads the history and the envelopes, never the Deliverables folder, so a
// file someone put there is not listed.
//
// A click asks Go to open the file (RunAPI.OpenRunDeliverable: the copy in
// the Deliverables folder, its bytes hashed again, handed to the system's
// own program for the type). A file Go does not open from here, an
// executable or a link or a macro document, has an icon too, and a click only
// shows it in its folder. A right-click, the menu key or Shift+F10 offers
// Show in folder for any icon. The file's name is the icon's tooltip and
// accessible name; nothing else of it is drawn. Open folder shows the
// folder itself; the card's title opens Run defaults, where the folder is
// chosen and the switch lives.
export function DeliverablesCard({ view, error, onRetry, now, onOpenSettings }: {
  view: app.RecentDeliverablesView | null;
  error: string | null;
  onRetry?: () => void;
  now: Date;
  onOpenSettings: () => void;
}) {
  const files = view?.files ?? [];
  const [failure, setFailure] = useState<string | null>(null);
  const status = error ? <CouldNotRead what="Deliverables" error={error} onRetry={onRetry} /> : cardStatus({
    on: view?.on ?? true, remember: view?.remember ?? true, count: files.length, error, reading: !view && !error,
  });
  const run = (f: () => Promise<void>) => { setFailure(null); f().catch((e) => setFailure(String(e))); };
  return (
    <CardFrame title="Deliverables" onOpen={onOpenSettings} className="home-card" status={status}>
      {files.length > 0 && (
        <ul className="deliverable-icons" data-deliverables>
          {files.map((f) => (
            <li key={`${f.runId}-${f.path}`} data-deliverable-run={f.runId}>
              <IconCell file={f} now={now} run={run} />
            </li>
          ))}
        </ul>
      )}
      {view?.on && (
        <p className="small">
          <button type="button" className="link build-way-part" data-open-deliverables-folder
            onClick={() => run(() => RunAPI.OpenDeliverablesFolder())}>
            Open folder
          </button>
        </p>
      )}
      <ErrorBox error={failure} />
      {view?.error && <p className="small">{view.error}</p>}
    </CardFrame>
  );
}

function IconCell({ file, now, run }: {
  file: app.RecentDeliverableView;
  now: Date;
  run: (f: () => Promise<void>) => void;
}) {
  const [menu, setMenu] = useState(false);
  const cell = useRef<HTMLDivElement>(null);
  const icon = useRef<HTMLButtonElement>(null);
  const name = plainName(file.name);
  const agent = agentOf(file.ref);
  // What a label says when it cannot hold a chip: invisible characters of the
  // agent's name become "_", as a file's name does.
  const agentLabel = plainName(agent);
  const date = iconDate(file.endedAt, now);
  const opens = clickDoes(file) === "open";
  useEffect(() => {
    if (!menu) return;
    const away = (e: MouseEvent) => { if (cell.current && !cell.current.contains(e.target as Node)) setMenu(false); };
    document.addEventListener("mousedown", away);
    cell.current?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus();
    return () => document.removeEventListener("mousedown", away);
  }, [menu]);
  const show = () => { setMenu(false); icon.current?.focus(); run(() => RunAPI.ShowRunDeliverable(file.runId, file.path)); };
  return (
    <div className={`deliverable-cell build-way-part${menu ? " is-menu-open" : ""}`} ref={cell}
      onContextMenu={(e) => { e.preventDefault(); setMenu(true); }}
      onKeyDown={(e) => { if (e.key === "Escape" && menu) { e.stopPropagation(); setMenu(false); icon.current?.focus(); } }}
      onBlur={(e) => { if (menu && !e.currentTarget.contains(e.relatedTarget as Node | null)) setMenu(false); }}>
      <button type="button" ref={icon} className="deliverable-icon" title={name} aria-label={iconName({ name, canOpen: opens }, agentLabel, date)}
        data-deliverable={file.path} data-deliverable-open={opens ? "" : undefined} data-deliverable-kind={iconKind(file)}
        onClick={() => run(() => (opens ? RunAPI.OpenRunDeliverable(file.runId, file.path) : RunAPI.ShowRunDeliverable(file.runId, file.path)))}
        onKeyDown={(e) => { if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) { e.preventDefault(); setMenu(true); } }}>
        <FileIcon kind={iconKind(file)} revealOnly={!opens} />
        <span className="deliverable-agent"><RevealText text={agent} /></span>
        {date && <span className="deliverable-date">{date}</span>}
      </button>
      {menu && (
        <div className="deliverable-menu" role="menu" aria-label={`Actions for ${name}`}>
          <button type="button" role="menuitem" data-deliverable-show={file.path} aria-label={`Show ${name} in its folder`} onClick={show}>
            Show in folder
          </button>
        </div>
      )}
    </div>
  );
}
