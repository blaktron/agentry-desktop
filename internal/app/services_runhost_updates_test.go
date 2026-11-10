package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// stubRunhostUpdates makes this app carry CLI ver, keeps the update record
// in a temporary folder, and answers each runhost's update with answer.
func stubRunhostUpdates(t *testing.T, ver string, answer func(rec *runhost.Record) (remote.UpdateOutcome, error)) *[]string {
	t.Helper()
	oldUpdate, oldCLI, oldPath := runhostUpdate, runhostCarriedCLI, runhostUpdatesPath
	t.Cleanup(func() { runhostUpdate, runhostCarriedCLI, runhostUpdatesPath = oldUpdate, oldCLI, oldPath })
	dir := t.TempDir()
	runhostUpdatesPath = func() (string, error) { return filepath.Join(dir, runhostUpdatesFile), nil }
	runhostCarriedCLI = func() string { return ver }
	var asked []string
	runhostUpdate = func(_ context.Context, rec *runhost.Record, _ *runhost.DeviceKeys, o remote.UpdateOptions) (remote.UpdateOutcome, error) {
		if o.Version != ver {
			t.Errorf("the update was asked for %s, not the carried %s", o.Version, ver)
		}
		asked = append(asked, rec.Name)
		return answer(rec)
	}
	return &asked
}

func pairedRecord(t *testing.T, home, name, cli string) {
	t.Helper()
	keys, err := runhost.NewDeviceKeys()
	if err != nil {
		t.Fatal(err)
	}
	if err := runhost.SaveKeys(name, keys); err != nil {
		t.Fatal(err)
	}
	saveRecord(t, home, runhost.Record{Name: name, Host: "10.0.0.5", Port: 22, CLIVersion: cli})
}

// After the app updates itself, each runhost on another CLI is updated once,
// and each outcome is kept for Settings: an update, what needs the admin
// sign-in, a runhost newer than the app, and one that was not reached.
func TestThePostUpdateStepUpdatesEveryRunhostOnceAndKeepsEachOutcome(t *testing.T) {
	r, home := newRunhostsAPI(t)
	for name, cli := range map[string]string{"lab": "0.7.9", "attic": "0.7.9", "newer": "0.7.11", "away": "0.7.9", "same": "0.7.10"} {
		pairedRecord(t, home, name, cli)
	}
	asked := stubRunhostUpdates(t, "0.7.10", func(rec *runhost.Record) (remote.UpdateOutcome, error) {
		switch rec.Name {
		case "lab":
			rec.CLIVersion = "0.7.10"
			return remote.UpdateOutcome{Name: "lab", From: "0.7.9", To: "0.7.10", Updated: true}, nil
		case "attic":
			return remote.UpdateOutcome{Name: "attic", From: "0.7.9", To: "0.7.9", NeedsAttention: []string{"its CLI 0.7.9 takes no update over the run key: agentry runhost add attic … repairs it"}}, nil
		case "newer":
			return remote.UpdateOutcome{Name: "newer", From: "0.7.11", To: "0.7.11", DeviceBehind: "0.7.11"}, nil
		}
		return remote.UpdateOutcome{}, errors.New("dial: no route to host")
	})
	got, err := r.PostUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 4 || strings.Contains(strings.Join(*asked, ","), "same") {
		t.Fatalf("asked %v", *asked)
	}
	if len(got) != 4 {
		t.Fatalf("the step returned %d runhosts", len(got))
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]RunhostView{}
	for _, v := range list {
		by[v.Name] = v
	}
	if v := by["lab"]; v.CLIVersion != "0.7.10" || v.Update == nil || !v.Update.Updated || v.Update.From != "0.7.9" {
		t.Errorf("lab: %+v %+v", v, v.Update)
	}
	if v := by["attic"]; v.Update == nil || len(v.Update.NeedsAttention) != 1 {
		t.Errorf("attic: %+v", v.Update)
	}
	if v := by["newer"]; v.Update == nil || v.Update.DeviceBehind != "0.7.11" {
		t.Errorf("newer: %+v", v.Update)
	}
	if v := by["away"]; v.Update == nil || !strings.Contains(v.Update.Problem, "no route") {
		t.Errorf("away: %+v", v.Update)
	}
	if by["same"].Update != nil {
		t.Errorf("a runhost on the carried CLI was updated: %+v", by["same"].Update)
	}

	// The same CLI does not run the step again; Update tries one by hand.
	*asked = nil
	if _, err := r.PostUpdate(); err != nil || len(*asked) != 0 {
		t.Fatalf("the step ran again: %v %v", *asked, err)
	}
	if _, err := r.Update("away"); err != nil || len(*asked) != 1 {
		t.Fatalf("update by hand: %v %v", *asked, err)
	}
}

// A build that carries no released CLI updates nothing.
func TestThePostUpdateStepNeedsAReleasedCLI(t *testing.T) {
	r, home := newRunhostsAPI(t)
	pairedRecord(t, home, "lab", "0.7.9")
	asked := stubRunhostUpdates(t, "", func(*runhost.Record) (remote.UpdateOutcome, error) { return remote.UpdateOutcome{}, nil })
	if _, err := r.PostUpdate(); err != nil || len(*asked) != 0 {
		t.Fatalf("a build without a released CLI updated %v (%v)", *asked, err)
	}
	if _, err := r.Update("lab"); err == nil {
		t.Fatal("Update ran from a build without a released CLI")
	}
}

