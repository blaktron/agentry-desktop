package schedules

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/schedule"
)

func TestWhenArgs(t *testing.T) {
	for _, c := range []struct {
		w    When
		want string
	}{
		{When{Kind: "once", Date: "2026-10-20", Time: "08:00"}, "--once|2026-10-20 08:00"},
		{When{Kind: "daily", Time: "09:00", End: "never", Zone: "Europe/London"}, "--daily|09:00|--zone|Europe/London"},
		{When{Kind: "weekdays", Time: "07:30", End: "after", Times: 5}, "--weekdays|07:30|--times|5"},
		{When{Kind: "weekly", Days: []string{"thu", "Tuesday"}, Time: "22:00", End: "on", Until: "2026-12-31"}, "--weekly|tue,thu|--at|22:00|--until|2026-12-31"},
		{When{Kind: "monthly", MonthDay: 31, Time: "06:00"}, "--monthly|31|--at|06:00"},
		// A once schedule has no end, whatever the form held.
		{When{Kind: "once", Date: "2026-10-20", Time: "08:00", End: "after", Times: 3}, "--once|2026-10-20 08:00"},
	} {
		got, err := c.w.Args()
		if err != nil {
			t.Fatalf("%+v: %v", c.w, err)
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%+v = %q, want %q", c.w, strings.Join(got, "|"), c.want)
		}
	}
	for _, w := range []When{
		{Kind: "daily", Time: "9am"},
		{Kind: "once", Time: "09:00", Date: "20/10/2026"},
		{Kind: "weekly", Time: "09:00"},
		{Kind: "monthly", Time: "09:00", MonthDay: 32},
		{Kind: "hourly", Time: "09:00"},
		{Kind: "daily", Time: "09:00", End: "after"},
		{Kind: "daily", Time: "09:00", End: "on", Until: "soon"},
		{Kind: "daily", Time: "09:00", Zone: "Europe/London; rm -rf"},
		{Kind: "daily", Time: "09:00", Zone: "--registry"},
	} {
		if _, err := w.Args(); err == nil {
			t.Errorf("%+v was accepted", w)
		}
	}
}

