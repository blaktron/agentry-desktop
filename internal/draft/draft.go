// Package draft is Describe it, on this computer: the built-in Drafter run here
// against the runner policy's model, with every input read by AI Review with
// the person's own Jev key before the model reads it, for no credits. It drives
// the CLI's own packages: pkg/drafter's Task, LocalWeb, Prescan and PostStep,
// and localrun's Drafter and RepairRun. This package keeps this order: no key,
// no draft (Prepare); the description scanned before the Drafter starts (Run);
// the answer finished as `agentry draft` finishes it, by the same code
// (corrected, validated, checked by AI Review with the person's key, and given
// one repair turn when the rules refuse it or the check flags something it can
// fix); and the draft decoded into the Build form's Model as data
// (build.FromDraft).
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
	"github.com/blaktron/agentry-cli/runnerimage/webscan"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/progress"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
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
	// Knowledge are the packs kept on this computer the Drafter reads
	// first, opened by knowledge.Env.ForDraft; nil for none. Its caller
	// removes them.
	Knowledge *knowledge.DraftPacks
	// Revision is the release Edit with AI revises on this computer,
	// staged in the run's own folder under revise/ (drafter.ReadRelease)
	// with the person's two texts; nil for a draft from a description.
	Revision *drafter.Revision
}

// Budgets are the budgets the draft runs under: the runner policy's, with the
// Drafter's own timeout floor, 30 minutes. The policy's run budget, 15 minutes
// by default, is sized for an agent answering a task, and a draft that
// researches outruns it; a policy that allows more keeps its own. The floor is
// the CLI's (localrun.Builtin.LocalBudgets), so a draft gets the same time
// here as from `agentry draft`. No budget of a draft counts tokens. That is
// with Run defaults' no-limit switch for the built-in agents off; on, the
// default, the draft has no timeout and runs to the hidden failsafe of turns,
// and Shown says no limit (settings.BuiltinBudgets). The cost cap is the
// policy's either way.
func (p *Plan) Budgets() settings.BuiltinLimits {
	return settings.BuiltinBudgets(localrun.Drafter(), p.Policy.Budgets)
}

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
		return fmt.Sprintf("Its searches go to %s with your key.", pol.WebSearch.ProviderName())
	case drafter.SearchNoKey:
		return fmt.Sprintf("It has no web search: no key for %s was found ($%s, or the keychain when the policy says so).",
			pol.WebSearch.ProviderName(), pol.WebSearch.KeyEnv())
	}
	return "It has no web search, since your runner policy names no search provider, so it reads the registry and the pages it knows."
}

// Prepare checks a draft before anything leaves this computer: the
// description and hints as the Drafter's task (at most 32 KiB, text), with
// the names of the packs it reads first when it reads any, the runner
// policy, and the person's keys read as the policy says. Without a Jev key
// it refuses, saying where to add one, and nothing else happens: a pack not
// scanned in Agentry is scanned with that key too.
//
// ships are the packs, among those it reads, the person chose to ship with
// the QuickAgent: the task tells the Drafter that every run of the
// QuickAgent reads them at /knowledge/<name>, so it writes the instructions
// to use them.
func Prepare(registry, description string, hints []string, packs *knowledge.DraftPacks, ships ...string) (*Plan, error) {
	var task string
	var err error
	if names := packs.Names(); len(names) > 0 {
		task, err = drafter.TaskWithShips(description, hints, names, ships)
	} else {
		task, err = drafter.Task(description, hints)
	}
	if err != nil {
		return nil, err
	}
	reads := "your description"
	if len(packs.Names()) > 0 {
		reads = "your description, your knowledge packs"
	}
	p, err := prepare(registry, reads, task)
	if err != nil {
		return nil, err
	}
	p.Description, p.Knowledge = description, packs
	return p, nil
}

