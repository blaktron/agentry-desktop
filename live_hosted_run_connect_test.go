package main

// A run In Agentry of a release whose MCP server signs in by OAuth, live
// (OAuth connections M9, agentry-desktop#667; plan D5, D19): the bound
// methods driven as the run panel drives them, against a real registry
// whose vault keeps connections. Skipped unless the variables are set,
// because the runs spend the account's credits and a person must consent:
//
//	AGENTRY_LIVE_HOSTED_CONNECT_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_CONNECT_TOKEN=<a PAT of the account: run:read, run:write, …> \
//	AGENTRY_LIVE_HOSTED_CONNECT_REF=<ns/name@version of a release with an `auth: oauth` server> \
//	AGENTRY_LIVE_HOSTED_CONNECT_TASK=<what each run is asked> \
//	AGENTRY_LIVE_HOSTED_CONNECT_PAGE=<a file the connect page's address is written to> \
//	go test -run TestLiveHostedRunConnectsOnceAndRunsTwice -v -timeout 50m .
//
// The account must keep no connection for that version. The test checks
// that the first start is refused connection-missing, opens (here: writes
// down) the site's Connect tools for the version, waits while the person
// connects in a browser signed in to the site as that account, and that
// the run the wait starts and a second start, which asks nothing, both
// succeed.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedRunConnectsOnceAndRunsTwice(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_CONNECT_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_CONNECT_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_HOSTED_CONNECT_REF")
	task := os.Getenv("AGENTRY_LIVE_HOSTED_CONNECT_TASK")
	pageFile := os.Getenv("AGENTRY_LIVE_HOSTED_CONNECT_PAGE")
	if reg == "" || token == "" || ref == "" || task == "" || pageFile == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_CONNECT_REGISTRY, _TOKEN, _REF, _TASK and _PAGE to connect once and run twice In Agentry")
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
	r.openBrowser = func(u string) error {
		t.Logf("Connect tools: %s", u)
		return os.WriteFile(pageFile, []byte(u+"\n"), 0o600)
	}

	avail, err := r.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !avail.Available {
		t.Fatalf("a run In Agentry of %s is not available: %+v", ref, avail)
	}

	out := t.TempDir()
	r.app.markPicked(out)
	start, err := r.StartHostedRun(reg, ref, task, nil, out, nil, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first start: refusal %q, missing %v", start.Refusal, start.Missing)
	if start.Refusal != hostedrun.SlugConnectionMissing || len(start.Missing) == 0 {
		t.Fatalf("the first start was not refused connection-missing: %+v", start)
	}
	if err := r.OpenHostedConnect(reg, ref); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	got, err := r.AwaitHostedConnections(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Run == nil {
		t.Fatalf("the wait ended without a run: %+v", got)
	}
	t.Logf("connected after %s; run %s: %s", time.Since(began).Round(time.Second), got.Run.ID, got.Run.Status)
	first := waitFinal(t, events, got.Run.ID)

	out2 := t.TempDir()
	r.app.markPicked(out2)
	again, err := r.StartHostedRun(reg, ref, task, nil, out2, nil, nil, false, false)
	if err != nil || again.Run == nil {
		t.Fatalf("the second start asked again or failed: %+v, %v", again, err)
	}
	t.Logf("second start: run %s: %s, nothing asked", again.Run.ID, again.Run.Status)
	second := waitFinal(t, events, again.Run.ID)
	for _, v := range []*HostedRunView{first, second} {
		if v.Outcome != "succeeded" {
			t.Fatalf("run %s did not succeed: %+v", v.ID, v)
		}
	}
	t.Logf("first result: %.300s", first.ResultJSON)
	t.Logf("second result: %.300s", second.ResultJSON)
}

// waitFinal waits for the follow to report run id final.
func waitFinal(t *testing.T, events *emitted, id string) *HostedRunView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		for _, v := range events.states() {
			if v.ID == id && v.Final {
				t.Logf("run %s: %s, outcome %s, exit %s, error %q", v.ID, v.State, v.Outcome, v.ExitClass, v.Error)
				return v
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("run %s never reported a final state", id)
	return nil
}
