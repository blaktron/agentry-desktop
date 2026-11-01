package app

// The Notifications page and the bell. What the app itself noticed is kept
// in notifications.yaml (internal/notifications): a run, a draft, a
// knowledge build or a Writing Voice build that ended here or In Agentry, a
// schedule's fire, a persistent agent's task that answered, a meeting's
// note, a QuickAgent installed and a newer Agentry Desktop. Where the
// active registry runs the notifications extension and the person is
// signed in, the account's own notices are read from it and merged into
// the list, and marking one read is written back. With a registry that
// runs no such extension, the list is the local record alone.
//
// One desktop notification kind, "something I started finished", covers a
// run, a draft, a knowledge build, a scheduled fire and an install that
// ended: it is raised unless the result is already on screen (the page
// reports what it shows through SetShowing) or the person has switched it
// off in Settings → Persistent Agents, beside the switch for a task's
// answer. A click opens the result in the app. A persistent agent's task
// and Meeting notes keep their own notifications; this page only lists
// them.

import (
	"context"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/notifications"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

const (
	// eventNotificationsChanged: the local record changed; the page and
	// the bell read again.
	eventNotificationsChanged = "notifications:changed"
	// eventNotificationsOpen: a click on a desktop notification; carries
	// the NotificationView to open.
	eventNotificationsOpen = "notifications:open"
	// noticeKeyPrefix is in front of an entry's id in the key its desktop
	// notification hands back on a click.
	noticeKeyPrefix = "notice:"
	// accountIDPrefix is in front of an account notice's id in the id the
	// page names it by, so the two sources cannot collide.
	accountIDPrefix = "a"
)

// Where the list's notifications come from.
const (
	sourceHere    = "here"
	sourceAgentry = "agentry"
)

// accountNoticesPage is how many of the account's notices the page reads.
const accountNoticesPage = 50

// extensionsFor is how long a registry's extension list is believed before
// it is read again; extensionsRetry how soon one that could not be read is
// asked for again.
const (
	extensionsFor   = 10 * time.Minute
	extensionsRetry = time.Minute
)

// accountCall bounds one call to the registry about the account's notices.
const accountCall = 20 * time.Second

// NotificationsAPI is the Notifications page's and the bell's bound surface.
type NotificationsAPI struct {
	app *App

	mu sync.Mutex
	// showing is what the page reports it has on screen while the window
	// is in front: the Open keys of the results shown, comma-separated, ""
	// for nothing a notification would be about.
	showing string
	// exts caches each registry's extension list.
	exts map[string]cachedExtensions

	// Seams for tests; nil is the platform's own.
	showNotice func(notify.Notice) error
	minimised  func() bool
	extensions func(ctx context.Context, reg string) ([]string, error)
	token      func(reg string) string
	now        func() time.Time
}

type cachedExtensions struct {
	exts []string
	at   time.Time
	// keep is how long this answer stands.
	keep time.Duration
}

func newNotificationsAPI(app *App) *NotificationsAPI {
	return &NotificationsAPI{app: app, exts: map[string]cachedExtensions{}}
}

// NotificationView is one notification as the page lists it.
type NotificationView struct {
	// ID names it to the page: a local entry's own id, or the account
	// notice's id behind accountIDPrefix.
	ID string `json:"id"`
	// Source is where it came from: here, or agentry for an account notice.
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	// Tone is ok, warn, bad or "".
	Tone      string `json:"tone,omitempty"`
	CreatedAt string `json:"createdAt"`
	Read      bool   `json:"read"`
	// Open is what a local entry opens in the app.
	Open *notifications.Open `json:"open,omitempty"`
	// AccountID is the account notice's id: its own, or for a local entry
	// about a run In Agentry, the account's notice about the same run,
	// which the page hides behind the local one and marks read with it.
	AccountID int64 `json:"accountId,omitempty"`
	// Link is the path on the site an account notice opens, from "/", and
	// Registry the registry it is read from; "" for a notice with no page.
	Link     string `json:"link,omitempty"`
	Registry string `json:"registry,omitempty"`
}

// NotificationsView is the page.
type NotificationsView struct {
	Notifications []NotificationView `json:"notifications"`
	// Unread counts the unread ones listed.
	Unread int `json:"unread"`
	// SignedIn says the active registry holds a sign-in; Account that it
	// runs the notifications extension, so the account's notices are in
	// the list; AccountError why they could not be read when they could
	// not.
	SignedIn     bool   `json:"signedIn"`
	Account      bool   `json:"account"`
	AccountError string `json:"accountError,omitempty"`
}

// NotificationsUnreadView is the bell's count: the unread notifications
// the page lists, counted as the page counts them.
type NotificationsUnreadView struct {
	Unread int `json:"unread"`
	// Here and Account are the two halves as listed; AccountError says the
	// account's could not be read, in which case Unread is the local count
	// alone.
	Here         int    `json:"here"`
	Account      int    `json:"account"`
	AccountError string `json:"accountError,omitempty"`
}

// localView is a local entry as the page lists it.
func localView(e notifications.Entry) NotificationView {
	open := e.Open
	return NotificationView{
		ID: e.ID, Source: sourceHere, Kind: e.Kind, Title: e.Title, Body: e.Body, Tone: e.Tone,
		CreatedAt: rfc3339(e.CreatedAt), Read: e.Read(), Open: &open,
	}
}

// accountTone is the tone an account notice's kind is drawn in.
func accountTone(kind string) string {
	switch kind {
	case "runFinished":
		return notifications.ToneOK
	case "runFailures":
		return notifications.ToneBad
	case "billing", "schedules":
		return notifications.ToneWarn
	}
	return ""
}

// accountView is an account notice as the page lists it.
func accountView(reg string, n hostedrun.Notice) NotificationView {
	return NotificationView{
		ID: accountIDPrefix + strconv.FormatInt(n.ID, 10), Source: sourceAgentry, Kind: n.Kind, Title: n.Title, Body: n.Body,
		Tone: accountTone(n.Kind), CreatedAt: rfc3339(n.CreatedAt), Read: n.Read(), AccountID: n.ID, Link: n.Link, Registry: reg,
	}
}

// accountNoticeRun is the run id an account notice's link names, "" for a
// notice about something else: the site's run page is /runs/<id>.
func accountNoticeRun(link string) string {
	rest, ok := strings.CutPrefix(link, "/runs/")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, "/")
	id, _, _ = strings.Cut(id, "?")
	if !validRunID(id) {
		return ""
	}
	return id
}

