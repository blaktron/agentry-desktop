package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/run"
)

// The Knowledge panel's bindings keep the run surface's gates (D-030): an
// export and a delete are asked in a native dialog, every path is a native
// picker's, and a build is a run under the run lock.

type knowledgeKeys struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *knowledgeKeys) Get(a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.m[a]; ok {
		return v, nil
	}
	return "", store.ErrNoEntry
}

func (k *knowledgeKeys) Set(a, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[a] = v
	return nil
}

func (k *knowledgeKeys) Delete(a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, a)
	return nil
}

// knowledgeFixture is a KnowledgeAPI over a runner home of its own, a
// keychain in memory and a fake core registry signed in as one account,
// with a pack named handbook kept; steps records the dialogs, pickers and
// reveals in order.
type knowledgeFixture struct {
	k     *KnowledgeAPI
	reg   *knowledgetest.Registry
	env   knowledge.Env
	steps []string
	// answer is the consent dialog's answer; picked the picker's.
	answer bool
	picked string
}

func newKnowledgeFixture(t *testing.T) *knowledgeFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	// No webview: the build's progress lines go nowhere.
	emit := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = emit })
	reg := knowledgetest.NewRegistry(t)
	tok := reg.Token("ana")
	f := &knowledgeFixture{reg: reg, answer: true}
	f.env = knowledge.Env{
		Home:     func() (string, error) { return home, nil },
		Keys:     &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: tok}[u == reg.URL] },
		HTTP:     reg.Client(),
	}
	app := &App{
		ctx: context.Background(),
		confirm: func(_ context.Context, title, _ string) (bool, error) {
			f.steps = append(f.steps, "ask: "+title)
			return f.answer, nil
		},
		pickDirectory: func(context.Context, string) (string, error) {
			f.steps = append(f.steps, "pick")
			return f.picked, nil
		},
	}
	r := &RunAPI{app: app, knowledgeEnv: &f.env, runnerHomeDir: t.TempDir()}
	f.k = &KnowledgeAPI{app: app, run: r, openFolder: func(dir string) error {
		f.steps = append(f.steps, "reveal: "+dir)
		return nil
	}}
	s, err := f.env.Store(reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Keep(context.Background(), knowledgetest.Make(t, "handbook", nil).Dir); err != nil {
		t.Fatal(err)
	}
	return f
}

// Export asks first, in a native dialog; then the native picker chooses
// the folder; then the pack is written into a new folder named for it
// there, and that folder — never a page — is revealed (D-035). A No, or a
// cancelled picker, writes nothing.
func TestKnowledgeExportAsksThenPicksThenRevealsTheFolder(t *testing.T) {
	f := newKnowledgeFixture(t)
	parent := t.TempDir()
	f.picked = parent

	f.answer = false
	if _, err := f.k.Export(f.reg.URL, "handbook"); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("export answered No: %v", err)
	}
	if strings.Join(f.steps, "|") != "ask: Export handbook unencrypted?" {
		t.Fatalf("export answered No went on to %v", f.steps)
	}

	f.answer, f.picked, f.steps = true, "", nil
	if _, err := f.k.Export(f.reg.URL, "handbook"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("export with the picker cancelled: %v", err)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Fatalf("a refused export wrote %v", entries)
	}

	f.picked, f.steps = parent, nil
	v, err := f.k.Export(f.reg.URL, "handbook")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(parent, "handbook")
	if v.Dir != want || strings.Join(f.steps, "|") != "ask: Export handbook unencrypted?|pick|reveal: "+want {
		t.Fatalf("export = %+v after %v", v, f.steps)
	}
	if _, err := kn.Load(want); err != nil {
		t.Fatalf("the export is not the pack: %v", err)
	}
	// The consent names what leaves the store: the pages and documents,
	// in the clear.
	msg := ""
	f.k.app.confirm = func(_ context.Context, _, m string) (bool, error) { msg = m; return false, nil }
	_, _ = f.k.Export(f.reg.URL, "handbook")
	if !strings.Contains(msg, "in the clear") || !strings.Contains(msg, "not encrypted") {
		t.Fatalf("the export's dialog says %q", msg)
	}
}

