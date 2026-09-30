package main

// A hosted session, live (agentry-desktop#403; plan hosted-sessions.md M6):
// the bound methods driven as RunPanel and SessionChat drive them, against
// a real registry that runs sessions. Skipped unless the variables are
// set, because a session spends the payer's credits past its free minutes:
//
//	AGENTRY_LIVE_SESSION_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_SESSION_TOKEN=<a Pro account's PAT: run:read, run:write, …> \
//	AGENTRY_LIVE_SESSION_REF=<ns>/<name>@<version, interactive mode declared> \
//	AGENTRY_LIVE_SESSION_OUT=<a folder to keep the output in, optional> \
//	go test -run TestLiveHostedSession -v -timeout 30m .
//
// The test checks that Talk to it In Agentry is offered and available,
// starts a session with a first message, waits for the agent's reply,
// adds a file through the picker's path, asks about it, replays the
// conversation as a chat that opens would, finishes, and finds the sealed
// output unpacked into the folder with transcript.md among the receipts.
// It logs the event kinds and the strip's facts, never the conversation.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedSession(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_SESSION_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_SESSION_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_SESSION_REF")
	if reg == "" || token == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_SESSION_REGISTRY, _TOKEN and _REF to run one hosted session")
	}
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
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
	r, events := newRunAPI(t)

	avail, err := r.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	if avail.Session == nil || !avail.Session.Available {
		t.Fatalf("Talk to it In Agentry is not available for %s: %+v (session %+v)", ref, avail, avail.Session)
	}
	t.Logf("offer: tier %s, balance %d; session: %d free minutes, %d credits a minute, idle %ds; the run's offer %v",
		avail.Tier, avail.Balance, avail.Session.FreeMinutes, avail.Session.CreditsPerMinute, avail.Session.IdleSeconds, avail.Offered)

	out := os.Getenv("AGENTRY_LIVE_SESSION_OUT")
	if out == "" {
		out = t.TempDir()
	}
	files := t.TempDir()
	csv := filepath.Join(files, "second.csv")
	if err := os.WriteFile(csv, []byte("id,amount,currency\n1,12.50,EUR\n2,,EUR\n3,7.00,usd\n3,7.00,usd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.app.markPicked(out, csv) // what the native pickers record (D-030)

	start, err := r.StartHostedSession(reg, ref, "Say hello in one sentence, then wait for a file I will add.", nil, out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Refusal != "" || start.Run == nil {
		t.Fatalf("the start was refused: %+v", start)
	}
	t.Logf("session %s: %s, mode %s, hold %d", start.Run.ID, start.Run.Status, start.Run.Mode, start.Run.HoldCredits)
	if err := r.ReplayHostedSession(); err != nil {
		t.Fatal(err)
	}

	kinds := func() []SessionEventView {
		events.mu.Lock()
		defer events.mu.Unlock()
		return append([]SessionEventView(nil), events.session...)
	}
	waitFor := func(what string, d time.Duration, ok func([]SessionEventView) bool) []SessionEventView {
		t.Helper()
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if got := kinds(); ok(got) {
				return got
			}
			time.Sleep(time.Second)
		}
		got := kinds()
		names := make([]string, len(got))
		for i, e := range got {
			names[i] = e.Kind
		}
		t.Fatalf("waited %s for %s; the chat was sent %v", d, what, names)
		return nil
	}
	count := func(evs []SessionEventView, kind string) int {
		n := 0
		for _, e := range evs {
			if e.Kind == kind {
				n++
			}
		}
		return n
	}

	waitFor("the agent's first reply", 15*time.Minute, func(evs []SessionEventView) bool { return count(evs, "assistant_turn") >= 1 })
	if err := r.AddHostedSessionFile(csv); err != nil {
		t.Fatal(err)
	}
	waitFor("the file added", 2*time.Minute, func(evs []SessionEventView) bool {
		for _, e := range evs {
			if e.Kind == "input_added" && e.Path == "/inputs/second.csv" {
				return true
			}
		}
		return false
	})
	// The runner tells the agent about the file in a turn of its own (a
	// Runner turn, agentry-cli#487), and a message sent while it is
	// answering is refused session-busy, as the chat's Send is held: wait
	// for that turn's reply first.
	added := len(kinds())
	notice := false
	deadline0 := time.Now().Add(time.Minute)
	for !notice && time.Now().Before(deadline0) {
		for _, e := range kinds()[added-1:] {
			notice = notice || (e.Kind == "user_turn" && e.Synthetic)
		}
		time.Sleep(time.Second)
	}
	t.Logf("a Runner turn about the file: %v", notice)
	if notice {
		waitFor("the reply to the Runner's turn", 10*time.Minute, func(evs []SessionEventView) bool { return count(evs, "assistant_turn") >= 2 })
	}
	replies := count(kinds(), "assistant_turn")
	if err := r.SendHostedTurn("I added second.csv. In one sentence: how many rows does it have, and is any duplicated?"); err != nil {
		t.Fatal(err)
	}
	waitFor("the reply about the file", 10*time.Minute, func(evs []SessionEventView) bool { return count(evs, "assistant_turn") > replies })

	// A chat that opens now is given every frame again, from the first.
	before := len(kinds())
	if err := r.ReplayHostedSession(); err != nil {
		t.Fatal(err)
	}
	waitFor("the replay", 2*time.Minute, func(evs []SessionEventView) bool {
		return len(evs) > before && evs[before].Seq == 1
	})

	events.mu.Lock()
	var strip HostedSessionStateView
	if n := len(events.sessionStates); n > 0 {
		strip = events.sessionStates[n-1]
	}
	events.mu.Unlock()
	if strip.Session == nil || strip.StartedAt == "" {
		t.Fatalf("no state carried the session record: %+v", strip)
	}
	t.Logf("strip: %s since %s, free %d, paid through %d, %d credits for time, idle %ds",
		strip.Status, strip.StartedAt, strip.Session.FreeMinutes, strip.Session.PaidThrough, strip.Session.TimeCredits, strip.Session.IdleSeconds)

	if err := r.EndHostedSession(true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Minute)
	var final *HostedRunView
	for final == nil && time.Now().Before(deadline) {
		for _, v := range events.states() {
			if v.Final {
				final = v
			}
		}
		time.Sleep(2 * time.Second)
	}
	if final == nil {
		t.Fatal("the session never reported a final state")
	}
	evs := kinds()
	names := make([]string, len(evs))
	for i, e := range evs {
		names[i] = e.Kind
	}
	t.Logf("events: %v", names)
	t.Logf("session %s: %s, outcome %s, exit %s, error %q, download error %q", final.ID, final.State, final.Outcome, final.ExitClass, final.Error, final.DownloadError)
	if final.Session != nil {
		t.Logf("credits: %d settled; time %d, tokens %d, unpaid %d, paid through minute %d", final.SettledCredits, final.Session.TimeCredits, final.Session.TokenCredits, final.Session.TokensUnpaid, final.Session.PaidThrough)
	}
	if final.DownloadError != "" {
		t.Fatalf("the output did not come down: %s", final.DownloadError)
	}
	var transcript string
	for _, a := range final.Outputs {
		t.Logf("output: %s (%d bytes)", a.Name, a.Bytes)
		if filepath.Base(a.Name) == "transcript.md" {
			transcript = a.Path
		}
	}
	if transcript == "" || !strings.HasPrefix(transcript, out+string(filepath.Separator)) {
		t.Fatalf("no transcript.md in the run folder %s: %+v", out, final.Outputs)
	}
	b, err := os.ReadFile(transcript) //nolint:gosec // a path this test's run folder holds
	if err != nil || !strings.Contains(string(b), "second.csv") {
		t.Fatalf("transcript.md does not carry the conversation: %v", err)
	}
	t.Logf("transcript.md: %d bytes, at %s", len(b), transcript)
}
