package main

// A schedule In Agentry, live (agentry-desktop#745; plan
// quickagent-scheduling.md M10): the bound methods driven as the Scheduled
// page and the When step drive them, against a registry that schedules
// runs. Skipped unless the variables are set, because the schedule's fire
// spends the account's credits:
//
//	AGENTRY_LIVE_SCHEDULE_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_SCHEDULE_TOKEN=<a Pro account's PAT: run:read, run:write, …> \
//	AGENTRY_LIVE_SCHEDULE_REF=ns/name@version \
//	go test -run TestLiveHostedSchedule -v -timeout 30m .
//
// It makes a once schedule a few minutes ahead, finds it in the list and on
// the calendar as due, and (with AGENTRY_LIVE_SCHEDULE_WAIT=1) waits for the
// registry to fire it and reads the run it started. With
// AGENTRY_LIVE_SCHEDULE_KEEP=1 the schedule is left for a look on the site
// and in the app; otherwise it is deleted. The token is never logged.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedSchedule(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_SCHEDULE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_SCHEDULE_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_SCHEDULE_REF")
	if reg == "" || token == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_SCHEDULE_REGISTRY, _TOKEN and _REF to make one schedule In Agentry")
	}
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: token})
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	s := &ScheduleAPI{app: app, run: &RunAPI{app: app}}

	// The run setup's offer, as the When step reads it.
	avail, err := s.run.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	if avail.Schedule == nil || !avail.Schedule.Available {
		t.Fatalf("Later and Repeat In Agentry are not offered: %+v", avail.Schedule)
	}

	// Once, a few minutes ahead, in UTC.
	at := time.Now().UTC().Add(4 * time.Minute).Truncate(time.Minute)
	name := "M10 live check " + at.Format("15:04")
	made, err := s.HostedAdd(HostedScheduleRequest{Registry: reg, Ref: ref, Name: name,
		When: schedules.When{Kind: "once", Date: at.Format("2006-01-02"), Time: at.Format("15:04"), Zone: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	if made.Refusal != "" {
		t.Fatalf("the registry did not make it: %s (%s) %v", made.Refusal, made.Detail, made.Missing)
	}
	t.Logf("made schedule %s, due %s", made.ID, at.Format(time.RFC3339))
	if os.Getenv("AGENTRY_LIVE_SCHEDULE_KEEP") == "" {
		t.Cleanup(func() {
			if ok, err := s.HostedRemove(reg, made.ID, name); !ok || err != nil {
				t.Errorf("delete: %v, %v", ok, err)
			}
		})
	}

	list, err := s.HostedSchedules(reg)
	if err != nil || list.Unavailable != "" {
		t.Fatalf("list: %+v, %v", list, err)
	}
	found := false
	for _, it := range list.Schedules {
		if it.ID == made.ID {
			found = true
			t.Logf("listed: %s %s@%s, %s, next %s", it.Name, it.Ref, it.Version, it.Describe, it.Next)
		}
	}
	if !found {
		t.Fatalf("the schedule is not listed")
	}
	from, to := at.Add(-24*time.Hour).Format(time.RFC3339), at.Add(24*time.Hour).Format(time.RFC3339)
	occurrence := func() *CalendarFire {
		cal, err := s.HostedCalendar(reg, from, to)
		if err != nil || cal.Unavailable != "" {
			t.Fatalf("calendar: %+v, %v", cal, err)
		}
		for i, f := range cal.Fires {
			if f.ScheduleID == made.ID {
				return &cal.Fires[i]
			}
		}
		return nil
	}
	if f := occurrence(); f == nil || f.Outcome != schedules.Due || f.At != at.Format(time.RFC3339) {
		t.Fatalf("the calendar's occurrence: %+v", f)
	}

	if os.Getenv("AGENTRY_LIVE_SCHEDULE_WAIT") == "" {
		return
	}
	deadline := at.Add(15 * time.Minute)
	var f *CalendarFire
	for time.Now().Before(deadline) {
		f = occurrence()
		if f != nil && f.Outcome != schedules.Due && f.Outcome != "pending" {
			break
		}
		time.Sleep(20 * time.Second)
	}
	if f == nil || f.Outcome != "ran" || f.RunID == "" {
		t.Fatalf("the occurrence did not start a run: %+v", f)
	}
	t.Logf("fired: run %s", f.RunID)
	rec, err := s.run.HostedRecord(reg, f.RunID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("the run's record: %s %s", rec.Run.Status, rec.Run.Outcome)
}