// Delete asks in a native dialog, which says when this computer's copy is
// the only one; No keeps the pack.
func TestKnowledgeDeleteAsksNatively(t *testing.T) {
	f := newKnowledgeFixture(t)
	msg := ""
	f.k.app.confirm = func(_ context.Context, _, m string) (bool, error) { msg = m; return false, nil }
	if err := f.k.Delete(f.reg.URL, "handbook", false); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("delete answered No: %v", err)
	}
	if !strings.Contains(msg, "It is the only copy") {
		t.Fatalf("the dialog for the only copy says %q", msg)
	}
	if _, err := f.env.Find(f.reg.URL, "handbook"); err != nil {
		t.Fatalf("a delete answered No removed the pack: %v", err)
	}
	f.k.app.confirm = func(context.Context, string, string) (bool, error) { return true, nil }
	if err := f.k.Delete(f.reg.URL, "handbook", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.Find(f.reg.URL, "handbook"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

// Create and Add take documents only from the native picker; a path the
// page typed is refused before anything is built.
func TestKnowledgeDocumentsMustComeFromTheNativePicker(t *testing.T) {
	f := newKnowledgeFixture(t)
	built := false
	f.k.build = func(context.Context, dockerx.Client, build.Options) (*build.Result, error) {
		built = true
		return nil, errors.New("not reached")
	}
	typed := filepath.Join(t.TempDir(), "typed.txt")
	if _, err := f.k.Create(f.reg.URL, "notes", []string{typed}, "", false); err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
		t.Fatalf("create with a typed path: %v", err)
	}
	if _, err := f.k.Add(f.reg.URL, "handbook", []string{typed}, "", false); err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
		t.Fatalf("add with a typed path: %v", err)
	}
	if built {
		t.Fatal("a build ran on a typed path")
	}
}

