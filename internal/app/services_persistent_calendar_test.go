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
