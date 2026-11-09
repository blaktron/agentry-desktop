package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

// TestLiveRunhostScheduleMovesFromTheApp moves a schedule on a runhost
// already paired with this runner home through the app's own call: to each
// saved model the runhost holds, then back to its default, reading the
// runhost's list after each move. A name the runhost does not hold is
// refused and leaves the schedule as it was. It runs only when asked:
//
//	AGENTRY_LIVE_KEYCHAIN=1               the system keychain, which holds its run key
//	AGENTRY_LIVE_RUNHOST_SCHEDULE         the paired runhost's name
//	AGENTRY_LIVE_RUNHOST_SCHEDULEID       a schedule there
//	AGENTRY_LIVE_RUNHOST_CONFIG_DIR       the config directory it was paired from
//	AGENTRY_RUNNER_HOME                   the runner home it was paired from
func TestLiveRunhostScheduleMovesFromTheApp(t *testing.T) {
	name, id := os.Getenv("AGENTRY_LIVE_RUNHOST_SCHEDULE"), os.Getenv("AGENTRY_LIVE_RUNHOST_SCHEDULEID")
	if name == "" || id == "" {
		t.Skip("moves a schedule on a paired runhost: set AGENTRY_LIVE_RUNHOST_SCHEDULE and AGENTRY_LIVE_RUNHOST_SCHEDULEID")
	}
	if dir := os.Getenv("AGENTRY_LIVE_RUNHOST_CONFIG_DIR"); dir != "" {
		t.Setenv(configdir.EnvVar, dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	app := &App{ctx: ctx}
	r := &RunhostsAPI{app: app, run: &RunAPI{app: app, runnerHomeDir: os.Getenv("AGENTRY_RUNNER_HOME")}}
	held, err := r.Models(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(held.Models) < 2 {
		t.Fatalf("runhost %s holds %d saved models: make two available on it first", name, len(held.Models))
	}
	modelOf := func() string {
		t.Helper()
		st, err := r.Store(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, sc := range st.Schedules {
			if sc.ID == id {
				return sc.Model
			}
		}
		t.Fatalf("runhost %s keeps no schedule %s", name, id)
		return ""
	}
	before := modelOf()
	if _, err := r.MoveSchedule(name, id, "0badc0de"); err == nil {
		t.Fatal("a saved model the runhost does not hold was taken")
	} else {
		t.Logf("refused: %v", err)
	}
	if got := modelOf(); got != before {
		t.Fatalf("a refused move changed the schedule: %q, was %q", got, before)
	}
	for _, m := range held.Models {
		v, err := r.MoveSchedule(name, id, m.ID)
		if err != nil {
			t.Fatalf("move to %s: %v", m.Name, err)
		}
		if v.Model != m.ID || modelOf() != m.ID {
			t.Fatalf("move to %s: the card has %q", m.Name, v.Model)
		}
		t.Logf("moved to %s (%s)", m.Name, m.ID)
	}
	if v, err := r.MoveSchedule(name, id, ""); err != nil || v.Model != "default" {
		t.Fatalf("move to the default: %+v %v", v, err)
	}
	list, err := r.Models(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list.Models {
		for _, u := range m.Users {
			if u.Kind == "schedule" && u.ID == id {
				t.Fatalf("the schedule follows the default, yet %s lists it", m.Name)
			}
		}
	}
}
