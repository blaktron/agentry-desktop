package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"

	"github.com/blaktron/agentry-desktop/internal/version"
)

// Builder runs the Knowledge Creator over a pack directory: the CLI's
// pkg/knowledge/build, the same package `agentry knowledge create` drives
// (stage, decode, the built-in Creator's run against the policy's model,
// apply). A test substitutes one.
type Builder func(ctx context.Context, d dockerx.Client, o build.Options) (*build.Result, error)

// BuildRequest is one Create knowledge or Add documents.
type BuildRequest struct {
	// Registry is the registry the panel shows: a new pack's key is
	// wrapped by it when this app is signed in to it.
	Registry string
	// Name is the pack.
	Name string
	// Documents are the files and folders to ingest, each one the native
	// picker returned (the binding checks).
	Documents []string
	// RunID names the Creator run's containers, so quitting can sweep them.
	RunID string
	// Build is the Creator seam; nil is pkg/knowledge/build.
	Build Builder
	// Log receives the build's progress, line by line.
	Log io.Writer
	// Progress, when set, is told how far the build has got: at its start,
	// at every usage the harness reports, and whenever the count of pages
	// the Creator has written changes (agentry-desktop#352).
	Progress func(Progress)
}

// CreatorTimeoutSeconds is the least a build on this computer is given
// (agentry-desktop#352): a local model, or a frontier model at high
// reasoning, can take more than 20 minutes to draft a pack, where the
// runner policy's run budget defaults to 15. A policy that allows more
// keeps its own.
const CreatorTimeoutSeconds = 1800

