package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/schedule"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-desktop/internal/destinations"
)

// scheduleFake is the agentry command, answering by subcommand, and the
// native dialog, recording what was asked.
type scheduleFake struct {
	calls  [][]string
	check  string
	asked  []string
	answer bool
	// readied records each readying of the schedule's target hosts, as
	// "<ref> <sign-ins given>", in the order of the command's calls;
	// readyErr is its answer.
	readied  []string
	readyErr error
	// version is what agentry --version prints, "" for nothing.
	version string
}

func (f *scheduleFake) exec(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case slices.Contains(args, "--check"):
		return []byte(f.check), nil, nil
	case slices.Contains(args, "add"):
		return []byte(`{"id":"ab12cd34ef"}`), nil, nil
	case slices.Contains(args, "--version"):
		return []byte(f.version), nil, nil
	}
	return nil, nil, nil
}

func (f *scheduleFake) adds() [][]string {
	var out [][]string
	for _, c := range f.calls {
		if slices.Contains(c, "add") && !slices.Contains(c, "--check") {
			out = append(out, c)
		}
	}
	return out
}

func newScheduleAPI(t *testing.T, f *scheduleFake, lingers bool) *ScheduleAPI {
	t.Helper()
	freshConfig(t)
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		f.asked = append(f.asked, title+"\n"+message)
		return f.answer, nil
	}}
	emit := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = emit })
	return &ScheduleAPI{
		app: app,
		findCLI: func(context.Context) (*schedules.CLI, error) {
			return &schedules.CLI{Path: "/usr/bin/agentry", Exec: f.exec}, nil
		},
		lingerOn: func() bool { return lingers },
		readyScheduleHosts: func(_ context.Context, s run.ScheduleHosts) (int, error) {
			f.readied = append(f.readied, fmt.Sprintf("%s %d", s.Ref, len(s.Given)))
			f.calls = append(f.calls, []string{"(ready hosts)"})
			if len(s.Given) > 0 && s.Before != nil {
				if err := s.Before(); err != nil {
					return 0, err
				}
			}
			return len(s.Given), f.readyErr
		},
	}
}

func scheduleReq(out string) schedules.Request {
	return schedules.Request{Registry: "https://r.example", Ref: "acme/digest@1.2.0", OutDir: out, MaxTurns: -1,
		When: schedules.When{Kind: "daily", Time: "09:00"}}
}

const readyCheck = `{"ref":"acme/digest","version":"1.2.0","ready":true,"gaps":[],"installed":true,"signedOut":{"available":true}}`

func TestScheduleAddTakesOnlyPickedPaths(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true}
	s := newScheduleAPI(t, f, true)
	out := t.TempDir()
	if _, err := s.Add(scheduleReq(out), nil); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("an unpicked output folder: %v", err)
	}
	s.app.markPicked(out)
	req := scheduleReq(out)
	req.Files = []string{"/etc/passwd"}
	if _, err := s.Check(req); err == nil {
		t.Fatal("an unpicked input was checked")
	}
	// A pack is named, never a folder: a path would reach the command's
	// pack-folder form with no picker behind it.
	req = scheduleReq(out)
	req.Knowledge = []string{"/home/me/secret-notes"}
	if _, err := s.Check(req); err == nil {
		t.Fatal("a knowledge path was checked")
	}
	req = scheduleReq(out)
	req.Registry = "http://r.example"
	if _, err := s.Check(req); err == nil {
		t.Fatal("a registry Settings would refuse was checked")
	}
	if len(f.calls) != 0 {
		t.Fatalf("the command ran: %q", f.calls)
	}
}

