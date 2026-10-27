package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// calendarRig is a persistent rig whose keychain is in memory and whose
// feed check is counted, never fetched.
type calendarRig struct {
	*persistentRig
	keys    *memSecrets
	checked []string
	refuse  error
}

func newCalendarRig(t *testing.T) *calendarRig {
	t.Helper()
	r := &calendarRig{persistentRig: newPersistentRig(t), keys: &memSecrets{}}
	r.p.secrets = r.keys
	r.p.calendarCheck = func(_ context.Context, address string) (string, int, error) {
		r.checked = append(r.checked, address)
		if r.refuse != nil {
			return "", 0, r.refuse
		}
		return "Work", 3, nil
	}
	return r
}

// feedOf is the view's feed named name.
func feedOf(v *PersistentAgentView, name string) (PersistentCalendarFeedView, bool) {
	i := slices.IndexFunc(v.CalendarFeeds, func(f PersistentCalendarFeedView) bool { return f.Name == name })
	if i < 0 {
		return PersistentCalendarFeedView{}, false
	}
	return v.CalendarFeeds[i], true
}

const (
	plantedFeed = "webcal://calendar.example.com/ical/planted-secret-0123456789/basic.ics"
	plantedHTTP = "https://calendar.example.com/ical/planted-secret-0123456789/basic.ics"
)

// TestACalendarFeedIsAddedOnceItReads: the address is fetched and read
// before anything is kept, a webcal address is kept as https in the
// keychain, under the feed's name, after the agreement that names it; the
// view lists the feed by name and host and never carries the address. A
// refused check or agreement keeps nothing; http and the Coding agent are
// refused before any fetch.
func TestACalendarFeedIsAddedOnceItReads(t *testing.T) {
	rig := newCalendarRig(t)
	account := persistent.CalendarKeychainAccount("assistant", "Work")
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if !v.ReadsCalendars || len(v.CalendarFeeds) != 0 {
		t.Fatalf("a new Assistant's feeds: %v %+v", v.ReadsCalendars, v.CalendarFeeds)
	}
	if c, _ := rig.p.Get("coder"); c.ReadsCalendars || len(c.CalendarFeeds) != 0 {
		t.Fatalf("the Coding agent's feeds: %+v", c.CalendarFeeds)
	}
	if _, err := rig.p.AddCalendarFeed("coder", "Work", plantedFeed); err == nil {
		t.Fatal("the Coding agent took a calendar feed")
	}

	_, err = rig.p.AddCalendarFeed("assistant", "Work", "http://calendar.example.com/ical/planted-secret-0123456789/basic.ics")
	if err == nil || !strings.Contains(err.Error(), "only https is read") {
		t.Fatalf("an http address: %v", err)
	}
	if strings.Contains(err.Error(), "planted-secret") {
		t.Errorf("the refusal repeats the address: %v", err)
	}
	if len(rig.checked) != 0 {
		t.Fatalf("an http address was fetched: %q", rig.checked)
	}

	rig.refuse = fmt.Errorf("%w: calendar.example.com answered HTTP 404", persistent.ErrCalendarFeedUnreadable)
	if _, err := rig.p.AddCalendarFeed("assistant", "Work", plantedFeed); !errors.Is(err, persistent.ErrCalendarFeedUnreadable) {
		t.Fatalf("an unreadable feed: %v", err)
	}
	if _, err := rig.keys.Get(account); err == nil {
		t.Fatal("an unreadable feed's address was kept")
	}
	if v, _ = rig.p.Get("assistant"); len(v.CalendarFeeds) != 0 {
		t.Fatalf("an unreadable feed was recorded: %+v", v.CalendarFeeds)
	}

	rig.refuse, rig.answer = nil, false
	if _, err := rig.p.AddCalendarFeed("assistant", "Work", plantedFeed); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if _, err := rig.keys.Get(account); err == nil {
		t.Fatal("an unagreed feed's address was kept")
	}
	if d := rig.dialogs(); len(d) == 0 || !strings.Contains(d[len(d)-1], "calendar feed Work") || strings.Contains(d[len(d)-1], "planted-secret") {
		t.Errorf("the agreement asked: %q", d)
	}

	rig.answer = true
	if v, err = rig.p.AddCalendarFeed("assistant", " Work ", plantedFeed); err != nil {
		t.Fatal(err)
	}
	if got := rig.checked[len(rig.checked)-1]; got != plantedHTTP {
		t.Errorf("the check fetched %q", got)
	}
	if got, _ := rig.keys.Get(account); got != plantedHTTP {
		t.Fatalf("the keychain holds %q", got)
	}
	f, ok := feedOf(v, "Work")
	if !ok || f.Host != "calendar.example.com" || !f.On || f.Set == "" || f.Entry != "calendar:Work" {
		t.Fatalf("the feed's view: %+v", v.CalendarFeeds)
	}
	if !slices.ContainsFunc(v.Roster, func(e PersistentRosterEntry) bool { return e.ID == "calendar:Work" && e.Kind == "calendar" && e.On }) {
		t.Errorf("the feed's card: %+v", v.Roster)
	}
	if b, _ := json.Marshal(v); strings.Contains(string(b), "planted-secret") {
		t.Fatal("the view carries the address")
	}
}

