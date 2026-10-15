package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/dockerimages"
)

type imagesFake struct {
	images  []dockerimages.Image
	removed []string
}

func (f *imagesFake) List(context.Context) ([]dockerimages.Image, error) { return f.images, nil }
func (f *imagesFake) Remove(_ context.Context, ref string) error {
	f.removed = append(f.removed, ref)
	return nil
}

// Run defaults' Remove button: the native dialog names the images and the
// CLI, a No removes nothing, and a Yes removes Agentry's images only. A run
// holding the docker lock refuses it.
func TestRemoveAgentryImagesAsksAndRemovesOnlyAgentrys(t *testing.T) {
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	f := &imagesFake{images: []dockerimages.Image{
		{Repository: "agentry-runner", Tag: "0.6.25", ID: "sha256:1"},
		{Repository: "postgres", Tag: "16", ID: "sha256:2"},
	}}
	var asked string
	answer := false
	r := &RunAPI{app: &App{ctx: context.Background(), confirm: func(_ context.Context, _, msg string) (bool, error) {
		asked = msg
		return answer, nil
	}}, images: f}

	view, err := r.AgentryImages()
	if err != nil || len(view.Images) != 1 || view.Images[0].Ref != "agentry-runner:0.6.25" {
		t.Fatalf("listed %+v, %v", view, err)
	}
	if _, err := r.RemoveAgentryImages(); err == nil || len(f.removed) != 0 {
		t.Fatalf("a No must remove nothing: err=%v removed=%v", err, f.removed)
	}
	if !strings.Contains(asked, "agentry-runner:0.6.25") || !strings.Contains(asked, "CLI") || strings.Contains(asked, "postgres") {
		t.Fatalf("the dialog said %q", asked)
	}
	answer = true
	rep, err := r.RemoveAgentryImages()
	if err != nil || len(rep.Removed) != 1 || len(f.removed) != 1 || f.removed[0] != "sha256:1" || rep.Removed[0] != "agentry-runner:0.6.25" {
		t.Fatalf("report %+v, removed %v, err %v", rep, f.removed, err)
	}

	r.mu.Lock()
	_, err = r.RemoveAgentryImages()
	r.mu.Unlock()
	if !errors.Is(err, errBusy) {
		t.Fatalf("while a run holds the lock: %v, want errBusy", err)
	}
}