// merge is the local entries and the account's notices as one list,
// newest first. An account notice about a run In Agentry that a local
// entry already reports is folded into that entry, which carries its id.
func merge(local []notifications.Entry, reg string, account []hostedrun.Notice) []NotificationView {
	views := make([]NotificationView, 0, len(local)+len(account))
	byRun := map[string]int{}
	for _, e := range local {
		if e.Open.Page == notifications.PageRun && e.Open.Hosted && e.Open.ID != "" {
			byRun[e.Open.ID] = len(views)
		}
		views = append(views, localView(e))
	}
	for _, n := range account {
		if id := accountNoticeRun(n.Link); id != "" {
			if i, ok := byRun[id]; ok && views[i].AccountID == 0 {
				views[i].AccountID = n.ID
				continue
			}
		}
		views = append(views, accountView(reg, n))
	}
	sort.SliceStable(views, func(i, j int) bool { return views[i].CreatedAt > views[j].CreatedAt })
	return views
}

// unreadOf counts the unread views.
func unreadOf(views []NotificationView) int {
	n := 0
	for _, v := range views {
		if !v.Read {
			n++
		}
	}
	return n
}

func (n *NotificationsAPI) ctx() context.Context {
	if n.app != nil && n.app.ctx != nil {
		return n.app.ctx
	}
	return context.Background()
}

// tokenFor is the active sign-in for reg, "" when signed out or refused.
func (n *NotificationsAPI) tokenFor(reg string) string {
	if reg == "" || tokenRejected(reg) {
		return ""
	}
	if n.token != nil {
		return n.token(reg)
	}
	return settings.TokenFor(reg)
}

