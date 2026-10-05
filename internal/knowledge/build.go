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

	"github.com/blaktron/agentry-cli/pkg/drafter"
	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"

	"github.com/blaktron/agentry-desktop/internal/progress"
	"github.com/blaktron/agentry-desktop/internal/settings"
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
	// Brief is what the pack should cover and for whom, in the person's
	// words, at most build.MaxBriefBytes: a build takes documents, a brief
	// or both (agentry-desktop#498; plan knowledge-and-drafter.md D15). The
	// CLI keeps it in the pack as sources/brief.md.
	Brief string
	// Web is the person's Search the web choice (D16): the Creator may
	// search, every result read by AI Review first with the person's own
	// Jev key. Without that key or a search key the build runs without the
	// web and says why, rather than failing.
	Web bool
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

// Progress is how far one build has got, for the panel's building view:
// the report every job carries (its start, its budgets, the Creator run's
// usage), and the pages it has written.
type Progress struct {
	progress.Report
	// Pages counts the pages the Creator has written so far. The apply
	// step checks them after the run, and may drop some.
	Pages int `json:"pages"`
}

// CreatorBudgets are the budgets a build on this computer runs under: the
// runner policy's, with the Knowledge Creator's own timeout floor, 60
// minutes (agentry-desktop#458). A local model, or a frontier model at
// high reasoning, takes far longer to draft a pack than the policy's run
// budget, 15 minutes by default, allows; a policy that allows more keeps
// its own. The floor is the CLI's (localrun.Builtin.LocalBudgets, the
// CLI's DR-54), not a figure kept here, so a build gets the same time from
// this app as from `agentry knowledge create`. It is a timeout alone: the
// turn and cost caps are the policy's, and no budget of a build counts
// tokens. Run defaults' no-time-limit switch for the built-in agents takes the timeout away
// (settings.BuiltinBudgets, D-062).
func CreatorBudgets(pol *policy.Policy) spec.Budgets {
	var b policy.Budgets
	if pol != nil {
		b = pol.Budgets
	}
	return settings.BuiltinBudgets(localrun.KnowledgeCreator(), b)
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
	// Brief says the build was given a brief, which Documents lists by the
	// name the pack cites it under (brief.md, or brief-N.md): the panel
	// names it as the brief, never as one more document
	// (agentry-desktop#600).
	Brief bool `json:"brief,omitempty"`
	// Skipped are documents no decoder could read.
	Skipped []string `json:"skipped,omitempty"`
	// Dropped are pages the apply step dropped (or kept) with its reason.
	Dropped []Dropped `json:"dropped,omitempty"`
	// WithoutWeb says why a build asked to search the web ran without it:
	// no Jev key, no search provider or key, or a sandboxed runtime with no
	// name servers for the tools.
	WithoutWeb string `json:"withoutWeb,omitempty"`
	// Searched says the Creator was given the web tools.
	Searched bool `json:"searched,omitempty"`
	// Stopped is the Creator run's own failure when it stopped before it
	// finished and the pack holds what it wrote: Add documents with the
	// same documents, brief or both continues it.
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
	out := buildResult(res, en, strings.TrimSpace(req.Brief) != "")
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
	out := buildResult(res, en, strings.TrimSpace(req.Brief) != "")
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
func (e Env) runBuild(ctx context.Context, req BuildRequest, dir, name, outDir string) (*built, error) {
	req.Brief = strings.TrimSpace(req.Brief)
	if err := CheckSources(req.Documents, req.Brief); err != nil {
		return nil, err
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
	if len(req.Documents) == 0 {
		logf(req.Log, "%s", NoDocumentsWarning)
	}
	web, withoutWeb, err := creatorWeb(pol, req.Web)
	if err != nil {
		return nil, err
	}
	switch {
	case web != nil:
		logf(req.Log, "The Creator may search the web with %s and your key; AI Review reads every result first, with your AI Review key. "+
			"Searching can raise what a build costs. It searches only for what the documents and the pack do not hold.", pol.WebSearch.ProviderName())
	case withoutWeb != "":
		logf(req.Log, "The Creator builds without the web: %s.", withoutWeb)
	}
	b := req.Build
	if b == nil {
		b = build.Build
	}
	budgets := CreatorBudgets(pol)
	turns := ""
	if budgets.MaxTurns > 0 {
		turns = fmt.Sprintf(" and %d turns", budgets.MaxTurns)
	}
	switch {
	case budgets.TimeoutSeconds > 0:
		logf(req.Log, "A build takes a while: it may run for up to %d minutes%s.", budgets.TimeoutSeconds/60, turns)
	case budgets.MaxTurns > 0:
		logf(req.Log, "A build takes a while: Run defaults gives the built-in agents no time limit, so it runs until it is done or reaches %d turns.", budgets.MaxTurns)
	default:
		logf(req.Log, "A build takes a while: Run defaults gives the built-in agents no time limit, so it runs until it is done.")
	}
	if total := DocumentsBytes(req.Documents); total > kn.MaxBytes {
		logf(req.Log, "The documents come to %.1f MB. A build that size can take a very long time, and the pack will stay on this computer: Agentry keeps packs of at most %s.",
			float64(total)/(1<<20), kn.FormatCap(kn.MaxBytes))
	}
	prog := newProgressMeter(req.Progress, budgets, filepath.Join(outDir, "reports", kn.WikiDir))
	stop := prog.watch()
	defer stop()
	res, err := b(ctx, dockerx.New(), build.Options{
		PackDir: dir, Name: name, Documents: req.Documents, Brief: req.Brief, Web: web,
		Home: home, Policy: pol, PolicySHA: polSHA, PolicyFile: &polFile,
		Build: version.Build(), Now: time.Now(), OutDir: outDir,
		Budgets: &budgets,
		RunID:   req.RunID, Stderr: req.Log, OnUsage: prog.Usage,
		// A pack made on this computer may hold up to 1 GB, documents
		// included; Agentry keeps packs of at most 10 MB (agentry-desktop#354,
		// agentry-cli#429).
		MaxBytes: kn.MaxLocalBytes,
	})
	stop()
	if err != nil {
		if res != nil && len(res.Skipped) > 0 {
			logf(req.Log, "not read (no decoder reads them, or the decoder failed): %s", strings.Join(res.Skipped, ", "))
		}
		return nil, err
	}
	return &built{Result: res, withoutWeb: withoutWeb, searched: web != nil}, nil
}

// built is one build's result with what became of its Search the web
// choice.
type built struct {
	*build.Result
	withoutWeb string
	searched   bool
}

// CheckSources refuses a build given neither documents nor a brief, or a
// brief past build.MaxBriefBytes (agentry-desktop#498), on this computer and
// In Agentry alike, in the words the Documents step uses.
func CheckSources(documents []string, brief string) error {
	err := build.CheckBrief(documents, strings.TrimSpace(brief))
	if errors.Is(err, build.ErrNothingToBuild) {
		return errors.New("choose documents, write a brief that says what the pack should cover, or both")
	}
	return err
}

// NoDocumentsWarning is what a build with no documents is told, in the log
// as on the Documents step (plan knowledge-and-drafter.md §7).
const NoDocumentsWarning = "Specific documents make the best knowledge. Without them the Creator researches the web and its own knowledge, which can be wrong or out of date."

// localWeb is pkg/drafter's LocalWeb, the person's own keys as work on
// this computer reads them; a test substitutes one.
var localWeb = drafter.LocalWeb

// creatorWeb is a build's web configuration (the CLI's knowledgeWeb, in
// its package main, which this cannot import): nil when the person did not
// ask for the web, and nil with the reason when they did and this computer
// cannot give it — no Jev key to scan every result with, no search provider
// or key, or a sandboxed runtime with no name servers for the tools. Only a
// policy or keychain fault is an error.
func creatorWeb(pol *policy.Policy, search bool) (*engine.WebRun, string, error) {
	if !search {
		return nil, "", nil
	}
	if why := WebUnavailable(pol); why != "" {
		return nil, why, nil
	}
	w, state, err := localWeb(pol, "")
	switch {
	case errors.Is(err, drafter.ErrNoJevKey):
		return nil, WebReason(pol, false, state), nil
	case err != nil:
		return nil, "", err
	}
	if why := WebReason(pol, true, state); why != "" {
		return nil, why, nil
	}
	return w, "", nil
}

// WebReason is why a build on this computer runs without the web, "" when
// it can search: the runtime first, then the person's own AI Review key
// (jev), which reads every result first, then a search provider and its
// key (search). The build's log and result and the Documents step
// (KnowledgeAPI.CreatorWeb, from presence alone) say the same words.
func WebReason(pol *policy.Policy, jev bool, search drafter.SearchState) string {
	switch {
	case WebUnavailable(pol) != "":
		return WebUnavailable(pol)
	case !jev:
		return "AI Review reads every result first, and that takes your own AI Review key: add one under Settings → Provider settings → AI Review key"
	case search == drafter.SearchNoProvider:
		return "no search provider is set up: choose one under Settings → Provider settings → Web search"
	case search == drafter.SearchNoKey:
		return fmt.Sprintf("no %s key was found ($%s): add one under Settings → Provider settings → Web search", pol.WebSearch.ProviderName(), pol.WebSearch.KeyEnv())
	}
	return ""
}

// WebUnavailable is why the runtime alone keeps a build here off the web,
// "" when it does not: under a sandboxed runtime the web tools need the
// host's name servers in the runner policy's docker.resolvers.
func WebUnavailable(pol *policy.Policy) string {
	if pol == nil {
		return ""
	}
	if rt := pol.Docker.Runtime; rt != "" && len(pol.Docker.Resolvers) == 0 {
		return fmt.Sprintf("under the %s runtime the web tools need the host's name servers in your runner policy's docker.resolvers", rt)
	}
	return ""
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

func buildResult(res *built, en store.Entry, brief bool) *BuildResult {
	out := &BuildResult{
		Name: en.Name(), Pages: en.Manifest.Pages, Bytes: en.Manifest.Bytes,
		Documents: append([]string{}, res.Documents...), Brief: brief, Skipped: res.Skipped, Key: KeyHolder(en),
		WithoutWeb: res.withoutWeb, Searched: res.searched,
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

// DocumentsBytes is the size of the regular files the paths name, walking
// folders and following no link, for the Documents step's total and the
// warning a large build gets (agentry-desktop#354). A path it cannot read
// counts nothing here; the build says why.
func DocumentsBytes(paths []string) int64 {
	var total int64
	for _, p := range paths {
		_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					total += info.Size()
				}
			}
			return nil
		})
	}
	return total
}

// progressMeter tells a build's Progress what it has done: the harness's
// usage as it arrives (the shared meter), and the pages written, counted
// under the run's reports/wiki, which the run's output mount fills as the
// Creator writes.
type progressMeter struct {
	*progress.Meter[Progress, *Progress]
	wiki string
}

// progressEvery is how often the pages written are counted; a test
// shortens it.
var progressEvery = 2 * time.Second

func newProgressMeter(report func(Progress), b spec.Budgets, wiki string) *progressMeter {
	return &progressMeter{
		Meter: progress.NewMeter(report, Progress{Report: progress.Start(b.TimeoutSeconds, b.MaxTurns)}),
		wiki:  wiki,
	}
}

// watch reports the start, then counts the pages until the returned stop
// is called; stop counts once more, so the last page written is reported.
func (m *progressMeter) watch() (stop func()) {
	if !m.Reporting() {
		return func() {}
	}
	m.Emit()
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
	m.Set(func(p *Progress) bool {
		if n == p.Pages {
			return false
		}
		p.Pages = n
		return true
	})
}

func logf(w io.Writer, format string, a ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format+"\n", a...)
	}
}
