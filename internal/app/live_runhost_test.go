package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// TestLiveRunhostFromTheApp sets a real server up as a runhost through the
// app's own calls, runs a published release on it, and removes it. It needs a
// test server and a model key, so it runs only when asked:
//
//	AGENTRY_LIVE_RUNHOST_HOST     the server's address
//	AGENTRY_LIVE_RUNHOST_USER     its admin account (sudo)
//	AGENTRY_LIVE_RUNHOST_PASSWORD that account's password
//	AGENTRY_LIVE_RUNHOST_REGISTRY the registry the release is on, signed in
//	AGENTRY_LIVE_RUNHOST_REF      a release with no secrets
//	ANTHROPIC_API_KEY             the model's key, which travels with the run
func TestLiveRunhostFromTheApp(t *testing.T) {
	host := os.Getenv("AGENTRY_LIVE_RUNHOST_HOST")
	if host == "" {
		t.Skip("sets a server up as a runhost and runs on it: set AGENTRY_LIVE_RUNHOST_HOST and the rest")
	}
	user, pw := os.Getenv("AGENTRY_LIVE_RUNHOST_USER"), os.Getenv("AGENTRY_LIVE_RUNHOST_PASSWORD")
	reg, ref := os.Getenv("AGENTRY_LIVE_RUNHOST_REGISTRY"), os.Getenv("AGENTRY_LIVE_RUNHOST_REF")
	runner := os.Getenv("AGENTRY_RUNNER_HOME")
	if runner == "" {
		t.Fatal("set AGENTRY_RUNNER_HOME to a scratch runner home whose policy names the model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	var asked []string
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, msg string) (bool, error) {
		asked = append(asked, title)
		t.Logf("dialog %q: %s", title, firstLine(msg))
		return true, nil
	}}
	run := &RunAPI{app: app, runnerHomeDir: runner}
	r := &RunhostsAPI{app: app, run: run}
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.DefaultRegistry = reg
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}

	admin := RunhostAdmin{User: user, Password: pw}
	v, err := r.Add(RunhostAddRequest{Name: "lab", Host: host, Admin: admin})
	if err != nil {
		t.Fatalf("add: %v (dialogs %v)", err, asked)
	}
	t.Logf("set up %s on %s, CLI %s, owner %s, host key %s", v.Name, v.Host, v.CLIVersion, v.Owner, v.Fingerprint)
	defer func() {
		if err := r.Remove("lab", admin, false, false); err != nil {
			t.Errorf("remove: %v", err)
		}
		if list, _ := r.List(); len(list) != 0 {
			t.Errorf("the record stayed: %+v", list)
		}
	}()
	s, err := r.Status("lab")
	if err != nil || !s.Reachable || !s.SameVersion {
		t.Fatalf("status %+v %v", s, err)
	}
	if err := r.SetDefault("lab"); err != nil {
		t.Fatal(err)
	}

	// The output folder as the native picker would have returned it.
	out := filepath.Join(t.TempDir(), "out")
	app.markPicked(out)
	res, err := run.RunOnRunhost("lab", reg, ref, "Write the report the release asks for.", nil, out, 0, -1, false, nil, false)
	if err != nil {
		t.Fatalf("run on lab: %v", err)
	}
	t.Logf("run %s: %s exit %d, ran on %+v, output %s", res.RunID, res.Status, res.ExitCode, res.RanOn, res.OutDir)
	if res.RanOn == nil || res.RanOn.Name != "lab" || res.RanOn.Fingerprint != v.Fingerprint {
		t.Fatalf("the result does not say it ran on lab: %+v", res.RanOn)
	}
	b, err := os.ReadFile(filepath.Join(res.OutDir, "result.json"))
	if err != nil || !strings.Contains(string(b), `"runhost"`) {
		t.Fatalf("result.json does not name the runhost: %v", err)
	}
	pf, _ := policy.Resolve("")
	t.Logf("policy %s", pf.Path)
}

