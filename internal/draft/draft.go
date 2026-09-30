// Package draft is Describe it, on this computer (plan builder-rework.md
// §7.1, §7.3, D22, D25; #282): the built-in Drafter run here against the
// runner policy's model, with every input read by AI Review with the
// person's own Jev key before the model reads it, for no credits. It drives
// the CLI's own packages: pkg/drafter's Task, LocalWeb, Prescan and
// PostStep, and localrun's Drafter and RepairRun. The order is the plan's
// and this package keeps it: no key, no draft (Prepare); the description
// scanned before the Drafter starts (Run); the answer finished as `agentry
// draft` finishes it, by the same code (agentry-cli#478; #395: corrected,
// validated, checked by AI Review with the person's key, and given one
// repair turn when the rules refuse it or the check flags something it can
// fix, plan builder-simplify.md §5.2, D6); and the draft decoded into the
// Build form's Model as data (build.FromDraft).
package draft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/progress"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// Plan is a draft on this computer, checked before anything leaves it: the
// task the Drafter is given, the runner policy, and the run's web
// configuration with the person's own keys.
type Plan struct {
	// Description is the person's own description, which AI Review's check
	// of the finished draft reads; Task is it with the hints, as the
	// Drafter is given it.
	Description string
	Task        string
	Policy      *policy.Policy
	PolicySHA   string
	PolicyFile  policy.File
	Web         *engine.WebRun
	Search      drafter.SearchState
}

// Budgets are the budgets the draft runs under: the runner policy's, with
// the Drafter's own timeout floor, 30 minutes (agentry-desktop#458). The
// policy's run budget, 15 minutes by default, is sized for an agent
// answering a task, and a draft that researches outruns it; a policy that
// allows more keeps its own. The floor is the CLI's
// (localrun.Builtin.LocalBudgets, the CLI's DR-54), so a draft gets the
// same time here as from `agentry draft`. It is a timeout alone: the turn and cost
// caps are the policy's, and no budget of a draft counts tokens.
func (p *Plan) Budgets() spec.Budgets { return localrun.Drafter().LocalBudgets(p.Policy.Budgets) }

// ModelName names the model the runner policy gives the Drafter.
func (p *Plan) ModelName() string { return modelName(p.Policy) }

// modelName names the model pol gives a run, as the log and the page say it.
func modelName(pol *policy.Policy) string {
	if m := strings.TrimSpace(pol.Model.Provider + " " + pol.Model.Name); m != "" {
		return m
	}
	return "none configured"
}

// SearchSentence says what web search a draft has under pol.
func SearchSentence(s drafter.SearchState, pol *policy.Policy) string {
	switch s {
	case drafter.SearchOn:
		return fmt.Sprintf("Its searches go to %s with your key.", providerName(pol.WebSearch.Provider))
	case drafter.SearchNoKey:
		return fmt.Sprintf("It has no web search: no key for %s was found ($%s, or the keychain when the policy says so).",
			providerName(pol.WebSearch.Provider), pol.WebSearch.KeyEnv())
	}
	return "It has no web search, since your runner policy names no search provider, so it reads the registry and the pages it knows."
}

// providerName is a search provider's name as a person knows it.
func providerName(p string) string {
	if p == "youcom" {
		return "You.com"
	}
	return p
}

// Prepare checks a draft before anything leaves this computer: the
// description and hints as the Drafter's task (at most 32 KiB, text), the
// runner policy, and the person's keys read as the policy says. Without a
// Jev key it refuses, saying where to add one, and nothing else happens
// (D22, D25).
func Prepare(registry, description string, hints []string) (*Plan, error) {
	task, err := drafter.Task(description, hints)
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
	w, search, err := drafter.LocalWeb(pol, registry)
	switch {
	case errors.Is(err, drafter.ErrNoJevKey):
		return nil, fmt.Errorf("drafting on this computer needs your own TypeSafe Jev key, because AI Review reads your description and everything the draft reads with it before the model does. Add one under Settings → Provider settings → AI Review key: %w", err)
	case err != nil:
		return nil, err
	}
	return &Plan{Description: description, Task: task, Policy: pol, PolicySHA: polSHA, PolicyFile: polFile, Web: w, Search: search}, nil
}