// Progress is how far one build has got, for the panel's building view.
type Progress struct {
	// StartedAt is when the build began, in Unix milliseconds: the panel
	// counts the elapsed time itself.
	StartedAt      int64 `json:"startedAt"`
	TimeoutSeconds int   `json:"timeoutSeconds"`
	MaxTurns       int   `json:"maxTurns"`
	// Turns, InputTokens and OutputTokens are the Creator run's usage as
	// the harness last reported it.
	Turns        int   `json:"turns"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	// Pages counts the pages the Creator has written so far. The apply
	// step checks them after the run, and may drop some.
	Pages int `json:"pages"`
}

// CreatorBudgets are the budgets a build on this computer runs under: the
// runner policy's, with a timeout of at least CreatorTimeoutSeconds.
func CreatorBudgets(pol *policy.Policy) spec.Budgets {
	b := spec.Budgets{TimeoutSeconds: CreatorTimeoutSeconds}
	if pol == nil {
		return b
	}
	b.MaxTurns, b.MaxCostUSD = pol.Budgets.MaxTurns, pol.Budgets.MaxCostUSD
	b.TimeoutSeconds = max(pol.Budgets.TimeoutSeconds, CreatorTimeoutSeconds)
	return b
}

// PolicyBudgets is CreatorBudgets for the policy this app's runs obey,
// read without creating it, for the review step's sentence on how long a
// build may take.
func PolicyBudgets() spec.Budgets { return CreatorBudgets(loadPolicy()) }

// BuildResult is what one build made, for the panel's result.
type BuildResult struct {
	Name      string   `json:"name"`
	Pages     int      `json:"pages"`
	Bytes     int64    `json:"bytes"`
	Documents []string `json:"documents"`
	// Skipped are documents no decoder could read.
	Skipped []string `json:"skipped,omitempty"`
	// Dropped are pages the apply step dropped (or kept) with its reason.
	Dropped []Dropped `json:"dropped,omitempty"`
	// Stopped is the Creator run's own failure when it stopped before it
	// finished and the pack holds what it wrote: Add documents with the
	// same documents continues it.
	Stopped string `json:"stopped,omitempty"`
	// Key says what the pack's data key is wrapped to.
	Key string `json:"key"`
	// AgentryStale says Agentry keeps the copy from before this build.
	AgentryStale bool `json:"agentryStale,omitempty"`
	// Rewrapped names packs this computer's key wrapped that the build's
	// sign-in moved to the account's key first (D20).
	Rewrapped []string `json:"rewrapped,omitempty"`
	// The Creator run's usage, as its receipt measured it.
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
}

// Dropped is one page the apply step did not take as written.
type Dropped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Kept   bool   `json:"kept,omitempty"`
}

// Create makes a new pack from the documents with the Creator on this
// computer and keeps it sealed: `agentry knowledge create`'s steps
// (createKept, in the CLI's package main, which this cannot import) over
// the same library calls. The pack is built in a 0700 directory inside the
// store, with the Creator run's output, whose reports/ holds every page it
// wrote; both go when the pack is kept, or when the build fails. Where the
// CLI leaves a pack it could not keep sealed in that directory for
// `knowledge import`, this removes it: the app has no import, and its
// pages would rest there in the clear. Nothing the Creator reads or writes
// is scanned, and the pack records "not scanned" (D19): the panel says so
// before the build.
func (e Env) Create(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if err := kn.ValidName(req.Name); err != nil {
		return nil, err
	}
	s, err := e.Store(req.Registry)
	if err != nil {
		return nil, err
	}
	if _, err := s.Find(req.Name); err == nil {
		return nil, fmt.Errorf("a knowledge pack named %s is already kept on this computer; add documents to it instead", req.Name)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	rewrapped, rerr := e.Rewrap(ctx, req.Registry)
	if rerr != nil {
		logf(req.Log, "warning: %v", rerr)
	}
	scratch, done, err := s.Scratch()
	if err != nil {
		return nil, err
	}
	defer done()
	dir := filepath.Join(scratch, req.Name)
	res, err := e.runBuild(ctx, req, dir, req.Name, filepath.Join(scratch, "creator-run"))
	if err != nil {
		return nil, err
	}
	sealCtx, cancel := sealContext(ctx)
	defer cancel()
	en, err := s.Keep(sealCtx, dir)
	if err != nil {
		return nil, fmt.Errorf("the pack was built but could not be kept sealed, so it was not kept: %w", err)
	}
	out := buildResult(res, en)
	out.Rewrapped = rewrapped
	return out, nil
}

// Add extends a kept pack with the documents (the CLI's addKept): the pack
// is opened into a 0700 directory inside the store, the Creator reads it as it stands and
// rewrites the pages the documents touch, and the result is sealed back
// under the key it had, so its wrapped key, and Agentry's copy, still fit.
func (e Env) Add(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if err := kn.ValidName(req.Name); err != nil {
		return nil, err
	}
	s, err := e.Store(req.Registry)
	if err != nil {
		return nil, err
	}
	kept, err := s.Find(req.Name)
	if err != nil {
		return nil, err
	}
	if kept.Key.Org != "" {
		return nil, fmt.Errorf("add documents to knowledge pack %s: %w", req.Name, store.ErrShared)
	}
	scratch, done, err := s.Scratch()
	if err != nil {
		return nil, err
	}
	defer done()
	dir := filepath.Join(scratch, req.Name)
	if _, err := s.Open(ctx, req.Name, dir); err != nil {
		return nil, err
	}
	res, err := e.runBuild(ctx, req, dir, "", filepath.Join(scratch, "creator-run"))
	if err != nil {
		return nil, err
	}
	sealCtx, cancel := sealContext(ctx)
	defer cancel()
	en, err := s.Reseal(sealCtx, req.Name, dir)
	if err != nil {
		return nil, err
	}
	out := buildResult(res, en)
	out.AgentryStale = en.Key.Agentry != nil
	return out, nil
}

// sealTimeout bounds keeping a built pack once the build's own context may
// be over; a test shortens it.
var sealTimeout = 2 * time.Minute

// sealContext is the context a built pack is kept sealed under. A build
// stopped by StopBuild has its context cancelled, and the pages it wrote
// were applied; sealing them needs the registry's wrap or unwrap, which a
// cancelled context would fail at once, losing them. So the seal outlives
// the build's cancellation, bounded by sealTimeout (agentry-desktop#353).
func sealContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), sealTimeout)
}

// runBuild runs the Creator over dir against the runner policy's model,
// the budgets the policy sets, as `agentry knowledge create` does.
func (e Env) runBuild(ctx context.Context, req BuildRequest, dir, name, outDir string) (*build.Result, error) {
	if len(req.Documents) == 0 {
		return nil, errors.New("choose the documents to ingest first")
	}
	home, err := e.Home()
	if err != nil {
		return nil, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, polSHA, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	// Said before anything is built or read, in the log as on the review
	// step (plan D19).
	logf(req.Log, "The Knowledge Creator runs on this computer against the model your runner policy names (%s). "+
		"It costs no credits, and nothing it reads or writes is scanned.", CreatorModel(pol))
	b := req.Build
	if b == nil {
		b = build.Build
	}
	budgets := CreatorBudgets(pol)
	turns := ""
	if budgets.MaxTurns > 0 {
		turns = fmt.Sprintf(" and %d turns", budgets.MaxTurns)
	}
	logf(req.Log, "A build takes a while: it may run for up to %d minutes%s.", budgets.TimeoutSeconds/60, turns)
	prog := newProgressMeter(req.Progress, budgets, filepath.Join(outDir, "reports", kn.WikiDir))
	stop := prog.watch()
	defer stop()
	res, err := b(ctx, dockerx.New(), build.Options{
		PackDir: dir, Name: name, Documents: req.Documents,
		Home: home, Policy: pol, PolicySHA: polSHA, PolicyFile: &polFile,
		Build: version.Build(), Now: time.Now(), OutDir: outDir,
		Budgets: &budgets,
		RunID:   req.RunID, Stderr: req.Log, OnUsage: prog.usage,
	})
	stop()
	if err != nil {
		if res != nil && len(res.Skipped) > 0 {
			logf(req.Log, "not read (no decoder reads them, or the decoder failed): %s", strings.Join(res.Skipped, ", "))
		}
		return nil, err
	}
	return res, nil
}

// CreatorModel names the model the policy gives the Creator, as the review
// step and the log say it.
func CreatorModel(pol *policy.Policy) string {
	if pol == nil {
		return "none configured"
	}
	if m := strings.TrimSpace(pol.Model.Provider + " " + pol.Model.Name); m != "" {
		return m
	}
	return "none configured"
}

// PolicyModel is CreatorModel for the policy this app's runs obey, read
// without creating it.
func PolicyModel() string { return CreatorModel(loadPolicy()) }

// loadPolicy is the policy this app's runs obey, read without creating
// it; nil when there is none or it cannot be read.
func loadPolicy() *policy.Policy {
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil
	}
	pol, _, err := policy.Load(polFile.Path)
	if err != nil {
		return nil
	}
	return pol
}

func buildResult(res *build.Result, en store.Entry) *BuildResult {
	out := &BuildResult{
		Name: en.Name(), Pages: en.Manifest.Pages, Bytes: en.Manifest.Bytes,
		Documents: append([]string{}, res.Documents...), Skipped: res.Skipped, Key: KeyHolder(en),
	}
	if res.Applied != nil {
		for _, d := range res.Applied.Dropped {
			out.Dropped = append(out.Dropped, Dropped{Path: d.Path, Reason: d.Reason, Kept: d.Kept})
		}
	}
	if res.RunErr != nil {
		out.Stopped = res.RunErr.Error()
	}
	if env := res.Envelope; env != nil {
		out.InputTokens, out.OutputTokens, out.CostUSD = env.Usage.InputTokens, env.Usage.OutputTokens, env.Usage.CostUSD
	}
	return out
}

// progressMeter tells a build's Progress what it has done: the harness's
// usage as it arrives, and the pages written, counted under the run's
// reports/wiki, which the run's output mount fills as the Creator writes.
type progressMeter struct {
	report func(Progress)
	wiki   string
	mu     sync.Mutex
	p      Progress
}

// progressEvery is how often the pages written are counted; a test
// shortens it.
var progressEvery = 2 * time.Second

func newProgressMeter(report func(Progress), b spec.Budgets, wiki string) *progressMeter {
	return &progressMeter{report: report, wiki: wiki, p: Progress{
		StartedAt: time.Now().UnixMilli(), TimeoutSeconds: b.TimeoutSeconds, MaxTurns: b.MaxTurns,
	}}
}

// watch reports the start, then counts the pages until the returned stop
// is called; stop counts once more, so the last page written is reported.
func (m *progressMeter) watch() (stop func()) {
	if m.report == nil {
		return func() {}
	}
	m.emit()
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(progressEvery)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				m.countPages()
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			wg.Wait()
			m.countPages()
		})
	}
}

func (m *progressMeter) usage(u receipt.Usage) {
	if m.report == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.p.Turns, m.p.InputTokens, m.p.OutputTokens = u.Turns, u.InputTokens, u.OutputTokens
	m.report(m.p)
}

func (m *progressMeter) countPages() {
	n := 0
	// A page as the apply step will read it: a regular .md file that is
	// not an index or the log, which the application writes itself.
	// WalkDir follows no link, so a link the run left is not walked.
	_ = filepath.WalkDir(m.wiki, func(rel string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".md") &&
			d.Name() != kn.IndexFile && rel != filepath.Join(m.wiki, kn.LogFile) {
			n++
		}
		return nil
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	if n != m.p.Pages {
		m.p.Pages = n
		m.report(m.p)
	}
}

// emit reports under the lock, so two reports from the usage and the
// counting goroutines arrive in the order they were made.
func (m *progressMeter) emit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.report(m.p)
}

func logf(w io.Writer, format string, a ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format+"\n", a...)
	}
}
