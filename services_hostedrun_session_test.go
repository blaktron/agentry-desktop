package main

// Talk to it In Agentry through the bound surface (agentry-desktop#403):
// the offer from the registry's facts alone, a session started, streamed,
// talked to, given a picked file, finished and delivered with its
// transcript, and quitting that leaves it to its idle close.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
)

var sessionTerms = &hostedrun.SessionTerms{Enabled: true, FreeMinutes: 10, CreditsPerMinute: 50, MinCredits: 50, IdleSeconds: 300,
	Allowed: map[string]bool{"standard": false, "pro": true, "business": true, "enterprise": true}}

// The offer is the registry's facts, read as the run offer reads them: the
// release declares the interactive mode, the status runs sessions and says
// the caller's payer may start one (plan hosted-sessions.md §6, D12 as
// amended). A payer on Standard is told why, with the way up.
func TestHostedAvailabilityOffersASession(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name      string
		set       func(*hostedFake)
		session   bool
		available bool
		upgrade   bool
		run       bool
	}{
		{"an interactive release on Pro", func(f *hostedFake) {}, true, true, false, false},
		{"both modes on Pro", func(f *hostedFake) { f.modes = []string{"autonomous", "interactive"} }, true, true, false, true},
		{"an interactive release on Standard", func(f *hostedFake) { f.sessionAllowed = &no }, true, false, true, false},
		{"sessions off", func(f *hostedFake) { f.sessions = &hostedrun.SessionTerms{Enabled: false} }, false, false, false, false},
		{"a registry from before sessions", func(f *hostedFake) { f.sessions, f.sessionAllowed = nil, nil }, false, false, false, false},
		{"no caller line", func(f *hostedFake) { f.sessionAllowed = nil }, false, false, false, false},
		{"autonomous only", func(f *hostedFake) { f.modes = []string{"autonomous"} }, false, false, false, true},
		{"a token from before", func(f *hostedFake) { f.scopes = []string{"agent:read", "user:profile"} }, true, false, false, false},
		// No sandbox host answering: the registry would refuse the start
		// (409 sessions-unavailable), so it is not available (#437); a run
		// of a both-modes release still queues, as it always has.
		{"no sandbox host answering", func(f *hostedFake) { f.noHosts = true }, true, false, false, false},
		{"no sandbox host, both modes", func(f *hostedFake) { f.noHosts = true; f.modes = []string{"autonomous", "interactive"} }, true, false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &hostedFake{enabled: true, modes: []string{"interactive"}, sessions: sessionTerms, sessionAllowed: &yes}
			c.set(f)
			reg := f.start(t)
			signIn(t, reg)
			r, _ := newRunAPI(t)
			v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if v.Offered != c.run {
				t.Fatalf("the run's offer = %v, want %v: a session fact moved it", v.Offered, c.run)
			}
			s := v.Session
			if (s != nil) != c.session {
				t.Fatalf("session offer = %+v, want offered %v", s, c.session)
			}
			if s == nil {
				return
			}
			if s.Available != c.available || s.Upgrade != c.upgrade {
				t.Fatalf("session offer = %+v", s)
			}
			if !s.Available && (s.Reason == "" || s.Remedy == "") {
				t.Fatalf("unavailable without a reason: %+v", s)
			}
			if s.FreeMinutes != 10 || s.CreditsPerMinute != 50 || s.MinCredits != 50 || s.IdleSeconds != 300 {
				t.Fatalf("the terms are the registry's: %+v", s)
			}
			if s.Transport != "Transport encrypted by the user key, messages not stored; the transcript comes back only in your sealed output." {
				t.Fatalf("D2's sentence = %q", s.Transport)
			}
		})
	}
}