func TestAddArgs(t *testing.T) {
	r := Request{
		Registry: "https://dev.secureagentry.net", Ref: "acme/digest@1.2.0", OutDir: "/home/p/reports",
		Task: "Today's summary", Files: []string{"/home/p/a.txt"}, Knowledge: []string{"handbook"},
		Intake: map[string]string{"team": "core", "audience": "ops"}, Voice: true, TimeoutSeconds: 600, MaxTurns: -1,
		AllowUnsigned: true, SignedOut: true, When: When{Kind: "daily", Time: "09:00"},
	}
	got, err := r.AddArgs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"schedule", "add", "acme/digest@1.2.0", "--out", "/home/p/reports", "--daily", "09:00",
		"--task", "Today's summary", "--file", "/home/p/a.txt", "--knowledge", "handbook",
		"--intake", "audience=ops", "--intake", "team=core", "--voice", "--timeout", "600",
		"--allow-unsigned", "--signed-out"}
	if !slices.Equal(got, want) {
		t.Fatalf("args\n got %q\nwant %q", got, want)
	}
	// The voice left off is said, so the policy's default does not decide;
	// a turn budget of 0 (unlimited) is passed.
	r.Voice, r.MaxTurns, r.SignedOut = false, 0, false
	got, _ = r.AddArgs()
	if !slices.Contains(got, "--no-voice") || !slices.Contains(got, "--max-turns") || slices.Contains(got, "--signed-out") {
		t.Fatalf("args %q", got)
	}
	for _, bad := range []Request{
		{Ref: "acme/digest", OutDir: "/o", When: When{Kind: "daily", Time: "09:00"}},
		{Ref: "--yes@1", OutDir: "/o", When: When{Kind: "daily", Time: "09:00"}},
		{Ref: "acme/digest@1.0.0", When: When{Kind: "daily", Time: "09:00"}},
		{Ref: "acme/digest@1.0.0", OutDir: "/o", When: When{Kind: "daily", Time: "09:00"}, Intake: map[string]string{"--yes x": "1"}},
	} {
		if _, err := bad.AddArgs(); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

// fakeCLI records each call and answers from a table by the subcommand.
type fakeCLI struct {
	calls   [][]string
	answers map[string]string
	fail    map[string]string
}

func (f *fakeCLI) exec(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	key := ""
	for i, a := range args {
		if a == "schedule" && i+1 < len(args) {
			key = args[i+1]
			break
		}
	}
	if slices.Contains(args, "--check") {
		key = "check"
	}
	if said, ok := f.fail[key]; ok {
		return []byte(f.answers[key]), []byte("installing images…\nagentry: warning: an old config\nagentry: " + said + "\n  - the model key\n"), errors.New("exit status 10")
	}
	return []byte(f.answers[key]), nil, nil
}

func TestCLICalls(t *testing.T) {
	f := &fakeCLI{answers: map[string]string{
		"check":            `{"ref":"acme/digest","version":"1.2.0","ready":false,"gaps":["no model key is kept in the keychain: run ` + "`agentry setup`" + `"],"needsConsent":true,"consentReason":"this version widens the declared capabilities","installed":true,"signedOut":{"available":true}}`,
		"add":              `{"id":"ab12cd34ef"}`,
		"check-signed-out": `{"available":false,"reason":"--signed-out is not available on windows: Docker Desktop stops when you sign out"}`,
		"reseal":           `{"sealed":["ab12cd34ef"],"failed":[{"id":"0011aabbcc","name":"digest","error":"the OS keychain has no entry provider:openai"}]}`,
	}}
	c := &CLI{Path: "/usr/bin/agentry", Exec: f.exec}
	r := Request{Registry: "https://r.example", Ref: "acme/digest@1.2.0", OutDir: "/o", MaxTurns: -1, When: When{Kind: "daily", Time: "09:00"}}
	ctx := context.Background()

	ck, err := c.Check(ctx, r)
	if err != nil || ck.Ready || len(ck.Gaps) != 1 || !ck.NeedsConsent || !ck.Installed {
		t.Fatalf("check %+v, %v", ck, err)
	}
	if last := f.calls[0]; last[len(last)-1] != "--check" || !slices.Contains(last, "--json") || slices.Contains(last, "--yes") {
		t.Fatalf("check call %q", last)
	}
	// Consent is passed only when the person gave it in the app.
	if _, err := c.Add(ctx, r, false); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.calls[1], "--yes") || slices.Contains(f.calls[1], "--check") {
		t.Fatalf("add call %q", f.calls[1])
	}
	id, err := c.Add(ctx, r, true)
	if err != nil || id != "ab12cd34ef" || !slices.Contains(f.calls[2], "--yes") {
		t.Fatalf("add %q, %v, call %q", id, err, f.calls[2])
	}
	if i := slices.Index(f.calls[2], "--registry"); i < 0 || f.calls[2][i+1] != "https://r.example" {
		t.Fatalf("registry not passed: %q", f.calls[2])
	}

	so, err := c.SignedOutHere(ctx)
	if err != nil || so.Available || !strings.Contains(so.Reason, "Docker Desktop") {
		t.Fatalf("signed out %+v, %v", so, err)
	}
	// A reseal with a failure exits 1 and still answers which.
	f.fail = map[string]string{"reseal": "the keys of 1 schedule(s) were not sealed again"}
	rs, err := c.Reseal(ctx)
	if err != nil || len(rs.Sealed) != 1 || len(rs.Failed) != 1 || rs.Failed[0].Name != "digest" {
		t.Fatalf("reseal %+v, %v", rs, err)
	}

	// A refusal comes back in the command's own words, without its
	// progress lines.
	f.fail = map[string]string{"add": "a scheduled run of acme/digest would not have what it needs"}
	_, err = c.Add(ctx, r, true)
	var ce *Error
	if !errors.As(err, &ce) || !strings.HasPrefix(err.Error(), "a scheduled run of acme/digest would not have what it needs\n  - the model key") || strings.Contains(err.Error(), "installing") {
		t.Fatalf("refusal %v", err)
	}

	// Actions take only a schedule id and a known verb.
	if err := c.Act(ctx, "pause", "ab12cd34ef"); err != nil {
		t.Fatal(err)
	}
	if err := c.Act(ctx, "fire", "ab12cd34ef"); err == nil {
		t.Fatal("fire was run from the app")
	}
	if err := c.Act(ctx, "remove", "../x"); err == nil {
		t.Fatal("a path was taken for an id")
	}
	on := true
	if err := c.EditSchedule(ctx, "ab12cd34ef", Edit{SignedOut: &on}, true); err != nil {
		t.Fatal(err)
	}
	if last := f.calls[len(f.calls)-1]; !slices.Contains(last, "--signed-out=true") || !slices.Contains(last, "--yes") {
		t.Fatalf("edit call %q", last)
	}
	if err := c.EditSchedule(ctx, "ab12cd34ef", Edit{}, false); err == nil {
		t.Fatal("an empty edit ran")
	}
}

