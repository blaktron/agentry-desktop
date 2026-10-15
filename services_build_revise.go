package main

// Edit with AI on the QuickAgent page: the Drafter given a QuickAgent's
// current release and the person's two texts, Additional instructions and
// Problems with this draft, drafts its next version. Two ways:
//
//   - In Agentry, a hosted revision, for a payer
//     above Standard at a hosted draft's terms. Publish this version names
//     its run as the upload's draftRun, which the registry proves against
//     the draft's record before S9 gives it the benefit of the doubt.
//   - On this computer, for anyone the registry says may publish to the
//     QuickAgent (canPublish), with their own model and AI Review
//     key, for no credits. The release is downloaded, its digest checked,
//     and staged under the run's revise/ for the CLI's own revision
//     (pkg/drafter). It leaves no record on the registry, so it publishes
//     under the ordinary S9.
//
// Both land as a revision: the QuickAgent's own name at its next minor,
// what the Builder cannot hold dropped without a word (build.FromRevision),
// and a diff of agent.yaml and the instruction file against the release,
// both sides written by the form's own writer. Publish this version saves
// the revision as drafted into a folder the native picker gave and submits
// it; Open in the Builder puts it in the form, where it is the person's own
// change and names no run.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// RevisionView is what a landed revision adds to its DraftView.
type RevisionView struct {
	// Namespace and Name are the QuickAgent's; From is the release
	// revised and Version the next minor the revision is set to.
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	From      string `json:"from"`
	Version   string `json:"version"`
	// Hosted says it was drafted In Agentry: Publish this version names
	// its run (draftRun). One drafted on this computer names none.
	Hosted bool `json:"hosted"`
	// Key names this landing. Publish this version takes it back, so it
	// publishes the revision on screen and no other (a later one may have
	// landed since).
	Key string `json:"key"`
	// Diff is agent.yaml and the instruction file, the release's against
	// the revision's, both as the form writes them.
	Diff []build.DiffFile `json:"diff"`
}

// landedRevision is the revision Publish this version publishes: Go's own
// copy, never the page's, so what is submitted with a run's id is the
// draft as it landed.
type landedRevision struct {
	key   string
	reg   string
	runID string
	model build.Model
}

// revisedSlot holds the last revision that landed.
type revisedSlot struct {
	mu  sync.Mutex
	cur *landedRevision
}

func (s *revisedSlot) set(r *landedRevision) {
	s.mu.Lock()
	s.cur = r
	s.mu.Unlock()
}

func (s *revisedSlot) get() *landedRevision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// ReviseInAgentry drafts the next version of ns/name's current release In
// Agentry: the texts held to the registry's rule, the release read as Edit
// reads it (one refusal for a QuickAgent this sign-in may not publish to),
// and the hosted revision created and followed as a draft is. The registry
// checks again who may revise it and the payer's tier (403 revise-tier).
func (b *BuildAPI) ReviseInAgentry(reg, ns, name, instructions, problems string) (*HostedDraftStartView, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, fmt.Errorf("a revision In Agentry spends credits, so it needs a signed-in registry: %w", err)
	}
	instructions, problems, err = hostedrun.CheckRevision(instructions, problems)
	if err != nil {
		return nil, err
	}
	rel, err := readRelease(b.app.ctx, url, token, ns, name, "")
	if err != nil {
		return nil, err
	}
	if err := b.claimHosted(); err != nil {
		return nil, err
	}
	claimed := true
	defer func() {
		if claimed {
			b.releaseHosted()
		}
	}()
	client := hostedrun.New(url, token)
	created, err := client.Create(b.app.ctx, hostedrun.CreateRequest{Kind: hostedrun.KindDraft,
		Revises: ns + "/" + name + "@" + rel.Version, Instructions: instructions, Problems: problems})
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return hostedDraftRefusal(refusal), nil
		}
		return nil, err
	}
	if !created.IsDraft() {
		return nil, fmt.Errorf("the registry answered the revision with a %q, not a draft", created.Kind)
	}
	if !validRunID(created.ID) {
		return nil, fmt.Errorf("the registry named this revision %q, which is not a run id", created.ID)
	}
	claimed = false
	b.followHostedDraft(url, token, created)
	return &HostedDraftStartView{Draft: hostedDraftView(created, false)}, nil
}