// TestACalendarFeedSetAgainAsksFirst: the same name, in any case, sets the
// feed's address again under the name it was first given, after a native
// question; a No keeps the old address and fetches nothing.
func TestACalendarFeedSetAgainAsksFirst(t *testing.T) {
	rig := newCalendarRig(t)
	if _, err := rig.p.AddCalendarFeed("assistant", "Work", plantedFeed); err != nil {
		t.Fatal(err)
	}
	account := persistent.CalendarKeychainAccount("assistant", "Work")
	before := len(rig.checked)
	rig.answer = false
	if _, err := rig.p.AddCalendarFeed("assistant", "work", "https://calendar.example.com/ical/planted-secret-new/basic.ics"); !errors.Is(err, errNotConfirmed) || len(rig.checked) != before {
		t.Fatalf("a refused replace: %v, fetched %d times", err, len(rig.checked)-before)
	}
	if got, _ := rig.keys.Get(account); got != plantedHTTP {
		t.Fatalf("a refused replace changed the keychain: %q", got)
	}
	rig.answer = true
	v, err := rig.p.AddCalendarFeed("assistant", "work", "https://calendar.example.com/ical/planted-secret-new/basic.ics")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := rig.keys.Get(account); got != "https://calendar.example.com/ical/planted-secret-new/basic.ics" {
		t.Fatalf("replaced: %q", got)
	}
	if len(v.CalendarFeeds) != 1 || v.CalendarFeeds[0].Name != "Work" {
		t.Fatalf("the feeds after a replace: %+v", v.CalendarFeeds)
	}
}

