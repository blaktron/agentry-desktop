package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// docOf is a verified document of org's holding the given settings: id to
// JSON value, a leading "!" on the id meaning locked.
func docOf(org string, rows map[string]string) managed.State {
	st := managedDoc("https://r.example", org, 1, "allowed")
	st.Document.Settings = map[string]managedsettings.Setting{}
	for id, v := range rows {
		locked := strings.HasPrefix(id, "!")
		st.Document.Settings[strings.TrimPrefix(id, "!")] = managedsettings.Setting{Value: json.RawMessage(v), Locked: locked}
	}
	return st
}

// A row is reported locked, default or, when no document sets it, not at all;
// the value is the JSON the document holds, and the organization is the one
// whose document set it.
func TestManagedRowsReportLockedDefaultAndNotManaged(t *testing.T) {
	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{
		"!malwareCheck":    `"required"`,
		"run.timeout":      `900`,
		"!install.sources": `{"scope":"organization-and-built-ins","namespaces":["acme"]}`,
	})}})
	rows := (&SettingsAPI{}).ManagedRows()
	got := map[string]ManagedRowView{}
	for _, r := range rows {
		got[r.ID] = r
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v; want the three the document sets", rows)
	}
	if r := got["malwareCheck"]; r.State != ManagedLocked || r.Value != `"required"` || r.Organization != "acme" || r.Applied {
		t.Fatalf("malwareCheck = %+v; want locked, required, acme, not applied by this client", r)
	}
	if r := got["run.timeout"]; r.State != ManagedDefault || r.Value != "900" {
		t.Fatalf("run.timeout = %+v; want a default of 900", r)
	}
	if r := got["install.sources"]; r.State != ManagedLocked || !strings.Contains(r.Value, `"namespaces":["acme"]`) {
		t.Fatalf("install.sources = %+v", r)
	}
	if _, ok := got["schedules"]; ok {
		t.Fatal("a row no document sets is listed")
	}
	if r := got[managedsettings.UnsafeMode]; r.ID != "" {
		t.Fatalf("an unset row appeared: %+v", r)
	}
}

func TestManagedRowsAreEmptyWithNoDocument(t *testing.T) {
	holdManaged(t, managed.Effective{})
	if rows := (&SettingsAPI{}).ManagedRows(); rows == nil || len(rows) != 0 {
		t.Fatalf("rows = %#v; want an empty list the page can map over", rows)
	}
}

func TestOnlyUnsafeModeIsAppliedByTheLinkedAgentry(t *testing.T) {
	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{"!run.unsafeMode": `"forbidden"`})}})
	rows := (&SettingsAPI{}).ManagedRows()
	if len(rows) != 1 || !rows[0].Applied {
		t.Fatalf("rows = %+v; want run.unsafeMode applied", rows)
	}
}

// A lock beats a default whichever document comes first, and between two
// locks the first document's wins.
func TestALockBeatsADefaultAcrossDocuments(t *testing.T) {
	a := docOf("acme", map[string]string{"malwareCheck": `"optional"`})
	b := docOf("globex", map[string]string{"!malwareCheck": `"required"`})
	holdManaged(t, managed.Effective{Documents: []managed.State{a, b}})
	rows := (&SettingsAPI{}).ManagedRows()
	if len(rows) != 1 || rows[0].State != ManagedLocked || rows[0].Organization != "globex" {
		t.Fatalf("rows = %+v; want globex's lock", rows)
	}
}

// Under a lock on the malware check the app refuses to write it; under a
// default it writes as before, and Use the organization's setting forgets the
// member's choice.
func TestMalwareCheckLockDefaultAndNotManaged(t *testing.T) {
	isolateHomes(t)
	s := &SettingsAPI{}

	holdManaged(t, managed.Effective{})
	if v, err := s.SetMalwareCheck(true); err != nil || !v.MalwareCheck {
		t.Fatalf("not managed: SetMalwareCheck = %+v, %v; want it written", v, err)
	}

	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{"malwareCheck": `"required"`})}})
	if v, err := s.SetMalwareCheck(false); err != nil || v.MalwareCheck {
		t.Fatalf("under a default: SetMalwareCheck = %+v, %v; want the member's choice written", v, err)
	}
	if v, err := s.SetMalwareCheck(true); err != nil || !v.MalwareCheck {
		t.Fatal(err)
	}
	v, err := s.UseManagedMalwareCheck()
	if err != nil || v.MalwareCheck {
		t.Fatalf("UseManagedMalwareCheck = %+v, %v; want the member's own choice forgotten", v, err)
	}
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if pol, _, err := policy.Load(path); err != nil || pol.MalwareCheck != nil {
		t.Fatalf("the policy still holds a malwareCheck choice: %v, %v", pol.MalwareCheck, err)
	}

	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{"!malwareCheck": `"required"`})}})
	_, err = s.SetMalwareCheck(false)
	if err == nil || !strings.Contains(err.Error(), "managed by acme") {
		t.Fatalf("under a lock: err = %v; want a refusal naming acme", err)
	}
}