func TestScheduleAddAsksOnlyWhatItNeeds(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true}
	s := newScheduleAPI(t, f, true)
	out := t.TempDir()
	s.app.markPicked(out)

	// Nothing to consent to: no dialog and no --yes.
	made, err := s.Add(scheduleReq(out), nil)
	if err != nil || made.ID != "ab12cd34ef" {
		t.Fatalf("add %+v %v", made, err)
	}
	if len(f.asked) != 0 || slices.Contains(f.adds()[0], "--yes") {
		t.Fatalf("asked %q, add %q", f.asked, f.adds()[0])
	}

	// A version whose capabilities need consent, signed out with linger
	// off: one dialog naming both, then --yes.
	f.check = `{"ref":"acme/digest","version":"1.2.0","ready":true,"gaps":[],"needsConsent":true,"consentReason":"this version widens the declared capabilities","installed":true,"signedOut":{"available":true}}`
	s.lingerOn = func() bool { return false }
	req := scheduleReq(out)
	req.SignedOut = true
	if _, err := s.Add(req, nil); err != nil {
		t.Fatal(err)
	}
	if len(f.asked) != 1 || !strings.Contains(f.asked[0], "widens the declared capabilities") || !strings.Contains(f.asked[0], "start when this computer starts") {
		t.Fatalf("asked %q", f.asked)
	}
	if last := f.adds()[1]; !slices.Contains(last, "--yes") || !slices.Contains(last, "--signed-out") {
		t.Fatalf("add %q", last)
	}

	// The unsigned consent alone is the flag the run carries, never --yes.
	f.check = readyCheck
	s.lingerOn = func() bool { return true }
	req = scheduleReq(out)
	req.AllowUnsigned = true
	if _, err := s.Add(req, nil); err != nil {
		t.Fatal(err)
	}
	if last := f.adds()[2]; slices.Contains(last, "--yes") || !slices.Contains(last, "--allow-unsigned") || len(f.asked) != 2 {
		t.Fatalf("add %q, asked %d", last, len(f.asked))
	}

	// A No makes nothing.
	f.answer = false
	f.check = `{"ready":true,"gaps":[],"needsConsent":true,"installed":true,"signedOut":{"available":true}}`
	if _, err := s.Add(scheduleReq(out), nil); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("declined: %v", err)
	}
	if len(f.adds()) != 3 {
		t.Fatal("a declined consent made a schedule")
	}
}

func TestScheduleAddRefusesWhatAFireWouldLack(t *testing.T) {
	f := &scheduleFake{check: `{"ready":false,"gaps":["no model key is kept in the keychain: run ` + "`agentry setup`" + `"],"installed":true,"signedOut":{"available":true}}`, answer: true}
	s := newScheduleAPI(t, f, true)
	out := t.TempDir()
	s.app.markPicked(out)
	if _, err := s.Add(scheduleReq(out), nil); err == nil || !strings.Contains(err.Error(), "no model key") {
		t.Fatalf("add with a gap: %v", err)
	}
	if len(f.adds()) != 0 || len(f.asked) != 0 {
		t.Fatal("a schedule with a gap was made or asked about")
	}
}

func TestScheduleRemoveAsksFirst(t *testing.T) {
	f := &scheduleFake{answer: false}
	s := newScheduleAPI(t, f, true)
	if done, err := s.Remove("ab12cd34ef", "Morning digest"); err != nil || done || len(f.calls) != 0 {
		t.Fatalf("declined remove: %v %v %q", done, err, f.calls)
	}
	f.answer = true
	if done, err := s.Remove("ab12cd34ef", "Morning digest"); err != nil || !done || !slices.Contains(f.calls[0], "remove") {
		t.Fatalf("remove: %v %v %q", done, err, f.calls)
	}
}

// scheduledHome makes a runner home whose one schedule fired twice and
// missed once, and returns it.
func scheduledHome(t *testing.T) (string, string) {
	t.Helper()
	home := os.Getenv(policy.HomeEnv)
	st := schedule.StoreIn(home)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	sc := schedule.Schedule{Name: "Morning digest", Ref: "acme/digest", Version: "1.2.0", Digest: "sha256:aa",
		Run:     schedule.RunSpec{OutDir: t.TempDir(), MaxTurns: -1, MaxCost: -1},
		When:    schedule.Recurrence{Kind: schedule.Daily, Time: "09:00", Zone: "UTC", Start: start, End: schedule.End{Kind: schedule.EndNever}},
		Created: start}
	if err := st.Create(&sc, nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/result.json", []byte(`{"result":"hello"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	for _, fire := range []schedule.Fire{
		{Occurrence: day(6), Outcome: schedule.Ran, RunID: "a1b2c3d4e5f60718", OutDir: dir, Started: day(6), Ended: day(6).Add(time.Minute)},
		{Occurrence: day(7), Outcome: schedule.Missed},
		{Occurrence: day(8), Outcome: schedule.Failed, ExitCode: 13, ExitClass: "docker", RunID: "b1b2c3d4e5f60718", Started: day(8), Ended: day(8)},
	} {
		if err := st.AppendFire(sc.ID, fire); err != nil {
			t.Fatal(err)
		}
	}
	return home, dir
}