// TestLiveRunhostCreditsFromTheApp sets a server up as a runhost, gives it
// the signed-in account's Agentry credits through the registry's authorize
// page, runs a release there paid with them, revokes the token and removes
// the runhost, all through the app's calls. The keychain is the in-memory
// one, so nothing of it reaches this computer's. Besides the variables above
// (no model key), AGENTRY_LIVE_RUNHOST_TOKEN is a sign-in token for the
// registry; the authorize page's address is printed on stderr for a helper
// to approve as the person's browser would.
func TestLiveRunhostCreditsFromTheApp(t *testing.T) {
	host, token := os.Getenv("AGENTRY_LIVE_RUNHOST_HOST"), os.Getenv("AGENTRY_LIVE_RUNHOST_TOKEN")
	if host == "" || token == "" {
		t.Skip("gives a runhost credits and runs on them: set AGENTRY_LIVE_RUNHOST_HOST, AGENTRY_LIVE_RUNHOST_TOKEN and the rest")
	}
	keyring.MockInit()
	user, pw := os.Getenv("AGENTRY_LIVE_RUNHOST_USER"), os.Getenv("AGENTRY_LIVE_RUNHOST_PASSWORD")
	reg, ref := os.Getenv("AGENTRY_LIVE_RUNHOST_REGISTRY"), os.Getenv("AGENTRY_LIVE_RUNHOST_REF")
	// This build signs in to that registry, as a build for it does.
	prevReg := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = reg
	t.Cleanup(func() { settings.DefaultRegistryURL = prevReg })
	runner := os.Getenv("AGENTRY_RUNNER_HOME")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, msg string) (bool, error) {
		t.Logf("dialog %q: %s", title, firstLine(msg))
		return true, nil
	}}
	run := &RunAPI{app: app, runnerHomeDir: runner, openBrowser: func(u string) error {
		_, _ = fmt.Fprintln(os.Stderr, "open "+u)
		return nil
	}}
	r := &RunhostsAPI{app: app, run: run}
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.DefaultRegistry = reg
	st.Upsert(settings.Registry{URL: reg})
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.StoreToken(reg, token); err != nil {
		t.Fatal(err)
	}

	admin := RunhostAdmin{User: user, Password: pw}
	v, err := r.Add(RunhostAddRequest{Name: "lab", Host: host, Admin: admin})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	t.Logf("set up %s, owner %s", v.Name, v.Owner)
	defer func() {
		if err := r.Remove("lab", admin, false, false); err != nil {
			t.Errorf("remove: %v", err)
		}
	}()
	v, err = r.Credits("lab")
	if err != nil {
		t.Fatalf("credits: %v", err)
	}
	if !strings.Contains(v.Credits, "credits only") {
		t.Fatalf("the runhost's record names no credits token: %+v", v)
	}
	t.Logf("credits token %q kept on lab", v.Credits)

	out := filepath.Join(t.TempDir(), "out")
	app.markPicked(out)
	res, err := run.RunOnRunhost("lab", reg, ref, "Write the report the release asks for.", nil, out, 0, -1, false, nil, false)
	if err != nil {
		t.Fatalf("run on lab: %v", err)
	}
	t.Logf("run %s: %s exit %d, paid with credits %v (%d), ran on %s", res.RunID, res.Status, res.ExitCode, res.PaidWithCredits, res.Credits, res.RanOn.Name)
	if res.Status != "ok" || !res.PaidWithCredits {
		t.Fatalf("the run did not pay with the runhost's credits: %+v", res)
	}
	v, err = r.RemoveCredits("lab")
	if err != nil || v.Credits != "" {
		t.Fatalf("revoke: %+v %v", v, err)
	}
}

