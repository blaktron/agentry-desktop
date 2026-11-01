package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/notifications"
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// notificationsRig is a NotificationsAPI with its own config directory,
// the notifications it raised, and the events it sent the page.
type notificationsRig struct {
	n      *NotificationsAPI
	mu     sync.Mutex
	raised []notify.Notice
	events []string
	opened []NotificationView
}

func newNotificationsRig(t *testing.T) *notificationsRig {
	t.Helper()
	t.Setenv(configdir.EnvVar, t.TempDir())
	rig := &notificationsRig{}
	rig.n = newNotificationsAPI(&App{ctx: context.Background()})
	rig.n.showNotice = func(n notify.Notice) error {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		rig.raised = append(rig.raised, n)
		return nil
	}
	rig.n.minimised = func() bool { return false }
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		rig.events = append(rig.events, name)
		if name == eventNotificationsOpen && len(data) == 1 {
			if v, ok := data[0].(NotificationView); ok {
				rig.opened = append(rig.opened, v)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })
	return rig
}

// raisedTitles waits for the notifications raised on their own goroutine.
func (r *notificationsRig) raisedTitles(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		n := len(r.raised)
		r.mu.Unlock()
		if n >= want || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, n := range r.raised {
		out = append(out, n.Title)
	}
	return out
}

// Something the person started that ended is kept and raises a desktop
// notification keyed to its entry; not while its result is on screen, not
// while the person has them off, and never for news that is not an ending.
func TestAnEndingIsKeptAndRaisedUnlessShownOrOff(t *testing.T) {
	rig := newNotificationsRig(t)
	e := localEntry(runhistory.Entry{ID: "r1", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeOK}, "acme/reporter@1.0.0")
	rig.n.noticed(e, true)
	got := rig.raisedTitles(t, 1)
	if len(got) != 1 || got[0] != "reporter finished" {
		t.Fatalf("raised %v", got)
	}
	list, _ := notifications.List()
	if len(list) != 1 || list[0].Read() {
		t.Fatalf("kept %+v", list)
	}
	rig.mu.Lock()
	key := rig.raised[0].Key
	rig.mu.Unlock()
	if key != noticeKeyPrefix+list[0].ID {
		t.Fatalf("key %q for entry %q", key, list[0].ID)
	}

	// The run page of r2 is in front: no notification for r2's ending.
	rig.n.SetShowing("home,run:r2")
	rig.n.noticed(localEntry(runhistory.Entry{ID: "r2", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeError, ExitClass: "timeout"}, "acme/reporter@1.0.0"), true)
	// Minimised, the page's report does not count.
	rig.n.minimised = func() bool { return true }
	rig.n.noticed(localEntry(runhistory.Entry{ID: "r3", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeOK}, "acme/reporter@1.0.0"), true)
	rig.n.minimised = func() bool { return false }
	rig.n.SetShowing("")
	// Off: kept, not raised.
	if _, err := rig.n.SetFinishedNotifications(false); err != nil {
		t.Fatal(err)
	}
	rig.n.noticed(localEntry(runhistory.Entry{ID: "r4", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeOK}, "acme/reporter@1.0.0"), true)
	if _, err := rig.n.SetFinishedNotifications(true); err != nil {
		t.Fatal(err)
	}
	// News that is not an ending is kept and not raised.
	rig.n.noticed(updateAvailableEntry("9.9.9", false), false)
	// The install's extra place: the Library in front holds it back.
	rig.n.SetShowing("library")
	rig.n.noticed(installEntry("https://reg.example", "acme", "reporter", "1.0.1"), true, "library")
	rig.n.SetShowing("")

	time.Sleep(50 * time.Millisecond)
	got = rig.raisedTitles(t, 2)
	if len(got) != 2 || got[1] != "reporter finished" {
		t.Fatalf("raised %v", got)
	}
	list, _ = notifications.List()
	if len(list) != 6 {
		t.Fatalf("kept %d entries", len(list))
	}
	unread, _ := rig.n.Unread("")
	if unread.Unread != 6 || unread.Here != 6 {
		t.Fatalf("unread %+v", unread)
	}
	rig.mu.Lock()
	changed := 0
	for _, ev := range rig.events {
		if ev == eventNotificationsChanged {
			changed++
		}
	}
	rig.mu.Unlock()
	if changed != 6 {
		t.Fatalf("the page was told %d times", changed)
	}
}