// ReviseOnThisComputer drafts the next version of ns/name's current release
// here, as Describe it on this computer drafts: no key, no revision; a
// native dialog saying where the texts and the release go; then, under the
// run lock, the release downloaded, held to its digest and staged under the
// run's revise/, AI Review reading the texts and the release before the
// Drafter does, and the CLI's own post-step. It is offered to anyone the
// registry says may publish to the QuickAgent, and spends no credits.
func (b *BuildAPI) ReviseOnThisComputer(reg, ns, name, instructions, problems string) (*DraftView, error) {
	url, token, err := storedCredential(reg)
	if err != nil {
		return nil, fmt.Errorf("a revision reads the release from the registry, so it needs a signed-in registry: %w", err)
	}
	instructions, problems, err = hostedrun.CheckRevision(instructions, problems)
	if err != nil {
		return nil, err
	}
	ctx := b.app.ctx
	rel, err := readRelease(ctx, url, token, ns, name, "")
	if err != nil {
		return nil, err
	}
	if rel.Digest == "" {
		return nil, fmt.Errorf("the registry gave no archive digest for %s/%s@%s, so the release cannot be checked", ns, name, rel.Version)
	}
	var head struct {
		Summary string `json:"summary"`
	}
	_ = json.Unmarshal(rel.Manifest, &head)
	rev := &drafter.Revision{Revises: drafter.Revises{Ref: ns + "/" + name + "@" + rel.Version, Digest: rel.Digest},
		Name: name, Summary: head.Summary, InstructionFile: rel.Path}
	if err := rev.WithTexts(instructions, problems); err != nil {
		return nil, err
	}
	p, err := draft.PrepareRevision(url, rev)
	if err != nil {
		return nil, err
	}
	goes := "TypeSafe (AI Review), your model's provider"
	if p.Search == drafter.SearchOn {
		goes += ", your search provider"
	}
	if err := b.app.askConsent("Edit "+ns+"/"+name+" with AI on this computer?",
		"The Drafter revises "+rev.Ref+" on this computer, against the model your runner policy names ("+p.ModelName()+"), for no credits. "+
			"AI Review (TypeSafe's Jev) reads your texts and the release's agent.yaml and "+rel.Path+" first, with your key, then every search result and page "+
			"before the model does. It then checks the finished draft with the questions the pipeline asks at publish. "+
			"Your texts and the release go to "+goes+" and the sites the Drafter reads, so they should hold no secret. "+
			"The run's receipts, the revision and a copy of the release's two files are kept in your Downloads folder under agentry-out. "+
			"A revision made here leaves no record in Agentry, so it is scanned as any new version is when you submit it. "+
			"Nothing is submitted until you choose Publish this version. Continue?"); err != nil {
		return nil, err
	}
	var res *draft.Result
	err = b.run.asRun(jobDraft, "a revision by the Drafter", draftRunRef, eventDraftLog, func(ctx context.Context, runID string, logw io.Writer) error {
		outDir, err := run.OutputDir("", drafter.Namespace, drafter.Name, runID)
		if err != nil {
			return err
		}
		staged, err := stageRelease(ctx, url, token, rev.Ref, rev.Digest, filepath.Join(outDir, drafter.ReviseInput))
		if err != nil {
			return err
		}
		if staged.Name != rev.Name || staged.InstructionFile != rev.InstructionFile {
			return fmt.Errorf("the archive of %s does not hold the release the registry described (%s, %s)", rev.Ref, staged.Name, staged.InstructionFile)
		}
		p.Revision.Dir = staged.Dir
		_, _ = fmt.Fprintf(logw, "Read %s (%s) and staged its agent.yaml and %s for the Drafter.\n", rev.Ref, rev.Digest, rev.InstructionFile)
		res, err = draft.Run(ctx, draft.Request{Plan: p, RunID: runID, Namespace: ns, Log: logw, Scanner: b.draftScanner, Runner: b.draftRunner, Checker: b.draftChecker,
			OnRepairTurn: b.run.addTurn, Progress: func(p draft.Progress) { emitEvent(b.app, eventDraftProgress, p) }})
		if res != nil {
			b.run.keepFolder(runID, res.OutDir)
		} else {
			// The staged release is in the run's folder whatever the
			// Drafter did.
			b.run.keepFolder(runID, outDir)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	v := draftView(res)
	v.Where, v.Description = draftHere, revisionWords(instructions, problems)
	if err := b.landRevision(ctx, url, token, v, res.Draft, rev.Ref, "", false); err != nil {
		return nil, err
	}
	return v, nil
}

// stageRelease downloads ref's archive, holds it to digest (the registry's
// record of the release the person chose), unpacks it into a scratch folder
// under the app's extract limits, and stages its agent.yaml and instruction
// file at dest for the Drafter (drafter.ReadRelease). The scratch folder
// is removed on return.
func stageRelease(ctx context.Context, url, token, ref, digest, dest string) (*drafter.Revision, error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, err
	}
	res, err := registry.Resolve(ctx, client, ref, false)
	if err != nil {
		return nil, err
	}
	if got := res.Namespace + "/" + res.Name + "@" + res.Version; got != ref {
		return nil, fmt.Errorf("the registry resolved %s as %s", ref, got)
	}
	if res.Digest != digest {
		return nil, fmt.Errorf("%s changed in the registry since it was read (%s, now %s); start the revision again", ref, digest, res.Digest)
	}
	archive, err := registry.Download(ctx, url, token, res)
	if err != nil {
		return nil, err
	}
	if computed := registry.Digest(archive); computed != digest {
		return nil, fmt.Errorf("digest mismatch: the registry says %s, the archive is %s; the release was not staged", digest, computed)
	}
	scratch, err := os.MkdirTemp("", "agentry-revise-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := pkgarchive.Extract(bytes.NewReader(archive), scratch, int64(len(archive)), registry.ExtractLimits()); err != nil {
		return nil, fmt.Errorf("unpack %s: %w", ref, err)
	}
	return drafter.ReadRelease(scratch, dest, ref, digest)
}

// randomKey names one landed revision.
func randomKey() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// revisionWords are the person's two texts as the draft's words: what the
// summary's unmentioned hosts and the licence rule read.
func revisionWords(instructions, problems string) string {
	return strings.TrimSpace(instructions + "\n\n" + problems)
}

// landRevision makes a landed draft a revision of ref: the release read
// again (the version the draft revised, for the diff, and every version,
// for the next minor), the draft put in the form under the QuickAgent's
// own name (build.FromRevision), the hosts it reaches that neither the
// release nor the person's words do, and the diff. A hosted one's run id is
// kept in Go for Publish this version.
func (b *BuildAPI) landRevision(ctx context.Context, url, token string, v *DraftView, d *drafter.Draft, ref, runID string, hosted bool) error {
	if d == nil {
		return errors.New("the revision holds no draft")
	}
	if d.Revises == nil || d.Revises.Ref != ref {
		return fmt.Errorf("the draft does not say it revises %s", ref)
	}
	ns, rest, ok := strings.Cut(ref, "/")
	name, version, ok2 := strings.Cut(rest, "@")
	if !ok || !ok2 {
		return fmt.Errorf("%q is not a release", ref)
	}
	rel, err := readRelease(ctx, url, token, ns, name, version)
	if err != nil {
		return err
	}
	before, err := build.FromRelease(rel.Manifest, rel.Instructions, ns, name, rel.Next)
	if err != nil {
		return err
	}
	after, err := build.FromRevision(d.Manifest, d.Instructions, ns, name, rel.Next, before.License, v.Description)
	if err != nil {
		return fmt.Errorf("the revision could not be opened in the form: %w", err)
	}
	label := build.ReleaseInstructionPath(rel.Manifest)
	if label != "AGENT.md" {
		label += ", written as AGENT.md"
	}
	v.Model = after
	// What the Builder drops it drops without a word, as Edit does.
	v.DecodeNotes = []string{}
	v.Unmentioned = draft.UnmentionedHosts(after, v.Description+"\n"+before.ManifestYAML())
	v.Revision = &RevisionView{Namespace: ns, Name: name, From: rel.Version, Version: rel.Next, Hosted: hosted,
		Diff: []build.DiffFile{
			build.LineDiff("agent.yaml", before.ManifestYAML(), after.ManifestYAML()),
			build.LineDiff(label, before.Instructions, after.Instructions),
		}}
	key, err := randomKey()
	if err != nil {
		return err
	}
	v.Revision.Key = key
	landed := &landedRevision{key: key, reg: url, model: *after}
	if hosted {
		landed.runID = runID
	}
	b.revised.set(landed)
	return nil
}

// PublishThisVersion is Publish this version: the revision key names, as
// drafted (Go's copy, not the page's), saved into dir, which must be a
// folder the native picker gave that holds no QuickAgent yet, and submitted
// as Submit submits, after the same native warning. A revision drafted In
// Agentry names its run (draftRun); one drafted on this computer names
// none. Only the revision that landed last is kept: an earlier one's key is
// refused, never answered with the later revision.
func (b *BuildAPI) PublishThisVersion(reg, dir, key string) (*build.PublishResult, error) {
	if dir == "" {
		return nil, errors.New("choose a folder to save the revision into first")
	}
	if !b.mu.TryLock() {
		return nil, errBuildBusy
	}
	defer b.mu.Unlock()
	if err := b.app.requirePicked("folder", dir); err != nil {
		return nil, err
	}
	cur := b.revised.get()
	switch {
	case cur == nil:
		return nil, errors.New("there is no revision to publish: Edit with AI first")
	case key == "" || cur.key != key:
		return nil, errors.New("this is not the revision that landed last, which is the one this app keeps: open the last one, or Edit with AI again")
	}
	if settings.Normalize(reg) != cur.reg {
		return nil, fmt.Errorf("the revision was drafted against %s, not %s", originOf(cur.reg), originOf(reg))
	}
	if _, err := os.Stat(filepath.Join(dir, "agent.yaml")); err == nil {
		return nil, fmt.Errorf("%s already holds an agent.yaml; choose an empty folder for the revision", dir)
	}
	m := cur.model
	if err := build.Save(dir, &m); err != nil {
		return nil, err
	}
	return b.submit(reg, dir, cur.runID)
}