// TestLiveRunhostPostUpdateFromTheApp is the app's side of an update: a
// runhost the CLI set up at an older release (AGENTRY_LIVE_RUNHOST_NAME, in
// the runner home AGENTRY_RUNNER_HOME, its keys in this computer's keychain)
// is brought to the CLI this build carries by the post-update step, over its
// run key. Then a second runner home pairs with it through the app, with the
// admin sign-in, and unpairs again. Besides the variables above it takes
// AGENTRY_LIVE_RUNHOST_HOST, _USER and _PASSWORD for the pairing, and
// AGENTRY_LIVE_RUNHOST_REGISTRY and _TOKEN, the runhost owner's sign-in.
// The runhost's keys are copied from this computer's keychain into the
// in-memory one, which the rest of the test uses.
func TestLiveRunhostPostUpdateFromTheApp(t *testing.T) {
	name := os.Getenv("AGENTRY_LIVE_RUNHOST_NAME")
	if name == "" {
		t.Skip("updates a runhost the CLI set up and pairs a second home with it: set AGENTRY_LIVE_RUNHOST_NAME and the rest")
	}
	carried := carriedCLIVersion()
	if carried == "" {
		t.Fatal("this test binary carries no released CLI")
	}
	host, user, pw := os.Getenv("AGENTRY_LIVE_RUNHOST_HOST"), os.Getenv("AGENTRY_LIVE_RUNHOST_USER"), os.Getenv("AGENTRY_LIVE_RUNHOST_PASSWORD")
	reg, token := os.Getenv("AGENTRY_LIVE_RUNHOST_REGISTRY"), os.Getenv("AGENTRY_LIVE_RUNHOST_TOKEN")
	runner := os.Getenv("AGENTRY_RUNNER_HOME")
	keys, err := runhost.LoadKeys(name)
	if err != nil {
		t.Fatal(err)
	}
	keyring.MockInit()
	if err := runhost.SaveKeys(name, keys); err != nil {
		t.Fatal(err)
	}
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("HOME", cfg)
	prevReg := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = reg
	t.Cleanup(func() { settings.DefaultRegistryURL = prevReg })
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.DefaultRegistry = reg
	st.Upsert(settings.Registry{URL: reg})
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.StoreToken(reg, token); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, msg string) (bool, error) {
		t.Logf("dialog %q: %s", title, firstLine(msg))
		return true, nil
	}}
	oldPath := runhostUpdatesPath
	dir := t.TempDir()
	runhostUpdatesPath = func() (string, error) { return filepath.Join(dir, runhostUpdatesFile), nil }
	t.Cleanup(func() { runhostUpdatesPath = oldPath })

	r := &RunhostsAPI{app: app, run: &RunAPI{app: app, runnerHomeDir: runner}}
	before, err := r.view(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s runs CLI %s; this build carries %s", name, before.CLIVersion, carried)
	if before.CLIVersion == carried {
		t.Fatalf("%s already runs the carried CLI: set it up with an older release", name)
	}
	got, err := r.PostUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Update == nil {
		t.Fatalf("the step returned %+v", got)
	}
	u := got[0].Update
	t.Logf("post-update: %s from %s to %s, updated %v, attention %v, problem %q", got[0].Name, u.From, u.To, u.Updated, u.NeedsAttention, u.Problem)
	if !u.Updated || u.To != carried || got[0].CLIVersion != carried || len(u.NeedsAttention) != 0 {
		t.Fatalf("the runhost was not brought to %s: %+v", carried, u)
	}
	after, err := r.Status(name)
	if err != nil || !after.Reachable || !after.SameVersion {
		t.Fatalf("status after: %+v %v", after, err)
	}
	if again, err := r.PostUpdate(); err != nil || len(again) != 0 {
		t.Fatalf("the step ran twice for one CLI: %+v %v", again, err)
	}

	second := t.TempDir()
	p := &RunhostsAPI{app: app, run: &RunAPI{app: app, runnerHomeDir: second}}
	pv, err := p.Pair(RunhostAddRequest{Name: name + "-pair", Host: host, Admin: RunhostAdmin{User: user, Password: pw}})
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	t.Logf("paired %s: CLI %s, owner %s", pv.Name, pv.CLIVersion, pv.Owner)
	if ps, err := p.Status(pv.Name); err != nil || !ps.Reachable || !ps.SameVersion {
		t.Fatalf("the paired home's status: %+v %v", ps, err)
	}
	if err := p.Unpair(pv.Name); err != nil {
		t.Fatalf("unpair: %v", err)
	}
	if list, _ := p.List(); len(list) != 0 {
		t.Fatalf("the paired home kept %+v", list)
	}
	if again, err := r.Status(name); err != nil || !again.Reachable {
		t.Fatalf("the first home lost its runhost: %+v %v", again, err)
	}
}