// Unpairing asks first, and asks nothing of the runhost when refused.
func TestUnpairAsksFirst(t *testing.T) {
	r, home := newRunhostsAPI(t)
	pairedRecord(t, home, "lab", "0.7.10")
	asked := ""
	r.app.confirm = func(_ context.Context, title, _ string) (bool, error) { asked = title; return false, nil }
	if err := r.Unpair("lab"); err == nil {
		t.Fatal("unpaired with the confirmation refused")
	}
	if !strings.Contains(asked, "Unpair") {
		t.Fatalf("the person was asked %q", asked)
	}
	if list, _ := r.List(); len(list) != 1 {
		t.Fatal("the record went with the confirmation refused")
	}
}

// With no runhost the step writes nothing: a fresh install's first start
// keeps its configuration folder as it was.
func TestThePostUpdateStepWritesNothingWithoutARunhost(t *testing.T) {
	r, _ := newRunhostsAPI(t)
	asked := stubRunhostUpdates(t, "0.7.10", func(*runhost.Record) (remote.UpdateOutcome, error) { return remote.UpdateOutcome{}, nil })
	if _, err := r.PostUpdate(); err != nil || len(*asked) != 0 {
		t.Fatalf("%v %v", *asked, err)
	}
	p, _ := runhostUpdatesPath()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("the step wrote %s: %v", p, err)
	}
}

// The runs of a runhost's schedules are collected once each, into the run
// history; a runhost that takes no schedule is not asked.
func TestRunhostScheduleRunsAreCollectedOnce(t *testing.T) {
	r, home := newRunhostsAPI(t)
	saveRecord(t, home, runhost.Record{Name: "lab", Host: "10.0.0.5", Port: 22, Capabilities: map[string]bool{"schedules": false}})
	oldPath, oldCollect := runhostCollectedPath, runhostCollectScheduled
	t.Cleanup(func() { runhostCollectedPath, runhostCollectScheduled = oldPath, oldCollect })
	dir := t.TempDir()
	runhostCollectedPath = func() (string, error) { return filepath.Join(dir, runhostCollectedFile), nil }
	called := 0
	runhostCollectScheduled = func(context.Context, string, string, string, int, string, func(string)) (*run.Result, error) {
		called++
		return &run.Result{}, nil
	}
	if n, err := r.CollectSchedules(); err != nil || n != 0 || called != 0 {
		t.Fatalf("a runhost that takes no schedule was asked: %d %v %d", n, err, called)
	}
	if _, err := os.Stat(filepath.Join(dir, runhostCollectedFile)); !os.IsNotExist(err) {
		t.Fatal("nothing collected, but the record was written")
	}
}

// A runhost schedule moves to the saved model the card names, by its id,
// or to the runhost's default for none; a name that is not a saved model's
// id never reaches the runhost, and the card gets the schedule as the
// runhost now keeps it.
func TestARunhostScheduleMovesToTheModelTheCardNames(t *testing.T) {
	r, _ := newRunhostsAPI(t)
	var asked []string
	old := runhostMoveSchedule
	t.Cleanup(func() { runhostMoveSchedule = old })
	runhostMoveSchedule = func(_ context.Context, _ *RunhostsAPI, name, id, ref string) (runhost.ScheduleInfo, error) {
		asked = append(asked, name+" "+id+" "+ref)
		if ref == "0badc0de" {
			return runhost.ScheduleInfo{}, errors.New(`this runhost holds no saved model "0badc0de"`)
		}
		return runhost.ScheduleInfo{ID: id, Name: "digest", Ref: "acme/digest@1.0.0", Model: ref, Fires: []runhost.ScheduleFire{{Job: "j1"}}}, nil
	}
	v, err := r.MoveSchedule("lab", "40t6idgwjk", "a7002bb0")
	if err != nil || v.Model != "a7002bb0" || v.Runs != 1 || v.Keeps != "nothing but its release" {
		t.Fatalf("a move to a saved model: %+v %v", v, err)
	}
	if v, err = r.MoveSchedule("lab", "40t6idgwjk", ""); err != nil || v.Model != "default" {
		t.Fatalf("a move to the default: %+v %v", v, err)
	}
	if _, err = r.MoveSchedule("lab", "40t6idgwjk", "0badc0de"); err == nil || !strings.Contains(err.Error(), "holds no saved model") {
		t.Fatalf("a refused move: %v", err)
	}
	if _, err = r.MoveSchedule("lab", "40t6idgwjk", "Llama"); err == nil {
		t.Fatal("a name that is no saved model's id was sent")
	}
	want := []string{"lab 40t6idgwjk a7002bb0", "lab 40t6idgwjk default", "lab 40t6idgwjk 0badc0de"}
	if strings.Join(asked, "|") != strings.Join(want, "|") {
		t.Fatalf("asked %q, want %q", asked, want)
	}
}
