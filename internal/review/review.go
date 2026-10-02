// Package review is Review Your QuickAgents locally (plan
// desktop-review-key-and-run-cards.md §6.3, D2, D19, D20, D24, D25, D28;
// agentry-desktop#469): the CLI's built-in Examiner, run on this computer
// over a QuickAgent the person built, with their own TypeSafe key and the
// model their runner policy names, for no credits. It drives the CLI's own
// pkg/examiner in the order `agentry review` does: the pre-step runs the
// registry's rules here and stages a read-only copy of the files a
// submission would carry; the Examiner runs over that copy with the review
// grant (AI Review asked as stage S5 asks it, every file it reads scanned
// first); the post-step writes report.json beside the run's receipts, in a
// folder no earlier run wrote. The staged copy is removed when the review
// returns, whatever it returns with.
package review

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/examiner"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/validate"

	"github.com/blaktron/agentry-desktop/internal/progress"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// Plan is a review on this computer, checked before anything leaves it:
// the QuickAgent's folder and what it declares itself to be, the runner
// policy, and the person's own key.
type Plan struct {
	Dir string
	// Ref is the QuickAgent's ns/name@version as its manifest declares it.
	Ref        string
	Policy     *policy.Policy
	PolicySHA  string
	PolicyFile policy.File
	Key        *engine.ReviewRun
}

// Budgets are the review's: the runner policy's, with the Examiner's own
// timeout floor (30 minutes, since one rubric review may take 15), as
// `agentry review` gives it.
func (p *Plan) Budgets() spec.Budgets { return localrun.Examiner().LocalBudgets(p.Policy.Budgets) }

// ModelName names the model the runner policy gives the Examiner.
func (p *Plan) ModelName() string { return ModelName(p.Policy) }

// ModelName names the model pol gives a run, as the dialog and the page say it.
func ModelName(pol *policy.Policy) string {
	if m := strings.TrimSpace(pol.Model.Provider + " " + pol.Model.Name); m != "" {
		return m
	}
	return "none configured"
}

// NoKeySentence is what a review without the person's key says, with
// where to add one.
const NoKeySentence = "Reviewing on this computer needs your own TypeSafe Jev key: AI Review is asked with it, and reads every file before the Examiner's model does. Add one under Settings → Provider settings → AI Review key"

// Prepare checks a review before anything leaves this computer: the
// folder holds a QuickAgent, the runner policy, and the person's own key,
// read as a draft reads it. Without the key it refuses, saying where to add
// one, and nothing else happens (D28).
func Prepare(dir string) (*Plan, error) {
	m, err := validate.ReadManifest(dir)
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
	key, err := examiner.LocalReview(pol)
	switch {
	case errors.Is(err, examiner.ErrNoJevKey):
		return nil, fmt.Errorf("%s: %w", NoKeySentence, err)
	case err != nil:
		return nil, err
	}
	return &Plan{Dir: dir, Ref: fmt.Sprintf("%s/%s@%s", m.Namespace, m.Name, m.Version), Policy: pol, PolicySHA: polSHA, PolicyFile: polFile, Key: key}, nil
}

// Stage is where a review is. It goes through them in order.
type Stage string

const (
	// StageRules: the registry's rules run here, and the registry is asked
	// for its baseline policy and its record of each declared server.
	StageRules Stage = "rules"
	// StageExamining: the Examiner's run, AI Review's ask among it.
	StageExamining Stage = "examining"
	// StageReporting: the report is written from the run's own record.
	StageReporting Stage = "reporting"
)

// Progress is how far one review has got, for the progress dialog and its
// corner card: the report every job carries, and the stage.
type Progress struct {
	progress.Report
	Stage Stage `json:"stage"`
}

func atStage(s Stage) func(*Progress) bool {
	return func(p *Progress) bool {
		p.Stage = s
		return true
	}
}