// One session end to end through the bound surface: created in the
// interactive mode with the task, its state and frames reported as the
// registry relays them — in the engine's event shape, a file named where
// the agent finds it and an error by its class's words — a message sent,
// a picked file added, the conversation replayed for a chat that opens on
// it, Finish, and the sealed output unpacked into the folder with
// transcript.md among the receipts. Nothing of it reaches the run log.
func TestHostedSessionEndToEnd(t *testing.T) {
	yes := true
	f := &hostedFake{
		enabled: true, modes: []string{"interactive"}, sessions: sessionTerms, sessionAllowed: &yes,
		outputs: map[string]string{
			"result.json":            hostedEnvelope,
			"receipts/transcript.md": "# Transcript\n\nYou: check the csv\n",
			"receipts/events.ndjson": "{}\n",
		},
	}
	reg := f.start(t)
	signIn(t, reg)
	// Before newRunAPI: its cleanup stops the follow before these are put
	// back.
	fastPoll(t)
	fastSessionRetry(t)
	r, events := newRunAPI(t)

	outDir, filesDir := t.TempDir(), t.TempDir()
	extra := filepath.Join(filesDir, "second.csv")
	if err := os.WriteFile(extra, []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.app.markPicked(outDir, extra)

	start, err := r.StartHostedSession(reg, "acme/reporter@1.0.0", "check the csv", nil, outDir, nil, nil, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Refusal != "" || start.Run == nil || start.Run.Mode != hostedrun.ModeInteractive || start.Run.HoldCredits != 0 || start.Run.Session == nil {
		t.Fatalf("start = %+v (run %+v)", start, start.Run)
	}
	f.mu.Lock()
	created := f.created
	f.mu.Unlock()
	if created == nil || created.Mode != hostedrun.ModeInteractive || created.Task != "check the csv" {
		t.Fatalf("created = %+v", created)
	}

	// The host starts it and the agent talks.
	now := time.Now().UTC()
	f.mu.Lock()
	f.run.Status, f.run.StartedAt = hostedrun.StatusRunning, &now
	f.mu.Unlock()
	f.frame(`{"kind":"user_turn","time":"2026-09-29T10:00:01Z","text":"check the csv"}`)
	f.frame(`{"kind":"assistant_text","time":"2026-09-29T10:00:02Z","text":"Looking"}`)
	f.frame(`{"kind":"assistant_turn","time":"2026-09-29T10:00:03Z","text":"Looking at it now."}`)
	waitForSession(t, events, 3)
	running := waitForStatus(t, events, hostedrun.StatusRunning)
	if running.Mode != hostedrun.ModeInteractive || running.Session == nil || running.Session.IdleSeconds != 300 {
		t.Fatalf("running = %+v", running)
	}
	if st := waitForSessionState(t, events, hostedrun.StatusRunning); st.StartedAt == "" || st.Session == nil || st.Session.FreeMinutes != 10 {
		t.Fatalf("state = %+v", st)
	}

	if err := r.SendHostedTurn("and the second column?"); err != nil {
		t.Fatal(err)
	}
	if err := r.AddHostedSessionFile(extra); err != nil {
		t.Fatal(err)
	}
	f.frame(`{"kind":"input_added","time":"2026-09-29T10:00:05Z","text":"second.csv"}`)
	f.frame(`{"kind":"error","time":"2026-09-29T10:00:06Z","class":"harness","text":"the harness's own words, never shown"}`)
	got := waitForSession(t, events, 5)
	if got[3].Kind != "input_added" || got[3].Path != "/inputs/second.csv" || got[3].Seq != 4 {
		t.Fatalf("input added = %+v", got[3])
	}
	if got[4].Kind != "error" || got[4].Text != "the harness reported an error" {
		t.Fatalf("error = %+v", got[4])
	}

	// A chat that opens now gets every frame again, with its numbers.
	if err := r.ReplayHostedSession(); err != nil {
		t.Fatal(err)
	}
	replayed := waitForSession(t, events, 10)
	if replayed[5].Seq != 1 || replayed[5].Kind != "user_turn" || replayed[5].Text != "check the csv" {
		t.Fatalf("the replay did not start at the first frame: %+v", replayed[5])
	}

	if err := r.EndHostedSession(true); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	posts := f.sessionPosts
	afters := f.streamAfters
	f.mu.Unlock()
	if len(posts["turns"]) != 1 || !strings.Contains(posts["turns"][0], `"and the second column?"`) {
		t.Fatalf("turns = %q", posts["turns"])
	}
	var file struct {
		Input string `json:"input"`
	}
	if len(posts["files"]) != 1 || json.Unmarshal([]byte(posts["files"][0]), &file) != nil || file.Input != "input-second.csv" {
		t.Fatalf("files = %q, want the upload's id", posts["files"])
	}
	if len(posts["end"]) != 1 || posts["end"][0] != `{"finish":true}` {
		t.Fatalf("end = %q", posts["end"])
	}
	if len(afters) < 2 || afters[0] != "0" || afters[len(afters)-1] != "0" {
		t.Fatalf("stream afters = %v, want the first stream and the replay from 0", afters)
	}

	f.frame(`{"kind":"session_end","time":"2026-09-29T10:01:00Z","reason":"finished"}`)
	f.finish(hostedrun.OutcomeSucceeded, "ok", 3)
	f.mu.Lock()
	f.run.Session.TokenCredits = 3
	f.mu.Unlock()
	final := waitForFinal(t, events)
	if final.DownloadError != "" || final.Mode != hostedrun.ModeInteractive {
		t.Fatalf("final = %+v", final)
	}
	transcript := filepath.Join(outDir, "receipts", "transcript.md")
	if b, err := os.ReadFile(transcript); err != nil || !strings.Contains(string(b), "check the csv") {
		t.Fatalf("transcript.md: %q %v", b, err)
	}
	if lines := events.lines(); len(lines) != 0 {
		t.Fatalf("a session In Agentry wrote to the run-log view: %q", lines)
	}
	// The session is over and the app follows nothing.
	deadline := time.Now().Add(10 * time.Second)
	for r.hostedCurrent() != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if r.hostedCurrent() != nil {
		t.Fatal("the follow outlived the session")
	}
}

// A file the page names without a native picker is refused before a byte
// leaves the machine (D-030), and a refusal comes back in the registry's
// words; Stop before the host has started it cancels the run.
func TestHostedSessionRefusals(t *testing.T) {
	yes := true
	f := &hostedFake{enabled: true, modes: []string{"interactive"}, sessions: sessionTerms, sessionAllowed: &yes}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	fastSessionRetry(t)
	r, _ := newRunAPI(t)
	if err := r.SendHostedTurn("hi"); err == nil {
		t.Fatal("a turn with no session followed was sent")
	}
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedSession(reg, "acme/reporter@1.0.0", "", nil, outDir, nil, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}

	unpicked := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(unpicked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.AddHostedSessionFile(unpicked); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("an unpicked file: %v", err)
	}
	f.mu.Lock()
	if len(f.sessionPosts["files"]) != 0 {
		t.Fatalf("an unpicked file reached the registry")
	}
	f.sessionRefuse = func(w http.ResponseWriter, route string) bool {
		hostedJSON(w, http.StatusConflict, map[string]any{
			"type": "https://agentry.dev/problems/session-starting", "title": "The session is starting",
			"detail": "The session has not started on its sandbox host yet. Send this again once it is running.",
		})
		return true
	}
	f.mu.Unlock()
	if err := r.SendHostedTurn("hi"); err == nil || !strings.Contains(err.Error(), "has not started on its sandbox host") || strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("a refused turn: %v, want the registry's sentence alone", err)
	}
	if err := r.EndHostedSession(false); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	cancelled := f.cancelled
	f.mu.Unlock()
	if cancelled != 1 {
		t.Fatalf("Stop on a session not yet started cancelled %d times, want once", cancelled)
	}
}

// Quitting while a session goes on In Agentry does not end it: the dialog
// says it goes on until its idle close and that reopening picks it up, and
// nothing is posted to its end route.
func TestQuitLeavesAHostedSessionToItsIdleClose(t *testing.T) {
	yes := true
	f := &hostedFake{enabled: true, modes: []string{"interactive"}, sessions: sessionTerms, sessionAllowed: &yes}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	fastSessionRetry(t)
	r, _ := newRunAPI(t)
	var said string
	r.app.confirm = func(_ context.Context, _, msg string) (bool, error) {
		said = msg
		return true, nil
	}
	if r.beforeClose(context.Background()) || said != "" {
		t.Fatal("an idle app asked before closing")
	}
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedSession(reg, "acme/reporter@1.0.0", "", nil, outDir, nil, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if r.beforeClose(context.Background()) {
		t.Fatal("Yes did not let the window close")
	}
	for _, want := range []string{"acme/reporter@1.0.0", "Quitting does not end it", "idle for 5 minutes", "picks the conversation up"} {
		if !strings.Contains(said, want) {
			t.Fatalf("the quit dialog said %q, want %q in it", said, want)
		}
	}
	r.shutdown()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sessionPosts["end"]) != 0 || f.cancelled != 0 {
		t.Fatal("quitting ended the session In Agentry")
	}
}

// fastSessionRetry shortens the pause before a failed stream is asked for
// again, for the test's life.
func fastSessionRetry(t *testing.T) {
	t.Helper()
	old := sessionRetry
	sessionRetry = 10 * time.Millisecond
	t.Cleanup(func() { sessionRetry = old })
}

// waitForSession waits until the chat has been sent n events.
func waitForSession(t *testing.T, events *emitted, n int) []SessionEventView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		events.mu.Lock()
		got := append([]SessionEventView(nil), events.session...)
		events.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	t.Fatalf("the chat was sent %d events, want %d: %+v", len(events.session), n, events.session)
	return nil
}