// A click on one of the page's notifications brings the window up and
// hands the page the entry to open; other keys are not the page's.
func TestAClickOpensTheEntry(t *testing.T) {
	rig := newNotificationsRig(t)
	saved := windowShow
	shown := 0
	windowShow = func(context.Context) { shown++ }
	t.Cleanup(func() { windowShow = saved })
	rig.n.noticed(hostedEntry("https://reg.example/", &hostedrun.Run{ID: "11111111-1111-1111-1111-111111111111", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeSucceeded}), true)
	list, _ := notifications.List()
	if !rig.n.clicked(context.Background(), noticeKeyPrefix+list[0].ID) {
		t.Fatal("the click was not the page's")
	}
	if rig.n.clicked(context.Background(), "assistant") {
		t.Fatal("an agent's key was taken as the page's")
	}
	if rig.n.clicked(context.Background(), noticeKeyPrefix+"gone") != true || shown != 1 {
		t.Fatalf("a missing entry: shown %d", shown)
	}
	rig.mu.Lock()
	defer rig.mu.Unlock()
	if len(rig.opened) != 1 || rig.opened[0].Open == nil || rig.opened[0].Open.Page != notifications.PageRun || !rig.opened[0].Open.Hosted || rig.opened[0].Open.Registry != "https://reg.example" {
		t.Fatalf("opened %+v", rig.opened)
	}
}

// The words of each ending: the QuickAgent by name, the built-ins by what
// they make, a failure by its class's meaning and never the runner's
// error line.
func TestTheWordsOfEachEnding(t *testing.T) {
	ref := "acme/reporter@1.0.0"
	cases := []struct {
		e           runhistory.Entry
		title, tone string
	}{
		{runhistory.Entry{Kind: runhistory.KindRun, Outcome: runhistory.OutcomeOK}, "reporter finished", notifications.ToneOK},
		{runhistory.Entry{Kind: runhistory.KindRun, Outcome: runhistory.OutcomeStopped}, "reporter was stopped", notifications.ToneWarn},
		{runhistory.Entry{Kind: runhistory.KindRun, Outcome: runhistory.OutcomeBlocked}, "reporter could not do the task", notifications.ToneWarn},
		{runhistory.Entry{Kind: runhistory.KindRun, Outcome: runhistory.OutcomeError, ExitClass: "timeout", Cause: "secret-looking line"}, "reporter failed", notifications.ToneBad},
		{runhistory.Entry{Kind: runhistory.KindChat, Outcome: runhistory.OutcomeOK}, "The chat with reporter ended", notifications.ToneOK},
		{runhistory.Entry{Kind: runhistory.KindDraft, Outcome: runhistory.OutcomeOK}, "Your draft is ready", notifications.ToneOK},
		{runhistory.Entry{Kind: runhistory.KindDraft, Outcome: runhistory.OutcomeError}, "The draft failed", notifications.ToneBad},
		{runhistory.Entry{Kind: runhistory.KindKnowledge, Outcome: runhistory.OutcomeOK}, "The knowledge build finished", notifications.ToneOK},
		{runhistory.Entry{Kind: runhistory.KindVoice, Outcome: runhistory.OutcomeOK}, "Your Writing Voice is ready", notifications.ToneOK},
		{runhistory.Entry{Kind: runhistory.KindReview, Outcome: runhistory.OutcomeStopped}, "The AI Review was stopped", notifications.ToneWarn},
	}
	for _, c := range cases {
		c.e.ID = "r1"
		got := localEntry(c.e, ref)
		if got.Title != c.title || got.Tone != c.tone || got.Kind != c.e.Kind || got.Open.Page != notifications.PageRun || got.Open.ID != "r1" {
			t.Errorf("%s %s: %+v", c.e.Kind, c.e.Outcome, got)
		}
		if strings.Contains(got.Body, "secret-looking") {
			t.Errorf("the cause reached the body: %q", got.Body)
		}
	}
	failed := localEntry(runhistory.Entry{ID: "r1", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeError, ExitClass: "timeout"}, ref)
	if meaning, _ := exitClassWords("timeout", 0); meaning == "" || !strings.HasPrefix(failed.Body, meaning) {
		t.Errorf("a failure's body is %q, want the class's meaning %q", failed.Body, meaning)
	}
	if got := localEntry(runhistory.Entry{ID: "r1", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeOK}, "not a ref"); got.Title != "The QuickAgent finished" {
		t.Errorf("no ref: %q", got.Title)
	}

	h := hostedEntry("https://reg.example", &hostedrun.Run{ID: "11111111-1111-1111-1111-111111111111", Ref: ref, Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeTimeout})
	if h.Title != "reporter did not finish In Agentry" || h.Tone != notifications.ToneBad || h.Body != "Stopped at the time limit for this tier." || !h.Open.Hosted {
		t.Errorf("hosted: %+v", h)
	}
	d := hostedEntry("https://reg.example", &hostedrun.Run{ID: "11111111-1111-1111-1111-111111111111", Kind: hostedrun.KindDraft, Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeSucceeded})
	if d.Title != "Your draft In Agentry is ready" || d.Kind != runhistory.KindDraft {
		t.Errorf("hosted draft: %+v", d)
	}

	ran, ok := fireEntryOf(schedules.Fire{ScheduleID: "s1", Name: "Morning report", Outcome: schedules.Ran, RunID: "abcd1234"})
	if !ok || ran.Title != "Morning report ran" || ran.Open.Page != notifications.PageRun || ran.Open.ID != "abcd1234" {
		t.Errorf("ran: %+v", ran)
	}
	missed, ok := fireEntryOf(schedules.Fire{ScheduleID: "s1", Name: "Morning report", Outcome: schedules.Missed})
	if !ok || missed.Title != "Morning report was missed" || missed.Open.Page != notifications.PageScheduled || missed.Open.ID != "s1" {
		t.Errorf("missed: %+v", missed)
	}
	if _, ok := fireEntryOf(schedules.Fire{Outcome: "running"}); ok {
		t.Error("a fire still running made an entry")
	}

	if got := accountNoticeRun("/runs/11111111-1111-1111-1111-111111111111?x=1"); got != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("run of link: %q", got)
	}
	for _, link := range []string{"", "/billing", "/runs/", "/runs/../x", "runs/abc"} {
		if got := accountNoticeRun(link); got != "" {
			t.Errorf("%q named run %q", link, got)
		}
	}
}