func TestScheduledFiresAreInRecentRunsAndOpen(t *testing.T) {
	freshConfig(t)
	_, dir := scheduledHome(t)
	// With Remember recent runs off, the schedule's own record still
	// shows: the app keeps nothing of it.
	setRemember(t, false)
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.RecentRuns("", 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range v.Runs {
		got = append(got, fmt.Sprintf("%s %s %v %s", row.ID, row.Outcome, row.Scheduled, row.Schedule))
	}
	want := []string{"b1b2c3d4e5f60718 error true Morning digest", "a1b2c3d4e5f60718 ok true Morning digest"}
	if !slices.Equal(got, want) {
		t.Fatalf("rows %q, want %q (a missed occurrence ran nothing and is no row)", got, want)
	}
	rec, err := r.RecordedRun("a1b2c3d4e5f60718")
	if err != nil || rec.FolderState != folderPresent || rec.ResultJSON != `"hello"` || !rec.Run.Scheduled || rec.Folder != dir {
		t.Fatalf("recorded %+v %v", rec, err)
	}
	if folder, err := r.folderOf("a1b2c3d4e5f60718"); err != nil || folder != dir {
		t.Fatalf("folder %q %v", folder, err)
	}
	if _, err := r.RecordedRun("c1b2c3d4e5f60718"); !errors.Is(err, errUnknownRun) {
		t.Fatalf("an unknown id: %v", err)
	}
}

func TestScheduleCalendarAndList(t *testing.T) {
	f := &scheduleFake{}
	s := newScheduleAPI(t, f, true)
	scheduledHome(t)
	s.now = func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }
	l, err := s.List()
	if err != nil || len(l.Schedules) != 1 || l.Schedules[0].Next != "2026-10-10T09:00:00Z" {
		t.Fatalf("list %+v %v", l, err)
	}
	cal, err := s.Calendar("2026-10-01T00:00:00Z", "2026-11-01T00:00:00Z")
	if err != nil || cal[0].Outcome != "ran" || cal[1].Outcome != "missed" || cal[2].Outcome != "failed" || cal[3].Outcome != "due" {
		t.Fatalf("calendar %+v %v", cal, err)
	}
	if _, err := s.Calendar("2026-01-01T00:00:00Z", "2026-12-01T00:00:00Z"); err == nil {
		t.Fatal("a year was read at once")
	}
	h, err := s.History(1)
	if err != nil || len(h) != 1 || h[0].Outcome != "failed" {
		t.Fatalf("history %+v %v", h, err)
	}
}

func TestResealAfterAKeyChange(t *testing.T) {
	freshConfig(t)
	var events []string
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) { events = append(events, fmt.Sprint(name, data)) }
	t.Cleanup(func() { emitEvent = emit })
	saved := resealCLI
	t.Cleanup(func() { resealCLI = saved; resealProblem = "" })
	called := 0
	resealCLI = func(context.Context) (*schedules.CLI, error) {
		called++
		return &schedules.CLI{Path: "/usr/bin/agentry", Exec: func(context.Context, string, []string) ([]byte, []byte, error) {
			return []byte(`{"sealed":[],"failed":[{"id":"ab12cd34ef","name":"Morning digest","error":"the OS keychain has no entry provider:openai"}]}`), nil, errors.New("exit status 13")
		}}, nil
	}
	// No schedule runs signed out: nothing is run.
	resealNow(&App{ctx: context.Background()})
	if called != 0 {
		t.Fatal("a re-seal ran with nothing to seal")
	}
	home := os.Getenv(policy.HomeEnv)
	sc := schedule.Schedule{Name: "Morning digest", Ref: "acme/digest", Version: "1.2.0", Digest: "sha256:aa", SignedOut: true,
		Run:  schedule.RunSpec{OutDir: t.TempDir(), MaxTurns: -1, MaxCost: -1},
		When: schedule.Recurrence{Kind: schedule.Daily, Time: "09:00", Zone: "UTC", Start: time.Now(), End: schedule.End{Kind: schedule.EndNever}}}
	if err := schedule.StoreIn(home).Create(&sc, nil); err != nil {
		t.Fatal(err)
	}
	resealNow(&App{ctx: context.Background()})
	if called != 1 || !strings.Contains(resealProblem, "Morning digest") || !strings.Contains(strings.Join(events, "\n"), eventSchedulesReseal) {
		t.Fatalf("called %d, problem %q, events %q", called, resealProblem, events)
	}
	l, _ := (&ScheduleAPI{app: &App{ctx: context.Background()}}).List()
	if l.ResealProblem == "" || l.Schedules[0].KeysSealed {
		t.Fatalf("list %+v", l)
	}
}

