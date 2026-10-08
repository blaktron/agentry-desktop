// BuildAPI: the Build surface's bindings — author a QuickAgent as a form,
// save it as a real workspace, validate it with the registry's own
// scanners, and publish it through the full pipeline. Publish progress
// streams as `build:publish-log` events. Since #282 it also drafts one from
// a description on this computer (Describe it), whose progress streams as
// `build:draft-log`.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"
	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/validate"
)

const (
	eventBuildLog = "build:publish-log"
	eventDraftLog = "build:draft-log"
	// eventDraftProgress carries how far a draft on this computer has got
	// (draft.Progress): its budgets, the stage and the harness's usage
	// (agentry-desktop#410).
	eventDraftProgress = "build:draft-progress"
)

// BuildAPI drives the Build page.
type BuildAPI struct {
	app *App
	mu  sync.Mutex
	// run is the run surface: a draft on this computer holds its lock and
	// is the run in flight, so quitting ends it and sweeps what it started.
	run *RunAPI
	// draftScanner, draftRunner and draftChecker are the Drafter's seams a
	// test substitutes; nil is the core's pkg/judge, this machine's docker
	// and the core's pkg/draftcheck, each with the person's own key.
	draftScanner func(key string) (webscan.Scanner, error)
	draftRunner  func(context.Context, localrun.Run) (*receipt.Envelope, error)
	draftChecker func(*engine.WebRun) (drafter.Checker, error)
	// hosted is the one draft In Agentry this app follows (#281).
	hosted hostedSlot
	// revised is the revision Edit with AI landed last, which Publish this
	// version publishes (agentry-desktop#501).
	revised revisedSlot
	// reviewHTTP and reviewRunner are the Examiner's seams a test
	// substitutes (agentry-desktop#469); nil is a client with a timeout and
	// this machine's docker.
	reviewHTTP   *http.Client
	reviewRunner func(context.Context, localrun.Run) (*receipt.Envelope, error)
}

// NewModel returns a fresh form model. The namespace defaults to the
// signed-in handle on the given registry when there is one.
func (b *BuildAPI) NewModel(reg string) (*build.Model, error) {
	ns := "example"
	if token := settings.TokenFor(reg); token != "" {
		if u, err := whoami(b.app, reg, token); err == nil {
			ns = u.Handle
		}
	}
	return build.Default(ns), nil
}

// Load reads an existing workspace back into the form.
func (b *BuildAPI) Load(dir string) (*build.Model, error) {
	if dir == "" {
		return nil, fmt.Errorf("pick a folder")
	}
	return build.Load(dir)
}

// Exists reports whether dir already holds an agent.yaml (the page asks
// before loading over unsaved form state).
func (b *BuildAPI) Exists(dir string) (bool, error) {
	if dir == "" {
		return false, nil
	}
	_, err := os.Stat(dir + "/agent.yaml")
	return err == nil, nil
}

// Save writes the workspace (agent.yaml + entrypoint always; README,
// LICENSE, .gitignore only when absent).
func (b *BuildAPI) Save(dir string, m build.Model) error {
	if dir == "" {
		return fmt.Errorf("pick a folder to save into")
	}
	// The folder came from the native picker (D-030): the page names no
	// path of its own.
	if err := b.app.requirePicked("folder", dir); err != nil {
		return err
	}
	return build.Save(dir, &m)
}

// Check renders the form's model without touching disk and runs the rules
// over it: the form's own and the core's static V-rules. The Build page
// draws each step's lines and problem from it (plan builder-rework.md §5,
// agentry-desktop#280). It reads nothing and writes nothing.
func (b *BuildAPI) Check(m build.Model) *build.CheckResult {
	return m.Check()
}

// Summary is the Build tab's summary of the form's model (plan
// builder-simplify.md §4.1, agentry-desktop#344): how it works and every
// capability it declares, with the hosts a draft's description did not name
// marked (the draft's Unmentioned; none for a QuickAgent written in the
// form). It reads nothing and writes nothing.
func (b *BuildAPI) Summary(m build.Model, unmentioned []string) *build.Summary {
	return m.Summarize(unmentioned)
}

// ServerChecks asks the registry what it last knew of each declared server
// version, the check the agent page shows, for the Build page's Tools and
// Review steps. It reads no file; its one request is the registry's public
// lookup of those versions.
func (b *BuildAPI) ServerChecks(reg string, m build.Model) []mcpvuln.Status {
	return mcpvuln.Check(b.app.ctx, reg, m.DeclaredServers()).Statuses
}

