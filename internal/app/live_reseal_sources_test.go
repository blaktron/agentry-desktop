package app

// A source key's change re-seals a signed-out schedule, live:
// RunAPI.SaveSourceKey and ForgetSourceKey driven as the run setup drives
// them, against this person's keychain and an existing schedule that runs
// while they are signed out, made beforehand with `agentry schedule add
// --signed-out` (Linux with Docker Engine and systemd 256 or later). Skipped
// unless AGENTRY_LIVE_RESEAL=1:
//
//	AGENTRY_LIVE_RESEAL=1 \
//	AGENTRY_LIVE_RESEAL_SCHEDULE=<the schedule's id> \
//	AGENTRY_LIVE_RESEAL_REPO=https://<host>/<owner>/<name> \
//	go test -run TestLiveResealSources -v ./internal/app
//
// The token saved is a dummy: what is checked is the sealed credential's
// account names, read with `systemd-creds decrypt --user`, never a value.
// It refuses to run when the keychain already holds a key for the host, so
// it never replaces or removes the person's own.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/sources"
	"github.com/blaktron/agentry-cli/pkg/schedule"

	"github.com/blaktron/agentry-desktop/internal/run"
)

func TestLiveResealSources(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_RESEAL") != "1" {
		t.Skip("set AGENTRY_LIVE_RESEAL=1 (see the file's comment)")
	}
	id, repo := os.Getenv("AGENTRY_LIVE_RESEAL_SCHEDULE"), os.Getenv("AGENTRY_LIVE_RESEAL_REPO")
	if id == "" || repo == "" {
		t.Fatal("AGENTRY_LIVE_RESEAL_SCHEDULE and AGENTRY_LIVE_RESEAL_REPO are required")
	}
	st, err := run.GetSourceKeyStatus("repository", repo)
	if err != nil || st.Problem != "" {
		t.Fatalf("the address: %v %q", err, st.Problem)
	}
	if st.Saved {
		t.Fatalf("the keychain already holds a key for %s: this check never replaces the person's own", st.Host)
	}
	home, err := run.Home()
	if err != nil {
		t.Fatal(err)
	}
	cred, err := schedule.StoreIn(home).CredentialPath(id)
	if err != nil {
		t.Fatal(err)
	}
	account := sources.KeyAccount(st.Host)
	sealed := func() []string {
		t.Helper()
		out, err := exec.Command("systemd-creds", "decrypt", "--user", "--name=agentry-keys", cred, "-").Output()
		if err != nil {
			t.Fatalf("decrypt %s: %v", cred, err)
		}
		var k struct {
			Accounts map[string]json.RawMessage `json:"accounts"`
		}
		if err := json.Unmarshal(out, &k); err != nil {
			t.Fatal("the sealed keys are not in the shape the CLI writes")
		}
		var names []string
		for a := range k.Accounts {
			names = append(names, a)
		}
		slices.Sort(names)
		return names
	}
	settled := func() {
		t.Helper()
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			resealState.Lock()
			busy := resealState.running
			resealState.Unlock()
			if !busy {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatal("the re-seal did not finish in two minutes")
	}
	problem := func() string {
		resealMu.Lock()
		defer resealMu.Unlock()
		return resealProblem
	}
	before := sealed()
	t.Logf("sealed before: %q", before)
	if slices.Contains(before, account) {
		t.Fatalf("%s is sealed before any key was saved", account)
	}

	r := &RunAPI{app: &App{ctx: context.Background()}}
	t.Cleanup(func() { _, _ = r.ForgetSourceKey("repository", repo) })
	if _, err := r.SaveSourceKey("repository", repo, "dummy-not-a-token-818"); err != nil {
		t.Fatal(err)
	}
	settled()
	if after := sealed(); !slices.Contains(after, account) || problem() != "" {
		t.Fatalf("after a save the sealed keys are %q (problem %q), want %s among them", after, problem(), account)
	} else {
		t.Logf("sealed after the save: %q", after)
	}

	if _, err := r.ForgetSourceKey("repository", repo); err != nil {
		t.Fatal(err)
	}
	settled()
	if after := sealed(); slices.Contains(after, account) || problem() != "" {
		t.Fatalf("after a forget the sealed keys are %q (problem %q), want %s gone", after, problem(), account)
	} else {
		t.Logf("sealed after the forget: %q", after)
	}
}