// Request is one draft's run.
type Request struct {
	Plan  *Plan
	RunID string
	// Namespace is the person's, which the draft is published under.
	Namespace string
	Log       io.Writer
	// Scanner builds the description's scanner from the Jev key; nil is
	// the core's pkg/judge through drafter.CoreJev.
	Scanner func(key string) (webscan.Scanner, error)
	// Runner runs the Drafter, its first turn and its one repair turn; nil
	// is localrun on this machine's docker.
	Runner func(context.Context, localrun.Run) (*receipt.Envelope, error)
	// OnRepairTurn is told the repair turn's run id before it starts, so the
	// caller's crash path can record and sweep it as it does the draft's
	// own; nil tells nobody.
	OnRepairTurn func(runID string)
	// Checker builds AI Review's check of the finished draft from the run's
	// web configuration; nil is drafter.CheckerFor, the core's
	// pkg/draftcheck with the person's own Jev key.
	Checker func(*engine.WebRun) (drafter.Checker, error)
	// Progress, when set, is told how far the draft has got
	// (agentry-desktop#410): at its start, as each stage begins, and at
	// every usage the harness reports; nil tells nobody.
	Progress func(Progress)
}

// Stage is where a draft on this computer is. It goes through them in
// order; the repair turn and the check after it only when the post-step
// gives one.
type Stage string

const (
	// StageDescription: AI Review reads the description, with the
	// person's key, before anything else.
	StageDescription Stage = "description"
	// StageDrafting: the Drafter's turn, its searches and reads each read
	// by AI Review first.
	StageDrafting Stage = "drafting"
	// StageChecking: the rules and AI Review's check of what it answered.
	StageChecking Stage = "checking"
	// StageRepair: the one repair turn, without the web tools.
	StageRepair Stage = "repair"
)

// Progress is how far one draft has got, for the progress dialog and its
// corner card: the report every job carries, and the stage the draft is in.
// Its budgets are Plan.Budgets (the runner policy's, with at least 30
// minutes), and the repair turn runs inside what is left of them; its usage
// counts the repair turn's on from the first turn's.
type Progress struct {
	progress.Report
	Stage Stage `json:"stage"`
}

// atStage moves a draft's Progress to s.
func atStage(s Stage) func(*Progress) bool {
	return func(p *Progress) bool {
		p.Stage = s
		return true
	}
}

// Result is a finished draft: the draft as `agentry draft` records it,
// decoded into the Build form's Model, with what the decode left out, and
// the folder holding the run's receipts and draft.json.
type Result struct {
	Draft  *drafter.Draft
	Model  *build.Model
	Notes  []string
	OutDir string
}

