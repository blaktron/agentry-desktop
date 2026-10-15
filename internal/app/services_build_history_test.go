package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
)

func TestARecordedDraftOpensTheSelectedDraftAndKeepsItsReview(t *testing.T) {
	freshConfig(t)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "draft.json"), []byte(draftFakeJSON(t, nil)), 0o600); err != nil {
		t.Fatal(err)
	}
	e := runhistory.Entry{ID: "draft001", Kind: runhistory.KindDraft, Where: runhistory.WhereHere, Outcome: runhistory.OutcomeOK, Folder: out, EndedAt: time.Now()}
	if err := runhistory.Add(e); err != nil {
		t.Fatal(err)
	}
	b := &BuildAPI{app: &App{ctx: context.Background()}}
	v, err := b.OpenRecordedDraft(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Model.Name != "meeting-actions" || v.Model.Instructions != "# Meeting actions\n\nRead the notes.\n" || !strings.Contains(v.Notes, "A first draft") || v.Check == nil || v.Check.Rubric != "flagged" {
		t.Fatalf("draft %+v", v)
	}
	// No result file may override the history's recorded failure.
	e.Outcome = runhistory.OutcomeError
	if err := runhistory.Add(e); err != nil {
		t.Fatal(err)
	}
	if _, err := b.OpenRecordedDraft(e.ID); err == nil {
		t.Fatal("failed draft opened")
	}
	if _, err := b.OpenRecordedDraft(out); err == nil {
		t.Fatal("frontend path accepted")
	}
	if _, err := b.OpenRecordedDraft("unknown1"); err == nil {
		t.Fatal("unknown run opened")
	}
	e.Outcome = runhistory.OutcomeOK
	if err := runhistory.Add(e); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "draft.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.json"), filepath.Join(out, "draft.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.OpenRecordedDraft(e.ID); err == nil {
		t.Fatal("linked draft opened")
	}
}

func TestACompletedHostedDraftOpensWithoutStartingAnotherRun(t *testing.T) {
	f := newDraftFake(t)
	b, _, _ := hostedDraftAPI(t, f)
	f.run = &hostedrun.Run{ID: draftRunID, Kind: hostedrun.KindDraft, Task: "Meeting notes into actions", Draft: &hostedrun.Draft{}}
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, nil), 4)
	v, err := b.OpenHostedDraft(f.url, draftRunID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Model.Name != "meeting-actions-2" || v.Model.Namespace != "chris" || v.Check == nil || v.Check.Rubric != "flagged" || !strings.Contains(v.Notes, "A first draft") {
		t.Fatalf("draft %+v", v)
	}
	if len(f.created) != 0 {
		t.Fatalf("created runs %v", f.created)
	}
	f.run.Outcome = hostedrun.OutcomeFailed
	if _, err := b.OpenHostedDraft(f.url, draftRunID); err == nil {
		t.Fatal("failed draft opened")
	}
	f.run.Outcome = hostedrun.OutcomeSucceeded
	f.set(hostedrun.StatusRunning)
	if _, err := b.OpenHostedDraft(f.url, draftRunID); err == nil {
		t.Fatal("running draft opened")
	}
	f.set(hostedrun.StatusFinished)
	f.run.Kind = hostedrun.KindRun
	if _, err := b.OpenHostedDraft(f.url, draftRunID); err == nil {
		t.Fatal("ordinary run opened as draft")
	}
	f.run.Kind = hostedrun.KindDraft
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, func(d map[string]any) { d["instructions"] = map[string]any{"wrong": "shape"} }), 4)
	if _, err := b.OpenHostedDraft(f.url, draftRunID); err == nil {
		t.Fatal("hostile draft opened")
	}
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, nil), 4)
	f.run.ID = "77777777-7777-7777-7777-777777777777"
	if _, err := b.OpenHostedDraft(f.url, draftRunID); err == nil {
		t.Fatal("registry-renamed draft opened")
	}
}

func TestAHostedRevisionOpenedFromHistoryKeepsItsPublishProvenance(t *testing.T) {
	f := newDraftFake(t)
	uploads := serveRelease(t, f)
	b, _, _ := hostedDraftAPI(t, f)
	f.run = &hostedrun.Run{ID: draftRunID, Kind: hostedrun.KindDraft, Draft: &hostedrun.Draft{Revises: &hostedrun.Revises{Ref: "chris/meeting-actions@1.2.0", Digest: editDigest}, Instructions: "Give every action a date."}}
	f.finish(hostedrun.OutcomeSucceeded, revisedDraftJSON(t), 4)
	v, err := b.OpenHostedDraft(f.url, draftRunID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Revision == nil || !v.Revision.Hosted || v.Revision.Version != "1.5.0" {
		t.Fatalf("revision %+v", v.Revision)
	}
	if len(f.created) != 0 || len(*uploads) != 0 {
		t.Fatal("opening created or published a run")
	}
	dir := t.TempDir()
	b.app.markPicked(dir)
	b.app.confirm = func(context.Context, string, string) (bool, error) { return true, nil }
	if _, err := b.PublishThisVersion(f.url, dir, "another-key"); err == nil {
		t.Fatal("unrelated revision key accepted")
	}
	if _, err := b.PublishThisVersion(f.url, dir, v.Revision.Key); err != nil {
		t.Fatal(err)
	}
	if len(*uploads) != 1 || (*uploads)[0] != draftRunID {
		t.Fatalf("publish provenance %v", *uploads)
	}
}
