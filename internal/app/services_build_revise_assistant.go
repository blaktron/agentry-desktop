package app

// The Drafter's revisions as the Assistant's quickagent_revise asks for them:
// the Build tab's two ways, on this computer and In Agentry, without their
// own dialogs (the Assistant's question asks the person) and without landing
// in the form. On this computer the Drafter revises the folder it is given;
// In Agentry it revises a published version, which the registry stages.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/blaktron/agentry-cli/runnerimage/applink"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
)

// appReviserOps are the Build tab's revisions, for the Assistant.
type appReviserOps struct{ build *BuildAPI }

func (o appReviserOps) prepare(ctx context.Context, reg, where, dir, ref, instructions string) (*pendingRevision, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, fmt.Errorf("a revision needs the person signed in to Agentry: %w", err)
	}
	switch where {
	case applink.ReviseOnComputer:
		return o.build.prepareFolderRevision(url, dir, instructions)
	case applink.ReviseInAgentry:
		return o.build.prepareHostedRevision(ctx, url, token, ref, instructions)
	}
	return nil, fmt.Errorf("a revision runs %s or %s", applink.ReviseInAgentry, applink.ReviseOnComputer)
}

// prepareFolderRevision readies the Drafter's revision of the QuickAgent
// folder dir on this computer: the folder read as a submission packs it,
// its digest recorded, and the plan made against the person's runner
// policy, with nothing sent anywhere yet.
func (b *BuildAPI) prepareFolderRevision(url, dir, instructions string) (*pendingRevision, error) {
	m, err := build.ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	files, err := pkgarchive.WalkWorkspace(dir)
	if err != nil {
		return nil, err
	}
	include := manifest.ResolveInclude(m.Publish.Include, files)
	if len(include) == 0 {
		return nil, errors.New("publish.include matches no file of the folder, so there is nothing to revise")
	}
	info, err := pkgarchive.BuildArchive(dir, include, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("pack the folder: %w", err)
	}
	stage, err := os.MkdirTemp("", "agentry-revise-folder-*")
	if err != nil {
		return nil, err
	}
	rev, err := drafter.ReadRelease(dir, filepath.Join(stage, drafter.ReviseInput), "", info.Digest)
	if err == nil {
		err = rev.WithTexts(instructions, "")
	}
	var p *draft.Plan
	if err == nil {
		p, err = draft.PrepareRevision(url, rev)
	}
	if err != nil {
		_ = os.RemoveAll(stage)
		return nil, err
	}
	goes := "TypeSafe (AI Review), the model's provider"
	if p.Search == drafter.SearchOn {
		goes += ", the search provider"
	}
	terms := "The Drafter revises it on this computer, against the model the runner policy names (" + p.ModelName() + "), for no credits. " +
		"AI Review (TypeSafe's Jev) reads the instructions and the folder's agent.yaml and " + rev.InstructionFile + " first, with the person's key, then every search result and page before the model does. " +
		"The instructions and those files go to " + goes + " and the sites the Drafter reads. The run is kept in Recent runs."
	return &pendingRevision{
		terms:   terms,
		cleanup: func() { _ = os.RemoveAll(stage) },
		run: func(context.Context) (*revisedFiles, error) {
			var res *draft.Result
			err := b.run.asRun(jobDraft, "a revision by the Drafter", draftRunRef, eventDraftLog, func(ctx context.Context, runID string, logw io.Writer) error {
				outDir, err := run.OutputDir("", drafter.Namespace, drafter.Name, runID)
				if err != nil {
					return err
				}
				staged, err := drafter.ReadRelease(dir, filepath.Join(outDir, drafter.ReviseInput), "", info.Digest)
				if err != nil {
					return err
				}
				p.Revision.Dir = staged.Dir
				_, _ = fmt.Fprintf(logw, "Staged %s's agent.yaml and %s (%s) for the Drafter.\n", rev.Ref, rev.InstructionFile, info.Digest)
				res, err = draft.Run(ctx, draft.Request{Plan: p, RunID: runID, Namespace: m.Namespace, Log: logw, Scanner: b.draftScanner, Runner: b.draftRunner, Checker: b.draftChecker,
					OnRepairTurn: b.run.addTurn, Progress: func(p draft.Progress) { emitEvent(b.app, eventDraftProgress, p) }})
				if res != nil {
					b.run.keepFolder(runID, res.OutDir)
				} else {
					b.run.keepFolder(runID, outDir)
				}
				return err
			})
			if err != nil {
				return nil, err
			}
			return revisedFrom(res)
		},
	}, nil
}

