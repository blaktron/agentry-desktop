// BuildAPI: the Build surface's bindings — author a BriefAgent as a form,
// save it as a real workspace, validate it with the registry's own
// scanners, and publish it through the full pipeline. Publish progress
// streams as `build:publish-log` events. Since #282 it also drafts one from
// a description on this computer (Describe it), whose progress streams as
// `build:draft-log`.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/webscan"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/validate"
)

const (
	eventBuildLog = "build:publish-log"
	eventDraftLog = "build:draft-log"
)

// BuildAPI drives the Build page.
type BuildAPI struct {
	app *App
	mu  sync.Mutex
	// run is the run surface: a draft on this computer holds its lock and
	// is the run in flight, so quitting ends it and sweeps what it started.
	run *RunAPI
	// draftScanner and draftRunner are the Drafter's seams a test
	// substitutes; nil is the core's pkg/judge and this machine's docker.
	draftScanner func(key string) (webscan.Scanner, error)
	draftRunner  func(context.Context, localrun.Run) (*receipt.Envelope, error)
	// hosted is the one draft In Agentry this app follows (#281).
	hosted hostedBuilds
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
	return validate.Run(dir)
}

// Publish uploads the workspace to the registry (creating the agent record
// if needed) and follows the pipeline to a verdict. visibility is
// "private" or "public".
func (b *BuildAPI) Publish(reg, dir, visibility string) (*build.PublishResult, error) {
	if !b.mu.TryLock() {
		return nil, fmt.Errorf("a publish is already in progress")
	}
	defer b.mu.Unlock()
	if visibility != "private" && visibility != "public" {
		return nil, fmt.Errorf("visibility must be private or public")
	}
	logw := (&RunAPI{app: b.app}).eventWriter(eventBuildLog)
	defer logw.Flush()
	return build.Publish(b.app.ctx, reg, settings.TokenFor(reg), dir, visibility, logw)
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
}

// DescribeStatus reads it: presence markers and whether a variable is set,
// never a key, and no policy is created.
func (b *BuildAPI) DescribeStatus() (*DescribeStatusView, error) {
	st, err := draft.GetStatus()
	if err != nil {
		return nil, err
	}
	return &DescribeStatusView{Key: *jevKeyView(st.Key), Search: string(st.Search), SearchSentence: st.SearchSentence, ModelName: st.ModelName}, nil
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
	// Where says where the draft was made: "here", on this computer with
	// the person's own key, or "agentry", on a sandbox host with AI Review
	// through the registry (#281).
	Where string `json:"where"`
	// Description and Hints are what the person asked for, for Describe
	// again: a Describe again's description is read back to the person's
	// first words there.
	Description string   `json:"description"`
	Hints       []string `json:"hints"`
}

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

// DescribeOnThisComputer drafts a BriefAgent from a description with the
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
// Nothing is written into a BriefAgent folder: the draft reaches one only
// when the person saves it where the native picker says.
func (b *BuildAPI) DescribeOnThisComputer(reg, description string, hints []string) (*DraftView, error) {
	p, err := draft.Prepare(reg, description, hints)
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
			"before the model does, and holds back what it flags. Your description goes to "+goes+" and the sites the Drafter reads, "+
			"so it should hold no secret. The run's receipts and the draft are kept in your Downloads folder under agentry-out; "+
			"nothing is written into a BriefAgent folder or submitted until you save or publish it. Continue?"); err != nil {
		return nil, err
	}
	ns := ""
	if token := settings.TokenFor(reg); token != "" {
		if u, err := whoami(b.app, reg, token); err == nil {
			ns = u.Handle
		}
	}
	var res *draft.Result
	err = b.run.asRun(drafter.Namespace+"/"+drafter.Name, eventDraftLog, func(ctx context.Context, runID string, logw io.Writer) error {
		var err error
		res, err = draft.Run(ctx, draft.Request{Plan: p, RunID: runID, Namespace: ns, Log: logw, Scanner: b.draftScanner, Runner: b.draftRunner})
		return err
	})
	if err != nil {
		return nil, err
	}
	v := draftView(res)
	v.Where, v.Description, v.Hints = "here", description, append([]string{}, hints...)
	return v, nil
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
	if d.Drafter != nil {
		v.DrafterModel, v.RunID = d.Drafter.Model, d.Drafter.RunID
	}
	if a := d.AIReview; a != nil {
		v.AIReview = DraftAIReviewView{Scan: a.Scan, Read: a.Read, Passed: a.Passed, Withheld: a.Withheld, NotScanned: a.NotScanned, WithheldSources: a.WithheldSources}
	}
	if v.AIReview.WithheldSources == nil {
		v.AIReview.WithheldSources = []drafter.WithheldSource{}
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
