// The managed state of the rows a Settings part governs, drawn the same way
// in every part: a lock reads "Managed by <organization>" with a padlock and
// leaves the part to disable its control; a default reads "Set by
// <organization>", with "Use <organization>'s setting" where the part keeps
// the member's own value in a file this app writes; a row no document sets
// draws nothing. Go lists the rows; the page reads them again when they
// change.
import { useEffect, useState } from "react";
import * as SettingsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/settingsapi";
import { EventsOn } from "../host";
import { Button } from "../ui";
import { LockGlyph } from "../unsafe";
import { partNotes, type ManagedPart, type ManagedRow } from "../managed";

/** The managed rows held here, read at mount and whenever they change. Null
 *  until the first read, and after one that failed (no row is drawn). */
export function useManagedRows(): ManagedRow[] | null {
  const [rows, setRows] = useState<ManagedRow[] | null>(null);
  useEffect(() => {
    let live = true;
    const read = () => {
      // Through a promise, so a binding an older harness lacks draws no row.
      Promise.resolve()
        .then(() => SettingsAPI.ManagedRows())
        .then((r) => { if (live) setRows(r as ManagedRow[]); })
        .catch(() => { if (live) setRows(null); });
    };
    read();
    const off = EventsOn("managed:changed", read);
    return () => { live = false; off(); };
  }, []);
  return rows;
}

/** The notes of a part's rows. use: the part's way to forget the member's own
 *  value, given only where it can be done; a default without it is shown
 *  read-only. */
export function ManagedNotes({ rows, part, ids, use, busy }: {
  rows: ManagedRow[] | null;
  part: ManagedPart;
  /** Only these of the part's rows. */
  ids?: string[];
  use?: () => void;
  busy?: boolean;
}) {
  const notes = partNotes(rows, part).filter((n) => !ids || ids.includes(n.id));
  if (!notes.length) return null;
  return (
    <div role="status" data-managed-part={part}>
      {notes.map((n) => (
        <p key={n.id} className="small" data-managed-row={n.id} data-managed-state={n.state}>
          <span className="badge">{n.state === "locked" && <><LockGlyph /> </>}{n.badge}</span> {n.lines.join(" ")}
          {n.state === "default" && use && (
            <>{" "}<Button variant="link" disabled={busy} onClick={use}>{n.use}</Button></>
          )}
        </p>
      ))}
    </div>
  );
}
