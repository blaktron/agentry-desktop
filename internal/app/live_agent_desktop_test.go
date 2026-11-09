package app

// The app's half of a persistent agent's use of this computer, live and
// headless: the link service on the runner home's socket, for an agent the
// CLI supervises here, and the link sessions to every paired runhost, for
// the agents there, with Blender running on this computer. The test holds
// them for a while and logs every exec question and every change of a
// runhost's link, while the agents are driven from a terminal with
// `agentry agent`. Skipped unless AGENTRY_LIVE_AGENT_DESKTOP is a duration:
//
//	AGENTRY_LIVE_AGENT_DESKTOP=15m \
//	AGENTRY_LIVE_KEYCHAIN=1 \
//	AGENTRY_RUNNER_HOME=<the scratch runner home the agents run from> \
//	AGENTRY_LIVE_DESKTOP_ANSWERS=decline,once  (the exec dialog's answers, in order) \
//	go test -run TestLiveAgentDesktop -v -timeout 60m ./internal/app
//
// AGENTRY_LIVE_AGENT_DESKTOP_STOP, a file, ends the test early once it
// exists; AGENTRY_LIVE_AGENT_DESKTOP_UNLINK, a file, gives the runhosts'
// links up while it exists, as closing the app would;
// AGENTRY_LIVE_AGENT_DESKTOP_LIST, a your-computer.yaml, is the person's
// list of applications the test starts from.

import (
	"cmp"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/linkservice"
)

func TestLiveAgentDesktop(t *testing.T) {
	hold, err := time.ParseDuration(os.Getenv("AGENTRY_LIVE_AGENT_DESKTOP"))
	if err != nil || hold <= 0 {
		t.Skip("holds the app's link service and runhost links for agents driven from a terminal: set AGENTRY_LIVE_AGENT_DESKTOP to a duration (see the file's comment)")
	}
	if os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s to the scratch runner home the agents run from", policy.HomeEnv)
	}
	ctx, cancel := context.WithTimeout(context.Background(), hold)
	defer cancel()
	app := &App{ctx: ctx}
	var mu sync.Mutex
	answers := strings.Split(cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_ANSWERS"), "decline,once"), ",")
	asked := 0
	app.chooseExec = func(_ context.Context, q linkservice.ExecQuestion) (linkservice.ExecAnswer, error) {
		mu.Lock()
		defer mu.Unlock()
		asked++
		a := "decline"
		if asked <= len(answers) {
			a = strings.TrimSpace(answers[asked-1])
		}
		title, message := execDialogText(q)
		t.Logf("exec dialog %d %q, answered %s:\n%s", asked, title, a, message)
		switch a {
		case "once":
			return linkservice.ExecAllowOnce, nil
		case "all":
			return linkservice.ExecAllowRun, nil
		}
		return linkservice.ExecDecline, nil
	}
	r := &RunAPI{app: app}
	y := newYourComputerAPI(app, r)
	if list := os.Getenv("AGENTRY_LIVE_AGENT_DESKTOP_LIST"); list != "" {
		b, err := os.ReadFile(list)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(y.list.Path(), b, 0o600); err != nil {
			t.Fatal(err)
		}
		for _, a := range y.State().Applications {
			t.Logf("on the list: %s (%s), mcp=%v computer=%v ready=%v %s", a.ID, a.From, a.MCP, a.Computer, a.Ready, a.Missing)
		}
	}
	y.start(ctx)
	defer y.shutdown()
	if y.socketErr != "" {
		t.Fatalf("the link socket: %s", y.socketErr)
	}
	t.Logf("the link service serves %s", y.socket)

	rh := &RunhostsAPI{app: app, run: r}
	unlink := os.Getenv("AGENTRY_LIVE_AGENT_DESKTOP_UNLINK")
	var linkCtx context.Context
	var stopLinks context.CancelFunc = func() {}
	linked := false
	last := map[string]bool{}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		away := unlink != "" && liveFileExists(unlink)
		switch {
		case !linked && !away:
			linkCtx, stopLinks = context.WithCancel(ctx)
			rh.startAgentsLinks(linkCtx)
			linked = true
			t.Log("holding the paired runhosts' agents links")
		case linked && away:
			stopLinks()
			rh.stopAgentsLinks()
			linked = false
			t.Log("gave the runhosts' agents links up, as closing the app would")
		}
		if views, err := rh.List(); err == nil {
			for _, v := range views {
				if last[v.Name] != v.LinkedHere {
					last[v.Name] = v.LinkedHere
					t.Logf("runhost %s: linked to this computer %v", v.Name, v.LinkedHere)
				}
			}
		}
		if stop := os.Getenv("AGENTRY_LIVE_AGENT_DESKTOP_STOP"); stop != "" && liveFileExists(stop) {
			t.Log("stopped by its stop file")
			break
		}
		select {
		case <-ctx.Done():
			stopLinks()
			for _, run := range y.State().Recent {
				t.Logf("recent run %s %s (%s): %+v", run.Run, run.State, run.Agent, run.Servers)
			}
			return
		case <-tick.C:
		}
	}
	stopLinks()
	for _, run := range y.State().Recent {
		t.Logf("recent run %s %s (%s): %+v", run.Run, run.State, run.Agent, run.Servers)
	}
}

func liveFileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
