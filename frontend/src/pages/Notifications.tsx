// The Notifications page: what the app noticed on this computer (a run, a
// draft, a knowledge build or a Writing Voice build that ended, a
// schedule's fire, a task that answered, a meeting's note, an install, a
// newer Agentry Desktop), merged with the account's own notices where the
// active registry keeps them. A row opens its result in the app, or the
// account notice's page on the site, and is marked read; Mark all read
// clears the bell. The list is read again whenever Go says it changed.
import { useCallback, useEffect, useState } from "react";
import * as NotificationsAPI from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/notificationsapi";
import type * as app from "../../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import { BrowserOpenURL, EventsOn } from "../host";
import { Button, Card, EmptyState, ErrorBox, PageHeader, Spinner } from "../ui";
import {
  accountLine, filterRows, kindWords, markedRead, noticeLink, noticeRoute, sourceWords, whenWords, type NoticeRoute, type NotificationRow,
} from "../notifications";

export default function Notifications({ registry, onOpen }: {
  registry: string;
  /** Opens the result a notification is about. */
  onOpen: (route: NoticeRoute) => void;
}) {
  const [view, setView] = useState<app.NotificationsView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(Date.now());

  const read = useCallback(async () => {
    try {
      setView(await NotificationsAPI.List(registry));
      setError(null);
    } catch (e) {
      setError(String(e));
    }
    setNow(Date.now());
  }, [registry]);

  useEffect(() => { void read(); }, [read]);
  useEffect(() => EventsOn("notifications:changed", () => { void read(); }), [read]);
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(t);
  }, []);

  const open = async (n: NotificationRow) => {
    if (!n.read) {
      setView((v) => (v ? { ...v, notifications: markedRead(v.notifications, n.id), unread: Math.max(0, v.unread - 1) } as app.NotificationsView : v));
      NotificationsAPI.MarkRead(registry, n.id, n.accountId ?? 0).catch((e) => setError(String(e)));
    }
    const route = noticeRoute(n);
    if (route) {
      onOpen(route);
      return;
    }
    const link = noticeLink(n);
    if (link) BrowserOpenURL(link);
  };

  const markAll = async () => {
    setBusy(true);
    try {
      await NotificationsAPI.MarkAllRead(registry);
      await read();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const rows = view ? filterRows(view.notifications, unreadOnly) : [];
  const actions = view && (
    <>
      <label className="check-row small" style={{ margin: 0 }}>
        <input type="checkbox" checked={unreadOnly} onChange={(e) => setUnreadOnly(e.target.checked)} data-unread-only />
        Unread only
      </label>
      <Button onClick={() => void markAll()} busy={busy} busyLabel="Marking…" disabled={view.unread === 0} data-mark-all-read>Mark all read</Button>
    </>
  );

  return (
    <div className="notifications-page" data-notifications>
      <PageHeader
        title="Notifications"
        sub="What finished while you were elsewhere: runs, drafts, knowledge builds, scheduled runs, tasks, installs and updates."
        actions={actions}
      />
      <ErrorBox error={error} onRetry={() => void read()} />
      {!view && !error && <Spinner label="Reading your notifications…" />}
      {view && rows.length === 0 && (
        <EmptyState title={unreadOnly ? "Nothing unread" : "Nothing yet"}>
          {unreadOnly
            ? "Every notification has been read."
            : "When a run, a draft, a knowledge build, a scheduled run or an install finishes, it is listed here, and your computer tells you unless the result is already in front of you."}
        </EmptyState>
      )}
      {rows.length > 0 && (
        <Card pad={false}>
          <ul className="notification-list" aria-label="Notifications">
            {rows.map((n) => {
              const opens = !!noticeRoute(n) || !!noticeLink(n);
              return (
                <li key={n.id} className={`notification-row${n.read ? "" : " unread"}${n.tone ? ` tone-${n.tone}` : ""}`} data-notification={n.id} data-read={n.read ? "true" : "false"}>
                  <button type="button" className="notification-open" onClick={() => void open(n)} aria-label={`${n.title}${n.read ? "" : ", unread"}`}>
                    <span className="notification-dot" aria-hidden="true" />
                    <span className="notification-text">
                      <span className="notification-title">{n.title}</span>
                      {n.body && <span className="notification-body small">{n.body}</span>}
                      <span className="notification-meta small muted">
                        {kindWords(n.kind) && <>{kindWords(n.kind)} · </>}{sourceWords(n)}{whenWords(n.createdAt, now) && <> · {whenWords(n.createdAt, now)}</>}
                        {opens && <> · {n.source === "agentry" ? "Opens on the site" : "Opens here"}</>}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </Card>
      )}
      {view && accountLine(view, registry) && <p className="small muted" data-account-line>{accountLine(view, registry)}</p>}
    </div>
  );
}