// A build is a run: it takes the run lock (so it is refused while a run
// holds it), and is the run in flight while it lasts, under the run id its
// containers are labelled with, so quitting ends it and sweeps them.
func TestKnowledgeBuildIsARunUnderTheRunLock(t *testing.T) {
	f := newKnowledgeFixture(t)
	doc := filepath.Join(t.TempDir(), "guide.txt")
	if err := os.WriteFile(doc, []byte("the guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.k.app.markPicked(doc)
	var during *liveRun
	var runID string
	f.k.build = func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		during, runID = f.k.run.current(), o.RunID
		if err := os.CopyFS(o.PackDir, os.DirFS(knowledgetest.Make(t, o.Name, nil).Dir)); err != nil {
			return nil, err
		}
		p, err := kn.Load(o.PackDir)
		if err != nil {
			return nil, err
		}
		return &build.Result{Documents: []string{"guide.txt"}, Applied: &kn.Result{Manifest: p.Manifest}}, nil
	}

	f.k.run.mu.Lock()
	if _, err := f.k.Create(f.reg.URL, "notes", []string{doc}, "", false); !errors.Is(err, errBusy) {
		t.Fatalf("create while a run holds the lock: %v", err)
	}
	f.k.run.mu.Unlock()

	res, err := f.k.Create(f.reg.URL, "notes", []string{doc}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if during == nil || during.ref != "the Knowledge Creator" || during.id != runID || runID == "" {
		t.Fatalf("the build ran as %+v with run id %q", during, runID)
	}
	if f.k.run.current() != nil {
		t.Fatal("the build is still the run in flight after it returned")
	}
	if res.Name != "notes" || res.Pages != 1 {
		t.Fatalf("result = %+v", res)
	}
}

// StopBuild stops the build in flight and nothing else: the Creator's
// context is cancelled and the build returns; with no build running, or a
// run holding the lock instead, it refuses and cancels nothing
// (agentry-desktop#353).
func TestStopBuildStopsOnlyAKnowledgeBuild(t *testing.T) {
	f := newKnowledgeFixture(t)
	if err := f.k.StopBuild(); err == nil {
		t.Fatal("StopBuild with nothing running passed")
	}
	cancelled := false
	l, err := f.k.run.beginRun(jobRun, "acme/triage@1.0.0", func() { cancelled = true })
	if err != nil {
		t.Fatal(err)
	}
	if err := f.k.StopBuild(); err == nil || cancelled {
		t.Fatalf("StopBuild during a run: %v, cancelled %v", err, cancelled)
	}
	f.k.run.endRun(l)

	doc := filepath.Join(t.TempDir(), "guide.txt")
	if err := os.WriteFile(doc, []byte("the guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.k.app.markPicked(doc)
	started := make(chan struct{})
	f.k.build = func(ctx context.Context, _ dockerx.Client, _ build.Options) (*build.Result, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.k.Create(f.reg.URL, "notes", []string{doc}, "", false)
		done <- err
	}()
	<-started
	if err := f.k.StopBuild(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("the stopped build returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the build did not stop")
	}
	if f.k.run.current() != nil {
		t.Fatal("the stopped build is still the run in flight")
	}
}

// A run with knowledge attached gets each pack opened from the store after
// the gates, and the opened copy is gone once the run returns; a pack that
// will not open is refused before the engine is asked for anything.
func TestARunAttachesKeptKnowledgeAndRemovesItsCopyAfter(t *testing.T) {
	f := newKnowledgeFixture(t)
	r := f.k.run
	var attached []string
	r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		for _, p := range req.Knowledge {
			if _, err := os.Stat(filepath.Join(p.Dir, kn.WikiDir, kn.IndexFile)); err != nil {
				t.Errorf("the engine was handed %s with no index: %v", p.Name, err)
			}
			attached = append(attached, p.Dir)
		}
		return &run.Result{Status: "ok", RunID: req.RunID, Knowledge: "handbook: mounted read-only"}, nil
	}
	v, err := r.RunAgent(f.reg.URL, "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, false, false, []string{"handbook"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(attached) != 1 || v.Knowledge == "" {
		t.Fatalf("attached = %v, result knowledge %q", attached, v.Knowledge)
	}
	if _, err := os.Stat(attached[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the opened copy outlived the run: %v", err)
	}

	called := false
	r.engineRun = func(context.Context, run.Request, io.Writer) (*run.Result, error) { called = true; return nil, nil }
	if _, err := r.RunAgent(f.reg.URL, "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, false, false, []string{"missing"}, false); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("a run with a pack that is not kept: %v", err)
	}
	if called {
		t.Fatal("the engine was asked to run with a pack that would not open")
	}
}

// Quitting mid-run with a pack attached removes the pack's opened copy
// before shutdown returns: the release runs before the run is marked done,
// which is what shutdown waits for (§9 "teardown on every path").
func TestQuittingMidRunRemovesTheAttachedCopyBeforeShutdownReturns(t *testing.T) {
	f := newKnowledgeFixture(t)
	eng := &fakeEngine{started: make(chan run.Request, 1)}
	r := f.k.run
	r.engineRun, r.newRunID = eng.run, func() (string, error) { return "ab12cd34", nil }
	r.sweep = func(context.Context, string) error { return nil }
	errc := make(chan error, 1)
	go func() {
		_, err := r.RunAgent(f.reg.URL, "acme/x@1.0.0", "go", nil, "", nil, 0, -1, false, nil, false, false, []string{"handbook"}, false)
		errc <- err
	}()
	var req run.Request
	select {
	case req = <-eng.started:
	case err := <-errc:
		t.Fatalf("the run never reached the engine: %v", err)
	}
	if len(req.Knowledge) != 1 {
		t.Fatalf("the engine was handed %d packs", len(req.Knowledge))
	}
	r.shutdown()
	if _, err := os.Stat(req.Knowledge[0].Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shutdown returned with the attached copy still in the store: %v", err)
	}
	<-errc
}

// The Documents step sizes only what the native picker returned, walking
// folders, and names both caps (agentry-desktop#354, D-030).
func TestDocumentsSizeSizesOnlyPickedDocuments(t *testing.T) {
	f := newKnowledgeFixture(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("123"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.k.DocumentsSize([]string{dir}); err == nil {
		t.Fatal("a folder the picker did not return was sized")
	}
	f.k.app.markPicked(dir)
	v, err := f.k.DocumentsSize([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if v.Bytes != 8 || v.LocalMaxBytes != kn.MaxLocalBytes || v.AgentryMaxBytes != kn.MaxBytes {
		t.Fatalf("view = %+v", v)
	}
}

// The Documents step's Search the web status (agentry-desktop#498): the
// runtime first, then the AI Review key, then a search provider and its
// key; ready only with all of them, and otherwise a reason that names
// where to fix it.
func TestCreatorWebViewSaysWhatIsMissingInOrder(t *testing.T) {
	pol := policy.Default()
	pol.WebSearch.Provider = "brave"
	for _, c := range []struct {
		name    string
		runtime string
		jev     bool
		search  drafter.SearchState
		ready   bool
		why     string
	}{
		{"all there", "", true, drafter.SearchOn, true, ""},
		{"no jev", "", false, drafter.SearchOn, false, "AI Review key"},
		{"no provider", "", true, drafter.SearchNoProvider, false, "no search provider"},
		{"no key", "", true, drafter.SearchNoKey, false, "no Brave key was found ($BRAVE_API_KEY)"},
		{"runsc", "runsc", true, drafter.SearchOn, false, "docker.resolvers"},
	} {
		p := *pol
		p.Docker.Runtime = c.runtime
		v := creatorWebView(c.jev, c.search, &p)
		if v.Ready != c.ready || !strings.Contains(v.Why, c.why) {
			t.Errorf("%s: %+v", c.name, v)
		}
	}
}