// waitForSessionState waits for a state event in a status.
func waitForSessionState(t *testing.T, events *emitted, status string) HostedSessionStateView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		events.mu.Lock()
		for _, s := range events.sessionStates {
			if s.Status == status {
				events.mu.Unlock()
				return s
			}
		}
		events.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no state event said %s", status)
	return HostedSessionStateView{}
}

// A session whose conversation the registry cannot show here (the
// runner's key did not open) is followed by its state alone: the chat is
// told why, a chat that opens later is told again rather than left empty,
// and the output still comes back when the session ends.
func TestAStreamTheRegistryCannotShowIsSaidAndTheSessionStillDelivers(t *testing.T) {
	yes := true
	f := &hostedFake{enabled: true, modes: []string{"interactive"}, sessions: sessionTerms, sessionAllowed: &yes,
		streamError: "key", outputs: map[string]string{"result.json": hostedEnvelope, "receipts/transcript.md": "# Transcript\n"}}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	fastSessionRetry(t)
	r, events := newRunAPI(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	if _, err := r.StartHostedSession(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil); err != nil {
		t.Fatal(err)
	}
	lost := waitForSession(t, events, 1)
	if lost[0].Kind != "stream_lost" || !strings.Contains(lost[0].Text, "cannot be shown here") {
		t.Fatalf("first event = %+v", lost[0])
	}
	if err := r.ReplayHostedSession(); err != nil {
		t.Fatal(err)
	}
	if again := waitForSession(t, events, 2); again[1].Kind != "stream_lost" {
		t.Fatalf("a chat opened after the stream was lost was told %+v", again[1])
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	if final := waitForFinal(t, events); final.DownloadError != "" || len(final.Outputs) != 2 {
		t.Fatalf("final = %+v", final)
	}
}