// Validate runs the same offline checks the Validate page runs (manifest
// V-rules + S1/S3/S4), on the saved workspace.
func (b *BuildAPI) Validate(dir string) (*validate.Result, error) {
	if err := b.app.requirePicked("folder", dir); err != nil {
		return nil, err
	}
	return validate.Run(dir)
}

// Submit sends the workspace to the registry publicly (agentry-desktop#425,
// reversing builder-simplify D3's private default) and follows the pipeline
// to a verdict. Nothing is sent until the person has said Yes to a native
// dialog the page cannot answer: what the submission spends in credits,
// read from the account, and that the QuickAgent becomes public, or that
// it stays private when it already exists private. A No sends nothing and
// returns nil.
func (b *BuildAPI) Submit(reg, dir string) (*build.PublishResult, error) {
	if !b.mu.TryLock() {
		return nil, errBuildBusy
	}
	defer b.mu.Unlock()
	// Submit sends the folder Save wrote, which the native picker gave
	// (D-030); the binding holds that itself rather than trust the page's
	// order of calls.
	if err := b.app.requirePicked("folder", dir); err != nil {
		return nil, err
	}
	return b.submit(reg, dir, "")
}

// submit is Submit's work under its lock, for a folder the picker gave:
// the native warning, then the upload, naming draftRun when Publish this
// version publishes a revision drafted In Agentry (PublishRevision).
func (b *BuildAPI) submit(reg, dir, draftRun string) (*build.PublishResult, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, fmt.Errorf("submitting needs a signed-in registry (sign in under Settings → Registry settings): %w", err)
	}
	m, err := build.ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	private, err := build.ExistsPrivate(b.app.ctx, url, token, m.Namespace, m.Name)
	if err != nil {
		return nil, err
	}
	sub, subErr := fetchSubscriptionStrictly(b.app.ctx, url, token)
	title := "Submit " + m.Namespace + "/" + m.Name + " publicly?"
	if private {
		title = "Submit " + m.Namespace + "/" + m.Name + "? It stays private"
	}
	if cerr := b.app.askConsent(title, submitConsentText(m.Namespace+"/"+m.Name+"@"+m.Version, originOf(url), private, sub, subErr)); cerr != nil {
		if errors.Is(cerr, errNotConfirmed) {
			return nil, nil //nolint:nilnil // a No is the answer: nothing was sent
		}
		return nil, cerr
	}
	logw := (&RunAPI{app: b.app}).eventWriter(eventBuildLog)
	defer logw.Flush()
	res, err := build.PublishRevision(b.app.ctx, url, token, dir, private, draftRun, logw)
	// The site changes a private agent's visibility; a registry that is
	// the hosted site mounts billing, and a bare one has no such page.
	if res != nil && res.Visibility == "private" && sub != nil {
		res.SettingsURL = siteSettingsURL(url, m.Namespace, m.Name)
	}
	return res, err
}

// errBuildBusy refuses a second Submit or Install while one is under way.
var errBuildBusy = errors.New("a submit or an install is already in progress")

// submitConsentText is Submit's warning: what it spends, and whether it
// is public. A new or public QuickAgent is published public; one that
// exists private stays private (the operator's decision of 2026-09-30):
// making it public is a step of its own, Make public in the Publisher
// section, for its namespace's administrators, and never a fork's
// (agentry-desktop#705, #706).
func submitConsentText(ref, origin string, private bool, sub *SubscriptionView, subErr error) string {
	sends := "This sends " + ref + " to " + origin + ", where the security pipeline scans it"
	lines := []string{sends + ", and makes it public: anyone can find and install it once the pipeline publishes it."}
	if private {
		lines = []string{
			sends + ".",
			"It exists there as a private QuickAgent, and this version stays private with it. Making it public is a step of its own: Make public in the Publisher section, for its namespace's administrators, and never for a fork.",
		}
	}
	switch {
	case subErr != nil:
		lines = append(lines, "What a submission costs could not be read ("+subErr.Error()+"); the registry charges it when the version is accepted.")
	case sub == nil:
		lines = append(lines, "This registry charges no credits.")
	case sub.Unmetered:
		lines = append(lines, "This account is unmetered: the submission spends no credits.")
	default:
		whose := "this account's"
		if sub.MemberOf != "" {
			whose = "the " + sub.MemberOf + " organization's"
		}
		lines = append(lines, fmt.Sprintf("It spends %s from %s balance of %d.", creditsText(sub.SubmissionCost), whose, sub.Balance))
	}
	return strings.Join(append(lines, "", "Submit?"), "\n")
}