// PrepareRevision checks a revision on this computer before anything
// leaves it, as Prepare checks a draft: the Drafter's task for rev
// (drafter.RevisionTask: the release's files under /inputs/revise, then the
// person's two texts, fenced), the runner policy and the person's own
// keys. AI Review's check of the finished draft reads the revision's
// description (drafter.RevisionDescription). rev's release is already
// staged and its texts set (drafter.ReadRelease, WithTexts).
func PrepareRevision(registry string, rev *drafter.Revision) (*Plan, error) {
	task, err := drafter.RevisionTask(rev)
	if err != nil {
		return nil, err
	}
	p, err := prepare(registry, "your texts, the release", task)
	if err != nil {
		return nil, err
	}
	p.Description, p.Revision = drafter.RevisionDescription(rev), rev
	return p, nil
}

// prepare is Prepare's shared half: the runner policy and the person's
// keys for task, refused without a Jev key, naming what AI Review would
// have read (reads).
func prepare(registry, reads, task string) (*Plan, error) {
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
		return nil, fmt.Errorf("drafting on this computer needs your own TypeSafe Jev key, because AI Review reads %s and everything the draft reads with it before the model does. Add one under Settings → Provider settings → AI Review key: %w", reads, err)
	case err != nil:
		return nil, err
	}
	return &Plan{Task: task, Policy: pol, PolicySHA: polSHA, PolicyFile: polFile, Web: w, Search: search}, nil
}

// Request is one draft's run.
type Request struct {
	Plan  *Plan
	RunID string
	// Namespace is the person's, which the draft is published under.
	Namespace string
	// Ships are the packs the person chose to ship with the QuickAgent,
	// each also among the packs the draft reads: the draft ships these and
	// no other. ShipIns are the same packs opened whole, with the origin
	// each claims, for the draft's package.
	Ships   []build.ShipSource
	ShipIns []drafter.ShipIn
	Log     io.Writer
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
	// Progress, when set, is told how far the draft has got: at its
	// start, as each stage begins, and at every usage the harness
	// reports; nil tells nobody.
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
	// StageKnowledge: AI Review reads each pack not scanned in Agentry,
	// with the person's key, before the Drafter starts; a draft with no
	// such pack passes it at once.
	StageKnowledge Stage = "knowledge"
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
	limits := p.Budgets()
	b, shown := limits.Run, limits.Shown
	prog := progress.NewMeter(req.Progress, Progress{Report: progress.Start(shown.TimeoutSeconds, shown.MaxTurns)})
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
	// A revision's release is read by AI Review too, before the Drafter
	// starts: a file it does not pass stops the draft here.
	if rev := p.Revision; rev != nil {
		if _, err := drafter.PrescanRelease(ctx, sc, rev); err != nil {
			return nil, err
		}
	}
	passed := "the description"
	if p.Revision != nil {
		passed = "your texts and the release"
	}
	_, _ = fmt.Fprintf(logw, "AI Review passed %s. The Drafter runs on this computer against the model your runner policy names (%s), for no credits. %s "+
		"Every result goes to AI Review with your key before the model reads it, and this computer reads each page from its site. It submits nothing.\n",
		passed, p.ModelName(), SearchSentence(p.Search, p.Policy))

	// A pack not scanned in Agentry is scanned now, after the description
	// and before the Drafter starts, and what AI Review flags in any pack
	// (now or in Agentry) is withheld from the copy the Drafter reads, as
	// `agentry draft --knowledge` does.
	var read *drafter.KnowledgeRead
	if k := p.Knowledge; k != nil && len(k.Packs) > 0 {
		if len(k.ScanFirst) > 0 {
			prog.Set(atStage(StageKnowledge))
		}
		if read, err = drafter.ReadyKnowledge(ctx, k.Packs, sc, k.Scratch, logw); err != nil {
			return nil, err
		}
	}

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
	if read != nil {
		first.Knowledge = read.Packs
	}
	if rev := p.Revision; rev != nil {
		// The release reaches the Drafter as files under /inputs/revise, as
		// `agentry draft --revise` stages it; its repair turn is given none
		// (localrun.RepairRun).
		first.Inputs = []string{rev.Dir}
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
		OnUsage:   prog.Usage,
		Knowledge: read,
		Ships:     req.ShipIns,
		Revision:  p.Revision,
		// The embedded rule bundle, as the desktop's own Validate uses: the
		// app has no rules checkout or --rules-update cache of its own.
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
	m, notes, err := build.FromDraft(d.Manifest, d.Instructions, req.Namespace, req.Ships...)
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
