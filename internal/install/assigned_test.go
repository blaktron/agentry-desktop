package install

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
)

// heldWith is managed settings in force for org with the given rows.
func heldWith(org string, rows map[string]managedsettings.Setting) managed.Effective {
	return managed.Effective{Documents: []managed.State{{
		Registry: "https://registry.example",
		Document: managedsettings.Document{Org: org, Version: 1, Settings: rows},
	}}}
}

func lockedRow(t *testing.T, v any) managedsettings.Setting {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return managedsettings.Setting{Value: b, Locked: true}
}

// An install an organization requires goes through every check of an
// ordinary install and leaves a record that names the organization and holds
// no capabilities, even for a release that declares a contract: the person's
// first run is the consent.
func TestAnAssignedInstallRecordsNoConsent(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	caps := map[string]any{"network": map[string]any{"egress": []string{"api.example"}}}
	reg := startRegistry(t, &fakeRegistry{manifestDoc: demoManifest(caps), archive: archiveWithManifest(t, demoManifest(caps))})
	e := NewEngine()

	res, err := e.InstallAssigned(context.Background(), Assignment{Registry: reg, Ref: "acme/demo", Organization: "acme", Held: heldWith("acme", nil)})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !strings.Contains(res.Message, "acme/demo@1.0.0") {
		t.Fatalf("result: %+v", res)
	}
	st, err := installed.Load()
	if err != nil || len(st.Records) != 1 {
		t.Fatalf("records: %+v err %v", st.Records, err)
	}
	rec := st.Records[0]
	if rec.AssignedBy != "acme" || !rec.Capabilities.Empty() || !rec.AwaitsConsent() {
		t.Fatalf("an assigned install recorded consent: %+v", rec)
	}
	if rec.Registry != installdir.ConsentRegistry || rec.Target != installdir.Target || !rec.Managed {
		t.Fatalf("record: %+v", rec)
	}
	if len(e.pending) != 0 {
		t.Fatalf("an assigned install left a plan pending: %v", e.pending)
	}

	// The same version again installs nothing.
	if _, err := e.InstallAssigned(context.Background(), Assignment{Registry: reg, Ref: "acme/demo", Organization: "acme", Held: heldWith("acme", nil)}); !errors.Is(err, ErrCurrent) {
		t.Fatalf("a current install was laid down again: %v", err)
	}
}

// A person's earlier consent survives an assigned update, and the
// organization's name is put back on the record: a contract the new version
// widens is asked about by the run.
func TestAnAssignedUpdateKeepsWhatThePersonAccepted(t *testing.T) {
	isolate(t)
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	caps := map[string]any{"network": map[string]any{"egress": []string{"api.example"}}}
	reg1 := startRegistry(t, &fakeRegistry{manifestDoc: demoManifest(caps), archive: archiveWithManifest(t, demoManifest(caps))})
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg1, "acme/demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan.ID, false, true, false); err != nil {
		t.Fatal(err)
	}
	doc2 := demoManifest(caps)
	doc2["version"] = "1.1.0"
	reg2 := startRegistry(t, &fakeRegistry{version: "1.1.0", manifestDoc: doc2, archive: archiveWithManifest(t, doc2)})
	if _, err := e.InstallAssigned(context.Background(), Assignment{Registry: reg2, Ref: "acme/demo@1.1.0", Organization: "acme", Held: heldWith("acme", nil)}); err != nil {
		t.Fatal(err)
	}
	st, _ := installed.Load()
	rec := st.Records[0]
	if rec.Version != "1.1.0" || rec.AssignedBy != "acme" || rec.Capabilities.Empty() || rec.AwaitsConsent() {
		t.Fatalf("record after an assigned update: %+v", rec)
	}
}

// The organization's own rules hold an assigned install: install sources,
// accepting a release's risk, and a release that is not verified.
func TestAnAssignedInstallIsHeldToEveryCheck(t *testing.T) {
	cases := []struct {
		name    string
		reg     *fakeRegistry
		held    managed.Effective
		refusal string
	}{
		{name: "sources", reg: &fakeRegistry{}, refusal: "not allowed by other's managed settings",
			held: heldWith("other", map[string]managedsettings.Setting{managedsettings.InstallSources: lockedRow(t, managedsettings.Sources{Scope: managedsettings.SourcesOrganizationBuiltIns})})},
		{name: "unsigned, risk forbidden", reg: &fakeRegistry{unsigned: true}, refusal: managedsettings.InstallAcceptRisk,
			held: heldWith("acme", map[string]managedsettings.Setting{managedsettings.InstallAcceptRisk: lockedRow(t, managedsettings.Forbidden)})},
		{name: "unsigned", reg: &fakeRegistry{unsigned: true}, held: heldWith("acme", nil), refusal: "never laid down without a verified attestation"},
		{name: "unscanned", reg: &fakeRegistry{unscanned: true}, held: heldWith("acme", nil), refusal: "never laid down without a verified attestation"},
		{name: "corrupted archive", reg: &fakeRegistry{corruptArchive: true}, held: heldWith("acme", nil), refusal: "digest mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			reg := startRegistry(t, tc.reg)
			_, err := NewEngine().InstallAssigned(context.Background(), Assignment{Registry: reg, Ref: "acme/demo", Organization: "acme", Held: tc.held})
			if err == nil || !strings.Contains(err.Error(), tc.refusal) {
				t.Fatalf("err %v, want %q", err, tc.refusal)
			}
			if st, _ := installed.Load(); len(st.Records) != 0 {
				t.Fatalf("a refused assigned install left a record: %+v", st.Records)
			}
		})
	}
}
