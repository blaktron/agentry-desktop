package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// OpenRecordedDraft reads the selected successful draft through Go's history,
// never through a path supplied by the page. It opens no package or program.
func (b *BuildAPI) OpenRecordedDraft(runID string) (*DraftView, error) {
	if !historyOn() || !validRunID(runID) {
		return nil, errUnknownRun
	}
	e, ok, err := runhistory.Find(runID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errUnknownRun
	}
	if e.Kind != runhistory.KindDraft {
		return nil, errors.New("that run is not a draft by the Drafter")
	}
	if e.Where == runhistory.WhereAgentry {
		return b.OpenHostedDraft(e.Registry, runID)
	}
	if e.Outcome != runhistory.OutcomeOK || e.EndedAt.IsZero() {
		return nil, errors.New("the Drafter did not finish this draft successfully, so there is no draft to open")
	}
	dir, err := recordedFolder(e)
	if err != nil {
		return nil, err
	}
	if !b.mu.TryLock() {
		return nil, errBuildBusy
	}
	defer b.mu.Unlock()
	raw, ok := readRegular(filepath.Join(dir, "draft.json"), draft.MaxDraftJSONBytes)
	if !ok {
		return nil, errors.New("the draft's draft.json is missing, not a regular file, or too large to open")
	}
	ns := ""
	reg := settings.Normalize(e.Registry)
	token := ""
	if settings.ValidateURL(reg) == nil {
		token = settings.TokenFor(reg)
		if token != "" {
			if u, err := whoami(b.app, reg, token); err == nil {
				ns = u.Handle
			}
		}
	}
	d, model, notes, err := draft.Decode(raw, ns)
	if err != nil {
		return nil, err
	}
	res := &draft.Result{Draft: d, Model: model, Notes: notes, OutDir: dir}
	v := draftView(res)
	v.Where = draftHere
	// Local revisions have no hosted provenance. Restore the normal revision
	// view and its Go-kept publish key, which still sends no hosted run id.
	if res.Draft.Revises != nil {
		if err := settings.ValidateURL(reg); err != nil {
			return nil, err
		}
		if err := b.landRevision(b.app.ctx, reg, token, v, res.Draft, res.Draft.Revises.Ref, runID, false); err != nil {
			return nil, err
		}
	} else {
		v.settle(b.app.ctx, reg, token, ns)
	}
	return v, nil
}

// OpenHostedDraft retrieves an existing completed draft with this registry's
// account token. It does not create, follow or charge for another run.
func (b *BuildAPI) OpenHostedDraft(reg, runID string) (*DraftView, error) {
	if !validRunID(runID) {
		return nil, errUnknownRun
	}
	reg = settings.Normalize(reg)
	client, err := hostedClient(reg)
	if err != nil {
		return nil, err
	}
	if !b.mu.TryLock() {
		return nil, errBuildBusy
	}
	defer b.mu.Unlock()
	ctx := b.app.ctx
	h, err := client.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	if h.ID != runID {
		return nil, fmt.Errorf("the registry answered the draft %s as %q", runID, h.ID)
	}
	if h.Kind != hostedrun.KindDraft {
		return nil, errors.New("that run is not a draft by the Drafter")
	}
	if h.Status != hostedrun.StatusFinished || h.Outcome != hostedrun.OutcomeSucceeded {
		return nil, errors.New("the Drafter did not finish this draft successfully, so there is no draft to open")
	}
	if len(h.Outputs) == 0 {
		return nil, errNoHostedOutput
	}
	v := b.landHostedDraft(ctx, client, reg, settings.TokenFor(reg), runID, h)
	if v.DraftError != "" {
		return nil, errors.New(v.DraftError)
	}
	if v.Draft == nil {
		return nil, errors.New("the Drafter returned no draft the form can open")
	}
	return v.Draft, nil
}