// Run makes the draft: AI Review reads the description first and one it
// does not pass stops the draft before the Drafter starts; then the
// Drafter runs, and its answer goes through the CLI's own post-step
// (drafter.PostStep): AI Review's record from the run's ledger, the long
// texts from its reports, the slips corrected, the package rendered beside
// the receipts and checked by `agentry validate`'s own code, AI Review's
// check of it with the person's key, and one repair turn, without the web
// tools and given only the rules' report, what the check flagged (with the
// task) and the Drafter's own manifest and instructions, for a draft the
// rules refuse or the check flagged for something it can fix. The draft
// kept is written as draft.json beside the receipts and decoded into the
// form's Model.
func Run(ctx context.Context, req Request) (*Result, error) {
	p := req.Plan
	logw := req.Log
	if logw == nil {
		logw = io.Discard
	}
	b := p.Budgets()
	prog := progress.NewMeter(req.Progress, Progress{Report: progress.Start(b.TimeoutSeconds, b.MaxTurns)})
	prog.Set(atStage(StageDescription))
	scanner := req.Scanner
	if scanner == nil {
		scanner = drafter.CoreJev
	}
	sc, err := scanner(p.Web.JevKey)
	if err != nil {
		return nil, fmt.Errorf("AI Review cannot scan with this key: %w", err)
	}
	desc, err := drafter.Prescan(ctx, sc, p.Task)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(logw, "AI Review passed the description. The Drafter runs on this computer against the model your runner policy names (%s), for no credits. %s "+
		"Every result goes to AI Review with your key before the model reads it, and this computer reads each page from its site. It submits nothing.\n",
		p.ModelName(), SearchSentence(p.Search, p.Policy))

	home, err := run.Home()
	if err != nil {
		return nil, err
	}
	outDir, err := run.OutputDir("", drafter.Namespace, drafter.Name, req.RunID)
	if err != nil {
		return nil, err
	}
	runner := req.Runner
	if runner == nil {
		runner = func(ctx context.Context, r localrun.Run) (*receipt.Envelope, error) {
			return r.Execute(ctx, dockerx.New())
		}
	}
	first := localrun.Run{
		Builtin: localrun.Drafter(), Home: home, Policy: p.Policy, PolicySHA: p.PolicySHA, PolicyFile: &p.PolicyFile,
		Build: version.Build(), Task: strings.NewReader(p.Task), OutDir: outDir,
		Budgets: &b,
		RunID:   req.RunID, Stderr: logw, Web: p.Web, OnUsage: prog.Usage,
	}
	prog.Set(atStage(StageDrafting))
	env, runErr := runner(ctx, first)
	prog.Set(atStage(StageChecking))
	checkerFor := req.Checker
	if checkerFor == nil {
		checkerFor = drafter.CheckerFor
	}
	checker, checkerErr := checkerFor(p.Web)
	post := drafter.PostStep{
		OutDir: outDir, Description: p.Description, Task: p.Task, Scan: desc, Ref: localrun.Drafter().Ref(), Build: version.Build(),
		Budgets: *first.Budgets, Checker: checker, CheckerErr: checkerErr, Log: logw,
		Repair: func(ctx context.Context, t drafter.RepairTurn) (*receipt.Envelope, error) {
			if req.OnRepairTurn != nil {
				req.OnRepairTurn(t.RunID)
			}
			prog.Set(atStage(StageRepair))
			defer prog.Set(atStage(StageChecking))
			return runner(ctx, localrun.RepairRun(first, t))
		},
		OnUsage: prog.Usage,
		// The embedded rule bundle, as the desktop's own Validate uses: the
		// app has no rules checkout or --rules-update cache of its own
		// (DR-43's addendum names the difference from `agentry draft`).
		Rules: nil,
	}
	d, _, err := post.Run(ctx, env, runErr)
	switch {
	case err != nil && runErr != nil:
		// The run failed, or its answer was not a draft and its repair
		// turn returned none either.
		var ce *engine.ClassifiedError
		if errors.As(err, &ce) && ce.Code == engine.ExitOutputSchema {
			return nil, fmt.Errorf("the Drafter's answer was not a draft, so none was made; its answer and the run's receipts are in %s: %w", outDir, err)
		}
		return nil, err
	case err != nil:
		return nil, fmt.Errorf("the Drafter's answer was not a draft (its receipts are in %s): %w", outDir, err)
	}
	// The texts the page renders beside Review are held to the bounds a
	// hosted draft's are (Decode), so both ways in show the same draft.
	if err := holdDraft(d); err != nil {
		return nil, fmt.Errorf("the draft could not be opened in the form (it is in %s): %w", outDir, err)
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "draft.json"), append(raw, '\n'), 0o644); err != nil { //nolint:gosec // the run's own output folder
		return nil, fmt.Errorf("write draft.json: %w", err)
	}
	m, notes, err := build.FromDraft(d.Manifest, d.Instructions, req.Namespace)
	if err != nil {
		return nil, fmt.Errorf("the draft could not be opened in the form (it is in %s): %w", outDir, err)
	}
	return &Result{Draft: d, Model: m, Notes: notes, OutDir: outDir}, nil
}

// Status is what Describe it, on this computer says before a draft: where
// the Jev key would come from, what search the draft would have and which
// model it would run against. It reads no key (presence markers and
// whether a variable is set) and creates no policy.
type Status struct {
	Key    *run.JevKeyStatus
	Search drafter.SearchState
	// SearchSentence says it in words; ModelName names the policy's model.
	SearchSentence string
	ModelName      string
}

// keyPresent reports whether the keychain holds provider's entry, from its
// presence marker; a seam, so the status is testable without a keychain.
var keyPresent = modelkey.Present

// GetStatus reads Status.
func GetStatus() (*Status, error) {
	key, err := run.GetJevKeyStatus()
	if err != nil {
		return nil, err
	}
	pol, err := run.PolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	// pkg/drafter's search rule (a provider named, and its key in the
	// variable or, for a keyring source, the keychain), from presence alone.
	search := drafter.SearchNoProvider
	if w := pol.WebSearch; w.On() {
		search = drafter.SearchNoKey
		if strings.TrimSpace(os.Getenv(w.KeyEnv())) != "" {
			search = drafter.SearchOn
		} else if w.APIKeySource == modelkey.SourceKeyring {
			if ok, _ := keyPresent(w.Provider); ok {
				search = drafter.SearchOn
			}
		}
	}
	return &Status{Key: key, Search: search, SearchSentence: SearchSentence(search, pol), ModelName: modelName(pol)}, nil
}
