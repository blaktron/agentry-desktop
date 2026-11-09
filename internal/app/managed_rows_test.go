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
	if r := got["malwareCheck"]; r.State != ManagedLocked || r.Value != `"required"` || r.Organization != "acme" || !r.Applied {
		t.Fatalf("malwareCheck = %+v; want locked, required, acme, applied", r)
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

// Every row of the catalogue is applied here: the runner's, and the two
// this app holds through its own paths (runs In Agentry, bug reports).
func TestEveryCatalogueRowIsApplied(t *testing.T) {
	for _, r := range managedsettings.Catalogue() {
		if !managed.Applies(r.ID) {
			t.Errorf("%s is not applied by this app", r.ID)
		}
	}
	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{"!bugReports": `"forbidden"`, "!run.where": `["local"]`})}})
	if r := managedCalls.current().RunRefusal(); r != "" {
		t.Errorf("a lock on a row the app holds refuses every run: %q", r)
	}
}

// Report a bug is refused, with the organization's words, while an
// organization forbids bug reports; nothing is sent.
func TestABugReportIsRefusedWhileBugReportsAreForbidden(t *testing.T) {
	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{"!bugReports": `"forbidden"`})}})
	got, err := (&SettingsAPI{}).ReportBug("https://r.example", "The run panel froze after I pressed Stop.")
	if err != nil {
		t.Fatal(err)
	}
	if got.Filed || got.Reason != "refused" || !strings.Contains(got.Message, "acme's managed settings (bugReports)") {
		t.Fatalf("ReportBug = %+v", got)
	}
}

// The setters refuse a value a lock leaves out, in the runner's words, and
// write nothing.
func TestSettersRefuseALockedValue(t *testing.T) {
	isolateHomes(t)
	isolateDeliverables(t)
	holdManaged(t, managed.Effective{Documents: []managed.State{docOf("acme", map[string]string{
		"!run.where":                 `["local"]`,
		"!run.timeout":               `600`,
		"!deliverables.destinations": `["webhook"]`,
		"!desktop.applications":      `"forbidden"`,
	})}})
	api := &SettingsAPI{}
	if _, err := api.SetRunWhere("agentry"); err == nil || !strings.Contains(err.Error(), "run.where") {
		t.Errorf("SetRunWhere(agentry) = %v", err)
	}
	if _, err := api.SetRunWhere("here"); err != nil {
		t.Errorf("SetRunWhere(here) = %v", err)
	}
	if _, err := api.SetRunBudgets(3600, 0); err == nil || !strings.Contains(err.Error(), "capped at 600 seconds by acme's managed settings (run.timeout)") {
		t.Errorf("SetRunBudgets(3600) = %v", err)
	}
	if _, err := api.SetRunBudgets(0, 0); err == nil {
		t.Error("SetRunBudgets with no timeout under a cap was accepted")
	}
	if _, err := api.SetDeliverables(true); err == nil || !strings.Contains(err.Error(), "deliverables.destinations") {
		t.Errorf("SetDeliverables(true) = %v", err)
	}
	if _, err := api.SetDeliverables(false); err != nil {
		t.Errorf("SetDeliverables(false) = %v", err)
	}
	if _, err := (&YourComputerAPI{}).SetComputerUse(true); err == nil || !strings.Contains(err.Error(), "desktop.applications") {
		t.Errorf("SetComputerUse(true) = %v", err)
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