// prepareHostedRevision readies the Drafter's revision of the published
// version ref In Agentry: offered only where the person's payer may revise,
// with what a draft holds in credits.
func (b *BuildAPI) prepareHostedRevision(ctx context.Context, url, token, ref, instructions string) (*pendingRevision, error) {
	st, err := b.DescribeInAgentryStatus(url)
	if err != nil {
		return nil, err
	}
	if !st.ReviseAvailable {
		why := cmpOr(st.ReviseReason, "this registry offers no revision In Agentry")
		return nil, fmt.Errorf("a revision In Agentry is not available: %s; have it revised on this computer (where: %s)", strings.TrimSuffix(why, "."), applink.ReviseOnComputer)
	}
	hold := cmpOr(st.HoldSentence, "It spends credits as a draft In Agentry does.")
	terms := "The Drafter revises " + ref + " In Agentry, as the registry holds it. " + hold +
		" The instructions and the Drafter's searches go to a search provider, to AI Review (TypeSafe's Jev) and to the sites the Drafter reads, so they should hold no secret. AI Review reads every result before the Drafter's model does, and holds back what it flags." +
		" The run is kept in Recent runs."
	return &pendingRevision{
		terms: terms,
		run: func(ctx context.Context) (*revisedFiles, error) {
			return b.reviseHosted(ctx, url, token, ref, instructions)
		},
	}, nil
}

// reviseHosted creates the revision In Agentry, follows it to its end and
// reads its draft.
func (b *BuildAPI) reviseHosted(ctx context.Context, url, token, ref, instructions string) (*revisedFiles, error) {
	if err := b.claimHosted(); err != nil {
		return nil, err
	}
	defer b.releaseHosted()
	client := hostedrun.New(url, token)
	created, err := client.Create(ctx, hostedrun.CreateRequest{Kind: hostedrun.KindDraft, Revises: ref, Instructions: instructions})
	if err != nil {
		if r := hostedrun.RefusalOf(err); r != nil {
			return nil, fmt.Errorf("the registry refused the revision: %s", quoted(cmpOr(r.Detail, r.Title)))
		}
		return nil, err
	}
	if !created.IsDraft() || !validRunID(created.ID) {
		return nil, fmt.Errorf("the registry answered the revision with %q, which is not a draft", created.ID)
	}
	final, err := client.Watch(ctx, created.ID, hostedrun.WatchOptions{Poll: hostedPoll})
	if err != nil {
		return nil, fmt.Errorf("lost touch with the registry while the revision ran (%w); it is in Recent runs", err)
	}
	if final.ID != created.ID {
		return nil, fmt.Errorf("the registry answered the revision %s as %q", created.ID, final.ID)
	}
	if final.Status != hostedrun.StatusFinished || final.Outcome != hostedrun.OutcomeSucceeded {
		return nil, fmt.Errorf("the revision In Agentry ended %s (%s) without a draft", final.Status, cmpOr(final.Outcome, "no outcome"))
	}
	outDir, err := run.OutputDir("", drafter.Namespace, drafter.Name, created.ID)
	if err != nil {
		return nil, err
	}
	if _, err := client.DownloadOutputs(ctx, final, outDir); err != nil {
		return nil, fmt.Errorf("the revision's output could not be downloaded: %w", err)
	}
	b.run.rememberFolder(created.ID, outDir)
	recordHosted(b.app, url, final, outDir)
	ns, _, _ := strings.Cut(ref, "/")
	res, err := draft.ReadHosted(outDir, ns)
	if err != nil {
		return nil, err
	}
	if d := res.Draft; d == nil || d.Revises == nil || d.Revises.Ref != ref {
		return nil, fmt.Errorf("the draft does not say it revises %s", ref)
	}
	return revisedFrom(res)
}

// revisedFrom renders a revision's draft as its package and takes the
// files a QuickAgent's folder takes from it: agent.yaml, the instruction
// and the persona. The README and the licence the folder already has stay.
func revisedFrom(res *draft.Result) (*revisedFiles, error) {
	if res == nil || res.Draft == nil {
		return nil, errors.New("the Drafter answered no revision")
	}
	d := res.Draft
	tmp, err := os.MkdirTemp("", "agentry-revision-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	pkg := filepath.Join(tmp, "package")
	m, err := drafter.Render(d, pkg)
	if err != nil || m == nil {
		return nil, fmt.Errorf("the revision could not be laid out as a package: %v", cmpOr(errText(err), "its manifest does not parse"))
	}
	out := &revisedFiles{files: map[string][]byte{}, notes: d.Notes}
	for _, p := range []string{"agent.yaml", instructionOf(m), m.PersonaFile()} {
		if p == "" {
			continue
		}
		if !rootFile(p) {
			return nil, fmt.Errorf("the revision names a file %s outside the folder's root", quoted(p))
		}
		b, err := os.ReadFile(filepath.Join(pkg, p)) //nolint:gosec // a file Render wrote in this function's own folder
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out.files[p] = b
	}
	if d.ManifestError != "" {
		out.problems = append(out.problems, d.ManifestError)
	} else if !d.Passed() {
		out.problems = append(out.problems, "the manifest rules or the scanners do not pass the revision; its notes name what fails")
	}
	return out, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
