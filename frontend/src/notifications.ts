// The Notifications page and the bell: what the app noticed on this
// computer (NotificationsAPI.List, a local record Go keeps) merged with the
// account's own notices where the active registry runs the notifications
// extension. Pure, so the node tests can import it: the page draws from
// these, and App turns an entry into the route that opens its result.
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";

export type NotificationRow = app.NotificationView;

/** The route a notification opens in the app; null for an account notice,
 *  which opens its page on the site (noticeLink). */
export type NoticeRoute =
  | { page: "run"; source: "local" | "hosted"; id: string; registry?: string; back: "notifications" }
  | { page: "agent"; ns: string; name: string; registry?: string; back: "notifications" }
  | { page: "persistent"; id: string }
  | { page: "meetings" }
  | { page: "scheduled"; focus?: string }
  | { page: "settings"; section: string; part?: string };

/** What a local entry opens. An entry with no page opens nothing. */
export function noticeRoute(n: Pick<NotificationRow, "source" | "open">): NoticeRoute | null {
  const o = n.open;
  if (n.source !== "here" || !o?.page) return null;
  switch (o.page) {
    case "run":
      if (!o.id) return null;
      return { page: "run", source: o.hosted ? "hosted" : "local", id: o.id, registry: o.registry || undefined, back: "notifications" };
    case "agent": {
      const [ns, name] = (o.id ?? "").split("/");
      if (!ns || !name) return null;
      return { page: "agent", ns, name, registry: o.registry || undefined, back: "notifications" };
    }
    case "persistent":
      return o.id ? { page: "persistent", id: o.id } : null;
    case "meetings":
      return { page: "meetings" };
    case "scheduled":
      return { page: "scheduled", focus: o.id || undefined };
    case "settings":
      return o.section ? { page: "settings", section: o.section, part: o.part || undefined } : null;
  }
  return null;
}

/** The address an account notice opens on the site: its link under the
 *  registry it came from. Only a path from "/" is one; anything else, or a
 *  notice with no page, opens nothing. */
export function noticeLink(n: Pick<NotificationRow, "source" | "link" | "registry">): string | null {
  if (n.source !== "agentry" || !n.link || !n.registry) return null;
  if (!n.link.startsWith("/") || n.link.startsWith("//") || n.link.includes("\\")) return null;
  return n.registry.replace(/\/+$/, "") + n.link;
}

/** Where a notification came from, as the row says it. */
export function sourceWords(n: Pick<NotificationRow, "source">): string {
  return n.source === "agentry" ? "In Agentry" : "On this computer";
}

/** A kind's label on the row. */
export function kindWords(kind: string): string {
  switch (kind) {
    case "run": return "Run";
    case "chat": return "Chat";
    case "draft": return "Draft";
    case "review": return "AI Review";
    case "knowledge": return "Knowledge";
    case "voice": return "Writing Voice";
    case "schedule": return "Schedule";
    case "update": return "Update";
    case "install": return "Install";
    case "task": return "Task";
    case "meeting": return "Meeting notes";
    // The account's kinds, named as its settings name the switches.
    case "runFinished": return "Run";
    case "runFailures": return "Run";
    case "billing": return "Billing";
    case "schedules": return "Schedule";
    case "bugReports": return "Bug report";
  }
  return "";
}

/** When it happened, against now: "just now", "5 min ago", "2 h ago",
 *  "yesterday", or the date. An unreadable time is "". */
export function whenWords(iso: string | undefined, now: number): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "";
  const s = Math.max(0, Math.floor((now - t) / 1000));
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} h ago`;
  const d = Math.floor(h / 24);
  if (d === 1) return "yesterday";
  if (d < 7) return `${d} days ago`;
  return new Date(t).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

/** The bell's count, as drawn: nothing at zero, the number, or "99+". */
export function bellCount(unread: number): string {
  if (unread <= 0) return "";
  return unread > 99 ? "99+" : String(unread);
}

/** The bell's accessible name. */
export function bellLabel(unread: number): string {
  if (unread <= 0) return "Notifications";
  return `Notifications, ${unread} unread`;
}

/** The sentence under the list about where the account's notices stand. */
export function accountLine(v: Pick<app.NotificationsView, "signedIn" | "account" | "accountError">, registry: string): string {
  const host = registry.replace(/^https?:\/\//, "").replace(/\/+$/, "");
  if (!v.signedIn) return host ? `Sign in to ${host} to see your account's notifications here too.` : "";
  if (!v.account) return host ? `${host} keeps no notifications for your account; what is here is what this app noticed.` : "";
  if (v.accountError) return `Your account's notifications could not be read from ${host}: ${v.accountError}`;
  return `Your account's notifications from ${host} are listed with what this app noticed.`;
}

/** The place a page shows while it is in front, as the key Go matches an
 *  ending against (an entry whose result is on screen raises no desktop
 *  notification): the run page by its run, a QuickAgent's page by its
 *  reference, the Library, Build (where a draft and an AI Review land),
 *  Knowledge, Voice, Schedule, a Settings section, Meeting notes, plus the
 *  runs whose dialogs are open. "" when the window is not in front. */
export function showingKeys(
  route: { page: string; id?: string; ns?: string; name?: string; section?: string },
  dialogs: { localRunId?: string | null; hostedRunId?: string | null },
  focused: boolean,
  visible: boolean,
): string {
  if (!focused || !visible) return "";
  const keys: string[] = [];
  switch (route.page) {
    case "run": if (route.id) keys.push(`run:${route.id}`); break;
    case "agent": if (route.ns && route.name) keys.push(`agent:${route.ns}/${route.name}`); break;
    case "library": keys.push("library"); break;
    case "build": case "validate": keys.push("build"); break;
    case "knowledge": keys.push("knowledge"); break;
    case "voice": keys.push("voice"); break;
    case "settings": if (route.section) keys.push(`settings:${route.section}`); break;
    case "meetings": keys.push("meetings"); break;
    case "scheduled": keys.push("scheduled"); break;
  }
  if (dialogs.localRunId) keys.push(`run:${dialogs.localRunId}`);
  if (dialogs.hostedRunId) keys.push(`run:${dialogs.hostedRunId}`);
  return keys.join(",");
}

/** The rows shown, by the filter. */
export function filterRows(rows: NotificationRow[], unreadOnly: boolean): NotificationRow[] {
  return unreadOnly ? rows.filter((r) => !r.read) : rows;
}

/** The rows with one marked read, as the page draws them before Go
 *  answers. */
export function markedRead(rows: NotificationRow[], id: string): NotificationRow[] {
  return rows.map((r) => (r.id === id ? ({ ...r, read: true } as NotificationRow) : r));
}
