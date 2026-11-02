package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
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
