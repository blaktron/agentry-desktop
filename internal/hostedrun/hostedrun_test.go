package hostedrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The whole account-side lifecycle against the fake extension: the status
// route's caller block, a create, the log streamed with a dropped
// connection in the middle, the finish, and the artefacts downloaded into
// the chosen folder (plan §4.1, §4.4, §4.5; agentry-desktop#77).
func TestCreateStreamFinishAndDownload(t *testing.T) {
	f := &fakeRuns{
		enabled: true, workersOnline: 1, queued: 1,
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

	// The log: eight lines, the first connection cut after three, the rest
	// on the resume. Nothing lost, nothing shown twice — and the fake
	// replays every row on the second connection to prove the client drops
	// what it has already seen.
	f.addEvents(logEvents("materializing", "verifying digest", "gateway up",
		"turn 1", "tool: read_input", "turn 2", "writing report", "done")...)
	f.cutAfter, f.replayAll = 3, true
	f.finish(OutcomeSucceeded, "ok", 2)

	var lines []string
	var statuses []string
	final, err := c.Follow(ctx, run.ID, FollowOptions{
		Poll:       20 * time.Millisecond,
		RetryDelay: time.Millisecond,
		OnEvent:    func(ev Event) { lines = append(lines, ev.Line) },
		OnStatus:   func(r *Run) { statuses = append(statuses, r.Status) },
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"materializing", "verifying digest", "gateway up", "turn 1", "tool: read_input", "turn 2", "writing report", "done"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("log lines = %q, want %q", lines, want)
	}
	if f.streams < 2 {
		t.Fatalf("the dropped connection was never resumed (%d streams)", f.streams)
	}
	if len(f.afterSeen) < 2 || f.afterSeen[1] != 3 {
		t.Fatalf("the resume did not start after the last event id: %v", f.afterSeen)
	}
	if len(statuses) == 0 || statuses[len(statuses)-1] != StatusFinished {
		t.Fatalf("statuses = %v", statuses)
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

	// The artefacts land under the chosen folder, folders and all.
	dir := t.TempDir()
	got, err := c.DownloadOutputs(ctx, final, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("downloaded %d artefacts: %+v", len(got), got)
	}
	for _, a := range got {
		if !strings.HasPrefix(a.Path, dir+string(filepath.Separator)) {
			t.Fatalf("%s landed outside the run folder: %s", a.Name, a.Path)
		}
		if _, err := os.Stat(a.Path); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "reports", "report.md"))
	if err != nil || string(body) != "# Report\n" {
		t.Fatalf("report.md = %q (%v)", body, err)
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
	var log []string
	ids, err := c.UploadInputs(t.Context(), files, func(l string) { log = append(log, l) })
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "input-1" {
		t.Fatalf("ids = %v", ids)
	}
	if len(f.uploads) != 1 || f.uploads[0].Name != "notes.txt" || f.uploads[0].Bytes != 5 {
		t.Fatalf("stored = %+v", f.uploads)
	}
	if len(log) != 1 || !strings.Contains(log[0], "notes.txt") {
		t.Fatalf("log = %v", log)
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
		{StatusQueued, "queued"}, {StatusClaimed, "materialising"}, {StatusRunning, "running"},
	} {
		if got := Read(&Run{Status: s.status}).Line; !strings.Contains(got, s.says) {
			t.Fatalf("%s reads %q", s.status, got)
		}
	}
	if got := Read(&Run{Status: StatusRunning, CancelRequested: true}).Line; !strings.Contains(got, "cancelling") {
		t.Fatalf("a cancelling run reads %q", got)
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
	f := &fakeRuns{enabled: true}
	base := f.start(t)
	c := New(base, testToken)
	c.HTTP.Transport = recorder(&headers)
	if _, err := c.Create(context.Background(), CreateRequest{Ref: "acme/reporter@1.0.0"}); err != nil {
		t.Fatal(err)
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
