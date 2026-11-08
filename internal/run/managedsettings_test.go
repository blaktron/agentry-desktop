package run

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/managedsettings"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/compile"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// holdManaged makes e the managed settings held on this computer for the
// test.
func holdManaged(t *testing.T, e managed.Effective) {
	t.Helper()
	was := currentManaged
	currentManaged = func() managed.Effective { return e }
	t.Cleanup(func() { currentManaged = was })
}

// managedDocument is org's verified document at version, with setting id
// locked to value.
func managedDocument(org string, version int64, id, value string) managed.State {
	return managed.State{
		Registry: "https://r.example",
		Document: managedsettings.Document{
			Registry: "https://r.example", Org: org, Version: version,
			Settings: map[string]managedsettings.Setting{id: {Value: json.RawMessage(`"` + value + `"`), Locked: true}},
		},
	}
}

// refusesAsPolicy checks err is the runner's policy refusal (exit 11)
// carrying want.
func refusesAsPolicy(t *testing.T, what string, err error, want string) {
	t.Helper()
	var classified *engine.ClassifiedError
	var refusal *compile.PolicyRefusal
	if !errors.As(err, &classified) || classified.Code != engine.ExitPolicy || !errors.As(err, &refusal) || !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: err = %v, want a policy refusal (exit %d) saying %q", what, err, engine.ExitPolicy, want)
	}
}

// Managed settings that refuse every run (a lock this version does not
// know, or settings that cannot be verified) refuse a run on this computer
// and on a runhost before anything is prepared or sent, with the runner's
// reason, as a policy refusal.
func TestManagedSettingsRefuseEveryRunBeforeItStarts(t *testing.T) {
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	for name, held := range map[string]managed.Effective{
		"a lock this version does not know": {Documents: []managed.State{managedDocument("acme", 2, "run.somethingNew", "on")}},
		"settings that cannot be verified":  {Problems: []managed.Problem{{Registry: "https://r.example", Err: errors.New("signature does not verify")}}},
	} {
		holdManaged(t, held)
		want := held.RunRefusal()
		if want == "" {
			t.Fatalf("%s: the fixture refuses nothing", name)
		}
		_, err := Run(context.Background(), Request{Registry: "https://r.example", Ref: "acme/x@1.0.0"}, io.Discard)
		refusesAsPolicy(t, name+", a run here", err, want)
		_, err = StartSession(context.Background(), Request{Registry: "https://r.example", Ref: "acme/x@1.0.0"}, io.Discard)
		refusesAsPolicy(t, name+", a chat here", err, want)
		_, ranOn, err := RunOnRunhost(context.Background(), RunhostRequest{Runhost: "lab", Registry: "https://r.example", Ref: "acme/x@1.0.0"}, io.Discard)
		refusesAsPolicy(t, name+", a run on a runhost", err, want)
		if ranOn != nil {
			t.Fatalf("%s: the refused runhost run names a runhost: %+v", name, ranOn)
		}
	}
}

// A run sent to a runhost carries what this computer's managed settings say
// of it, so the runhost refuses Unsafe Mode for a member whose organization
// forbids it; with none held, it carries nothing.
func TestARunhostJobCarriesTheManagedSettingsNarrowing(t *testing.T) {
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	home, _ := Home()
	sig := &Signature{Namespace: "acme", Name: "demo", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), Ref: "acme/demo@1.0.0"}
	pol := policy.Default()

	run, err := runhostCarry(home, sig, &manifest.Manifest{}, pol, RunhostRequest{}, managed.Effective{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Managed != nil {
		t.Fatalf("with no managed settings the job carries %+v", run.Managed)
	}

	held := managed.Effective{Documents: []managed.State{managedDocument("acme", 7, managedsettings.UnsafeMode, managedsettings.UnsafeModeForbidden)}}
	run, err = runhostCarry(home, sig, &manifest.Manifest{}, pol, RunhostRequest{}, held)
	if err != nil {
		t.Fatal(err)
	}
	n := run.Managed
	if n == nil || !strings.Contains(n.UnsafeModeRefusal, "forbidden by acme") {
		t.Fatalf("the job's narrowing: %+v; want Unsafe Mode refused by acme", n)
	}
	if len(n.Documents) != 1 || n.Documents[0].Organization != "acme" || n.Documents[0].Version != 7 {
		t.Fatalf("the job's documents: %+v", n.Documents)
	}
	if err := n.Validate(); err != nil {
		t.Fatalf("the runhost would refuse the narrowing as sent: %v", err)
	}
}
