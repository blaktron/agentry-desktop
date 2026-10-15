// Review Your QuickAgents locally: on the Build tab's summary and on
// Validate, a click runs the built-in Examiner over the QuickAgent in a
// folder the person picked, on this computer, with their own TypeSafe key
// and their runner policy's model, after a native dialog that says where its
// text goes. It is a local job under the run lock, followed in the progress
// dialog and corner card every job has, and stopped only by its own Stop.
// Its report is written to a folder of its own beside the run's receipts;
// the page shows it, and Dismiss makes the app forget the folder, as for a
// run. Submit is never held or changed by it: the registry scans again at
// Submit, and its answer is the one that counts.
package main

import (
	"context"
	"errors"
	"io"
	"log"

	"github.com/blaktron/agentry-cli/pkg/examiner"

	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/review"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

const (
	eventReviewLog = "build:review-log"
	// eventReviewProgress carries how far a review has got
	// (review.Progress): its budgets, the stage and the harness's usage.
	eventReviewProgress = "build:review-progress"
)

// reviewRunRef names a review as the run in flight: what quitting says it
// ends. Words, not the Examiner's ref, as draftRunRef is. StopReview tells
// a review by its kind, jobReview, which only ReviewLocally takes.
const reviewRunRef = "the Examiner"

// ReviewStatusView is what the action says before a review: whether the
// person's own key is there, and the model the Examiner would run against.
// Never a key.
type ReviewStatusView struct {
	Key       JevKeyView `json:"key"`
	ModelName string     `json:"modelName"`
}

// ReviewStatus reads it: presence markers and whether a variable is set,
// never a key, and no policy is created.
func (b *BuildAPI) ReviewStatus() (*ReviewStatusView, error) {
	st, err := draft.GetStatus()
	if err != nil {
		return nil, err
	}
	return &ReviewStatusView{Key: *jevKeyView(st.Key), ModelName: st.ModelName}, nil
}

// ReviewLocally reviews the QuickAgent in dir with the built-in Examiner on
// this computer, in this order:
//  1. dir must be a folder the native picker gave, once links are
//     resolved, since its files leave this computer.
//  2. Without the person's own Jev key it refuses, saying where to add one,
//     and nothing leaves this computer.
//  3. A native dialog asks, saying where the QuickAgent's text goes
//     (the page's click is not the consent). A No reviews nothing
//     and returns nil.
//  4. Under the run lock, as the run in flight, the registry's rules run
//     here, the Examiner runs over a read-only copy, and the report is
//     written to a fresh folder.
//
// reg is the registry the pre-step asks for its public policy and its
// record of each declared server; no token is sent to it.
func (b *BuildAPI) ReviewLocally(reg, dir string) (*ReviewView, error) {
	if dir == "" {
		return nil, errors.New("pick the QuickAgent's folder to review")
	}
	if err := b.app.requirePickedResolved("folder", dir); err != nil {
		return nil, err
	}
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, err
	}
	p, err := review.Prepare(dir)
	if err != nil {
		return nil, err
	}
	// Another local job is refused before the dialog, so a Yes is not
	// answered with a refusal; the run lock below still decides.
	if b.run.InFlight() != nil {
		return nil, b.run.busy()
	}
	if err := b.app.askConsent("Review "+p.What()+" on this computer?", reviewConsentText(p, reg)); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return nil, nil //nolint:nilnil // a No is the answer: nothing was reviewed
		}
		return nil, err
	}
	var res *review.Result
	var runID string
	err = b.run.asRun(jobReview, "a review by the Examiner", reviewRunRef, eventReviewLog, func(ctx context.Context, id string, logw io.Writer) error {
		runID = id
		var err error
		res, err = review.Run(ctx, review.Request{Plan: p, Registry: reg, RunID: id, Log: logw, HTTP: b.reviewHTTP, Runner: b.reviewRunner,
			OnStaged: func(stage string) { b.run.recordStage(id, stage) },
			Progress: func(p review.Progress) { emitEvent(b.app, eventReviewProgress, reviewProgressOf(id, p)) }})
		if res != nil {
			b.run.keepFolder(id, res.OutDir)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return reviewView(runID, res), nil
}

// reviewConsentText is the review's dialog: where the QuickAgent's text
// goes, and what it does not do.
func reviewConsentText(p *review.Plan, reg string) string {
	what := p.What()
	if p.Ref != "" {
		what += " from " + p.Dir
	}
	return "The Examiner reviews " + what + " on this computer, for no credits. " +
		"The registry's own rules run here first, and " + originOf(reg) + " is asked for its public policy and its record of each MCP server the QuickAgent declares, with no sign-in. " +
		"Then the QuickAgent's text goes to TypeSafe (AI Review's Jev) with your key, and to the model your runner policy names (" + p.ModelName() + "), which reads it from a read-only copy. " +
		"Nothing of the QuickAgent runs, and nothing is submitted. The report and the run's receipts are kept in your Downloads folder under agentry-out. " +
		"The review is not the registry's: the registry scans again at Submit, and its answer is the one that counts. Continue?"
}

// recordStage adds a review's staged copy to its run's crash-path record,
// so a review the app dies inside leaves the copy for the next start's
// sweep. Best effort, like the record itself.
func (r *RunAPI) recordStage(runID, stage string) {
	dir, err := r.inFlight()
	if err != nil {
		return
	}
	if err := run.RecordStage(dir, runID, stage); err != nil {
		log.Printf("agentry-desktop: %v", err)
	}
}

// StopReview stops the review that is running, from its progress dialog or
// its corner card: its context is cancelled, the Examiner's run tears down
// and writes its receipts as far as it got, and the report says what of it
// is missing. It stops only a review, never a run, a chat, a draft or a
// build, and says so when none is running.
func (b *BuildAPI) StopReview() error {
	r := b.run
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.live == nil || r.live.kind != jobReview {
		return errors.New("no review is running on this computer")
	}
	r.live.cancel()
	return nil
}

// ReviewProgressView is how far a review has got, with its run id, for the
// page's store.
type ReviewProgressView struct {
	review.Progress
	RunID string `json:"runId"`
}

func reviewProgressOf(runID string, p review.Progress) ReviewProgressView {
	return ReviewProgressView{Progress: p, RunID: runID}
}

// ReviewView is a finished review for the page: the report, as the page
// draws it. Every string the QuickAgent or the Examiner's model could have
// written (titles, excerpts, details, files, the summary) is text the page
// renders as text; every word the page shows as a verdict (the outcome, a
// severity, a kind) is held here to the report's own words.
type ReviewView struct {
	RunID  string `json:"runId"`
	OutDir string `json:"outDir"`
	// Ref is the QuickAgent examined, as its manifest declares it, and Files
	// the files a submission would carry.
	Ref   string `json:"ref"`
	Files int    `json:"files"`
	// Outcome is published, needs_review, rejected, refused_at_submit or
	// undetermined; OutcomeWords says it for a person.
	Outcome      string `json:"outcome"`
	OutcomeWords string `json:"outcomeWords"`
	Reason       string `json:"reason"`
	Assumes      string `json:"assumes"`
	// PolicyFromRegistry says the outcome is decided under the registry's
	// own baseline policy; otherwise the one compiled into this build, and
	// PolicyError says why. PublishesUpTo is that policy's threshold.
	PolicyFromRegistry bool   `json:"policyFromRegistry"`
	PolicyError        string `json:"policyError,omitempty"`
	PublishesUpTo      string `json:"publishesUpTo"`
	// AICounted says AI Review answered and was weighed with the rules.
	AICounted bool `json:"aiCounted"`

	Rules    ReviewRulesView    `json:"rules"`
	AIReview ReviewAIReviewView `json:"aiReview"`
	Reading  ReviewReadingView  `json:"reading"`

	NotRunHere []string `json:"notRunHere"`
	Notice     string   `json:"notice"`
	// ExaminerModel is the model the Examiner ran against.
	ExaminerModel string `json:"examinerModel,omitempty"`
	// RunError says why the Examiner's run failed, when it did; the rest of
	// the view is what the report holds all the same.
	RunError string `json:"runError,omitempty"`
}

// ReviewRulesView is the report's first kind: the registry's rules, run here.
type ReviewRulesView struct {
	Manifest      []ReviewIssueView   `json:"manifest"`
	Findings      []ReviewFindingView `json:"findings"`
	BundleVersion string              `json:"bundleVersion"`
	BundleSource  string              `json:"bundleSource"`
	Servers       []ReviewServerView  `json:"servers"`
}

// ReviewIssueView is one manifest rule's result: an error is refused at
// Submit.
type ReviewIssueView struct {
	Rule    string `json:"rule"`
	Error   bool   `json:"error"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// ReviewServerView is the registry's record of one declared server.
type ReviewServerView struct {
	Ref    string `json:"ref"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// ReviewFindingView is one finding of any kind. Scanner is the registry's
// scanner id for the first two kinds, and secret, script, instruction or
// other for the Examiner's.
type ReviewFindingView struct {
	Scanner    string `json:"scanner"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Title      string `json:"title"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// ReviewAIReviewView is the report's second kind: AI Review, from the
// run's own ledger.
type ReviewAIReviewView struct {
	Ran        bool                `json:"ran"`
	Model      string              `json:"model,omitempty"`
	RubricHash string              `json:"rubricHash,omitempty"`
	Findings   []ReviewFindingView `json:"findings"`
	Files      int                 `json:"files"`
	NotRead    int                 `json:"notRead"`
	Failure    string              `json:"failure,omitempty"`
}

// ReviewReadingView is the report's third kind: the Examiner's own
// reading, never counted, with the files AI Review withheld from it.
type ReviewReadingView struct {
	Summary   string              `json:"summary,omitempty"`
	Findings  []ReviewFindingView `json:"findings"`
	NotRead   []ReviewFileView    `json:"notRead"`
	Withheld  []ReviewFileView    `json:"withheld"`
	Error     string              `json:"error,omitempty"`
	Recovered bool                `json:"recovered,omitempty"`
}

// ReviewFileView is a file and why it was not read.
type ReviewFileView struct {
	File string `json:"file"`
	Why  string `json:"why"`
}

// The words the page may show as a verdict, a severity or a kind.
var (
	reviewOutcomes    = map[string]bool{examiner.OutcomePublished: true, examiner.OutcomeNeedsReview: true, examiner.OutcomeRejected: true, examiner.OutcomeRefused: true}
	reviewSeverities  = map[string]bool{"info": true, "low": true, "medium": true, "high": true, "critical": true}
	reviewConfidences = map[string]bool{"low": true, "medium": true, "high": true}
	reviewReadKinds   = map[string]bool{"secret": true, "script": true, "instruction": true, "other": true}
)

// word is w when it is one of words, else "".
func word(w string, words map[string]bool) string {
	if words[w] {
		return w
	}
	return ""
}

func reviewView(runID string, res *review.Result) *ReviewView {
	r := res.Report
	v := &ReviewView{
		RunID: runID, OutDir: res.OutDir, Ref: r.Package.Ref, Files: r.Package.Files,
		Outcome: word(r.Expected.Outcome, reviewOutcomes), Reason: r.Expected.Reason, Assumes: r.Expected.Assumes,
		PolicyFromRegistry: r.Expected.Policy.Source == examiner.PolicyFromRegistry, PolicyError: r.Expected.Policy.Error,
		PublishesUpTo: word(examiner.AutoPublishUpTo(r.Expected.Policy.Policy), reviewSeverities),
		NotRunHere:    append([]string{}, r.NotRunHere...), Notice: examiner.Notice, ExaminerModel: r.Examiner.Model,
	}
	if v.Outcome == "" {
		v.Outcome = examiner.OutcomeUndetermined
	}
	v.OutcomeWords = examiner.OutcomeWords(v.Outcome)
	for _, k := range r.Expected.Counted {
		v.AICounted = v.AICounted || k == examiner.KindAIReview
	}
	if res.RunErr != nil {
		v.RunError = res.RunErr.Error()
	}

	v.Rules = ReviewRulesView{Manifest: []ReviewIssueView{}, Findings: findingViews(r.RegistryRules.Findings, nil), Servers: []ReviewServerView{},
		BundleVersion: r.RegistryRules.Bundle.Version, BundleSource: r.RegistryRules.Bundle.Source}
	for _, is := range r.RegistryRules.Manifest {
		v.Rules.Manifest = append(v.Rules.Manifest, ReviewIssueView{Rule: is.Rule, Error: is.Severity == "error", Path: is.Path, Message: is.Message})
	}
	for _, s := range r.RegistryRules.Servers {
		v.Rules.Servers = append(v.Rules.Servers, ReviewServerView{Ref: s.Ref, Status: s.Status, Error: s.Error})
	}

	a := r.AIReview
	v.AIReview = ReviewAIReviewView{Ran: a.Ran, Model: a.Model, RubricHash: a.RubricHash, Findings: findingViews(a.Findings, nil),
		Files: a.Files, NotRead: a.NotRead, Failure: a.Failure}

	rd := r.Reading
	v.Reading = ReviewReadingView{Summary: rd.Summary, Findings: findingViews(rd.Findings, reviewReadKinds),
		NotRead: fileViews(rd.NotRead), Withheld: fileViews(rd.Withheld), Error: rd.Error, Recovered: rd.Recovered}
	return v
}

// findingViews are findings as the page draws them. kinds, when set, holds
// the scanner to the Examiner's four kinds of its own, "other" for anything
// else its model wrote.
func findingViews(fs []examiner.Finding, kinds map[string]bool) []ReviewFindingView {
	out := make([]ReviewFindingView, 0, len(fs))
	for _, f := range fs {
		scanner := f.Scanner
		if kinds != nil && !kinds[scanner] {
			scanner = "other"
		}
		out = append(out, ReviewFindingView{Scanner: scanner, Severity: word(f.Severity, reviewSeverities), Confidence: word(f.Confidence, reviewConfidences),
			Title: f.Title, File: f.File, Line: max(f.Line, 0), Excerpt: f.Excerpt, Detail: f.Detail})
	}
	return out
}

func fileViews(ns []examiner.NotRead) []ReviewFileView {
	out := make([]ReviewFileView, 0, len(ns))
	for _, n := range ns {
		out = append(out, ReviewFileView{File: n.File, Why: n.Why})
	}
	return out
}
