package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
)

// fakeRemote is the app's chat with an agent another client runs.
type fakeRemote struct {
	mu     sync.Mutex
	sent   []string
	busy   bool
	ended  bool
	events chan session.Event
}

func (f *fakeRemote) Events() <-chan session.Event { return f.events }
func (f *fakeRemote) Send(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return persistent.ErrBusy
	}
	f.sent = append(f.sent, text)
	return nil
}
func (f *fakeRemote) AddInput(context.Context, string) error { return errors.New("no files") }
func (f *fakeRemote) End(context.Context, bool) (*receipt.Envelope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ended {
		f.ended = true
		close(f.events)
	}
	return nil, nil
}
func (f *fakeRemote) Ended() (bool, session.EndReason) { return false, "" }

// elsewhereRig is a persistent rig whose assistant a terminal's supervisor
// may hold: its holder record names another process, and the lock probe,
// Attach and RequestStop are the test's.
type elsewhereRig struct {
	*persistentRig
	mu       sync.Mutex
	held     bool
	attaches int
	remote   *fakeRemote
	stops    int
	states   []string
}

func newElsewhereRig(t *testing.T) *elsewhereRig {
	t.Helper()
	r := &elsewhereRig{persistentRig: newPersistentRig(t)}
	if _, err := r.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	r.p.held = func(_, name string) (bool, *persistent.Holder) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if name != "assistant" || !r.held {
			return false, nil
		}
		return true, &persistent.Holder{Client: persistent.ClientCLI, PID: os.Getpid() + 1}
	}
	r.p.attach = func(_ context.Context, _, name string, _ []session.Event) (session.Session, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if name != "assistant" || !r.held {
			return nil, persistent.ErrNotRunning
		}
		r.attaches++
		r.remote = &fakeRemote{events: make(chan session.Event, 4)}
		return r.remote, nil
	}
	r.p.requestStop = func(_ context.Context, home, _ string) (persistent.Stopped, error) {
		r.mu.Lock()
		r.stops++
		r.mu.Unlock()
		r.hold(t, home, false)
		return persistent.Stopped{}, nil
	}
	old := emitEvent
	emitEvent = func(a *App, name string, data ...any) {
		if name == eventAgentState {
			r.mu.Lock()
			r.states = append(r.states, data[0].(string))
			r.mu.Unlock()
			return
		}
		old(a, name, data...)
	}
	t.Cleanup(func() { emitEvent = old })
	return r
}

// hold writes or removes the terminal supervisor's holder record, as its
// Acquire and Release do.
func (r *elsewhereRig) hold(t *testing.T, home string, on bool) {
	t.Helper()
	r.mu.Lock()
	r.held = on
	r.mu.Unlock()
	dir := persistent.Dir(home, "assistant")
	if !on {
		_ = os.Remove(filepath.Join(dir, "owner.json"))
		return
	}
	b, _ := json.Marshal(persistent.Holder{Client: persistent.ClientCLI, PID: os.Getpid() + 1, Since: time.Now()})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "owner.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (r *elsewhereRig) stateEvents() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.states...)
}

// An agent a terminal starts is seen at the next look, without the page
// asking: its state reads running in a terminal and the page is told; its
// stop there is seen the same way.
func TestAnAgentStartedInATerminalIsSeen(t *testing.T) {
	r := newElsewhereRig(t)
	r.p.checkElsewhere()
	if got := r.stateEvents(); len(got) != 0 {
		t.Fatalf("a look with nothing changed told the page %v", got)
	}
	r.hold(t, r.home, true)
	r.p.checkElsewhere()
	if got := r.stateEvents(); len(got) != 1 || got[0] != "assistant" {
		t.Fatalf("the start in a terminal told the page %v", got)
	}
	v, err := r.p.Get("assistant")
	if err != nil || v.State != agentElsewhere || v.Holder != "a terminal" {
		t.Fatalf("the view: %+v, %v", v, err)
	}
	r.p.checkElsewhere()
	if got := r.stateEvents(); len(got) != 1 || r.attaches != 1 {
		t.Fatalf("a second look: told %v, attached %d times", got, r.attaches)
	}
	r.hold(t, r.home, false)
	r.p.checkElsewhere()
	if got := r.stateEvents(); len(got) != 2 {
		t.Fatalf("the stop in a terminal told the page %v", got)
	}
	if v, _ := r.p.Get("assistant"); v.State != agentStopped {
		t.Fatalf("after the stop: %q", v.State)
	}
	if !r.remote.ended {
		t.Error("the chat with the stopped agent was not left")
	}
}