// accountStub serves a registry's /meta and the notifications extension's
// routes for the page's tests.
type accountStub struct {
	mu      sync.Mutex
	exts    []string
	notices []hostedrun.Notice
	calls   []string
}

func (a *accountStub) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/x/notifications/notices", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.calls = append(a.calls, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"notices": a.notices})
	})
	mux.HandleFunc("GET /api/v1/x/notifications/notices/unread-count", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		n := 0
		for _, x := range a.notices {
			if !x.Read() {
				n++
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"unread": n})
	})
	mux.HandleFunc("POST /api/v1/x/notifications/notices/read-all", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.calls = append(a.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"marked": 1})
	})
	mux.HandleFunc("POST /api/v1/x/notifications/notices/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.calls = append(a.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hostedrun.Notice{ID: 1})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// Signed in to a registry that runs the extension, the page merges the
// account's notices with the local record, folds the account's notice
// about a run In Agentry into the local entry about it, and writes a mark
// read back; with a registry that runs no extension, the page is the local
// record alone.
func TestThePageMergesTheAccountsNoticesWhereTheRegistryHasThem(t *testing.T) {
	rig := newNotificationsRig(t)
	runID := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC()
	stub := &accountStub{exts: []string{"runs", hostedrun.NotificationsExtension}, notices: []hostedrun.Notice{
		{ID: 5, Kind: "billing", Title: "Low balance", Body: "12 credits left.", CreatedAt: now.Add(-time.Minute)},
		{ID: 4, Kind: "runFinished", Title: "reporter finished", Body: "It spent 3 credits.", Link: "/runs/" + runID, CreatedAt: now.Add(-2 * time.Minute)},
	}}
	reg := stub.start(t)
	rig.n.extensions = func(_ context.Context, r string) ([]string, error) {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		return stub.exts, nil
	}
	rig.n.token = func(string) string { return "agy_token" }

	local := hostedEntry(reg, &hostedrun.Run{ID: runID, Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeSucceeded})
	local.CreatedAt = now.Add(-3 * time.Minute)
	rig.n.noticed(local, false)
	rig.n.noticed(localEntry(runhistory.Entry{ID: "r9", Kind: runhistory.KindRun, Outcome: runhistory.OutcomeOK, EndedAt: now}, "acme/other@1.0.0"), false)

	v, err := rig.n.List(reg)
	if err != nil {
		t.Fatal(err)
	}
	if !v.SignedIn || !v.Account || v.AccountError != "" {
		t.Fatalf("view %+v", v)
	}
	var titles []string
	for _, n := range v.Notifications {
		titles = append(titles, n.Source+":"+n.Title)
	}
	want := []string{"here:other finished", "agentry:Low balance", "here:reporter finished In Agentry"}
	if strings.Join(titles, "|") != strings.Join(want, "|") {
		t.Fatalf("list %v, want %v", titles, want)
	}
	folded := v.Notifications[2]
	if folded.AccountID != 4 || folded.Open == nil || folded.Open.ID != runID {
		t.Fatalf("the account's run notice was not folded: %+v", folded)
	}
	if v.Unread != 3 {
		t.Fatalf("unread %d", v.Unread)
	}
	stub.mu.Lock()
	if len(stub.calls) != 1 || !strings.HasSuffix(stub.calls[0], "Bearer agy_token") {
		t.Fatalf("calls %v", stub.calls)
	}
	stub.mu.Unlock()

	u, err := rig.n.Unread(reg)
	if err != nil || u.Unread != 4 || u.Here != 2 || u.Account != 2 {
		t.Fatalf("unread %+v %v", u, err)
	}

	// Reading the folded entry marks the account's notice too.
	if err := rig.n.MarkRead(reg, folded.ID, folded.AccountID); err != nil {
		t.Fatal(err)
	}
	// Reading an account notice by its page id goes to the registry alone.
	if err := rig.n.MarkRead(reg, accountIDPrefix+"5", 0); err != nil {
		t.Fatal(err)
	}
	if err := rig.n.MarkAllRead(reg); err != nil {
		t.Fatal(err)
	}
	stub.mu.Lock()
	calls := strings.Join(stub.calls, "\n")
	stub.mu.Unlock()
	for _, w := range []string{"POST /api/v1/x/notifications/notices/4/read", "POST /api/v1/x/notifications/notices/5/read", "POST /api/v1/x/notifications/notices/read-all"} {
		if !strings.Contains(calls, w) {
			t.Errorf("no %q in\n%s", w, calls)
		}
	}
	if n, _ := notifications.Unread(); n != 0 {
		t.Fatalf("local unread after mark all: %d", n)
	}

	// A registry without the extension: the local record alone, and no
	// call to it.
	stub.mu.Lock()
	stub.exts = []string{"runs"}
	before := len(stub.calls)
	stub.mu.Unlock()
	rig.n.exts = map[string]cachedExtensions{}
	v, err = rig.n.List(reg)
	if err != nil {
		t.Fatal(err)
	}
	if !v.SignedIn || v.Account || len(v.Notifications) != 2 {
		t.Fatalf("without the extension: %+v", v)
	}
	stub.mu.Lock()
	after := len(stub.calls)
	stub.mu.Unlock()
	if after != before {
		t.Fatalf("the registry was called without the extension: %d calls", after-before)
	}
	// Signed out: the same, and SignedIn says so.
	rig.n.token = func(string) string { return "" }
	rig.n.exts = map[string]cachedExtensions{}
	v, _ = rig.n.List(reg)
	if v.SignedIn || v.Account {
		t.Fatalf("signed out: %+v", v)
	}
}

// A schedule's fires: the first read of this copy starts the watermark and
// notices nothing; the fires that end after it are noticed once, a ran or
// failed one raised and a missed one only listed.
func TestSchedulesFiresAreNoticedOnce(t *testing.T) {
	rig := newNotificationsRig(t)
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	now := base
	rig.n.now = func() time.Time { return now }
	var list []schedules.Fire
	saved := fires
	fires = func() ([]schedules.Fire, error) { return list, nil }
	t.Cleanup(func() { fires = saved })

	list = []schedules.Fire{{ScheduleID: "s1", Name: "Old", Outcome: schedules.Ran, RunID: "abcd1234", Ended: base.Add(-time.Hour).Format(time.RFC3339)}}
	rig.n.noticeFires()
	if l, _ := notifications.List(); len(l) != 0 {
		t.Fatalf("the first read noticed %+v", l)
	}
	list = append([]schedules.Fire{
		{ScheduleID: "s2", Name: "Later", Outcome: schedules.Missed, Ended: base.Add(2 * time.Minute).Format(time.RFC3339)},
		{ScheduleID: "s1", Name: "Morning report", Outcome: schedules.Ran, RunID: "abcd1235", Ended: base.Add(time.Minute).Format(time.RFC3339)},
	}, list...)
	rig.n.noticeFires()
	rig.n.noticeFires()
	l, _ := notifications.List()
	if len(l) != 2 || l[0].Title != "Later was missed" || l[1].Title != "Morning report ran" {
		t.Fatalf("noticed %+v", l)
	}
	if got := rig.raisedTitles(t, 1); len(got) != 1 || got[0] != "Morning report ran" {
		t.Fatalf("raised %v", got)
	}
	seen, _ := notifications.FiresSeen()
	if !seen.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("watermark %v", seen)
	}
}

// The switch is read from and written to desktop.yaml; absent is on.
func TestTheFinishedSwitch(t *testing.T) {
	rig := newNotificationsRig(t)
	if !rig.n.FinishedNotifications() {
		t.Fatal("off by default")
	}
	if on, err := rig.n.SetFinishedNotifications(false); err != nil || on {
		t.Fatalf("set off: %v %v", on, err)
	}
	st, _ := settings.Load()
	if !st.FinishedNotificationsOff || rig.n.FinishedNotifications() {
		t.Fatal("not off")
	}
	if on, _ := rig.n.SetFinishedNotifications(true); !on || !rig.n.FinishedNotifications() {
		t.Fatal("not back on")
	}
}
