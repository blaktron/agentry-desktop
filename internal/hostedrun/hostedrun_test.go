package hostedrun

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The whole account-side lifecycle against the fake extension: the status
// route's caller block, a create, the run watched from queued through
// running to its finish, and the pushed run's one output.tar unpacked into
// the chosen folder (plan §4.1, §4.4; hosted runs in production M3,
// agentry-desktop#182). The run's events route is never read: a pushed run
// keeps no log.
func TestCreateWatchFinishAndDownload(t *testing.T) {
	f := &fakeRuns{
		enabled: true, workersOnline: 0, queued: 1, pushed: true,
		caller: &Caller{Tier: "standard", Caps: Caps{TimeoutSeconds: 300, MaxTurns: 20, MaxTokens: 20000}, HoldCredits: 20, TokensPerCredit: 1000},
		outputs: map[string][]byte{
			"result.json":         []byte(`"the incident summary"`),
			"reports/report.md":   []byte("# Report\n"),
			"receipts/run.json":   []byte(`{"agentry":"1"}`),
			"outputs/summary.txt": []byte("done"),
		},
	}
	base := f.start(t)
	c := New(base, testToken)
	ctx := t.Context()

	st, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Caller == nil || st.Caller.HoldCredits != 20 || st.Caller.Caps.MaxTokens != 20000 {
		t.Fatalf("status = %+v (caller %+v)", st, st.Caller)
	}

	run, err := c.Create(ctx, CreateRequest{Ref: "acme/reporter@1.0.0", Task: "summarise", Intake: map[string]string{"since": "yesterday"}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusQueued || run.QueuePosition != 2 || run.HoldCredits != 20 {
		t.Fatalf("created run = %+v", run)
	}

	// Queued second, then first, then a runner takes it and the agent
	// starts, then it finishes. A read that says nothing new is not
	// reported twice.
	f.mu.Lock()
	f.steps = []func(*Run){
		step(StatusQueued, 2), step(StatusQueued, 1), step(StatusQueued, 1),
		step(StatusClaimed, 0), step(StatusRunning, 0),
	}
	f.mu.Unlock()
	var seen []string
	final, err := c.Watch(ctx, run.ID, WatchOptions{
		Poll: 5 * time.Millisecond,
		OnStatus: func(r *Run) {
			seen = append(seen, r.Status+":"+strconv.Itoa(r.QueuePosition))
			if r.Status == StatusRunning {
				f.finish(OutcomeSucceeded, "ok", 2)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"queued:2", "queued:1", "claimed:0", "running:0", "finished:0"}
	if strings.Join(seen, " ") != strings.Join(want, " ") {
		t.Fatalf("statuses = %v, want %v", seen, want)
	}
	if final.Outcome != OutcomeSucceeded || final.ExitClass != "ok" || final.Usage.Tokens() != 1200 {
		t.Fatalf("final = %+v usage %+v", final, final.Usage)
	}
	if final.SettledCredits == nil || *final.SettledCredits != 2 {
		t.Fatalf("settlement = %v", final.SettledCredits)
	}
	if got := final.Receipt.CapabilitiesEnforced["network.egress"]; got != "llm-proxy allowlist: api.example.com" {
		t.Fatalf("capabilities_enforced = %v", final.Receipt.CapabilitiesEnforced)
	}
	if v := Read(final); v.Tone != "ok" || !v.Settled {
		t.Fatalf("verdict = %+v", v)
	}

	// The bundle is unpacked under the chosen folder, folders and all;
	// the bundle itself is not kept.
	dir := t.TempDir()
	got, err := c.DownloadOutputs(ctx, final, dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range got {
		names = append(names, a.Name)
		if !strings.HasPrefix(a.Path, dir+string(filepath.Separator)) {
			t.Fatalf("%s landed outside the run folder: %s", a.Name, a.Path)
		}
		if _, err := os.Stat(a.Path); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(names, " ") != "outputs/summary.txt receipts/run.json reports/report.md result.json" {
		t.Fatalf("unpacked %v", names)
	}
	body, err := os.ReadFile(filepath.Join(dir, "reports", "report.md"))
	if err != nil || string(body) != "# Report\n" {
		t.Fatalf("report.md = %q (%v)", body, err)
	}
	if _, err := os.Stat(filepath.Join(dir, PushedOutput)); err == nil {
		t.Fatal("the bundle itself was left in the run folder")
	}
	if f.eventsRead != 0 {
		t.Fatalf("the client read the events route %d time(s)", f.eventsRead)
	}
}

// A bundle longer than the sealed object the registry reported is cut
// there and refused, so a registry that streams without end cannot fill
// the disk.
func TestUnpackStopsAtTheReportedSize(t *testing.T) {
	f := &fakeRuns{enabled: true, pushed: true, outputs: map[string][]byte{"result.json": bytes.Repeat([]byte("x"), 64<<10)}}
	c := New(f.start(t), testToken)
	run := &Run{ID: "r1", Outputs: []Artefact{{Name: PushedOutput, Bytes: 4096}}}
	got, err := c.DownloadOutputs(t.Context(), run, t.TempDir())
	if err == nil {
		t.Fatalf("a bundle past its reported size was accepted: %+v", got)
	}
}

// A pull worker's run lists its outputs file by file; they come down as
// they are, into the same folder shape a pushed run's bundle makes.
func TestDownloadAPulledRunsOutputs(t *testing.T) {
	f := &fakeRuns{enabled: true, outputs: map[string][]byte{
		"result.json":       []byte(`"ok"`),
		"reports/report.md": []byte("# Report\n"),
	}}
	c := New(f.start(t), testToken)
	run, err := c.Create(t.Context(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	f.finish(OutcomeSucceeded, "ok", 1)
	final, err := c.Watch(t.Context(), run.ID, WatchOptions{Poll: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	got, err := c.DownloadOutputs(t.Context(), final, dir)
	if err != nil || len(got) != 2 || got[0].Name != "reports/report.md" || got[1].Name != "result.json" {
		t.Fatalf("downloaded %+v (%v)", got, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "reports", "report.md")); err != nil || string(b) != "# Report\n" {
		t.Fatalf("report.md = %q (%v)", b, err)
	}
}

// Watch gives up only after its retries: a registry that answers errors
// for longer than that ends the watch with the error, and a cancelled
// context ends it at once.
func TestWatchGivesUpAndStops(t *testing.T) {
	c := New("http://127.0.0.1:1", testToken)
	if _, err := c.Watch(t.Context(), "r1", WatchOptions{Poll: time.Millisecond, Retries: 2}); err == nil {
		t.Fatal("a registry that never answers did not end the watch")
	}
	f := &fakeRuns{enabled: true}
	c = New(f.start(t), testToken)
	run, err := c.Create(t.Context(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if _, err := c.Watch(ctx, run.ID, WatchOptions{Poll: time.Millisecond}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled watch = %v", err)
	}
}

// A bundle no host would write is refused, not half-applied: a link, a
// name that leaves the folder, a name twice.
func TestUnpackRefusesWhatAHostNeverWrites(t *testing.T) {
	for name, entries := range map[string][]tar.Header{
		"a symlink":          {{Name: "outputs/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}},
		"an escape":          {{Name: "../escaped.txt", Typeflag: tar.TypeReg, Size: 1}},
		"an absolute":        {{Name: "/tmp/abs.txt", Typeflag: tar.TypeReg, Size: 1}},
		"a name twice":       {{Name: "result.json", Typeflag: tar.TypeReg, Size: 1}, {Name: "./result.json", Typeflag: tar.TypeReg, Size: 1}},
		"a directory":        {{Name: "outputs/", Typeflag: tar.TypeDir}},
		"a hard link":        {{Name: "outputs/hard", Typeflag: tar.TypeLink, Linkname: "result.json"}},
		"a backslash escape": {{Name: "..\\escaped.txt", Typeflag: tar.TypeReg, Size: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			for i := range entries {
				h := entries[i]
				h.Mode = 0o600
				if err := tw.WriteHeader(&h); err != nil {
					t.Fatal(err)
				}
				if h.Size > 0 {
					_, _ = tw.Write([]byte("x"))
				}
			}
			_ = tw.Close()
			f := &fakeRuns{enabled: true, pushed: true, bundle: buf.Bytes()}
			c := New(f.start(t), testToken)
			parent := t.TempDir()
			dir := filepath.Join(parent, "run")
			run := &Run{ID: "r1", Outputs: []Artefact{{Name: PushedOutput, Bytes: int64(buf.Len())}}}
			if _, err := c.DownloadOutputs(t.Context(), run, dir); err == nil {
				t.Fatalf("%s was accepted", name)
			}
			for _, p := range []string{filepath.Join(parent, "escaped.txt"), "/tmp/abs.txt"} {
				if _, err := os.Stat(p); err == nil && name != "a name twice" {
					t.Fatalf("a file landed outside the run folder: %s", p)
				}
			}
		})
	}
}

// Cancel: queued cancels the run outright; running records the request and
// the run finishes as interrupted with what it settled (plan §5).
func TestCancelQueuedAndRunning(t *testing.T) {
	f := &fakeRuns{enabled: true, outputs: map[string][]byte{}}
	base := f.start(t)
	c := New(base, testToken)
	ctx := t.Context()

	run, err := c.Create(ctx, CreateRequest{Ref: "acme/reporter@1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := c.Cancel(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("a queued run cancelled = %+v", cancelled)
	}
	if v := Read(cancelled); v.Settled || !strings.Contains(v.Line, "released") {
		t.Fatalf("cancelled verdict = %+v", v)
	}

	f.mu.Lock()
	f.run.Status = StatusRunning
	f.mu.Unlock()
	running, err := c.Cancel(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !running.CancelRequested || running.Status != StatusRunning {
		t.Fatalf("a running run cancelled = %+v", running)
	}
	f.finish(OutcomeInterrupted, "interrupted", 3)
	done, err := c.Get(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	v := Read(done)
	if v.Outcome != OutcomeInterrupted || !v.Settled {
		t.Fatalf("interrupted verdict = %+v", v)
	}
	if done.SettledCredits == nil || *done.SettledCredits != 3 {
		t.Fatalf("settlement = %v", done.SettledCredits)
	}
}

// A refusal reads as a sentence with its slug and, for the 402, the
// balance and the reset the panel shows — never raw problem JSON.
func TestRefusalsCarryTheirSlugAndFields(t *testing.T) {
	f := &fakeRuns{enabled: true, createRefusal: creditsExhausted}
	c := New(f.start(t), testToken)

	_, err := c.Create(t.Context(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	ref := RefusalOf(err)
	if ref == nil {
		t.Fatalf("create error = %v; want a refusal", err)
	}
	if ref.Slug != "credits-exhausted" || ref.Balance != 4 || ref.Hold != 20 || ref.Tier != "standard" || ref.ResetsAt == "" {
		t.Fatalf("refusal = %+v", ref)
	}
	mustContain(t, ref.Sentence(), "a run holds 20 credits")
	if strings.Contains(ref.Sentence(), "{") {
		t.Fatalf("the sentence carries JSON: %q", ref.Sentence())
	}

	f.mu.Lock()
	f.createRefusal = runActive
	f.mu.Unlock()
	_, err = c.Create(t.Context(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	ref = RefusalOf(err)
	if ref == nil || ref.Slug != "run-active" {
		t.Fatalf("second create = %v", err)
	}
	mustContain(t, ref.Sentence(), "One hosted run at a time")

	// A registry with no runs extension answers 404 to the status route;
	// the caller reads that as "no hosted runs here", not as a crash.
	none := New("http://127.0.0.1:1/", testToken)
	if _, err := none.Status(t.Context()); err == nil {
		t.Fatal("an unreachable registry must be an error")
	}
}

// The account's active run is what a reopened panel reattaches to.
func TestListFindsTheActiveRun(t *testing.T) {
	f := &fakeRuns{enabled: true}
	c := New(f.start(t), testToken)
	created, err := c.Create(t.Context(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != created.ID || !Active(runs[0].Status) {
		t.Fatalf("list = %+v", runs)
	}
	f.finish(OutcomeSucceeded, "ok", 1)
	runs, err = c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if Active(runs[0].Status) {
		t.Fatalf("a finished run still reads as active: %+v", runs[0])
	}
}

// The input limits are the app's, named before anything is uploaded.
func TestStageInputsRefusesOverThePlansLimits(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	var many []string
	for i := 0; i < MaxInputFiles+1; i++ {
		many = append(many, write("f"+string(rune('a'+i))+".txt", 1))
	}
	if _, err := StageInputs(many); err == nil || !strings.Contains(err.Error(), "at most 20 input files") {
		t.Fatalf("21 files: %v", err)
	}

	big := write("big.bin", int(MaxInputBytes)+1)
	if _, err := StageInputs([]string{big}); err == nil || !strings.Contains(err.Error(), "10 MB") {
		t.Fatalf("an oversized file: %v", err)
	}

	sub := filepath.Join(dir, "folder")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := StageInputs([]string{sub}); err == nil || !strings.Contains(err.Error(), "is a folder") {
		t.Fatalf("a folder: %v", err)
	}

	a, b := write("one/notes.txt", 2), write("two/notes.txt", 2)
	if _, err := StageInputs([]string{a, b}); err == nil || !strings.Contains(err.Error(), "both named notes.txt") {
		t.Fatalf("duplicate names: %v", err)
	}

	ok, err := StageInputs([]string{write("ok.txt", 10)})
	if err != nil || len(ok) != 1 || ok[0].Name != "ok.txt" || ok[0].Size != 10 {
		t.Fatalf("a good file: %+v %v", ok, err)
	}
}

// Uploads travel one file at a time and their ids are what a create names.
func TestUploadInputs(t *testing.T) {
	f := &fakeRuns{enabled: true}
	c := New(f.start(t), testToken)
	dir := t.TempDir()
	p := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := StageInputs([]string{p})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := c.UploadInputs(t.Context(), files)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "input-1" {
		t.Fatalf("ids = %v", ids)
	}
	if len(f.uploads) != 1 || f.uploads[0].Name != "notes.txt" || f.uploads[0].Bytes != 5 {
		t.Fatalf("stored = %+v", f.uploads)
	}
}

// An artefact name from the registry never leaves the run folder.
func TestOutputPathStaysInTheRunFolder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../escape.txt", "/etc/passwd", "reports/../../out.txt", "..", "", "a/../../b"} {
		if p, err := OutputPath(dir, name); err == nil {
			t.Fatalf("%q was accepted as %s", name, p)
		}
	}
	for _, name := range []string{"result.json", "reports/report.md", "outputs/deep/file.txt"} {
		p, err := OutputPath(dir, name)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			t.Fatalf("%q landed at %s", name, p)
		}
	}
}

// A download that names an escaping artefact writes nothing outside the
// folder and says so.
func TestDownloadRefusesAnEscapingArtefact(t *testing.T) {
	f := &fakeRuns{enabled: true, outputs: map[string][]byte{"result.json": []byte(`"ok"`)}}
	c := New(f.start(t), testToken)
	dir := t.TempDir()
	run := &Run{ID: "r1", Outputs: []Artefact{{Name: "../escaped.txt", Bytes: 3}}}
	got, err := c.DownloadOutputs(t.Context(), run, dir)
	if err == nil || len(got) != 0 {
		t.Fatalf("an escaping name was accepted: %v %+v", err, got)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escaped.txt")); statErr == nil {
		t.Fatal("a file landed outside the run folder")
	}
}

// Every outcome of §5 reads distinguishably, and the two that refund say
// so — a refusal names the platform's own verification.
func TestVerdictPerOutcome(t *testing.T) {
	cases := []struct {
		outcome string
		tone    string
		settled bool
		says    string
	}{
		{OutcomeSucceeded, "ok", true, "ran to the end"},
		{OutcomeFailed, "bad", true, "the agent ran and failed"},
		{OutcomeTimeout, "bad", true, "time limit"},
		{OutcomeBudget, "bad", true, "turn or token budget"},
		{OutcomeInterrupted, "warn", true, "you cancelled it"},
		{OutcomeRefused, "bad", false, "the platform's own verification refused this release"},
		{OutcomeError, "bad", false, "the platform could not run it"},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		v := Read(&Run{Status: StatusFinished, Outcome: c.outcome})
		if v.Outcome != c.outcome || v.Tone != c.tone || v.Settled != c.settled {
			t.Fatalf("%s = %+v", c.outcome, v)
		}
		if !strings.Contains(v.Line, c.says) {
			t.Fatalf("%s reads %q, want it to mention %q", c.outcome, v.Line, c.says)
		}
		if seen[v.Line] {
			t.Fatalf("%s does not read distinguishably: %q", c.outcome, v.Line)
		}
		seen[v.Line] = true
		if c.outcome == OutcomeRefused && !strings.Contains(v.Line, "refunded") {
			t.Fatalf("a refusal must say the credits came back: %q", v.Line)
		}
	}
	// The states a run passes through each say where it stands.
	for _, s := range []struct{ status, says string }{
		{StatusQueued, "queued"}, {StatusClaimed, "running"}, {StatusRunning, "running"},
	} {
		if got := Read(&Run{Status: s.status}).Line; !strings.Contains(got, s.says) {
			t.Fatalf("%s reads %q", s.status, got)
		}
	}
	if got := Read(&Run{Status: StatusRunning, CancelRequested: true}).Line; !strings.Contains(got, "cancelling") {
		t.Fatalf("a cancelling run reads %q", got)
	}
	// And the one word the panel's badge shows, the site's own.
	for _, s := range []struct {
		run  Run
		word string
	}{
		{Run{Status: StatusQueued}, "queued"},
		{Run{Status: StatusClaimed}, "running"},
		{Run{Status: StatusRunning}, "running"},
		{Run{Status: StatusFinished, Outcome: OutcomeSucceeded}, "done — pass"},
		{Run{Status: StatusFinished, Outcome: OutcomeFailed}, "done — fail"},
		{Run{Status: StatusFinished, Outcome: OutcomeError}, "done — fail"},
		{Run{Status: StatusFinished}, "done"},
		{Run{Status: StatusCancelled}, "cancelled"},
	} {
		if got := State(&s.run); got != s.word {
			t.Fatalf("%s/%s reads %q, want %q", s.run.Status, s.run.Outcome, got, s.word)
		}
	}
}

// A harness that reports only the split still shows a token count.
func TestUsageTokens(t *testing.T) {
	if got := (&Usage{InputTokens: 900, OutputTokens: 300}).Tokens(); got != 1200 {
		t.Fatalf("split only = %d", got)
	}
	if got := (&Usage{InputTokens: 900, OutputTokens: 300, TotalTokens: 1150}).Tokens(); got != 1150 {
		t.Fatalf("reported total = %d", got)
	}
	var none *Usage
	if none.Tokens() != 0 {
		t.Fatal("no usage must be no tokens")
	}
}

// The availability rule as a table: the registry's extensions × the
// release's declared modes × the token's scopes (plan §4.7, D6, D7).
func TestOfferMatrix(t *testing.T) {
	full := []string{ScopeRunRead, ScopeRunWrite, "agent:read"}
	base := Facts{SignedIn: true, Extension: true, Modes: []string{"autonomous"}, Scopes: full, ScopesKnown: true, Enabled: true, StatusKnown: true}
	with := func(f func(*Facts)) Facts {
		c := base
		f(&c)
		return c
	}
	cases := []struct {
		name      string
		facts     Facts
		offered   bool
		available bool
		says      string
		remedy    string
	}{
		{"everything holds", base, true, true, "", ""},
		{"not signed in", with(func(f *Facts) { f.SignedIn = false }), false, false, "", ""},
		{"no runs extension", with(func(f *Facts) { f.Extension = false }), false, false, "", ""},
		{"interactive only", with(func(f *Facts) { f.Modes = []string{"interactive"} }), false, false, "", ""},
		{"no mode declared", with(func(f *Facts) { f.Modes = nil }), false, false, "", ""},
		{"both modes", with(func(f *Facts) { f.Modes = []string{"autonomous", "interactive"} }), true, true, "", ""},
		{"a declared secret", with(func(f *Facts) { f.DeclaredSecrets = []string{"PAGERDUTY_TOKEN"} }), true, false, "PAGERDUTY_TOKEN", ""},
		{"an old token", with(func(f *Facts) { f.Scopes = []string{"agent:read"} }), true, false, "run:read", "Sign in again"},
		{"read but not write", with(func(f *Facts) { f.Scopes = []string{ScopeRunRead} }), true, false, "run:write", "Sign in again"},
		{"scopes unreadable", with(func(f *Facts) { f.ScopesKnown = false; f.Scopes = nil }), true, false, "did not say what the stored token may do", "Sign in again"},
		{"runs turned off", with(func(f *Facts) { f.Enabled = false }), true, false, "not taking hosted runs", "this machine"},
		{"status unreadable", with(func(f *Facts) { f.StatusKnown = false }), true, false, "did not answer", "this machine"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := Decide(c.facts)
			if o.Offered != c.offered || o.Available != c.available {
				t.Fatalf("offered=%v available=%v, want %v/%v (%+v)", o.Offered, o.Available, c.offered, c.available, o)
			}
			if c.says != "" && !strings.Contains(o.Reason, c.says) {
				t.Fatalf("reason %q, want it to mention %q", o.Reason, c.says)
			}
			if c.remedy != "" && !strings.Contains(o.Remedy, c.remedy) {
				t.Fatalf("remedy %q, want it to mention %q", o.Remedy, c.remedy)
			}
			if !o.Offered && (o.Reason != "" || o.Remedy != "") {
				t.Fatalf("a mode that is not offered says nothing: %+v", o)
			}
			if o.Offered && !o.Available && o.Reason == "" {
				t.Fatal("unavailable without a reason")
			}
		})
	}
}

// A run this client is following is read and streamed with the account's
// bearer token, and with nothing else: no worker token, no model key.
func TestOnlyTheAccountsTokenTravels(t *testing.T) {
	var headers []string
	f := &fakeRuns{enabled: true, pushed: true, outputs: map[string][]byte{"result.json": []byte(`"ok"`)}}
	base := f.start(t)
	c := New(base, testToken)
	c.HTTP.Transport = recorder(&headers)
	c.Download.Transport = recorder(&headers)
	run, err := c.Create(context.Background(), CreateRequest{Ref: "acme/reporter@1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	// The download goes through its own client: it carries the same
	// account token and nothing else.
	f.finish(OutcomeSucceeded, "ok", 1)
	done, err := c.Get(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.DownloadOutputs(context.Background(), done, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(headers) != 3 || !strings.Contains(headers[2], "Authorization: Bearer "+testToken) {
		t.Fatalf("the download's headers = %v", headers)
	}
	for _, h := range headers {
		if strings.Contains(strings.ToLower(h), "worker") {
			t.Fatalf("a worker credential reached the app's request: %s", h)
		}
	}
	if len(headers) == 0 || !strings.Contains(headers[0], "Authorization: Bearer "+testToken) {
		t.Fatalf("headers = %v", headers)
	}
}