// The app's own hold of the lock (a sign-in) is not another client's run.
func TestTheAppsOwnSetUpIsNotARunElsewhere(t *testing.T) {
	r := newElsewhereRig(t)
	r.p.held = nil
	owner, err := persistent.Acquire(r.home, "assistant", persistent.ClientDesktop)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	r.p.checkElsewhere()
	if v, _ := r.p.Get("assistant"); v.State == agentElsewhere || len(r.stateEvents()) != 0 {
		t.Fatalf("the app's own set-up read as a run elsewhere: %q, %v", v.State, r.stateEvents())
	}
}

// A message to an agent a terminal runs goes through its supervisor's
// socket, its events reach the page keyed by its name, and a send while
// another chat's turn is in flight says so.
func TestTheAppChatsWithAnAgentATerminalRuns(t *testing.T) {
	r := newElsewhereRig(t)
	if err := r.p.Send("assistant", "Hello"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("a send to a stopped agent: %v", err)
	}
	r.hold(t, r.home, true)
	if err := r.p.Send("assistant", "Hello"); err != nil {
		t.Fatal(err)
	}
	if r.remote == nil || len(r.remote.sent) != 1 || r.remote.sent[0] != "Hello" {
		t.Fatalf("sent %+v", r.remote)
	}
	r.remote.events <- session.Event{Kind: session.KindAssistantTurn, Text: "Hi there", Time: time.Now()}
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.persistentRig.mu.Lock()
		n := len(r.emitted)
		r.persistentRig.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	r.persistentRig.mu.Lock()
	emitted := append([]any{}, r.emitted...)
	r.persistentRig.mu.Unlock()
	if len(emitted) != 1 {
		t.Fatalf("emitted %v", emitted)
	}
	if ev, ok := emitted[0].(PersistentEventView); !ok || ev.Agent != "assistant" || ev.Event.Text != "Hi there" {
		t.Fatalf("the event %+v", emitted[0])
	}
	r.remote.mu.Lock()
	r.remote.busy = true
	r.remote.mu.Unlock()
	if err := r.p.Send("assistant", "Again"); err == nil || !strings.Contains(err.Error(), "another client") {
		t.Fatalf("a busy send: %v", err)
	}
	if r.attaches != 1 {
		t.Fatalf("attached %d times", r.attaches)
	}
}

// Stop stops an agent a terminal runs through its supervisor's socket; it
// then reads stopped and the app starts it itself.
func TestTheAppStopsAnAgentATerminalRuns(t *testing.T) {
	r := newElsewhereRig(t)
	r.hold(t, r.home, true)
	r.p.checkElsewhere()
	v, err := r.p.Stop("assistant")
	if err != nil || r.stops != 1 || v.State != agentStopped {
		t.Fatalf("the stop: %+v after %d stops, %v", v, r.stops, err)
	}
	if v, err := r.p.Start("assistant"); err != nil || v.State != persistent.StateRunning {
		t.Fatalf("the start after it: %+v, %v", v, err)
	}
}

// A supervisor that closes its socket without answering a stop but lets the
// lock go stopped; one that keeps it did not.
func TestAStopElsewhereIsJudgedByTheLock(t *testing.T) {
	r := newElsewhereRig(t)
	r.hold(t, r.home, true)
	r.p.requestStop = func(context.Context, string, string) (persistent.Stopped, error) {
		return persistent.Stopped{}, errors.New("closed the socket without answering")
	}
	if _, err := r.p.Stop("assistant"); err == nil || !strings.Contains(err.Error(), "did not stop") {
		t.Fatalf("a stop that left it running: %v", err)
	}
	r.p.requestStop = func(_ context.Context, home, _ string) (persistent.Stopped, error) {
		r.hold(t, home, false)
		return persistent.Stopped{}, errors.New("closed the socket without answering")
	}
	if v, err := r.p.Stop("assistant"); err != nil || v.State != agentStopped {
		t.Fatalf("a stop that let the lock go: %+v, %v", v, err)
	}
}