func TestTooOld(t *testing.T) {
	for line, old := range map[string]bool{
		"agentry version 0.6.85":       true,
		"agentry version 0.6.86-dev.1": false,
		"agentry version 0.6.86":       false,
		"agentry version 0.7.0":        false,
		"agentry version 0.5.99":       true,
		"agentry version 1.0.0":        false,
		"agentry version (devel)":      false,
	} {
		if got := TooOld(line) != ""; got != old {
			t.Errorf("TooOld(%q) = %v, want %v", line, got, old)
		}
	}
}

func TestFindCLI(t *testing.T) {
	savedOS, savedExe, savedLook, savedStat := goos, executable, lookPath, statFile
	t.Cleanup(func() { goos, executable, lookPath, statFile = savedOS, savedExe, savedLook, savedStat })
	have := map[string]bool{}
	statFile = func(p string) (fs.FileInfo, error) {
		if have[p] {
			return os.Stat(os.Args[0])
		}
		return nil, fs.ErrNotExist
	}
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	executable = func() (string, error) { return filepath.Join("C:", "Agentry", "agentry-desktop.exe"), nil }

	goos = "linux"
	if _, err := FindCLI(); !errors.Is(err, ErrNoCLI) {
		t.Fatalf("no CLI: %v", err)
	}
	have[linuxPackageCLI] = true
	if p, err := FindCLI(); err != nil || p != linuxPackageCLI {
		t.Fatalf("linux %q %v", p, err)
	}
	goos = "windows"
	beside := filepath.Join("C:", "Agentry", "bin", "agentry.exe")
	have[beside] = true
	if p, err := FindCLI(); err != nil || p != beside {
		t.Fatalf("windows %q %v", p, err)
	}
	goos = "darwin"
	if _, err := FindCLI(); err == nil || !strings.Contains(err.Error(), "macOS") {
		t.Fatalf("darwin %v", err)
	}
}

