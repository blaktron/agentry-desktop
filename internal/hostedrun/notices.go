package hostedrun

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// The account's notices: the hosted service writes one when a run, a
// draft, a knowledge build or a scheduled fire of the person's ends, when
// a start is refused for credits, when one of their schedules is paused
// and when a bug report they filed is marked fixed. A registry that runs
// the notifications extension lists it in its /meta extensions as
// NotificationsExtension; its routes sit under /api/v1/x/notifications and
// answer a token with the user:profile scope, which the desktop's sign-in
// carries.

// NotificationsExtension is the extension's name in a registry's /meta.
const NotificationsExtension = "notifications"

// Notice is one thing the hosted service told the person. Plain text, to
// be shown as such.
type Notice struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
	// Link is a path on the site the notice opens, from "/"; "" when the
	// notice has no page.
	Link      string     `json:"link,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
}

// Read reports whether the person marked the notice read.
func (n Notice) Read() bool { return n.ReadAt != nil }

// NoticesPage is one page of the account's notices, newest first; Next is
// the cursor of the following page, 0 on the last.
type NoticesPage struct {
	Notices []Notice `json:"notices"`
	Next    int64    `json:"next,omitempty"`
}

// noticesPath builds an absolute URL under the notifications extension's
// mount.
func (c *Client) noticesPath(segments ...string) string {
	u := c.base + "/api/v1/x/notifications/notices"
	for _, s := range segments {
		u += "/" + url.PathEscape(s)
	}
	return u
}

// Notices reads a page of the account's notices, newest first: at most
// limit (the route's default when 0), only the unread ones when unread,
// and the page before the cursor when before is not 0.
func (c *Client) Notices(ctx context.Context, unread bool, before int64, limit int) (*NoticesPage, error) {
	q := url.Values{}
	if unread {
		q.Set("unread", "true")
	}
	if before > 0 {
		q.Set("before", strconv.FormatInt(before, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	u := c.noticesPath()
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := c.request(ctx, http.MethodGet, u, nil, "")
	if err != nil {
		return nil, err
	}
	var page NoticesPage
	if err := c.do("read the account's notifications", req, http.StatusOK, &page); err != nil {
		return nil, err
	}
	if page.Notices == nil {
		page.Notices = []Notice{}
	}
	return &page, nil
}

// UnreadNotices is how many of the account's notices are unread.
func (c *Client) UnreadNotices(ctx context.Context) (int64, error) {
	req, err := c.request(ctx, http.MethodGet, c.noticesPath("unread-count"), nil, "")
	if err != nil {
		return 0, err
	}
	var out struct {
		Unread int64 `json:"unread"`
	}
	if err := c.do("count the account's unread notifications", req, http.StatusOK, &out); err != nil {
		return 0, err
	}
	return out.Unread, nil
}

// ReadNotice marks one of the account's notices read, and returns it as
// it now stands.
func (c *Client) ReadNotice(ctx context.Context, id int64) (*Notice, error) {
	req, err := c.request(ctx, http.MethodPost, c.noticesPath(strconv.FormatInt(id, 10), "read"), nil, "")
	if err != nil {
		return nil, err
	}
	var n Notice
	if err := c.do("mark the notification read", req, http.StatusOK, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// ReadAllNotices marks every unread notice of the account's read, and
// returns how many were unread.
func (c *Client) ReadAllNotices(ctx context.Context) (int64, error) {
	req, err := c.request(ctx, http.MethodPost, c.noticesPath("read-all"), nil, "")
	if err != nil {
		return 0, err
	}
	var out struct {
		Marked int64 `json:"marked"`
	}
	if err := c.do("mark the notifications read", req, http.StatusOK, &out); err != nil {
		return 0, err
	}
	return out.Marked, nil
}