// TestACalendarFeedIsRenamedAndRemoved: a rename moves the address to the
// new name's keychain entry and keeps the feed switched as it was; a name
// another feed has, or one that differs only in its capitals, is refused;
// Remove takes the feed and its address away.
func TestACalendarFeedIsRenamedAndRemoved(t *testing.T) {
	rig := newCalendarRig(t)
	for _, n := range []string{"Work", "Family"} {
		if _, err := rig.p.AddCalendarFeed("assistant", n, plantedFeed); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rig.p.RenameCalendarFeed("assistant", "Work", "family"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("a rename onto another feed: %v", err)
	}
	if _, err := rig.p.RenameCalendarFeed("assistant", "Work", "WORK"); err == nil || !strings.Contains(err.Error(), "capitals") {
		t.Fatalf("a rename of the capitals alone: %v", err)
	}
	if _, err := rig.p.RenameCalendarFeed("assistant", "Holidays", "Days off"); err == nil {
		t.Fatal("a feed the agent lacks was renamed")
	}

	v, err := rig.p.RenameCalendarFeed("assistant", "work", "Office")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := feedOf(v, "Work"); ok {
		t.Fatalf("the old name stayed: %+v", v.CalendarFeeds)
	}
	if f, ok := feedOf(v, "Office"); !ok || !f.On || f.Host != "calendar.example.com" {
		t.Fatalf("the renamed feed: %+v", v.CalendarFeeds)
	}
	if got, _ := rig.keys.Get(persistent.CalendarKeychainAccount("assistant", "Office")); got != plantedHTTP {
		t.Fatalf("the new name's keychain entry: %q", got)
	}
	if _, err := rig.keys.Get(persistent.CalendarKeychainAccount("assistant", "Work")); err == nil {
		t.Fatal("the old name's keychain entry stayed")
	}

	// A feed switched off stays off when renamed, and asks nothing.
	if _, err := rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: "calendar:Family", On: false}}); err != nil {
		t.Fatal(err)
	}
	asked := len(rig.dialogs())
	if v, err = rig.p.RenameCalendarFeed("assistant", "Family", "Home"); err != nil {
		t.Fatal(err)
	}
	if f, ok := feedOf(v, "Home"); !ok || f.On {
		t.Fatalf("the renamed feed that was off: %+v", v.CalendarFeeds)
	}
	if len(rig.dialogs()) != asked {
		t.Errorf("renaming a feed that is off asked: %q", rig.dialogs()[asked:])
	}

	// A No to a renamed feed's new sentence leaves the feed as it was.
	rig.answer = false
	if _, err := rig.p.RenameCalendarFeed("assistant", "Office", "Job"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a refused rename: %v", err)
	}
	if _, err := rig.keys.Get(persistent.CalendarKeychainAccount("assistant", "Job")); err == nil {
		t.Fatal("a refused rename left the new name's keychain entry")
	}
	if got, _ := rig.keys.Get(persistent.CalendarKeychainAccount("assistant", "Office")); got != plantedHTTP {
		t.Fatalf("a refused rename moved the address: %q", got)
	}
	rig.answer = true

	if v, err = rig.p.RemoveCalendarFeed("assistant", "office"); err != nil {
		t.Fatal(err)
	}
	if _, ok := feedOf(v, "Office"); ok {
		t.Fatalf("removed: %+v", v.CalendarFeeds)
	}
	if _, err := rig.keys.Get(persistent.CalendarKeychainAccount("assistant", "Office")); err == nil {
		t.Fatal("a removed feed's address stayed in the keychain")
	}
}

// TestCalendarFeedsWaitForTheAgentToStop: while another client runs the
// Assistant, adding, renaming and removing a feed are refused and change
// nothing.
func TestCalendarFeedsWaitForTheAgentToStop(t *testing.T) {
	rig := newElsewhereRig(t)
	keys := &memSecrets{}
	rig.p.secrets = keys
	fetched := 0
	rig.p.calendarCheck = func(context.Context, string) (string, int, error) { fetched++; return "", 1, nil }
	if _, err := rig.p.AddCalendarFeed("assistant", "Work", plantedFeed); err != nil {
		t.Fatal(err)
	}
	rig.hold(t, rig.home, true)
	for what, f := range map[string]func() (*PersistentAgentView, error){
		"add":    func() (*PersistentAgentView, error) { return rig.p.AddCalendarFeed("assistant", "Family", plantedFeed) },
		"rename": func() (*PersistentAgentView, error) { return rig.p.RenameCalendarFeed("assistant", "Work", "Office") },
		"remove": func() (*PersistentAgentView, error) { return rig.p.RemoveCalendarFeed("assistant", "Work") },
	} {
		if _, err := f(); err == nil || !strings.Contains(err.Error(), "stop it first") {
			t.Errorf("%s while it runs elsewhere: %v", what, err)
		}
	}
	if fetched != 1 {
		t.Errorf("a feed was fetched while the agent ran: %d", fetched)
	}
	rig.hold(t, rig.home, false)
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.CalendarFeeds) != 1 || v.CalendarFeeds[0].Name != "Work" {
		t.Fatalf("the feeds after the refusals: %+v", v.CalendarFeeds)
	}
	if _, err := keys.Get(persistent.CalendarKeychainAccount("assistant", "Work")); err != nil {
		t.Fatal("a refused removal took the address")
	}
}

const plantedSession = `[{"n":"SID","v":"planted-cookie-0123456789","d":".example.com","p":"/"}]`