// storeWith makes a runner home holding a daily schedule, made a week
// before now, with three fires recorded.
func storeWith(t *testing.T, now time.Time) (Store, schedule.Schedule) {
	t.Helper()
	home := t.TempDir()
	s := schedule.StoreIn(home)
	sc := schedule.Schedule{
		Name: "Morning digest", Ref: "acme/digest", Version: "1.2.0", Digest: "sha256:aa",
		Run:     schedule.RunSpec{OutDir: t.TempDir(), MaxTurns: -1, MaxCost: -1},
		When:    schedule.Recurrence{Kind: schedule.Daily, Time: "09:00", Zone: "UTC", Start: now.AddDate(0, 0, -7), End: schedule.End{Kind: schedule.EndNever}},
		Created: now.AddDate(0, 0, -7),
	}
	if err := s.Create(&sc, nil); err != nil {
		t.Fatal(err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	for _, f := range []schedule.Fire{
		{Occurrence: day(6), Outcome: schedule.Ran, RunID: "run-0006", OutDir: "/o/6", Started: day(6), Ended: day(6).Add(time.Minute)},
		{Occurrence: day(7), Outcome: schedule.Missed},
		{Occurrence: day(8), Outcome: schedule.Failed, ExitCode: 13, ExitClass: "docker", Message: "docker is not running", RunID: "run-0008"},
	} {
		if err := s.AppendFire(sc.ID, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveState(sc.ID, schedule.State{Last: day(8), Runs: 2}); err != nil {
		t.Fatal(err)
	}
	return StoreIn(home), sc
}

func TestStoreReadsTheCLIStore(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	st, sc := storeWith(t, now)

	items, err := st.List(now)
	if err != nil || len(items) != 1 {
		t.Fatalf("list %+v, %v", items, err)
	}
	it := items[0]
	if it.ID != sc.ID || it.Next != "2026-10-10T09:00:00Z" || it.Last == nil || it.Last.Outcome != Failed || it.Runs != 2 || it.When.Kind != "daily" {
		t.Fatalf("item %+v", it)
	}

	// The month: three past fires as they happened, the rest due.
	from, to := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cal, err := st.Between(from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	var outcomes []string
	for _, f := range cal {
		outcomes = append(outcomes, f.At[8:10]+":"+f.Outcome)
	}
	if outcomes[0] != "06:ran" || outcomes[1] != "07:missed" || outcomes[2] != "08:failed" || outcomes[3] != "10:due" || outcomes[len(outcomes)-1] != "31:due" {
		t.Fatalf("calendar %q", outcomes)
	}
	// Nothing reads as done when nothing ran: no fire for the 9th yet.
	for _, o := range outcomes {
		if strings.HasPrefix(o, "09:") {
			t.Fatalf("the 9th shows %q with no fire recorded and its time past", o)
		}
	}

	fires, _ := st.Fires(2)
	if len(fires) != 2 || fires[0].Outcome != Failed || fires[0].ExitCode != 13 {
		t.Fatalf("fires %+v", fires)
	}
	if f, ok := st.FindRun("run-0006"); !ok || f.OutDir != "/o/6" {
		t.Fatalf("find run %+v %v", f, ok)
	}
	if _, ok := st.FindRun(""); ok {
		t.Fatal("an empty run id found a fire")
	}

	// A paused schedule has nothing due; its past stays.
	sc.Paused = true
	if err := st.s.Save(sc); err != nil {
		t.Fatal(err)
	}
	cal, _ = st.Between(from, to, now)
	if len(cal) != 3 {
		t.Fatalf("paused calendar %d entries", len(cal))
	}
	if st.SignedOutLive() {
		t.Fatal("no schedule runs signed out")
	}
}

// A repeat that ends after N runs spends them from now on, whichever
// month the calendar shows.
func TestARepeatThatEndsShowsOnlyItsRunsLeft(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	home := t.TempDir()
	s := schedule.StoreIn(home)
	sc := schedule.Schedule{Name: "three", Ref: "acme/x", Version: "1.0.0", Digest: "sha256:aa",
		Run:     schedule.RunSpec{OutDir: t.TempDir(), MaxTurns: -1, MaxCost: -1},
		When:    schedule.Recurrence{Kind: schedule.Daily, Time: "09:00", Zone: "UTC", Start: now, End: schedule.End{Kind: schedule.EndAfter, After: 3}},
		Created: now}
	if err := s.Create(&sc, nil); err != nil {
		t.Fatal(err)
	}
	st := StoreIn(home)
	oct, _ := st.Between(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), now)
	nov, _ := st.Between(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), now)
	if len(oct) != 2 || len(nov) != 1 || nov[0].At != "2026-11-01T09:00:00Z" {
		t.Fatalf("october %d, november %+v", len(oct), nov)
	}
	// A month far ahead still counts from now.
	later, _ := st.Between(time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC), now)
	if len(later) != 0 {
		t.Fatalf("march 2027 shows %d", len(later))
	}
}

func TestAnEmptyRunnerHomeHasNoSchedules(t *testing.T) {
	st := StoreIn(t.TempDir())
	if items, err := st.List(time.Now()); err != nil || len(items) != 0 {
		t.Fatalf("%v %v", items, err)
	}
	if fires, err := st.Fires(10); err != nil || len(fires) != 0 {
		t.Fatalf("%v %v", fires, err)
	}
	if st.Any() {
		t.Fatal("an empty home has a schedule")
	}
}
