package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/session"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// fakeAgent is a running agent's supervisor: it records what it was sent and
// plays the events a test feeds it.
type fakeAgent struct {
	mu      sync.Mutex
	sent    []string
	events  chan session.Event
	done    chan struct{}
	stopped int
	roster  persistent.Roster
}

func newFakeAgent() *fakeAgent {
	return &fakeAgent{events: make(chan session.Event, 16), done: make(chan struct{})}
}

func (f *fakeAgent) Send(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	return nil
}
func (f *fakeAgent) Subscribe() (<-chan session.Event, func()) { return f.events, func() {} }
func (f *fakeAgent) Status() persistent.Status {
	return persistent.Status{State: persistent.StateRunning, Segment: 1, Run: "seg1", Since: time.Now()}
}
func (f *fakeAgent) Roster() persistent.Roster { return f.roster }
func (f *fakeAgent) Done() <-chan struct{}     { return f.done }
func (f *fakeAgent) Stop(context.Context) (*receipt.Envelope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	if f.stopped == 1 {
		close(f.events)
		close(f.done)
	}
	return &receipt.Envelope{}, nil
}

// persistentRig is a PersistentAPI over a runner home of its own, whose
// starts give the fake agent and whose dialogs answer yes.
type persistentRig struct {
	p       *PersistentAPI
	home    string
	agent   *fakeAgent
	asked   []string
	answer  bool
	started int
	mu      sync.Mutex
	emitted []any
}

func newPersistentRig(t *testing.T) *persistentRig {
	t.Helper()
	_, home := isolateHomes(t)
	rig := &persistentRig{home: home, agent: newFakeAgent(), answer: true}
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, msg string) (bool, error) {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		rig.asked = append(rig.asked, title+"\n"+msg)
		return rig.answer, nil
	}}
	rig.p = &PersistentAPI{app: app, run: &RunAPI{app: app, runnerHomeDir: home}}
	rig.p.start = func(_ context.Context, opts persistent.Options) (agentSupervisor, error) {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		rig.started++
		if opts.Client != persistent.ClientDesktop {
			t.Errorf("the supervisor's client is %q", opts.Client)
		}
		return rig.agent, nil
	}
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		rig.mu.Lock()
		defer rig.mu.Unlock()
		if name == eventAgentEvent {
			rig.emitted = append(rig.emitted, data...)
		}
	}
	t.Cleanup(func() { emitEvent = old })
	// Before the seam goes back: every agent stopped and its followers done.
	t.Cleanup(func() {
		rig.p.stopAll()
		rig.p.followers.Wait()
	})
	return rig
}

func (r *persistentRig) dialogs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.asked...)
}

// A start of an agent whose roster turns on what the person has not agreed
// to on this computer asks in a native dialog naming each entry, and records
// the agreement; a No starts nothing, and a start after the agreement asks
// nothing.
func TestAPersistentAgentsStartAsksForItsRosterOnce(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.Start("assistant"); err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("a disabled agent started: %v", err)
	}
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	rig.answer = false
	if _, err := rig.p.Start("assistant"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if rig.started != 0 {
		t.Fatal("a No started the agent")
	}
	d := rig.dialogs()
	if len(d) != 1 || !strings.Contains(d[0], "Shares the folder") || !strings.Contains(d[0], "for the first time on this computer") {
		t.Fatalf("the dialog: %q", d)
	}
	rig.answer = true
	v, err := rig.p.Start("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if v.State != persistent.StateRunning || rig.started != 1 {
		t.Fatalf("state %q after %d starts", v.State, rig.started)
	}
	if _, err := rig.p.Stop("assistant"); err != nil {
		t.Fatal(err)
	}
	rig.agent = newFakeAgent()
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if got := len(rig.dialogs()); got != 2 {
		t.Fatalf("%d dialogs: the second start asked again", got)
	}
}