// siteSettingsURL is the site's settings page for ns/name, on the
// registry's own origin, where a private QuickAgent is made public.
func siteSettingsURL(reg, ns, name string) string {
	return originOf(reg) + "/" + neturl.PathEscape(ns) + "/" + neturl.PathEscape(name) + "/settings"
}

// creditsText is "1 credit" or "N credits".
func creditsText(n int64) string {
	if n == 1 {
		return "1 credit"
	}
	return fmt.Sprintf("%d credits", n)
}

// DescribeStatusView is what Describe it, on this computer says before a
// draft: whether a Jev key of the person's is there (and where from), what
// web search the draft would have, and the model it would run against.
// Never a key.
type DescribeStatusView struct {
	Key            JevKeyView `json:"key"`
	Search         string     `json:"search"`
	SearchSentence string     `json:"searchSentence"`
	ModelName      string     `json:"modelName"`
	// MaxPacks and MaxBytes are what a draft on this computer reads of the
	// person's knowledge packs (agentry-desktop#497): the CLI's own bounds,
	// drafter.MaxKnowledgePacks and MaxKnowledgeBytes.
	MaxPacks int   `json:"maxPacks"`
	MaxBytes int64 `json:"maxBytes"`
}

// DescribeStatus reads it: presence markers and whether a variable is set,
// never a key, and no policy is created.
func (b *BuildAPI) DescribeStatus() (*DescribeStatusView, error) {
	st, err := draft.GetStatus()
	if err != nil {
		return nil, err
	}
	return &DescribeStatusView{Key: *jevKeyView(st.Key), Search: string(st.Search), SearchSentence: st.SearchSentence, ModelName: st.ModelName,
		MaxPacks: drafter.MaxKnowledgePacks, MaxBytes: drafter.MaxKnowledgeBytes}, nil
}

// DraftView is a finished draft for the Build page: the form's Model, what
// the decode left out, and the Drafter's notes, matches, sources and open
// questions, with AI Review's record of what it read and held back. All of
// it is text the page renders under the run-output reader's rules.
type DraftView struct {
	Model         *build.Model      `json:"model"`
	DecodeNotes   []string          `json:"decodeNotes"`
	Notes         string            `json:"notes"`
	Matches       []drafter.Match   `json:"matches"`
	Sources       []drafter.Source  `json:"sources"`
	OpenQuestions []string          `json:"openQuestions"`
	AIReview      DraftAIReviewView `json:"aiReview"`
	ManifestError string            `json:"manifestError,omitempty"`
	DrafterModel  string            `json:"drafterModel,omitempty"`
	RunID         string            `json:"runId,omitempty"`
	OutDir        string            `json:"outDir"`
	// Where says where the draft was made: draftHere or draftInAgentry.
	Where string `json:"where"`
	// Description and Hints are what the person asked for, for Describe
	// again: a Describe again's description is read back to the person's
	// first words there.
	Description string   `json:"description"`
	Hints       []string `json:"hints"`
	// Unmentioned are the hosts the draft reaches that the person's words
	// do not name, "*" included (plan §7.6); Problems are the rules it
	// still fails as the post-step recorded them, with MoreProblems past
	// the first draft.MaxProblems.
	Unmentioned  []string `json:"unmentioned"`
	Problems     []string `json:"problems"`
	MoreProblems int      `json:"moreProblems"`
	// Check is AI Review's check of the finished draft (agentry-cli#418),
	// or nil for a draft that has none.
	Check *DraftCheckView `json:"check"`
	// Knowledge are the person's packs the Drafter was given
	// (agentry-desktop#497): each one's name, how it was scanned (in
	// Agentry, or for this draft with the person's key) and every file AI
	// Review withheld from it, with the reason. Never empty for a draft
	// that read none: an empty list.
	Knowledge []drafter.PackRead `json:"knowledge"`
	// Revision is set when the draft is Edit with AI's revision of a
	// release (agentry-desktop#501): the QuickAgent, the versions, and the
	// diff. Nil for a draft from a description.
	Revision *RevisionView `json:"revision,omitempty"`
}