// Request is one review's run.
type Request struct {
	Plan *Plan
	// Registry is the registry the pre-step asks for its policy and its
	// server records, already held to Settings' URL rule; no token goes to it.
	Registry string
	RunID    string
	Log      io.Writer
	// HTTP reaches the registry; nil is a client with a timeout.
	HTTP *http.Client
	// Runner runs the Examiner; nil is localrun on this machine's docker.
	Runner func(context.Context, localrun.Run) (*receipt.Envelope, error)
	// OnStaged is told where the pre-step staged its copy, before the
	// Examiner starts, so the caller's crash path can record it and sweep it
	// when the app dies mid-review (agentry-desktop#463's sweep); nil tells
	// nobody.
	OnStaged func(dir string)
	// Progress, when set, is told how far the review has got; nil tells
	// nobody.
	Progress func(Progress)
}

// Result is a finished review: its report, and the folder holding it with
// the run's receipts.
type Result struct {
	Report *examiner.Report
	OutDir string
	// RunErr is why the Examiner's run failed, when it did and its reading
	// was not recovered; the report is whole either way, and says what of
	// it is missing.
	RunErr error
}

// Run reviews the QuickAgent: the pre-step, the Examiner's run, and the
// post-step's report, written to a fresh folder under the person's
// Downloads. An error is a review that could not start (no manifest, a
// folder that cannot be copied) or a report that could not be written;
// a run that failed still returns its report.
func Run(ctx context.Context, req Request) (*Result, error) {
	p := req.Plan
	logw := req.Log
	if logw == nil {
		logw = io.Discard
	}
	b := p.Budgets()
	prog := progress.NewMeter(req.Progress, Progress{Report: progress.Start(b.TimeoutSeconds, b.MaxTurns)})
	prog.Set(atStage(StageRules))
	client := req.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	// The embedded rule bundle, as the app's Validate and its drafts use:
	// the app keeps no rules cache of its own. The report names the bundle.
	prep, err := examiner.Prepare(ctx, p.Dir, examiner.Options{Registry: req.Registry, HTTP: client, Log: logw})
	if err != nil {
		return nil, err
	}
	defer prep.Cleanup()
	if req.OnStaged != nil {
		req.OnStaged(prep.Stage)
	}

	home, err := run.Home()
	if err != nil {
		return nil, err
	}
	outDir, err := run.OutputDir("", examiner.Namespace, examiner.Name, req.RunID)
	if err != nil {
		return nil, err
	}
	// A folder that holds another review's report or a run's receipts is
	// refused, as `agentry review` refuses one: the post-step reads AI
	// Review from the run's ledger there, and must never read another run's.
	if err := fresh(outDir); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(logw, "The registry's rules ran here. The Examiner runs on this computer against the model your runner policy names (%s), for no credits, "+
		"over a read-only copy of %d file(s). Their text goes to AI Review (TypeSafe's Jev) with your key and to that model. Nothing of the QuickAgent runs, and nothing is submitted.\n",
		p.ModelName(), prep.Files)

	runner := req.Runner
	if runner == nil {
		runner = func(ctx context.Context, r localrun.Run) (*receipt.Envelope, error) {
			return r.Execute(ctx, dockerx.New())
		}
	}
	prog.Set(atStage(StageExamining))
	env, runErr := runner(ctx, localrun.Run{
		Builtin: localrun.Examiner(), Home: home, Policy: p.Policy, PolicySHA: p.PolicySHA, PolicyFile: &p.PolicyFile,
		Build: version.Build(), Task: strings.NewReader(examiner.Task(prep)), Inputs: prep.Inputs, OutDir: outDir,
		Budgets: &b, RunID: req.RunID, Stderr: logw, Review: p.Key, OnUsage: prog.Usage,
	})
	prog.Set(atStage(StageReporting))
	report := examiner.Finish(prep, env, runErr, outDir, localrun.Examiner().Ref(), version.Build())
	if _, err := examiner.Write(outDir, report); err != nil {
		return nil, err
	}
	res := &Result{Report: report, OutDir: outDir}
	if runErr != nil && !report.Reading.Recovered {
		res.RunErr = runErr
	}
	return res, nil
}

// fresh refuses a folder that already holds a review's report or a run's
// receipts.
func fresh(outDir string) error {
	for _, name := range []string{examiner.ReportFile, "receipts"} {
		if _, err := os.Lstat(filepath.Join(outDir, name)); err == nil {
			return fmt.Errorf("%s already holds a review or a run's receipts, and a review is written only to a folder of its own", outDir)
		}
	}
	return nil
}
