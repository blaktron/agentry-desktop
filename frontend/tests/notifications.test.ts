// The Notifications page's helpers: what each entry opens, the bell's
// count, the places the page reports as on screen, and the words.
import { test } from "node:test";
import assert from "node:assert/strict";
import type * as app from "../bindings/github.com/blaktron/agentry-desktop/internal/app/models";
import {
  accountLine, bellCount, bellLabel, filterRows, kindWords, markedRead, noticeLink, noticeRoute, showingKeys, sourceWords, whenWords,
} from "../src/notifications.ts";

const row = (extra: Partial<app.NotificationView>): app.NotificationView =>
  ({ id: "n1", source: "here", kind: "run", title: "reporter finished", createdAt: "2026-10-31T12:00:00Z", read: false, ...extra }) as app.NotificationView;

test("a local entry opens the page its Open names, with the way back to Notifications", () => {
  assert.deepEqual(noticeRoute(row({ open: { page: "run", id: "r1" } as app.NotificationView["open"] })), { page: "run", source: "local", id: "r1", registry: undefined, back: "notifications" });
  assert.deepEqual(
    noticeRoute(row({ open: { page: "run", id: "11111111-1111-1111-1111-111111111111", hosted: true, registry: "https://reg.example" } as app.NotificationView["open"] })),
    { page: "run", source: "hosted", id: "11111111-1111-1111-1111-111111111111", registry: "https://reg.example", back: "notifications" },
  );
  assert.deepEqual(noticeRoute(row({ open: { page: "agent", id: "acme/reporter", registry: "https://reg.example" } as app.NotificationView["open"] })), { page: "agent", ns: "acme", name: "reporter", registry: "https://reg.example", back: "notifications" });
  assert.deepEqual(noticeRoute(row({ open: { page: "persistent", id: "assistant" } as app.NotificationView["open"] })), { page: "persistent", id: "assistant" });
  assert.deepEqual(noticeRoute(row({ open: { page: "meetings" } as app.NotificationView["open"] })), { page: "meetings" });
  assert.deepEqual(noticeRoute(row({ open: { page: "scheduled", id: "s1" } as app.NotificationView["open"] })), { page: "scheduled", focus: "s1" });
  assert.deepEqual(noticeRoute(row({ open: { page: "settings", section: "updates", part: "desktop" } as app.NotificationView["open"] })), { page: "settings", section: "updates", part: "desktop" });
  // Nothing to open: no page, a run with no id, an agent with no namespace,
  // a page the app does not have, or an account notice.
  assert.equal(noticeRoute(row({ open: { page: "" } as app.NotificationView["open"] })), null);
  assert.equal(noticeRoute(row({ open: { page: "run" } as app.NotificationView["open"] })), null);
  assert.equal(noticeRoute(row({ open: { page: "agent", id: "reporter" } as app.NotificationView["open"] })), null);
  assert.equal(noticeRoute(row({ open: { page: "elsewhere" } as app.NotificationView["open"] })), null);
  assert.equal(noticeRoute(row({ source: "agentry", open: { page: "run", id: "r1" } as app.NotificationView["open"] })), null);
});

test("an account notice opens its path on the site it came from, and nothing else", () => {
  assert.equal(noticeLink(row({ source: "agentry", link: "/runs/abc", registry: "https://reg.example/" })), "https://reg.example/runs/abc");
  assert.equal(noticeLink(row({ source: "agentry", registry: "https://reg.example" })), null);
  assert.equal(noticeLink(row({ source: "agentry", link: "//evil.example/x", registry: "https://reg.example" })), null);
  assert.equal(noticeLink(row({ source: "agentry", link: "https://evil.example/x", registry: "https://reg.example" })), null);
  assert.equal(noticeLink(row({ source: "agentry", link: "/runs\\abc", registry: "https://reg.example" })), null);
  assert.equal(noticeLink(row({ source: "here", link: "/runs/abc", registry: "https://reg.example" })), null);
});

test("the bell counts the unread, up to 99+", () => {
  assert.equal(bellCount(0), "");
  assert.equal(bellCount(3), "3");
  assert.equal(bellCount(150), "99+");
  assert.equal(bellLabel(0), "Notifications");
  assert.equal(bellLabel(3), "Notifications, 3 unread");
});

test("the page reports the results on screen while the window is in front", () => {
  assert.equal(showingKeys({ page: "run", id: "r1" }, {}, true, true), "run:r1");
  assert.equal(showingKeys({ page: "agent", ns: "acme", name: "reporter" }, {}, true, true), "agent:acme/reporter");
  assert.equal(showingKeys({ page: "library" }, {}, true, true), "library");
  assert.equal(showingKeys({ page: "settings", section: "updates" }, {}, true, true), "settings:updates");
  assert.equal(showingKeys({ page: "settings" }, {}, true, true), "");
  assert.equal(showingKeys({ page: "home" }, { localRunId: "r2", hostedRunId: "h1" }, true, true), "run:r2,run:h1");
  assert.equal(showingKeys({ page: "meetings" }, { localRunId: "r2" }, true, true), "meetings,run:r2");
  // Not in front: nothing, whatever is open.
  assert.equal(showingKeys({ page: "run", id: "r1" }, { localRunId: "r2" }, false, true), "");
  assert.equal(showingKeys({ page: "run", id: "r1" }, {}, true, false), "");
});

test("when it happened reads against now", () => {
  const now = Date.parse("2026-10-31T12:00:00Z");
  assert.equal(whenWords("2026-10-31T11:59:40Z", now), "just now");
  assert.equal(whenWords("2026-10-31T11:55:00Z", now), "5 min ago");
  assert.equal(whenWords("2026-10-31T09:00:00Z", now), "3 h ago");
  assert.equal(whenWords("2026-10-30T09:00:00Z", now), "yesterday");
  assert.equal(whenWords("2026-10-28T09:00:00Z", now), "3 days ago");
  assert.match(whenWords("2026-09-01T09:00:00Z", now), /2026/);
  assert.equal(whenWords("not a time", now), "");
  assert.equal(whenWords(undefined, now), "");
});

test("the words of a row and of the account's standing", () => {
  assert.equal(sourceWords(row({})), "On this computer");
  assert.equal(sourceWords(row({ source: "agentry" })), "In Agentry");
  assert.equal(kindWords("knowledge"), "Knowledge");
  assert.equal(kindWords("runFailures"), "Run");
  assert.equal(kindWords("bugReports"), "Bug report");
  assert.equal(kindWords("surprise"), "");
  assert.equal(accountLine({ signedIn: false, account: false }, "https://reg.example/"), "Sign in to reg.example to see your account's notifications here too.");
  assert.equal(accountLine({ signedIn: true, account: false }, "https://reg.example"), "reg.example keeps no notifications for your account; what is here is what this app noticed.");
  assert.equal(accountLine({ signedIn: true, account: true, accountError: "timed out" }, "https://reg.example"), "Your account's notifications could not be read from reg.example: timed out");
  assert.equal(accountLine({ signedIn: true, account: true }, "https://reg.example"), "Your account's notifications from reg.example are listed with what this app noticed.");
  assert.equal(accountLine({ signedIn: false, account: false }, ""), "");
});

test("the unread filter and the optimistic read", () => {
  const rows = [row({ id: "a" }), row({ id: "b", read: true }), row({ id: "c" })];
  assert.deepEqual(filterRows(rows, true).map((r) => r.id), ["a", "c"]);
  assert.equal(filterRows(rows, false).length, 3);
  assert.deepEqual(markedRead(rows, "a").map((r) => r.read), [true, true, false]);
});