// DraftCheckView is AI Review's check of a finished draft for the page:
// the summary's one line, and for Review's folded "How it was drafted"
// each half's verdict, each rubric question it scored at 2 or more with its
// file, the draft questions it flagged, and why it had no answer. Every
// string is text the page renders as text.
type DraftCheckView struct {
	Line *draft.Line `json:"line"`
	// Rubric and Draft are each half's verdict (pass, flagged,
	// not_scanned), "" when the check had no answer.
	Rubric     string             `json:"rubric"`
	Draft      string             `json:"draft"`
	Findings   []DraftFindingView `json:"findings"`
	Flagged    []string           `json:"flagged"`
	Unanswered string             `json:"unanswered"`
	// Checks is how many checks the post-step asked: 2 when a flag sent
	// the draft to its one repair turn and the answer was checked again.
	Checks int `json:"checks"`
}

// verdictWord is a verdict the draft wrote, held to the check's three
// words: anything else is not_scanned, as the site's import reads it, so a
// draft cannot put words of its own where the page says the verdict.
func verdictWord(v string) string {
	switch v {
	case drafter.CheckPass, drafter.CheckFlagged:
		return v
	}
	return drafter.CheckNotScanned
}

// DraftFindingView is one rubric question the check scored at 2 or more.
// Question names it for a person, "" for an id that is not the rubric's.
type DraftFindingView struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Score    int    `json:"score"`
	Path     string `json:"path"`
}

// checkView is the check as the page takes it.
func checkView(c *drafter.Check) *DraftCheckView {
	if c == nil {
		return nil
	}
	v := &DraftCheckView{Line: draft.CheckLine(c), Findings: []DraftFindingView{}, Flagged: []string{}, Unanswered: c.Unanswered, Checks: c.Checks}
	if c.Rubric != nil && c.Draft != nil && c.Unanswered == "" {
		v.Rubric, v.Draft = verdictWord(c.Rubric.Verdict), verdictWord(c.Draft.Verdict)
		for _, f := range c.Rubric.Findings {
			v.Findings = append(v.Findings, DraftFindingView{ID: f.ID, Question: draft.RubricQuestion(f.ID), Score: f.Score, Path: f.Path})
		}
		v.Flagged = append(v.Flagged, c.Draft.Flagged...)
	}
	return v
}

// Where a draft was made (DraftView.Where): on this computer with the
// person's own key, or on a sandbox host with AI Review through the
// registry (#281). The page reads the same two words.
const (
	draftHere      = "here"
	draftInAgentry = "agentry"
)

// DraftAIReviewView is AI Review's record of a draft: the receipt's counts
// and each text it held back from the model, with why.
type DraftAIReviewView struct {
	// Scan is where the texts were scanned: "jev" with the person's own
	// key, "registry" through the registry for a draft In Agentry.
	Scan            string                   `json:"scan"`
	Read            int                      `json:"read"`
	Passed          int                      `json:"passed"`
	Withheld        int                      `json:"withheld"`
	NotScanned      int                      `json:"notScanned"`
	WithheldSources []drafter.WithheldSource `json:"withheldSources"`
}