// A running agent does not hold the run lock: an ordinary run takes it
// beside the agent.
func TestAPersistentAgentRunsBesideAnOrdinaryRun(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if err := rig.p.run.acquire(jobRun, "a run of acme/demo"); err != nil {
		t.Fatalf("a run beside the agent: %v", err)
	}
	rig.p.run.release()
}

// An agent's events reach the page keyed by its name, and a message is sent
// as its next turn.
func TestAPersistentAgentsEventsAreKeyedByItsName(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if err := rig.p.Send("assistant", "  "); err == nil {
		t.Fatal("an empty message was sent")
	}
	if err := rig.p.Send("assistant", "Hello"); err != nil {
		t.Fatal(err)
	}
	rig.agent.events <- session.Event{Kind: session.KindAssistantTurn, Text: "Hi there", Time: time.Now()}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rig.mu.Lock()
		n := len(rig.emitted)
		rig.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	rig.mu.Lock()
	defer rig.mu.Unlock()
	if len(rig.emitted) != 1 {
		t.Fatalf("emitted %v", rig.emitted)
	}
	ev, ok := rig.emitted[0].(PersistentEventView)
	if !ok || ev.Agent != "assistant" || ev.Event.Kind != "assistant_turn" || ev.Event.Text != "Hi there" {
		t.Fatalf("the event: %#v", rig.emitted[0])
	}
	if len(rig.agent.sent) != 1 || rig.agent.sent[0] != "Hello" {
		t.Fatalf("sent %q", rig.agent.sent)
	}
}