// Home's Scheduled card: the month as the page's calendar reads it, the next
// three occurrences however far the month view reaches, and the reason when
// the agentry command is not there, found on disk without running anything.
func TestScheduledHomeCard(t *testing.T) {
	f := &scheduleFake{}
	s := newScheduleAPI(t, f, true)
	scheduledHome(t)
	s.now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	s.locateCLI = func() (string, error) { return "/usr/bin/agentry", nil }
	v, err := s.HomeCard("2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Schedules != 1 || v.Reason != "" || len(v.Month) != 3+2 || v.Month[0].Outcome != "ran" || v.Month[4].At != "2026-11-01T09:00:00Z" {
		t.Fatalf("card %+v", v)
	}
	var next []string
	for _, n := range v.Next {
		next = append(next, n.At+" "+n.Outcome)
	}
	if got, want := strings.Join(next, ","), "2026-10-31T09:00:00Z due,2026-11-01T09:00:00Z due,2026-11-02T09:00:00Z due"; got != want {
		t.Fatalf("next = %s, want %s", got, want)
	}
	if len(f.calls) != 0 {
		t.Fatalf("the card ran the agentry command: %v", f.calls)
	}
	s.locateCLI = func() (string, error) { return "", schedules.ErrNoCLI }
	if v, err = s.HomeCard("2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z", ""); err != nil || v.Reason != schedules.ErrNoCLI.Error() {
		t.Fatalf("no command: %+v %v", v, err)
	}
	if _, err := s.HomeCard("2026-01-01T00:00:00Z", "2026-12-01T00:00:00Z", ""); err == nil {
		t.Fatal("a year was read at once")
	}
}

// An empty runner home is an empty card, not an error.
func TestScheduledHomeCardEmpty(t *testing.T) {
	s := newScheduleAPI(t, &scheduleFake{}, false)
	s.locateCLI = func() (string, error) { return "/usr/bin/agentry", nil }
	v, err := s.HomeCard("2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z", "")
	if err != nil || v.Schedules != 0 || len(v.Month) != 0 || v.Next == nil || len(v.Next) != 0 {
		t.Fatalf("empty card %+v %v", v, err)
	}
}

// A schedule's target hosts are readied with the person here, after the
// consent and before the agentry command makes it, with the sign-ins the
// setup gave: the command then finds the key accepted and the sign-in kept.
// A host that cannot be readied makes no schedule.
func TestScheduleAddReadiesTargetHostsBeforeTheCommand(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true}
	s := newScheduleAPI(t, f, true)
	out := t.TempDir()
	s.app.markPicked(out)
	given := map[string]run.HostSignIn{"server": {Password: "pw", Save: true}}
	if _, err := s.Add(scheduleReq(out), given); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.readied, []string{"acme/digest@1.2.0 1"}) {
		t.Fatalf("readied %q", f.readied)
	}
	i := slices.IndexFunc(f.calls, func(c []string) bool { return c[0] == "(ready hosts)" })
	j := slices.IndexFunc(f.calls, func(c []string) bool { return slices.Contains(c, "add") && !slices.Contains(c, "--check") })
	if i < 0 || j < 0 || i > j {
		t.Fatalf("hosts readied at %d, schedule added at %d: %q", i, j, f.calls)
	}

	f.readyErr = errors.New("target host server: the host key was not accepted")
	if _, err := s.Add(scheduleReq(out), given); err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Fatalf("a host not readied: %v", err)
	}
	if len(f.adds()) != 1 {
		t.Fatalf("a schedule was made for a host not readied: %q", f.adds())
	}
}

// Only an agentry that readies target hosts (v0.6.111) makes a schedule of a
// QuickAgent that asks for a server: an older one is refused before any host
// is readied or any sign-in kept.
func TestScheduleAddOfATargetHostNeedsANewEnoughAgentry(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true, version: "agentry version 0.6.110 default registry: https://secureagentry.net"}
	s := newScheduleAPI(t, f, true)
	out := t.TempDir()
	s.app.markPicked(out)
	given := map[string]run.HostSignIn{"server": {Password: "pw"}}
	if _, err := s.Add(scheduleReq(out), given); err == nil || !strings.Contains(err.Error(), "needs agentry v0.6.111 or later") {
		t.Fatalf("an older agentry: %v", err)
	}
	if len(f.adds()) != 0 {
		t.Fatal("a schedule was made by an agentry that does not know target hosts")
	}
	f.version = "agentry version 0.6.111 default registry: https://secureagentry.net"
	if _, err := s.Add(scheduleReq(out), given); err != nil {
		t.Fatal(err)
	}
}