// TestLiveRunhostScheduleFromTheApp makes a schedule on a runhost the CLI
// set up (AGENTRY_LIVE_RUNHOST_NAME, in AGENTRY_RUNNER_HOME), through the
// app's own calls and the CLI at AGENTRY_LIVE_CLI: the check names what the
// runhost's store would keep, the consent says so, the schedule is made,
// Settings lists it, a run of it is started there and collected into the run
// history, and it is removed. AGENTRY_LIVE_RUNHOST_REF is a release with no
// secrets, AGENTRY_LIVE_RUNHOST_REGISTRY its registry.
func TestLiveRunhostScheduleFromTheApp(t *testing.T) {
	name, cli := os.Getenv("AGENTRY_LIVE_RUNHOST_NAME"), os.Getenv("AGENTRY_LIVE_CLI")
	if name == "" || cli == "" {
		t.Skip("schedules a release on a runhost: set AGENTRY_LIVE_RUNHOST_NAME, AGENTRY_LIVE_CLI and the rest")
	}
	reg, ref := os.Getenv("AGENTRY_LIVE_RUNHOST_REGISTRY"), os.Getenv("AGENTRY_LIVE_RUNHOST_REF")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var said []string
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, msg string) (bool, error) {
		said = append(said, msg)
		t.Logf("dialog %q: %s", title, msg)
		return true, nil
	}}
	run := &RunAPI{app: app, runnerHomeDir: os.Getenv("AGENTRY_RUNNER_HOME")}
	r := &RunhostsAPI{app: app, run: run}
	s := &ScheduleAPI{app: app, findCLI: func(context.Context) (*schedules.CLI, error) { return schedules.New(cli), nil }}
	oldPath := runhostCollectedPath
	dir := t.TempDir()
	runhostCollectedPath = func() (string, error) { return filepath.Join(dir, runhostCollectedFile), nil }
	t.Cleanup(func() { runhostCollectedPath = oldPath })

	req := schedules.Request{Registry: reg, Ref: ref, On: name, MaxTurns: -1, Task: "Write the report the release asks for.",
		When: schedules.When{Kind: "daily", Time: "03:00", Zone: "UTC"}}
	ck, err := s.Check(req)
	if err != nil {
		t.Fatal(err)
	}
	if ck.Runhost == nil || !ck.Runhost.TakesSchedules || !ck.Ready {
		t.Fatalf("check %+v %+v", ck, ck.Runhost)
	}
	t.Logf("the store would keep %s", ck.Runhost.Said())
	made, err := s.Add(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(said) == 0 || !strings.Contains(said[len(said)-1], "Its store keeps") || !strings.Contains(said[len(said)-1], name) {
		t.Fatalf("the consent did not name what the runhost keeps: %q", said)
	}
	t.Logf("schedule %s made", made.ID)
	defer func() {
		if _, err := r.RemoveSchedule(name, made.ID); err != nil {
			t.Errorf("remove: %v", err)
		}
		st, err := r.Store(name)
		if err != nil || len(st.Schedules) != 0 {
			t.Errorf("the store after removal: %+v %v", st, err)
		}
	}()
	st, err := r.Store(name)
	if err != nil || len(st.Schedules) != 1 || st.Schedules[0].ID != made.ID || !st.Sealed {
		t.Fatalf("Settings' store: %+v %v", st, err)
	}
	t.Logf("Settings lists %s %q keeping %s, next %s", st.Schedules[0].ID, st.Schedules[0].Name, st.Schedules[0].Keeps, st.Schedules[0].Next)

	// A run of it now, as an occurrence would start it, then the app
	// collects it.
	_, _, c, err := r.dial(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := remote.RunSchedule(ctx, c, made.ID)
	_ = c.Close()
	if err != nil || f.Job == "" {
		t.Fatalf("run now: %+v %v", f, err)
	}
	deadline := time.Now().Add(15 * time.Minute)
	for {
		_, _, c, err := r.dial(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		list, err := remote.Schedules(ctx, c)
		_ = c.Close()
		if err != nil {
			t.Fatal(err)
		}
		finished := false
		for _, sc := range list {
			for _, fr := range sc.Fires {
				if fr.Job == f.Job && fr.Outcome != "running" {
					finished = true
					t.Logf("run %s: %s, exit %d", fr.Job, fr.Outcome, fr.ExitCode)
				}
			}
		}
		if finished {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run did not finish")
		}
		time.Sleep(10 * time.Second)
	}
	n, err := r.CollectSchedules()
	if err != nil || n != 1 {
		t.Fatalf("collected %d: %v", n, err)
	}
	if n, _ := r.CollectSchedules(); n != 0 {
		t.Fatalf("a run was collected twice")
	}
}

// TestLiveRunhostAgentFromTheApp starts the Assistant on a runhost the CLI
// set up (AGENTRY_LIVE_RUNHOST_NAME, in AGENTRY_RUNNER_HOME, with its own
// model), through the app's own calls: the list names it, a first start puts
// what it would be given there to the person in the native dialog, a message
// is answered through the relayed owner socket and leaves it running, and it
// stops.
func TestLiveRunhostAgentFromTheApp(t *testing.T) {
	name := os.Getenv("AGENTRY_LIVE_RUNHOST_NAME")
	if name == "" || os.Getenv("AGENTRY_LIVE_RUNHOST_AGENT") == "" {
		t.Skip("starts and talks to the Assistant on a runhost: set AGENTRY_LIVE_RUNHOST_NAME and AGENTRY_LIVE_RUNHOST_AGENT=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, msg string) (bool, error) {
		t.Logf("dialog %q: %s", title, msg)
		return true, nil
	}}
	r := &RunhostsAPI{app: app, run: &RunAPI{app: app, runnerHomeDir: os.Getenv("AGENTRY_RUNNER_HOME")}}
	state := func(list []RunhostAgentView) string {
		for _, a := range list {
			if a.Name == "assistant" {
				return a.State
			}
		}
		return "missing"
	}
	list, err := r.Agents(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("agents on %s: %+v", name, list)
	if state(list) == "running" {
		if _, err := r.StopAgent(name, "assistant"); err != nil {
			t.Fatal(err)
		}
	}
	if list, err = r.StartAgent(name, "assistant"); err != nil {
		t.Fatal(err)
	}
	if state(list) != "running" {
		t.Fatalf("after Start: %+v", list)
	}
	reply, err := r.SendAgent(name, "assistant", "Reply with exactly the word pong and nothing else.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("reply: %q", reply)
	if !strings.Contains(strings.ToLower(reply), "pong") {
		t.Fatalf("reply %q", reply)
	}
	if list, err = r.Agents(name); err != nil || state(list) != "running" {
		t.Fatalf("after the message the agent is %s (%v)", state(list), err)
	}
	if list, err = r.StopAgent(name, "assistant"); err != nil || state(list) == "running" {
		t.Fatalf("after Stop: %+v %v", list, err)
	}
}
