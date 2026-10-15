package app

// A run and a schedule In Agentry given an input source, live: the bound
// methods driven as the run panel drives them, against a real registry that
// keeps secrets. Skipped unless the variables are set, because the runs
// spend the account's credits:
//
//	AGENTRY_LIVE_HOSTED_SOURCE_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_SOURCE_TOKEN=<a Pro account's PAT: run:read, run:write, secret:write> \
//	AGENTRY_LIVE_HOSTED_SOURCE_REF=chris/source-type-probe@0.1.1 \
//	AGENTRY_LIVE_HOSTED_SOURCE_REPO=https://github.com/<a private repository> \
//	AGENTRY_LIVE_HOSTED_SOURCE_KEY=<a token that reads it> \
//	go test -run TestLiveHostedRunAndScheduleWithASource -v -timeout 40m ./internal/app
//
// The release asks for the repository as its `repo` question. The test
// starts a run with the token typed for it, then saves the token on this
// (in-memory) keychain and starts one with no token typed, then makes a
// schedule with "Save in Agentry" ticked and runs it now; each run must
// succeed, and the schedule and the key saved in Agentry are removed at
// the end. The token is never logged.

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedRunAndScheduleWithASource(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_SOURCE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_SOURCE_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_HOSTED_SOURCE_REF")
	repo := os.Getenv("AGENTRY_LIVE_HOSTED_SOURCE_REPO")
	key := os.Getenv("AGENTRY_LIVE_HOSTED_SOURCE_KEY")
	if reg == "" || token == "" || ref == "" || repo == "" || key == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_SOURCE_REGISTRY, _TOKEN, _REF, _REPO and _KEY to run hosted runs and a schedule with a source")
	}
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	setUserHome(t, t.TempDir())
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
	client := hostedrun.New(settings.Normalize(reg), token)
	ctx := context.Background()
	intake := map[string]string{"repo": repo}
	noKey := func(what, text string) {
		t.Helper()
		if strings.Contains(text, key) {
			t.Fatalf("%s carries the token", what)
		}
	}
	wait := func(label, id string) *hostedrun.Run {
		t.Helper()
		deadline := time.Now().Add(15 * time.Minute)
		for time.Now().Before(deadline) {
			r, err := client.Get(ctx, id)
			if err == nil && (r.Status == "finished" || r.Status == "cancelled") {
				line := r.Receipt.CapabilitiesEnforced["source.repo"]
				noKey(label, r.Error+line)
				t.Logf("%s: run %s %s, outcome %s, error %q; source.repo: %.240s", label, r.ID, r.Status, r.Outcome, r.Error, line)
				if r.Outcome != "succeeded" || !strings.Contains(line, "with an access token") {
					t.Fatalf("%s: the run did not fetch its source with a token and succeed", label)
				}
				return r
			}
			time.Sleep(5 * time.Second)
		}
		t.Fatalf("%s: run %s never finished", label, id)
		return nil
	}

	// A run with the token typed for it.
	r, _ := newRunAPI(t)
	out := t.TempDir()
	r.app.markPicked(out)
	start, err := r.StartHostedRun(reg, ref, "Read the repository and summarise what it is for in three sentences.", nil, out, intake, nil, false, false, map[string]string{"repo": key})
	if err != nil || start.Run == nil {
		t.Fatalf("the typed-token start: %+v, %v", start, err)
	}
	wait("typed token", start.Run.ID)

	// A run with the key saved on this computer for the host, no token typed.
	if err := run.SaveSourceKey("repository", repo, key); err != nil {
		t.Fatal(err)
	}
	r, _ = newRunAPI(t)
	out = t.TempDir()
	r.app.markPicked(out)
	start, err = r.StartHostedRun(reg, ref, "Read the repository and summarise what it is for in three sentences.", nil, out, intake, nil, false, false, nil)
	if err != nil || start.Run == nil {
		t.Fatalf("the saved-key start: %+v, %v", start, err)
	}
	wait("saved key", start.Run.ID)

	// A schedule In Agentry, the key saved there with leave, run now.
	app := &App{ctx: ctx, confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	sch := &ScheduleAPI{app: app, run: &RunAPI{app: app}}
	made, err := sch.HostedAdd(HostedScheduleRequest{Registry: reg, Ref: ref, Task: "Read the repository and summarise it.", Intake: intake,
		SaveSourceKeys: true, When: schedules.When{Kind: "daily", Time: "03:00", Zone: "UTC"}})
	if err != nil || made.ID == "" {
		t.Fatalf("the schedule: %+v, %v", made, err)
	}
	host := strings.SplitN(strings.TrimPrefix(repo, "https://"), "/", 2)[0]
	t.Cleanup(func() {
		if _, err := sch.HostedRemove(reg, made.ID, ref); err != nil {
			t.Errorf("remove the schedule: %v", err)
		}
		req, _ := http.NewRequest(http.MethodDelete, settings.Normalize(reg)+"/api/v1/x/vault/me/source-keys/"+host, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("delete the key saved in Agentry: %v", err)
			return
		}
		_ = res.Body.Close()
		t.Logf("schedule %s removed; the key saved in Agentry for %s deleted: %d", made.ID, host, res.StatusCode)
	})
	now, err := sch.HostedRunNow(reg, made.ID)
	if err != nil || now.RunID == "" {
		t.Fatalf("run now: %+v, %v", now, err)
	}
	wait("schedule's run now", now.RunID)
}
