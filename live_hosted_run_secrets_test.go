package main

// A run In Agentry of a release that declares a secret, live
// (agentry-desktop#300; plan hosted-credentials-vault.md D12, D13): the
// bound methods driven as the run panel drives them, against a real
// registry that keeps secrets. Skipped unless the variables are set,
// because the run spends the account's credits:
//
//	AGENTRY_LIVE_HOSTED_SECRET_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_SECRET_TOKEN=<the sign-in grant's PAT: run:read, run:write, secret:write, …> \
//	AGENTRY_LIVE_HOSTED_SECRET_REF=agentry/github-issue-triage \
//	AGENTRY_LIVE_HOSTED_SECRET_NAME=GITHUB_TOKEN \
//	AGENTRY_LIVE_HOSTED_SECRET_VALUE=<a probe value> \
//	go test -run TestLiveHostedRunWithASecret -v -timeout 20m .
//
// The account must not keep the name. The test checks that the mode is
// offered with the name declared, that the first start is refused
// secret-missing naming it, that the value set with keep false lets the
// second start run, and that the run finishes; the value is never logged.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedRunWithASecret(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_SECRET_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_SECRET_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_HOSTED_SECRET_REF")
	name := os.Getenv("AGENTRY_LIVE_HOSTED_SECRET_NAME")
	value := os.Getenv("AGENTRY_LIVE_HOSTED_SECRET_VALUE")
	if reg == "" || token == "" || ref == "" || name == "" || value == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_SECRET_REGISTRY, _TOKEN, _REF, _NAME and _VALUE to run one hosted run with a secret")
	}
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: token})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	r, events := newRunAPI(t)

	avail, err := r.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("availability: offered %v, available %v (%s), declares %v", avail.Offered, avail.Available, avail.Reason, avail.DeclaredSecrets)
	if !avail.Available || !slices.Contains(avail.DeclaredSecrets, name) {
		t.Fatalf("a run In Agentry of %s is not available with %s declared: %+v", ref, name, avail)
	}
	if !r.KeepsSecrets(reg) {
		t.Fatal("the registry does not say it keeps secrets")
	}

	out := t.TempDir()
	r.app.markPicked(out)
	start, err := r.StartHostedRun(reg, ref, "List the open issues of octocat/Hello-World and stop.", nil, out, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first start: refusal %q, missing %v", start.Refusal, start.Missing)
	if start.Refusal != "secret-missing" || !slices.Contains(start.Missing, name) {
		t.Fatalf("the first start was not refused secret-missing naming %s: %+v", name, start)
	}

	if err := r.SetHostedSecret(reg, name, value, false); err != nil {
		t.Fatal(err)
	}
	out2 := t.TempDir()
	r.app.markPicked(out2)
	start, err = r.StartHostedRun(reg, ref, "List the open issues of octocat/Hello-World and stop.", nil, out2, nil, nil, false)
	if err != nil || start.Run == nil {
		t.Fatalf("the second start: %+v, %v", start, err)
	}
	t.Logf("run %s: %s", start.Run.ID, start.Run.Status)

	deadline := time.Now().Add(15 * time.Minute)
	var final *HostedRunView
	for final == nil && time.Now().Before(deadline) {
		for _, v := range events.states() {
			if v.Final {
				final = v
			}
		}
		time.Sleep(2 * time.Second)
	}
	if final == nil {
		t.Fatal("the run never reported a final state")
	}
	t.Logf("run %s: %s, outcome %s, exit %s, error %q", final.ID, final.State, final.Outcome, final.ExitClass, final.Error)
	for _, l := range events.lines() {
		if strings.Contains(l, value) {
			t.Fatal("an event carried the value")
		}
	}
	if final.Outcome != "succeeded" {
		t.Fatalf("the run did not succeed: %+v", final)
	}
}