// DescribeOnThisComputer drafts a QuickAgent from a description with the
// built-in Drafter on this computer (plan builder-rework.md §7.1, D22, D25),
// in this order:
//  1. Without the person's own Jev key it refuses, saying where to add one,
//     and nothing leaves this computer.
//  2. A native dialog asks, saying where the description goes (D-030: the
//     page's click is not the consent).
//  3. Under the run lock, as the run in flight, AI Review reads the
//     description, the Drafter runs against the runner policy's model with
//     every result read by AI Review before its model, and the answer is
//     finished and decoded into the form's Model.
//
// Nothing is written into a QuickAgent folder: the draft reaches one only
// when the person saves it where the native picker says.
//
// packs are knowledge packs kept on this computer the Drafter reads first
// (agentry-desktop#497; plan knowledge-and-drafter.md K4): opened before
// the dialog and held to the Drafter's bound, and the dialog says which of
// them AI Review scans first with the person's key, because no scan in
// Agentry stands in for them (D3).
func (b *BuildAPI) DescribeOnThisComputer(reg, description string, hints, packs []string) (*DraftView, error) {
	k, err := b.run.knowledge().ForDraft(b.app.ctx, reg, packs)
	if err != nil {
		return nil, err
	}
	defer k.Done()
	p, err := draft.Prepare(reg, description, hints, k)
	if err != nil {
		return nil, err
	}
	goes := "TypeSafe (AI Review), your model's provider"
	if p.Search == drafter.SearchOn {
		goes += ", your search provider"
	}
	if err := b.app.askConsent("Draft on this computer?",
		"The Drafter runs on this computer against the model your runner policy names ("+p.ModelName()+"), for no credits. "+
			"AI Review (TypeSafe's Jev) reads your description first, with your key, then every search result, page and registry answer "+
			"before the model does, and holds back what it flags. It then sends the finished draft and your description to TypeSafe for AI Review's check, "+
			"with the questions the pipeline asks at publish, and if the rules refuse the draft or the check flags something it can fix, the model has one more turn, with no web access, to fix it. "+
			knowledgeConsent(k)+
			"Your description goes to "+goes+" and the sites the Drafter reads, "+
			"so it should hold no secret. The run's receipts and the draft are kept in your Downloads folder under agentry-out; "+
			"nothing is written into a QuickAgent folder or submitted until you save or publish it. Continue?"); err != nil {
		return nil, err
	}
	ns := ""
	if token := settings.TokenFor(reg); token != "" {
		if u, err := whoami(b.app, reg, token); err == nil {
			ns = u.Handle
		}
	}
	var res *draft.Result
	err = b.run.asRun(jobDraft, "a draft by the Drafter", draftRunRef, eventDraftLog, func(ctx context.Context, runID string, logw io.Writer) error {
		var err error
		res, err = draft.Run(ctx, draft.Request{Plan: p, RunID: runID, Namespace: ns, Log: logw, Scanner: b.draftScanner, Runner: b.draftRunner, Checker: b.draftChecker,
			OnRepairTurn: b.run.addTurn, Progress: func(p draft.Progress) { emitEvent(b.app, eventDraftProgress, p) }})
		if res != nil {
			b.run.keepFolder(runID, res.OutDir)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	v := draftView(res)
	v.Where, v.Description, v.Hints = draftHere, description, append([]string{}, hints...)
	v.settle(b.app.ctx, reg, settings.TokenFor(reg), ns)
	return v, nil
}

// knowledgeConsent is what the dialog says of a draft's packs: that the
// Drafter reads them first, and which of them AI Review scans first with the
// person's key (D3), the rest standing on their scan in Agentry. "" for none.
func knowledgeConsent(k *knowledge.DraftPacks) string {
	names := k.Names()
	if len(names) == 0 {
		return ""
	}
	out := "It reads your knowledge packs " + strings.Join(names, ", ") + " first, and leaves out what AI Review flags in them. "
	switch first := k.ScanFirst; len(first) {
	case 0:
	case 1:
		out += first[0] + " has not been scanned in Agentry, so AI Review reads every file of it with your key before the draft starts, and withholds what it flags. "
	default:
		out += strings.Join(first, ", ") + " have not been scanned in Agentry, so AI Review reads every file of them with your key before the draft starts, and withholds what it flags. "
	}
	return out
}

// draftRunRef names a draft on this computer as the run in flight: what
// quitting says it ends, and how StopDraft tells a draft from a run or a
// knowledge build. It is words, not the Drafter's ref, so no run or session
// the page starts (whose ref the page names) can ever carry it, as
// knowledgeRunRef cannot.
const draftRunRef = "the Drafter"

// StopDraft stops the draft on this computer that is running, from the
// progress dialog or its corner card (agentry-desktop#410): its context is
// cancelled and the Drafter's run tears down as a quit's does, and no draft
// is made. It stops only a draft, never a run, a session or a knowledge
// build, and says so when none is running. A draft In Agentry is cancelled
// with CancelHostedDraft instead.
func (b *BuildAPI) StopDraft() error {
	r := b.run
	r.liveMu.Lock()
	defer r.liveMu.Unlock()
	if r.live == nil || r.live.ref != draftRunRef {
		return errors.New("no draft is running on this computer")
	}
	r.live.cancel()
	return nil
}

// settle is a decoded draft's last step, from either place: what the
// Builder sets on it (plan builder-simplify.md §4.2, D10; build.SetByBuilder),
// its name made unique among the person's own QuickAgents on reg, then the
// hosts it reaches that the person's own words do not name marked.
func (v *DraftView) settle(ctx context.Context, reg, token, ns string) {
	words := draft.PersonsWords(v.Description, v.Hints)
	var taken []string
	if ns != "" {
		names, err := ownNames(ctx, reg, token, ns)
		if err != nil {
			v.DecodeNotes = append(v.DecodeNotes, fmt.Sprintf("Your QuickAgents' names could not be read from the registry, so check that no QuickAgent of yours is already named %s.", v.Model.Name))
		}
		taken = names
	}
	v.DecodeNotes = append(v.DecodeNotes, build.SetByBuilder(v.Model, words, taken)...)
	v.Unmentioned = draft.UnmentionedHosts(v.Model, words)
}

// ownNames are the names of the QuickAgents in the person's own namespace
// on reg, for a draft's unique name.
func ownNames(ctx context.Context, reg, token, ns string) ([]string, error) {
	client, err := registry.New(reg, token)
	if err != nil {
		return nil, err
	}
	resp, err := client.ListAgentsWithResponse(ctx, &apiclient.ListAgentsParams{Namespace: ns})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("list your QuickAgents", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	names := make([]string, 0, len(*resp.JSON200))
	for _, a := range *resp.JSON200 {
		names = append(names, a.Name)
	}
	return names, nil
}

// draftView is a finished draft as the page takes it. The sources AI Review
// withheld are marked from its record, the run's ledger, whatever the
// Drafter said of them, so a flagged address is never offered as a link.
func draftView(res *draft.Result) *DraftView {
	d := res.Draft
	v := &DraftView{Model: res.Model, DecodeNotes: res.Notes, Notes: drafterNotes(d), Matches: d.Matches, Sources: markWithheld(d),
		OpenQuestions: d.OpenQuestions, ManifestError: d.ManifestError, OutDir: res.OutDir, Hints: []string{}}
	if v.DecodeNotes == nil {
		v.DecodeNotes = []string{}
	}
	v.Problems, v.MoreProblems = draft.Problems(d)
	v.Unmentioned = []string{}
	if d.Drafter != nil {
		v.DrafterModel, v.RunID = d.Drafter.Model, d.Drafter.RunID
	}
	if a := d.AIReview; a != nil {
		v.AIReview = DraftAIReviewView{Scan: a.Scan, Read: a.Read, Passed: a.Passed, Withheld: a.Withheld, NotScanned: a.NotScanned, WithheldSources: a.WithheldSources}
	}
	if v.AIReview.WithheldSources == nil {
		v.AIReview.WithheldSources = []drafter.WithheldSource{}
	}
	if d.AIReview != nil {
		v.Check = checkView(d.AIReview.Check)
	}
	v.Knowledge = append([]drafter.PackRead{}, d.Knowledge...)
	for i := range v.Knowledge {
		if v.Knowledge[i].Withheld == nil {
			v.Knowledge[i].Withheld = []drafter.WithheldFile{}
		}
	}
	return v
}

// markWithheld is the Drafter's sources, each one AI Review withheld marked
// so from its record, and each withheld text the Drafter did not list added.
func markWithheld(d *drafter.Draft) []drafter.Source {
	out := append([]drafter.Source{}, d.Sources...)
	if d.AIReview == nil {
		return out
	}
	for _, w := range d.AIReview.WithheldSources {
		found := false
		for i := range out {
			if out[i].URL == w.URL {
				out[i].Withheld, found = true, true
			}
		}
		if !found {
			out = append(out, drafter.Source{URL: w.URL, Withheld: true})
		}
	}
	return out
}

// drafterNotes is the Drafter's own notes, without the "Withheld by AI
// Review" list the post-step appends: the page shows that list from AI
// Review's record, as text, where the notes' reader would make each
// address a link to the page AI Review flagged. The list is cut by
// rebuilding it with the CLI's own NoteWithheld, so no copy of its words
// lives here; notes that do not end with it are returned whole.
func drafterNotes(d *drafter.Draft) string {
	if d.AIReview == nil || len(d.AIReview.WithheldSources) == 0 {
		return d.Notes
	}
	section := &drafter.Draft{AIReview: d.AIReview}
	section.NoteWithheld()
	if !strings.HasSuffix(d.Notes, section.Notes) {
		return d.Notes
	}
	return strings.TrimRight(strings.TrimSuffix(d.Notes, section.Notes), "\n")
}
