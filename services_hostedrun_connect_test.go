package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
)

// OAuth connections M9 (agentry-desktop#667; plan mcp-oauth-connections.md
// D5, D19): run create's 409 connection-missing drives the ask, as
// secret-missing does; the app opens the site's Connect tools for the
// version it runs and makes the same create again until Agentry holds the
// connection, then the run starts, its inputs uploaded once. The app never
// makes, lists or receives a hosted connection.

const linearServer = "https://mcp.linear.app/mcp"

func connectionMissing(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "https://agentry.dev/problems/connection-missing", "title": "A tool is not connected", "status": 409,
		"detail":      "The release's tools sign in by OAuth, and you have not connected " + linearServer + " for this version. Connect them, then start the run.",
		"missing":     []string{linearServer},
		"connections": []string{linearServer},
	})
}

// refuseUntil answers connection-missing to the first n creates, as a
// vault that holds the connection from the n+1th on.
func refuseUntil(f *hostedFake, n int) {
	seen := 0
	f.refuse = func(w http.ResponseWriter) {
		seen++
		if seen == n {
			f.refuse = nil // under f.mu: the next create is answered
		}
		connectionMissing(w)
	}
}

// startWithInput makes a start of acme/linear@0.1.0 with one input file.
func startWithInput(t *testing.T, r *RunAPI, reg string) *HostedStartView {
	t.Helper()
	inputDir := t.TempDir()
	input := filepath.Join(inputDir, "notes.txt")
	if err := os.WriteFile(input, []byte("triage"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	r.app.markPicked(inputDir, outDir)
	start, err := r.StartHostedRun(reg, "acme/linear@0.1.0", "list my issues", []string{input}, outDir, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return start
}

func TestAStartWaitsForItsConnectionsAndRunsOnceTheyAreMade(t *testing.T) {
	f := &hostedFake{enabled: true}
	refuseUntil(f, 3)
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	r.connectPoll = 10 * time.Millisecond

	start := startWithInput(t, r, reg)
	if start.Refusal != hostedrun.SlugConnectionMissing || strings.Join(start.Missing, ",") != linearServer || start.Run != nil {
		t.Fatalf("the first start = %+v", start)
	}
	got, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Refusal != "" || got.Run == nil || got.Run.Ref != "acme/linear@0.1.0" {
		t.Fatalf("after the connection = %+v", got)
	}
	f.mu.Lock()
	uploads, creates := f.uploads, append([]string(nil), f.creates...)
	f.mu.Unlock()
	if uploads != 1 {
		t.Fatalf("the input went up %d times; a wait makes the same create with the same inputs", uploads)
	}
	if len(creates) != 4 {
		t.Fatalf("%d creates, want the start's, two refused while waiting, and the one that ran", len(creates))
	}
	for _, c := range creates[1:] {
		if c != creates[0] {
			t.Fatalf("a create while waiting differs from the start's:\n%s\n%s", c, creates[0])
		}
	}
	// What the panel received names no token: there is none to name.
	b, _ := json.Marshal([]*HostedStartView{start, got})
	if strings.Contains(strings.ToLower(string(b)), "token") {
		t.Fatalf("a view carries a token field: %s", b)
	}
	// The start is spent: there is nothing left to wait for.
	if _, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0"); err == nil {
		t.Fatal("a second wait found the spent start")
	}
}

func TestAnotherRefusalEndsTheWaitAndForgetsTheStart(t *testing.T) {
	f := &hostedFake{enabled: true}
	seen := 0
	f.refuse = func(w http.ResponseWriter) {
		seen++
		if seen < 3 {
			connectionMissing(w)
			return
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/secret-missing","title":"A declared secret is missing","status":409,"missing":["LINEAR_TEAM"]}`))
	}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	r.connectPoll = 10 * time.Millisecond

	startWithInput(t, r, reg)
	got, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0")
	if err != nil || got.Refusal != "secret-missing" || strings.Join(got.Missing, ",") != "LINEAR_TEAM" {
		t.Fatalf("got %+v, %v; want the secret ask, as a start answers it", got, err)
	}
	if _, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0"); err == nil {
		t.Fatal("the start was kept after a refusal that is not about connections")
	}
}

func TestStopKeepsTheStartForTheNextWait(t *testing.T) {
	f := &hostedFake{enabled: true}
	f.refuse = connectionMissing
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	r.connectPoll = 10 * time.Millisecond

	startWithInput(t, r, reg)
	done := make(chan *HostedStartView, 1)
	go func() {
		v, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0")
		if err != nil {
			t.Error(err)
		}
		done <- v
	}()
	// A second wait on the same start while one is under way is refused.
	time.Sleep(50 * time.Millisecond)
	if _, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0"); err == nil || !strings.Contains(err.Error(), "already waiting") {
		t.Fatalf("a second wait: %v", err)
	}
	r.StopAwaitingConnections()
	select {
	case v := <-done:
		if v == nil || v.Refusal != hostedrun.SlugConnectionMissing || !strings.HasPrefix(v.Detail, "Stopped waiting.") {
			t.Fatalf("stopped = %+v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not end the wait")
	}
	// The connection is made; the next wait runs it, with no new upload.
	f.mu.Lock()
	f.refuse = nil
	f.mu.Unlock()
	got, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0")
	if err != nil || got.Run == nil {
		t.Fatalf("the next wait = %+v, %v", got, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploads != 1 {
		t.Fatalf("uploads = %d", f.uploads)
	}
}

func TestADiscardedWaitAnswersNothing(t *testing.T) {
	f := &hostedFake{enabled: true}
	f.refuse = connectionMissing
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	r.connectPoll = 10 * time.Millisecond

	startWithInput(t, r, reg)
	done := make(chan *HostedStartView, 1)
	go func() {
		v, _ := r.AwaitHostedConnections(reg, "acme/linear@0.1.0")
		done <- v
	}()
	time.Sleep(50 * time.Millisecond)
	r.DiscardHostedConnect() // the panel's Cancel, or its close
	select {
	case v := <-done:
		if v == nil || v.Refusal != "" || v.Run != nil {
			t.Fatalf("a discarded wait answered %+v; want nothing to ask about", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Discard did not end the wait")
	}
}

func TestAWaitThatRunsOutSaysSoAndKeepsTheStart(t *testing.T) {
	f := &hostedFake{enabled: true}
	f.refuse = connectionMissing
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	r.connectPoll, r.connectWait = 10*time.Millisecond, 40*time.Millisecond

	startWithInput(t, r, reg)
	got, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0")
	if err != nil || got.Refusal != hostedrun.SlugConnectionMissing || !strings.Contains(got.Detail, "still keeps no connection to "+linearServer) {
		t.Fatalf("got %+v, %v", got, err)
	}
	r.DiscardHostedConnect()
	if _, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0"); err == nil {
		t.Fatal("a discarded start was waited on")
	}
}

func TestANewStartReplacesTheOneWaiting(t *testing.T) {
	f := &hostedFake{enabled: true}
	f.refuse = connectionMissing
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	startWithInput(t, r, reg)
	out := t.TempDir()
	r.app.markPicked(out)
	if _, err := r.StartHostedRun(reg, "acme/other@1.0.0", "go", nil, out, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AwaitHostedConnections(reg, "acme/linear@0.1.0"); err == nil {
		t.Fatal("the replaced start was waited on")
	}
	f.mu.Lock()
	f.refuse = nil
	f.mu.Unlock()
	if v, err := r.AwaitHostedConnections(reg, "acme/other@1.0.0"); err != nil || v.Run == nil || v.Run.Ref != "acme/other@1.0.0" {
		t.Fatalf("the new start's wait = %+v, %v", v, err)
	}
}

func TestOpenHostedConnectOpensTheSitesConnectToolsForTheVersion(t *testing.T) {
	r, _ := newRunAPI(t)
	var opened []string
	r.openPage = func(u string) error { opened = append(opened, u); return nil }
	if err := r.OpenHostedConnect("https://dev.secureagentry.net/", "acme/linear@0.1.0"); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://dev.secureagentry.net/acme/linear/run/setup?version=0.1.0&step=tools"}
	if !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened %v, want %v", opened, want)
	}
	// Settings' URL rule (D-039), and an exact version, before anything opens.
	for _, c := range [][2]string{{"http://registry.example", "acme/linear@0.1.0"}, {"https://registry.example", "acme/linear"}} {
		if err := r.OpenHostedConnect(c[0], c[1]); err == nil {
			t.Fatalf("OpenHostedConnect(%q, %q) opened", c[0], c[1])
		}
	}
	if len(opened) != 1 {
		t.Fatalf("opened %v", opened)
	}
}