// TestACalendarFeedIsSignedInThroughABrowser: an address that asks for a
// sign-in says to sign in through a browser and keeps nothing; Sign in in
// a browser window keeps the address and the browser's cookies, and the
// feed shows signed in; signed in again with no address, the feed's own is
// opened; a cancelled or failed sign-in keeps the keychain as it was; set
// again without one, the sign-in goes; a rename moves it and a removal
// removes it. The view never carries the address or a cookie.
func TestACalendarFeedIsSignedInThroughABrowser(t *testing.T) {
	rig := newCalendarRig(t)
	part := persistent.CalendarSessionAccount("assistant", "Work", 1)
	var opened []string
	signIn := func(ctx context.Context, address string) (string, error) {
		opened = append(opened, address)
		return plantedSession, nil
	}
	rig.p.calendarSignIn = func(ctx context.Context, address string) (string, error) { return signIn(ctx, address) }

	rig.refuse = fmt.Errorf("%w at accounts.google.com", persistent.ErrCalendarFeedSignIn)
	if _, err := rig.p.AddCalendarFeed("assistant", "Work", plantedFeed); err == nil || !strings.Contains(err.Error(), "Sign in in a browser window") || strings.Contains(err.Error(), "planted-secret") {
		t.Fatalf("an address that asks for a sign-in: %v", err)
	}
	if len(rig.keys.m) != 0 {
		t.Fatalf("kept %v", rig.keys.m)
	}
	v, err := rig.p.SignInCalendarFeed("assistant", "Work", plantedFeed)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := rig.keys.Get(part); got != plantedSession || len(opened) != 1 || opened[0] != plantedHTTP {
		t.Fatalf("the sign-in: keys %v, opened %v", rig.keys.m, opened)
	}
	if f, _ := feedOf(v, "Work"); !f.SignedIn {
		t.Fatalf("the feed's view: %+v", v.CalendarFeeds)
	}
	if b, _ := json.Marshal(v); strings.Contains(string(b), "planted-secret") || strings.Contains(string(b), "planted-cookie") {
		t.Fatal("the view carries the address or the sign-in")
	}
	// Signed in again with no address: the feed's own is opened, and
	// nothing asks to replace it.
	asked := len(rig.dialogs())
	if _, err := rig.p.SignInCalendarFeed("assistant", "work", ""); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 2 || opened[1] != plantedHTTP {
		t.Fatalf("signed in again opened %v", opened)
	}
	if d := rig.dialogs(); len(d) > asked && strings.Contains(d[len(d)-1], "Replace the address") {
		t.Errorf("signing in again asked to replace the address: %q", d)
	}
	// Cancelled: nothing changes.
	signIn = func(ctx context.Context, _ string) (string, error) {
		go rig.p.CancelCalendarSignIn("assistant")
		<-ctx.Done()
		return "", ctx.Err()
	}
	if _, err := rig.p.SignInCalendarFeed("assistant", "Work", ""); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("a cancelled sign-in: %v", err)
	}
	signIn = func(context.Context, string) (string, error) { return "", errors.New("the browser window was closed") }
	if _, err := rig.p.SignInCalendarFeed("assistant", "Work", ""); err == nil {
		t.Fatal("a closed window kept a sign-in")
	}
	if got, _ := rig.keys.Get(part); got != plantedSession {
		t.Fatalf("a failed sign-in changed the kept one: %q", got)
	}
	if _, err := rig.p.SignInCalendarFeed("assistant", "Home", ""); err == nil {
		t.Fatal("a new feed was signed in with no address")
	}
	// Renamed: the sign-in moves with the address.
	if _, err := rig.p.RenameCalendarFeed("assistant", "Work", "Office"); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.keys.Get(part); err == nil {
		t.Fatal("the old name's sign-in was left")
	}
	if got, _ := rig.keys.Get(persistent.CalendarSessionAccount("assistant", "Office", 1)); got != plantedSession {
		t.Fatalf("the sign-in did not move: %v", rig.keys.m)
	}
	// Set again without a sign-in: it goes.
	rig.refuse = nil
	v, err = rig.p.AddCalendarFeed("assistant", "Office", plantedFeed)
	if err != nil {
		t.Fatal(err)
	}
	if f, _ := feedOf(v, "Office"); f.SignedIn {
		t.Fatal("still signed in")
	}
	if _, err := rig.keys.Get(persistent.CalendarSessionAccount("assistant", "Office", 1)); err == nil {
		t.Fatal("the sign-in was left after the address was set without one")
	}
	signIn = func(context.Context, string) (string, error) { return plantedSession, nil }
	if _, err := rig.p.SignInCalendarFeed("assistant", "Office", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.RemoveCalendarFeed("assistant", "Office"); err != nil {
		t.Fatal(err)
	}
	if len(rig.keys.m) != 0 {
		t.Fatalf("a removal left %v", rig.keys.m)
	}
}