// Reset wipes nothing without a Yes in its native dialog, which says what
// goes and what is kept; with one, a running agent is stopped first and
// then put back to its defaults.
func TestResettingAPersistentAgentAsksFirst(t *testing.T) {
	rig := newPersistentRig(t)
	rig.p.docker = &dockerx.Fake{}
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	name := "Sandra"
	if _, err := rig.p.Configure("assistant", PersistentChange{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	asked := len(rig.dialogs())
	rig.answer = false
	if _, err := rig.p.Reset("assistant"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	d := rig.dialogs()[asked:]
	if len(d) != 1 {
		t.Fatalf("the dialogs: %q", d)
	}
	for _, want := range []string{"Reset Sandra?", "back to its defaults", "its name and personality", "what you agreed to", "files in the share folder you chose", "it is stopped first"} {
		if !strings.Contains(d[0], want) {
			t.Errorf("the dialog lacks %q: %q", want, d[0])
		}
	}
	if _, here := rig.p.running("assistant"); !here {
		t.Fatal("a No stopped the agent")
	}
	rig.answer = true
	v, err := rig.p.Reset("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if _, here := rig.p.running("assistant"); here || rig.agent.stopped == 0 {
		t.Fatal("the running agent was not stopped first")
	}
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DisplayName != "" || len(cfg.Consented) != 0 || !cfg.Enabled {
		t.Errorf("the configuration after a reset: %+v", cfg)
	}
	if v.DisplayName == name {
		t.Errorf("the view after a reset still names it %q", v.DisplayName)
	}
}

// The Assistant offers Reset history, Reset agent and Reset all, the
// others Reset alone; each smaller reset asks in its native dialog with the
// words the page shows. Reset history keeps the name, Reset agent does not.
func TestResettingTheAssistantInParts(t *testing.T) {
	rig := newPersistentRig(t)
	rig.p.docker = &dockerx.Fake{}
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	name := "Sandra"
	v, err := rig.p.Configure("assistant", PersistentChange{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, r := range v.Resets {
		kinds = append(kinds, r.Kind)
	}
	if strings.Join(kinds, ",") != "history,agent,all" || v.Resets[2].Title != "Reset all" {
		t.Fatalf("the Assistant's resets: %+v", v.Resets)
	}
	coder, err := rig.p.Get("coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(coder.Resets) != 1 || coder.Resets[0].Kind != "all" || coder.Resets[0].Title != "Reset" {
		t.Fatalf("the Coding agent's resets: %+v", coder.Resets)
	}
	if _, err := rig.p.ResetPart("coder", "history"); err == nil || !strings.Contains(err.Error(), "one reset") {
		t.Errorf("Reset history of the Coding agent: %v", err)
	}
	if _, err := rig.p.ResetPart("assistant", "all"); err == nil {
		t.Error("ResetPart took the full reset")
	}

	if _, err := rig.p.ResetPart("assistant", ""); err == nil {
		t.Error("ResetPart took no kind")
	}
	// What the person gave it: a stored secret's name (its value is the
	// keychain's) and a sign-in, on and agreed to.
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	secret := persistent.SecretEntryID("DEMO_TOKEN")
	cfg.Secrets = []persistent.StoredSecret{{Name: "DEMO_TOKEN"}}
	cfg.SignIns = map[string]persistent.ImageSignIn{"gog": {Account: "person@example.com"}}
	cfg.Switches = map[string]bool{secret: true}
	cfg.Consented = map[string]string{secret: "Gives your secret DEMO_TOKEN"}
	if err := persistent.SaveConfig(rig.home, "assistant", cfg); err != nil {
		t.Fatal(err)
	}

	asked := len(rig.dialogs())
	rig.answer = false
	if _, err := rig.p.ResetPart("assistant", "history"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	d := rig.dialogs()[asked:]
	if len(d) != 1 || !strings.Contains(d[0], "Reset Sandra's history?") || !strings.Contains(d[0], v.Resets[0].Body) {
		t.Fatalf("Reset history's dialog: %q", d)
	}
	rig.answer = true
	after, err := rig.p.ResetPart("assistant", "history")
	if err != nil {
		t.Fatal(err)
	}
	kept, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	if after.DisplayName != name || !kept.Switches[secret] || kept.Consented[secret] == "" || len(kept.Secrets) != 1 || len(kept.SignIns) != 1 {
		t.Errorf("Reset history did not keep what the person chose: view %q, config %+v", after.DisplayName, kept)
	}
	asked = len(rig.dialogs())
	after, err = rig.p.ResetPart("assistant", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if d := rig.dialogs()[asked:]; len(d) != 1 || !strings.Contains(d[0], "Reset Sandra, keeping its secrets and sign-ins?") {
		t.Fatalf("Reset agent's dialog: %q", d)
	}
	cfg, err = persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	if after.DisplayName == name || cfg.DisplayName != "" || !cfg.Enabled {
		t.Errorf("after Reset agent: view %q, config %+v", after.DisplayName, cfg)
	}
	// The credentials stay, switched off and not agreed to.
	if len(cfg.Secrets) != 1 || len(cfg.SignIns) != 1 || cfg.Switches[secret] || len(cfg.Consented) != 0 {
		t.Errorf("Reset agent's credentials: %+v", cfg)
	}
}

// The name and personality are saved as the person wrote them, drawn back
// in the view; the sidebar's label is the name.
func TestConfiguringTheAssistantsNameAndPersonality(t *testing.T) {
	rig := newPersistentRig(t)
	name, personality := "Sandra", "Warm but brief."
	v, err := rig.p.Configure("assistant", PersistentChange{Name: &name, Personality: &personality})
	if err != nil {
		t.Fatal(err)
	}
	if v.DisplayName != "Sandra" || v.Personality != personality || v.Title != "Assistant" {
		t.Fatalf("the view: %+v", v)
	}
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil || cfg.DisplayName != "Sandra" {
		t.Fatalf("config %+v, %v", cfg, err)
	}
}

// The share folder comes from the native picker alone, held to the share
// folder's guards: the home folder itself is refused, a folder of the
// person's is saved, and a cancel changes nothing.
func TestTheShareFolderComesFromThePicker(t *testing.T) {
	rig := newPersistentRig(t)
	// The home as the system resolves it: a temporary folder can be reached
	// through a link (macOS's /var is /private/var), and a share reached
	// through one is refused.
	home, _ := os.UserHomeDir()
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
		setUserHome(t, home)
	}
	pick := home
	rig.p.app.pickDirectory = func(context.Context, string) (string, error) { return pick, nil }
	if _, err := rig.p.ChooseShare("assistant"); err == nil {
		t.Fatal("the home folder was taken as a share folder")
	}
	pick = filepath.Join(home, "notes-for-sandra")
	if err := os.MkdirAll(pick, 0o755); err != nil {
		t.Fatal(err)
	}
	v, err := rig.p.ChooseShare("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if v.Share != pick || v.DefaultShare {
		t.Fatalf("share %q default %v", v.Share, v.DefaultShare)
	}
	pick = ""
	if v, err = rig.p.ChooseShare("assistant"); err != nil || v.Share != filepath.Join(home, "notes-for-sandra") {
		t.Fatalf("a cancel: %q, %v", v.Share, err)
	}
	if v, err = rig.p.Configure("assistant", PersistentChange{DefaultShare: true}); err != nil || !v.DefaultShare {
		t.Fatalf("back to the default: %+v, %v", v, err)
	}
}

// An agent the app does not offer is refused by every binding.
func TestOnlyTheOfferedAgentsAreDriven(t *testing.T) {
	rig := newPersistentRig(t)
	for _, name := range []string{"nobody", "examiner"} {
		if _, err := rig.p.Start(name); err == nil {
			t.Errorf("%s started", name)
		}
		if _, err := rig.p.Get(name); err == nil {
			t.Errorf("%s was read", name)
		}
	}
}

// Quitting stops every agent the app runs.
func TestQuittingStopsEveryPersistentAgent(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	rig.p.stopAll()
	rig.agent.mu.Lock()
	defer rig.agent.mu.Unlock()
	if rig.agent.stopped != 1 {
		t.Fatalf("stopped %d times", rig.agent.stopped)
	}
}

// A change that turns on what the person has not agreed to on this computer
// asks in a native dialog naming it, and a No changes nothing.
func TestAWideningChangeAsksAndANoChangesNothing(t *testing.T) {
	rig := newPersistentRig(t)
	rig.answer = false
	if _, err := rig.p.Configure("assistant", PersistentChange{AddKnowledge: "team-wiki"}); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	d := rig.dialogs()
	if len(d) != 1 || !strings.Contains(d[0], "team-wiki") {
		t.Fatalf("the dialog: %q", d)
	}
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil || len(cfg.Knowledge) != 0 {
		t.Fatalf("a No left %v (%v)", cfg.Knowledge, err)
	}
	rig.answer = true
	v, err := rig.p.Configure("assistant", PersistentChange{AddKnowledge: "team-wiki"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Knowledge) != 1 || v.Knowledge[0] != "team-wiki" {
		t.Fatalf("knowledge %v", v.Knowledge)
	}
	// Turning it off narrows: nothing to ask.
	before := len(rig.dialogs())
	if _, err := rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: persistent.KnowledgeEntryID("team-wiki"), On: false}}); err != nil {
		t.Fatal(err)
	}
	if len(rig.dialogs()) != before {
		t.Fatal("switching an entry off asked")
	}
}

// A file added to the share is copied beside what is there, never over it,
// and only a regular file is copied.
func TestAFileAddedToTheShareIsNeverWrittenOverAFile(t *testing.T) {
	share, src := t.TempDir(), t.TempDir()
	f := filepath.Join(src, "notes.md")
	if err := os.WriteFile(f, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "notes.md"), []byte("the agent's"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := copyInto(share, f)
	if err != nil || name != "notes (1).md" {
		t.Fatalf("copied as %q, %v", name, err)
	}
	if b, _ := os.ReadFile(filepath.Join(share, "notes.md")); string(b) != "the agent's" {
		t.Fatalf("the file there was written over: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(share, name)); string(b) != "new" {
		t.Fatalf("the copy holds %q", b)
	}
	if _, err := copyInto(share, src); err == nil {
		t.Fatal("a folder was copied")
	}
	// A link the agent left at the name is not followed: the copy takes
	// the next name.
	if err := os.Symlink(filepath.Join(src, "elsewhere"), filepath.Join(share, "report.md")); err != nil {
		t.Fatal(err)
	}
	g := filepath.Join(src, "report.md")
	if err := os.WriteFile(g, []byte("r"), 0o600); err != nil {
		t.Fatal(err)
	}
	if name, err := copyInto(share, g); err != nil || name != "report (1).md" {
		t.Fatalf("beside a link: %q, %v", name, err)
	}
	if _, err := os.Stat(filepath.Join(src, "elsewhere")); !os.IsNotExist(err) {
		t.Fatal("the copy followed the link it found")
	}
}

// A quit while an agent starts cancels the start and waits for it, so the
// start leaves nothing running.
func TestAQuitDuringAStartCancelsIt(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	rig.p.start = func(ctx context.Context, _ persistent.Options) (agentSupervisor, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	errc := make(chan error, 1)
	go func() {
		_, err := rig.p.Start("assistant")
		errc <- err
	}()
	<-entered
	done := make(chan struct{})
	go func() {
		rig.p.stopAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the quit did not end the start")
	}
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("the start ended with %v", err)
	}
}

// The context a supervisor starts under is the agent's for its whole life:
// a start that succeeds does not cancel it.
func TestAStartedAgentsContextOutlivesItsStart(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	var started context.Context
	rig.p.start = func(ctx context.Context, _ persistent.Options) (agentSupervisor, error) {
		started = ctx
		return rig.agent, nil
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if err := started.Err(); err != nil {
		t.Fatalf("the running agent's context ended with its start: %v", err)
	}
	if d := rig.dialogs(); len(d) != 1 || !strings.HasPrefix(d[0], "Start the Assistant?") {
		t.Fatalf("the dialog: %q", d)
	}
}

// An agent that stops without being asked says so in its chat; one the
// person stops does not.
func TestAnAgentThatStopsByItselfSaysSo(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	// The segment ends by itself.
	_, _ = rig.agent.Stop(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	var notice string
	for notice == "" && time.Now().Before(deadline) {
		rig.mu.Lock()
		for _, e := range rig.emitted {
			if ev, ok := e.(PersistentEventView); ok && ev.Event.Kind == "notice" {
				notice = ev.Event.Text
			}
		}
		rig.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.HasPrefix(notice, "The agent stopped by itself") {
		t.Fatalf("the notice: %q", notice)
	}
	rig.p.followers.Wait()
	rig.mu.Lock()
	rig.emitted = nil
	rig.mu.Unlock()
	rig.agent = newFakeAgent()
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Stop("assistant"); err != nil {
		t.Fatal(err)
	}
	rig.p.followers.Wait()
	rig.mu.Lock()
	defer rig.mu.Unlock()
	if len(rig.emitted) != 0 {
		t.Fatalf("a stop the person asked for was told as its own: %v", rig.emitted)
	}
}

// Enabling the Coding agent asks in a native dialog that says what it may
// do; a No leaves it disabled and changes nothing, a Yes enables it. The
// Assistant's switch asks nothing, and turning the Coding agent off asks
// nothing either.
func TestEnablingTheCodingAgentAsksFirst(t *testing.T) {
	rig := newPersistentRig(t)
	rig.answer = false
	v, err := rig.p.SetEnabled(coderAgent, true)
	if err != nil || v.State != agentDisabled {
		t.Fatalf("after a No: %+v, %v", v, err)
	}
	asked := rig.dialogs()
	if len(asked) != 1 || !strings.Contains(asked[0], "builds and runs software") || !strings.Contains(asked[0], "codingshare") || !strings.Contains(asked[0], "riskier") {
		t.Fatalf("the dialog: %q", asked)
	}
	rig.answer = true
	if v, err = rig.p.SetEnabled(coderAgent, true); err != nil || v.State != agentStopped {
		t.Fatalf("after a Yes: %+v, %v", v, err)
	}
	if v, err = rig.p.SetEnabled(coderAgent, false); err != nil || v.State != agentDisabled {
		t.Fatalf("off: %+v, %v", v, err)
	}
	if _, err = rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if n := len(rig.dialogs()); n != 2 {
		t.Fatalf("%d dialogs, want the Coding agent's two", n)
	}
	// Only the Coding agent has a dev server.
	if _, err := rig.p.SetDevHost("assistant", "me@devbox.example", run.HostSignIn{Password: "x"}); err == nil {
		t.Fatal("the Assistant took a dev server")
	}
}

// Run when Agentry starts is saved for an agent and read back.
func TestRunOnStartIsSaved(t *testing.T) {
	rig := newPersistentRig(t)
	on := true
	v, err := rig.p.Configure(coderAgent, PersistentChange{RunOnStart: &on})
	if err != nil || !v.RunOnStart {
		t.Fatalf("%+v, %v", v, err)
	}
}

// The Coding agent's ports: added with the person's agreement and shown,
// opened in the browser only while the agent runs with the port on, and
// taken away; the Assistant has none.
func TestTheCodingAgentsPortsOpenOnlyWhilePublished(t *testing.T) {
	rig := newPersistentRig(t)
	var opened []string
	old := browserOpenURL
	browserOpenURL = func(_ context.Context, url string) { opened = append(opened, url) }
	t.Cleanup(func() { browserOpenURL = old })

	if _, err := rig.p.SetEnabled(coderAgent, true); err != nil {
		t.Fatal(err)
	}
	rig.answer = false
	if _, err := rig.p.Configure(coderAgent, PersistentChange{AddPort: &PersistentPort{Name: "web", Port: 3000}}); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if d := rig.dialogs(); !strings.Contains(d[len(d)-1], "http://127.0.0.1:3000") {
		t.Fatalf("the dialog: %q", d[len(d)-1])
	}
	rig.answer = true
	v, err := rig.p.Configure(coderAgent, PersistentChange{AddPort: &PersistentPort{Name: "web", Port: 3000}})
	if err != nil || len(v.Ports) != 1 || v.Ports[0] != (PersistentPort{Name: "web", Port: 3000}) {
		t.Fatalf("after a Yes: %+v, %v", v.Ports, err)
	}
	if err := rig.p.OpenPort(coderAgent, "web"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("a port of a stopped agent opened: %v", err)
	}
	cfg, _ := persistent.LoadConfig(rig.home, coderAgent)
	a, _ := persistent.Lookup(coderAgent)
	rig.agent.roster = persistent.BuildRoster(a, cfg, nil)
	if _, err := rig.p.Start(coderAgent); err != nil {
		t.Fatal(err)
	}
	if err := rig.p.OpenPort(coderAgent, "web"); err != nil || len(opened) != 1 || opened[0] != "http://127.0.0.1:3000/" {
		t.Fatalf("open: %v, opened %q", err, opened)
	}
	if err := rig.p.OpenPort(coderAgent, "api"); err == nil {
		t.Error("a port it does not have opened")
	}
	if v, err = rig.p.Configure(coderAgent, PersistentChange{RemovePort: "web"}); err != nil || len(v.Ports) != 0 {
		t.Fatalf("removing: %+v, %v", v.Ports, err)
	}
	if _, err := rig.p.Configure("assistant", PersistentChange{AddPort: &PersistentPort{Name: "web", Port: 3000}}); err == nil {
		t.Error("the Assistant took a port")
	}
}