// TestAScheduleDeliversToTheDeliverablesFolderAndTheChosenDestinations: the
// folder is the app's own setting, never the page's, and a destination the
// setup chose is held to the CLI's rule and to a command that takes it.
func TestAScheduleDeliversToTheDeliverablesFolderAndTheChosenDestinations(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true, version: "agentry version 0.6.139 default"}
	s := newScheduleAPI(t, f, true)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	if _, err := (&DestinationsAPI{app: s.app}).Save(destinations.Input{Name: "n8n", URL: "https://hooks.example.com/x", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	s.app.markPicked(out)
	req := scheduleReq(out)
	req.DeliverTo = "/etc"
	req.Destinations = []string{"n8n"}
	if _, err := s.Add(req, nil); err != nil {
		t.Fatal(err)
	}
	adds := f.adds()
	if len(adds) != 1 {
		t.Fatalf("adds %q", adds)
	}
	root, _ := defaultDeliverablesRoot()
	j := strings.Join(adds[0], " ")
	if !strings.Contains(j, "--deliver-to "+root) || strings.Contains(j, "/etc") || !strings.Contains(j, "--destination n8n") {
		t.Errorf("add: %s", j)
	}
	// A destination that is not saved refuses before the command runs.
	f.calls = nil
	req.Destinations = []string{"nope"}
	if _, err := s.Add(req, nil); err == nil || len(f.adds()) != 0 {
		t.Errorf("an unsaved destination: %v %q", err, f.adds())
	}
}

// TestAScheduleNeedsAnAgentryThatKeepsWhereItsFiresDeliver: an older command is
// refused for a destination somebody chose, and passed no folder otherwise.
func TestAScheduleNeedsAnAgentryThatKeepsWhereItsFiresDeliver(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true, version: "agentry version 0.6.138 default"}
	s := newScheduleAPI(t, f, true)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	if _, err := (&DestinationsAPI{app: s.app}).Save(destinations.Input{Name: "n8n", URL: "https://hooks.example.com/x", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	s.app.markPicked(out)
	req := scheduleReq(out)
	req.Destinations = []string{"n8n"}
	if _, err := s.Add(req, nil); err == nil || !strings.Contains(err.Error(), "needs agentry v0.6.139") {
		t.Fatalf("an old command with a destination: %v", err)
	}
	req.Destinations = nil
	if _, err := s.Add(req, nil); err != nil {
		t.Fatal(err)
	}
	if adds := f.adds(); len(adds) != 1 || strings.Contains(strings.Join(adds[0], " "), "--deliver-to") {
		t.Errorf("an old command was passed the folder: %q", adds)
	}
}

// TestAScheduleThatSendsToABucketNeedsAnAgentryThatKnowsBuckets: an older command
// refuses a destinations file holding a bucket as a whole, so no schedule is made
// for a bucket with one, and a webhook still is.
func TestAScheduleThatSendsToABucketNeedsAnAgentryThatKnowsBuckets(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true, version: "agentry version 0.6.139 default"}
	s := newScheduleAPI(t, f, true)
	keyring.MockInit()
	t.Setenv(modelkey.EnvNoKeyring, "")
	api := &DestinationsAPI{app: s.app}
	if _, err := api.Save(destinations.Input{Name: "n8n", URL: "https://hooks.example.com/x", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Save(destinations.Input{Name: "archive", Kind: "s3", Bucket: "my-reports", Region: "eu-west-1", AccessKeyID: "AKIAKEYID1234", Secret: "SECRET"}); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	s.app.markPicked(out)
	req := scheduleReq(out)
	req.Destinations = []string{"archive"}
	if _, err := s.Add(req, nil); err == nil || !strings.Contains(err.Error(), "needs agentry v0.6.140") || len(f.adds()) != 0 {
		t.Fatalf("an old command with a bucket: %v %q", err, f.adds())
	}
	req.Destinations = []string{"n8n"}
	if _, err := s.Add(req, nil); err != nil {
		t.Fatalf("a webhook with the same command: %v", err)
	}
	f.version = "agentry version 0.6.140-dev.2 default"
	f.calls = nil
	req.Destinations = []string{"archive"}
	if _, err := s.Add(req, nil); err != nil {
		t.Fatalf("a bucket with a command that knows it: %v", err)
	}
	if adds := f.adds(); len(adds) != 1 || !strings.Contains(strings.Join(adds[0], " "), "--destination archive") {
		t.Errorf("adds %q", adds)
	}
}
