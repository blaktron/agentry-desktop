package main

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The run In Agentry App follows from the start: the attach takes a run,
// never a knowledge build or a draft in flight for the same account, and
// downloads into a folder Go chose itself, since the page has picked none at
// start.
func TestTheAttachAtStartTakesOnlyARun(t *testing.T) {
	build := &hostedrun.Run{ID: "11111111-1111-1111-1111-111111111111", Kind: hostedrun.KindKnowledgeBuild, Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	draft := &hostedrun.Run{ID: "22222222-2222-2222-2222-222222222222", Kind: hostedrun.KindDraft, Status: hostedrun.StatusQueued, CreatedAt: time.Now().UTC()}
	run := &hostedrun.Run{ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusRunning, HoldCredits: 20, CreatedAt: time.Now().UTC()}

	f := &hostedFake{enabled: true, others: []*hostedrun.Run{build, draft}}
	reg := f.start(t)
	home, _ := signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)

	got, err := r.AttachHostedRunAtStart(reg)
	if err != nil || got.Run != nil {
		t.Fatalf("with a build and a draft in flight and no run, the attach = %+v, %v", got, err)
	}
	if r.hostedCurrent() != nil {
		t.Fatal("the attach followed something that is not a run")
	}

	f.mu.Lock()
	f.run = run
	f.mu.Unlock()
	got, err = r.AttachHostedRunAtStart(reg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Run == nil || got.Run.ID != run.ID {
		t.Fatalf("attach = %+v", got)
	}
	// No folder was picked: the run's folder is the default one, named
	// for it, under the person's Downloads.
	want := filepath.Join(home, "Downloads", "agentry-out", "acme-reporter-"+run.ID)
	if got.Run.OutDir != want {
		t.Fatalf("the run's folder = %q, want %q", got.Run.OutDir, want)
	}
	// Asked again, it answers with the run it already follows.
	again, err := r.AttachHostedRunAtStart(reg)
	if err != nil || again.Run == nil || again.Run.ID != run.ID {
		t.Fatalf("a second attach = %+v, %v", again, err)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	waitForIdle(t, r)
}

// Run defaults' saved folder is the one a run attached at start downloads
// into, a folder of its own inside it.
func TestTheAttachAtStartUsesRunDefaultsFolder(t *testing.T) {
	run := &hostedrun.Run{ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusQueued, CreatedAt: time.Now().UTC()}
	f := &hostedFake{enabled: true, run: run}
	reg := f.start(t)
	signIn(t, reg)
	saved := t.TempDir()
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.DefaultOutputDir = saved
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	r, _ := newRunAPI(t)
	fastPoll(t)
	got, err := r.AttachHostedRunAtStart(reg)
	if err != nil || got.Run == nil {
		t.Fatalf("attach = %+v, %v", got, err)
	}
	if !strings.HasPrefix(got.Run.OutDir, saved+string(filepath.Separator)) || !strings.Contains(got.Run.OutDir, run.ID) {
		t.Fatalf("the run's folder = %q, want one of its own inside %q", got.Run.OutDir, saved)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	waitForIdle(t, r)
}

// A chat In Agentry in flight at start is attached as a run is, since chats
// follow in the cards, and one the app already follows is the answer to a
// second attach.
func TestTheAttachAtStartTakesAChat(t *testing.T) {
	chat := &hostedrun.Run{ID: "44444444-4444-4444-4444-444444444444", Ref: "acme/helper@1.0.0", Mode: hostedrun.ModeInteractive, Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	f := &hostedFake{enabled: true, run: chat}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	got, err := r.AttachHostedRunAtStart(reg)
	if err != nil || got.Run == nil || got.Run.ID != chat.ID || got.Run.Mode != hostedrun.ModeInteractive {
		t.Fatalf("attach at start with a chat in flight = %+v, %v", got, err)
	}
	if cur := r.hostedCurrent(); cur == nil || !cur.session {
		t.Fatal("the chat is not followed as a session")
	}
	again, err := r.AttachHostedRunAtStart(reg)
	if err != nil || again.Run == nil || again.Run.ID != chat.ID {
		t.Fatalf("a second attach = %+v, %v", again, err)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	waitForIdle(t, r)
}

// waitForIdle waits for the follow loop to end once the run is over.
func waitForIdle(t *testing.T, r *RunAPI) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for r.hostedCurrent() != nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if r.hostedCurrent() != nil {
		t.Fatal("the follow loop outlived the run")
	}
}

// A run the app lost touch with while it was still moving is said to be
// so, and following it again brings its outputs down into the folder it was
// started to, which a picker returned and the page cannot name again.
func TestFollowingALostRunAgainKeepsItsFolder(t *testing.T) {
	run := &hostedrun.Run{ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	f := &hostedFake{enabled: true, run: run}
	reg := f.start(t)
	signIn(t, reg)
	r, events := newRunAPI(t)
	fastPoll(t)
	picked := t.TempDir()
	r.app.markPicked(picked)
	if a, err := r.AttachHostedRun(reg, picked); err != nil || a.Run == nil {
		t.Fatalf("attach = %+v, %v", a, err)
	}
	// Watch gives up after ten retries; the eleventh failure loses it,
	// and the next read, which answers, says the run is still going.
	f.mu.Lock()
	f.failGets = 11
	f.mu.Unlock()
	lost := waitForFinal(t, events)
	if !strings.Contains(lost.DownloadError, "lost touch with the registry") || lost.Status != hostedrun.StatusRunning {
		t.Fatalf("lost = %+v", lost)
	}
	waitForIdle(t, r)
	again, err := r.AttachHostedRunAtStart(reg)
	if err != nil || again.Run == nil || again.Run.ID != run.ID {
		t.Fatalf("follow again = %+v, %v", again, err)
	}
	if again.Run.OutDir != picked {
		t.Fatalf("followed again into %q, want the folder it was started to, %q", again.Run.OutDir, picked)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	waitForIdle(t, r)
}

// App's attach at start and a run setup's attach can be in flight at once;
// the run is followed once, and both answer with the one folder it downloads
// into.
func TestTwoAttachesAtOnceFollowTheRunOnce(t *testing.T) {
	run := &hostedrun.Run{ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	f := &hostedFake{enabled: true, run: run, listDelay: 50 * time.Millisecond}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	picked := t.TempDir()
	r.app.markPicked(picked)
	var wg sync.WaitGroup
	var atStart, setup *HostedStartView
	var err1, err2 error
	wg.Add(2)
	go func() { defer wg.Done(); atStart, err1 = r.AttachHostedRunAtStart(reg) }()
	go func() { defer wg.Done(); setup, err2 = r.AttachHostedRun(reg, picked) }()
	wg.Wait()
	if err1 != nil || err2 != nil || atStart.Run == nil || setup.Run == nil {
		t.Fatalf("attaches = %+v %v, %+v %v", atStart, err1, setup, err2)
	}
	if atStart.Run.OutDir != setup.Run.OutDir {
		t.Fatalf("one run, two folders: %q and %q", atStart.Run.OutDir, setup.Run.OutDir)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	waitForIdle(t, r)
}

// A run setup that finds a run this app lost touch with follows it into
// the folder it was started to as well; once its card is dismissed, the
// app no longer keeps that folder for it.
func TestALostRunsFolderIsUsedOnceAndForgottenOnDismiss(t *testing.T) {
	run := &hostedrun.Run{ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	f := &hostedFake{enabled: true, run: run}
	reg := f.start(t)
	home, _ := signIn(t, reg)
	r, events := newRunAPI(t)
	fastPoll(t)
	first, other := t.TempDir(), t.TempDir()
	r.app.markPicked(first)
	r.app.markPicked(other)
	lose := func(dir string) {
		t.Helper()
		if a, err := r.AttachHostedRun(reg, dir); err != nil || a.Run == nil {
			t.Fatalf("attach = %+v, %v", a, err)
		}
		f.mu.Lock()
		f.failGets = 11
		f.mu.Unlock()
		waitForFinal(t, events)
		waitForIdle(t, r)
		events.mu.Lock()
		events.runs = nil
		events.mu.Unlock()
	}
	lose(first)
	again, err := r.AttachHostedRun(reg, other)
	if err != nil || again.Run == nil || again.Run.OutDir != first {
		t.Fatalf("the setup followed a lost run into %+v, %v; want %q", again, err, first)
	}
	f.mu.Lock()
	f.failGets = 11
	f.mu.Unlock()
	waitForFinal(t, events)
	waitForIdle(t, r)
	r.ForgetRun(run.ID)
	got, err := r.AttachHostedRunAtStart(reg)
	if err != nil || got.Run == nil {
		t.Fatalf("attach = %+v, %v", got, err)
	}
	if want := filepath.Join(home, "Downloads", "agentry-out", "acme-reporter-"+run.ID); got.Run.OutDir != want {
		t.Fatalf("after Dismiss the run downloads into %q, want Go's own %q", got.Run.OutDir, want)
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 0)
	waitForIdle(t, r)
}

// When what this app already follows is another run (a second registry's),
// following a run says so and names both, rather than answering with the
// other run's folder for a run no loop follows.
func TestFollowingWhileAnotherRunIsFollowedSaysSo(t *testing.T) {
	run := &hostedrun.Run{ID: "33333333-3333-3333-3333-333333333333", Ref: "acme/reporter@1.0.0", Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	f := &hostedFake{enabled: true, run: run}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	other := &hostedFollow{registry: "https://other.example.test", id: "55555555-5555-5555-5555-555555555555", outDir: t.TempDir(), cancel: func() {}, done: make(chan struct{})}
	r.hostedMu.Lock()
	r.hosted = other
	r.hostedMu.Unlock()
	_, err := r.follow(reg, hostedTestToken, t.TempDir(), run)
	if err == nil || !strings.Contains(err.Error(), other.id) || !strings.Contains(err.Error(), run.ID) {
		t.Fatalf("follow over another run = %v", err)
	}
	if r.hostedCurrent() != other {
		t.Fatal("the run already followed was replaced")
	}
	r.hostedMu.Lock()
	r.hosted = nil
	r.hostedMu.Unlock()
}

// A run the registry created is under way whatever this app does next: a
// start that cannot follow it (another registry's run was attached while it
// was being created) leaves the secrets set for it to the run and the vault,
// as a start that started does.
func TestAStartThatCannotFollowItsRunLeavesItsSecrets(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	if err := r.SetHostedSecret(reg, "GITHUB_TOKEN", "held", false); err != nil {
		t.Fatal(err)
	}
	other := &hostedFollow{registry: "https://other.example.test", id: "55555555-5555-5555-5555-555555555555", outDir: t.TempDir(), cancel: func() {}, done: make(chan struct{})}
	t.Cleanup(func() {
		r.hostedMu.Lock()
		r.hosted = nil
		r.hostedMu.Unlock()
	})
	f.onCreate = func() {
		r.hostedMu.Lock()
		r.hosted = other
		r.hostedMu.Unlock()
	}
	out := t.TempDir()
	r.app.markPicked(out)
	_, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, nil, nil, false, false, nil)
	if err == nil || !strings.Contains(err.Error(), other.id) {
		t.Fatalf("start = %v", err)
	}
	f.mu.Lock()
	deleted := append([]string(nil), f.vaultDeletes...)
	f.mu.Unlock()
	if len(deleted) != 0 {
		t.Fatalf("a start whose run exists deleted %v", deleted)
	}
}
