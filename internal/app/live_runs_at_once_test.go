package app

// Runs at once, live: several runs of one release through RunAgent at the
// same time, with the real engine, this machine's container engine and a
// scripted stand-in model on the LAN that holds each run's turn. Skipped
// unless AGENTRY_LIVE_RUNS_AT_ONCE=1:
//
//	AGENTRY_LIVE_RUNS_AT_ONCE=1 \
//	AGENTRY_LIVE_RUNS_AT_ONCE_REGISTRY=<the registry> \
//	AGENTRY_LIVE_RUNS_AT_ONCE_REF=<an autonomous release, namespace/name@version> \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home whose policy names the stand-in> \
//	HOME=<a scratch home: the default output folder is under it> \
//	go test -run TestLiveRunsAtOnce -v -timeout 30m ./internal/app
//
// It refuses to run on the person's own settings. The number of runs at once
// defaults to 2 and the runs started to 3 (AGENTRY_LIVE_RUNS_AT_ONCE_N,
// AGENTRY_LIVE_RUNS_AT_ONCE_RUNS).
//
// What it holds: no more runs are under way than the number, and that many
// are under way together at some point; the run past the number waits in
// the queue and starts as one ends; every run ends ok with its own output
// folder; and nothing labelled with any of the runs' ids is left in the
// container engine.

import (
	"cmp"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func TestLiveRunsAtOnce(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_RUNS_AT_ONCE") != "1" {
		t.Skip("runs real runs: set AGENTRY_LIVE_RUNS_AT_ONCE=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" || os.Getenv("HOME") == "" {
		t.Fatalf("set %s, %s and HOME to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg, ref := os.Getenv("AGENTRY_LIVE_RUNS_AT_ONCE_REGISTRY"), os.Getenv("AGENTRY_LIVE_RUNS_AT_ONCE_REF")
	if reg == "" || ref == "" {
		t.Fatal("set AGENTRY_LIVE_RUNS_AT_ONCE_REGISTRY and AGENTRY_LIVE_RUNS_AT_ONCE_REF to the registry and the release to run")
	}
	n, _ := strconv.Atoi(cmp.Or(os.Getenv("AGENTRY_LIVE_RUNS_AT_ONCE_N"), "2"))
	runs, _ := strconv.Atoi(cmp.Or(os.Getenv("AGENTRY_LIVE_RUNS_AT_ONCE_RUNS"), "3"))

	// Each run's progress: the most runs seen under way together, from the
	// first report of each (begun) to its RunAgent's return.
	var mu sync.Mutex
	underWay, most := map[string]bool{}, 0
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) != 1 {
			return
		}
		switch v := data[0].(type) {
		case RunProgress:
			if name == eventRunProgress {
				mu.Lock()
				if !underWay[v.RunID] {
					underWay[v.RunID] = true
					most = max(most, len(underWay))
					t.Logf("%s began: %d under way", v.RunID, len(underWay))
				}
				mu.Unlock()
			}
		case []QueuedRunView:
			t.Logf("queue: %d waiting", len(v))
		}
	}
	t.Cleanup(func() { emitEvent = old })

	r := &RunAPI{app: &App{ctx: context.Background()}}
	r.setRunsAtOnce(n)

	type outcome struct {
		res *RunResultView
		err error
	}
	results := make(chan outcome, runs)
	for i := 0; i < runs; i++ {
		go func() {
			res, err := r.RunAgent(reg, ref, "Say that the smoke check answered.", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil, "", "")
			if res != nil {
				mu.Lock()
				delete(underWay, res.RunID)
				mu.Unlock()
			}
			results <- outcome{res, err}
		}()
		// Started one after the other, as a person's clicks are.
		time.Sleep(2 * time.Second)
	}

	var ids []string
	folders := map[string]bool{}
	for i := 0; i < runs; i++ {
		o := <-results
		if o.err != nil {
			t.Errorf("a run: %v", o.err)
			continue
		}
		t.Logf("run %s: %s exit %d, folder %s", o.res.RunID, o.res.Status, o.res.ExitCode, o.res.OutDir)
		if o.res.Status != "ok" {
			t.Errorf("run %s ended %q: %s", o.res.RunID, o.res.Status, o.res.Error)
		}
		if folders[o.res.OutDir] {
			t.Errorf("two runs shared the folder %s", o.res.OutDir)
		}
		folders[o.res.OutDir] = true
		ids = append(ids, o.res.RunID)
	}
	mu.Lock()
	seen := most
	mu.Unlock()
	t.Logf("the most runs under way together: %d (the number: %d)", seen, n)
	if seen > n {
		t.Errorf("%d runs were under way together, more than the number, %d", seen, n)
	}
	if runs >= n && seen < n {
		t.Errorf("only %d runs were ever under way together, fewer than the number, %d", seen, n)
	}

	// Nothing of any run is left in the engine.
	for _, id := range ids {
		for _, kind := range []string{"container", "network", "volume"} {
			args := []string{kind, "ls", "-q", "--filter", "label=agentry.run=" + id}
			if kind == "container" {
				args = []string{"ps", "-aq", "--filter", "label=agentry.run=" + id}
			}
			out, err := exec.Command("docker", args...).Output()
			if err != nil {
				t.Errorf("docker %s: %v", strings.Join(args, " "), err)
				continue
			}
			if s := strings.TrimSpace(string(out)); s != "" {
				t.Errorf("run %s left %s(s): %s", id, kind, s)
			}
		}
	}
}