// hasExtension reports whether reg runs the notifications extension, from
// its /meta, read at most once every extensionsFor. A registry whose
// extensions cannot be read is taken as one without, and asked again after
// extensionsRetry.
func (n *NotificationsAPI) hasExtension(ctx context.Context, reg string) bool {
	now := time.Now()
	if n.now != nil {
		now = n.now()
	}
	n.mu.Lock()
	c, ok := n.exts[reg]
	n.mu.Unlock()
	if !ok || now.Sub(c.at) > c.keep {
		read := n.extensions
		if read == nil {
			read = extensionsOf
		}
		exts, err := read(ctx, reg)
		c = cachedExtensions{exts: exts, at: now, keep: extensionsFor}
		if err != nil {
			c = cachedExtensions{at: now, keep: extensionsRetry}
		}
		n.mu.Lock()
		n.exts[reg] = c
		n.mu.Unlock()
	}
	for _, x := range c.exts {
		if x == hostedrun.NotificationsExtension {
			return true
		}
	}
	return false
}

// accountClient is the client for the account's notices on reg, nil where
// the person is signed out or the registry runs no inbox; signedIn says
// which.
func (n *NotificationsAPI) accountClient(ctx context.Context, reg string) (client *hostedrun.Client, signedIn bool) {
	token := n.tokenFor(reg)
	if token == "" {
		return nil, false
	}
	if !n.hasExtension(ctx, reg) {
		return nil, true
	}
	return hostedrun.New(reg, token), true
}

// List is the page: the local record and, where reg runs the extension and
// is signed in, the account's newest notices merged in.
func (n *NotificationsAPI) List(reg string) (*NotificationsView, error) {
	local, err := notifications.List()
	if err != nil {
		return nil, err
	}
	v := &NotificationsView{}
	var account []hostedrun.Notice
	ctx, cancel := context.WithTimeout(n.ctx(), accountCall)
	defer cancel()
	client, signedIn := n.accountClient(ctx, reg)
	v.SignedIn = signedIn
	if client != nil {
		v.Account = true
		page, err := client.Notices(ctx, false, 0, accountNoticesPage)
		if err != nil {
			v.AccountError = err.Error()
		} else {
			account = page.Notices
		}
	}
	v.Notifications = merge(local, reg, account)
	v.Unread = unreadOf(v.Notifications)
	return v, nil
}

// Unread is the bell's count: the unread notifications the page would
// list, counted as the page counts them, so the bell and the page agree
// (an account notice folded into a local entry counts once).
func (n *NotificationsAPI) Unread(reg string) (*NotificationsUnreadView, error) {
	list, err := n.List(reg)
	if err != nil {
		return nil, err
	}
	v := &NotificationsUnreadView{Unread: list.Unread, AccountError: list.AccountError}
	for _, x := range list.Notifications {
		if x.Read {
			continue
		}
		if x.Source == sourceAgentry {
			v.Account++
		} else {
			v.Here++
		}
	}
	return v, nil
}

// MarkRead marks one notification read: a local entry by its id, or an
// account notice by its id behind accountIDPrefix, written back to the
// registry. Reading a local entry about a run In Agentry also marks the
// account's notice about it, when the page knows which (accountID).
func (n *NotificationsAPI) MarkRead(reg, id string, accountID int64) error {
	ctx, cancel := context.WithTimeout(n.ctx(), accountCall)
	defer cancel()
	if rest, ok := strings.CutPrefix(id, accountIDPrefix); ok && accountID == 0 {
		if parsed, err := strconv.ParseInt(rest, 10, 64); err == nil {
			accountID = parsed
			id = ""
		}
	}
	if id != "" {
		if _, err := notifications.MarkRead(id); err != nil {
			return err
		}
	}
	// The page and the bell read again once both halves are marked, so the
	// count they read is the one after the write-back.
	defer n.changed()
	if accountID != 0 {
		client, _ := n.accountClient(ctx, reg)
		if client == nil {
			return nil
		}
		if _, err := client.ReadNotice(ctx, accountID); err != nil {
			return err
		}
	}
	return nil
}

// MarkAllRead marks every notification read, the account's too where the
// registry runs the extension.
func (n *NotificationsAPI) MarkAllRead(reg string) error {
	if _, err := notifications.MarkAllRead(); err != nil {
		return err
	}
	defer n.changed()
	ctx, cancel := context.WithTimeout(n.ctx(), accountCall)
	defer cancel()
	if client, _ := n.accountClient(ctx, reg); client != nil {
		if _, err := client.ReadAllNotices(ctx); err != nil {
			return err
		}
	}
	return nil
}

