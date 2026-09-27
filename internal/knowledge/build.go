package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
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
}

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
	en, err := s.Keep(ctx, dir)
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
	en, err := s.Reseal(ctx, req.Name, dir)
	if err != nil {
		return nil, err
	}
	out := buildResult(res, en)
	out.AgentryStale = en.Key.Agentry != nil
	return out, nil
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
	res, err := b(ctx, dockerx.New(), build.Options{
		PackDir: dir, Name: name, Documents: req.Documents,
		Home: home, Policy: pol, PolicySHA: polSHA, PolicyFile: &polFile,
		Build: version.Build(), Now: time.Now(), OutDir: outDir,
		Budgets: &spec.Budgets{TimeoutSeconds: pol.Budgets.TimeoutSeconds, MaxTurns: pol.Budgets.MaxTurns, MaxCostUSD: pol.Budgets.MaxCostUSD},
		RunID:   req.RunID, Stderr: req.Log,
	})
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
func PolicyModel() string {
	polFile, err := policy.Resolve("")
	if err != nil {
		return "none configured"
	}
	pol, _, err := policy.Load(polFile.Path)
	if err != nil {
		return "none configured"
	}
	return CreatorModel(pol)
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

func logf(w io.Writer, format string, a ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format+"\n", a...)
	}
}