// FinishedNotifications reports whether something the person started
// ending raises a desktop notification.
func (n *NotificationsAPI) FinishedNotifications() bool { return finishedNotificationsOn() }

// SetFinishedNotifications switches those notifications on or off, and
// answers how they now stand.
func (n *NotificationsAPI) SetFinishedNotifications(on bool) (bool, error) {
	st, err := settings.Load()
	if err != nil {
		return finishedNotificationsOn(), err
	}
	st.FinishedNotificationsOff = !on
	if err := settings.Save(st); err != nil {
		return finishedNotificationsOn(), err
	}
	return on, nil
}

// finishedNotificationsOn is Settings → Persistent Agents' "Notify me when
// something I started finishes"; a settings file that cannot be read leaves
// it on.
func finishedNotificationsOn() bool {
	st, err := settings.Load()
	return err != nil || !st.FinishedNotificationsOff
}

// SetShowing is the page's report of the results on screen while the
// window has the focus, as the keys of what they would open (a run page is
// "run:<id>", a QuickAgent's page "agent:<namespace>/<name>", a Settings
// section "settings:<section>"), comma-separated; "" for none. A result
// that is on screen when it ends raises no notification: it is already in
// front of the person.
func (n *NotificationsAPI) SetShowing(keys string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.showing = keys
}

// isShowing reports whether any of keys is on screen now: the page's
// report, unless the window is minimised, which a page cannot always tell.
func (n *NotificationsAPI) isShowing(keys ...string) bool {
	n.mu.Lock()
	showing, minimised := n.showing, n.minimised
	n.mu.Unlock()
	if showing == "" {
		return false
	}
	shown := strings.Split(showing, ",")
	hit := false
	for _, k := range keys {
		if k != "" && slices.Contains(shown, k) {
			hit = true
			break
		}
	}
	if !hit {
		return false
	}
	if minimised == nil {
		minimised = func() bool { return n.app != nil && n.app.ctx != nil && nativeWindowMinimised() }
	}
	return !minimised()
}

// changed tells the page the local record moved.
func (n *NotificationsAPI) changed() {
	if n == nil {
		return
	}
	emitEvent(n.app, eventNotificationsChanged)
}

// noticed keeps e in the local record and tells the page. When finished,
// it is something the person started that has ended, and a desktop
// notification is raised for it unless its result is on screen (its Open's
// key, or any of alsoShown) or the person has them off. Best effort: a
// job's result is never refused because the record could not be written.
func (n *NotificationsAPI) noticed(e notifications.Entry, finished bool, alsoShown ...string) {
	if n == nil {
		return
	}
	kept, err := notifications.Add(e)
	if err != nil {
		log.Printf("agentry-desktop: notifications: %v", err)
		return
	}
	n.changed()
	if !finished || !finishedNotificationsOn() || n.isShowing(append([]string{kept.Open.Key()}, alsoShown...)...) {
		return
	}
	show := n.showNotice
	if show == nil {
		// Only the running application raises one: a test that builds the
		// services has no window and no notification to show.
		if application.Get() == nil {
			return
		}
		show = notify.Show
	}
	go func() {
		if err := show(notify.Notice{Title: kept.Title, Body: kept.Body, Key: noticeKeyPrefix + kept.ID}); err != nil {
			log.Printf("agentry-desktop: desktop notification for %q not raised: %v", kept.Title, err)
		}
	}()
}

// clicked is a click on one of this page's desktop notifications: the
// window comes up and the page opens the entry's result. Reports whether
// the key was one of this page's.
func (n *NotificationsAPI) clicked(ctx context.Context, key string) bool {
	id, ok := strings.CutPrefix(key, noticeKeyPrefix)
	if !ok || n == nil {
		return false
	}
	e, found, err := notifications.Find(id)
	if err != nil || !found {
		return true
	}
	windowShow(ctx)
	emitEvent(n.app, eventNotificationsOpen, localView(e))
	return true
}
